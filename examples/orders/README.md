# orders: a COBOL batch job with a docuconf contract

`ORDERS-BATCH` reads a file of orders, keeps the ones from allowed origins and prints a summary. In a cluster it runs as a Kubernetes CronJob. Its configuration is one annotated copybook, [`orders-config.cpy`](orders-config.cpy), from which `docuconf-cobol generate` writes the contract ([`contract.cue`](contract.cue)) and the loader the program calls ([`ORDCFG.cbl`](ORDCFG.cbl)). At boot, `docuconf exec` checks the environment and the orders file against the contract before the job starts.

| Variable | COBOL field | Rules |
|---|---|---|
| `PORT` | `CFG-PORT PIC 9(5)` | 1 to 65535, default 8080 |
| `LOG_LEVEL` | `CFG-LOG-LEVEL PIC X(5)` with four level-88s | `debug`, `info`, `warn`, `error`; default `info` |
| `DATABASE_URL` | `CFG-DATABASE-URL PIC X(200)` | url, secret, required, scheme `postgres` |
| `ALLOWED_ORIGINS` | `CFG-ALLOWED-ORIGINS PIC X(64) OCCURS 8` | list of strings, 1 to 8 items; default `["http://localhost:3000"]` |
| `REQUEST_TIMEOUT` | `CFG-REQUEST-TIMEOUT PIC 9(6)`, in milliseconds | duration, 1s to 5m, default `30s` |
| `WORKER_COUNT` | `CFG-WORKER-COUNT PIC 9(2)` | 1 to 64, default 4 |
| file `orders` | `CFG-ORDERS-PATH PIC X(256)` receives its path | text, required, at most 1 MiB, at `/data/orders.txt` or `ORDERS_FILE` |

These are the six variables every docuconf SDK's orders example uses. A batch job serves no HTTP, so `PORT` here is the port of a Prometheus metrics endpoint that this job does not open yet; it is kept so the contract matches the other languages' examples, and the job prints it. `DATABASE_URL` and `REQUEST_TIMEOUT` describe the database the job would write its summary to; this example prints the summary instead.

## 1. Annotate the copybook

The record is ordinary COBOL. The comment lines above each field give its description, and `@` tags add what the PIC clause cannot say:

```cobol
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
```

