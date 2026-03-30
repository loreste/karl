# Configuration Reference

Karl uses a JSON configuration file with sensible defaults. All settings can be overridden via environment variables.

## Table of Contents

- [Configuration File Location](#configuration-file-location)
- [Complete Configuration Example](#complete-configuration-example)
- [Configuration Sections](#configuration-sections)
  - [NG Protocol](#ng-protocol)
  - [Sessions](#sessions)
  - [Jitter Buffer](#jitter-buffer)
  - [RTCP](#rtcp)
  - [Forward Error Correction](#forward-error-correction)
  - [Recording](#recording)
  - [REST API](#rest-api)
  - [WebRTC](#webrtc)
  - [Integration](#integration)
  - [Database](#database)
  - [SRTP](#srtp)
  - [Alerts](#alerts)
- [Environment Variables](#environment-variables)

---

## Configuration File Location

Karl looks for configuration in the following order:

1. Path specified via `-config` flag: `./karl -config /path/to/config.json`
2. Path specified via `KARL_CONFIG_PATH` environment variable
3. Default: `./config/config.json`

---

## Complete Configuration Example

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
    "cors_origins": "*",
    "tls_enabled": false,
    "tls_cert": "",
    "tls_key": ""
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

---

## Configuration Sections

### NG Protocol

Controls the rtpengine-compatible NG protocol interface.

```json
{
  "ng_protocol": {
    "enabled": true,
    "socket_path": "/var/run/karl/karl.sock",
    "udp_port": 22222,
    "timeout": 30
  }
}
```

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `enabled` | bool | `true` | Enable NG protocol listener |
| `socket_path` | string | `/var/run/karl/karl.sock` | Unix socket path for local communication |
| `udp_port` | int | `22222` | UDP port for NG protocol |
| `timeout` | int | `30` | Request timeout in seconds |

### Sessions

Controls session management and RTP port allocation.

```json
{
  "sessions": {
    "max_sessions": 10000,
    "session_ttl": 3600,
    "cleanup_interval": 60,
    "min_port": 30000,
    "max_port": 40000
  }
}
```

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `max_sessions` | int | `10000` | Maximum concurrent sessions |
| `session_ttl` | int | `3600` | Session time-to-live in seconds |
| `cleanup_interval` | int | `60` | Interval for cleaning stale sessions (seconds) |
| `min_port` | int | `30000` | Minimum RTP port number |
| `max_port` | int | `40000` | Maximum RTP port number |

**Port Range Calculation:**

Each session requires 4 ports (RTP + RTCP for each direction). With the default range of 30000-40000:
- Available ports: 10,000
- Maximum sessions: 2,500

Adjust `min_port` and `max_port` based on your expected concurrent call volume.

### Jitter Buffer

Controls the adaptive jitter buffer for smooth audio playback.

```json
{
  "jitter_buffer": {
    "enabled": true,
    "min_delay": 20,
    "max_delay": 200,
    "target_delay": 50,
    "adaptive_mode": true,
    "max_size": 100
  }
}
```

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `enabled` | bool | `true` | Enable jitter buffer |
| `min_delay` | int | `20` | Minimum buffering delay (ms) |
| `max_delay` | int | `200` | Maximum buffering delay (ms) |
| `target_delay` | int | `50` | Target delay for adaptive mode (ms) |
| `adaptive_mode` | bool | `true` | Automatically adjust buffer size based on network conditions |
| `max_size` | int | `100` | Maximum buffer size in packets |

**Tuning Guide:**

- **Low latency networks**: `min_delay: 10`, `target_delay: 30`
- **Standard networks**: `min_delay: 20`, `target_delay: 50` (default)
- **High jitter networks**: `min_delay: 40`, `target_delay: 100`

### RTCP

Controls RTCP (RTP Control Protocol) for quality monitoring.

```json
{
  "rtcp": {
    "enabled": true,
    "interval": 5,
    "reduced_size": false,
    "mux_enabled": true
  }
}
```

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `enabled` | bool | `true` | Enable RTCP processing |
| `interval` | int | `5` | RTCP report interval in seconds |
| `reduced_size` | bool | `false` | Use reduced-size RTCP (RFC 5506) |
| `mux_enabled` | bool | `true` | Enable RTCP-mux (RTP and RTCP on same port) |

### Forward Error Correction

Controls FEC for packet loss recovery.

```json
{
  "fec": {
    "enabled": true,
    "block_size": 48,
    "redundancy": 0.30,
    "adaptive_mode": true,
    "max_redundancy": 0.50,
    "min_redundancy": 0.10
  }
}
```

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `enabled` | bool | `true` | Enable Forward Error Correction |
| `block_size` | int | `48` | Number of packets per FEC block |
| `redundancy` | float | `0.30` | Base redundancy ratio (0.0-1.0) |
| `adaptive_mode` | bool | `true` | Adjust redundancy based on packet loss |
| `max_redundancy` | float | `0.50` | Maximum redundancy when adapting |
| `min_redundancy` | float | `0.10` | Minimum redundancy when adapting |

**Redundancy Impact:**

| Redundancy | Bandwidth Overhead | Recovery Capability |
|------------|-------------------|---------------------|
| 0.10 (10%) | Low | Up to ~8% packet loss |
| 0.30 (30%) | Medium | Up to ~20% packet loss |
| 0.50 (50%) | High | Up to ~35% packet loss |

### Recording

Controls call recording functionality.

```json
{
  "recording": {
    "enabled": true,
    "base_path": "/var/lib/karl/recordings",
    "format": "wav",
    "mode": "stereo",
    "sample_rate": 8000,
    "bits_per_sample": 16,
    "max_file_size": 104857600,
    "retention_days": 30
  }
}
```

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `enabled` | bool | `false` | Enable recording system |
| `base_path` | string | `/var/lib/karl/recordings` | Directory for recording files |
| `format` | string | `wav` | Output format: `wav` or `pcm` |
| `mode` | string | `stereo` | Recording mode: `mixed`, `stereo`, or `separate` |
| `sample_rate` | int | `8000` | Sample rate in Hz (8000, 16000, 48000) |
| `bits_per_sample` | int | `16` | Bits per sample (8 or 16) |
| `max_file_size` | int | `104857600` | Max file size before rotation (bytes) |
| `retention_days` | int | `30` | Days to keep recordings before cleanup |

**Recording Modes:**

| Mode | Description | Files Created |
|------|-------------|---------------|
| `mixed` | Both parties mixed into mono | 1 file |
| `stereo` | Caller left channel, callee right | 1 file |
| `separate` | Each party in separate file | 2 files |

**Storage Calculation:**

WAV at 8kHz/16-bit: ~1 MB per minute per channel
- Mixed mode: ~1 MB/minute
- Stereo mode: ~2 MB/minute
- Separate mode: ~2 MB/minute (2 files)

### REST API

Controls the REST API server.

```json
{
  "api": {
    "enabled": true,
    "address": ":8080",
    "auth_enabled": false,
    "rate_limit_per_min": 60,
    "cors_enabled": false,
    "cors_origins": "*",
    "tls_enabled": false,
    "tls_cert": "",
    "tls_key": ""
  }
}
```

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `enabled` | bool | `true` | Enable REST API |
| `address` | string | `:8080` | Listen address and port |
| `auth_enabled` | bool | `false` | Require API key authentication |
| `rate_limit_per_min` | int | `60` | Max requests per minute per client |
| `cors_enabled` | bool | `false` | Enable CORS headers |
| `cors_origins` | string | `*` | Allowed CORS origins |
| `tls_enabled` | bool | `false` | Enable HTTPS |
| `tls_cert` | string | | Path to TLS certificate |
| `tls_key` | string | | Path to TLS private key |

### WebRTC

Controls WebRTC functionality for browser-based clients.

```json
{
  "webrtc": {
    "enabled": true,
    "webrtc_port": 8443,
    "stun_servers": [
      "stun:stun.l.google.com:19302"
    ],
    "turn_servers": [],
    "max_bitrate": 2000000,
    "start_bitrate": 1000000,
    "bw_estimation": true,
    "tcc_enabled": true
  }
}
```

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `enabled` | bool | `true` | Enable WebRTC support |
| `webrtc_port` | int | `8443` | WebRTC signaling port |
| `stun_servers` | array | Google STUN | STUN servers for NAT traversal |
| `turn_servers` | array | `[]` | TURN servers for relay |
| `max_bitrate` | int | `2000000` | Maximum bitrate (bps) |
| `start_bitrate` | int | `1000000` | Initial bitrate (bps) |
| `bw_estimation` | bool | `true` | Enable bandwidth estimation |
| `tcc_enabled` | bool | `true` | Enable Transport-CC feedback |

### Integration

Controls integration with SIP proxies.

```json
{
  "integration": {
    "opensips_ip": "",
    "opensips_port": 0,
    "kamailio_ip": "",
    "kamailio_port": 0,
    "media_ip": "auto",
    "public_ip": "",
    "keepalive_interval": 30
  }
}
```

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `opensips_ip` | string | | OpenSIPS server IP for registration |
| `opensips_port` | int | | OpenSIPS server port |
| `kamailio_ip` | string | | Kamailio server IP for registration |
| `kamailio_port` | int | | Kamailio server port |
| `media_ip` | string | `auto` | IP address for media (SDP). Use `auto` for detection |
| `public_ip` | string | | Public IP for NAT scenarios |
| `keepalive_interval` | int | `30` | Keepalive interval to SIP proxies (seconds) |

### Database

Controls database connections for CDR and session storage.

```json
{
  "database": {
    "mysql_dsn": "",
    "redis_enabled": false,
    "redis_addr": "localhost:6379",
    "redis_cleanup_interval": 3600,
    "max_connections": 10,
    "connection_timeout": 30
  }
}
```

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `mysql_dsn` | string | | MySQL connection string |
| `redis_enabled` | bool | `false` | Enable Redis for session caching |
| `redis_addr` | string | `localhost:6379` | Redis server address |
| `redis_cleanup_interval` | int | `3600` | Redis cleanup interval (seconds) |
| `max_connections` | int | `10` | Maximum database connections |
| `connection_timeout` | int | `30` | Connection timeout (seconds) |

**MySQL DSN Format:**

```
user:password@tcp(host:port)/database?parseTime=true
```

Example:
```
karl:secretpassword@tcp(127.0.0.1:3306)/karl?parseTime=true
```

### SRTP

Controls SRTP encryption settings.

```json
{
  "srtp": {
    "srtp_key": "",
    "srtp_salt": ""
  }
}
```

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `srtp_key` | string | | Master key for SRTP (base64 encoded) |
| `srtp_salt` | string | | Master salt for SRTP (base64 encoded) |

### Alerts

Controls quality alerting thresholds.

```json
{
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

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `packet_loss_threshold` | float | `0.05` | Alert when packet loss exceeds 5% |
| `jitter_threshold` | float | `50.0` | Alert when jitter exceeds 50ms |
| `bandwidth_threshold` | int | `1000000` | Alert when bandwidth exceeds threshold |
| `notify_admin` | bool | `false` | Send alert notifications |
| `admin_email` | string | | Email for alerts |
| `slack_webhook` | string | | Slack webhook URL for alerts |

---

## Environment Variables

All configuration options can be overridden via environment variables:

| Variable | Config Path | Description |
|----------|-------------|-------------|
| `KARL_CONFIG_PATH` | - | Path to configuration file |
| `KARL_LOG_LEVEL` | - | Logging level (debug, info, warn, error) |
| `KARL_HEALTH_PORT` | - | Health check port (default: `:8086`) |
| `KARL_METRICS_PORT` | - | Prometheus metrics port (default: `:9091`) |
| `KARL_API_PORT` | `api.address` | REST API port |
| `KARL_NG_PORT` | `ng_protocol.udp_port` | NG protocol UDP port |
| `KARL_RTP_MIN_PORT` | `sessions.min_port` | Minimum RTP port |
| `KARL_RTP_MAX_PORT` | `sessions.max_port` | Maximum RTP port |
| `KARL_MAX_SESSIONS` | `sessions.max_sessions` | Maximum concurrent sessions |
| `KARL_RECORDING_PATH` | `recording.base_path` | Recording storage path |
| `KARL_RECORDING_ENABLED` | `recording.enabled` | Enable recording |
| `KARL_MYSQL_DSN` | `database.mysql_dsn` | MySQL connection string |
| `KARL_REDIS_ENABLED` | `database.redis_enabled` | Enable Redis |
| `KARL_REDIS_ADDR` | `database.redis_addr` | Redis address |
| `KARL_MEDIA_IP` | `integration.media_ip` | Media IP address |
| `KARL_PUBLIC_IP` | `integration.public_ip` | Public IP address |
| `KARL_RUN_DIR` | - | Runtime directory |

Environment variables take precedence over configuration file values.

---

## Next Steps

- [Getting Started](./getting-started.md)
- [Kubernetes Deployment](./how-to/deploying-kubernetes.md)
- [Monitoring Setup](./how-to/monitoring-prometheus.md)
