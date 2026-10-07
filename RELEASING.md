# Releasing

1. Make sure CI is green on `main`, including the conformance suite and the example's smoke test.
2. Update `Version` in `internal/gen/contract.go`, regenerate the example (`go run ./cmd/docuconf-cobol generate examples/orders/orders-config.cpy`) and the golden files (`go test ./internal/gen -update`), and commit.
3. Tag and push: `git tag v0.1.0 && git push origin v0.1.0`.
4. `release.yml` builds `docuconf-cobol` for Linux, macOS and Windows, writes `SHA256SUMS`, and publishes them on the GitHub release for the tag. It uses the workflow's `GITHUB_TOKEN`; no other secret is needed.
5. `go install github.com/docuconf/docuconf-cobol/cmd/docuconf-cobol@v0.1.0` works as soon as the tag is pushed.

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
