# syntax=docker/dockerfile:1.6

# ---- build stage ----
FROM golang:1.22-alpine AS builder

# CGO is required by mattn/go-sqlite3 / glebarez/sqlite for SQLite support.
ENV CGO_ENABLED=1
WORKDIR /src

# Cache go.mod/go.sum first for better layer reuse.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

# Build a static-ish binary stripped of debug info.
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w" -o /out/opc-api ./cmd/server

# ---- runtime stage ----
# Alpine is small (~5MB) and ships with ca-certificates; sqlite CLI is handy
# for one-off inspection but not required by the binary itself. We install both
# because the deploy/backup scripts rely on `sqlite3` being on the host, not
# the container, but we keep `sqlite-libs` available for the CGO-built binary.
FROM alpine:3.20

# Install runtime dependencies:
#   - ca-certificates: HTTPS calls to the Anthropic API
#   - sqlite-libs:     shared libs for CGO-built SQLite
#   - wget:            used by the docker-compose healthcheck
#   - tzdata:          correct timestamps in container logs
#   - curl:            diagnostic / healthcheck fallback
RUN apk add --no-cache \
    ca-certificates \
    sqlite-libs \
    wget \
    tzdata \
    curl

# Run as a non-root user for least privilege.
RUN addgroup -S -g 10001 opc && \
    adduser  -S -u 10001 -G opc opc

WORKDIR /app

# Copy the binary from the builder.
COPY --from=builder /out/opc-api /app/opc-api

# Data + backup directories (mounted as volumes in compose).
RUN mkdir -p /data /backups && \
    chown -R opc:opc /app /data /backups

USER opc

EXPOSE 8080

# Use exec form so signals (SIGTERM) reach the Go process for graceful shutdown.
ENTRYPOINT ["/app/opc-api"]
