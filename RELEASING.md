# Releasing

Releases are automated with [release-please](https://github.com/googleapis/release-please); see
[CONTRIBUTING.md](CONTRIBUTING.md#how-releases-happen) for the commit conventions it reads.

## Each release

1. Merge the open release PR (`chore(main): release X.Y.Z`) once CI is green on it, including the conformance
   suite and the example's smoke test. It already updates `Version` in `internal/gen/contract.go` and
   `CHANGELOG.md`. The golden files and the example do not need regenerating: their comparisons ignore
   `metadata.generator.version`.
2. release-please tags the merge commit `vX.Y.Z` and creates the GitHub release with the changelog entries.
3. `release.yml` builds `docuconf-cobol` for Linux, macOS and Windows, writes `SHA256SUMS`, and attaches them to
   that release. It uses the workflow's `GITHUB_TOKEN`; no other secret is needed.
4. `go install github.com/docuconf/docuconf-cobol/cmd/docuconf-cobol@vX.Y.Z` works as soon as the tag exists.

If the release PR was created with `GITHUB_TOKEN` (no release GitHub App configured), the tag does not trigger
`release.yml` by itself, so `.github/workflows/release-please.yml` starts it with `gh workflow run`. To redo a
release by hand: `gh workflow run release.yml --ref vX.Y.Z`.

Before the first release, replace the docuconf-go pseudo-version in `go.mod` with a tagged docuconf-go release that includes `ContractCUE` and file inputs in `LoadContract`.
