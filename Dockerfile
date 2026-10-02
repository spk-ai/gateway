# syntax=docker/dockerfile:1.7@sha256:a57df69d0ea827fb7266491f2813635de6f17269be881f696fbfdf2d83dda33e
ARG BUF_VERSION=1.66.1
# Release sha256.txt entries for buf-Linux-x86_64 and buf-Linux-aarch64.
ARG BUF_SHA256_X86_64=ef835cb38ed973849f68e0e6a88153cb2168507e09eecbec43a2ada2f6a698be
ARG BUF_SHA256_AARCH64=76610016cad907feb304f3827652ef31b54c64f297a3a0a15dbb975a810d1445

# Images are pinned to the index digests of their tags (golang:1.25-bookworm
# was Go 1.25.14 on 2026-10-02). Update each tag and digest together.

# Stage 1: Download buf binary
FROM --platform=$BUILDPLATFORM golang:1.25-bookworm@sha256:3b4a11519ad929d1e1d261a12cff056f0c85b735253d7d861346b9c6f8b36437 AS buf
ARG BUF_VERSION BUF_SHA256_X86_64 BUF_SHA256_AARCH64
RUN set -eu; \
    arch="$(uname -m)"; \
    case "${arch}" in \
      x86_64) sha256="${BUF_SHA256_X86_64}" ;; \
      aarch64) sha256="${BUF_SHA256_AARCH64}" ;; \
      *) echo "no pinned buf checksum for ${arch}" >&2; exit 1 ;; \
    esac; \
    curl -fsSL \
      "https://github.com/bufbuild/buf/releases/download/v${BUF_VERSION}/buf-Linux-${arch}" \
      -o /usr/local/bin/buf; \
    echo "${sha256}  /usr/local/bin/buf" | sha256sum -c -; \
    chmod +x /usr/local/bin/buf

# Stage 2: Generate + compile
FROM --platform=$BUILDPLATFORM golang:1.25-bookworm@sha256:3b4a11519ad929d1e1d261a12cff056f0c85b735253d7d861346b9c6f8b36437 AS build
WORKDIR /src
ENV CGO_ENABLED=0 GO111MODULE=on

COPY --from=buf /usr/local/bin/buf /usr/local/bin/buf

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/root/go/pkg/mod \
    go mod download && go mod verify

COPY buf.gen.yaml buf.yaml ./

# buf.gen.yaml pins the reviewed spk-ai/api revision and generated packages.
RUN buf generate --include-imports

COPY . .

ARG TARGETOS TARGETARCH
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/root/go/pkg/mod \
    GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
      -trimpath -ldflags="-s -w" -o /out/gateway ./cmd/gateway

# Stage 3: Runtime
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
WORKDIR /app
LABEL org.opencontainers.image.source="https://github.com/agynio/gateway"
COPY --from=build /out/gateway ./gateway
EXPOSE 8080
ENTRYPOINT ["/app/gateway"]
