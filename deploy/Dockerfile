# syntax=docker/dockerfile:1.7@sha256:a57df69d0ea827fb7266491f2813635de6f17269be881f696fbfdf2d83dda33e

FROM --platform=$BUILDPLATFORM golang:1.26.5-alpine3.24@sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2 AS build-base
WORKDIR /src
ENV CGO_ENABLED=0 \
    GOTOOLCHAIN=local

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
  go mod download && go mod verify

COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY web/ ./web/

FROM build-base AS test
RUN --mount=type=cache,target=/go/pkg/mod \
  --mount=type=cache,target=/root/.cache/go-build \
  go test -mod=readonly ./...

FROM build-base AS build
ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod \
  --mount=type=cache,target=/root/.cache/go-build \
  GOOS=$TARGETOS GOARCH=$TARGETARCH go build -mod=readonly -buildvcs=false \
  -trimpath -ldflags="-s -w" -o /out/Loonstagram ./cmd/Loonstagram

FROM alpine:3.24.1@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b AS runtime
RUN addgroup -S -g 10001 appuser \
  && adduser -S -D -H -u 10001 -G appuser appuser \
  && mkdir -p /data \
  && chown appuser:appuser /data
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/Loonstagram /usr/local/bin/Loonstagram
USER 10001:10001
EXPOSE 3000
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD ["wget", "-q", "-O", "/dev/null", "http://127.0.0.1:3000/healthz"]
ENTRYPOINT ["/usr/local/bin/Loonstagram"]
