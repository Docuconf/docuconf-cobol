# docuconf-cobol

The COBOL SDK for [docuconf](https://github.com/docuconf/docuconf-go): typed configuration contracts between an app and the Kubernetes platform that runs it. You declare your configuration once, as an annotated copybook. `docuconf-cobol generate` turns it into a CUE contract for the platform and a loader program your code CALLs, and `docuconf exec` checks the real environment and files against the contract when the container starts.

Documentation: [docuconf.dev](https://docuconf.dev) · [COBOL guide](https://docuconf.dev/languages/cobol/)

- Example: [`examples/orders`](examples/orders), a CronJob-style batch program, with a step-by-step walkthrough
- Licence: MIT

## 1. Install

You need Go 1.25 or later and GnuCOBOL 3 (`apt-get install gnucobol3`). Until the first release is tagged, install both tools from source:

```sh
go install github.com/docuconf/docuconf-cobol/cmd/docuconf-cobol@latest
# docuconf exec is not in a docuconf-go release yet. This is the commit
# this SDK is tested against (go.mod pins the same one):
go install github.com/docuconf/docuconf-go/cmd/docuconf@v0.0.0-20261008010717-a84031e0174b
```

Once docuconf-go tags a release that includes `docuconf exec`, use that version instead of the commit.

From the first release on, each tag also publishes the generator as release binaries with checksums (`SHA256SUMS`) on the GitHub releases page, and as an image, `ghcr.io/docuconf/docuconf-cobol` (linux/amd64, arm64 and s390x), holding one static binary at `/docuconf-cobol`: `COPY --from=ghcr.io/docuconf/docuconf-cobol:<version> /docuconf-cobol /usr/local/bin/docuconf-cobol`. The loader needs GnuCOBOL 3 either way; it uses only standard COBOL plus `ACCEPT ... FROM ENVIRONMENT`, `FUNCTION TRIM` and `NUMVAL-F`.

## 2. Declare: annotate the copybook

The record your program already uses is the declaration. The comment above a field is its description (its first paragraph) and details (the rest), and `@` tags add what the PIC clause cannot say. This is [`examples/orders/orders-config.cpy`](examples/orders/orders-config.cpy):

```cobol
      *> Configuration of the ORDERS-BATCH job, read from the
      *> environment by the generated loader ORDCFG.
      *> @service orders-batch  @prefix CFG-  @program ORDCFG
       01  ORDERS-CONFIG.
      *> Port of the Prometheus metrics endpoint
      *> @min 1  @max 65535  @default 8080
           05  CFG-PORT                PIC 9(5).
```

`CFG-PORT` becomes the variable `PORT` (the `@prefix` is dropped, hyphens become underscores). `PIC 9(5)` makes it an int from 0 to 99999, and `@max 65535` narrows that. Generate the contract and the loader:

```sh
docuconf-cobol generate orders-config.cpy
```

```text
wrote contract.cue
wrote ORDCFG.cbl
```

Commit both. `docuconf-cobol generate -check` (in CI) fails when they no longer match the copybook, and prints the exact command that regenerates them.

## 3. Run: CALL the loader, start under docuconf exec

Your program CALLs the loader once, before it reads any configuration. This is the start of [`ORDERS-BATCH.cbl`](examples/orders/ORDERS-BATCH.cbl):

```cobol
       MAIN.
           CALL "ORDCFG" USING ORDERS-CONFIG
           IF RETURN-CODE NOT = 0
               STOP RUN
           END-IF
```

On a problem the loader has already printed every problem and written the termination log, and `RETURN-CODE` is 1, so `STOP RUN` ends the job with exit code 1. The same form works with IBM Enterprise COBOL. Build and run:

```sh
cobc -x -o orders-batch ORDERS-BATCH.cbl ORDCFG.cbl
DATABASE_URL=postgres://orders:s3cret@db:5432/orders ORDERS_FILE=orders.txt \
  docuconf exec -contract contract.cue -- ./orders-batch
```

`docuconf exec` checks the environment and the file inputs against `contract.cue`, then replaces itself with the program, passing it the environment it checked, with the contract's defaults filled in.

## 4. See an error

With `PORT=0` and no `DATABASE_URL`, the job does not start:

```sh
PORT=0 ORDERS_FILE=orders.txt docuconf exec -contract contract.cue -- ./orders-batch
```

```text
docuconf: 2 configuration problems:
  DATABASE_URL: is required but not set (missing_required)
  PORT: 0 is below min 1 (out_of_range)
```

It exits 1 and writes the same lines to the termination log, so `kubectl describe pod` shows them. Run without `docuconf exec`, the loader catches the same `PORT=0` itself: it checks required variables, types, enums, `@min`/`@max` (and `@item-min`, `@item-max`, `@min-items`, `@max-items`), plus what only COBOL can know, a value that does not fit its field. It never prints a value.

## 5. Test your config

Test the contract and the loader with an explicit environment, so a test never depends on the shell it runs in. [`test-config.sh`](examples/orders/test-config.sh) does both, and CI runs it:

```sh
env -i PATH="$PATH" DOCUCONF_TERMINATION_LOG=- "$docuconf" check -contract contract.cue -env-file "$work/good.env"
env -i DATABASE_URL=postgres://orders:pw@db/orders WORKER_COUNT=7 \
	DOCUCONF_TERMINATION_LOG=- "$work/cfgtest"
```

`docuconf check` runs the contract's checks without starting anything. [`CFGTEST.cbl`](examples/orders/CFGTEST.cbl) CALLs the loader and checks the fields it stored:

```cobol
      *> WORKER_COUNT=7 is set; every other optional takes its default.
           IF CFG-WORKER-COUNT NOT = 7
               DISPLAY "FAIL: WORKER_COUNT " CFG-WORKER-COUNT
               END-DISPLAY
               ADD 1 TO WS-FAILS
           END-IF
```

## 6. Export

`contract.cue` is the export. The platform team validates its values against it with `docuconf vet` and renders the pod's environment with `docuconf render`, or uses the [Helm library chart](https://github.com/docuconf/docuconf-go/tree/main/helm). The lengths the PIC sets are in the contract: `maxLength` on strings, URLs and json values, and `itemMaxLength` on string list items; the loader also enforces them at boot, in bytes.

## 7. Deploy

The image's entrypoint is `docuconf exec`, so the job never starts with a bad configuration. From [`examples/orders/Dockerfile`](examples/orders/Dockerfile):

```dockerfile
ENTRYPOINT ["docuconf", "exec", "-contract", "/etc/docuconf/contract.cue", "--", "/app/orders-batch"]
```

The [example's walkthrough](examples/orders/README.md#6-deploy) has the CronJob.

## Reference

### How docuconf works in COBOL

The other docuconf SDKs validate everything inside the app. COBOL cannot reasonably parse X.509 certificates, run RE2 patterns or check JSON Schemas, so the work is split in three:

1. **The declaration is a copybook.** The PIC clauses give the types and limits (`PIC 9(5)` is an int from 0 to 99999, `PIC X(64)` a string of at most 64 characters, `OCCURS 8` a list of at most 8 items, level-88s an enum), and the tags give the rest.
2. **`docuconf-cobol generate`** (a Go tool built on the docuconf-go SDK) writes `contract.cue` and a loader program. The loader reads each variable with `ACCEPT ... FROM ENVIRONMENT`, applies defaults, converts the wire values into the typed fields (`NUMVAL` for numbers, csv, json or indexed lists into the `OCCURS` table and its count, durations in any of the four wire encodings into the unit the field counts in), and checks ranges, enums and what fits.
3. **`docuconf exec`** (in the [docuconf CLI](https://github.com/docuconf/docuconf-go)) is the container's entrypoint. It checks everything with the Go SDK's contract-first loader, which passes the whole conformance suite: ranges, patterns, schemes, list bounds, JSON Schemas, TLS certificates. It passes the program the environment it checked: `-env-file` values fill what the process environment does not set, and every variable still unset gets its contract default in its wire encoding (`-no-defaults` turns that off).

The loader's problems look like the other SDKs': a `docuconf: N configuration problems:` line, then one `NAME: message (code)` line per problem, on stderr and in the termination log (`DOCUCONF_TERMINATION_LOG`, `-` for none, else `/dev/termination-log` when it exists).

### Annotations

Annotations are comment lines directly above an item (`*>` anywhere, or `*` in column 7 of a fixed-format copybook). A comment line that does not start with `@` is text; a line that starts with `@` holds tags, as many as fit. Tags may also follow text on a line, including an inline comment after the field: `05 CFG-PORT PIC 9(5).  *> Port number @default 80` is a description and a default. Only a known tag starts the tags, so `ops@example.com` in a description stays text. A value with spaces is quoted, with `""` for a quote, as in a COBOL literal. Tags are case-insensitive, and `@max-length`, `@maxLength` and `@maxlength` are the same tag. A misspelt tag is an error that suggests the right one. A blank line ends a comment block.

#### Descriptions and details

The first paragraph of the text is the item's `description`, on one line; every input needs one, of at least 5 characters. An empty comment line (`*>` alone) ends a paragraph, and the text after the first paragraph is the item's `details`: CommonMark, for generated docs only, never read at runtime. COBOL has no doc comment syntax of its own, so the text is used as written, with its indentation, and Markdown such as lists, `code` and fenced code blocks (inside which a line starting with `@` is text, not a tag) works as in any Markdown file. Lines of punctuation only, such as `*> -----`, are dropped. `@desc` and `@details` set either explicitly. Details must not be blank and have at most 4000 characters (Unicode code points); `generate` fails otherwise, as it does for a missing description. From the example:

```cobol
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
```

`docuconf docs` (in the [docuconf CLI](https://github.com/docuconf/docuconf-go)) generates CONFIG.md and CONFIG.agents.md from the exported contract, with the details under each input:

```sh
docuconf docs contract.cue -o CONFIG.md
docuconf docs contract.cue --format agents -o CONFIG.agents.md
grep -A5 'Each worker reads' CONFIG.md
```

```text
Each worker reads its share of the orders file and holds
one database connection, so keep this at or below the
pool size:

- one connection per worker;
- plus one for the summary step.
```

| Where | Tag | Meaning |
|---|---|---|
| 01 record (or the first field of a copybook with no 01) | `@service <name>` | the service name in the contract (or `-name`); an upper-case name is lower-cased, with a warning |
| | `@prefix <PREFIX->` | dropped from data names to derive variable names: `CFG-LOG-LEVEL` is `LOG_LEVEL` |
| | `@program <NAME>` | the loader's PROGRAM-ID (default `<MEMBER>L` for a copybook named like a PDS member, `ORDCFGC.cpy` giving `ORDCFGCL`, else `LOAD-<record>`) |
| | `@package <name>` | the CUE package of contract.cue |
| group item | `@group <name>` | the contract `group` of every item under it |
| any field | `@env <NAME>` | the variable name, instead of the derived one |
| | `@desc "<text>"` | the description, instead of the comment's first paragraph (at least 5 characters, as the spec requires) |
| | `@details "<text>"` | the details, instead of the comment after its first paragraph (CommonMark, at most 4000 characters) |
| | `@type <type>` | `string`, `int`, `float`, `bool`, `duration`, `url`, `enum`, `json`, when the PIC does not say |
| | `@required`, `@secret` | neither may have a default |
| | `@default <value>...` | the default, one value, or one per item for a list. A `VALUE` clause with a literal is the default when there is no `@default` (`VALUE SPACES` or `ZEROS` only initialises); if both are given they must agree |
| | `@present <FIELD>` | a `PIC X` field the loader sets to `Y` when the variable has a value (set or defaulted), else `N` |
| | `@examples <v>...`, `@deprecated "<msg>"`, `@config-key <key>` | as in the contract |
| | `@ignore` | the field is not configuration |
| string | `@min-length`, `@max-length`, `@pattern "<re2>"` | `maxLength` defaults to the PIC X size |
| int | `@min`, `@max` | within what the PIC holds, which is exported anyway |
| float | `@min`, `@max` | on a `PIC S9(n)V9(m)` field, or `COMP-1`/`COMP-2` with `@type float`; a default or bound with more decimal places than the PIC is an error |
| duration | `@unit ns\|us\|ms\|s\|m\|h` | what the numeric field counts in; required. `PIC 9(4)V999` with `@unit s` keeps milliseconds. A `VALUE` counts in the unit |
| | `@min`, `@max` | Go durations (`1s`, `5m`) |
| | `@encoding go\|iso8601\|seconds\|timespan` | the wire form (default `go`) |
| url | `@schemes <s>...`, `@max-length` | `maxLength` defaults to the PIC X size |
| enum | level-88 `VALUE "x"` items, or `@values <v>...` | |
| bool | `PIC X` (`Y`/`N`) or `PIC 9` (`1`/`0`), with `@type bool` | the variable is `true`/`false` on the wire; `@default` takes `true`, `false`, `Y`, `N`, `1` or `0`. Without `@type bool`, a `PIC X` with `88 F-ON VALUE "Y"` and `88 F-OFF VALUE "N"` is an enum of `Y` and `N`, set as `VERBOSE=Y` |
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

Mistakes are reported with the copybook line, in line order. For [`bad-config.cpy`](tests/testdata/bad-config.cpy):

<!-- generate: tests/testdata/bad-config.cpy -->
```text
bad-config.cpy:4: CFG-PORT: unknown tag @defualt; did you mean @default?
bad-config.cpy:4: CFG-PORT: @max 100000 is outside what PIC 9(5) holds (0 to 99999)
bad-config.cpy:7: CFG-RATIO: @default: 0.125 has more decimal places than PIC 9V99 holds
```

### Copybook shapes

- **Fixed format** (the default): columns 1-6 (sequence numbers) and 73-80 (the identification area) are ignored, as cobc ignores them. A comment whose annotation runs into column 73 gets a warning. **Free format**: pass `-free`; forgetting it gives `column 7 holds 'r'; is this a free-format copybook? (use -free)`.
- **Several 01 records**: pass `-record <NAME>` to pick the configuration record. The loader COPYs the whole copybook.
- **No 01 record** (the program writes `01 WS-CFG. COPY CFGFLDS.`): the loader declares its own 01 around the fields, `DC-RECORD` unless `-record` names another, and your program CALLs it `USING WS-CFG`. Put the record's tags above the first field.
- The loader COPYs the copybook as a member (`COPY ORDCFGC.`) when its file name is a member name of up to 8 upper-case characters with a `.cpy`, `.cbl` or `.cob` extension, and by quoted file name otherwise. Compile with the copybook's directory on cobc's copy path (`-I`) or in the current directory.
- Names starting `DC` are the loader's own (working storage `DC-...`, paragraphs `DC-...`, `DCV-...`, `DCI-...`, `DCF-...`). A copybook name that collides with one is an error at generate time, not a cobc error later.

### Limits

- A COBOL field is padded with spaces, so a value's trailing spaces are lost. Leading spaces and other trailing characters (a newline) are kept.
- `PIC X(n)` holds n bytes, and the contract's `maxLength` and `itemMaxLength` count characters (Unicode code points), so a value with multi-byte UTF-8 characters can pass `docuconf exec` and still not fit: `ZÜ01` is 4 characters but 5 bytes. The loader reports it as `out_of_range`. To have the platform reject such values before deploying, declare a smaller `@max-length` (or `@item-max-length`) that leaves room for them, or restrict the value to ASCII with `@pattern "^[ -~]*$"`. Strings, URLs and json values get a `maxLength`, and string list items an `itemMaxLength`, from their PIC X size; an enum's values are checked against the field when the copybook is generated.
- The loader reads values of up to 8191 bytes and list items of up to 1024 bytes, and lists of up to 1000 items. It reports at most 100 problems in full.
- `@min-length`, `@max-length` and `@pattern` on strings, URL schemes, JSON Schemas and file contents are checked by `docuconf exec` only.
- A json variable is passed through as text; GnuCOBOL 3 has no JSON PARSE.
- `OCCURS DEPENDING ON` must be on the record's last item, as cobc requires; otherwise use `OCCURS n` with `@count`.
- Config overlays and profiles (SPEC §4.7, §4.4) do not apply: a COBOL program reads its configuration from the environment.

### Command line

```usage
docuconf-cobol generate [-free] [-name svc] [-program PGM] [-package pkg] [-prefix CFG-] [-record NAME]
                        [-runtime inline|copy] [-o dir] [-contract contract.cue] [-check] <copybook>
docuconf-cobol runtime [-o dir]
```

`generate` writes `contract.cue` (or `-contract`, for a repository with several jobs) and `<PROGRAM>.cbl` next to the copybook, or in `-o`, which it creates. Flags may come before or after the copybook. `-check` writes nothing and exits 1 when either file is out of date. `-free` reads a free-format copybook; the loader compiles in either format.

By default each loader holds the docuconf runtime (about 750 lines, and a 1 MB item table in working storage). With `-runtime copy`, the loader instead COPYs `DCRTWS` and `DCRTPD`, which `docuconf-cobol runtime -o <copylib>` writes once into a shared copy library; a runtime fix then means replacing two copybooks and recompiling. Keep the copybooks at the version of the `docuconf-cobol` that generated the loaders.

### Conformance

`go test ./tests` runs the shared conformance suite (SPEC §12) through the whole COBOL boot path. For each case, it writes a copybook declaring the case's variables, generates the loader, compiles a program that CALLs it, and runs that program under `docuconf exec` with the case's environment and contract:

- **Cases with typed values**: `docuconf exec` accepts the environment, and every field the loader filled in (strings, ints up to the 64-bit limit, floats, bools, durations in all four encodings, csv, json and indexed lists, json text, unset optionals through `@present`) matches the expected value.
- **Error cases**: `docuconf exec` reports each expected violation code and the program never starts.
- **0 skipped.** String values are compared without trailing spaces (see Limits); no case depends on them.

It also runs the loader's own checks without `docuconf exec`. It needs `cobc` and the docuconf CLI:

<!-- not executed: CI runs it -->
```sh
DOCUCONF=$(which docuconf) \
DOCUCONF_CONFORMANCE=../docuconf-go/conformance/cases.json \
DOCUCONF_REQUIRE_CONFORMANCE=1 go test ./...
```

Without them the suite skips, unless `DOCUCONF_REQUIRE_CONFORMANCE=1`. `DOCUCONF_SPEC` points the contract tests at docuconf-go's `spec/cue` for `cue vet -c`. `go test ./tests` also checks that every code block in this README and the example's walkthrough is a piece of a file CI builds or runs.

### Development

`docuconf-cobol` imports `github.com/docuconf/docuconf-go` for `ContractCUE` (which checks a contract as the SDKs do and writes it in the standard layout) and its contract-first checks. Until those are in a docuconf-go release, `go.mod` pins a docuconf-go commit that has them (currently one that adds `details` to inputs and the `docuconf docs` command, and whose `exec` passes `-env-file` values and contract defaults to the program), by pseudo-version, with no `replace` directive. CI checks out the same commit to build the `docuconf` CLI and to read the spec and conformance cases.

The runtime the loader is built on is in [`copybooks/`](copybooks): `DCRTWS.cpy` (working storage) and `DCRTPD.cpy` (paragraphs: reading variables, parsing ints, floats, bools, durations, csv and JSON lists, applying `DOCUCONF_FILE_ROOT`, reporting problems).
