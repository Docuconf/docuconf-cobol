#!/bin/sh
# Smoke test for the shop example: builds SHOP with cobc and runs it
# under docuconf exec as the web role, as the worker role, and as a
# worker with json logs but no LOG_SCHEMA_URL, which docuconf exec
# refuses to start. tests/shop_test.go runs the whole matrix.
#
#   DOCUCONF=/path/to/docuconf examples/shop/smoke.sh
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
cobc -x -o "$work/shop" SHOP.cbl

# shop ENV...: runs SHOP under docuconf exec with the base environment,
# ENV and an empty file root; sets out and code.
shop() {
	mkdir -p "$work/root"
	code=0
	out=$(env -i PATH="$PATH" DATABASE_URL=postgres://shop@db:5432/shop \
		DOCUCONF_FILE_ROOT="$work/root" DOCUCONF_TERMINATION_LOG=- "$@" \
		"$docuconf" exec -contract contract.json -- "$work/shop" 2>&1) || code=$?
	echo "$out"
}

echo "== web"
shop DOCUCONF_ROLE=web ALLOWED_ORIGINS=https://shop.example
[ "$code" = 0 ] || fail "web: exit code $code, want 0"
[ "$out" = "shop web: port 8080, origins https://shop.example" ] || fail "web: unexpected output"

echo "== worker"
shop DOCUCONF_ROLE=worker QUEUES=default,mail
[ "$code" = 0 ] || fail "worker: exit code $code, want 0"
[ "$out" = "shop worker: queues default,mail, webhooks off" ] || fail "worker: unexpected output"

echo "== worker, json logs without LOG_SCHEMA_URL"
shop DOCUCONF_ROLE=worker QUEUES=default LOG_FORMAT=json
[ "$code" = 1 ] || fail "json logs: exit code $code, want 1"
echo "$out" | grep -q '^  LOG_SCHEMA_URL: is required when LOG_FORMAT is "json" but not set (missing_required)$' ||
	fail "json logs: missing_required not reported"
echo "$out" | grep -q "^shop " && fail "json logs: shop ran"
echo "$out" | grep -q "postgres://" && fail "the secret was printed"
echo "smoke: ok"
