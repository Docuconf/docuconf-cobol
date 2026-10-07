      *> docuconf runtime: paragraphs for a generated loader.
      *> docuconf-cobol generate inlines it into every loader. Each
      *> paragraph works on DC-WORK (DCRTWS.cpy): DC-NAME is the
      *> variable being read, DC-RAW(1:DC-LEN) its value, and a
      *> problem is reported through DC-PROBLEM, which never prints
      *> the value.
      *> Valid in fixed and free format.

      *> Prints DC-NAME, DC-MSG and DC-CODE on stderr and counts it.
       DC-PROBLEM.
           ADD 1 TO DC-PROBLEMS
           MOVE "N" TO DC-OK
           DISPLAY "docuconf: " DC-NAME(1:DC-NAME-LEN) ": "
               FUNCTION TRIM(DC-MSG TRAILING) " ("
               FUNCTION TRIM(DC-CODE TRAILING) ")"
               UPON SYSERR
           END-DISPLAY.

       DC-BAD-TYPE.
           MOVE "invalid_type" TO DC-CODE
           PERFORM DC-PROBLEM.

       DC-BAD-RANGE.
           MOVE "out_of_range" TO DC-CODE
           PERFORM DC-PROBLEM.

      *> Reads the variable named in DC-NAME. DC-SET is "N" when it is
      *> not set; DC-LEN is its length without trailing spaces, which a
      *> COBOL field cannot keep.
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
           IF DC-SET = "Y" AND DC-RAW NOT = SPACES
               MOVE FUNCTION LENGTH(FUNCTION TRIM(DC-RAW TRAILING))
                   TO DC-LEN
               IF DC-LEN >= LENGTH OF DC-RAW
                   MOVE "is longer than the 8191 bytes a loader reads"
                       TO DC-MSG
                   PERFORM DC-BAD-RANGE
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
           IF DC-OK = "N"
               MOVE "is not a number" TO DC-MSG
               PERFORM DC-BAD-TYPE
           END-IF.

      *> true or false, in any case. DC-BOOL is "Y" or "N".
       DC-PARSE-BOOL.
           MOVE "Y" TO DC-OK
           MOVE SPACES TO DC-TMP
           IF DC-LEN <= 5
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
           IF DC-OK = "N"
               MOVE "is not a duration in the contract's encoding"
                   TO DC-MSG
               PERFORM DC-BAD-TYPE
           ELSE
               IF DC-NEG = "Y"
                   COMPUTE DC-NS = 0 - DC-NS
               END-IF
           END-IF.

      *> Reads a decimal number at DC-P into DC-NUM; DC-OK is "N" when
      *> there is none.
       DC-DUR-NUMBER.
           MOVE DC-P TO DC-Q
           PERFORM UNTIL DC-P > DC-LEN
                   OR (DC-RAW(DC-P:1) IS NOT NUMERIC
                       AND DC-RAW(DC-P:1) NOT = ".")
               ADD 1 TO DC-P
           END-PERFORM
           IF DC-P = DC-Q
               MOVE "N" TO DC-OK
           ELSE
               IF DC-RAW(DC-Q:DC-P - DC-Q) = "."
                   MOVE "N" TO DC-OK
               ELSE
                   COMPUTE DC-NUM =
                       FUNCTION NUMVAL(DC-RAW(DC-Q:DC-P - DC-Q))
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
      *> every docuconf SDK does for local runs.
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
