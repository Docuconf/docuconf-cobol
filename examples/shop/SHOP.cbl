      *> SHOP: one program, two roles. It runs under docuconf exec,
      *> which checks the environment and the files against
      *> contract.json first: the role, the variables of that role,
      *> the requiredIf conditions and the otel@1 fragment. Only a
      *> valid configuration reaches SHOP, which reads its role and
      *> values into a typed record and prints one line.
       IDENTIFICATION DIVISION.
       PROGRAM-ID. SHOP.
       DATA DIVISION.
       WORKING-STORAGE SECTION.
      *> The typed configuration this program uses.
       01  SHOP-CONFIG.
           05  SHOP-ROLE               PIC X(6).
               88  ROLE-WEB            VALUE "web".
               88  ROLE-WORKER         VALUE "worker".
           05  SHOP-PORT               PIC 9(5).
           05  SHOP-ORIGIN-COUNT       PIC 99.
           05  SHOP-ORIGIN-TABLE.
               10  SHOP-ORIGINS        PIC X(256) OCCURS 16 TIMES.
           05  SHOP-QUEUE-COUNT        PIC 99.
           05  SHOP-QUEUE-TABLE.
               10  SHOP-QUEUES         PIC X(256) OCCURS 16 TIMES.
           05  SHOP-WEBHOOKS           PIC X.
               88  WEBHOOKS-ON         VALUE "Y".
      *> A csv list, split: docuconf exec checked it, so it only has
      *> to fit the table.
       01  WS-NAME                     PIC X(32).
       01  WS-RAW                      PIC X(4096).
       01  WS-RAW-LEN                  PIC 9(4).
       01  WS-ITEM-COUNT               PIC 99.
       01  WS-ITEM-TABLE.
           05  WS-ITEMS                PIC X(256) OCCURS 16 TIMES.
       01  WS-START                    PIC 9(4).
       01  WS-POS                      PIC 9(4).
       01  WS-I                        PIC 99.
       01  WS-LINE                     PIC X(4096).
       01  WS-PTR                      PIC 9(4).
       01  WS-SHOW-NUM                 PIC Z(4)9.
       PROCEDURE DIVISION.
       MAIN.
           PERFORM LOAD-CONFIG
           IF ROLE-WEB
               PERFORM SHOW-WEB
           ELSE
               PERFORM SHOW-WORKER
           END-IF
           MOVE 0 TO RETURN-CODE
           STOP RUN.

      *> docuconf exec exports the contract's defaults, so every
      *> variable of the role is set here.
       LOAD-CONFIG.
           ACCEPT SHOP-ROLE FROM ENVIRONMENT "DOCUCONF_ROLE"
           IF NOT ROLE-WEB AND NOT ROLE-WORKER
               DISPLAY "shop: DOCUCONF_ROLE is not web or worker; "
                   "run shop under docuconf exec" UPON SYSERR
               END-DISPLAY
               MOVE 1 TO RETURN-CODE
               STOP RUN
           END-IF
           IF ROLE-WEB
               MOVE SPACES TO WS-RAW
               ACCEPT WS-RAW FROM ENVIRONMENT "PORT"
               MOVE FUNCTION NUMVAL(WS-RAW) TO SHOP-PORT
               MOVE "ALLOWED_ORIGINS" TO WS-NAME
               PERFORM SPLIT-LIST
               MOVE WS-ITEM-COUNT TO SHOP-ORIGIN-COUNT
               MOVE WS-ITEM-TABLE TO SHOP-ORIGIN-TABLE
           ELSE
               MOVE "QUEUES" TO WS-NAME
               PERFORM SPLIT-LIST
               MOVE WS-ITEM-COUNT TO SHOP-QUEUE-COUNT
               MOVE WS-ITEM-TABLE TO SHOP-QUEUE-TABLE
               MOVE SPACES TO WS-RAW
               ACCEPT WS-RAW FROM ENVIRONMENT "WEBHOOKS_ENABLED"
      *> A bool is true or false in any case (SPEC section 5).
               IF FUNCTION LOWER-CASE(WS-RAW) = "true"
                   MOVE "Y" TO SHOP-WEBHOOKS
               ELSE
                   MOVE "N" TO SHOP-WEBHOOKS
               END-IF
           END-IF.

      *> Splits the csv list in WS-NAME on every comma, without
      *> trimming, into WS-ITEMS and WS-ITEM-COUNT.
       SPLIT-LIST.
           MOVE SPACES TO WS-RAW
           ACCEPT WS-RAW FROM ENVIRONMENT WS-NAME
           MOVE FUNCTION LENGTH(FUNCTION TRIM(WS-RAW TRAILING))
               TO WS-RAW-LEN
           MOVE SPACES TO WS-ITEM-TABLE
           MOVE 0 TO WS-ITEM-COUNT
           MOVE 1 TO WS-START
           PERFORM VARYING WS-POS FROM 1 BY 1
                   UNTIL WS-POS > WS-RAW-LEN + 1
               IF WS-POS > WS-RAW-LEN OR WS-RAW(WS-POS:1) = ","
                   IF WS-ITEM-COUNT = 16
                       DISPLAY "shop: " FUNCTION TRIM(WS-NAME)
                           " has more than 16 items" UPON SYSERR
                       END-DISPLAY
                       MOVE 1 TO RETURN-CODE
                       STOP RUN
                   END-IF
                   ADD 1 TO WS-ITEM-COUNT
                   IF WS-POS > WS-START
                       MOVE WS-RAW(WS-START:WS-POS - WS-START)
                           TO WS-ITEMS(WS-ITEM-COUNT)
                   END-IF
                   COMPUTE WS-START = WS-POS + 1
               END-IF
           END-PERFORM.

       SHOW-WEB.
           MOVE SHOP-PORT TO WS-SHOW-NUM
           MOVE SPACES TO WS-LINE
           MOVE 1 TO WS-PTR
           STRING "shop web: port " FUNCTION TRIM(WS-SHOW-NUM)
               ", origins " DELIMITED BY SIZE
               INTO WS-LINE WITH POINTER WS-PTR
           END-STRING
           PERFORM VARYING WS-I FROM 1 BY 1
                   UNTIL WS-I > SHOP-ORIGIN-COUNT
               IF WS-I > 1
                   STRING "," DELIMITED BY SIZE
                       INTO WS-LINE WITH POINTER WS-PTR
                   END-STRING
               END-IF
               STRING FUNCTION TRIM(SHOP-ORIGINS(WS-I) TRAILING)
                   DELIMITED BY SIZE INTO WS-LINE WITH POINTER WS-PTR
               END-STRING
           END-PERFORM
           DISPLAY FUNCTION TRIM(WS-LINE TRAILING) END-DISPLAY.

      *> The queues come from the typed table, not the raw variable.
       SHOW-WORKER.
           MOVE SPACES TO WS-LINE
           MOVE 1 TO WS-PTR
           STRING "shop worker: queues " DELIMITED BY SIZE
               INTO WS-LINE WITH POINTER WS-PTR
           END-STRING
           PERFORM VARYING WS-I FROM 1 BY 1
                   UNTIL WS-I > SHOP-QUEUE-COUNT
               IF WS-I > 1
                   STRING "," DELIMITED BY SIZE
                       INTO WS-LINE WITH POINTER WS-PTR
                   END-STRING
               END-IF
               STRING FUNCTION TRIM(SHOP-QUEUES(WS-I) TRAILING)
                   DELIMITED BY SIZE INTO WS-LINE WITH POINTER WS-PTR
               END-STRING
           END-PERFORM
           IF WEBHOOKS-ON
               STRING ", webhooks on" DELIMITED BY SIZE
                   INTO WS-LINE WITH POINTER WS-PTR
               END-STRING
           ELSE
               STRING ", webhooks off" DELIMITED BY SIZE
                   INTO WS-LINE WITH POINTER WS-PTR
               END-STRING
           END-IF
           DISPLAY FUNCTION TRIM(WS-LINE TRAILING) END-DISPLAY.
