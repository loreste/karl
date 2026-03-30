# Getting Started with Karl Media Server

This guide will help you get Karl running in under 5 minutes.

## Prerequisites

- Go 1.25 or higher (for building from source)
- Docker (optional, for container deployment)
- A SIP proxy (OpenSIPS or Kamailio) for production use

## Option 1: Run from Source

```bash
# Clone the repository
git clone https://github.com/loreste/karl.git
cd karl

# Build
go build -o karl

# Run with default settings
./karl
```

Karl is now running with default configuration:
- NG Protocol: UDP port 22222
- RTP Ports: 30000-40000
- REST API: TCP port 8080
- Health Check: TCP port 8086
- Metrics: TCP port 9091

## Option 2: Run with Docker

```bash
# Run with host networking (recommended for media servers)
docker run -d \
  --name karl \
  --network host \
  loreste/karl:latest
```

Or with explicit port mapping:

```bash
docker run -d \
  --name karl \
  -p 22222:22222/udp \
  -p 30000-30100:30000-30100/udp \
  -p 8080:8080 \
  -p 8086:8086 \
  -p 9091:9091 \
  loreste/karl:latest
```

## Verify Installation

### Test NG Protocol

```bash
# Send a ping command
echo -n "d7:command4:pinge" | nc -u localhost 22222
```

Expected response contains `pong`.

### Check Health Endpoint

```bash
curl http://localhost:8086/health
```

Expected response:
```json
{"status":"UP"}
```

### View Metrics

```bash
curl http://localhost:9091/metrics | head -20
```

## Connect to Your SIP Proxy

### OpenSIPS

Add to your `opensips.cfg`:

```opensips
loadmodule "rtpengine.so"
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")
```

### Kamailio

Add to your `kamailio.cfg`:

```kamailio
loadmodule "rtpengine.so"
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")
```

## Make a Test Call

1. Register two SIP phones with your SIP proxy
2. Make a call between them
3. Verify media flows through Karl:

```bash
# Check active sessions
curl http://localhost:8080/api/v1/sessions
```

## Next Steps

- [Configuration Reference](./configuration.md) - Customize Karl for your environment
- [Kubernetes Deployment](./how-to/deploying-kubernetes.md) - Deploy to Kubernetes
- [Set Up Monitoring](./how-to/monitoring-prometheus.md) - Add Prometheus and Grafana
- [Enable Recording](./how-to/setting-up-recording.md) - Record calls

## Common Issues

### "connection refused" on port 22222

Karl isn't running or is bound to a different interface. Check:

```bash
# Is Karl running?
ps aux | grep karl

# What port is it listening on?
ss -ulnp | grep 22222
```

### No audio in calls

1. Check that RTP ports (30000-40000) are accessible
2. Verify firewall rules allow UDP traffic
3. Check Karl logs for errors:

```bash
# If running as service
journalctl -u karl -f

# If running in Docker
docker logs -f karl
```

See the [Troubleshooting Guide](./how-to/troubleshooting.md) for more help.
