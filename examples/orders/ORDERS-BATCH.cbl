      *> ORDERS-BATCH: summarises a file of orders. A Kubernetes
      *> CronJob runs it under docuconf exec, which validates the
      *> environment and the orders file against contract.cue first.
       IDENTIFICATION DIVISION.
       PROGRAM-ID. ORDERS-BATCH.
       ENVIRONMENT DIVISION.
       INPUT-OUTPUT SECTION.
       FILE-CONTROL.
      *> The path comes from the configuration: ORDERS_FILE, else the
      *> contract's /data/orders.txt.
           SELECT ORDERS-FILE ASSIGN TO CFG-ORDERS-PATH
               ORGANIZATION IS LINE SEQUENTIAL
               FILE STATUS IS WS-STATUS.
       DATA DIVISION.
       FILE SECTION.
       FD  ORDERS-FILE.
       01  ORDER-LINE                  PIC X(200).
       WORKING-STORAGE SECTION.
       COPY "orders-config.cpy".
       01  WS-STATUS                   PIC XX.
       01  WS-EOF                      PIC X VALUE "N".
       01  WS-ORDER-ID                 PIC X(20).
       01  WS-ORIGIN                   PIC X(64).
       01  WS-AMOUNT-TEXT              PIC X(20).
       01  WS-AMOUNT                   PIC S9(9)V99.
       01  WS-READ                     PIC 9(7) VALUE 0.
       01  WS-ACCEPTED                 PIC 9(7) VALUE 0.
       01  WS-TOTAL                    PIC S9(11)V99 VALUE 0.
       01  WS-I                        PIC 99.
       01  WS-OK                       PIC X.
       01  WS-SHOW-NUM                 PIC Z(9)9.
       01  WS-SHOW-MONEY               PIC -(11)9.99.
       PROCEDURE DIVISION.
       MAIN.
           CALL "ORDCFG" USING ORDERS-CONFIG
           IF RETURN-CODE NOT = 0
               STOP RUN
           END-IF
           PERFORM SHOW-CONFIG
           PERFORM SUMMARISE
           MOVE 0 TO RETURN-CODE
           STOP RUN.

      *> The typed configuration. DATABASE_URL is a secret: never shown.
       SHOW-CONFIG.
           DISPLAY "orders-batch configuration:" END-DISPLAY
           MOVE CFG-PORT TO WS-SHOW-NUM
           DISPLAY "  PORT            " FUNCTION TRIM(WS-SHOW-NUM)
               " (metrics, not opened by this job)" END-DISPLAY
           DISPLAY "  LOG_LEVEL       " FUNCTION TRIM(CFG-LOG-LEVEL)
               END-DISPLAY
           DISPLAY "  DATABASE_URL    ***" END-DISPLAY
           PERFORM VARYING WS-I FROM 1 BY 1
                   UNTIL WS-I > CFG-ORIGIN-COUNT
               DISPLAY "  ALLOWED_ORIGINS "
                   FUNCTION TRIM(CFG-ALLOWED-ORIGINS(WS-I)) END-DISPLAY
           END-PERFORM
           MOVE CFG-REQUEST-TIMEOUT TO WS-SHOW-NUM
           DISPLAY "  REQUEST_TIMEOUT " FUNCTION TRIM(WS-SHOW-NUM) "ms"
               END-DISPLAY
           MOVE CFG-WORKER-COUNT TO WS-SHOW-NUM
           DISPLAY "  WORKER_COUNT    " FUNCTION TRIM(WS-SHOW-NUM)
               END-DISPLAY
           DISPLAY "  orders file     " FUNCTION TRIM(CFG-ORDERS-PATH)
               END-DISPLAY.

      *> Each line is "<order id> <origin> <amount>". Orders from an
      *> origin outside ALLOWED_ORIGINS are counted but not accepted.
       SUMMARISE.
           OPEN INPUT ORDERS-FILE
           IF WS-STATUS NOT = "00"
               DISPLAY "orders-batch: cannot open the orders file, "
                   "status " WS-STATUS UPON SYSERR END-DISPLAY
               MOVE 1 TO RETURN-CODE
               STOP RUN
           END-IF
           PERFORM UNTIL WS-EOF = "Y"
               READ ORDERS-FILE
                   AT END
                       MOVE "Y" TO WS-EOF
                   NOT AT END
                       PERFORM ONE-ORDER
               END-READ
           END-PERFORM
           CLOSE ORDERS-FILE
           DISPLAY "summary:" END-DISPLAY
           MOVE WS-READ TO WS-SHOW-NUM
           DISPLAY "  orders read     " FUNCTION TRIM(WS-SHOW-NUM)
               END-DISPLAY
           MOVE WS-ACCEPTED TO WS-SHOW-NUM
           DISPLAY "  accepted        " FUNCTION TRIM(WS-SHOW-NUM)
               END-DISPLAY
           MOVE WS-TOTAL TO WS-SHOW-MONEY
           DISPLAY "  accepted total  " FUNCTION TRIM(WS-SHOW-MONEY)
               END-DISPLAY.

       ONE-ORDER.
           IF ORDER-LINE = SPACES
               EXIT PARAGRAPH
           END-IF
           ADD 1 TO WS-READ
           MOVE SPACES TO WS-ORDER-ID WS-ORIGIN WS-AMOUNT-TEXT
           UNSTRING ORDER-LINE DELIMITED BY ALL SPACE
               INTO WS-ORDER-ID WS-ORIGIN WS-AMOUNT-TEXT
           END-UNSTRING
           MOVE "N" TO WS-OK
           PERFORM VARYING WS-I FROM 1 BY 1
                   UNTIL WS-I > CFG-ORIGIN-COUNT
               IF WS-ORIGIN = CFG-ALLOWED-ORIGINS(WS-I)
                   MOVE "Y" TO WS-OK
               END-IF
           END-PERFORM
           IF WS-OK = "Y"
               ADD 1 TO WS-ACCEPTED
               COMPUTE WS-AMOUNT = FUNCTION NUMVAL(WS-AMOUNT-TEXT)
               ADD WS-AMOUNT TO WS-TOTAL
           END-IF.
