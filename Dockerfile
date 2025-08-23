# Multi-stage build for HyperCache
# Stage 1: Build stage
FROM golang:1.23.2-alpine AS builder

# Install necessary packages
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod files first (for better caching)
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary with optimizations
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o hypercache \
    cmd/hypercache/main.go

# Stage 2: Final minimal image
FROM alpine:3.18

# Install necessary runtime packages
RUN apk add --no-cache ca-certificates tzdata wget net-tools

# Create non-root user and necessary directories
RUN addgroup -g 1000 hypercache && \
    adduser -D -u 1000 -G hypercache hypercache && \
    mkdir -p /data /app/logs /config && \
    chown -R hypercache:hypercache /data /app /config && \
    chmod 755 /data /app /config /app/logs

# Copy the binary
COPY --from=builder /app/hypercache /hypercache
RUN chmod +x /hypercache

# Set working directory
WORKDIR /app

# Switch to non-root user
USER hypercache

# Expose ports
# RESP Protocol
EXPOSE 8080
# HTTP API
EXPOSE 9080
# Gossip Protocol
EXPOSE 7946

# Health check using the proper health endpoint
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:9080/health || exit 1

# Default command
ENTRYPOINT ["/hypercache"]
CMD ["--config", "/config/hypercache.yaml", "--protocol", "resp"]
