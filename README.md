# docuconf-cobol

The COBOL SDK for [docuconf](https://github.com/docuconf/docuconf-go): typed configuration contracts between an app and the Kubernetes platform that runs it. You declare your configuration once, as an annotated copybook. `docuconf-cobol generate` turns it into a CUE contract for the platform and a loader program your code CALLs, and `docuconf exec` checks the real environment and files against the contract when the container starts.

- Example: [`examples/orders`](examples/orders), a CronJob-style batch program, with a step-by-step walkthrough
- Licence: MIT

```cobol
      *> @service orders-batch  @prefix CFG-  @program ORDCFG
       01  ORDERS-CONFIG.
      *> Port of the Prometheus metrics endpoint
      *> @min 1  @max 65535  @default 8080
           05  CFG-PORT                PIC 9(5).
      *> Postgres connection string of the orders database
      *> @type url  @schemes postgres  @secret  @required
           05  CFG-DATABASE-URL        PIC X(200).
      *> Time allowed for each database call, in milliseconds
      *> @unit ms  @min 1s  @max 5m  @default 30s
           05  CFG-REQUEST-TIMEOUT     PIC 9(6).
```

```sh
docuconf-cobol generate orders-config.cpy       # writes contract.cue and ORDCFG.cbl
cobc -x -o orders-batch ORDERS-BATCH.cbl ORDCFG.cbl
docuconf exec --contract contract.cue -- ./orders-batch
```

```cobol
           CALL "ORDCFG" USING ORDERS-CONFIG
           IF RETURN-CODE NOT = 0
               STOP RUN RETURNING 1
           END-IF
```

## How docuconf works in COBOL

The other docuconf SDKs validate everything inside the app. COBOL cannot reasonably parse X.509 certificates, run RE2 patterns or check JSON Schemas, so the work is split in three:

1. **The declaration is a copybook.** It is the record your program already uses, with comment annotations. The PIC clauses give the types and limits (`PIC 9(5)` is an int from 0 to 99999, `PIC X(64)` a string of at most 64 characters, `OCCURS 8` a list of at most 8 items, level-88s an enum), and the tags give the rest (descriptions, defaults, secrets, URL schemes, file inputs).
2. **`docuconf-cobol generate`** (a Go tool built on the docuconf-go SDK) reads the copybook and writes `contract.cue`, which the platform validates its values against before deploying, and a loader program. The loader reads each variable with `ACCEPT ... FROM ENVIRONMENT`, applies defaults and converts the wire values into the typed fields: `NUMVAL` for numbers, csv, json or indexed lists into the `OCCURS` table and its count, durations in any of the four wire encodings into the unit the field counts in. It is self-contained: one `.cbl` file that `COPY`s your copybook.
3. **`docuconf exec`** (in the [docuconf CLI](https://github.com/docuconf/docuconf-go#boot-validation-for-any-language-docuconf-exec-and-docuconf-check)) is the container's entrypoint. It checks the environment and file inputs with the Go SDK's contract-first loader, which passes the whole conformance suite: ranges, patterns, schemes, list bounds, JSON Schemas, TLS certificates. On a problem it prints every violation (never a secret value), writes the termination log and exits 1. Otherwise it `exec`s your program, which keeps the PID and receives signals directly.

```dockerfile
ENTRYPOINT ["docuconf", "exec", "--contract", "/etc/docuconf/contract.cue", "--", "/app/orders-batch"]
```

The loader still checks what only COBOL can know: a value that is valid for the contract but does not fit its field (a 5-character name with an accented letter is more than `PIC X(5)` bytes; `0.125` has more decimal places than `PIC 9V99`; `1500us` is finer than a field counting milliseconds). It also reports a missing required variable and a value of the wrong type, so a program run without `docuconf exec` fails loudly rather than reading spaces. It prints one line per problem on stderr, in the same `NAME: message (code)` form, and returns `RETURN-CODE` 1.

## Annotations

Annotations are comment lines (`*>` in column 7) directly above an item, so the same copybook works in fixed format (columns 8 to 72) and free format. A comment line that does not start with `@` is the item's description; a line that starts with `@` holds tags, as many as fit. A value with spaces is quoted, with `""` for a quote, as in a COBOL literal. Tags are case-insensitive, and `@max-length`, `@maxLength` and `@maxlength` are the same tag. A blank line ends a comment block.

| Where | Tag | Meaning |
|---|---|---|
| 01 record | `@service <name>` | the service name in the contract (or `-name`) |
| | `@prefix <PREFIX->` | dropped from data names to derive variable names: `CFG-LOG-LEVEL` is `LOG_LEVEL` |
| | `@program <NAME>` | the loader's PROGRAM-ID (default `LOAD-<record>`) |
| | `@package <name>` | the CUE package of contract.cue |
| group item | `@group <name>` | the contract `group` of every item under it |
| any field | `@env <NAME>` | the variable name, instead of the derived one |
| | `@desc "<text>"` | the description, instead of the comment text |
| | `@type <type>` | `string`, `int`, `float`, `bool`, `duration`, `url`, `enum`, `json`, when the PIC does not say |
| | `@required`, `@secret` | |
| | `@default <value>...` | the default, one value, or one per item for a list |
| | `@present <FIELD>` | a `PIC X` field the loader sets to `Y` when the variable has a value (set or defaulted), else `N` |
| | `@examples <v>...`, `@deprecated "<msg>"`, `@config-key <key>` | as in the contract |
| | `@ignore` | the field is not configuration |
| string | `@min-length`, `@max-length`, `@pattern "<re2>"` | `maxLength` defaults to the PIC X size |
| int | `@min`, `@max` | within what the PIC holds, which is exported anyway |
| float | `@min`, `@max` | on a `PIC S9(n)V9(m)` field, or `COMP-1`/`COMP-2` with `@type float` |
| duration | `@unit ns\|us\|ms\|s\|m\|h` | what the numeric field counts in; required. `PIC 9(4)V999` with `@unit s` keeps milliseconds |
| | `@min`, `@max` | Go durations (`1s`, `5m`) |
| | `@encoding go\|iso8601\|seconds\|timespan` | the wire form (default `go`) |
| url | `@schemes <s>...`, `@max-length` | `maxLength` defaults to the PIC X size |
| enum | level-88 `VALUE "x"` items, or `@values <v>...` | |
| bool | `PIC X` (Y/N) or `PIC 9` (1/0), with `@type bool` | |
| json | `@schema <file.json>`, `@max-length` | the variable's raw JSON text goes into the `PIC X` field; `docuconf exec` checks it against the schema, and its length as received against `maxLength`, which defaults to the PIC X size |
| list (`OCCURS`) | `@count <FIELD>`, or `OCCURS 1 TO n DEPENDING ON <FIELD>` | the field that receives the number of items |
| | `@min-items`, `@max-items` | `maxItems` defaults to the `OCCURS` size; `OCCURS a TO b` sets both |
| | `@item-min`, `@item-max` | for int items |
| | `@item-min-length`, `@item-max-length` | for string items; `itemMaxLength` defaults to the PIC X size of one entry |
| | `@encoding csv\|json\|indexed`, `@separator "<s>"` | the wire form (default csv, `,`) |
| file input (a `PIC X` field that receives its path) | `@file <name> [<type>]` | type `text` (default), `binary`, `config`, `caBundle`, `tls` or `keystore` |
| | `@path </abs/path>`, `@path-env <NAME>` | where the platform mounts it; the loader puts the effective path in the field, with `DOCUCONF_FILE_ROOT` in front when set |
| | `@required`, `@max-size <n[Ki\|Mi\|Gi]>`, `@reload restart` | |
| | `@format`, `@schema`, `@dns-names`, `@key-algorithms`, `@min-remaining`, `@require-ca`, `@min-certificates`, `@password-var`, `@pattern`, `@min-length`, `@max-length` | per file type, as in the contract |

Every elementary item of the record is configuration unless it is `FILLER`, marked `@ignore`, or named by another item's `@count`, `@present` or `DEPENDING ON`. A comment block above a count field that comes before its table describes the table.

Mistakes are reported with the copybook line, for example:

```
orders-config.cpy:7: CFG-PORT: @max 100000 is outside what PIC 9(5) holds (0 to 99999)
orders-config.cpy:7: CFG-PORT (PORT): default 0 is below min 1
orders-config.cpy:1: comment runs past column 72, where cobc stops reading; continue it on the next *> line
```

### Limits

- A COBOL field is padded with spaces, so a value's trailing spaces are lost. Leading spaces and other trailing characters (a newline) are kept.
- `PIC X(n)` holds n bytes, and the contract's `maxLength` and `itemMaxLength` count characters (Unicode code points), so a value with multi-byte UTF-8 characters can pass `docuconf exec` and still not fit: `ZÜ01` is 4 characters but 5 bytes. The loader reports it as `out_of_range`. To have the platform reject such values before deploying, declare a smaller `@max-length` (or `@item-max-length`) that leaves room for them, or restrict the value to ASCII with `@pattern "^[ -~]*$"`. Strings, URLs and json values get a `maxLength`, and string list items an `itemMaxLength`, from their PIC X size; an enum's values are checked against the field when the copybook is generated.
- The loader reads values of up to 8191 bytes and list items of up to 1024 bytes, and lists of up to 1000 items.
- A json variable is passed through as text; GnuCOBOL 3 has no JSON PARSE.
- `OCCURS DEPENDING ON` must be on the record's last item, as cobc requires; otherwise use `OCCURS n` with `@count`.
- Config overlays and profiles (SPEC §4.7, §4.4) do not apply: a COBOL program reads its configuration from the environment.

## Install

```sh
go install github.com/docuconf/docuconf-cobol/cmd/docuconf-cobol@latest
go install github.com/docuconf/docuconf-go/cmd/docuconf@latest     # for docuconf exec
```

Release binaries with checksums (`SHA256SUMS`) are on the GitHub releases page, and the generator is also an image, `ghcr.io/docuconf/docuconf-cobol` (linux/amd64, arm64 and s390x), holding one static binary at `/docuconf-cobol`: `COPY --from=ghcr.io/docuconf/docuconf-cobol:<version> /docuconf-cobol /usr/local/bin/docuconf-cobol`. The loader needs GnuCOBOL 3 (`apt-get install gnucobol3`); it uses only standard COBOL plus `ACCEPT ... FROM ENVIRONMENT`, `FUNCTION TRIM` and `NUMVAL-F`.

```
docuconf-cobol generate [-free] [-name svc] [-program PGM] [-package pkg] [-prefix CFG-] [-o dir] [-check] <copybook>
```

`generate` writes `contract.cue` and `<PROGRAM>.cbl` next to the copybook (or in `-o`). The loader `COPY`s the copybook by its file name, so compile with the copybook's directory on cobc's copy path (`-I`) or in the current directory. `-check` writes nothing and exits 1 when either file is out of date. `-free` reads a free-format copybook; the loader compiles in either format.

## Conformance

`go test ./tests` runs the shared conformance suite (SPEC §12) through the whole COBOL boot path. For each case, it writes a copybook declaring the case's variables, generates the loader, compiles a program that CALLs it, and runs that program under `docuconf exec` with the case's environment and contract:

- **68 cases with typed values**: `docuconf exec` accepts the environment, and every field the loader filled in (strings, ints up to the 64-bit limit, floats, bools, durations in all four encodings, csv, json and indexed lists, json text, unset optionals through `@present`) matches the expected value.
- **47 error cases**: `docuconf exec` reports each expected violation code and the program never starts.
- **0 skipped.** No case needs a type COBOL cannot represent. String values are compared without trailing spaces (see Limits); no case depends on them.

`go test ./tests` also runs the loader's own checks without `docuconf exec`. It needs `cobc` and the docuconf CLI:

```sh
DOCUCONF=$(which docuconf) \
DOCUCONF_CONFORMANCE=../docuconf-go/conformance/cases.json \
DOCUCONF_REQUIRE_CONFORMANCE=1 go test ./...
```

Without them the suite skips, unless `DOCUCONF_REQUIRE_CONFORMANCE=1`. `DOCUCONF_SPEC` points the contract tests at docuconf-go's `spec/cue` for `cue vet -c`.

## Development

`docuconf-cobol` imports `github.com/docuconf/docuconf-go` for `ContractCUE` (which checks a contract as the SDKs do and writes it in the standard layout) and its contract-first checks. Until those are in a docuconf-go release, `go.mod` pins a docuconf-go commit that has them (currently the one that adds `maxLength` on url and json values and item lengths on string lists), by pseudo-version; no `replace` directive is used.

The runtime the loader is built on is in [`copybooks/`](copybooks): `DCRTWS.cpy` (working storage) and `DCRTPD.cpy` (paragraphs: reading variables, parsing ints, floats, bools, durations, csv and JSON lists, applying `DOCUCONF_FILE_ROOT`). The generator inlines both into each loader.
