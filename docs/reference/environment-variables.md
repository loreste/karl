# Environment Variables Reference

Complete list of environment variables supported by Karl Media Server.

## Table of Contents

- [Core Configuration](#core-configuration)
- [Network Configuration](#network-configuration)
- [Session Configuration](#session-configuration)
- [Recording Configuration](#recording-configuration)
- [Database Configuration](#database-configuration)
- [Monitoring Configuration](#monitoring-configuration)
- [WebRTC Configuration](#webrtc-configuration)
- [Advanced Configuration](#advanced-configuration)

---

## Core Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `KARL_CONFIG_PATH` | `./config/config.json` | Path to JSON configuration file |
| `KARL_LOG_LEVEL` | `info` | Logging level: `debug`, `info`, `warn`, `error` |
| `KARL_RUN_DIR` | `./run/karl` | Runtime directory for sockets and PID files |
| `KARL_ENVIRONMENT` | `production` | Environment name: `production`, `staging`, `development` |

### Examples

```bash
# Use custom config file
export KARL_CONFIG_PATH=/etc/karl/config.json

# Enable debug logging
export KARL_LOG_LEVEL=debug

# Set runtime directory
export KARL_RUN_DIR=/var/run/karl
```

---

## Network Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `KARL_NG_PORT` | `22222` | UDP port for NG protocol |
| `KARL_API_PORT` | `:8080` | TCP port for REST API |
| `KARL_HEALTH_PORT` | `:8086` | TCP port for health endpoints |
| `KARL_METRICS_PORT` | `:9091` | TCP port for Prometheus metrics |
| `KARL_MEDIA_IP` | `auto` | IP address for media (SDP). Use `auto` for auto-detection |
| `KARL_PUBLIC_IP` | (auto) | Public IP for NAT scenarios |

### Examples

```bash
# Set specific ports
export KARL_NG_PORT=22223
export KARL_API_PORT=:8081
export KARL_HEALTH_PORT=:8087
export KARL_METRICS_PORT=:9092

# Set media IP explicitly
export KARL_MEDIA_IP=192.168.1.100

# Set public IP for NAT
export KARL_PUBLIC_IP=203.0.113.50
```

---

## Session Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `KARL_RTP_MIN_PORT` | `30000` | Minimum port for RTP media |
| `KARL_RTP_MAX_PORT` | `40000` | Maximum port for RTP media |
| `KARL_MAX_SESSIONS` | `10000` | Maximum concurrent sessions |
| `KARL_SESSION_TTL` | `3600` | Session timeout in seconds |
| `KARL_CLEANUP_INTERVAL` | `60` | Interval for cleaning stale sessions (seconds) |

### Examples

```bash
# Limit RTP port range
export KARL_RTP_MIN_PORT=30000
export KARL_RTP_MAX_PORT=35000

# Increase session capacity
export KARL_MAX_SESSIONS=20000

# Shorter session timeout
export KARL_SESSION_TTL=1800
```

### Port Range Calculation

Each session requires 4 ports (RTP + RTCP × 2 directions):
```
Max sessions = (MAX_PORT - MIN_PORT + 1) / 4
```

Default: (40000 - 30000 + 1) / 4 = 2,500 sessions

---

## Recording Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `KARL_RECORDING_ENABLED` | `false` | Enable/disable call recording |
| `KARL_RECORDING_PATH` | `/var/lib/karl/recordings` | Directory for recording files |
| `KARL_RECORDING_FORMAT` | `wav` | Output format: `wav`, `pcm` |
| `KARL_RECORDING_MODE` | `stereo` | Recording mode: `mixed`, `stereo`, `separate` |
| `KARL_RECORDING_SAMPLE_RATE` | `8000` | Sample rate in Hz |
| `KARL_RECORDING_RETENTION_DAYS` | `30` | Days to keep recordings |

### Examples

```bash
# Enable recording
export KARL_RECORDING_ENABLED=true
export KARL_RECORDING_PATH=/mnt/recordings

# Configure stereo recording
export KARL_RECORDING_MODE=stereo
export KARL_RECORDING_SAMPLE_RATE=16000

# Set retention to 7 days
export KARL_RECORDING_RETENTION_DAYS=7
```

---

## Database Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `KARL_MYSQL_DSN` | (empty) | MySQL connection string |
| `KARL_REDIS_ENABLED` | `false` | Enable Redis for session sharing |
| `KARL_REDIS_ADDR` | `localhost:6379` | Redis server address |
| `KARL_REDIS_PASSWORD` | (empty) | Redis password |
| `KARL_REDIS_DB` | `0` | Redis database number |

### Examples

```bash
# MySQL configuration
export KARL_MYSQL_DSN="karl:password@tcp(mysql:3306)/karl?parseTime=true"

# Redis configuration for clustering
export KARL_REDIS_ENABLED=true
export KARL_REDIS_ADDR=redis.example.com:6379
export KARL_REDIS_PASSWORD=secretpassword
export KARL_REDIS_DB=0
```

### MySQL DSN Format

```
user:password@tcp(host:port)/database?parseTime=true
```

---

## Monitoring Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `KARL_METRICS_ENABLED` | `true` | Enable Prometheus metrics |
| `KARL_HEALTH_CHECK_INTERVAL` | `30` | Health check interval (seconds) |

### Examples

```bash
# Disable metrics (not recommended)
export KARL_METRICS_ENABLED=false

# Faster health checks
export KARL_HEALTH_CHECK_INTERVAL=15
```

---

## WebRTC Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `KARL_WEBRTC_ENABLED` | `true` | Enable WebRTC support |
| `KARL_STUN_SERVERS` | `stun:stun.l.google.com:19302` | STUN servers (comma-separated) |
| `KARL_TURN_SERVERS` | (empty) | TURN servers (JSON format) |
| `KARL_WEBRTC_MAX_BITRATE` | `2000000` | Maximum bitrate (bps) |

### Examples

```bash
# Enable WebRTC with custom STUN
export KARL_WEBRTC_ENABLED=true
export KARL_STUN_SERVERS="stun:stun1.example.com:3478,stun:stun2.example.com:3478"

# Configure TURN (JSON format)
export KARL_TURN_SERVERS='[{"url":"turn:turn.example.com:3478","username":"user","credential":"pass"}]'

# Limit bitrate
export KARL_WEBRTC_MAX_BITRATE=1000000
```

---

## Advanced Configuration

### Jitter Buffer

| Variable | Default | Description |
|----------|---------|-------------|
| `KARL_JITTER_BUFFER_ENABLED` | `true` | Enable jitter buffer |
| `KARL_JITTER_BUFFER_MIN_DELAY` | `20` | Minimum delay (ms) |
| `KARL_JITTER_BUFFER_MAX_DELAY` | `200` | Maximum delay (ms) |
| `KARL_JITTER_BUFFER_TARGET_DELAY` | `50` | Target delay (ms) |

### Forward Error Correction

| Variable | Default | Description |
|----------|---------|-------------|
| `KARL_FEC_ENABLED` | `true` | Enable FEC |
| `KARL_FEC_REDUNDANCY` | `0.30` | Redundancy ratio (0.0-1.0) |
| `KARL_FEC_ADAPTIVE_MODE` | `true` | Enable adaptive FEC |

### API Security

| Variable | Default | Description |
|----------|---------|-------------|
| `KARL_API_AUTH_ENABLED` | `false` | Enable API authentication |
| `KARL_API_RATE_LIMIT` | `60` | Requests per minute per client |
| `KARL_API_CORS_ENABLED` | `false` | Enable CORS |
| `KARL_API_CORS_ORIGINS` | `*` | Allowed CORS origins |
| `KARL_API_TLS_ENABLED` | `false` | Enable HTTPS |
| `KARL_API_TLS_CERT` | (empty) | Path to TLS certificate |
| `KARL_API_TLS_KEY` | (empty) | Path to TLS private key |

### Examples

```bash
# Tune jitter buffer for high-latency network
export KARL_JITTER_BUFFER_MIN_DELAY=40
export KARL_JITTER_BUFFER_MAX_DELAY=300
export KARL_JITTER_BUFFER_TARGET_DELAY=100

# Increase FEC for lossy network
export KARL_FEC_REDUNDANCY=0.50
export KARL_FEC_ADAPTIVE_MODE=true

# Enable API security
export KARL_API_AUTH_ENABLED=true
export KARL_API_RATE_LIMIT=120
export KARL_API_TLS_ENABLED=true
export KARL_API_TLS_CERT=/etc/karl/certs/server.crt
export KARL_API_TLS_KEY=/etc/karl/certs/server.key
```

---

## Alerts

| Variable | Default | Description |
|----------|---------|-------------|
| `KARL_ALERT_PACKET_LOSS_THRESHOLD` | `0.05` | Alert when packet loss exceeds 5% |
| `KARL_ALERT_JITTER_THRESHOLD` | `50` | Alert when jitter exceeds 50ms |
| `KARL_ALERT_NOTIFY_ADMIN` | `false` | Send alert notifications |
| `KARL_ALERT_ADMIN_EMAIL` | (empty) | Admin email for alerts |
| `KARL_ALERT_SLACK_WEBHOOK` | (empty) | Slack webhook URL |

---

## Priority Order

Environment variables override configuration file values:

1. Environment variables (highest priority)
2. Configuration file
3. Default values (lowest priority)

---

## Kubernetes ConfigMap Example

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: karl-env
data:
  KARL_LOG_LEVEL: "info"
  KARL_NG_PORT: "22222"
  KARL_RTP_MIN_PORT: "30000"
  KARL_RTP_MAX_PORT: "40000"
  KARL_MAX_SESSIONS: "10000"
  KARL_REDIS_ENABLED: "true"
  KARL_REDIS_ADDR: "redis:6379"
  KARL_RECORDING_ENABLED: "true"
  KARL_RECORDING_PATH: "/var/lib/karl/recordings"
```

---

## Docker Compose Example

```yaml
services:
  karl:
    image: loreste/karl:latest
    environment:
      - KARL_LOG_LEVEL=info
      - KARL_NG_PORT=22222
      - KARL_RTP_MIN_PORT=30000
      - KARL_RTP_MAX_PORT=40000
      - KARL_REDIS_ENABLED=true
      - KARL_REDIS_ADDR=redis:6379
      - KARL_RECORDING_ENABLED=true
```

---

## Next Steps

- [Configuration Reference](../configuration.md)
- [Installation Guide](../installation.md)
- [Kubernetes Deployment](../how-to/deploying-kubernetes.md)
