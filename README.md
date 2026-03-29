# Karl Media Server

**A modern, high-performance RTP media proxy built for the cloud era.**

Karl is a drop-in replacement for rtpengine, engineered from the ground up in Go to solve the operational challenges that have plagued VoIP infrastructure teams for years. No more wrestling with kernel modules, no more complex C dependencies, no more deployment headaches.

[![Go Report Card](https://goreportcard.com/badge/github.com/loreste/karl)](https://goreportcard.com/report/github.com/loreste/karl)
[![Go Reference](https://pkg.go.dev/badge/github.com/loreste/karl.svg)](https://pkg.go.dev/github.com/loreste/karl)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL%203.0-blue.svg)](https://opensource.org/licenses/GPL-3.0)

---

## Why Karl?

If you've ever deployed rtpengine in production, you know the pain:

- **Kernel module compilation** that breaks with every OS update
- **Complex dependencies** scattered across multiple repositories
- **Limited observability** making production debugging a nightmare
- **No native cloud support** requiring elaborate workarounds for AWS, GCP, or Kubernetes
- **Sparse documentation** leaving you to reverse-engineer the codebase

**Karl eliminates all of this.** A single binary. Zero kernel dependencies. Full NG protocol compatibility. Deploy in seconds, not hours.

```bash
# That's it. Really.
go build -o karl && ./karl
```

---

## Core Capabilities

### Full NG Protocol Compatibility

Karl implements the complete rtpengine NG protocol, ensuring seamless integration with your existing OpenSIPS or Kamailio infrastructure. No configuration changes needed on your SIP proxy.

| Command | Status | Description |
|---------|--------|-------------|
| `ping` | Supported | Health check and keepalive |
| `offer` | Supported | SDP offer processing with full media negotiation |
| `answer` | Supported | SDP answer processing and session establishment |
| `delete` | Supported | Session termination and cleanup |
| `query` | Supported | Real-time session statistics |
| `list` | Supported | Active call enumeration |
| `start recording` | Supported | Initiate call recording |
| `stop recording` | Supported | Terminate call recording |
| `block media` | Supported | Media flow control |
| `play DTMF` | Supported | DTMF injection |

### Enterprise-Grade Media Handling

- **Adaptive Jitter Buffer**: Dynamic buffering with configurable min/max delay, automatic adjustment based on network conditions
- **Forward Error Correction**: XOR-based FEC with adaptive redundancy (10-50%) based on real-time packet loss metrics
- **RFC 3550 RTCP**: Full implementation including SR/RR reports, SDES, and BYE packets with RTT calculation
- **SRTP/DTLS-SRTP**: Complete encryption support for secure media transport

### WebRTC Native

Karl treats WebRTC as a first-class citizen, not an afterthought:

- **ICE/STUN/TURN**: Full NAT traversal support with ICE-Lite option
- **DTLS-SRTP**: Seamless WebRTC-to-SIP encryption bridging
- **Codec Transcoding**: Opus to G.711 and back, transparent to endpoints
- **Oruds Bandwidth Estimation**: Transport-CC support for adaptive bitrate

### Call Recording

Professional-grade call recording with multiple output modes:

- **Mixed Mode**: Single mono file with both parties
- **Stereo Mode**: Left channel caller, right channel callee
- **Separate Mode**: Individual files per call leg
- **Format Support**: WAV (16-bit PCM) with configurable sample rates
- **Retention Policies**: Automatic cleanup based on configurable retention periods

### Comprehensive REST API

Full programmatic control over every aspect of the media server:

```bash
# List active sessions
curl http://localhost:8080/api/v1/sessions

# Get real-time statistics
curl http://localhost:8080/api/v1/stats

# Control recordings
curl -X POST http://localhost:8080/api/v1/recording/start \
  -d '{"session_id": "abc123", "mode": "stereo"}'
```

### Production Observability

- **Prometheus Metrics**: 50+ metrics covering sessions, RTCP, FEC, jitter buffer, and API performance
- **Health Endpoints**: Kubernetes-ready liveness and readiness probes
- **Structured Logging**: JSON-formatted logs for easy ingestion into ELK/Splunk
- **CDR Generation**: Detailed call records with quality metrics (MOS, jitter, packet loss)

---

## Quick Start

### Prerequisites

- Go 1.21 or higher
- MySQL/MariaDB (optional, for CDR persistence)
- Redis (optional, for distributed session caching)

### Installation

```bash
# Clone and build
git clone https://github.com/loreste/karl.git
cd karl
go build -o karl

# Run with defaults
./karl

# Or with custom config
./karl -config /etc/karl/config.json
```

### Docker

```bash
docker run -d \
  --name karl \
  -p 22222:22222/udp \
  -p 30000-40000:30000-40000/udp \
  -p 8080:8080 \
  -p 9091:9091 \
  loreste/karl:latest
```

### Integration with OpenSIPS

```opensips
# opensips.cfg
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")
```

### Integration with Kamailio

```kamailio
# kamailio.cfg
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")
```

---

## Configuration

Karl uses a JSON configuration file with sensible defaults:

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
  "recording": {
    "enabled": true,
    "base_path": "/var/lib/karl/recordings",
    "format": "wav",
    "mode": "stereo"
  },
  "api": {
    "enabled": true,
    "address": ":8080",
    "auth_enabled": false
  }
}
```

See [DOCUMENTATION.md](./DOCUMENTATION.md) for complete configuration reference.

---

## Performance

Karl is designed for high-throughput environments:

| Metric | Performance |
|--------|-------------|
| Session creation | 1.6M ops/sec |
| Jitter buffer operations | 4.4M ops/sec |
| FEC encoding | 10.3M ops/sec |
| Memory per session | ~1 KB |
| Concurrent sessions | 10,000+ tested |

Benchmarked on Apple M4 / AWS c6g.xlarge equivalent.

---

## Architecture

```
                                    +------------------+
                                    |   Prometheus     |
                                    |   :9091/metrics  |
                                    +--------+---------+
                                             |
+-------------+     NG Protocol      +-------+--------+     RTP/RTCP      +------------+
|  OpenSIPS   | <------------------> |                | <---------------> |  Endpoints |
|  Kamailio   |     UDP:22222        |      Karl      |   UDP:30000-40000 |  (SIP/WebRTC)
+-------------+                      |                |                   +------------+
                                     +-------+--------+
                                             |
                                     +-------+--------+
                                     |   REST API     |
                                     |   :8080        |
                                     +----------------+
```

---

## Monitoring

### Prometheus Metrics

```
# Active sessions
karl_sessions_active

# Session duration histogram
karl_session_duration_seconds

# RTCP round-trip time
karl_rtcp_rtt_seconds

# FEC recovery rate
karl_fec_recoveries_total

# Jitter buffer latency
karl_jitter_buffer_latency_seconds
```

### Health Checks

```bash
# Simple health check
curl http://localhost:8086/health

# Detailed component status
curl http://localhost:8086/health/detail
```

---

## Roadmap

- [ ] SIPREC recording integration
- [ ] T.38 fax support
- [ ] Oruds Gateway (SRTP-to-RTP transcryption)
- [ ] Oruds Clustering with Redis-based session sync
- [ ] Web-based management UI
- [ ] gRPC API

---

## Contributing

Contributions are welcome! Please read our [Contributing Guide](CONTRIBUTING.md) for details on our code of conduct and the process for submitting pull requests.

---

## License

Karl Media Server is licensed under the [GNU General Public License v3.0](LICENSE).

---

## Acknowledgments

Karl builds upon the excellent work of the open-source community:

- [Pion](https://github.com/pion) - WebRTC and RTCP libraries
- [rtpengine](https://github.com/sipwise/rtpengine) - Protocol inspiration and compatibility target

---

**Built with determination for VoIP engineers who deserve better tools.**
