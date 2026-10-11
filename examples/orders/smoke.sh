#!/bin/sh
# Smoke test for the orders example: builds ORDERS-BATCH with cobc and
# runs it under docuconf exec, first with a valid environment, then with
# PORT=0 and no DATABASE_URL. It then checks the webhook key set: PAYHOOK
# accepts a webhook signed with either key of a set that is mid-rotation,
# and an empty key stops the job at boot. With Docker available it also
# builds the image and runs the first two checks in a container. Needs
# openssl and libcrypto (libssl-dev) for PAYHOOK.
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
cobc -x -fstatic-call -o "$work/payhook" PAYHOOK.cbl ORDCFG.cbl -lcrypto

# Two webhook keys: the old one and, mid-rotation, the new one.
old_key=old-webhook-key-0123456789abcdef0123
new_key=new-webhook-key-0123456789abcdef0123

# A valid environment: the job runs and prints its summary.
echo "== valid environment"
if ! out=$(env -i PATH="$PATH" \
	DATABASE_URL=postgres://orders:s3cret@db:5432/orders \
	ALLOWED_ORIGINS=https://shop.example.com,http://localhost:3000 \
	WEBHOOK_KEYS="$old_key,$new_key" \
	ORDERS_FILE="$here/orders.txt" \
	DOCUCONF_TERMINATION_LOG="$work/termination-log" \
	"$docuconf" exec -contract contract.cue -- "$work/orders-batch" 2>&1); then
	echo "$out"
	fail "the job failed with a valid environment"
fi
echo "$out"
echo "$out" | grep -q "accepted total  67.49" || fail "unexpected summary"
echo "$out" | grep -q "s3cret" && fail "the secret was printed"
echo "$out" | grep -q "webhook-key" && fail "a webhook key was printed"
echo "$out" | grep -q "WEBHOOK_KEYS    \*\*\*" || fail "WEBHOOK_KEYS not shown redacted"

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

# Mid-rotation, PAYHOOK accepts a webhook signed with either key, and
# rejects one signed with any other key, or unsigned (exit code 2).
echo "== webhooks"
body='{"order":"42","status":"paid"}'
payhook() { # signature
	printf '%s\n' "$body" | env -i PATH="$PATH" \
		DATABASE_URL=postgres://orders:s3cret@db:5432/orders \
		WEBHOOK_KEYS="$old_key,$new_key" ORDERS_FILE="$here/orders.txt" \
		DOCUCONF_TERMINATION_LOG=- \
		"$docuconf" exec -contract contract.cue -- "$work/payhook" "$1" >/dev/null 2>&1
}
code=0; payhook 00 || code=$?
[ "$code" = 2 ] || fail "an unsigned webhook got exit code $code, want 2"
for key in "$old_key" "$new_key" other-webhook-key-0123456789abcdef; do
	sig=$(printf '%s' "$body" | openssl dgst -sha256 -hmac "$key" | sed 's/.*= //')
	want=0
	case $key in other*) want=2 ;; esac
	code=0; payhook "$sig" || code=$?
	[ "$code" = "$want" ] || fail "webhook signed with the ${key%%-*} key: exit code $code, want $want"
done
echo "old and new key accepted, other key rejected"

# An empty second key (a trailing comma): docuconf exec stops the job at
# boot, without printing the key.
echo "== empty webhook key"
if out=$(env -i PATH="$PATH" \
	DATABASE_URL=postgres://orders:s3cret@db:5432/orders \
	WEBHOOK_KEYS="$old_key," ORDERS_FILE="$here/orders.txt" \
	DOCUCONF_TERMINATION_LOG=- \
	"$docuconf" exec -contract contract.cue -- "$work/orders-batch" 2>&1); then
	echo "$out"
	fail "the job started with an empty webhook key"
fi
echo "$out"
echo "$out" | grep -q "WEBHOOK_KEYS: .*(out_of_range)" || fail "out_of_range not reported for WEBHOOK_KEYS"
echo "$out" | grep -q "webhook-key" && fail "a webhook key was printed"
echo "$out" | grep -q "orders-batch configuration" && fail "the job ran"

# The loader alone stops the job on the same empty key: a key set's
# empty key is out of range whatever its bounds.
echo "== empty webhook key, no docuconf exec"
if out=$(env -i PATH="$PATH" \
	DATABASE_URL=postgres://orders:s3cret@db:5432/orders \
	WEBHOOK_KEYS="$old_key," ORDERS_FILE="$here/orders.txt" \
	DOCUCONF_TERMINATION_LOG=- \
	"$work/orders-batch" 2>&1); then
	echo "$out"
	fail "the job started with an empty webhook key"
fi
echo "$out"
echo "$out" | grep -q "^  WEBHOOK_KEYS: key 2 is empty (out_of_range)$" || fail "the loader did not report the empty key"
echo "$out" | grep -q "webhook-key" && fail "a webhook key was printed"

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
