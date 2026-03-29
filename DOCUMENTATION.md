# Karl Media Server Documentation

## Table of Contents

- [Installation](#installation)
- [Configuration](#configuration)
- [NG Protocol Reference](#ng-protocol-reference)
- [REST API Reference](#rest-api-reference)
- [Recording System](#recording-system)
- [Monitoring & Metrics](#monitoring--metrics)
- [Development](#development)
- [Troubleshooting](#troubleshooting)

---

## Installation

### Prerequisites

- Go 1.21 or higher
- MySQL/MariaDB (optional, for CDR and session persistence)
- Redis (optional, for distributed caching)
- Prometheus (optional, for metrics collection)

### From Source

```bash
# Clone the repository
git clone https://github.com/loreste/karl.git
cd karl

# Build the application
go build -o karl

# Create configuration directory
sudo mkdir -p /etc/karl
sudo mkdir -p /var/lib/karl/recordings
sudo mkdir -p /var/run/karl

# Copy example configuration
sudo cp config/config.json /etc/karl/

# Run
./karl -config /etc/karl/config.json
```

### Using Docker

```bash
# Run with default configuration
docker run -d \
  --name karl \
  -p 22222:22222/udp \
  -p 30000-40000:30000-40000/udp \
  -p 8080:8080 \
  -p 8086:8086 \
  -p 9091:9091 \
  loreste/karl:latest

# Run with custom configuration
docker run -d \
  --name karl \
  -v /path/to/config.json:/etc/karl/config.json \
  -v /path/to/recordings:/var/lib/karl/recordings \
  -p 22222:22222/udp \
  -p 30000-40000:30000-40000/udp \
  -p 8080:8080 \
  loreste/karl:latest
```

### Docker Compose

```yaml
version: '3.8'
services:
  karl:
    image: loreste/karl:latest
    ports:
      - "22222:22222/udp"
      - "30000-40000:30000-40000/udp"
      - "8080:8080"
      - "8086:8086"
      - "9091:9091"
    volumes:
      - ./config.json:/etc/karl/config.json
      - recordings:/var/lib/karl/recordings
    depends_on:
      - mysql
      - redis

  mysql:
    image: mariadb:10.11
    environment:
      MYSQL_ROOT_PASSWORD: rootpassword
      MYSQL_DATABASE: karl
      MYSQL_USER: karl
      MYSQL_PASSWORD: karlpassword
    volumes:
      - mysql_data:/var/lib/mysql

  redis:
    image: redis:7-alpine
    volumes:
      - redis_data:/data

  prometheus:
    image: prom/prometheus:latest
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml

volumes:
  recordings:
  mysql_data:
  redis_data:
```

### System Requirements

| Resource | Minimum | Recommended |
|----------|---------|-------------|
| CPU | 2 cores | 4+ cores |
| RAM | 2 GB | 4+ GB |
| Disk | 100 MB (application) | + recording storage |
| Network | 100 Mbps | 1 Gbps |

### Systemd Service

```ini
# /etc/systemd/system/karl.service
[Unit]
Description=Karl Media Server
After=network.target mysql.service redis.service

[Service]
Type=simple
User=karl
Group=karl
ExecStart=/usr/local/bin/karl -config /etc/karl/config.json
Restart=always
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
```

---

## Configuration

Karl uses a JSON configuration file. All settings have sensible defaults.

### Complete Configuration Reference

```json
{
  "version": "1.0.0",
  "environment": "production",

  "transport": {
    "udp_enabled": true,
    "udp_port": 12000,
    "tcp_enabled": false,
    "tcp_port": 12001,
    "tls_enabled": false,
    "tls_port": 12002,
    "tls_cert": "/etc/karl/certs/server.crt",
    "tls_key": "/etc/karl/certs/server.key",
    "ipv6_enabled": false,
    "mtu": 1500
  },

  "ng_protocol": {
    "enabled": true,
    "socket_path": "/var/run/karl/karl.sock",
    "udp_port": 22222,
    "timeout": 30
  },

  "sessions": {
    "max_sessions": 10000,
    "session_ttl": 3600,
    "cleanup_interval": 60,
    "min_port": 30000,
    "max_port": 40000
  },

  "jitter_buffer": {
    "enabled": true,
    "min_delay": 20,
    "max_delay": 200,
    "target_delay": 50,
    "adaptive_mode": true,
    "max_size": 100
  },

  "rtcp": {
    "enabled": true,
    "interval": 5,
    "reduced_size": false,
    "mux_enabled": true
  },

  "fec": {
    "enabled": true,
    "block_size": 48,
    "redundancy": 0.30,
    "adaptive_mode": true,
    "max_redundancy": 0.50,
    "min_redundancy": 0.10
  },

  "recording": {
    "enabled": true,
    "base_path": "/var/lib/karl/recordings",
    "format": "wav",
    "mode": "stereo",
    "sample_rate": 8000,
    "bits_per_sample": 16,
    "max_file_size": 104857600,
    "retention_days": 30
  },

  "api": {
    "enabled": true,
    "address": ":8080",
    "auth_enabled": false,
    "rate_limit_per_min": 60,
    "cors_enabled": false,
    "cors_origins": "*"
  },

  "webrtc": {
    "enabled": true,
    "webrtc_port": 8443,
    "stun_servers": [
      "stun:stun.l.google.com:19302",
      "stun:stun1.l.google.com:19302"
    ],
    "turn_servers": [],
    "max_bitrate": 2000000,
    "start_bitrate": 1000000,
    "bw_estimation": true,
    "tcc_enabled": true
  },

  "integration": {
    "opensips_ip": "",
    "opensips_port": 0,
    "kamailio_ip": "",
    "kamailio_port": 0,
    "media_ip": "auto",
    "public_ip": "",
    "keepalive_interval": 30
  },

  "database": {
    "mysql_dsn": "",
    "redis_enabled": false,
    "redis_addr": "localhost:6379",
    "redis_cleanup_interval": 3600,
    "max_connections": 10,
    "connection_timeout": 30
  },

  "srtp": {
    "srtp_key": "",
    "srtp_salt": ""
  },

  "alert_settings": {
    "packet_loss_threshold": 0.05,
    "jitter_threshold": 50.0,
    "bandwidth_threshold": 1000000,
    "notify_admin": false,
    "admin_email": "",
    "slack_webhook": ""
  }
}
```

### Configuration Sections

#### NG Protocol (`ng_protocol`)

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `enabled` | bool | `true` | Enable NG protocol listener |
| `socket_path` | string | `/var/run/karl/karl.sock` | Unix socket path |
| `udp_port` | int | `22222` | UDP port for NG protocol |
| `timeout` | int | `30` | Request timeout in seconds |

#### Sessions (`sessions`)

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `max_sessions` | int | `10000` | Maximum concurrent sessions |
| `session_ttl` | int | `3600` | Session time-to-live in seconds |
| `cleanup_interval` | int | `60` | Stale session cleanup interval |
| `min_port` | int | `30000` | Minimum RTP port |
| `max_port` | int | `40000` | Maximum RTP port |

#### Jitter Buffer (`jitter_buffer`)

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `enabled` | bool | `true` | Enable jitter buffer |
| `min_delay` | int | `20` | Minimum delay in milliseconds |
| `max_delay` | int | `200` | Maximum delay in milliseconds |
| `target_delay` | int | `50` | Target delay in milliseconds |
| `adaptive_mode` | bool | `true` | Enable adaptive sizing |
| `max_size` | int | `100` | Maximum buffer size in packets |

#### Forward Error Correction (`fec`)

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `enabled` | bool | `true` | Enable FEC |
| `block_size` | int | `48` | Packets per FEC block |
| `redundancy` | float | `0.30` | Redundancy ratio (0.0-1.0) |
| `adaptive_mode` | bool | `true` | Adjust based on loss rate |
| `max_redundancy` | float | `0.50` | Maximum redundancy |
| `min_redundancy` | float | `0.10` | Minimum redundancy |

#### Recording (`recording`)

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `enabled` | bool | `false` | Enable recording system |
| `base_path` | string | `/var/lib/karl/recordings` | Recording storage path |
| `format` | string | `wav` | Output format (`wav`, `pcm`) |
| `mode` | string | `stereo` | Recording mode (`mixed`, `stereo`, `separate`) |
| `sample_rate` | int | `8000` | Sample rate in Hz |
| `bits_per_sample` | int | `16` | Bits per sample |
| `max_file_size` | int | `104857600` | Max file size before rotation (bytes) |
| `retention_days` | int | `30` | Days to retain recordings |

#### REST API (`api`)

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `enabled` | bool | `true` | Enable REST API |
| `address` | string | `:8080` | Listen address |
| `auth_enabled` | bool | `false` | Enable API key authentication |
| `rate_limit_per_min` | int | `60` | Requests per minute per client |
| `cors_enabled` | bool | `false` | Enable CORS |
| `cors_origins` | string | `*` | Allowed CORS origins |

---

## NG Protocol Reference

Karl implements the rtpengine NG protocol for compatibility with OpenSIPS and Kamailio.

### Protocol Format

Messages use bencode encoding with a cookie prefix:

```
<cookie> <bencoded-message>
```

### Commands

#### ping

Health check command.

**Request:**
```
d7:command4:pinge
```

**Response:**
```
d6:result4:ponge
```

#### offer

Process SDP offer for new or existing call.

**Request parameters:**
| Parameter | Required | Description |
|-----------|----------|-------------|
| `call-id` | Yes | Unique call identifier |
| `from-tag` | Yes | SIP From-tag |
| `sdp` | Yes | SDP offer |
| `ICE` | No | ICE handling (`remove`, `force`) |
| `DTLS` | No | DTLS handling (`passive`, `active`) |
| `SDES` | No | SDES handling (`off`, `unencrypted`) |
| `direction` | No | Media direction |
| `replace` | No | SDP elements to replace |
| `flags` | No | Additional flags |

**Response:**
```
d6:result2:ok3:sdp...e
```

#### answer

Process SDP answer for existing call.

**Request parameters:**
| Parameter | Required | Description |
|-----------|----------|-------------|
| `call-id` | Yes | Call identifier |
| `from-tag` | Yes | SIP From-tag |
| `to-tag` | Yes | SIP To-tag |
| `sdp` | Yes | SDP answer |

#### delete

Terminate call and release resources.

**Request parameters:**
| Parameter | Required | Description |
|-----------|----------|-------------|
| `call-id` | Yes | Call identifier |
| `from-tag` | No | Specific party to delete |
| `to-tag` | No | Specific party to delete |

#### query

Get call statistics.

**Request parameters:**
| Parameter | Required | Description |
|-----------|----------|-------------|
| `call-id` | Yes | Call identifier |

**Response includes:**
- Total call duration
- Packets sent/received per leg
- Bytes sent/received per leg
- Packet loss statistics
- Jitter measurements

#### list

List all active calls.

**Response:**
```
d6:result2:ok5:callsl...ee
```

#### start recording

Start recording a call.

**Request parameters:**
| Parameter | Required | Description |
|-----------|----------|-------------|
| `call-id` | Yes | Call identifier |

#### stop recording

Stop recording a call.

**Request parameters:**
| Parameter | Required | Description |
|-----------|----------|-------------|
| `call-id` | Yes | Call identifier |

### Integration Examples

#### OpenSIPS

```opensips
loadmodule "rtpengine.so"

modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")

route {
    if (is_method("INVITE")) {
        rtpengine_manage("ICE=remove RTP/AVP");
    }

    if (is_method("BYE")) {
        rtpengine_delete();
    }
}

onreply_route {
    if (has_body("application/sdp")) {
        rtpengine_manage("ICE=remove RTP/AVP");
    }
}
```

#### Kamailio

```kamailio
loadmodule "rtpengine.so"

modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")

request_route {
    if (is_method("INVITE")) {
        rtpengine_manage("ICE=remove");
    }

    if (is_method("BYE")) {
        rtpengine_delete();
    }
}

onreply_route[MANAGE_REPLY] {
    if (has_body("application/sdp")) {
        rtpengine_manage("ICE=remove");
    }
}
```

---

## REST API Reference

### Base URL

```
http://<server>:8080/api/v1
```

### Authentication

When `auth_enabled` is true, include the API key in the header:

```
Authorization: Bearer <api-key>
```

### Endpoints

#### Sessions

**List Sessions**
```http
GET /api/v1/sessions
```

Response:
```json
{
  "sessions": [
    {
      "id": "abc123",
      "call_id": "call-456",
      "state": "active",
      "created_at": "2024-01-15T10:30:00Z",
      "caller": {
        "ip": "192.168.1.100",
        "port": 30000
      },
      "callee": {
        "ip": "192.168.1.101",
        "port": 30002
      }
    }
  ],
  "total": 1
}
```

**Get Session**
```http
GET /api/v1/sessions/{id}
```

**Delete Session**
```http
DELETE /api/v1/sessions/{id}
```

#### Statistics

**Aggregate Statistics**
```http
GET /api/v1/stats
```

Response:
```json
{
  "active_sessions": 42,
  "total_sessions": 15234,
  "packets_sent": 1234567,
  "packets_received": 1234000,
  "bytes_sent": 98765432,
  "bytes_received": 98700000,
  "avg_jitter_ms": 12.5,
  "avg_packet_loss": 0.002
}
```

**Call Statistics**
```http
GET /api/v1/stats/{call_id}
```

#### Recording

**Start Recording**
```http
POST /api/v1/recording/start
Content-Type: application/json

{
  "session_id": "abc123",
  "mode": "stereo"
}
```

**Stop Recording**
```http
POST /api/v1/recording/stop
Content-Type: application/json

{
  "session_id": "abc123"
}
```

**List Recordings**
```http
GET /api/v1/recordings
```

**Get Recording**
```http
GET /api/v1/recordings/{id}
```

**Delete Recording**
```http
DELETE /api/v1/recordings/{id}
```

#### Health

**Simple Health Check**
```http
GET /health
```

Response:
```json
{
  "status": "healthy"
}
```

**Detailed Health**
```http
GET /health/detail
```

Response:
```json
{
  "status": "healthy",
  "components": {
    "ng_listener": {
      "status": "UP",
      "details": {
        "uptime": "24h15m30s",
        "active_calls": "42"
      }
    },
    "database": {
      "status": "UP"
    },
    "redis": {
      "status": "UP"
    }
  }
}
```

---

## Recording System

### Recording Modes

| Mode | Description | Output |
|------|-------------|--------|
| `mixed` | Both parties mixed into mono | Single file |
| `stereo` | Caller on left, callee on right | Single file |
| `separate` | Each party in separate file | Two files |

### File Naming

Recordings are stored with the following naming convention:

```
{base_path}/{YYYY-MM-DD}/{call_id}_{timestamp}_{mode}.wav
```

Example:
```
/var/lib/karl/recordings/2024-01-15/call-123_1705312200_stereo.wav
```

### Codec Support

Karl automatically transcodes to PCM for recording:

| Input Codec | Supported |
|-------------|-----------|
| G.711 u-law | Yes |
| G.711 a-law | Yes |
| Opus | Yes |
| G.722 | Yes |

---

## Monitoring & Metrics

### Prometheus Metrics

Karl exposes metrics at `:9091/metrics`.

#### Session Metrics

```prometheus
# Active sessions
karl_sessions_active

# Total sessions created
karl_sessions_total

# Session duration histogram
karl_session_duration_seconds_bucket{le="10"}
karl_session_duration_seconds_bucket{le="60"}
karl_session_duration_seconds_bucket{le="300"}
karl_session_duration_seconds_bucket{le="3600"}
```

#### RTCP Metrics

```prometheus
# RTCP packets sent/received
karl_rtcp_sr_sent_total
karl_rtcp_rr_sent_total
karl_rtcp_sr_received_total
karl_rtcp_rr_received_total

# Round-trip time
karl_rtcp_rtt_seconds

# Jitter
karl_rtcp_jitter_seconds

# Packet loss
karl_rtcp_packet_loss_fraction
```

#### FEC Metrics

```prometheus
# FEC packets sent
karl_fec_packets_sent_total

# Successful recoveries
karl_fec_recoveries_total

# Recovery failures
karl_fec_recovery_failures_total

# Current redundancy ratio
karl_fec_redundancy_ratio
```

#### Jitter Buffer Metrics

```prometheus
# Buffer size
karl_jitter_buffer_size{session_id="..."}

# Buffer latency
karl_jitter_buffer_latency_seconds{session_id="..."}

# Dropped packets
karl_jitter_buffer_packets_dropped_total{session_id="...", reason="late|duplicate|overflow"}
```

#### API Metrics

```prometheus
# Request count
karl_api_requests_total{endpoint="/sessions", method="GET", status="200"}

# Request duration
karl_api_request_duration_seconds{endpoint="/sessions"}
```

### Grafana Dashboard

Import the included Grafana dashboard from `grafana/karl-dashboard.json` for comprehensive visualization.

### Alerting Rules

Example Prometheus alerting rules:

```yaml
groups:
  - name: karl
    rules:
      - alert: HighPacketLoss
        expr: karl_rtcp_packet_loss_fraction > 0.05
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High packet loss detected"

      - alert: NoActiveSessions
        expr: karl_sessions_active == 0
        for: 10m
        labels:
          severity: info
        annotations:
          summary: "No active sessions"
```

---

## Development

### Building from Source

```bash
git clone https://github.com/loreste/karl.git
cd karl

# Install dependencies
go mod download

# Run tests
go test ./...

# Run tests with race detection
go test -race ./...

# Run benchmarks
go test -bench=. ./internal/tests/...

# Build
go build -o karl
```

### Running Tests

```bash
# All tests
go test ./...

# With coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Stress tests
go test -v ./internal/tests/... -run "Stress|Concurrency|Memory|Goroutine"
```

### Code Structure

```
karl/
├── main.go                 # Entry point
├── server.go               # Core server
├── services.go             # Service initialization
├── config.go               # Configuration loading
├── webrtc.go               # WebRTC handling
├── internal/
│   ├── session_manager.go  # Session registry
│   ├── ng_socket_listener.go # NG protocol handler
│   ├── rtcp_handler.go     # RTCP implementation
│   ├── jitter_buffer.go    # Adaptive jitter buffer
│   ├── fec_handler.go      # Forward error correction
│   ├── rtp_control.go      # RTP packet handling
│   ├── api/
│   │   ├── router.go       # REST API router
│   │   └── handlers_*.go   # API handlers
│   ├── auth/
│   │   ├── apikey.go       # API key auth
│   │   └── ratelimit.go    # Rate limiting
│   ├── recording/
│   │   ├── recorder.go     # Recording engine
│   │   ├── mixer.go        # Audio mixing
│   │   └── manager.go      # Recording lifecycle
│   └── ng_protocol/
│       ├── bencode.go      # Bencode encoding
│       ├── types.go        # Protocol types
│       └── commands/       # Command handlers
└── config/
    └── config.json         # Example configuration
```

---

## Troubleshooting

### Common Issues

#### "bind: address already in use"

Another process is using the configured port.

```bash
# Find the process
sudo lsof -i :22222
sudo lsof -i :30000-40000

# Kill if necessary
sudo kill <PID>
```

#### "too many open files"

Increase file descriptor limits:

```bash
# Temporary
ulimit -n 65535

# Permanent - add to /etc/security/limits.conf
karl soft nofile 65535
karl hard nofile 65535
```

#### Sessions not being created

1. Check NG protocol is enabled and port is accessible
2. Verify SIP proxy configuration points to Karl
3. Check logs for bencode parsing errors

```bash
# Test NG protocol connectivity
echo -n "d7:command4:pinge" | nc -u 127.0.0.1 22222
```

#### High packet loss

1. Check network path between endpoints
2. Increase jitter buffer size
3. Enable FEC with higher redundancy
4. Check for CPU saturation

#### Recording files are empty

1. Verify recording base path exists and is writable
2. Check that sessions are reaching "active" state
3. Ensure codecs are supported for transcoding

### Logging

Set log level via environment variable:

```bash
KARL_LOG_LEVEL=debug ./karl
```

Log levels: `debug`, `info`, `warn`, `error`

### Debug Mode

Enable verbose protocol logging:

```json
{
  "debug": {
    "log_rtp_packets": true,
    "log_rtcp_packets": true,
    "log_ng_messages": true
  }
}
```

### Getting Help

1. Check logs: `journalctl -u karl -f`
2. Review metrics: `curl localhost:9091/metrics`
3. Open an issue: https://github.com/loreste/karl/issues

---

## Contact

- **Repository**: https://github.com/loreste/karl
- **Issues**: https://github.com/loreste/karl/issues
