# ORDERS-BATCH, checked at boot by docuconf exec.
# Build from this directory: docker build -t orders-batch .

# The docuconf CLI. Pinned to the docuconf-go commit go.mod pins (exec and
# the length limits) until a release includes it; then use @latest or a
# version tag.
FROM golang:1.25 AS docuconf
ARG DOCUCONF_VERSION=v0.0.0-20261007225926-e9554f671c32
RUN CGO_ENABLED=0 go install github.com/docuconf/docuconf-go/cmd/docuconf@${DOCUCONF_VERSION}

# The COBOL program and its generated loader, compiled with GnuCOBOL.
FROM debian:bookworm-slim AS build
RUN apt-get update \
 && apt-get install -y --no-install-recommends gnucobol3 \
 && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY orders-config.cpy ORDCFG.cbl ORDERS-BATCH.cbl ./
RUN mkdir /out && cobc -x -o /out/orders-batch ORDERS-BATCH.cbl ORDCFG.cbl

FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends libcob4 \
 && rm -rf /var/lib/apt/lists/*
COPY --from=docuconf /go/bin/docuconf /usr/local/bin/docuconf
COPY --from=build /out/orders-batch /app/orders-batch
COPY contract.cue /etc/docuconf/contract.cue
# A sample input; in the cluster the platform mounts the real file here.
COPY orders.txt /data/orders.txt
USER 65532:65532
ENTRYPOINT ["docuconf", "exec", "--contract", "/etc/docuconf/contract.cue", "--", "/app/orders-batch"]
