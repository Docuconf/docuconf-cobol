      *> Configuration of the ORDERS-BATCH job, read from the
      *> environment by the generated loader ORDCFG.
      *> @service orders-batch  @prefix CFG-  @program ORDCFG
       01  ORDERS-CONFIG.
      *> Port of the Prometheus metrics endpoint
      *> @min 1  @max 65535  @default 8080
           05  CFG-PORT                PIC 9(5).
      *> Log verbosity
      *> @default info
           05  CFG-LOG-LEVEL           PIC X(5).
               88  LOG-DEBUG           VALUE "debug".
               88  LOG-INFO            VALUE "info".
               88  LOG-WARN            VALUE "warn".
               88  LOG-ERROR           VALUE "error".
      *> Postgres connection string of the orders database
      *> @type url  @schemes postgres  @secret  @required
           05  CFG-DATABASE-URL        PIC X(200).
      *> Origins whose orders the job accepts
      *> @min-items 1  @default "http://localhost:3000"
      *> @count CFG-ORIGIN-COUNT
           05  CFG-ALLOWED-ORIGINS     PIC X(64) OCCURS 8 TIMES.
           05  CFG-ORIGIN-COUNT        PIC 9(2).
      *> Time allowed for each database call, in milliseconds
      *> @unit ms  @min 1s  @max 5m  @default 30s
           05  CFG-REQUEST-TIMEOUT     PIC 9(6).
      *> Number of workers that share the input
      *>
      *> Each worker reads its share of the orders file and holds
      *> one database connection, so keep this at or below the
      *> pool size:
      *>
      *> - one connection per worker;
      *> - plus one for the summary step.
      *> @min 1  @max 64  @default 4
           05  CFG-WORKER-COUNT        PIC 9(2).
      *> Keys that verify the signature on incoming payment webhooks
      *>
      *> A webhook is accepted when it is signed with any key in the
      *> set, so the key can be rotated without turning webhooks
      *> away. Each key is 32 to 256 characters, so an empty or
      *> truncated key fails at boot. Without this variable, the
      *> service rejects every webhook.
      *> @type keySet  @key-min-length 32
      *> @count CFG-WEBHOOK-KEY-COUNT
           05  CFG-WEBHOOK-KEYS        PIC X(256) OCCURS 2 TIMES.
           05  CFG-WEBHOOK-KEY-COUNT   PIC 9.
      *> The orders to summarise, one per line
      *> @file orders text  @path /data/orders.txt
      *> @path-env ORDERS_FILE  @required  @max-size 1Mi
           05  CFG-ORDERS-PATH         PIC X(256).
