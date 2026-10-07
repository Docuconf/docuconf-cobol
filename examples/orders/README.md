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
               ...
```

`CFG-PORT` becomes `PORT`: the `@prefix` is dropped and hyphens become underscores. `PIC 9(5)` makes it an int that cannot be negative or above 99999; `@max 65535` narrows that. The level-88 values make `LOG_LEVEL` an enum. See the [annotation reference](../../README.md#annotations) for every tag.

## 2. Generate the contract and the loader

```sh
docuconf-cobol generate examples/orders/orders-config.cpy
```

```
wrote examples/orders/contract.cue
wrote examples/orders/ORDCFG.cbl
```

Commit both. CI runs `docuconf-cobol generate -check`, which fails when they no longer match the copybook.

## 3. CALL the loader

```cobol
       WORKING-STORAGE SECTION.
       COPY "orders-config.cpy".
       PROCEDURE DIVISION.
           CALL "ORDCFG" USING ORDERS-CONFIG
           IF RETURN-CODE NOT = 0
               STOP RUN RETURNING 1
           END-IF
```

The loader reads each variable with `ACCEPT ... FROM ENVIRONMENT`, applies the defaults, converts the values (`NUMVAL` for numbers, the csv list into the `OCCURS` table and its count, `30s` into `30000` milliseconds) and puts the orders file's path into `CFG-ORDERS-PATH`, which [`ORDERS-BATCH.cbl`](ORDERS-BATCH.cbl) uses in `SELECT ORDERS-FILE ASSIGN TO CFG-ORDERS-PATH`.

## 4. Build with cobc

```sh
cd examples/orders
cobc -x -o orders-batch ORDERS-BATCH.cbl ORDCFG.cbl
```

## 5. Run it under docuconf exec

```sh
DATABASE_URL=postgres://orders:s3cret@db:5432/orders ORDERS_FILE=orders.txt \
  docuconf exec --contract contract.cue -- ./orders-batch
```

```
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

```
$ PORT=0 ORDERS_FILE=orders.txt docuconf exec --contract contract.cue -- ./orders-batch
docuconf: orders-batch: 2 configuration problems:
  DATABASE_URL: is required but not set (missing_required)
  PORT: 0 is below min 1 (out_of_range)
```

The same lines go to the termination log, so `kubectl describe pod` shows them. Run the job without `docuconf exec` and only the loader's own checks apply: it reports the missing `DATABASE_URL`, but it does not check ranges, so `PORT=0` would get through. That is why the image's entrypoint is `docuconf exec`.

`docuconf check --contract contract.cue` runs the same checks without starting anything, for an init container or CI.

[`smoke.sh`](smoke.sh) runs both cases (and, where Docker is available, builds the image and runs them in a container):

```sh
DOCUCONF=/path/to/docuconf examples/orders/smoke.sh
```

## 6. Export and deploy

`contract.cue` is the export: the platform team validates its values against it with `docuconf vet` and renders the pod's environment with `docuconf render`, or uses the [Helm library chart](https://github.com/docuconf/docuconf-go/tree/main/helm) in docuconf-go. The [`Dockerfile`](Dockerfile) builds the job with GnuCOBOL and copies in the `docuconf` CLI and the contract, with `docuconf exec` as the entrypoint. A CronJob runs it nightly:

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: orders-batch
spec:
  schedule: "15 2 * * *"
  concurrencyPolicy: Forbid
  jobTemplate:
    spec:
      backoffLimit: 0
      template:
        spec:
          restartPolicy: Never
          containers:
            - name: orders-batch
              image: registry.example.com/orders-batch:1.0.0
              terminationMessagePolicy: FallbackToLogsOnError
              env:
                - name: DATABASE_URL
                  valueFrom:
                    secretKeyRef: {name: orders-db, key: url}
                - name: ALLOWED_ORIGINS
                  value: https://shop.example.com,http://localhost:3000
              volumeMounts:
                - name: orders
                  mountPath: /data
                  readOnly: true
          volumes:
            - name: orders
              configMap:
                name: orders-input
```

`docuconf render` writes the `env`, `volumes` and `volumeMounts` from the platform's values, so the contract and the pod spec cannot drift apart.
