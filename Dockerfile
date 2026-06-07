FROM golang:1.26-alpine AS builder

RUN apk add --no-cache gcc musl-dev vips-dev pkgconfig

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o /app/server ./cmd/server

FROM alpine:3.23


RUN apk add --no-cache \
    ffmpeg \
    vips \
    ca-certificates \
    tzdata \
    su-exec \
    && rm -rf /var/cache/apk/*

COPY --from=builder /app/server /usr/local/bin/server
COPY --from=builder /app/migrations /migrations

RUN addgroup -S app && adduser -S app -G app
RUN mkdir -p /data && chown app:app /data

ENV COMPRESSOR_SERVER_HOST=0.0.0.0
ENV COMPRESSOR_SERVER_PORT=8080
ENV COMPRESSOR_STORAGE_BASE_PATH=/data

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:8080/api/v1/health || exit 1

# Railway mounts the volume root-owned, so start as root only to fix /data's
# ownership, then drop to the unprivileged "app" user to run the server.
ENTRYPOINT ["/bin/sh", "-c", "chown -R app:app /data && exec su-exec app server"]
