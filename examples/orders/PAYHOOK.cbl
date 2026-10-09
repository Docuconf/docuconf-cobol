      *> PAYHOOK: checks the signature on one payment webhook against
      *> the key set in WEBHOOK_KEYS, as a service's
      *> POST /webhooks/payments would. A batch job has no HTTP
      *> endpoint, so the webhook receiver runs this program once per
      *> webhook: the body is one line on standard input, and the
      *> X-Signature header, the hex HMAC-SHA256 of the body, is the
      *> first argument. Exit code 0: accepted; 2: bad signature;
      *> 1: a configuration problem (ORDCFG has printed it).
      *>
      *> HMAC-SHA256 and the constant-time comparison come from
      *> OpenSSL's libcrypto:
      *>   cobc -x PAYHOOK.cbl ORDCFG.cbl -lcrypto
       IDENTIFICATION DIVISION.
       PROGRAM-ID. PAYHOOK.
       DATA DIVISION.
       WORKING-STORAGE SECTION.
       COPY "orders-config.cpy".
       01  WS-BODY                     PIC X(8192).
       01  WS-BODY-LEN                 USAGE BINARY-C-LONG UNSIGNED.
       01  WS-SIGNATURE                PIC X(256).
       01  WS-SIG-LEN                  PIC 9(4).
       01  WS-KEY-LEN                  USAGE BINARY-LONG.
       01  WS-SHA256                   USAGE POINTER.
       01  WS-RESULT                   USAGE POINTER.
       01  WS-MAC                      PIC X(64).
       01  WS-MAC-LEN                  USAGE BINARY-LONG UNSIGNED.
       01  WS-HEX                      PIC X(64).
       01  WS-HEX-DIGITS               PIC X(16)
                                       VALUE "0123456789abcdef".
       01  WS-BYTE                     PIC 999.
       01  WS-I                        PIC 99.
       01  WS-K                        PIC 9.
       01  WS-DIFF                     USAGE BINARY-LONG.
       01  WS-OK                       PIC X VALUE "N".
       PROCEDURE DIVISION.
       MAIN.
           CALL "ORDCFG" USING ORDERS-CONFIG
           IF RETURN-CODE NOT = 0
               STOP RUN
           END-IF
           MOVE SPACES TO WS-BODY WS-SIGNATURE
           ACCEPT WS-BODY END-ACCEPT
           ACCEPT WS-SIGNATURE FROM ARGUMENT-VALUE END-ACCEPT
           MOVE FUNCTION LOWER-CASE(WS-SIGNATURE) TO WS-SIGNATURE
           MOVE 0 TO WS-SIG-LEN
           INSPECT FUNCTION TRIM(WS-SIGNATURE TRAILING)
               TALLYING WS-SIG-LEN FOR CHARACTERS
           IF WS-BODY = SPACES
               MOVE 0 TO WS-BODY-LEN
           ELSE
               MOVE FUNCTION LENGTH(FUNCTION TRIM(WS-BODY TRAILING))
                 TO WS-BODY-LEN
           END-IF
           CALL "EVP_sha256" RETURNING WS-SHA256 END-CALL
      *> Check every key, so the time taken does not say which one
      *> matched; with no keys configured, nothing is accepted.
           PERFORM VARYING WS-K FROM 1 BY 1
                   UNTIL WS-K > CFG-WEBHOOK-KEY-COUNT
               PERFORM CHECK-KEY
           END-PERFORM
           IF WS-OK = "Y"
               DISPLAY "payhook: accepted" END-DISPLAY
               MOVE 0 TO RETURN-CODE
           ELSE
               DISPLAY "payhook: bad signature" UPON SYSERR
                   END-DISPLAY
               MOVE 2 TO RETURN-CODE
           END-IF
           STOP RUN.

      *> The hex HMAC-SHA256 of the body under key WS-K, compared in
      *> constant time with the signature.
      *> A key shorter than the contract's 32 characters is skipped:
      *> docuconf exec stops the job before it gets here, but the loader
      *> alone does not check item lengths, and an empty key would let
      *> anyone sign.
       CHECK-KEY.
           MOVE FUNCTION LENGTH(FUNCTION TRIM(
               CFG-WEBHOOK-KEYS(WS-K) TRAILING)) TO WS-KEY-LEN
           IF WS-KEY-LEN < 32
               EXIT PARAGRAPH
           END-IF
           CALL "HMAC" USING BY VALUE WS-SHA256
               BY REFERENCE CFG-WEBHOOK-KEYS(WS-K)
               BY VALUE WS-KEY-LEN
               BY REFERENCE WS-BODY
               BY VALUE WS-BODY-LEN
               BY REFERENCE WS-MAC
               BY REFERENCE WS-MAC-LEN
               RETURNING WS-RESULT
           END-CALL
           PERFORM VARYING WS-I FROM 1 BY 1 UNTIL WS-I > 32
               COMPUTE WS-BYTE = FUNCTION ORD(WS-MAC(WS-I:1)) - 1
               MOVE WS-HEX-DIGITS(WS-BYTE / 16 + 1:1)
                 TO WS-HEX(WS-I * 2 - 1:1)
               MOVE WS-HEX-DIGITS(FUNCTION MOD(WS-BYTE, 16) + 1:1)
                 TO WS-HEX(WS-I * 2:1)
           END-PERFORM
           CALL "CRYPTO_memcmp" USING BY REFERENCE WS-HEX
               BY REFERENCE WS-SIGNATURE
               BY VALUE 64
               RETURNING WS-DIFF
           END-CALL
           IF WS-DIFF = 0 AND WS-SIG-LEN = 64
               MOVE "Y" TO WS-OK
           END-IF.
