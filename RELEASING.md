# Releasing

1. Make sure CI is green on `main`, including the conformance suite and the example's smoke test.
2. Update `Version` in `internal/gen/contract.go`, regenerate the example (`go run ./cmd/docuconf-cobol generate examples/orders/orders-config.cpy`) and the golden files (`go test ./internal/gen -update`), and commit.
3. Tag and push: `git tag v0.1.0 && git push origin v0.1.0`.
4. `release.yml` builds `docuconf-cobol` for Linux, macOS and Windows, writes `SHA256SUMS`, and publishes them on the GitHub release for the tag. It uses the workflow's `GITHUB_TOKEN`; no other secret is needed.
5. `go install github.com/docuconf/docuconf-cobol/cmd/docuconf-cobol@v0.1.0` works as soon as the tag is pushed.

Before the first release, replace the docuconf-go pseudo-version in `go.mod` with a tagged docuconf-go release that includes `ContractCUE` and file inputs in `LoadContract`.
