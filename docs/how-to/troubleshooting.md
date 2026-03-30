# Troubleshooting Guide

This guide covers diagnosing and resolving common issues with Karl Media Server.

## Table of Contents

- [Diagnostic Tools](#diagnostic-tools)
- [Startup Issues](#startup-issues)
- [Connection Issues](#connection-issues)
- [Media Issues](#media-issues)
- [Recording Issues](#recording-issues)
- [Performance Issues](#performance-issues)
- [Kubernetes Issues](#kubernetes-issues)
- [Getting Help](#getting-help)

---

## Diagnostic Tools

### Check Karl Status

```bash
# Process running
ps aux | grep karl

# Systemd status
systemctl status karl

# Docker status
docker ps | grep karl
docker logs karl

# Kubernetes status
kubectl get pods -l app=karl
kubectl describe pod -l app=karl
```

### Test NG Protocol

```bash
# Send ping command
echo -n "d7:command4:pinge" | nc -u localhost 22222

# Expected response contains "pong"
```

### Check Health Endpoints

```bash
# Simple health
curl http://localhost:8086/health

# Detailed health
curl http://localhost:8086/health/detail

# Kubernetes probes
curl http://localhost:8086/startup
curl http://localhost:8086/live
curl http://localhost:8086/ready
```

### View Metrics

```bash
# All metrics
curl http://localhost:9091/metrics

# Active sessions
curl -s http://localhost:9091/metrics | grep karl_sessions_active
```

### Check Logs

```bash
# Systemd
journalctl -u karl -f

# Docker
docker logs -f karl

# Kubernetes
kubectl logs -l app=karl -f

# With debug level
KARL_LOG_LEVEL=debug ./karl
```

### API Queries

```bash
# List sessions
curl http://localhost:8080/api/v1/sessions

# Server stats
curl http://localhost:8080/api/v1/stats

# Specific session
curl http://localhost:8080/api/v1/sessions/{session_id}
```

---

## Startup Issues

### "bind: address already in use"

Another process is using the port.

**Solution**:
```bash
# Find the process
sudo lsof -i :22222
sudo lsof -i :8080

# Kill if necessary
sudo kill <PID>

# Or change Karl's ports
KARL_NG_PORT=22223 ./karl
```

### "permission denied" for socket

Insufficient permissions for Unix socket or ports.

**Solution**:
```bash
# Create directory with correct permissions
sudo mkdir -p /var/run/karl
sudo chown karl:karl /var/run/karl

# For ports below 1024
sudo setcap 'cap_net_bind_service=+ep' /usr/local/bin/karl
```

### "too many open files"

File descriptor limit too low.

**Solution**:
```bash
# Temporary fix
ulimit -n 65535

# Permanent fix - add to /etc/security/limits.conf
karl soft nofile 65535
karl hard nofile 65535

# Or in systemd service
[Service]
LimitNOFILE=65535
```

### Configuration File Not Found

```bash
# Check file exists
ls -la /etc/karl/config.json

# Verify path
KARL_CONFIG_PATH=/etc/karl/config.json ./karl

# Use default config
./karl  # Uses ./config/config.json
```

### Invalid Configuration

```bash
# Validate JSON
cat /etc/karl/config.json | jq .

# Check for common issues
# - Missing commas
# - Trailing commas
# - Invalid values
```

---

## Connection Issues

### SIP Proxy Can't Connect

**Symptoms**: No response to NG protocol commands

**Diagnosis**:
```bash
# Check Karl is listening
ss -ulnp | grep 22222

# Test from SIP proxy
echo -n "d7:command4:pinge" | nc -u karl-ip 22222

# Check firewall
sudo iptables -L -n | grep 22222
```

**Solutions**:
1. Verify Karl is running and listening on correct interface
2. Check firewall rules allow UDP 22222
3. Verify SIP proxy configuration points to correct IP/port

### API Not Responding

**Diagnosis**:
```bash
# Check API is listening
ss -tlnp | grep 8080

# Test locally
curl http://localhost:8080/api/v1/stats

# Check from remote
curl http://karl-ip:8080/api/v1/stats
```

**Solutions**:
1. Verify API is enabled in config
2. Check firewall for TCP 8080
3. Verify bind address (use `0.0.0.0` for all interfaces)

### Redis Connection Failed

**Symptoms**: Sessions not shared between instances

**Diagnosis**:
```bash
# Test Redis connectivity
redis-cli -h redis-host ping

# Check Karl logs
grep -i redis /var/log/karl.log

# Verify config
grep -A 5 '"database"' /etc/karl/config.json
```

**Solutions**:
1. Verify Redis is running
2. Check network connectivity
3. Verify Redis address in configuration
4. Check Redis authentication if enabled

---

## Media Issues

### No Audio (Both Directions)

**Diagnosis**:
```bash
# Check sessions are created
curl http://localhost:8080/api/v1/sessions

# Verify RTP ports are allocated
ss -ulnp | grep -E "3[0-9]{4}"

# Check packet counters
curl http://localhost:9091/metrics | grep packets
```

**Common Causes**:
1. RTP ports blocked by firewall
2. Incorrect media_ip configuration
3. NAT issues

**Solutions**:
```bash
# Open RTP ports
sudo iptables -A INPUT -p udp --dport 30000:40000 -j ACCEPT

# Check media_ip
KARL_MEDIA_IP=192.168.1.100 ./karl

# For NAT, set public_ip
KARL_PUBLIC_IP=203.0.113.50 ./karl
```

### One-Way Audio

**Common Causes**:
1. Asymmetric NAT
2. Firewall blocking return traffic
3. Incorrect SDP manipulation

**Solutions**:
```bash
# In SIP proxy, force symmetric RTP
rtpengine_manage("... symmetric");

# Check both directions have packets
curl http://localhost:8080/api/v1/sessions/{id}
# Verify packets_sent and packets_received > 0
```

### Poor Audio Quality

**Diagnosis**:
```bash
# Check RTCP metrics
curl http://localhost:9091/metrics | grep rtcp

# Look for:
# - High packet loss (> 5%)
# - High jitter (> 50ms)
# - High RTT (> 200ms)
```

**Solutions**:
1. Enable/increase FEC redundancy
2. Increase jitter buffer size
3. Check network path for issues

```json
{
  "fec": {
    "enabled": true,
    "redundancy": 0.30,
    "adaptive_mode": true
  },
  "jitter_buffer": {
    "enabled": true,
    "max_delay": 200,
    "adaptive_mode": true
  }
}
```

### Sessions Not Created

**Diagnosis**:
```bash
# Check NG protocol logs
grep -i "offer\|answer" /var/log/karl.log

# Verify SDP is being sent
tcpdump -i any port 22222 -A
```

**Common Causes**:
1. Invalid SDP in request
2. Missing required parameters
3. Session limit reached

**Solutions**:
1. Check SIP proxy is sending complete requests
2. Verify call-id and from-tag are present
3. Increase max_sessions if needed

---

## Recording Issues

### Recordings Not Created

**Diagnosis**:
```bash
# Check recording is enabled
grep -A 10 '"recording"' /etc/karl/config.json

# Verify directory exists and is writable
ls -la /var/lib/karl/recordings
touch /var/lib/karl/recordings/test.txt

# Check for recording messages
grep -i recording /var/log/karl.log
```

**Solutions**:
1. Enable recording in configuration
2. Create directory with correct permissions
3. Ensure `record-call` flag is sent by SIP proxy

### Empty Recording Files

**Causes**:
1. No media packets received
2. Codec not supported
3. Session ended too quickly

**Diagnosis**:
```bash
# Check session had media
curl http://localhost:8080/api/v1/sessions/{id}
# Verify packets_received > 0

# Check codec
grep -i codec /var/log/karl.log
```

### Disk Space Issues

```bash
# Check disk usage
df -h /var/lib/karl/recordings

# Find large files
du -sh /var/lib/karl/recordings/*

# Enable retention policy
{
  "recording": {
    "retention_days": 7
  }
}

# Manual cleanup
find /var/lib/karl/recordings -name "*.wav" -mtime +7 -delete
```

---

## Performance Issues

### High CPU Usage

**Diagnosis**:
```bash
# Check process CPU
top -p $(pgrep karl)

# Profile (if built with profiling)
curl http://localhost:9091/debug/pprof/profile?seconds=30 > profile.pb.gz
```

**Common Causes**:
1. Too many concurrent sessions
2. FEC overhead too high
3. Insufficient resources

**Solutions**:
1. Scale horizontally
2. Reduce FEC redundancy
3. Increase CPU allocation

### High Memory Usage

**Diagnosis**:
```bash
# Check memory
ps aux | grep karl

# Memory metrics
curl http://localhost:9091/metrics | grep memory
```

**Solutions**:
1. Reduce max_sessions
2. Decrease jitter buffer size
3. Add memory limits and let orchestrator handle restarts

### Slow Session Creation

**Diagnosis**:
```bash
# Check command latency
curl http://localhost:9091/metrics | grep ng_command_duration
```

**Solutions**:
1. Check Redis latency (if using Redis)
2. Reduce port allocation range
3. Scale horizontally

---

## Kubernetes Issues

### Pod Not Starting

```bash
# Check pod status
kubectl describe pod -l app=karl

# Common issues:
# - Image pull errors
# - Resource limits
# - Probe failures
# - Volume mount issues
```

### Probes Failing

```bash
# Test probes manually
kubectl exec -it <pod> -- curl localhost:8086/startup
kubectl exec -it <pod> -- curl localhost:8086/live
kubectl exec -it <pod> -- curl localhost:8086/ready

# Check probe configuration
kubectl get pod <pod> -o yaml | grep -A 10 "Probe"
```

### No Network Connectivity

```bash
# Check service endpoints
kubectl get endpoints karl

# Test from within cluster
kubectl run test --rm -it --image=alpine -- sh
apk add curl netcat-openbsd
nc -u karl 22222

# Check network policies
kubectl get networkpolicies
```

### hostNetwork Issues

```bash
# Verify hostNetwork is enabled
kubectl get pod <pod> -o yaml | grep hostNetwork

# Check node IP
kubectl get pod <pod> -o wide

# Verify no port conflicts on node
kubectl exec -it <pod> -- ss -ulnp
```

---

## Getting Help

### Information to Gather

When reporting issues, include:

1. **Karl version**
```bash
./karl --version
```

2. **Configuration** (sanitize sensitive data)
```bash
cat /etc/karl/config.json
```

3. **Logs**
```bash
journalctl -u karl --since "1 hour ago" > karl-logs.txt
```

4. **Metrics**
```bash
curl http://localhost:9091/metrics > metrics.txt
```

5. **Environment**
```bash
uname -a
cat /etc/os-release
```

### Support Channels

- **GitHub Issues**: [github.com/loreste/karl/issues](https://github.com/loreste/karl/issues)
- **GitHub Discussions**: [github.com/loreste/karl/discussions](https://github.com/loreste/karl/discussions)

### Useful Commands Summary

```bash
# Quick health check
curl -s http://localhost:8086/health | jq .

# Session count
curl -s http://localhost:9091/metrics | grep karl_sessions_active

# Recent errors
grep -i error /var/log/karl.log | tail -20

# Test NG protocol
echo -n "d7:command4:pinge" | nc -u localhost 22222

# Port check
ss -ulnp | grep -E "22222|3[0-9]{4}"
```

---

## Next Steps

- [Configuration Reference](../configuration.md)
- [Monitoring Setup](./monitoring-prometheus.md)
- [Kubernetes Deployment](./deploying-kubernetes.md)
