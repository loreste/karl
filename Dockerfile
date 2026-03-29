FROM golang:1.23.2-alpine AS builder

WORKDIR /app

# Install git for go mod download
RUN apk add --no-cache git

# Copy go.mod and go.sum
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application with optimizations
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o karl .

# Use a minimal alpine image for the final stage
FROM alpine:3.19

# Install necessary runtime dependencies
RUN apk --no-cache add ca-certificates tzdata

# Create Karl user and group
RUN addgroup -S karl && adduser -S -G karl karl

# Create directories
RUN mkdir -p /etc/karl /var/run/karl /var/log/karl /var/lib/karl/recordings && \
    chown -R karl:karl /etc/karl /var/run/karl /var/log/karl /var/lib/karl

# Copy the binary from the build stage
COPY --from=builder /app/karl /usr/local/bin/
RUN chmod +x /usr/local/bin/karl

# Copy config
COPY --from=builder /app/config/config.json /etc/karl/config.json
RUN chown karl:karl /etc/karl/config.json

# Set up the working directory
WORKDIR /app

# Switch to the karl user
USER karl

# Expose ports
# NG Protocol UDP
EXPOSE 22222/udp
# RTP/RTCP port range
EXPOSE 30000-40000/udp
# REST API
EXPOSE 8080/tcp
# Health check endpoint
EXPOSE 8086/tcp
# Prometheus metrics
EXPOSE 9091/tcp
# Legacy RTP port
EXPOSE 12000/udp

# Health check using the liveness endpoint
HEALTHCHECK --interval=30s --timeout=3s --start-period=15s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8086/live || exit 1

# Set environment variables with sensible defaults
ENV KARL_CONFIG_PATH=/etc/karl/config.json
ENV KARL_LOG_LEVEL=info
ENV KARL_RUN_DIR=/var/run/karl
ENV KARL_HEALTH_PORT=:8086
ENV KARL_METRICS_PORT=:9091
ENV KARL_API_PORT=:8080

# Run the application
ENTRYPOINT ["/usr/local/bin/karl"]
