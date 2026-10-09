      *> Every variable type and every file type, for the golden test.
      *> @service gateway-batch  @prefix GW-  @program GWCFG
       01  GATEWAY-CONFIG.
      *> Name shown in reports
      *> @min-length 2  @pattern "^[a-z][a-z0-9-]*$"  @default "gateway"
           05  GW-REPORT-NAME          PIC X(32).
      *> Port of the metrics endpoint
      *> @min 1  @max 65535  @default 9090
           05  GW-PORT                 PIC 9(5).
      *> Offset applied to every sequence number
      *> @present GW-OFFSET-SET
           05  GW-OFFSET               PIC S9(9).
           05  GW-OFFSET-SET           PIC X.
      *> Share of the traffic to sample
      *> @default 0.25  @max 1
           05  GW-SAMPLE-RATIO         PIC 9V9(4).
      *> Scale factor for amounts
      *> @type float
           05  GW-SCALE                COMP-2.
      *> Reject unknown fields
      *> @type bool  @default false
           05  GW-STRICT               PIC X.
               88  GW-IS-STRICT        VALUE "Y".
      *> Log every request
      *> @type bool
           05  GW-VERBOSE              PIC 9.
      *> Time allowed per request
      *> @unit ms  @min 1s  @max 5m  @default 30s  @encoding iso8601
           05  GW-REQUEST-TIMEOUT      PIC 9(6).
      *> Interval between retries, in seconds with fractions
      *> @unit s  @encoding seconds  @default 1500ms
           05  GW-RETRY-INTERVAL       PIC 9(4)V999.
      *> Upstream base URL
      *> @type url  @schemes https  @default "https://api.example.com"
      *> @max-length 80
           05  GW-UPSTREAM             PIC X(100).
      *> Log verbosity
      *> @default info
           05  GW-LOG-LEVEL            PIC X(5).
               88  GW-LOG-DEBUG        VALUE "debug".
               88  GW-LOG-INFO         VALUE "info".
      *> Deployment region
      *> @values eu us  @required
           05  GW-REGION               PIC X(2).
      *> Shards this instance owns
      *> @encoding json  @item-min 0  @item-max 1023
      *> @count GW-SHARD-COUNT
           05  GW-SHARDS               PIC 9(4) OCCURS 16.
           05  GW-SHARD-COUNT          PIC 99.
      *> Tags added to every record
      *> @separator ";"  @count GW-TAG-COUNT  @default a b
      *> @item-min-length 1
           05  GW-TAGS                 PIC X(10) OCCURS 5.
           05  GW-TAG-COUNT            PIC 9.
      *> Rate limits per client
      *> @type json  @schema schema-limits.json
      *> @default "{""perMinute"": 60}"
           05  GW-RATE-LIMITS          PIC X(200).
      *> API token for the upstream
      *> @secret  @min-length 20
           05  GW-API-TOKEN            PIC X(64).
      *> @ignore
           05  GW-WORK-AREA            PIC X(10).
           05  GW-FILES.
      *> Serving certificate
      *> @file serving-tls tls  @path /etc/gateway/tls
      *> @dns-names api.example.com  @min-remaining 720h  @require-ca
               10  GW-TLS-DIR          PIC X(100).
      *> Private CAs the upstream uses
      *> @file upstream-ca caBundle  @path /etc/gateway/ca/ca.pem
               10  GW-CA-PATH          PIC X(100).
      *> Partner keystore
      *> @file partner-keystore keystore  @path /etc/gateway/ks/p.p12
      *> @password-var PARTNER_KEYSTORE_PASSWORD  @required
               10  GW-KEYSTORE-PATH    PIC X(100).
      *> Routing table
      *> @file routes config  @path /etc/gateway/routes/routes.yaml
      *> @schema schema-limits.json  @max-size 64Ki
               10  GW-ROUTES-PATH      PIC X(100).
      *> Licence key
      *> @file licence text  @path /etc/gateway/licence/key.txt
      *> @path-env LICENCE_FILE  @pattern "^[A-Z0-9-]+$"
      *> @max-length 64
               10  GW-LICENCE-PATH     PIC X(100).
      *> GeoIP database
      *> @file geoip binary  @path /var/lib/geoip/GeoLite2.mmdb
               10  GW-GEOIP-PATH       PIC X(100).
      *> Keys that verify webhook signatures
      *> @type keySet  @key-min-length 32  @count GW-WEBHOOK-KEY-COUNT
           05  GW-WEBHOOK-KEYS         PIC X(256) OCCURS 2.
           05  GW-WEBHOOK-KEY-COUNT    PIC 9.
      *> API keys that callers present, as a JSON array
      *> @type keySet  @encoding json  @max-keys 3  @count GW-API-KEY-N
           05  GW-API-KEYS             PIC X(64) OCCURS 4.
           05  GW-API-KEY-N            PIC 9.
      *> Port the gateway used to listen on
      *> @deprecated "Use PORT instead"  @replaced-by PORT
           05  GW-OLD-PORT             PIC 9(5).
      *> Keystore password
      *> @secret @required
           05  GW-PARTNER-KEYSTORE-PASSWORD PIC X(64).
      *> Brokers to connect to
      *> @encoding indexed  @min-items 1
           05  GW-BROKER-COUNT         PIC 9.
           05  GW-BROKERS              PIC X(40)
                   OCCURS 1 TO 4 DEPENDING ON GW-BROKER-COUNT.
