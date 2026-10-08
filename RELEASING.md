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

## GitHub Packages and Releases

Everything a release publishes goes to GitHub, from the same `v*` tag:

- **GitHub release** (`release` job): `docuconf-cobol_<tag>_<os>_<arch>` binaries for linux (amd64, arm64, s390x),
  darwin (amd64, arm64) and windows (amd64), static (`CGO_ENABLED=0`), plus `SHA256SUMS`. A re-run reuses the
  release and replaces the assets.
- **Container image** (`image` job): `ghcr.io/docuconf/docuconf-cobol:<version>` and `:latest` (not for
  pre-releases), for linux/amd64, linux/arm64 and linux/s390x, built with buildx from the root `Dockerfile`: the
  generator as a static binary at `/docuconf-cobol` on `gcr.io/distroless/static-debian12:nonroot`.

Both jobs use only the workflow's own `GITHUB_TOKEN` (`contents: write` for the release, `packages: write` for the
image): no accounts and no secrets. The only requirement is that the `Docuconf` organization lets `GITHUB_TOKEN` write
packages, which it does unless package creation has been restricted under Organization settings > Packages.

After the first image push, open the `docuconf-cobol` package under the organization's Packages tab and set its
visibility to public (images on ghcr.io start out private); later versions keep the setting.

### Installing

No token is needed for public releases and images.

- `go install github.com/docuconf/docuconf-cobol/cmd/docuconf-cobol@v0.1.0`.
- A binary from the release: download it with `SHA256SUMS`, then `sha256sum --ignore-missing -c SHA256SUMS`.
- In a build image:
  ```dockerfile
  COPY --from=ghcr.io/docuconf/docuconf-cobol:0.1.0 /docuconf-cobol /usr/local/bin/docuconf-cobol
  ```
  or run it on the copybook directly:
  `docker run --rm --user "$(id -u):$(id -g)" -v "$PWD:/work" -w /work ghcr.io/docuconf/docuconf-cobol:0.1.0 generate orders-config.cpy`.

## docuconf-go version

The spec, the CUE meta-schema and the shared conformance suite live in
[docuconf-go](https://github.com/Docuconf/docuconf-go). `.github/docuconf-go.ref` holds the full docuconf-go commit SHA
this SDK is tested against. `go.mod` pins the same commit by pseudo-version, and CI fails when the two differ
(`scripts/check-docuconf-go-pin.sh`); the bump pull request updates both, along with `go.sum` and the pseudo-version
in the README and `examples/orders/Dockerfile`.

- **Push and pull request CI** check out docuconf-go at that commit, so a change in docuconf-go never breaks this
  repository's CI by surprise.
- **Bump pull requests.** `.github/workflows/docuconf-go-bump.yml` opens (or updates) a
  `build(deps): bump docuconf-go to <sha>` pull request on the `docuconf-go-bump` branch whenever docuconf-go's `main`
  moves: on a `docuconf-go-updated` dispatch from docuconf-go, and daily as a catch-up. CI on that pull request is the
  compatibility check; merge it when it is green. Run the workflow by hand (optionally with a `sha`) to pin a
  specific commit.
- **Nightly.** CI also runs every night against docuconf-go `main`, and can be started by hand with a
  `docuconf_go_ref` input to try any branch or commit.
- **`scripts/conformance.sh`** runs just the shared conformance suite and the `cue vet` tests against a docuconf-go
  checkout: `DOCUCONF_GO_DIR=../docuconf-go scripts/conformance.sh`. It builds the docuconf CLI and this module
  against that checkout through a temporary `go.work`, leaving `go.mod` alone. docuconf-go runs it on every pull request that
  touches the spec, so a breaking spec change shows up there before it merges.

Without the release GitHub App (`RELEASE_APP_ID` and `RELEASE_APP_PRIVATE_KEY`), the bump pull request is created with
`GITHUB_TOKEN`, which starts no workflows, so the bump workflow starts CI on the branch itself. That needs
**Settings → Actions → General → Allow GitHub Actions to create and approve pull requests**.
