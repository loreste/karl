# Karl Media Server

Karl is a high-performance, production-ready media server designed for handling WebRTC, SIP, RTP, and SRTP communications. It integrates with OpenSIPS, Kamailio, and RTPengine, supporting real-time media routing, transcoding, and security features like DTLS-SRTP.

[![Go Report Card](https://goreportcard.com/badge/github.com/karlmediaserver/karl)](https://goreportcard.com/report/github.com/karlmediaserver/karl)
[![Go Reference](https://pkg.go.dev/badge/github.com/karlmediaserver/karl.svg)](https://pkg.go.dev/github.com/karlmediaserver/karl)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

## 🚀 Features

### 🔹 RTP & SRTP Handling
- Handles RTP and SRTP packets for secure media transmission
- RTP-to-SRTP conversion for interoperability
- Advanced packet loss recovery and jitter buffer optimization
- Robust error handling with metrics

### 🌍 WebRTC Integration
- Supports WebRTC to SIP calls with DTLS-SRTP
- ICE, STUN, TURN support for NAT traversal
- Real-time WebRTC statistics & logging
- WebRTC to external SIP destinations with codec transcoding

### 📡 SIP & SIP Proxy Compatibility
- Fully integrates with OpenSIPS and Kamailio
- SIP NAT handling for external call routing
- Failover mechanism for SIP proxy redundancy
- Priority-based load balancing for SIP trunks

### 🔄 Media Transcoding & Codec Support
- Opus ↔ G.711 transcoding for WebRTC-to-SIP
- Live SDP debugging for call negotiation
- Supports adaptive codec selection for optimal quality

### 🎥 Recording & Monitoring
- Call recording for WebRTC and SIP users
- Real-time media quality monitoring (packet loss, jitter, bandwidth)
- Prometheus metrics & alerting for media health tracking

### 🏗️ Highly Configurable
- Dynamic runtime configuration via JSON and .env files
- API-based config updates with WebSocket notifications
- Web-based UI (upcoming) for managing settings & monitoring

### ☁️ Cloud & NAT Optimizations
- ICE/TURN/STUN support for cloud-based NAT traversal
- Multi-region TURN support for better media relay
- Runs seamlessly on AWS, Google Cloud, and on-prem

### ⚡ Performance & Scalability
- High-performance RTP handling with low-latency processing
- Optimized for high-throughput SIP/WebRTC calls
- Multi-threaded processing for better concurrency

## 📋 Requirements

- Go 1.18 or higher
- MySQL/MariaDB for session tracking
- Redis (optional) for caching
- Prometheus (optional) for metrics collection

## 🛠️ Quick Start

```bash
# Clone the repository
git clone https://github.com/karlmediaserver/karl.git
cd karl

# Build the binary
go build -o karl

# Run with default configuration
./karl
```

See [DOCUMENTATION.md](./DOCUMENTATION.md) for detailed installation and configuration instructions.

## 📊 Monitoring

Karl provides comprehensive metrics via Prometheus. Access them at:

```
http://localhost:9091/metrics
```

## 📃 Documentation

- [Installation Guide](./DOCUMENTATION.md#installation)
- [Configuration Options](./DOCUMENTATION.md#configuration)
- [API Reference](./DOCUMENTATION.md#api-reference)
- [Production Deployment](./PRODUCTION-READY.md)
- [Development Guide](./DOCUMENTATION.md#development)
- [Troubleshooting](./DOCUMENTATION.md#troubleshooting)

## 📝 License

Karl Media Server is licensed under the GPL 3.0 License - see the [LICENSE](LICENSE) file for details.
