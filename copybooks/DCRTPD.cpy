      *> docuconf runtime: paragraphs for a generated loader.
      *> docuconf-cobol generate inlines it into every loader, or the
      *> loader COPYs it (-runtime copy). The loader declares the
      *> termination log file DC-TLOG, with record DC-TLOG-LINE. Each
      *> paragraph works on DC-WORK (DCRTWS.cpy): DC-NAME is the
      *> variable being read, DC-RAW(1:DC-LEN) its value, and a
      *> problem is reported through DC-PROBLEM, which never prints
      *> the value.
      *> Valid in fixed and free format.

      *> Records a problem: DC-NAME, DC-MSG and DC-CODE. DC-REPORT
      *> prints them all at the end.
       DC-PROBLEM.
           ADD 1 TO DC-PROBLEMS
           MOVE "N" TO DC-OK
           IF DC-PROBLEMS <= 100
               MOVE SPACES TO DC-LINE(DC-PROBLEMS)
               STRING "  " DC-NAME(1:DC-NAME-LEN) ": "
                   FUNCTION TRIM(DC-MSG TRAILING) " ("
                   FUNCTION TRIM(DC-CODE TRAILING) ")"
                   DELIMITED BY SIZE INTO DC-LINE(DC-PROBLEMS)
               END-STRING
           END-IF.

      *> Prints every problem on stderr, under one header, as every
      *> docuconf SDK does, and writes the same lines to the
      *> termination log: DOCUCONF_TERMINATION_LOG ("-" for none), else
      *> /dev/termination-log when it exists.
       DC-REPORT.
           MOVE SPACES TO DC-HEAD
           IF DC-PROBLEMS = 1
               MOVE "docuconf: 1 configuration problem:" TO DC-HEAD
           ELSE
               MOVE DC-PROBLEMS TO DC-IDX-ED
               STRING "docuconf: " FUNCTION TRIM(DC-IDX-ED)
                   " configuration problems:"
                   DELIMITED BY SIZE INTO DC-HEAD
               END-STRING
           END-IF
           DISPLAY FUNCTION TRIM(DC-HEAD TRAILING) UPON SYSERR
           END-DISPLAY
           PERFORM VARYING DC-K FROM 1 BY 1
                   UNTIL DC-K > DC-PROBLEMS OR DC-K > 100
               DISPLAY FUNCTION TRIM(DC-LINE(DC-K) TRAILING)
                   UPON SYSERR
               END-DISPLAY
           END-PERFORM
           MOVE SPACES TO DC-TLOG-PATH
           ACCEPT DC-TLOG-PATH
               FROM ENVIRONMENT "DOCUCONF_TERMINATION_LOG"
               ON EXCEPTION
                   MOVE SPACES TO DC-TLOG-PATH
           END-ACCEPT
           IF DC-TLOG-PATH = SPACES
               MOVE "/dev/termination-log" TO DC-TLOG-PATH
               OPEN INPUT DC-TLOG
               IF DC-TLOG-STATUS = "00"
                   CLOSE DC-TLOG
               ELSE
                   MOVE "-" TO DC-TLOG-PATH
               END-IF
           END-IF
           IF DC-TLOG-PATH NOT = "-"
               OPEN OUTPUT DC-TLOG
               IF DC-TLOG-STATUS = "00"
                   MOVE DC-HEAD TO DC-TLOG-LINE
                   WRITE DC-TLOG-LINE END-WRITE
                   PERFORM VARYING DC-K FROM 1 BY 1
                           UNTIL DC-K > DC-PROBLEMS OR DC-K > 100
                       MOVE DC-LINE(DC-K) TO DC-TLOG-LINE
                       WRITE DC-TLOG-LINE END-WRITE
                   END-PERFORM
                   CLOSE DC-TLOG
               END-IF
           END-IF.

       DC-BAD-TYPE.
           MOVE "invalid_type" TO DC-CODE
           PERFORM DC-PROBLEM.

       DC-BAD-RANGE.
           MOVE "out_of_range" TO DC-CODE
           PERFORM DC-PROBLEM.

      *> Reads the variable named in DC-NAME. DC-SET is "N" when it is
      *> not set; DC-LEN is its exact length, trailing spaces included,
      *> so a value is never trimmed (SPEC section 5).
       DC-GET-ENV.
           MOVE FUNCTION LENGTH(FUNCTION TRIM(DC-NAME TRAILING))
               TO DC-NAME-LEN
           MOVE SPACES TO DC-RAW
           MOVE "Y" TO DC-SET
           MOVE "Y" TO DC-OK
           ACCEPT DC-RAW FROM ENVIRONMENT DC-NAME
               ON EXCEPTION
                   MOVE "N" TO DC-SET
           END-ACCEPT
           MOVE 0 TO DC-LEN
           IF DC-SET = "Y"
               PERFORM DC-ENV-LENGTH
               IF DC-ENV-LEN >= LENGTH OF DC-RAW
                   COMPUTE DC-LEN = LENGTH OF DC-RAW - 1
                   MOVE "is longer than the 8191 bytes a loader reads"
                       TO DC-MSG
                   PERFORM DC-BAD-RANGE
               ELSE
                   MOVE DC-ENV-LEN TO DC-LEN
               END-IF
           END-IF.

      *> The length of the value DC-GET-ENV read, into DC-ENV-LEN. A
      *> COBOL field pads a value with spaces, so the length comes from
      *> the C library, strlen(getenv(name)). Where the program cannot
      *> call the C library, it is the length without trailing spaces.
       DC-ENV-LENGTH.
           MOVE LOW-VALUES TO DC-NAME-Z
           MOVE DC-NAME(1:DC-NAME-LEN) TO DC-NAME-Z(1:DC-NAME-LEN)
           MOVE "Y" TO DC-C-OK
           SET DC-ENV-PTR TO NULL
           CALL "getenv" USING BY REFERENCE DC-NAME-Z
               RETURNING DC-ENV-PTR
               ON EXCEPTION
                   MOVE "N" TO DC-C-OK
           END-CALL
           IF DC-C-OK = "Y" AND DC-ENV-PTR NOT = NULL
               CALL "strlen" USING BY VALUE DC-ENV-PTR
                   RETURNING DC-ENV-LEN
                   ON EXCEPTION
                       MOVE "N" TO DC-C-OK
               END-CALL
           ELSE
               MOVE "N" TO DC-C-OK
           END-IF
           IF DC-C-OK = "N"
               MOVE 0 TO DC-ENV-LEN
               IF DC-RAW NOT = SPACES
                   MOVE FUNCTION LENGTH(FUNCTION TRIM(DC-RAW TRAILING))
                       TO DC-ENV-LEN
               END-IF
           END-IF.

      *> An integer: an optional sign and decimal digits, in the 64-bit
      *> range. The result is in DC-INT.
       DC-PARSE-INT.
           MOVE "Y" TO DC-OK
           MOVE 0 TO DC-INT
           MOVE 1 TO DC-P
           IF DC-RAW(1:1) = "-" OR DC-RAW(1:1) = "+"
               MOVE 2 TO DC-P
           END-IF
           IF DC-P > DC-LEN
               MOVE "N" TO DC-OK
           END-IF
           PERFORM VARYING DC-I FROM DC-P BY 1
                   UNTIL DC-I > DC-LEN OR DC-OK = "N"
               IF DC-RAW(DC-I:1) IS NOT NUMERIC
                   MOVE "N" TO DC-OK
               END-IF
           END-PERFORM
           IF DC-OK = "N"
               MOVE "is not an integer" TO DC-MSG
               PERFORM DC-BAD-TYPE
           ELSE
               COMPUTE DC-NUM = FUNCTION NUMVAL(DC-RAW(1:DC-LEN))
                   ON SIZE ERROR
                       MOVE "is outside the 64-bit integer range"
                           TO DC-MSG
                       PERFORM DC-BAD-RANGE
               END-COMPUTE
           END-IF
           IF DC-OK = "Y"
               IF DC-NUM > 9223372036854775807
                       OR DC-NUM + 1 < -9223372036854775807
                   MOVE "is outside the 64-bit integer range" TO DC-MSG
                   PERFORM DC-BAD-RANGE
               ELSE
                   MOVE DC-NUM TO DC-INT
               END-IF
           END-IF.

      *> A decimal or exponent float. NUMVAL-F wants an upper-case E
      *> with a sign, so 1e3 is rewritten as 1E+3 in DC-TMP first.
       DC-PARSE-FLOAT.
           MOVE "Y" TO DC-OK
           MOVE SPACES TO DC-TMP
           MOVE 0 TO DC-TMP-LEN
           IF DC-LEN > 60
               MOVE "N" TO DC-OK
           END-IF
           PERFORM VARYING DC-I FROM 1 BY 1
                   UNTIL DC-I > DC-LEN OR DC-OK = "N"
               MOVE FUNCTION UPPER-CASE(DC-RAW(DC-I:1)) TO DC-C
               ADD 1 TO DC-TMP-LEN
               MOVE DC-C TO DC-TMP(DC-TMP-LEN:1)
               IF DC-C = "E" AND DC-I < DC-LEN
                   IF DC-RAW(DC-I + 1:1) IS NUMERIC
                       ADD 1 TO DC-TMP-LEN
                       MOVE "+" TO DC-TMP(DC-TMP-LEN:1)
                   END-IF
               END-IF
           END-PERFORM
           IF DC-OK = "Y"
               IF FUNCTION TEST-NUMVAL-F(DC-TMP(1:DC-TMP-LEN)) NOT = 0
                   MOVE "N" TO DC-OK
               END-IF
           END-IF
           IF DC-OK = "Y"
               PERFORM DC-FLOAT-MAG
           END-IF
           IF DC-OK = "N"
               MOVE "is not a number" TO DC-MSG
               PERFORM DC-BAD-TYPE
           END-IF.

      *> DC-OK is "N" when DC-RAW(1:DC-LEN), a decimal float of SPEC
      *> section 5's form, rounds beyond the largest double: it is not
      *> finite, as 1e400 is not. DC-MAG is the power of ten of its
      *> first significant digit; at 308 its digits are compared with
      *> those of 2^1024 - 2^970, the first value that rounds to
      *> infinity.
       DC-FLOAT-MAG.
           MOVE 0 TO DC-MAG
           MOVE 0 TO DC-SIG-LEN
           MOVE ALL "0" TO DC-SIG
           MOVE "N" TO DC-IN-TIME
           MOVE 1 TO DC-P
           IF DC-RAW(1:1) = "-" OR DC-RAW(1:1) = "+"
               MOVE 2 TO DC-P
           END-IF
           PERFORM VARYING DC-I FROM DC-P BY 1
                   UNTIL DC-I > DC-LEN
                       OR DC-RAW(DC-I:1) = "e" OR DC-RAW(DC-I:1) = "E"
               EVALUATE TRUE
                   WHEN DC-RAW(DC-I:1) = "."
                       MOVE "Y" TO DC-IN-TIME
                   WHEN DC-SIG-LEN = 0 AND DC-RAW(DC-I:1) = "0"
                       IF DC-IN-TIME = "Y"
                           SUBTRACT 1 FROM DC-MAG
                       END-IF
                   WHEN OTHER
                       IF DC-SIG-LEN = 0 AND DC-IN-TIME = "Y"
                           SUBTRACT 1 FROM DC-MAG
                       END-IF
                       IF DC-IN-TIME = "N" AND DC-SIG-LEN > 0
                           ADD 1 TO DC-MAG
                       END-IF
                       IF DC-SIG-LEN < 40
                           ADD 1 TO DC-SIG-LEN
                           MOVE DC-RAW(DC-I:1) TO DC-SIG(DC-SIG-LEN:1)
                       END-IF
               END-EVALUATE
           END-PERFORM
           IF DC-SIG-LEN > 0 AND DC-I < DC-LEN
               ADD 1 TO DC-I
               MOVE "+" TO DC-C
               IF DC-RAW(DC-I:1) = "-" OR DC-RAW(DC-I:1) = "+"
                   MOVE DC-RAW(DC-I:1) TO DC-C
                   ADD 1 TO DC-I
               END-IF
               PERFORM UNTIL DC-I >= DC-LEN OR DC-RAW(DC-I:1) NOT = "0"
                   ADD 1 TO DC-I
               END-PERFORM
               IF DC-LEN - DC-I + 1 > 6
                   IF DC-C = "+"
                       MOVE "N" TO DC-OK
                   END-IF
                   MOVE 0 TO DC-SIG-LEN
               ELSE
                   COMPUTE DC-CP =
                       FUNCTION NUMVAL(DC-RAW(DC-I:DC-LEN - DC-I + 1))
                   IF DC-C = "-"
                       SUBTRACT DC-CP FROM DC-MAG
                   ELSE
                       ADD DC-CP TO DC-MAG
                   END-IF
               END-IF
           END-IF
           IF DC-SIG-LEN > 0
               IF DC-MAG > 308
                   MOVE "N" TO DC-OK
               END-IF
               IF DC-MAG = 308 AND DC-SIG >= DC-INF-DIGITS
                   MOVE "N" TO DC-OK
               END-IF
           END-IF.

      *> true or false, in any case. DC-BOOL is "Y" or "N".
       DC-PARSE-BOOL.
           MOVE "Y" TO DC-OK
           MOVE SPACES TO DC-TMP
           IF DC-LEN = 4 OR DC-LEN = 5
               MOVE FUNCTION LOWER-CASE(DC-RAW(1:DC-LEN)) TO DC-TMP
           END-IF
           EVALUATE DC-TMP
               WHEN "true"
                   MOVE "Y" TO DC-BOOL
               WHEN "false"
                   MOVE "N" TO DC-BOOL
               WHEN OTHER
                   MOVE "is not true or false" TO DC-MSG
                   PERFORM DC-BAD-TYPE
           END-EVALUATE.

      *> A duration in the encoding named in DC-ENC, as nanoseconds in
      *> DC-NS.
       DC-PARSE-DURATION.
           MOVE "Y" TO DC-OK
           MOVE 0 TO DC-NS
           MOVE "N" TO DC-NEG
           MOVE 1 TO DC-P
           IF DC-RAW(1:1) = "-" OR DC-RAW(1:1) = "+"
               IF DC-RAW(1:1) = "-"
                   MOVE "Y" TO DC-NEG
               END-IF
               MOVE 2 TO DC-P
           END-IF
           IF DC-P > DC-LEN
               MOVE "N" TO DC-OK
           END-IF
           IF DC-OK = "Y"
               EVALUATE DC-ENC
                   WHEN "go"
                       PERFORM DC-DUR-GO
                   WHEN "iso8601"
                       PERFORM DC-DUR-ISO
                   WHEN "seconds"
                       PERFORM DC-DUR-SECONDS
                   WHEN "timespan"
                       PERFORM DC-DUR-TIMESPAN
               END-EVALUATE
           END-IF
           IF DC-OK = "Y"
               IF DC-NEG = "Y"
                   COMPUTE DC-NS = 0 - DC-NS
               END-IF
               IF DC-NS > 9223372036854775807
                       OR DC-NS + 1 < -9223372036854775807
                   MOVE "N" TO DC-OK
               END-IF
           END-IF
           IF DC-OK = "N"
               MOVE "is not a duration in the contract's encoding"
                   TO DC-MSG
               PERFORM DC-BAD-TYPE
           END-IF.

      *> Reads a decimal number at DC-P into DC-NUM, with a point or,
      *> in ISO 8601, a comma before its fraction; DC-OK is "N" when
      *> there is none.
       DC-DUR-NUMBER.
           MOVE DC-P TO DC-Q
           PERFORM UNTIL DC-P > DC-LEN
                   OR (DC-RAW(DC-P:1) IS NOT NUMERIC
                       AND DC-RAW(DC-P:1) NOT = "."
                       AND DC-RAW(DC-P:1) NOT = ",")
               ADD 1 TO DC-P
           END-PERFORM
           IF DC-P = DC-Q OR DC-P - DC-Q > 60
               MOVE "N" TO DC-OK
           ELSE
               MOVE SPACES TO DC-TMP
               MOVE DC-RAW(DC-Q:DC-P - DC-Q) TO DC-TMP
               INSPECT DC-TMP REPLACING ALL "," BY "."
               IF DC-TMP = "."
                   MOVE "N" TO DC-OK
               ELSE
                   COMPUTE DC-NUM =
                       FUNCTION NUMVAL(DC-TMP(1:DC-P - DC-Q))
                       ON SIZE ERROR
                           MOVE "N" TO DC-OK
                   END-COMPUTE
               END-IF
           END-IF.

      *> Go: 1h2m3.5s, units ns, us, ms, s, m and h.
       DC-DUR-GO.
           IF DC-RAW(DC-P:DC-LEN - DC-P + 1) = "0"
               MOVE DC-LEN TO DC-P
               ADD 1 TO DC-P
           END-IF
           PERFORM UNTIL DC-P > DC-LEN OR DC-OK = "N"
               PERFORM DC-DUR-NUMBER
               MOVE DC-P TO DC-Q
               PERFORM UNTIL DC-P > DC-LEN
                       OR DC-RAW(DC-P:1) IS NUMERIC
                       OR DC-RAW(DC-P:1) = "."
                   ADD 1 TO DC-P
               END-PERFORM
               MOVE SPACES TO DC-UNIT
               IF DC-P = DC-Q OR DC-P - DC-Q > 3
                   MOVE "N" TO DC-OK
               ELSE
                   MOVE DC-RAW(DC-Q:DC-P - DC-Q) TO DC-UNIT
               END-IF
               EVALUATE DC-UNIT
                   WHEN "ns"
                       MOVE 1 TO DC-UNIT-NS
                   WHEN "us"
                   WHEN X"C2B573"
                   WHEN X"CEBC73"
                       MOVE 1000 TO DC-UNIT-NS
                   WHEN "ms"
                       MOVE 1000000 TO DC-UNIT-NS
                   WHEN "s"
                       MOVE 1000000000 TO DC-UNIT-NS
                   WHEN "m"
                       MOVE 60000000000 TO DC-UNIT-NS
                   WHEN "h"
                       MOVE 3600000000000 TO DC-UNIT-NS
                   WHEN OTHER
                       MOVE "N" TO DC-OK
               END-EVALUATE
               IF DC-OK = "Y"
                   COMPUTE DC-NS = DC-NS + DC-NUM * DC-UNIT-NS
                       ON SIZE ERROR
                           MOVE "N" TO DC-OK
                   END-COMPUTE
               END-IF
           END-PERFORM.

      *> ISO 8601: PnDTnHnMnS, with a fraction on the last component.
      *> Years and months have no fixed length and are rejected.
       DC-DUR-ISO.
           IF DC-RAW(DC-P:1) NOT = "P" AND DC-RAW(DC-P:1) NOT = "p"
               MOVE "N" TO DC-OK
           END-IF
           ADD 1 TO DC-P
           MOVE "N" TO DC-IN-TIME
           IF DC-P > DC-LEN
               MOVE "N" TO DC-OK
           END-IF
           PERFORM UNTIL DC-P > DC-LEN OR DC-OK = "N"
               MOVE FUNCTION UPPER-CASE(DC-RAW(DC-P:1)) TO DC-C
               IF DC-C = "T" AND DC-IN-TIME = "N"
                   MOVE "Y" TO DC-IN-TIME
                   ADD 1 TO DC-P
                   IF DC-P > DC-LEN
                       MOVE "N" TO DC-OK
                   END-IF
               ELSE
                   PERFORM DC-DUR-NUMBER
                   IF DC-P > DC-LEN
                       MOVE "N" TO DC-OK
                   ELSE
                       MOVE FUNCTION UPPER-CASE(DC-RAW(DC-P:1)) TO DC-C
                       ADD 1 TO DC-P
                   END-IF
                   MOVE 0 TO DC-UNIT-NS
                   IF DC-OK = "Y" AND DC-IN-TIME = "N"
                       EVALUATE DC-C
                           WHEN "W"
                               MOVE 604800000000000 TO DC-UNIT-NS
                           WHEN "D"
                               MOVE 86400000000000 TO DC-UNIT-NS
                       END-EVALUATE
                   END-IF
                   IF DC-OK = "Y" AND DC-IN-TIME = "Y"
                       EVALUATE DC-C
                           WHEN "H"
                               MOVE 3600000000000 TO DC-UNIT-NS
                           WHEN "M"
                               MOVE 60000000000 TO DC-UNIT-NS
                           WHEN "S"
                               MOVE 1000000000 TO DC-UNIT-NS
                       END-EVALUATE
                   END-IF
                   IF DC-UNIT-NS = 0
                       MOVE "N" TO DC-OK
                   END-IF
                   IF DC-OK = "Y"
                       COMPUTE DC-NS = DC-NS + DC-NUM * DC-UNIT-NS
                           ON SIZE ERROR
                               MOVE "N" TO DC-OK
                       END-COMPUTE
                   END-IF
               END-IF
           END-PERFORM.

      *> A plain number of seconds, such as 90 or 0.25.
       DC-DUR-SECONDS.
           PERFORM DC-DUR-NUMBER
           IF DC-P <= DC-LEN
               MOVE "N" TO DC-OK
           END-IF
           IF DC-OK = "Y"
               COMPUTE DC-NS = DC-NUM * 1000000000
                   ON SIZE ERROR
                       MOVE "N" TO DC-OK
               END-COMPUTE
           END-IF.

      *> .NET TimeSpan: [d.]hh:mm:ss[.fffffff].
       DC-DUR-TIMESPAN.
           MOVE SPACES TO DC-PART(1) DC-PART(2) DC-PART(3) DC-PART(4)
           MOVE 0 TO DC-PARTS
           UNSTRING DC-RAW(DC-P:DC-LEN - DC-P + 1) DELIMITED BY ":"
               INTO DC-PART(1) DC-PART(2) DC-PART(3) DC-PART(4)
               TALLYING IN DC-PARTS
           END-UNSTRING
           IF DC-PARTS NOT = 3
               MOVE "N" TO DC-OK
           END-IF
           MOVE 0 TO DC-DAYS
           IF DC-OK = "Y"
               MOVE 0 TO DC-I
               INSPECT DC-PART(1) TALLYING DC-I FOR ALL "."
               IF DC-I > 0
                   UNSTRING DC-PART(1) DELIMITED BY "."
                       INTO DC-TMP DC-PART(4)
                   END-UNSTRING
                   MOVE DC-PART(4) TO DC-PART(1)
                   IF FUNCTION TEST-NUMVAL(DC-TMP) NOT = 0
                       MOVE "N" TO DC-OK
                   ELSE
                       COMPUTE DC-DAYS = FUNCTION NUMVAL(DC-TMP)
                   END-IF
               END-IF
           END-IF
           IF DC-OK = "Y"
               IF FUNCTION TEST-NUMVAL(DC-PART(1)) NOT = 0
                       OR FUNCTION TEST-NUMVAL(DC-PART(2)) NOT = 0
                       OR FUNCTION TEST-NUMVAL(DC-PART(3)) NOT = 0
                   MOVE "N" TO DC-OK
               END-IF
           END-IF
           IF DC-OK = "Y"
               COMPUTE DC-NS =
                   ((DC-DAYS * 24 + FUNCTION NUMVAL(DC-PART(1))) * 60
                     + FUNCTION NUMVAL(DC-PART(2))) * 60000000000
                   + FUNCTION NUMVAL(DC-PART(3)) * 1000000000
                   ON SIZE ERROR
                       MOVE "N" TO DC-OK
               END-COMPUTE
           END-IF.

      *> Splits DC-RAW(1:DC-LEN) on DC-SEP(1:DC-SEP-LEN) into DC-ITEMS.
       DC-SPLIT-CSV.
           MOVE 0 TO DC-ITEM-COUNT
           PERFORM DC-NEXT-ITEM
           MOVE 1 TO DC-P
           PERFORM UNTIL DC-P > DC-LEN OR DC-OK = "N"
               IF DC-P + DC-SEP-LEN - 1 <= DC-LEN
                       AND DC-RAW(DC-P:DC-SEP-LEN)
                           = DC-SEP(1:DC-SEP-LEN)
                   ADD DC-SEP-LEN TO DC-P
                   PERFORM DC-NEXT-ITEM
               ELSE
                   MOVE DC-RAW(DC-P:1) TO DC-C
                   PERFORM DC-ITEM-ADD-BYTE
                   ADD 1 TO DC-P
               END-IF
           END-PERFORM.

      *> Starts the next item, failing when the field's table is full.
       DC-NEXT-ITEM.
           IF DC-ITEM-COUNT >= DC-ITEM-LIMIT
               MOVE "has more items than its COBOL table holds"
                   TO DC-MSG
               MOVE "too_many_items" TO DC-CODE
               PERFORM DC-PROBLEM
           ELSE
               ADD 1 TO DC-ITEM-COUNT
               MOVE 0 TO DC-ITEM-LEN(DC-ITEM-COUNT)
               MOVE SPACES TO DC-ITEM(DC-ITEM-COUNT)
               MOVE "S" TO DC-ITEM-KIND(DC-ITEM-COUNT)
           END-IF.

      *> Appends DC-C to the current item.
       DC-ITEM-ADD-BYTE.
           IF DC-ITEM-LEN(DC-ITEM-COUNT) >= LENGTH OF DC-ITEM(1)
               MOVE "has an item longer than 1024 bytes" TO DC-MSG
               PERFORM DC-BAD-RANGE
           ELSE
               ADD 1 TO DC-ITEM-LEN(DC-ITEM-COUNT)
               MOVE DC-C TO DC-ITEM(DC-ITEM-COUNT)
                   (DC-ITEM-LEN(DC-ITEM-COUNT):1)
           END-IF.

      *> Reads DC-NAME__0, DC-NAME__1, ... into DC-ITEMS until one is
      *> not set. DC-SET is "Y" when there is at least one item.
       DC-READ-INDEXED.
           MOVE DC-NAME TO DC-BASE
           MOVE 0 TO DC-ITEM-COUNT
           MOVE "M" TO DC-NEG
           PERFORM VARYING DC-I FROM 0 BY 1
                   UNTIL DC-I > DC-ITEM-LIMIT OR DC-NEG NOT = "M"
               MOVE DC-I TO DC-IDX-ED
               MOVE SPACES TO DC-NAME
               STRING FUNCTION TRIM(DC-BASE) "__"
                   FUNCTION TRIM(DC-IDX-ED)
                   DELIMITED BY SIZE INTO DC-NAME
               END-STRING
               PERFORM DC-GET-ENV
               EVALUATE TRUE
                   WHEN DC-SET = "N"
                       MOVE "E" TO DC-NEG
                   WHEN DC-I = DC-ITEM-LIMIT
                       MOVE "E" TO DC-NEG
                       PERFORM DC-BASE-NAME
                       MOVE "has more items than its COBOL table holds"
                           TO DC-MSG
                       MOVE "too_many_items" TO DC-CODE
                       PERFORM DC-PROBLEM
                   WHEN DC-LEN > LENGTH OF DC-ITEM(1)
                       MOVE "E" TO DC-NEG
                       MOVE "has an item longer than 1024 bytes"
                           TO DC-MSG
                       PERFORM DC-BAD-RANGE
                   WHEN OTHER
                       ADD 1 TO DC-ITEM-COUNT
                       MOVE DC-RAW(1:1024) TO DC-ITEM(DC-ITEM-COUNT)
                       MOVE DC-LEN TO DC-ITEM-LEN(DC-ITEM-COUNT)
                       MOVE "S" TO DC-ITEM-KIND(DC-ITEM-COUNT)
               END-EVALUATE
           END-PERFORM
           PERFORM DC-BASE-NAME
           MOVE "N" TO DC-SET
           IF DC-ITEM-COUNT > 0
               MOVE "Y" TO DC-SET
           END-IF.

       DC-BASE-NAME.
           MOVE DC-BASE TO DC-NAME
           MOVE FUNCTION LENGTH(FUNCTION TRIM(DC-NAME TRAILING))
               TO DC-NAME-LEN.

      *> Parses a JSON array of strings and numbers into DC-ITEMS, with
      *> DC-ITEM-KIND "S" for a string and "N" for a number.
       DC-SPLIT-JSON.
           MOVE 0 TO DC-ITEM-COUNT
           MOVE DC-PROBLEMS TO DC-PROBLEMS-AT
           MOVE 1 TO DC-P
           PERFORM DC-JSON-SPACE
           IF DC-P > DC-LEN
               MOVE "N" TO DC-OK
           ELSE
               IF DC-RAW(DC-P:1) NOT = "["
                   MOVE "N" TO DC-OK
               END-IF
           END-IF
           ADD 1 TO DC-P
           PERFORM DC-JSON-SPACE
           IF DC-OK = "Y" AND DC-P <= DC-LEN
               IF DC-RAW(DC-P:1) = "]"
                   ADD 1 TO DC-P
               ELSE
                   MOVE "M" TO DC-C
                   PERFORM UNTIL DC-C NOT = "M" OR DC-OK = "N"
                       PERFORM DC-JSON-ITEM
                       PERFORM DC-JSON-SPACE
                       IF DC-P > DC-LEN
                           MOVE "N" TO DC-OK
                       ELSE
                           EVALUATE DC-RAW(DC-P:1)
                               WHEN ","
                                   ADD 1 TO DC-P
                                   PERFORM DC-JSON-SPACE
                                   MOVE "M" TO DC-C
                               WHEN "]"
                                   ADD 1 TO DC-P
                                   MOVE "E" TO DC-C
                               WHEN OTHER
                                   MOVE "N" TO DC-OK
                           END-EVALUATE
                       END-IF
                   END-PERFORM
               END-IF
           ELSE
               MOVE "N" TO DC-OK
           END-IF
           PERFORM DC-JSON-SPACE
           IF DC-P <= DC-LEN
               MOVE "N" TO DC-OK
           END-IF
           IF DC-OK = "N" AND DC-PROBLEMS = DC-PROBLEMS-AT
               MOVE "is not a JSON array of strings or numbers"
                   TO DC-MSG
               PERFORM DC-BAD-TYPE
           END-IF.

       DC-JSON-SPACE.
           PERFORM UNTIL DC-P > DC-LEN
                   OR (DC-RAW(DC-P:1) NOT = SPACE
                       AND DC-RAW(DC-P:1) NOT = X"09"
                       AND DC-RAW(DC-P:1) NOT = X"0A"
                       AND DC-RAW(DC-P:1) NOT = X"0D")
               ADD 1 TO DC-P
           END-PERFORM.

       DC-JSON-ITEM.
           PERFORM DC-NEXT-ITEM
           IF DC-OK = "Y"
               IF DC-RAW(DC-P:1) = '"'
                   MOVE "S" TO DC-ITEM-KIND(DC-ITEM-COUNT)
                   ADD 1 TO DC-P
                   PERFORM DC-JSON-STRING
               ELSE
                   MOVE "N" TO DC-ITEM-KIND(DC-ITEM-COUNT)
                   MOVE DC-P TO DC-Q
                   PERFORM UNTIL DC-P > DC-LEN
                           OR (DC-RAW(DC-P:1) IS NOT NUMERIC
                               AND DC-RAW(DC-P:1) NOT = "-"
                               AND DC-RAW(DC-P:1) NOT = "+"
                               AND DC-RAW(DC-P:1) NOT = "."
                               AND DC-RAW(DC-P:1) NOT = "e"
                               AND DC-RAW(DC-P:1) NOT = "E")
                       MOVE DC-RAW(DC-P:1) TO DC-C
                       PERFORM DC-ITEM-ADD-BYTE
                       ADD 1 TO DC-P
                   END-PERFORM
                   IF DC-P = DC-Q
                       MOVE "N" TO DC-OK
                   END-IF
               END-IF
           END-IF.

      *> A JSON string body, after its opening quote.
       DC-JSON-STRING.
           MOVE "M" TO DC-NEG
           PERFORM UNTIL DC-NEG NOT = "M" OR DC-OK = "N"
               IF DC-P > DC-LEN
                   MOVE "N" TO DC-OK
               ELSE
                   MOVE DC-RAW(DC-P:1) TO DC-C
                   ADD 1 TO DC-P
                   EVALUATE TRUE
                       WHEN DC-C = '"'
                           MOVE "E" TO DC-NEG
                       WHEN DC-C = "\"
                           PERFORM DC-JSON-ESCAPE
                       WHEN OTHER
                           PERFORM DC-ITEM-ADD-BYTE
                   END-EVALUATE
               END-IF
           END-PERFORM.

       DC-JSON-ESCAPE.
           IF DC-P > DC-LEN
               MOVE "N" TO DC-OK
           ELSE
               MOVE DC-RAW(DC-P:1) TO DC-C
               ADD 1 TO DC-P
               EVALUATE DC-C
                   WHEN '"'
                   WHEN "\"
                   WHEN "/"
                       PERFORM DC-ITEM-ADD-BYTE
                   WHEN "b"
                       MOVE X"08" TO DC-C
                       PERFORM DC-ITEM-ADD-BYTE
                   WHEN "f"
                       MOVE X"0C" TO DC-C
                       PERFORM DC-ITEM-ADD-BYTE
                   WHEN "n"
                       MOVE X"0A" TO DC-C
                       PERFORM DC-ITEM-ADD-BYTE
                   WHEN "r"
                       MOVE X"0D" TO DC-C
                       PERFORM DC-ITEM-ADD-BYTE
                   WHEN "t"
                       MOVE X"09" TO DC-C
                       PERFORM DC-ITEM-ADD-BYTE
                   WHEN "u"
                       PERFORM DC-JSON-HEX4
                       IF DC-CP >= 55296 AND DC-CP <= 56319
                           MOVE DC-CP TO DC-CP2
                           IF DC-P + 1 <= DC-LEN
                               IF DC-RAW(DC-P:2) = "\u"
                                   ADD 2 TO DC-P
                                   PERFORM DC-JSON-HEX4
                               ELSE
                                   MOVE "N" TO DC-OK
                               END-IF
                           ELSE
                               MOVE "N" TO DC-OK
                           END-IF
                           IF DC-OK = "Y"
                               IF DC-CP < 56320 OR DC-CP > 57343
                                   MOVE "N" TO DC-OK
                               ELSE
                                   COMPUTE DC-CP = 65536
                                       + (DC-CP2 - 55296) * 1024
                                       + (DC-CP - 56320)
                               END-IF
                           END-IF
                       END-IF
                       IF DC-OK = "Y"
                           PERFORM DC-ITEM-ADD-UTF8
                       END-IF
                   WHEN OTHER
                       MOVE "N" TO DC-OK
               END-EVALUATE
           END-IF.

      *> Four hex digits at DC-P into DC-CP.
       DC-JSON-HEX4.
           MOVE 0 TO DC-CP
           IF DC-P + 3 > DC-LEN
               MOVE "N" TO DC-OK
           ELSE
               PERFORM VARYING DC-J FROM 0 BY 1 UNTIL DC-J > 3
                   MOVE FUNCTION UPPER-CASE(DC-RAW(DC-P + DC-J:1))
                       TO DC-C
                   EVALUATE TRUE
                       WHEN DC-C IS NUMERIC
                           COMPUTE DC-CP = DC-CP * 16
                               + FUNCTION ORD(DC-C) - FUNCTION ORD("0")
                       WHEN DC-C >= "A" AND DC-C <= "F"
                           COMPUTE DC-CP = DC-CP * 16 + 10
                               + FUNCTION ORD(DC-C) - FUNCTION ORD("A")
                       WHEN OTHER
                           MOVE "N" TO DC-OK
                   END-EVALUATE
               END-PERFORM
               ADD 4 TO DC-P
           END-IF.

      *> Appends code point DC-CP to the current item as UTF-8.
       DC-ITEM-ADD-UTF8.
           EVALUATE TRUE
               WHEN DC-CP < 128
                   MOVE FUNCTION CHAR(DC-CP + 1) TO DC-C
                   PERFORM DC-ITEM-ADD-BYTE
               WHEN DC-CP < 2048
                   COMPUTE DC-BYTE = 192 + DC-CP / 64
                   MOVE FUNCTION CHAR(DC-BYTE + 1) TO DC-C
                   PERFORM DC-ITEM-ADD-BYTE
                   COMPUTE DC-BYTE = 128 + FUNCTION MOD(DC-CP, 64)
                   MOVE FUNCTION CHAR(DC-BYTE + 1) TO DC-C
                   PERFORM DC-ITEM-ADD-BYTE
               WHEN DC-CP < 65536
                   COMPUTE DC-BYTE = 224 + DC-CP / 4096
                   MOVE FUNCTION CHAR(DC-BYTE + 1) TO DC-C
                   PERFORM DC-ITEM-ADD-BYTE
                   COMPUTE DC-BYTE = 128
                       + FUNCTION MOD(DC-CP / 64, 64)
                   MOVE FUNCTION CHAR(DC-BYTE + 1) TO DC-C
                   PERFORM DC-ITEM-ADD-BYTE
                   COMPUTE DC-BYTE = 128 + FUNCTION MOD(DC-CP, 64)
                   MOVE FUNCTION CHAR(DC-BYTE + 1) TO DC-C
                   PERFORM DC-ITEM-ADD-BYTE
               WHEN OTHER
                   COMPUTE DC-BYTE = 240 + DC-CP / 262144
                   MOVE FUNCTION CHAR(DC-BYTE + 1) TO DC-C
                   PERFORM DC-ITEM-ADD-BYTE
                   COMPUTE DC-BYTE = 128
                       + FUNCTION MOD(DC-CP / 4096, 64)
                   MOVE FUNCTION CHAR(DC-BYTE + 1) TO DC-C
                   PERFORM DC-ITEM-ADD-BYTE
                   COMPUTE DC-BYTE = 128
                       + FUNCTION MOD(DC-CP / 64, 64)
                   MOVE FUNCTION CHAR(DC-BYTE + 1) TO DC-C
                   PERFORM DC-ITEM-ADD-BYTE
                   COMPUTE DC-BYTE = 128 + FUNCTION MOD(DC-CP, 64)
                   MOVE FUNCTION CHAR(DC-BYTE + 1) TO DC-C
                   PERFORM DC-ITEM-ADD-BYTE
           END-EVALUATE.

      *> Copies item DC-K into DC-RAW and DC-LEN, to parse it.
       DC-ITEM-TO-RAW.
           MOVE SPACES TO DC-RAW
           MOVE DC-ITEM-LEN(DC-K) TO DC-LEN
           IF DC-LEN > 0
               MOVE DC-ITEM(DC-K)(1:DC-LEN) TO DC-RAW
           END-IF.

      *> Puts DOCUCONF_FILE_ROOT in front of the path in DC-RAW, as
      *> every docuconf SDK does for local runs. DC-FROM-ENV is "Y"
      *> when the path came from the input's pathEnv variable: docuconf
      *> exec sets an unset one to the path it checked, the root
      *> (cleaned, as Go's filepath.Join cleans it) already in front,
      *> so a value under the cleaned root is kept as it is.
       DC-APPLY-ROOT.
           MOVE SPACES TO DC-ROOT
           ACCEPT DC-ROOT FROM ENVIRONMENT "DOCUCONF_FILE_ROOT"
               ON EXCEPTION
                   MOVE SPACES TO DC-ROOT
           END-ACCEPT
           IF DC-ROOT NOT = SPACES
               MOVE FUNCTION LENGTH(FUNCTION TRIM(DC-ROOT TRAILING))
                   TO DC-ROOT-LEN
               IF DC-ROOT-LEN > 1 AND DC-ROOT(DC-ROOT-LEN:1) = "/"
                   SUBTRACT 1 FROM DC-ROOT-LEN
               END-IF
           END-IF
           IF DC-ROOT NOT = SPACES AND DC-FROM-ENV = "Y"
               PERFORM DC-CLEAN-ROOT
               EVALUATE TRUE
                   WHEN DC-CROOT(1:DC-CROOT-LEN) = "."
                       IF DC-RAW(1:1) NOT = "/"
                           MOVE SPACES TO DC-ROOT
                       END-IF
                   WHEN DC-CROOT(1:DC-CROOT-LEN) = "/"
                       CONTINUE
                   WHEN DC-LEN > DC-CROOT-LEN
                       IF DC-RAW(1:DC-CROOT-LEN)
                               = DC-CROOT(1:DC-CROOT-LEN)
                               AND DC-RAW(DC-CROOT-LEN + 1:1) = "/"
                           MOVE SPACES TO DC-ROOT
                       END-IF
               END-EVALUATE
           END-IF
           IF DC-ROOT NOT = SPACES
               IF DC-ROOT-LEN + DC-LEN >= LENGTH OF DC-RAW
                   MOVE "is too long with DOCUCONF_FILE_ROOT" TO DC-MSG
                   PERFORM DC-BAD-RANGE
               ELSE
                   MOVE SPACES TO DC-PATH
                   STRING DC-ROOT(1:DC-ROOT-LEN) DC-RAW(1:DC-LEN)
                       DELIMITED BY SIZE INTO DC-PATH
                   END-STRING
                   ADD DC-ROOT-LEN TO DC-LEN
                   MOVE DC-PATH TO DC-RAW
               END-IF
           END-IF.

      *> DC-CROOT(1:DC-CROOT-LEN) is DC-ROOT(1:DC-ROOT-LEN) cleaned
      *> lexically, as Go's filepath.Clean does: no empty or "."
      *> elements, and ".." removing the element before it.
       DC-CLEAN-ROOT.
           MOVE SPACES TO DC-CROOT
           MOVE 0 TO DC-CROOT-LEN
           MOVE 0 TO DC-CBASE
           IF DC-ROOT(1:1) = "/"
               MOVE "/" TO DC-CROOT(1:1)
               MOVE 1 TO DC-CROOT-LEN
               MOVE 1 TO DC-CBASE
           END-IF
           MOVE 1 TO DC-I
           PERFORM UNTIL DC-I > DC-ROOT-LEN
               MOVE DC-I TO DC-J
               PERFORM UNTIL DC-J > DC-ROOT-LEN
                       OR DC-ROOT(DC-J:1) = "/"
                   ADD 1 TO DC-J
               END-PERFORM
               COMPUTE DC-Q = DC-J - DC-I
               EVALUATE TRUE
                   WHEN DC-Q = 0
                       CONTINUE
                   WHEN DC-Q = 1 AND DC-ROOT(DC-I:1) = "."
                       CONTINUE
                   WHEN DC-Q = 2 AND DC-ROOT(DC-I:2) = ".."
                       PERFORM DC-CLEAN-UP
                   WHEN OTHER
                       IF DC-CROOT-LEN > DC-CBASE
                           ADD 1 TO DC-CROOT-LEN
                           MOVE "/" TO DC-CROOT(DC-CROOT-LEN:1)
                       END-IF
                       MOVE DC-ROOT(DC-I:DC-Q)
                           TO DC-CROOT(DC-CROOT-LEN + 1:DC-Q)
                       ADD DC-Q TO DC-CROOT-LEN
               END-EVALUATE
               COMPUTE DC-I = DC-J + 1
           END-PERFORM
           IF DC-CROOT-LEN = 0
               MOVE "." TO DC-CROOT
               MOVE 1 TO DC-CROOT-LEN
           END-IF.

      *> A ".." element: drops the last element of DC-CROOT, unless
      *> there is none (kept at the top of a rooted path) or it is
      *> ".." itself.
       DC-CLEAN-UP.
           MOVE DC-CROOT-LEN TO DC-CI
           PERFORM UNTIL DC-CI <= DC-CBASE
                   OR DC-CROOT(DC-CI:1) = "/"
               SUBTRACT 1 FROM DC-CI
           END-PERFORM
           EVALUATE TRUE
               WHEN DC-CROOT-LEN > DC-CBASE
                       AND DC-CROOT-LEN - DC-CI = 2
                       AND DC-CROOT(DC-CI + 1:2) = ".."
               WHEN DC-CROOT-LEN = DC-CBASE AND DC-CBASE = 0
                   IF DC-CROOT-LEN > 0
                       ADD 1 TO DC-CROOT-LEN
                       MOVE "/" TO DC-CROOT(DC-CROOT-LEN:1)
                   END-IF
                   MOVE ".." TO DC-CROOT(DC-CROOT-LEN + 1:2)
                   ADD 2 TO DC-CROOT-LEN
               WHEN DC-CROOT-LEN > DC-CBASE
                   MOVE DC-CI TO DC-CROOT-LEN
                   IF DC-CROOT-LEN > DC-CBASE
                           AND DC-CROOT(DC-CROOT-LEN:1) = "/"
                       SUBTRACT 1 FROM DC-CROOT-LEN
                   END-IF
               WHEN OTHER
                   CONTINUE
           END-EVALUATE
           MOVE SPACES TO DC-CROOT(DC-CROOT-LEN + 1:).

      *> Counts the characters (UTF-8 code points) of DC-RAW(1:DC-LEN)
      *> into DC-CHARS: every byte but a continuation byte (X"80" to
      *> X"BF") starts one. Length limits count characters, not bytes.
       DC-COUNT-CHARS.
           MOVE 0 TO DC-CHARS
           PERFORM VARYING DC-CI FROM 1 BY 1 UNTIL DC-CI > DC-LEN
               IF DC-RAW(DC-CI:1) < X"80" OR DC-RAW(DC-CI:1) > X"BF"
                   ADD 1 TO DC-CHARS
               END-IF
           END-PERFORM.

      *> DC-REF is "Y" when DC-RAW starts with an injector reference
      *> (vault:, op://, ref+), which a secret must not still hold.
       DC-CHECK-REF.
           MOVE "N" TO DC-REF
           IF (DC-LEN >= 6 AND DC-RAW(1:6) = "vault:")
                   OR (DC-LEN >= 5 AND DC-RAW(1:5) = "op://")
                   OR (DC-LEN >= 4 AND DC-RAW(1:4) = "ref+")
               MOVE "Y" TO DC-REF
           END-IF.

      *> Checks that DC-RAW(1:DC-LEN) is a URL of the form scheme://...
      *> (^[a-zA-Z][a-zA-Z0-9+.-]*://[^\s]+$). DC-OK is "N" when it is
      *> not; else DC-Q is the length of the scheme.
       DC-CHECK-URL.
           MOVE "Y" TO DC-OK
           MOVE 0 TO DC-Q
           IF DC-LEN = 0 OR NOT (DC-RAW(1:1) IS ALPHABETIC)
                   OR DC-RAW(1:1) = SPACE
               MOVE "N" TO DC-OK
           END-IF
           PERFORM VARYING DC-CI FROM 2 BY 1
                   UNTIL DC-CI > DC-LEN OR DC-Q > 0 OR DC-OK = "N"
               EVALUATE TRUE
                   WHEN DC-CI + 2 <= DC-LEN
                           AND DC-RAW(DC-CI:3) = "://"
                       COMPUTE DC-Q = DC-CI - 1
                   WHEN DC-RAW(DC-CI:1) IS ALPHABETIC
                           AND DC-RAW(DC-CI:1) NOT = SPACE
                   WHEN DC-RAW(DC-CI:1) IS NUMERIC
                   WHEN DC-RAW(DC-CI:1) = "+"
                   WHEN DC-RAW(DC-CI:1) = "."
                   WHEN DC-RAW(DC-CI:1) = "-"
                       CONTINUE
                   WHEN OTHER
                       MOVE "N" TO DC-OK
               END-EVALUATE
           END-PERFORM
           IF DC-Q = 0 OR DC-Q + 3 >= DC-LEN
               MOVE "N" TO DC-OK
           END-IF
           IF DC-OK = "Y"
               COMPUTE DC-CI = DC-Q + 4
               PERFORM UNTIL DC-CI > DC-LEN
                   IF DC-RAW(DC-CI:1) = SPACE OR X"09" OR X"0A"
                           OR X"0C" OR X"0D"
                       MOVE "N" TO DC-OK
                   END-IF
                   ADD 1 TO DC-CI
               END-PERFORM
           END-IF.

      *> Checks that DC-RAW(1:DC-LEN) is one JSON document (RFC 8259):
      *> DC-OK is "N" when it is not. Containers nest up to 256 deep.
      *> DC-JSTATE is what comes next: V a value, W a value or "]",
      *> K a key or "}", L a key, C a colon, A what follows a value,
      *> E the end.
       DC-CHECK-JSON.
           MOVE "Y" TO DC-OK
           MOVE 0 TO DC-JDEPTH
           MOVE 1 TO DC-P
           MOVE "V" TO DC-JSTATE
           PERFORM UNTIL DC-OK = "N" OR DC-JSTATE = "E"
               PERFORM DC-JSON-SPACE
               IF DC-P > DC-LEN
                   IF DC-JSTATE = "A" AND DC-JDEPTH = 0
                       MOVE "E" TO DC-JSTATE
                   ELSE
                       MOVE "N" TO DC-OK
                   END-IF
               ELSE
                   MOVE DC-RAW(DC-P:1) TO DC-C
                   EVALUATE DC-JSTATE
                       WHEN "V"
                       WHEN "W"
                           PERFORM DC-JV-VALUE
                       WHEN "K"
                       WHEN "L"
                           EVALUATE TRUE
                               WHEN DC-C = '"'
                                   ADD 1 TO DC-P
                                   PERFORM DC-JV-STRING
                                   MOVE "C" TO DC-JSTATE
                               WHEN DC-C = "}" AND DC-JSTATE = "K"
                                   ADD 1 TO DC-P
                                   SUBTRACT 1 FROM DC-JDEPTH
                                   MOVE "A" TO DC-JSTATE
                               WHEN OTHER
                                   MOVE "N" TO DC-OK
                           END-EVALUATE
                       WHEN "C"
                           IF DC-C = ":"
                               ADD 1 TO DC-P
                               MOVE "V" TO DC-JSTATE
                           ELSE
                               MOVE "N" TO DC-OK
                           END-IF
                       WHEN "A"
                           PERFORM DC-JV-AFTER
                   END-EVALUATE
               END-IF
           END-PERFORM.

      *> A value at DC-P: a scalar, or the start of a container.
       DC-JV-VALUE.
           EVALUATE TRUE
               WHEN DC-C = "{" OR DC-C = "["
                   IF DC-JDEPTH >= LENGTH OF DC-JSTACK
                       MOVE "N" TO DC-OK
                   ELSE
                       ADD 1 TO DC-JDEPTH
                       MOVE DC-C TO DC-JSTACK(DC-JDEPTH:1)
                       ADD 1 TO DC-P
                       IF DC-C = "{"
                           MOVE "K" TO DC-JSTATE
                       ELSE
                           MOVE "W" TO DC-JSTATE
                       END-IF
                   END-IF
               WHEN DC-C = "]" AND DC-JSTATE = "W"
                   ADD 1 TO DC-P
                   SUBTRACT 1 FROM DC-JDEPTH
                   MOVE "A" TO DC-JSTATE
               WHEN DC-C = '"'
                   ADD 1 TO DC-P
                   PERFORM DC-JV-STRING
                   MOVE "A" TO DC-JSTATE
               WHEN DC-C = "t"
                   PERFORM DC-JV-WORD-TRUE
               WHEN DC-C = "f"
                   PERFORM DC-JV-WORD-FALSE
               WHEN DC-C = "n"
                   PERFORM DC-JV-WORD-NULL
               WHEN DC-C = "-" OR DC-C IS NUMERIC
                   PERFORM DC-JV-NUMBER
                   MOVE "A" TO DC-JSTATE
               WHEN OTHER
                   MOVE "N" TO DC-OK
           END-EVALUATE.

       DC-JV-WORD-TRUE.
           IF DC-P + 3 <= DC-LEN AND DC-RAW(DC-P:4) = "true"
               ADD 4 TO DC-P
               MOVE "A" TO DC-JSTATE
           ELSE
               MOVE "N" TO DC-OK
           END-IF.

       DC-JV-WORD-FALSE.
           IF DC-P + 4 <= DC-LEN AND DC-RAW(DC-P:5) = "false"
               ADD 5 TO DC-P
               MOVE "A" TO DC-JSTATE
           ELSE
               MOVE "N" TO DC-OK
           END-IF.

       DC-JV-WORD-NULL.
           IF DC-P + 3 <= DC-LEN AND DC-RAW(DC-P:4) = "null"
               ADD 4 TO DC-P
               MOVE "A" TO DC-JSTATE
           ELSE
               MOVE "N" TO DC-OK
           END-IF.

      *> What may follow a value inside a container: a comma or the
      *> container's closing bracket.
       DC-JV-AFTER.
           EVALUATE TRUE
               WHEN DC-JDEPTH = 0
                   MOVE "N" TO DC-OK
               WHEN DC-C = ","
                   ADD 1 TO DC-P
                   IF DC-JSTACK(DC-JDEPTH:1) = "["
                       MOVE "V" TO DC-JSTATE
                   ELSE
                       MOVE "L" TO DC-JSTATE
                   END-IF
               WHEN (DC-C = "]" AND DC-JSTACK(DC-JDEPTH:1) = "[")
                       OR (DC-C = "}" AND DC-JSTACK(DC-JDEPTH:1) = "{")
                   ADD 1 TO DC-P
                   SUBTRACT 1 FROM DC-JDEPTH
               WHEN OTHER
                   MOVE "N" TO DC-OK
           END-EVALUATE.

      *> A string body after its opening quote: no control character,
      *> and only the escapes JSON defines.
       DC-JV-STRING.
           MOVE "M" TO DC-NEG
           PERFORM UNTIL DC-NEG NOT = "M" OR DC-OK = "N"
               IF DC-P > DC-LEN
                   MOVE "N" TO DC-OK
               ELSE
                   MOVE DC-RAW(DC-P:1) TO DC-C
                   ADD 1 TO DC-P
                   EVALUATE TRUE
                       WHEN DC-C = '"'
                           MOVE "E" TO DC-NEG
                       WHEN DC-C < SPACE
                           MOVE "N" TO DC-OK
                       WHEN DC-C = "\"
                           IF DC-P > DC-LEN
                               MOVE "N" TO DC-OK
                           ELSE
                               MOVE DC-RAW(DC-P:1) TO DC-C
                               ADD 1 TO DC-P
                               EVALUATE DC-C
                                   WHEN '"'
                                   WHEN "\"
                                   WHEN "/"
                                   WHEN "b"
                                   WHEN "f"
                                   WHEN "n"
                                   WHEN "r"
                                   WHEN "t"
                                       CONTINUE
                                   WHEN "u"
                                       PERFORM DC-JSON-HEX4
                                   WHEN OTHER
                                       MOVE "N" TO DC-OK
                               END-EVALUATE
                           END-IF
                   END-EVALUATE
               END-IF
           END-PERFORM.

      *> A number: -?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?
       DC-JV-NUMBER.
           IF DC-RAW(DC-P:1) = "-"
               ADD 1 TO DC-P
           END-IF
           EVALUATE TRUE
               WHEN DC-P > DC-LEN
                   MOVE "N" TO DC-OK
               WHEN DC-RAW(DC-P:1) = "0"
                   ADD 1 TO DC-P
               WHEN DC-RAW(DC-P:1) IS NUMERIC
                   PERFORM DC-JV-DIGITS
               WHEN OTHER
                   MOVE "N" TO DC-OK
           END-EVALUATE
           IF DC-OK = "Y" AND DC-P <= DC-LEN AND DC-RAW(DC-P:1) = "."
               ADD 1 TO DC-P
               PERFORM DC-JV-SOME-DIGITS
           END-IF
           IF DC-OK = "Y" AND DC-P <= DC-LEN
                   AND (DC-RAW(DC-P:1) = "e" OR DC-RAW(DC-P:1) = "E")
               ADD 1 TO DC-P
               IF DC-P <= DC-LEN
                       AND (DC-RAW(DC-P:1) = "+"
                           OR DC-RAW(DC-P:1) = "-")
                   ADD 1 TO DC-P
               END-IF
               PERFORM DC-JV-SOME-DIGITS
           END-IF.

      *> One digit or more at DC-P.
       DC-JV-SOME-DIGITS.
           IF DC-P > DC-LEN OR DC-RAW(DC-P:1) IS NOT NUMERIC
               MOVE "N" TO DC-OK
           ELSE
               PERFORM DC-JV-DIGITS
           END-IF.

       DC-JV-DIGITS.
           PERFORM UNTIL DC-P > DC-LEN
                   OR DC-RAW(DC-P:1) IS NOT NUMERIC
               ADD 1 TO DC-P
           END-PERFORM.

      *> Reports an indexed list whose items do not run from DC-NAME__0
      *> with no gap: an item set after the first unset index, up to the
      *> 1000 items a loader reads. DC-NAME is the list's name. The
      *> list is then set, and wrong, rather than unset.
       DC-INDEXED-GAP.
           MOVE DC-NAME TO DC-BASE
           MOVE DC-OK TO DC-SAVE-OK
           MOVE "N" TO DC-GAP
           COMPUTE DC-J = DC-ITEM-COUNT + 1
           PERFORM VARYING DC-I FROM DC-J BY 1
                   UNTIL DC-I > 1000 OR DC-GAP = "Y"
               MOVE DC-I TO DC-IDX-ED
               MOVE SPACES TO DC-NAME
               STRING FUNCTION TRIM(DC-BASE) "__"
                   FUNCTION TRIM(DC-IDX-ED)
                   DELIMITED BY SIZE INTO DC-NAME
               END-STRING
               MOVE FUNCTION LENGTH(FUNCTION TRIM(DC-NAME TRAILING))
                   TO DC-NAME-LEN
               MOVE "Y" TO DC-GAP
               ACCEPT DC-RAW FROM ENVIRONMENT DC-NAME
                   ON EXCEPTION
                       MOVE "N" TO DC-GAP
               END-ACCEPT
           END-PERFORM
           PERFORM DC-BASE-NAME
           MOVE DC-SAVE-OK TO DC-OK
           IF DC-GAP = "Y"
               MOVE "Y" TO DC-SET
               MOVE "items must be numbered from __0 with no gap"
                   TO DC-MSG
               PERFORM DC-BAD-TYPE
           END-IF.

      *> Whether DC-RAW(1:DC-LEN) matches the compiled pattern in the
      *> DC-RX tables, anywhere in the value: DC-RX-OK is "Y" or "N".
      *> generate compiles the RE2 pattern with Go's regexp/syntax; this
      *> runs the program as a Pike VM over the value's code points, so
      *> it matches exactly what docuconf exec matches.
       DC-RX-MATCH.
           PERFORM DC-RX-DECODE
           MOVE "N" TO DC-RX-OK
           MOVE 0 TO DC-RX-GEN
           PERFORM VARYING DC-RX-I FROM 1 BY 1 UNTIL DC-RX-I > DC-RX-N
               MOVE 0 TO DC-RX-MARK(DC-RX-I)
           END-PERFORM
           MOVE 0 TO DC-RX-SN
           PERFORM VARYING DC-RX-POS FROM 0 BY 1
                   UNTIL DC-RX-POS > DC-RX-NCP OR DC-RX-OK = "Y"
               ADD 1 TO DC-RX-GEN
               MOVE 0 TO DC-RX-CN
               PERFORM DC-RX-CONTEXT
               PERFORM VARYING DC-RX-J FROM 1 BY 1
                       UNTIL DC-RX-J > DC-RX-SN
                   MOVE DC-RX-SEED(DC-RX-J) TO DC-RX-T
                   PERFORM DC-RX-ADD
               END-PERFORM
               MOVE DC-RX-START TO DC-RX-T
               PERFORM DC-RX-ADD
               MOVE 0 TO DC-RX-SN
               IF DC-RX-OK = "N" AND DC-RX-POS < DC-RX-NCP
                   MOVE DC-RX-CP(DC-RX-POS + 1) TO DC-RX-CUR
                   PERFORM VARYING DC-RX-J FROM 1 BY 1
                           UNTIL DC-RX-J > DC-RX-CN
                       MOVE DC-RX-CL(DC-RX-J) TO DC-RX-I
                       PERFORM DC-RX-STEP
                   END-PERFORM
               END-IF
           END-PERFORM.

      *> Decodes DC-RAW(1:DC-LEN) as UTF-8 into DC-RX-CP(1:DC-RX-NCP);
      *> a byte that does not start a valid sequence is U+FFFD, as Go
      *> reads it.
       DC-RX-DECODE.
           MOVE 0 TO DC-RX-NCP
           MOVE 1 TO DC-RX-I
           PERFORM UNTIL DC-RX-I > DC-LEN
               COMPUTE DC-BYTE = FUNCTION ORD(DC-RAW(DC-RX-I:1)) - 1
               ADD 1 TO DC-RX-NCP
               EVALUATE TRUE
                   WHEN DC-BYTE < 128
                       MOVE 0 TO DC-RX-NB
                       MOVE DC-BYTE TO DC-RX-CUR
                   WHEN DC-BYTE >= 194 AND DC-BYTE < 224
                       MOVE 1 TO DC-RX-NB
                       COMPUTE DC-RX-CUR = DC-BYTE - 192
                   WHEN DC-BYTE >= 224 AND DC-BYTE < 240
                       MOVE 2 TO DC-RX-NB
                       COMPUTE DC-RX-CUR = DC-BYTE - 224
                   WHEN DC-BYTE >= 240 AND DC-BYTE < 245
                       MOVE 3 TO DC-RX-NB
                       COMPUTE DC-RX-CUR = DC-BYTE - 240
                   WHEN OTHER
                       MOVE 9 TO DC-RX-NB
               END-EVALUATE
               IF DC-RX-NB > 0 AND DC-RX-NB < 9
                   IF DC-RX-I + DC-RX-NB > DC-LEN
                       MOVE 9 TO DC-RX-NB
                   ELSE
                       PERFORM VARYING DC-RX-K FROM 1 BY 1
                               UNTIL DC-RX-K > DC-RX-NB OR DC-RX-NB = 9
                           COMPUTE DC-BYTE = FUNCTION ORD(
                               DC-RAW(DC-RX-I + DC-RX-K:1)) - 1
                           IF DC-BYTE < 128 OR DC-BYTE > 191
                               MOVE 9 TO DC-RX-NB
                           ELSE
                               COMPUTE DC-RX-CUR = DC-RX-CUR * 64
                                   + DC-BYTE - 128
                           END-IF
                       END-PERFORM
                   END-IF
                   EVALUATE TRUE
                       WHEN DC-RX-NB = 2 AND (DC-RX-CUR < 2048
                               OR (DC-RX-CUR >= 55296
                                   AND DC-RX-CUR <= 57343))
                       WHEN DC-RX-NB = 3 AND (DC-RX-CUR < 65536
                               OR DC-RX-CUR > 1114111)
                           MOVE 9 TO DC-RX-NB
                   END-EVALUATE
               END-IF
               IF DC-RX-NB = 9
                   MOVE 65533 TO DC-RX-CP(DC-RX-NCP)
                   ADD 1 TO DC-RX-I
               ELSE
                   MOVE DC-RX-CUR TO DC-RX-CP(DC-RX-NCP)
                   COMPUTE DC-RX-I = DC-RX-I + DC-RX-NB + 1
               END-IF
           END-PERFORM.

      *> The empty-width flags that hold at position DC-RX-POS, as Go's
      *> syntax.EmptyOpContext: 1 begin line, 2 end line, 4 begin text,
      *> 8 end text, 16 word boundary, 32 no word boundary.
       DC-RX-CONTEXT.
           MOVE 0 TO DC-RX-CTX
           MOVE "N" TO DC-RX-EOK
           MOVE "N" TO DC-RX-HIT
           IF DC-RX-POS = 0
               ADD 5 TO DC-RX-CTX
           ELSE
               MOVE DC-RX-CP(DC-RX-POS) TO DC-RX-PREV
               IF DC-RX-PREV = 10
                   ADD 1 TO DC-RX-CTX
               END-IF
               PERFORM DC-RX-WORD
               MOVE DC-RX-HIT TO DC-RX-EOK
           END-IF
           MOVE "N" TO DC-RX-HIT
           IF DC-RX-POS >= DC-RX-NCP
               ADD 10 TO DC-RX-CTX
           ELSE
               MOVE DC-RX-CP(DC-RX-POS + 1) TO DC-RX-PREV
               IF DC-RX-PREV = 10
                   ADD 2 TO DC-RX-CTX
               END-IF
               PERFORM DC-RX-WORD
           END-IF
           IF DC-RX-EOK NOT = DC-RX-HIT
               ADD 16 TO DC-RX-CTX
           ELSE
               ADD 32 TO DC-RX-CTX
           END-IF.

      *> DC-RX-HIT is "Y" when DC-RX-PREV is an ASCII word character.
       DC-RX-WORD.
           IF (DC-RX-PREV >= 48 AND DC-RX-PREV <= 57)
                   OR (DC-RX-PREV >= 65 AND DC-RX-PREV <= 90)
                   OR (DC-RX-PREV >= 97 AND DC-RX-PREV <= 122)
                   OR DC-RX-PREV = 95
               MOVE "Y" TO DC-RX-HIT
           ELSE
               MOVE "N" TO DC-RX-HIT
           END-IF.

      *> Adds instruction DC-RX-T and what it leads to without reading
      *> a character to the current list, once per position.
       DC-RX-ADD.
           MOVE 1 TO DC-RX-KN
           MOVE DC-RX-T TO DC-RX-STK(1)
           PERFORM UNTIL DC-RX-KN = 0 OR DC-RX-OK = "Y"
               MOVE DC-RX-STK(DC-RX-KN) TO DC-RX-T
               SUBTRACT 1 FROM DC-RX-KN
               IF DC-RX-MARK(DC-RX-T) NOT = DC-RX-GEN
                   MOVE DC-RX-GEN TO DC-RX-MARK(DC-RX-T)
                   EVALUATE DC-RX-OP(DC-RX-T)
                       WHEN "M"
                           MOVE "Y" TO DC-RX-OK
                       WHEN "S"
                           ADD 1 TO DC-RX-KN
                           MOVE DC-RX-ARG(DC-RX-T)
                               TO DC-RX-STK(DC-RX-KN)
                           ADD 1 TO DC-RX-KN
                           MOVE DC-RX-OUT(DC-RX-T)
                               TO DC-RX-STK(DC-RX-KN)
                       WHEN "J"
                           ADD 1 TO DC-RX-KN
                           MOVE DC-RX-OUT(DC-RX-T)
                               TO DC-RX-STK(DC-RX-KN)
                       WHEN "E"
                           PERFORM DC-RX-EMPTY
                           IF DC-RX-EOK = "Y"
                               ADD 1 TO DC-RX-KN
                               MOVE DC-RX-OUT(DC-RX-T)
                                   TO DC-RX-STK(DC-RX-KN)
                           END-IF
                       WHEN "F"
                           CONTINUE
                       WHEN OTHER
                           ADD 1 TO DC-RX-CN
                           MOVE DC-RX-T TO DC-RX-CL(DC-RX-CN)
                   END-EVALUATE
               END-IF
           END-PERFORM.

      *> DC-RX-EOK is "Y" when every flag instruction DC-RX-T needs
      *> holds in DC-RX-CTX.
       DC-RX-EMPTY.
           MOVE "Y" TO DC-RX-EOK
           MOVE 1 TO DC-RX-BIT
           PERFORM 6 TIMES
               COMPUTE DC-RX-A = DC-RX-ARG(DC-RX-T) / DC-RX-BIT
               COMPUTE DC-RX-B = DC-RX-CTX / DC-RX-BIT
               IF FUNCTION MOD(DC-RX-A, 2) = 1
                       AND FUNCTION MOD(DC-RX-B, 2) = 0
                   MOVE "N" TO DC-RX-EOK
               END-IF
               MULTIPLY 2 BY DC-RX-BIT
           END-PERFORM.

      *> Instruction DC-RX-I on character DC-RX-CUR: when it matches,
      *> its next instruction is a seed of the next position.
       DC-RX-STEP.
           MOVE "N" TO DC-RX-HIT
           EVALUATE DC-RX-OP(DC-RX-I)
               WHEN "A"
                   MOVE "Y" TO DC-RX-HIT
               WHEN "N"
                   IF DC-RX-CUR NOT = 10
                       MOVE "Y" TO DC-RX-HIT
                   END-IF
               WHEN "R"
                   PERFORM VARYING DC-RX-K FROM DC-RX-R1(DC-RX-I) BY 1
                           UNTIL DC-RX-K >= DC-RX-R1(DC-RX-I)
                               + DC-RX-RN(DC-RX-I)
                           OR DC-RX-HIT = "Y"
                       IF DC-RX-CUR >= DC-RX-LO(DC-RX-K)
                               AND DC-RX-CUR <= DC-RX-HI(DC-RX-K)
                           MOVE "Y" TO DC-RX-HIT
                       END-IF
                   END-PERFORM
           END-EVALUATE
           IF DC-RX-HIT = "Y"
               ADD 1 TO DC-RX-SN
               MOVE DC-RX-OUT(DC-RX-I) TO DC-RX-SEED(DC-RX-SN)
           END-IF.
