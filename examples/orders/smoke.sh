#!/bin/sh
# Smoke test for the orders example: builds ORDERS-BATCH with cobc and
# runs it under docuconf exec, first with a valid environment, then with
# PORT=0 and no DATABASE_URL. With Docker available it also builds the
# image and runs the same two checks in a container.
#
#   DOCUCONF=/path/to/docuconf examples/orders/smoke.sh
set -eu

here=$(cd "$(dirname "$0")" && pwd)
docuconf=${DOCUCONF:-docuconf}
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

fail() {
	echo "smoke: FAIL: $*" >&2
	exit 1
}

cd "$here"
cobc -x -o "$work/orders-batch" ORDERS-BATCH.cbl ORDCFG.cbl

# A valid environment: the job runs and prints its summary.
echo "== valid environment"
if ! out=$(env -i PATH="$PATH" \
	DATABASE_URL=postgres://orders:s3cret@db:5432/orders \
	ALLOWED_ORIGINS=https://shop.example.com,http://localhost:3000 \
	ORDERS_FILE="$here/orders.txt" \
	DOCUCONF_TERMINATION_LOG="$work/termination-log" \
	"$docuconf" exec -contract contract.cue -- "$work/orders-batch" 2>&1); then
	echo "$out"
	fail "the job failed with a valid environment"
fi
echo "$out"
echo "$out" | grep -q "accepted total  67.49" || fail "unexpected summary"
echo "$out" | grep -q "s3cret" && fail "the secret was printed"

# PORT=0 and no DATABASE_URL: docuconf exec refuses to start the job.
echo "== PORT=0, no DATABASE_URL"
if out=$(env -i PATH="$PATH" PORT=0 \
	ORDERS_FILE="$here/orders.txt" \
	DOCUCONF_TERMINATION_LOG="$work/termination-log" \
	"$docuconf" exec -contract contract.cue -- "$work/orders-batch" 2>&1); then
	echo "$out"
	fail "the job started with an invalid environment"
fi
echo "$out"
echo "$out" | grep -q "DATABASE_URL: is required but not set (missing_required)" || fail "missing_required not reported"
echo "$out" | grep -q "PORT: 0 is below min 1 (out_of_range)" || fail "out_of_range not reported"
echo "$out" | grep -q "orders-batch configuration" && fail "the job ran"
grep -q missing_required "$work/termination-log" || fail "termination log not written"

# Without docuconf exec the loader still enforces the ranges, and writes
# the termination log.
echo "== PORT=0, no docuconf exec"
if out=$(env -i PATH="$PATH" PORT=0 \
	DATABASE_URL=postgres://orders:s3cret@db:5432/orders \
	ORDERS_FILE="$here/orders.txt" \
	DOCUCONF_TERMINATION_LOG="$work/termination-log-loader" \
	"$work/orders-batch" 2>&1); then
	echo "$out"
	fail "the job ran with PORT=0"
fi
echo "$out"
echo "$out" | grep -q "^docuconf: 1 configuration problem:$" || fail "no header"
echo "$out" | grep -q "^  PORT: is below min 1 (out_of_range)$" || fail "PORT=0 not reported by the loader"
grep -q "PORT: is below min 1" "$work/termination-log-loader" || fail "the loader wrote no termination log"

# -env-file values reach the program, and exec exports the defaults.
echo "== -env-file"
printf 'DATABASE_URL=postgres://orders:s3cret@db:5432/orders\nORDERS_FILE=%s\nWORKER_COUNT=7\n' "$here/orders.txt" >"$work/.env"
out=$(env -i PATH="$PATH" "$docuconf" exec -contract contract.cue -env-file "$work/.env" -- "$work/orders-batch" 2>&1) ||
	{ echo "$out"; fail "the job failed with an -env-file"; }
echo "$out" | grep -q "WORKER_COUNT    7" || { echo "$out"; fail "the -env-file value did not reach the job"; }

if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
	echo "== docker"
	image=docuconf-cobol-orders:smoke
	docker build -q -t "$image" . >/dev/null
	out=$(docker run --rm -v "$here/orders.txt:/data/orders.txt:ro" \
		-e DATABASE_URL=postgres://orders:s3cret@db:5432/orders \
		-e ALLOWED_ORIGINS=https://shop.example.com,http://localhost:3000 "$image" 2>&1) ||
		{ echo "$out"; fail "the container failed with a valid environment"; }
	echo "$out" | grep -q "accepted total  67.49" || { echo "$out"; fail "unexpected summary in the container"; }
	if out=$(docker run --rm -e PORT=0 "$image" 2>&1); then
		echo "$out"
		fail "the container started the job with an invalid environment"
	fi
	echo "$out" | grep -q missing_required || { echo "$out"; fail "missing_required not reported in the container"; }
	echo "$out" | grep -q out_of_range || { echo "$out"; fail "out_of_range not reported in the container"; }
	echo "docker: ok"
else
	echo "== docker: not available, image not built"
fi
echo "smoke: ok"
