# ORDERS-BATCH, checked at boot by docuconf exec.
# Build from this directory: docker build -t orders-batch .

# The docuconf CLI, built from source: docuconf-go publishes no release
# binary or image yet. Pinned to the docuconf-go commit go.mod pins (exec
# passing -env-file values and contract defaults to the program, and the
# length limits); when a release exists, copy its binary instead and drop
# this stage.
FROM golang:1.25 AS docuconf
ARG DOCUCONF_VERSION=v0.0.0-20261008234855-9620369f34b0
RUN CGO_ENABLED=0 go install github.com/docuconf/docuconf-go/cmd/docuconf@${DOCUCONF_VERSION}

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
