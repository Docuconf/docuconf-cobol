      *> CFGTEST: a test of the configuration. test-config.sh runs it
      *> with an explicit environment (env -i), so nothing depends on
      *> the shell it runs in, and it checks what ORDCFG stored.
       IDENTIFICATION DIVISION.
       PROGRAM-ID. CFGTEST.
       DATA DIVISION.
       WORKING-STORAGE SECTION.
       COPY "orders-config.cpy".
       01  WS-FAILS                    PIC 99 VALUE 0.
       PROCEDURE DIVISION.
       MAIN.
           CALL "ORDCFG" USING ORDERS-CONFIG
           IF RETURN-CODE NOT = 0
               DISPLAY "FAIL: ORDCFG returned " RETURN-CODE
               END-DISPLAY
               STOP RUN
           END-IF
      *> WORKER_COUNT=7 is set; every other optional takes its default.
           IF CFG-WORKER-COUNT NOT = 7
               DISPLAY "FAIL: WORKER_COUNT " CFG-WORKER-COUNT
               END-DISPLAY
               ADD 1 TO WS-FAILS
           END-IF
           IF CFG-PORT NOT = 8080
               DISPLAY "FAIL: PORT " CFG-PORT END-DISPLAY
               ADD 1 TO WS-FAILS
           END-IF
           IF NOT LOG-INFO
               DISPLAY "FAIL: LOG_LEVEL " CFG-LOG-LEVEL END-DISPLAY
               ADD 1 TO WS-FAILS
           END-IF
           IF CFG-REQUEST-TIMEOUT NOT = 30000
               DISPLAY "FAIL: REQUEST_TIMEOUT " CFG-REQUEST-TIMEOUT
               END-DISPLAY
               ADD 1 TO WS-FAILS
           END-IF
           IF CFG-ORIGIN-COUNT NOT = 1 OR
                   CFG-ALLOWED-ORIGINS(1) NOT = "http://localhost:3000"
               DISPLAY "FAIL: ALLOWED_ORIGINS" END-DISPLAY
               ADD 1 TO WS-FAILS
           END-IF
           IF WS-FAILS = 0
               DISPLAY "ok" END-DISPLAY
               MOVE 0 TO RETURN-CODE
           ELSE
               MOVE 1 TO RETURN-CODE
           END-IF
           STOP RUN.
