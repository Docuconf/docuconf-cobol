      *> The shared export fixture of the docuconf conformance suite
      *> (docuconf-go conformance/export/fixture.yaml), declared as a
      *> copybook. TestExportFixture exports it and compares the result
      *> with conformance/export/golden.cue.
      *> @service docuconf-fixture  @app-version "1.0.0"
      *> @program FIXCFG
       01  FIXTURE-CONFIG.
      *> Service name, used in logs and metrics
      *>
      *> Lower case, as a DNS label allows.
      *> @env APP_NAME  @default orders  @min-length 2
      *> @pattern "^[a-z][a-z0-9-]*$"  @group general
      *> @examples orders billing  @config-key App:Name
           05  FX-APP-NAME             PIC X(40).
      *> Primary Postgres connection string
      *> @env DATABASE_URL  @type url  @required  @secret
      *> @schemes postgres postgresql  @group database
           05  FX-DATABASE-URL         PIC X(2048).
      *> HTTP listen port
      *> @env PORT  @min 1  @max 65535  @default 8080
           05  FX-PORT                 PIC 9(5).
      *> Fraction of requests traced
      *> @env TRACE_RATIO  @min 0  @max 1  @default 0.25
           05  FX-TRACE-RATIO          PIC 9V99.
      *> Serve the debug endpoints
      *> @env DEBUG  @type bool  @default false
           05  FX-DEBUG                PIC X.
      *> Upstream request timeout
      *> @env REQUEST_TIMEOUT  @unit ms  @min 1s  @max 5m
      *> @default 1m30s
           05  FX-REQUEST-TIMEOUT      PIC 9(6).
      *> Minimum log level
      *> @env LOG_LEVEL  @default info
           05  FX-LOG-LEVEL            PIC X(5).
               88  FX-LOG-DEBUG        VALUE "debug".
               88  FX-LOG-INFO         VALUE "info".
               88  FX-LOG-WARN         VALUE "warn".
               88  FX-LOG-ERROR        VALUE "error".
      *> CORS origins allowed to call the API
      *> @env ALLOWED_ORIGINS  @count FX-ORIGIN-COUNT
      *> @min-items 1  @item-min-length 1  @separator ";"
           05  FX-ALLOWED-ORIGINS      PIC X(255) OCCURS 5.
           05  FX-ORIGIN-COUNT         PIC 9.
      *> Shards this instance owns
      *> @env SHARDS  @count FX-SHARD-COUNT  @item-max 1023
           05  FX-SHARDS               PIC 9(4) OCCURS 64.
           05  FX-SHARD-COUNT          PIC 99.
      *> Keys that verify webhook signatures
      *> @env WEBHOOK_KEYS  @type keySet  @count FX-WEBHOOK-KEY-COUNT
      *> @key-min-length 32
           05  FX-WEBHOOK-KEYS         PIC X(256) OCCURS 2.
           05  FX-WEBHOOK-KEY-COUNT    PIC 9.
      *> Per-client rate limits
      *> @env RATE_LIMITS  @type json  @schema rate-limits.json
      *> @default "{""perMinute"": 60}"
           05  FX-RATE-LIMITS          PIC X(1024).
      *> Port the service used to listen on
      *> @env OLD_PORT  @deprecated "Use PORT instead"
      *> @replaced-by PORT
           05  FX-OLD-PORT             PIC 9(5).
      *> Password of the partner keystore
      *> @env PARTNER_PASSWORD  @secret
           05  FX-PARTNER-PASSWORD     PIC X(128).
      *> Application settings
      *> @file settings config  @path /etc/app/settings/settings.json
      *> @path-env SETTINGS_FILE  @required  @max-size 64Ki
      *> @group general  @schema settings.json
           05  FX-SETTINGS-PATH        PIC X(256).
      *> Routing rules
      *> @file rules config  @path /etc/app/rules/rules.yaml
      *> @schema settings.json
           05  FX-RULES-PATH           PIC X(256).
      *> Feature defaults
      *> @file flags config  @format toml  @schema settings.json
      *> @path /etc/app/flags/flags.toml
           05  FX-FLAGS-PATH           PIC X(256).
      *> Certificate the service serves HTTPS with
      *> @file serving-tls tls  @path /etc/app/tls
      *> @dns-names app.example.test api.example.test
      *> @key-algorithms ECDSA Ed25519  @min-remaining 720h
      *> @require-ca
           05  FX-TLS-PATH             PIC X(256).
      *> CAs the service trusts
      *> @file trust caBundle  @path /etc/app/trust/bundle.pem
      *> @min-certificates 2
           05  FX-TRUST-PATH           PIC X(256).
      *> Client certificate for the partner API
      *> @file partner keystore  @path /etc/app/partner/keystore.p12
      *> @password-var PARTNER_PASSWORD
           05  FX-PARTNER-PATH         PIC X(256).
      *> Licence key
      *> @file licence text  @path /etc/app/licence/licence.key
      *> @min-length 8  @max-length 64  @pattern "^[A-Z0-9-]+\n?$"
           05  FX-LICENCE-PATH         PIC X(256).
      *> GeoIP database
      *> @file geoip binary  @path /data/geoip/geoip.mmdb
      *> @max-size 128Mi  @deprecated "Use geo-db instead"
      *> @replaced-by geo-db
           05  FX-GEOIP-PATH           PIC X(256).
      *> City-level location database
      *> @file geo-db binary  @path /data/geo-db/geo.mmdb
           05  FX-GEO-DB-PATH          PIC X(256).
