# Releasing

1. Make sure CI is green on `main`, including the conformance suite and the example's smoke test.
2. Update `Version` in `internal/gen/contract.go`, regenerate the example (`go run ./cmd/docuconf-cobol generate examples/orders/orders-config.cpy`) and the golden files (`go test ./internal/gen -update`), and commit.
3. Tag and push: `git tag v0.1.0 && git push origin v0.1.0`.
4. `release.yml` builds `docuconf-cobol` for Linux, macOS and Windows, writes `SHA256SUMS`, and publishes them on the GitHub release for the tag. It uses the workflow's `GITHUB_TOKEN`; no other secret is needed.
5. `go install github.com/docuconf/docuconf-cobol/cmd/docuconf-cobol@v0.1.0` works as soon as the tag is pushed.

Before the first release, replace the docuconf-go pseudo-version in `go.mod` with a tagged docuconf-go release that includes `ContractCUE` and file inputs in `LoadContract`.

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
