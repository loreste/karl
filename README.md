# Karl Media Server

## Open Source RTP Proxy and Media Server for VoIP and WebRTC

[![Go Report Card](https://goreportcard.com/badge/github.com/loreste/karl)](https://goreportcard.com/report/github.com/loreste/karl)
[![Go Reference](https://pkg.go.dev/badge/github.com/loreste/karl.svg)](https://pkg.go.dev/github.com/loreste/karl)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL%203.0-blue.svg)](https://opensource.org/licenses/GPL-3.0)

Karl is a high-performance, cloud-native RTP media proxy and media server written in Go. It serves as a modern, drop-in replacement for rtpengine, designed specifically for cloud deployments, Kubernetes environments, and modern VoIP infrastructure.

**Key highlights:**
- Single binary deployment with zero kernel dependencies
- Full rtpengine NG protocol compatibility
- Native Kubernetes support with health probes
- WebRTC-to-SIP bridging out of the box
- Prometheus metrics and production-grade observability

---

## Table of Contents

- [The Problem We Solve](#the-problem-we-solve)
- [Why Choose Karl](#why-choose-karl)
- [Features](#features)
- [Quick Start](#quick-start)
- [Documentation](#documentation)
- [Installation](#installation)
- [Configuration](#configuration)
- [Kubernetes Deployment](#kubernetes-deployment)
- [Integration with SIP Proxies](#integration-with-sip-proxies)
- [API Reference](#api-reference)
- [Monitoring and Observability](#monitoring-and-observability)
- [Performance](#performance)
- [Architecture](#architecture)
- [Roadmap](#roadmap)
- [Contributing](#contributing)
- [License](#license)

---

## The Problem We Solve

### The Challenge with Traditional RTP Proxies

Running media servers in production VoIP environments has traditionally been painful. If you've operated rtpengine or similar RTP proxies, you've likely encountered these challenges:

**Kernel Module Dependencies**
Traditional RTP proxies require kernel modules for performance. These modules break with every kernel update, require recompilation, and are incompatible with containerized environments. Running them on managed Kubernetes services like EKS, GKE, or AKS is either impossible or requires privileged containers with host access.

**Complex Deployment**
Installing rtpengine means managing multiple repositories, compiling kernel modules, resolving C library dependencies, and maintaining complex build pipelines. A simple upgrade can take hours of downtime.

**Limited Cloud Compatibility**
Most RTP proxies were designed for bare-metal servers. They struggle with:
- Dynamic IP addresses in cloud environments
- Auto-scaling groups and container orchestration
- NAT traversal in VPC networks
- Load balancer health checks

**Poor Observability**
Debugging media quality issues in production requires deep visibility into packet loss, jitter, and codec performance. Traditional tools offer limited metrics and no native integration with modern observability stacks like Prometheus and Grafana.

**WebRTC as an Afterthought**
As WebRTC adoption grows, bridging browser-based calls to traditional SIP infrastructure requires complex configuration and often separate components.

### How Karl Solves These Problems

Karl was built from the ground up to address every one of these challenges:

| Challenge | Traditional RTP Proxy | Karl |
|-----------|----------------------|------|
| Kernel modules | Required, breaks on updates | None required |
| Deployment | Hours of setup | Single binary, seconds to deploy |
| Cloud/Kubernetes | Workarounds needed | Native support |
| Observability | Limited | 50+ Prometheus metrics |
| WebRTC | Separate component | Built-in |
| Container support | Difficult | First-class citizen |

---

## Why Choose Karl

### Zero Kernel Dependencies

Karl runs entirely in userspace. No kernel modules to compile, no privileged containers, no compatibility issues with managed Kubernetes services. Deploy on any Linux distribution, any cloud provider, any container orchestrator.

```bash
# That's the entire installation
go build -o karl && ./karl
```

### Full rtpengine Compatibility

Karl implements the complete NG (Next Generation) protocol used by rtpengine. Your existing OpenSIPS or Kamailio configuration works without modification. No changes to your SIP proxy, no migration complexity.

### Cloud-Native Architecture

Built for modern infrastructure:
- **Kubernetes-native health probes**: Startup, liveness, and readiness endpoints
- **Environment variable configuration**: Easy integration with ConfigMaps and Secrets
- **Horizontal scaling**: Redis-backed session sharing for multi-instance deployments
- **Graceful shutdown**: Proper SIGTERM handling for zero-downtime deployments

### Production-Grade Observability

Know exactly what's happening in your media infrastructure:
- **50+ Prometheus metrics** covering sessions, RTCP quality, FEC recovery, and API performance
- **Structured JSON logging** for easy ingestion into ELK, Splunk, or CloudWatch
- **Real-time quality metrics** including MOS scores, jitter, and packet loss per call
- **Call Detail Records (CDR)** with full quality statistics

### WebRTC Native

WebRTC isn't bolted on—it's a core feature:
- **ICE/STUN/TURN** support for NAT traversal
- **DTLS-SRTP** encryption bridging between WebRTC and SIP
- **Opus codec transcoding** to G.711 and back
- **Bandwidth estimation** with Transport-CC support

---

## Features

### NG Protocol Commands

Full compatibility with rtpengine NG protocol:

| Command | Description |
|---------|-------------|
| `ping` | Health check and keepalive |
| `offer` | Process SDP offer, allocate media ports |
| `answer` | Process SDP answer, complete session setup |
| `delete` | Terminate session, release resources |
| `query` | Get real-time session statistics |
| `list` | Enumerate all active calls |
| `start recording` | Begin call recording |
| `stop recording` | End call recording |
| `block media` | Mute/unmute media streams |
| `play DTMF` | Inject DTMF tones |

### Media Processing

- **Adaptive Jitter Buffer**: Dynamic buffering (20-200ms) with automatic adjustment based on network conditions
- **Forward Error Correction**: XOR-based FEC with adaptive redundancy (10-50%) based on real-time packet loss
- **RTCP Processing**: Full RFC 3550 implementation with SR/RR reports, RTT calculation, and quality metrics
- **SRTP/DTLS-SRTP**: Complete encryption support for secure media transport
- **Codec Support**: G.711 (PCMU/PCMA), G.722, G.729, Opus, AMR/AMR-WB, iLBC, Speex with transparent transcoding (pure Go implementation, no CGO required)
- **T.38 Fax**: Full T.38 fax passthrough and gateway mode with V.21 tone detection
- **SIPREC**: RFC 7865/7866 compliant session recording

### Call Recording

Professional recording capabilities:

| Mode | Description |
|------|-------------|
| Mixed | Single mono file with both parties |
| Stereo | Left channel caller, right channel callee |
| Separate | Individual files per call leg |
| SIPREC | RFC 7865/7866 compliant session recording |

- **Format**: WAV (16-bit PCM) at configurable sample rates
- **Storage**: Local filesystem or network storage
- **Retention**: Automatic cleanup based on configurable policies
- **Failover**: Recording continuity across node failures

### Clustering & High Availability

Enterprise-grade clustering support:

- **Redis-backed session state**: Distributed session sharing across nodes
- **Consistent hashing**: Sticky session placement with failover
- **Split-brain detection**: Quorum-based partition tolerance
- **Port re-allocation**: Consistent port assignment during failover
- **CDR coordination**: Distributed call detail record aggregation
- **Proxy notification**: Automatic SIP proxy notification on failover

### Security

Comprehensive security features:

- **TLS/HTTPS**: Secure API and management interfaces
- **Authentication**: API key and token-based authentication
- **Authorization**: Role-based access control for operations
- **Rate limiting**: Configurable per-IP and per-call rate limits
- **DoS protection**: Automatic blocking of abusive sources
- **Input validation**: Strict validation of all protocol inputs
- **Secrets management**: Secure handling of credentials

### REST API

Programmatic control over all server functions:

```bash
# List active sessions
curl http://localhost:8080/api/v1/sessions

# Get server statistics
curl http://localhost:8080/api/v1/stats

# Start recording a call
curl -X POST http://localhost:8080/api/v1/recording/start \
  -H "Content-Type: application/json" \
  -d '{"session_id": "abc123", "mode": "stereo"}'

# Stop recording
curl -X POST http://localhost:8080/api/v1/recording/stop \
  -d '{"session_id": "abc123"}'
```

---

## Quick Start

### Prerequisites

- Go 1.25 or higher
- MySQL/MariaDB (optional, for CDR storage)
- Redis (optional, for distributed session caching)

### Run in 30 Seconds

```bash
# Clone the repository
git clone https://github.com/loreste/karl.git
cd karl

# Build and run
go build -o karl
./karl
```

Karl is now listening on:
- UDP port 22222 for NG protocol (SIP proxy communication)
- UDP ports 30000-40000 for RTP media
- TCP port 8080 for REST API
- TCP port 8086 for health checks
- TCP port 9091 for Prometheus metrics

### Test the Connection

```bash
# Ping test
echo -n "d7:command4:pinge" | nc -u localhost 22222
```

---

## Documentation

Comprehensive documentation is available in the [`docs/`](./docs) directory.

### Getting Started

- [Quick Start Guide](./docs/getting-started.md) - Get running in 5 minutes
- [Installation Guide](./docs/installation.md) - Detailed installation options
- [Configuration Reference](./docs/configuration.md) - All configuration options

### How-To Guides

Step-by-step guides for common tasks:

| Guide | Description |
|-------|-------------|
| [Deploy on Kubernetes](./docs/how-to/deploying-kubernetes.md) | Production K8s deployment with probes, scaling, and monitoring |
| [Integrate with OpenSIPS](./docs/how-to/integrating-opensips.md) | Connect Karl to OpenSIPS with NAT, WebRTC, and recording |
| [Integrate with Kamailio](./docs/how-to/integrating-kamailio.md) | Connect Karl to Kamailio with complete examples |
| [Set Up Call Recording](./docs/how-to/setting-up-recording.md) | Configure recording modes, storage, and retention |
| [Monitor with Prometheus](./docs/how-to/monitoring-prometheus.md) | Metrics, Grafana dashboards, and alerting rules |
| [Scale Horizontally](./docs/how-to/scaling-horizontally.md) | Redis clustering and load balancing |
| [Bridge WebRTC to SIP](./docs/how-to/webrtc-sip-bridging.md) | Connect browser clients to SIP infrastructure |
| [Secure with TLS](./docs/how-to/securing-with-tls.md) | HTTPS for API and management interfaces |
| [Troubleshooting](./docs/how-to/troubleshooting.md) | Diagnose and fix common issues |

### Reference

- [NG Protocol Reference](./docs/reference/ng-protocol.md) - Complete protocol specification
- [Environment Variables](./docs/reference/environment-variables.md) - All supported variables

---

## Installation

### From Source

```bash
git clone https://github.com/loreste/karl.git
cd karl
go build -o karl
sudo mv karl /usr/local/bin/
```

### Docker

```bash
docker run -d \
  --name karl \
  --network host \
  loreste/karl:latest
```

Or with port mapping (limited RTP port range):

```bash
docker run -d \
  --name karl \
  -p 22222:22222/udp \
  -p 30000-30100:30000-30100/udp \
  -p 8080:8080 \
  -p 8086:8086 \
  -p 9091:9091 \
  -v /path/to/config.json:/etc/karl/config.json \
  -v /path/to/recordings:/var/lib/karl/recordings \
  loreste/karl:latest
```

### Docker Compose

```yaml
version: '3.8'
services:
  karl:
    image: loreste/karl:latest
    network_mode: host
    volumes:
      - ./config.json:/etc/karl/config.json
      - ./recordings:/var/lib/karl/recordings
    environment:
      - KARL_CONFIG_PATH=/etc/karl/config.json
      - KARL_LOG_LEVEL=info
    restart: unless-stopped
```

---

## Configuration

Karl uses a JSON configuration file with sensible defaults. All settings can also be overridden via environment variables.

### Configuration File

```json
{
  "ng_protocol": {
    "enabled": true,
    "udp_port": 22222,
    "timeout": 30
  },
  "sessions": {
    "max_sessions": 10000,
    "session_ttl": 3600,
    "min_port": 30000,
    "max_port": 40000
  },
  "jitter_buffer": {
    "enabled": true,
    "min_delay": 20,
    "max_delay": 200,
    "adaptive_mode": true
  },
  "fec": {
    "enabled": true,
    "redundancy": 0.30,
    "adaptive_mode": true
  },
  "recording": {
    "enabled": true,
    "base_path": "/var/lib/karl/recordings",
    "format": "wav",
    "mode": "stereo",
    "retention_days": 30
  },
  "api": {
    "enabled": true,
    "address": ":8080",
    "auth_enabled": false
  },
  "webrtc": {
    "enabled": true,
    "stun_servers": ["stun:stun.l.google.com:19302"]
  },
  "database": {
    "mysql_dsn": "",
    "redis_enabled": false,
    "redis_addr": "redis:6379"
  }
}
```

### Environment Variables

All configuration can be set via environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `KARL_CONFIG_PATH` | Path to configuration file | `config/config.json` |
| `KARL_HEALTH_PORT` | Health check endpoint port | `:8086` |
| `KARL_METRICS_PORT` | Prometheus metrics port | `:9091` |
| `KARL_API_PORT` | REST API port | `:8080` |
| `KARL_NG_PORT` | NG protocol UDP port | `22222` |
| `KARL_RTP_MIN_PORT` | RTP port range start | `30000` |
| `KARL_RTP_MAX_PORT` | RTP port range end | `40000` |
| `KARL_MAX_SESSIONS` | Maximum concurrent sessions | `10000` |
| `KARL_RECORDING_PATH` | Recording storage path | `/var/lib/karl/recordings` |
| `KARL_RECORDING_ENABLED` | Enable call recording | `true` |
| `KARL_MYSQL_DSN` | MySQL connection string | (empty) |
| `KARL_REDIS_ADDR` | Redis server address | (empty) |
| `KARL_REDIS_ENABLED` | Enable Redis session cache | `false` |
| `KARL_MEDIA_IP` | Media IP address | `auto` |
| `KARL_PUBLIC_IP` | Public IP for SDP | (auto-detected) |

---

## Kubernetes Deployment

Karl is designed for Kubernetes from the ground up. Complete manifests are provided in the `deploy/kubernetes/` directory.

### Quick Deploy

```bash
kubectl apply -k deploy/kubernetes/
```

### Health Probes

Karl exposes Kubernetes-native probe endpoints:

| Probe | Endpoint | Purpose |
|-------|----------|---------|
| Startup | `/startup` | Wait for initialization (up to 150s) |
| Liveness | `/live` | Detect deadlocks, trigger restart |
| Readiness | `/ready` | Check if ready for traffic |

### Deployment Architecture

The default deployment uses `hostNetwork: true` for optimal RTP performance:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: karl
spec:
  replicas: 1
  template:
    spec:
      hostNetwork: true
      containers:
      - name: karl
        image: loreste/karl:latest
        ports:
        - containerPort: 22222
          protocol: UDP
        - containerPort: 8080
          protocol: TCP
        - containerPort: 8086
          protocol: TCP
        - containerPort: 9091
          protocol: TCP
        livenessProbe:
          httpGet:
            path: /live
            port: 8086
          periodSeconds: 15
        readinessProbe:
          httpGet:
            path: /ready
            port: 8086
          periodSeconds: 5
        startupProbe:
          httpGet:
            path: /startup
            port: 8086
          failureThreshold: 30
          periodSeconds: 5
```

### Scaling

For horizontal scaling, enable Redis for session sharing:

1. Deploy Redis (or use managed Redis)
2. Set `KARL_REDIS_ENABLED=true` and `KARL_REDIS_ADDR=redis:6379`
3. Increase deployment replicas

---

## Integration with SIP Proxies

### OpenSIPS

```opensips
# opensips.cfg
loadmodule "rtpengine.so"
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")

route {
    # ... your routing logic ...

    if (has_body("application/sdp")) {
        rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
    }
}
```

### Kamailio

```kamailio
# kamailio.cfg
loadmodule "rtpengine.so"
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")

request_route {
    # ... your routing logic ...

    if (has_body("application/sdp")) {
        rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
    }
}
```

### FreeSWITCH

Use the `mod_rtpengine` module with:

```xml
<param name="rtpengine-ip" value="127.0.0.1"/>
<param name="rtpengine-port" value="22222"/>
```

---

## API Reference

### Sessions

**List all sessions**
```bash
GET /api/v1/sessions
```

**Get session details**
```bash
GET /api/v1/sessions/{session_id}
```

**Delete a session**
```bash
DELETE /api/v1/sessions/{session_id}
```

### Statistics

**Get server statistics**
```bash
GET /api/v1/stats
```

Response:
```json
{
  "active_sessions": 150,
  "total_sessions": 10542,
  "packets_processed": 15234567,
  "packets_dropped": 234,
  "uptime_seconds": 86400
}
```

### Recording

**Start recording**
```bash
POST /api/v1/recording/start
Content-Type: application/json

{
  "session_id": "abc123",
  "mode": "stereo"
}
```

**Stop recording**
```bash
POST /api/v1/recording/stop
Content-Type: application/json

{
  "session_id": "abc123"
}
```

### Health

**Simple health check**
```bash
GET /health
```

**Detailed health status**
```bash
GET /health/detail
```

---

## Monitoring and Observability

### Prometheus Metrics

Karl exposes comprehensive metrics at `:9091/metrics`:

**Session Metrics**
```
karl_sessions_active          # Current active sessions
karl_sessions_total           # Total sessions created
karl_session_duration_seconds # Session duration histogram
```

**Media Quality Metrics**
```
karl_rtcp_rtt_seconds         # RTCP round-trip time
karl_rtcp_jitter_seconds      # Reported jitter
karl_rtcp_packet_loss_ratio   # Packet loss percentage
karl_fec_recoveries_total     # Packets recovered via FEC
karl_jitter_buffer_latency    # Jitter buffer delay
```

**NG Protocol Metrics**
```
karl_ng_commands_total        # Commands processed by type
karl_ng_command_duration      # Command processing time
karl_ng_active_calls          # Current call count
```

### Grafana Dashboard

Import the provided dashboard from `deploy/grafana/karl-dashboard.json` for:
- Real-time session monitoring
- Media quality visualization
- Resource utilization tracking
- Alert configuration

### Health Endpoints

| Endpoint | Purpose | Response |
|----------|---------|----------|
| `/health` | Simple status | `{"status":"UP"}` |
| `/health/detail` | Component status | Full component breakdown |
| `/live` | Kubernetes liveness | 200 if alive |
| `/ready` | Kubernetes readiness | 200 if ready for traffic |
| `/startup` | Kubernetes startup | 200 when initialized |

---

## Performance

Karl is optimized for high-throughput media processing:

| Metric | Performance |
|--------|-------------|
| Session creation | 4M operations/second |
| Jitter buffer operations | 4.4M operations/second |
| FEC encoding | 10.3M operations/second |
| G.711 transcoding | 3.3M operations/second |
| iLBC encoding | 1.3M operations/second |
| Buffer pool operations | 58M operations/second |
| Memory per session | ~624 bytes |
| Tested concurrent sessions | 10,000+ |

### Performance Features

- **Zero-copy forwarding**: Minimized memory copies in fast path
- **Socket sharding**: Per-core sockets for scalability
- **Buffer pooling**: Reusable buffers reduce GC pressure
- **Worker pools**: Efficient concurrent packet processing
- **Batch RTCP**: Aggregated RTCP processing
- **Async recording**: Non-blocking recording writes

### Resource Recommendations

| Concurrent Calls | CPU | Memory |
|-----------------|-----|--------|
| < 100 | 250m | 256Mi |
| 100-500 | 500m | 512Mi |
| 500-1000 | 1000m | 1Gi |
| > 1000 | 2000m | 2Gi |

---

## Architecture

```
                                    ┌──────────────────┐
                                    │   Prometheus     │
                                    │   :9091/metrics  │
                                    └────────┬─────────┘
                                             │
┌─────────────┐     NG Protocol      ┌───────┴────────┐     RTP/RTCP      ┌────────────┐
│  OpenSIPS   │ ◄──────────────────► │                │ ◄───────────────► │  Endpoints │
│  Kamailio   │     UDP:22222        │      Karl      │   UDP:30000-40000 │ (SIP/WebRTC)│
└─────────────┘                      │                │                   └────────────┘
                                     └───────┬────────┘
                                             │
                                     ┌───────┴────────┐
                                     │   REST API     │
                                     │   :8080        │
                                     └────────────────┘
```

### Component Overview

- **NG Protocol Handler**: Processes commands from SIP proxies
- **Session Manager**: Tracks call state and media allocations
- **RTP Forwarder**: High-performance packet routing with worker pools
- **Jitter Buffer**: Adaptive buffering for smooth playback
- **FEC Handler**: Forward error correction for lossy networks
- **RTCP Handler**: Quality monitoring and statistics
- **Recording Manager**: Call recording with multiple output modes
- **WebRTC Bridge**: ICE/DTLS/SRTP for browser integration

---

## Roadmap

Karl is a full-featured rtpengine replacement. See the detailed [ROADMAP.md](./ROADMAP.md) for implementation details.

### Phase 1: Control-Plane Parity ✅ Complete
- [x] Complete NG protocol flag support (100+ flags)
- [x] Behavioral semantics parity
- [x] Response format compatibility

### Phase 2: Relay-Grade Media ✅ Complete
- [x] NAT/interface logic parity
- [x] IPv4↔IPv6 bridging
- [x] ICE-full and ICE-lite modes
- [x] Media fast path with zero-copy forwarding

### Phase 3: Enterprise Features ✅ Complete
- [x] SIPREC recording integration (RFC 7865/7866)
- [x] T.38 fax passthrough with V.21 detection
- [x] SRTP↔RTP gateway mode
- [x] Multi-node clustering with Redis backend
- [x] Split-brain detection and failover

### Phase 4: Operational Maturity ✅ Complete
- [x] Performance engineering (buffer pools, socket sharding)
- [x] Comprehensive test suite with race detection
- [x] Security hardening (TLS, authentication, rate limiting)
- [x] Memory leak and GC leak testing

### Phase 5: Testing & Validation (Ongoing)
- [x] Unit tests for core components
- [ ] Integration tests with SIP proxies
- [ ] Chaos testing infrastructure

---

## Contributing

Contributions are welcome! Please see our [Contributing Guide](CONTRIBUTING.md) for:

- Code style guidelines
- Pull request process
- Development setup
- Testing requirements

### Building from Source

```bash
git clone https://github.com/loreste/karl.git
cd karl
go mod download
go build -o karl
go test ./...
```

---

## License

Karl Media Server is licensed under the [GNU General Public License v3.0](LICENSE).

---

## Acknowledgments

Karl builds on the work of the open-source community:

- [Pion](https://github.com/pion) - WebRTC, DTLS, SRTP, and RTP libraries
- [rtpengine](https://github.com/sipwise/rtpengine) - Protocol specification and compatibility reference

---

## Support

- **Documentation**: [Full documentation](./docs/README.md)
- **How-To Guides**: [Step-by-step guides](./docs/how-to)
- **Issues**: [GitHub Issues](https://github.com/loreste/karl/issues)
- **Discussions**: [GitHub Discussions](https://github.com/loreste/karl/discussions)

---

**Karl Media Server** — Built for VoIP engineers who need reliable, observable, cloud-native media infrastructure.
