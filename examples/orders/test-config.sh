#!/bin/sh
# Tests the orders job's configuration, as the README's "Testing your
# config" shows: the contract with docuconf check, and the loader with
# CFGTEST.cbl, each with an explicit environment.
#
#   DOCUCONF=/path/to/docuconf examples/orders/test-config.sh
set -eu

here=$(cd "$(dirname "$0")" && pwd)
docuconf=${DOCUCONF:-docuconf}
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cd "$here"

# 1. The contract accepts a known-good environment and rejects a bad one.
printf 'DATABASE_URL=postgres://orders:pw@db/orders\nORDERS_FILE=%s\n' "$here/orders.txt" >"$work/good.env"
env -i PATH="$PATH" DOCUCONF_TERMINATION_LOG=- "$docuconf" check -contract contract.cue -env-file "$work/good.env"
if env -i PATH="$PATH" DOCUCONF_TERMINATION_LOG=- WORKER_COUNT=65 \
	"$docuconf" check -contract contract.cue -env-file "$work/good.env" 2>"$work/err"; then
	echo "test-config: FAIL: WORKER_COUNT=65 was accepted" >&2
	exit 1
fi
grep -q "WORKER_COUNT: 65 is above max 64 (out_of_range)" "$work/err"

# 2. The platform's values pass, and render gives the pod spec the
#    CronJob uses (k8s/cronjob.yaml).
"$docuconf" vet -contract contract.cue -values k8s/values.yaml -files k8s/files.yaml
"$docuconf" render -contract contract.cue -values k8s/values.yaml -files k8s/files.yaml >"$work/rendered.yaml"
diff -u k8s/rendered.yaml "$work/rendered.yaml"

# 3. The loader stores what the program reads.
cobc -x -o "$work/cfgtest" CFGTEST.cbl ORDCFG.cbl
env -i DATABASE_URL=postgres://orders:pw@db/orders WORKER_COUNT=7 \
	DOCUCONF_TERMINATION_LOG=- "$work/cfgtest"
echo "test-config: ok"
