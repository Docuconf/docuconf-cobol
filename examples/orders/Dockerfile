# ORDERS-BATCH, checked at boot by docuconf exec.
# Build from this directory: docker build -t orders-batch .

# The docuconf CLI, built from source: docuconf-go publishes no release
# binary or image yet. Pinned to the docuconf-go commit go.mod pins (the
# keySet type, strict parsing, file inputs). At that commit the CLI's own
# module still requires an older SDK, so it is built in a Go workspace
# with the SDK from the same commit; when a release exists, copy its
# binary instead and drop this stage.
FROM golang:1.25 AS docuconf
ARG DOCUCONF_VERSION=v0.0.0-20261009192536-caca80452b05
RUN git clone -q https://github.com/docuconf/docuconf-go /docuconf-go \
 && cd /docuconf-go \
 && git checkout -q "${DOCUCONF_VERSION##*-}" \
 && go work init . ./cmd/docuconf \
 && cd cmd/docuconf \
 && CGO_ENABLED=0 go build -o /go/bin/docuconf .

# The COBOL program and its generated loader, compiled with GnuCOBOL.
FROM debian:bookworm-slim AS build
RUN apt-get update \
 && apt-get install -y --no-install-recommends gnucobol3 \
 && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY orders-config.cpy ORDCFG.cbl ORDERS-BATCH.cbl ./
RUN mkdir /out && cobc -x -o /out/orders-batch ORDERS-BATCH.cbl ORDCFG.cbl

# libcob4 is the GnuCOBOL 3.1.2 runtime in Debian bookworm (3.1.2-5).
# Debian renamed it libcob4t64 in 3.1.2-5.1, after bookworm, and Ubuntu
# 24.04 ships that name: change it if you change the base image.
FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends libcob4 \
 && rm -rf /var/lib/apt/lists/*
COPY --from=docuconf /go/bin/docuconf /usr/local/bin/docuconf
COPY --from=build /out/orders-batch /app/orders-batch
COPY contract.cue /etc/docuconf/contract.cue
# No input is baked in: the platform mounts the orders file at
# /data/orders.txt (or sets ORDERS_FILE).
USER 65532:65532
ENTRYPOINT ["docuconf", "exec", "-contract", "/etc/docuconf/contract.cue", "--", "/app/orders-batch"]
