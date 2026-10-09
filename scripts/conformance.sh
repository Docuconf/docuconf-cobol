#!/usr/bin/env bash
# Runs the shared conformance suite (docuconf-go conformance/cases.json,
# through generated loaders and `docuconf exec`) and the test that `cue vet`s
# generated contracts against the meta-schema (docuconf-go spec/cue). Not the
# full suite. Used by this repo's CI and by docuconf-go's downstream gate.
#
#   DOCUCONF_GO_DIR=/path/to/docuconf-go scripts/conformance.sh
#
# Everything is built against that checkout: the docuconf CLI from its
# cmd/docuconf, and docuconf-cobol itself through a temporary go.work that
# uses it in place of the version go.mod pins. The committed go.mod and
# go.sum are not touched. Needs Go, GnuCOBOL (cobc) and cue on PATH.
set -euo pipefail

: "${DOCUCONF_GO_DIR:?set DOCUCONF_GO_DIR to a docuconf-go checkout}"
DOCUCONF_GO_DIR=$(cd "$DOCUCONF_GO_DIR" && pwd)
export DOCUCONF_GO_DIR
export DOCUCONF_CONFORMANCE="${DOCUCONF_CONFORMANCE:-$DOCUCONF_GO_DIR/conformance/cases.json}"
export DOCUCONF_SPEC_CUE="${DOCUCONF_SPEC_CUE:-$DOCUCONF_GO_DIR/spec/cue}"
export DOCUCONF_SPEC="$DOCUCONF_SPEC_CUE" # the name this repository's tests read
export DOCUCONF_REQUIRE_CONFORMANCE=1
export DOCUCONF_REQUIRE_VET=1

# TestCueVet skips itself when cue is missing; here that is an error.
if ! command -v cue >/dev/null; then
  echo "cue not found on PATH" >&2
  exit 1
fi

repo=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# Build and test against the checkout, not the pinned pseudo-version:
# docuconf-cobol, and the docuconf CLI, whose own go.mod pins the SDK by
# version and may lag behind the checkout.
(cd "$work" && go work init "$repo" "$DOCUCONF_GO_DIR" "$DOCUCONF_GO_DIR/cmd/docuconf")
export GOWORK="$work/go.work"

# The docuconf CLI (docuconf exec) from the same checkout.
(cd "$DOCUCONF_GO_DIR/cmd/docuconf" && go build -o "$work/bin/docuconf" .)
export DOCUCONF="$work/bin/docuconf"

cd "$repo"
go build ./...
go test -count=1 ./internal/gen -run '^TestCueVet$' -v
# -v for the runner's counts (cases run, covered by each pass, skipped),
# without the per-case lines.
go test -count=1 -v ./tests -run '^(TestConformance|TestExportFixture)$' 2>&1 \
  | grep -Ev '^ *(=== RUN|--- PASS|=== PAUSE|=== CONT)'
