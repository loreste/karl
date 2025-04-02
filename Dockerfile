FROM golang:1.23.2-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o karl

# Use a minimal alpine image for the final stage
FROM alpine:3.19

# Install necessary runtime dependencies
RUN apk --no-cache add ca-certificates tzdata

# Create Karl user and group
RUN addgroup -S karl && adduser -S -G karl karl

# Create directories
RUN mkdir -p /etc/karl /var/run/karl /var/log/karl
RUN chown -R karl:karl /etc/karl /var/run/karl /var/log/karl

# Copy the binary from the build stage
COPY --from=builder /app/karl /usr/local/bin/
RUN chmod +x /usr/local/bin/karl

# Copy config
COPY --from=builder /app/config/config.json /etc/karl/config.json
RUN chown karl:karl /etc/karl/config.json

# Set up the working directory
WORKDIR /usr/local/bin

# Switch to the karl user
USER karl

# Expose ports
EXPOSE 12000/udp 12001/tcp 9091/tcp

# Set environment variables
ENV KARL_CONFIG_PATH=/etc/karl/config.json
ENV KARL_LOG_LEVEL=3
ENV KARL_RUN_DIR=/var/run/karl

# Run the application
CMD ["karl"]