FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN go mod tidy
RUN go test ./...
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/instafix ./cmd/instafix

FROM alpine:3.21
RUN apk add --no-cache ca-certificates \
  && adduser -D -H -u 10001 appuser \
  && mkdir -p /data \
  && chown appuser:appuser /data
USER appuser
COPY --from=build /out/instafix /usr/local/bin/instafix
EXPOSE 3000
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD wget -qO- http://127.0.0.1:3000/healthz || exit 1
ENTRYPOINT ["/usr/local/bin/instafix"]
