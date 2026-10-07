# syntax=docker/dockerfile:1
#
# docuconf-cobol (the generator) as a static binary on distroless, published
# as ghcr.io/docuconf/docuconf-cobol by .github/workflows/release.yml
# (linux/amd64, linux/arm64 and linux/s390x). To use it in a build image:
#
#   COPY --from=ghcr.io/docuconf/docuconf-cobol:<version> /docuconf-cobol /usr/local/bin/docuconf-cobol

# Cross-compile on the build machine's own architecture; no emulation needed.
FROM --platform=$BUILDPLATFORM golang:1.25 AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /docuconf-cobol ./cmd/docuconf-cobol

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /docuconf-cobol /docuconf-cobol
ENTRYPOINT ["/docuconf-cobol"]
