# Karl Media Server Documentation

## Table of Contents

- [Installation](#installation)
- [Configuration](#configuration)
- [API Reference](#api-reference)
- [Development](#development)
- [Troubleshooting](#troubleshooting)

## Installation

### Prerequisites

- Go 1.23.2 or higher
- MySQL/MariaDB for database storage
- Redis (optional) for caching
- Prometheus (optional) for metrics

### From Source

```bash
# Clone the repository
git clone https://github.com/karlmediaserver/karl.git
cd karl

# Build the application
go build -o karl

# Create configuration directory
mkdir -p /etc/karl

# Copy example configuration
cp config/config.json /etc/karl/
```

### Using Docker

```bash
# Pull the latest image
docker pull karlmediaserver/karl:latest

# Run with default configuration
docker run -p 12000:12000/udp -p 9091:9091 karlmediaserver/karl:latest

# Run with custom configuration
docker run -v /path/to/your/config.json:/etc/karl/config.json \
  -p 12000:12000/udp -p 9091:9091 karlmediaserver/karl:latest
```

### System Requirements

- CPU: 2+ cores recommended for moderate traffic
- RAM: 2GB minimum, 4GB+ recommended
- Network: Low-latency connection recommended for media handling
- Disk: 50MB for application, plus space for logs and recordings

## Configuration

Karl Media Server uses a JSON configuration file located at `config/config.json`. 

### Configuration Options

#### Transport Settings

```json
"transport": {
  "udp_enabled": true,
  "udp_port": 12000,
  "tcp_enabled": true,
  "tcp_port": 12001,
  "tls_enabled": true,
  "tls_port": 12002,
  "tls_cert": "certs/server.crt",
  "tls_key": "certs/server.key"
}
```

| Setting | Description | Default |
|---------|-------------|---------|
| udp_enabled | Enable UDP transport | true |
| udp_port | UDP port for RTP/RTCP | 12000 |
| tcp_enabled | Enable TCP transport | true |
| tcp_port | TCP port for RTP/RTCP | 12001 |
| tls_enabled | Enable TLS transport | true |
| tls_port | TLS port for secure RTP/RTCP | 12002 |
| tls_cert | Path to TLS certificate | certs/server.crt |
| tls_key | Path to TLS private key | certs/server.key |

#### WebRTC Settings

```json
"webrtc": {
  "enabled": true,
  "webrtc_port": 8443,
  "stun_servers": [
    "stun:stun.l.google.com:19302",
    "stun:stun1.l.google.com:19302"
  ],
  "turn_servers": [
    {
      "url": "turn:your-turn-server.com:3478",
      "username": "turnuser",
      "credential": "turnpass"
    }
  ]
}
```

| Setting | Description | Default |
|---------|-------------|---------|
| enabled | Enable WebRTC support | true |
| webrtc_port | Port for WebRTC signaling | 8443 |
| stun_servers | Array of STUN server URLs | ["stun:stun.l.google.com:19302"] |
| turn_servers | Array of TURN server configurations | [] |

#### Integration Settings

```json
"integration": {
  "opensips_ip": "127.0.0.1",
  "opensips_port": 5060,
  "kamailio_ip": "127.0.0.1",
  "kamailio_port": 5061,
  "rtpengine_socket": "/var/run/karl/rtpengine.sock",
  "unix_socket_path": "/var/run/karl/karl.sock",
  "media_ip": "192.168.1.100"
}
```

| Setting | Description | Default |
|---------|-------------|---------|
| opensips_ip | OpenSIPS server IP | 127.0.0.1 |
| opensips_port | OpenSIPS SIP port | 5060 |
| kamailio_ip | Kamailio server IP | 127.0.0.1 |
| kamailio_port | Kamailio SIP port | 5061 |
| rtpengine_socket | Path to RTPengine socket | /var/run/karl/rtpengine.sock |
| unix_socket_path | Path to Karl socket | /var/run/karl/karl.sock |
| media_ip | Local media IP address | (Auto-detected) |

#### Database Settings

```json
"database": {
  "mysql_dsn": "user:password@tcp(localhost:3306)/rtpdb",
  "redis_enabled": true,
  "redis_addr": "localhost:6379",
  "redis_cleanup_interval": 3600
}
```

| Setting | Description | Default |
|---------|-------------|---------|
| mysql_dsn | MySQL connection string | "" |
| redis_enabled | Enable Redis caching | false |
| redis_addr | Redis server address | localhost:6379 |
| redis_cleanup_interval | Session cleanup interval (seconds) | 3600 |

#### SRTP Settings

```json
"srtp": {
  "srtp_key": "your-base64-encoded-key",
  "srtp_salt": "your-base64-encoded-salt"
}
```

| Setting | Description | Default |
|---------|-------------|---------|
| srtp_key | Base64-encoded SRTP master key | (Generated) |
| srtp_salt | Base64-encoded SRTP master salt | (Generated) |

### Environment Variables

Karl Media Server also supports configuration via environment variables:

| Variable | Description | Example |
|----------|-------------|---------|
| KARL_CONFIG_PATH | Custom config path | /etc/karl/custom-config.json |
| KARL_LOG_LEVEL | Log verbosity (1-5) | 3 |
| KARL_METRICS_PORT | Metrics port | 9091 |
| KARL_MEDIA_IP | Override media IP | 192.168.1.100 |
| KARL_SRTP_KEY | SRTP master key | (base64 encoded key) |
| KARL_SRTP_SALT | SRTP master salt | (base64 encoded salt) |

## API Reference

Karl Media Server provides a RESTful API for management and monitoring.

### Base URL

All API endpoints are available at:

```
http://<server-address>:9091/api/v1
```

### Endpoints

#### Health Check

```
GET /health
```

Returns server health status.

Response:
```json
{
  "status": "healthy",
  "uptime": "3h15m20s",
  "version": "1.0.0"
}
```

#### Configuration Management

```
GET /config
```

Returns current server configuration.

```
POST /config
```

Updates server configuration dynamically.

Request body:
```json
{
  "webrtc": {
    "enabled": true,
    "stun_servers": ["stun:stun.l.google.com:19302"]
  }
}
```

#### RTP Statistics

```
GET /stats/rtp
```

Returns RTP statistics.

Response:
```json
{
  "packets_received": 12500,
  "packets_sent": 12450,
  "packets_dropped": 50,
  "active_sessions": 5
}
```

#### WebRTC Statistics

```
GET /stats/webrtc
```

Returns WebRTC statistics.

Response:
```json
{
  "active_connections": 3,
  "ice_stats": {
    "succeeded": 8,
    "failed": 1
  }
}
```

## Development

### Building from Source

```bash
# Clone the repository
git clone https://github.com/karlmediaserver/karl.git
cd karl

# Get dependencies
go mod download

# Build the application
go build -o karl
```

### Testing

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run specific tests
go test ./internal/tests/codec_converter_test.go
```

### Development Environment

It's recommended to set up a development environment with:

1. MySQL or MariaDB for database functionality
2. Redis for caching (optional)
3. Prometheus for metrics monitoring (optional)

You can use the following docker-compose file for a quick setup:

```yaml
version: '3'
services:
  mysql:
    image: mariadb:10.8
    environment:
      MYSQL_ROOT_PASSWORD: password
      MYSQL_DATABASE: rtpdb
      MYSQL_USER: karl
      MYSQL_PASSWORD: karl
    ports:
      - "3306:3306"
    volumes:
      - ./mysql_schema.sql:/docker-entrypoint-initdb.d/mysql_schema.sql

  redis:
    image: redis:7.0
    ports:
      - "6379:6379"

  prometheus:
    image: prom/prometheus:v2.36.0
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
```

### Code Structure

- `main.go` - Application entry point
- `server.go` - Core server implementation
- `config.go` - Configuration handling
- `services.go` - Service initialization
- `webrtc.go` - WebRTC handling
- `internal/` - Internal package with core functionality
  - `codec_converter.go` - Codec conversion utilities
  - `config_loader.go` - Configuration loading and validation
  - `rtp_control.go` - RTP packet handling
  - `sip_register.go` - SIP registration

## Troubleshooting

### Common Issues

#### "Failed to create /var/run/karl/"

**Problem**: Permission denied when creating the run directory.

**Solution**: Either run Karl as root/with sudo, or modify the run directory in the source code to use a directory the user has permissions for.

#### "Failed to bind to UDP port"

**Problem**: The configured UDP port is already in use.

**Solution**: Change the UDP port in the configuration file or ensure no other application is using the configured port.

#### "SRTP context is not initialized"

**Problem**: Invalid SRTP key/salt configuration.

**Solution**: Ensure proper base64-encoded SRTP key and salt are provided in the configuration.

#### "MySQL connection test failed"

**Problem**: Unable to connect to MySQL database.

**Solution**: Verify MySQL server is running and the connection string in config.json is correct.

### Logs

Karl logs are available in the console output and in the logs directory:

```
logs/karl.log
```

The log level can be configured with the `KARL_LOG_LEVEL` environment variable (1-5, with 5 being most verbose).

### Getting Help

If you encounter issues not covered here:

1. Check the logs for detailed error messages
2. Consult the GitHub repository issues
3. Join the Karl Media Server community forum
4. Open a new issue on GitHub with detailed information

## Contact and Support

- GitHub: [https://github.com/karlmediaserver/karl](https://github.com/karlmediaserver/karl)
- Website: [https://karlmediaserver.io](https://karlmediaserver.io)
- Email: support@karlmediaserver.io