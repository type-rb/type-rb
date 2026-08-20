# TypeRB in containers

TypeRB publishes the compiler binary as a small multi-platform OCI image:

```text
ghcr.io/type-rb/trb:<version>
```

The image supports Linux on `amd64` and `arm64`. It contains only the static
`trb` binary and its license. It intentionally does not select or bundle a Go,
Ruby, Node, or Bun toolchain.

## Try the compiler without installing it

For an existing TypeRB project, mount the project and run the compiler-only
image directly:

```sh
docker run --rm \
  --volume "$PWD:/workspace" \
  --workdir /workspace \
  ghcr.io/type-rb/trb:X.Y.Z check
```

The same pattern supports formatting and source generation. For example,
`fmt --check` verifies formatting, and this command transpiles one standalone
file to Go on standard output:

```sh
docker run --rm \
  --volume "$PWD:/workspace" \
  --workdir /workspace \
  ghcr.io/type-rb/trb:X.Y.Z \
  build --stdout --mode go hello.trb
```

These commands need no TypeRB installation on the host. Running generated
programs or installing native dependencies still requires the selected target
toolchain. Compose the compiler image with a Go, Ruby, Node, or Bun image as
shown below when the container must build or run the application.

## Verify an image

Run an exact release image to print its compiler version:

```sh
docker run --rm ghcr.io/type-rb/trb:X.Y.Z
```

An exact tag always identifies the corresponding TypeRB release. Projects
should select an exact version, and may also pin the multi-platform digest when
the complete container input must be byte-for-byte reproducible:

```dockerfile
FROM ghcr.io/type-rb/trb:X.Y.Z@sha256:<digest> AS typerb
```

The floating `<major>.<minor>` and `latest` tags are conveniences for local
experimentation. Do not use them for reproducible application builds.

## Add TypeRB to a target toolchain

Use the compiler image as an external build stage, then copy `trb` into the
native toolchain selected by the project.

### Go

```dockerfile
ARG TYPERB_VERSION=X.Y.Z
ARG GO_VERSION=1.26

FROM ghcr.io/type-rb/trb:${TYPERB_VERSION} AS typerb

FROM golang:${GO_VERSION}-bookworm AS build
COPY --from=typerb /usr/local/bin/trb /usr/local/bin/trb

WORKDIR /src
COPY . .
RUN trb install --frozen
RUN CGO_ENABLED=0 trb build --compile --outfile /out/app

FROM scratch AS runtime
COPY --from=build /out/app /app
ENTRYPOINT ["/app"]
```

The final stage needs neither TypeRB nor the Go toolchain. Replace `scratch`
with an appropriate minimal operating-system image when the application needs
CA certificates, time-zone data, native libraries, or files that are not
embedded in the executable. `trb build --compile` does not remove system
library requirements introduced by dependencies or embed application files.

### Ruby

```dockerfile
ARG TYPERB_VERSION=X.Y.Z
ARG RUBY_VERSION=4.0

FROM ghcr.io/type-rb/trb:${TYPERB_VERSION} AS typerb

FROM ruby:${RUBY_VERSION}-slim AS build
COPY --from=typerb /usr/local/bin/trb /usr/local/bin/trb
```

Ruby applications retain Ruby in their runtime stage, but do not need to copy
the TypeRB compiler into that stage after generating the application source.

### TypeScript

Choose the JavaScript runtime separately from the TypeScript output mode:

```dockerfile
ARG TYPERB_VERSION=X.Y.Z
ARG BUN_VERSION=1.3

FROM ghcr.io/type-rb/trb:${TYPERB_VERSION} AS typerb

FROM oven/bun:${BUN_VERSION} AS build
COPY --from=typerb /usr/local/bin/trb /usr/local/bin/trb
```

A browser application can copy its built assets into a static web-server
image. A Bun or Node server keeps its selected JavaScript runtime in the final
stage. TypeRB does not publish combined compiler-and-runtime images because
native runtime versions, operating systems, and application dependencies are
owned by each project.

## Image scope

The compiler image is an artifact carrier rather than a general-purpose
development environment. It has no shell, Git client, certificate store, or
target toolchain. The supported direct invocation is the version check shown
above; application checks, builds, package installation, and execution belong
in a project image that supplies the required native tools.

Every stable TypeRB release publishes the exact release tag, the matching
`<major>.<minor>` tag, and `latest`. The image is built from the same Linux
release binaries published on GitHub, and its build provenance is attached to
the image digest.
