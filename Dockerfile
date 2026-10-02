# syntax=docker/dockerfile:1.7
ARG GO_VERSION=1.25
ARG BUF_VERSION=1.66.1

# Stage 1: Download buf binary
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-bookworm AS buf
ARG BUF_VERSION
RUN curl -sSL \
      "https://github.com/bufbuild/buf/releases/download/v${BUF_VERSION}/buf-$(uname -s)-$(uname -m)" \
      -o /usr/local/bin/buf && \
    chmod +x /usr/local/bin/buf

# Stage 2: Generate + compile
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-bookworm AS build
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
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
LABEL org.opencontainers.image.source="https://github.com/agynio/gateway"
COPY --from=build /out/gateway ./gateway
EXPOSE 8080
ENTRYPOINT ["/app/gateway"]
