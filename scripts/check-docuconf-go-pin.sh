#!/usr/bin/env bash
# Fails unless go.mod's github.com/docuconf/docuconf-go version and
# .github/docuconf-go.ref name the same docuconf-go commit. CI runs it; the
# docuconf-go bump workflow updates both together.
set -euo pipefail
cd "$(dirname "$0")/.."

ref=$(tr -d '[:space:]' < .github/docuconf-go.ref)
ver=$(go mod edit -json | jq -r '.Require[] | select(.Path == "github.com/docuconf/docuconf-go") | .Version')
if [ -z "$ver" ]; then
  echo "::error file=go.mod::go.mod does not require github.com/docuconf/docuconf-go" >&2
  exit 1
fi

if [[ $ver =~ ^v[0-9]+\.[0-9]+\.[0-9]+-([0-9a-z.]+\.)?[0-9]{14}-([0-9a-f]{12})$ ]]; then
  # A pseudo-version ends in the commit's first 12 hex digits.
  commit=${BASH_REMATCH[2]}
  match=$([ "${ref:0:12}" = "$commit" ] && echo yes || echo no)
else
  # A release tag: resolve it to its commit.
  commit=$(git ls-remote https://github.com/Docuconf/docuconf-go "refs/tags/$ver^{}" "refs/tags/$ver" | head -1 | cut -f1)
  match=$([ -n "$commit" ] && [ "$ref" = "$commit" ] && echo yes || echo no)
fi

if [ "$match" != yes ]; then
  echo "::error file=go.mod::go.mod pins docuconf-go $ver (${commit:-unresolved}) but .github/docuconf-go.ref is $ref; run the docuconf-go bump workflow or update both" >&2
  exit 1
fi
echo "go.mod ($ver) and .github/docuconf-go.ref ($ref) agree"
