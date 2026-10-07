      *> Fields for the loader's own checks (loader_test.go).
      *> @service loader-test  @prefix T-  @program TESTCFG
       01  TEST-CONFIG.
      *> A short name
           05  T-NAME              PIC X(5).
      *> A secret token
      *> @secret
           05  T-TOKEN             PIC X(3).
      *> How many to process
      *> @default 7
           05  T-COUNT             PIC 9(3).
      *> Offset, may be negative
      *> @present T-OFFSET-SET
           05  T-OFFSET            PIC S9(4).
           05  T-OFFSET-SET        PIC X.
      *> Share to sample
           05  T-RATIO             PIC 9V99.
      *> Request timeout
      *> @unit ms  @default 30s
           05  T-TIMEOUT           PIC 9(7).
      *> Retry interval, as a .NET TimeSpan
      *> @unit s  @encoding timespan
           05  T-RETRY             PIC 9(6).
      *> Verbose output
      *> @type bool
           05  T-VERBOSE           PIC 9.
      *> Shards owned
      *> @encoding json  @count T-SHARD-COUNT
           05  T-SHARDS            PIC S9(4) OCCURS 3.
           05  T-SHARD-COUNT       PIC 9.
      *> Tags to add
      *> @count T-TAG-COUNT  @separator ";"
           05  T-TAGS              PIC X(8) OCCURS 2.
           05  T-TAG-COUNT         PIC 9.
      *> Region to serve
      *> @required  @values eu us
           05  T-REGION            PIC X(2).
      *> The orders file
      *> @file orders  @path /data/orders.txt  @path-env ORDERS_FILE
           05  T-ORDERS-PATH       PIC X(60).
