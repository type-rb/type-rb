ARG TYPERB_VERSION=latest
ARG GO_VERSION=1.27

FROM ghcr.io/type-rb/trb:${TYPERB_VERSION} AS typerb

FROM golang:${GO_VERSION}-bookworm
COPY --from=typerb /usr/local/bin/trb /usr/local/bin/trb

WORKDIR /workspace
