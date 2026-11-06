FROM golang:1.26-alpine AS builder

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

WORKDIR /app

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildDate=${BUILD_DATE}" \
    -o karl .

# --- Runtime ---
FROM alpine:3.21

RUN apk --no-cache add ca-certificates tzdata

RUN addgroup -S karl && adduser -S -G karl karl

RUN mkdir -p /etc/karl /var/run/karl /var/log/karl /var/lib/karl/recordings && \
    chown -R karl:karl /etc/karl /var/run/karl /var/log/karl /var/lib/karl

COPY --from=builder /app/karl /usr/local/bin/
COPY --from=builder /app/config/config.json /etc/karl/config.json
RUN chown karl:karl /etc/karl/config.json

WORKDIR /app
USER karl

# NG Protocol
EXPOSE 22222/udp
# RTP/RTCP port range
EXPOSE 30000-40000/udp
# REST API
EXPOSE 8080/tcp
# Health checks
EXPOSE 8086/tcp
# Prometheus metrics
EXPOSE 9091/tcp

HEALTHCHECK --interval=30s --timeout=3s --start-period=15s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8086/live || exit 1

ENV KARL_CONFIG_PATH=/etc/karl/config.json
ENV KARL_LOG_LEVEL=info
ENV KARL_RUN_DIR=/var/run/karl
ENV KARL_HEALTH_PORT=:8086
ENV KARL_METRICS_PORT=:9091
ENV KARL_API_PORT=:8080

ENTRYPOINT ["/usr/local/bin/karl"]
