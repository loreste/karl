# Installation Guide

This guide covers all installation methods for Karl Media Server.

## Table of Contents

- [System Requirements](#system-requirements)
- [From Source](#from-source)
- [Docker](#docker)
- [Docker Compose](#docker-compose)
- [Kubernetes](#kubernetes)
- [Systemd Service](#systemd-service)
- [Building from Source](#building-from-source)

---

## System Requirements

### Minimum Requirements

| Resource | Minimum | Recommended |
|----------|---------|-------------|
| CPU | 2 cores | 4+ cores |
| RAM | 1 GB | 4+ GB |
| Disk | 100 MB | + recording storage |
| Network | 100 Mbps | 1 Gbps |
| OS | Linux (kernel 4.x+) | Linux (kernel 5.x+) |

### Supported Platforms

- Linux (amd64, arm64)
- macOS (amd64, arm64) - development only
- Windows (amd64) - development only

### Network Requirements

| Port | Protocol | Purpose |
|------|----------|---------|
| 22222 | UDP | NG Protocol (SIP proxy communication) |
| 30000-40000 | UDP | RTP/RTCP media (configurable range) |
| 8080 | TCP | REST API |
| 8086 | TCP | Health checks |
| 9091 | TCP | Prometheus metrics |

---

## From Source

### Prerequisites

- Go 1.25 or higher
- Git

### Steps

```bash
# Clone repository
git clone https://github.com/loreste/karl.git
cd karl

# Download dependencies
go mod download

# Build
go build -o karl

# Verify build
./karl --version
```

### Install System-Wide

```bash
# Copy binary
sudo cp karl /usr/local/bin/

# Create directories
sudo mkdir -p /etc/karl
sudo mkdir -p /var/lib/karl/recordings
sudo mkdir -p /var/run/karl

# Create user
sudo useradd -r -s /bin/false karl

# Set permissions
sudo chown -R karl:karl /var/lib/karl
sudo chown -R karl:karl /var/run/karl

# Copy configuration
sudo cp config/config.json /etc/karl/
sudo chown karl:karl /etc/karl/config.json
```

---

## Docker

### Quick Start

```bash
# Pull latest image
docker pull loreste/karl:latest

# Run with host networking (recommended)
docker run -d \
  --name karl \
  --network host \
  --restart unless-stopped \
  loreste/karl:latest
```

### With Custom Configuration

```bash
# Create config directory
mkdir -p /opt/karl/config
mkdir -p /opt/karl/recordings

# Copy your configuration
cp config.json /opt/karl/config/

# Run with volume mounts
docker run -d \
  --name karl \
  --network host \
  -v /opt/karl/config:/etc/karl:ro \
  -v /opt/karl/recordings:/var/lib/karl/recordings \
  -e KARL_CONFIG_PATH=/etc/karl/config.json \
  --restart unless-stopped \
  loreste/karl:latest
```

### With Port Mapping (Limited RTP Range)

```bash
docker run -d \
  --name karl \
  -p 22222:22222/udp \
  -p 30000-30100:30000-30100/udp \
  -p 8080:8080 \
  -p 8086:8086 \
  -p 9091:9091 \
  -v /opt/karl/config:/etc/karl:ro \
  -v /opt/karl/recordings:/var/lib/karl/recordings \
  --restart unless-stopped \
  loreste/karl:latest
```

### Build Docker Image

```bash
# Clone repository
git clone https://github.com/loreste/karl.git
cd karl

# Build image
docker build -t karl:local .

# Run local image
docker run -d --name karl --network host karl:local
```

---

## Docker Compose

### Basic Setup

Create `docker-compose.yml`:

```yaml
version: '3.8'

services:
  karl:
    image: loreste/karl:latest
    network_mode: host
    volumes:
      - ./config.json:/etc/karl/config.json:ro
      - recordings:/var/lib/karl/recordings
    environment:
      - KARL_CONFIG_PATH=/etc/karl/config.json
      - KARL_LOG_LEVEL=info
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8086/health"]
      interval: 30s
      timeout: 10s
      retries: 3

volumes:
  recordings:
```

### Full Stack with Redis and MySQL

```yaml
version: '3.8'

services:
  karl:
    image: loreste/karl:latest
    network_mode: host
    volumes:
      - ./config.json:/etc/karl/config.json:ro
      - recordings:/var/lib/karl/recordings
    environment:
      - KARL_CONFIG_PATH=/etc/karl/config.json
      - KARL_MYSQL_DSN=karl:password@tcp(127.0.0.1:3306)/karl
      - KARL_REDIS_ENABLED=true
      - KARL_REDIS_ADDR=127.0.0.1:6379
    depends_on:
      mysql:
        condition: service_healthy
      redis:
        condition: service_healthy
    restart: unless-stopped

  mysql:
    image: mariadb:10.11
    environment:
      MYSQL_ROOT_PASSWORD: rootpassword
      MYSQL_DATABASE: karl
      MYSQL_USER: karl
      MYSQL_PASSWORD: password
    volumes:
      - mysql_data:/var/lib/mysql
    ports:
      - "3306:3306"
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "localhost"]
      interval: 10s
      timeout: 5s
      retries: 5
    restart: unless-stopped

  redis:
    image: redis:7-alpine
    volumes:
      - redis_data:/data
    ports:
      - "6379:6379"
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5
    restart: unless-stopped

  prometheus:
    image: prom/prometheus:latest
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml:ro
      - prometheus_data:/prometheus
    ports:
      - "9090:9090"
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.path=/prometheus'
    restart: unless-stopped

  grafana:
    image: grafana/grafana:latest
    volumes:
      - grafana_data:/var/lib/grafana
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
    restart: unless-stopped

volumes:
  recordings:
  mysql_data:
  redis_data:
  prometheus_data:
  grafana_data:
```

### Start the Stack

```bash
# Start all services
docker-compose up -d

# View logs
docker-compose logs -f karl

# Stop all services
docker-compose down
```

---

## Kubernetes

See the dedicated [Kubernetes Deployment Guide](./how-to/deploying-kubernetes.md) for complete instructions.

### Quick Deploy

```bash
# Clone repository
git clone https://github.com/loreste/karl.git
cd karl

# Deploy using kustomize
kubectl apply -k deploy/kubernetes/

# Check status
kubectl get pods -l app=karl
```

---

## Systemd Service

### Create Service File

Create `/etc/systemd/system/karl.service`:

```ini
[Unit]
Description=Karl Media Server
Documentation=https://github.com/loreste/karl
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=karl
Group=karl
ExecStart=/usr/local/bin/karl -config /etc/karl/config.json
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/karl /var/run/karl
PrivateTmp=true

# Resource limits
LimitNOFILE=65535
LimitNPROC=4096

[Install]
WantedBy=multi-user.target
```

### Enable and Start

```bash
# Reload systemd
sudo systemctl daemon-reload

# Enable service
sudo systemctl enable karl

# Start service
sudo systemctl start karl

# Check status
sudo systemctl status karl

# View logs
sudo journalctl -u karl -f
```

---

## Building from Source

### Development Build

```bash
# Clone
git clone https://github.com/loreste/karl.git
cd karl

# Install dependencies
go mod download

# Run tests
go test ./...

# Build with debug symbols
go build -o karl

# Run
./karl
```

### Production Build

```bash
# Build optimized binary
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -ldflags="-s -w" \
  -o karl

# Verify binary
file karl
```

### Cross-Compilation

```bash
# Linux ARM64
GOOS=linux GOARCH=arm64 go build -o karl-linux-arm64

# macOS ARM64
GOOS=darwin GOARCH=arm64 go build -o karl-darwin-arm64

# Windows
GOOS=windows GOARCH=amd64 go build -o karl.exe
```

---

## Verify Installation

After installing Karl using any method:

### 1. Check Process

```bash
# Using systemctl
sudo systemctl status karl

# Using Docker
docker ps | grep karl

# Using ps
ps aux | grep karl
```

### 2. Test NG Protocol

```bash
echo -n "d7:command4:pinge" | nc -u localhost 22222
```

### 3. Check Health

```bash
curl http://localhost:8086/health
```

### 4. View Metrics

```bash
curl http://localhost:9091/metrics | head
```

---

## Next Steps

- [Configuration Reference](./configuration.md)
- [Integrate with OpenSIPS](./how-to/integrating-opensips.md)
- [Integrate with Kamailio](./how-to/integrating-kamailio.md)
- [Set Up Monitoring](./how-to/monitoring-prometheus.md)