`CFG-PORT` becomes `PORT`: the `@prefix` is dropped and hyphens become underscores. `PIC 9(5)` makes it an int that cannot be negative or above 99999; `@max 65535` narrows that. The level-88 values make `LOG_LEVEL` an enum. See the [annotation reference](../../README.md#annotations) for every tag.

## 2. Generate the contract and the loader

```sh
docuconf-cobol generate orders-config.cpy
```

```text
wrote contract.cue
wrote ORDCFG.cbl
```

Commit both. CI runs `docuconf-cobol generate -check`, which fails when they no longer match the copybook.

**Generated docs.** [`CONFIG.md`](CONFIG.md), the reference for developers, and [`CONFIG.agents.md`](CONFIG.agents.md), the rules and facts AI agents need, are generated from `contract.cue` by the `docuconf` CLI, through the docs model in [`docs.json`](docs.json). Never edit them by hand; regenerate them after generating the contract (CI fails if they are out of date):

```sh
docuconf docs contract.cue -o CONFIG.md
docuconf docs contract.cue --format agents -o CONFIG.agents.md
docuconf docs contract.cue --format model -o docs.json
```

## 3. CALL the loader

```cobol
       WORKING-STORAGE SECTION.
       COPY "orders-config.cpy".
```

```cobol
       MAIN.
           CALL "ORDCFG" USING ORDERS-CONFIG
           IF RETURN-CODE NOT = 0
               STOP RUN
           END-IF
```

The loader reads each variable with `ACCEPT ... FROM ENVIRONMENT`, applies the defaults, converts the values (`NUMVAL` for numbers, the csv list into the `OCCURS` table and its count, `30s` into `30000` milliseconds), checks the ranges, and puts the orders file's path into `CFG-ORDERS-PATH`, which [`ORDERS-BATCH.cbl`](ORDERS-BATCH.cbl) uses in `SELECT ORDERS-FILE ASSIGN TO CFG-ORDERS-PATH`. On a problem it prints them all and sets `RETURN-CODE` to 1, and `STOP RUN` ends the job with that code.

## 4. Build with cobc

```sh
cobc -x -o orders-batch ORDERS-BATCH.cbl ORDCFG.cbl
```

## 5. Run it under docuconf exec

```sh
DATABASE_URL=postgres://orders:s3cret@db:5432/orders ORDERS_FILE=orders.txt \
  docuconf exec -contract contract.cue -- ./orders-batch
```

```text
orders-batch configuration:
  PORT            8080 (metrics, not opened by this job)
  LOG_LEVEL       info
  DATABASE_URL    ***
  ALLOWED_ORIGINS http://localhost:3000
  REQUEST_TIMEOUT 30000ms
  WORKER_COUNT    4
  orders file     orders.txt
summary:
  orders read     5
  accepted        1
  accepted total  42.50
```

With `PORT=0` and no `DATABASE_URL`, `docuconf exec` stops before the job starts and exits 1:

```sh
PORT=0 ORDERS_FILE=orders.txt docuconf exec -contract contract.cue -- ./orders-batch
```

```text
docuconf: 2 configuration problems:
  DATABASE_URL: is required but not set (missing_required)
  PORT: 0 is below min 1 (out_of_range)
```

The same lines go to the termination log, so `kubectl describe pod` shows them. Run the job without `docuconf exec` and the loader still refuses `PORT=0` (it checks `@min` and `@max`); what it leaves to `docuconf exec` is the URL scheme of `DATABASE_URL` and the orders file itself.

For a local run, keep the variables in a `.env` file: `docuconf exec -env-file .env` checks its values and passes them to the job (a variable already set in the environment wins). `docuconf check -contract contract.cue` runs the same checks without starting anything, for an init container or CI.

[`smoke.sh`](smoke.sh) runs these cases (and, where Docker is available, builds the image and runs them in a container), and [`test-config.sh`](test-config.sh) tests the configuration with [`CFGTEST.cbl`](CFGTEST.cbl):

<!-- not executed: CI runs them -->
```sh
DOCUCONF=/path/to/docuconf examples/orders/smoke.sh
DOCUCONF=/path/to/docuconf examples/orders/test-config.sh
```

## 6. Deploy

`contract.cue` is the export: the platform team checks its values with `docuconf vet` and renders the pod's environment and volumes with `docuconf render`, from [`k8s/values.yaml`](k8s/values.yaml) and [`k8s/files.yaml`](k8s/files.yaml), which says the orders file is the key `orders.txt` of the ConfigMap `orders-input`:

```sh
docuconf vet -contract contract.cue -values k8s/values.yaml -files k8s/files.yaml
docuconf render -contract contract.cue -values k8s/values.yaml -files k8s/files.yaml
```

The output ([`k8s/rendered.yaml`](k8s/rendered.yaml)) is the `env`, `volumes` and `volumeMounts` of [`k8s/cronjob.yaml`](k8s/cronjob.yaml), so the pod spec cannot drift from the contract. The [`Dockerfile`](Dockerfile) builds the job with GnuCOBOL, copies in the `docuconf` CLI and the contract, and starts the job through `docuconf exec`:

```dockerfile
ENTRYPOINT ["docuconf", "exec", "-contract", "/etc/docuconf/contract.cue", "--", "/app/orders-batch"]
```

The image holds no input: the platform mounts it. A ConfigMap holds at most 1 MiB, and batch input usually lives on a PersistentVolumeClaim; [`k8s/cronjob-pvc.yaml`](k8s/cronjob-pvc.yaml) mounts one and sets `ORDERS_FILE`. The contract has no PVC file source yet, so that volume is written by hand; `docuconf exec` still checks the file before the job starts.
