# shop: roles, requiredIf and fragments with docuconf exec

`shop` is one program that runs as two workloads: a `web` server and a `worker` that takes jobs from queues. It shows three parts of the contract that only `docuconf exec` enforces in COBOL:

- **roles** ([SPEC section 4.9](https://github.com/docuconf/docuconf-go/blob/main/spec/SPEC.md#49-roles)): `DOCUCONF_ROLE` says which workload this is, and an input marked with roles is checked only in those;
- **requiredIf** ([SPEC section 4.2.1](https://github.com/docuconf/docuconf-go/blob/main/spec/SPEC.md#421-conditional-requirements)): `LOG_SCHEMA_URL` is required when `LOG_FORMAT` is `json`, and the webhook signing key when `WEBHOOKS_ENABLED` is `true`;
- an **included fragment** ([SPEC section 4.8](https://github.com/docuconf/docuconf-go/blob/main/spec/SPEC.md#48-well-known-fragments)): `otel@1`, the OpenTelemetry variables, checked at boot like the app's own.

The program is a stub: there is no server, database or queue. It reads its configuration and prints one line.

## The contract

`docuconf-cobol generate` writes a contract and a loader from a copybook, but a copybook cannot declare roles, conditions or fragments, and the generated loader does not know them. So this example is contract-first: [`contract.json`](contract.json) is written by hand (the same contract every docuconf SDK's shop example uses), and `docuconf exec` checks the whole of it before [`SHOP.cbl`](SHOP.cbl) starts.

| Input | Roles | Rules |
|---|---|---|
| `DOCUCONF_ROLE` | all | required, `web` or `worker` |
| `DATABASE_URL` | all | url, required, secret, scheme `postgres` or `postgresql` |
| `PORT` | `web` | int, 1 to 65535, default 8080 |
| `ALLOWED_ORIGINS` | `web` | list of strings (csv), required |
| `QUEUES` | `worker` | list of strings (csv), required |
| `WEBHOOKS_ENABLED` | `worker` | bool, default `false` |
| `LOG_FORMAT` | all | `text` or `json`, default `text` |
| `LOG_SCHEMA_URL` | all | url, scheme `https`, required when `LOG_FORMAT` is `json` |
| `OTEL_*` | all | the `otel@1` fragment, all optional |
| file `webhook-signing-key` | `worker` | text, secret, at least 32 characters, at `/etc/shop/webhooks/signing.key`; required when `WEBHOOKS_ENABLED` is `true` |

[`CONFIG.md`](CONFIG.md), [`CONFIG.agents.md`](CONFIG.agents.md) and [`docs.json`](docs.json) are generated from the contract; regenerate them when it changes (CI fails if they are out of date):

```sh
docuconf docs contract.json -o CONFIG.md
docuconf docs contract.json --format agents -o CONFIG.agents.md
docuconf docs contract.json --format model -o docs.json
```

## The program

`SHOP` reads `DOCUCONF_ROLE` and that role's variables with `ACCEPT ... FROM ENVIRONMENT` into a typed record: the port as a number, each csv list split on every comma into an `OCCURS` table and its count, and the bool as a level-88 flag. `docuconf exec` exports the contract's defaults, so every variable the role uses is set. The line it prints comes from that record, not from the raw variables:

```cobol
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
```

Started without `docuconf exec` and no valid role, it says so and exits 1.

## Run each role

```sh
cobc -x -o shop SHOP.cbl
```

The web server:

```sh
DATABASE_URL=postgres://shop@db:5432/shop DOCUCONF_ROLE=web ALLOWED_ORIGINS=https://shop.example \
  docuconf exec -contract contract.json -- ./shop
```

```text
shop web: port 8080, origins https://shop.example
```

The worker, which needs no `ALLOWED_ORIGINS`:

```sh
DATABASE_URL=postgres://shop@db:5432/shop DOCUCONF_ROLE=worker QUEUES=default,mail \
  docuconf exec -contract contract.json -- ./shop
```

```text
shop worker: queues default,mail, webhooks off
```

With json logs and no `LOG_SCHEMA_URL`, `docuconf exec` lists every problem, never shows a secret, and exits 1 before the program starts:

```sh
DATABASE_URL=postgres://shop@db:5432/shop DOCUCONF_ROLE=worker QUEUES=default LOG_FORMAT=json \
  docuconf exec -contract contract.json -- ./shop || echo "exit $?"
```

```text
docuconf: 1 configuration problem:
  LOG_SCHEMA_URL: is required when LOG_FORMAT is "json" but not set (missing_required)
exit 1
```

With webhooks on, the worker needs the signing key file. Set `DOCUCONF_FILE_ROOT` to check the files under another directory, as a local run or a test does:

```sh
DATABASE_URL=postgres://shop@db:5432/shop DOCUCONF_ROLE=worker QUEUES=default WEBHOOKS_ENABLED=true \
  docuconf exec -contract contract.json -- ./shop || echo "exit $?"
```

```text
docuconf: 1 configuration problem:
  webhook-signing-key: /etc/shop/webhooks/signing.key does not exist, and is required when WEBHOOKS_ENABLED is true (file_missing)
exit 1
```

The same setting for the web role starts: the key file belongs to the worker.

## Tests

`tests/shop_test.go` (part of `go test ./...`) builds SHOP and runs this matrix through `docuconf exec`. Each case starts from `DATABASE_URL=postgres://shop@db:5432/shop` and a fresh, empty `DOCUCONF_FILE_ROOT`, and checks the exit code and the exact output: the line SHOP prints, or the one problem `docuconf exec` reports.

| # | Environment (plus the base) | Expected |
|---|---|---|
| 1 | `DOCUCONF_ROLE=web ALLOWED_ORIGINS=https://shop.example` | ok: `shop web: port 8080, origins https://shop.example` |
| 2 | `DOCUCONF_ROLE=worker QUEUES=default,mail` | ok: the worker needs no `ALLOWED_ORIGINS` |
| 3 | `DOCUCONF_ROLE=web` | `ALLOWED_ORIGINS` `missing_required` |
| 4 | `QUEUES=default` (no role) | `DOCUCONF_ROLE` `missing_required`, and nothing else |
| 5 | `DOCUCONF_ROLE=cron` | `DOCUCONF_ROLE` `not_in_enum`, and nothing else |
| 6 | `DOCUCONF_ROLE=worker QUEUES=default LOG_FORMAT=json` | `LOG_SCHEMA_URL` `missing_required`: `is required when LOG_FORMAT is "json"` |
| 7 | as 6, plus `LOG_SCHEMA_URL=https://logs.example/v1.json` | ok |
| 8 | `DOCUCONF_ROLE=worker QUEUES=default WEBHOOKS_ENABLED=true`, no key file | `webhook-signing-key` `file_missing`: `, and is required when WEBHOOKS_ENABLED is true` |
| 9 | as 8, with a 40-character key file under the root | ok: `webhooks on` |
| 10 | `DOCUCONF_ROLE=web ALLOWED_ORIGINS=https://shop.example WEBHOOKS_ENABLED=true` | ok: the key file belongs to another role |
| 11 | `DOCUCONF_ROLE=web ALLOWED_ORIGINS=https://shop.example OTEL_EXPORTER_OTLP_PROTOCOL=carrier-pigeon` | `OTEL_EXPORTER_OTLP_PROTOCOL` `not_in_enum`: fragment variables are checked at boot |

[`smoke.sh`](smoke.sh) builds SHOP and runs cases 1, 2 and 6 with the real process:

<!-- not executed: CI runs them -->
```sh
DOCUCONF=/path/to/docuconf examples/shop/smoke.sh
go test ./...
```
