# syntax=docker/dockerfile:1

FROM scratch

ARG SOURCE_REVISION
ARG TARGETARCH
ARG TYPERB_VERSION

LABEL org.opencontainers.image.description="The TypeRB compiler binary"
LABEL org.opencontainers.image.licenses="MIT"
LABEL org.opencontainers.image.revision="${SOURCE_REVISION}"
LABEL org.opencontainers.image.source="https://github.com/type-rb/type-rb"
LABEL org.opencontainers.image.title="TypeRB compiler"
LABEL org.opencontainers.image.url="https://type-rb.github.io/"
LABEL org.opencontainers.image.version="${TYPERB_VERSION}"

COPY --chmod=0755 dist/container/linux/${TARGETARCH}/trb /usr/local/bin/trb
COPY LICENSE /usr/share/licenses/type-rb/LICENSE

ENTRYPOINT ["/usr/local/bin/trb"]
CMD ["version"]
