# Karl Kubernetes Deployment

This directory contains Kubernetes manifests for deploying Karl Media Server.

## Quick Start

```bash
# Deploy using kubectl
kubectl apply -k .

# Or apply individual files
kubectl apply -f configmap.yaml
kubectl apply -f pvc.yaml
kubectl apply -f deployment.yaml
kubectl apply -f service.yaml
```

## Architecture Considerations

### Host Network Mode (Default)

The default deployment uses `hostNetwork: true` which is the simplest approach for media servers. This allows Karl to:

- Receive RTP/RTCP traffic directly on the host's network interface
- Avoid NAT complications with UDP traffic
- Use the full port range (30000-40000) without NodePort limitations

**Limitation**: Only one Karl pod per node.

### Non-Host Network Mode

If you need multiple pods per node or prefer network isolation:

1. Edit `deployment.yaml` and set `hostNetwork: false`
2. Use the `karl-external` NodePort service
3. Configure your SIP proxy to use the NodePort (32222)
4. Limit the RTP port range to match available NodePorts

## Configuration

### ConfigMap

Edit `configmap.yaml` to customize Karl's configuration:

```bash
kubectl edit configmap karl-config
```

Key settings to configure:
- `ng_protocol.udp_port`: Port for NG protocol (default: 22222)
- `sessions.min_port` / `sessions.max_port`: RTP port range
- `integration.media_ip`: Set to node's external IP or use "auto"
- `recording.enabled`: Enable/disable call recording

### Storage

The PVC is configured for 50Gi. Adjust in `pvc.yaml` based on:
- Expected call volume
- Recording retention policy
- Recording format (stereo WAV ~1MB/minute)

## Monitoring

### Prometheus

Karl exposes metrics at `:9091/metrics`. If using Prometheus Operator:

```bash
kubectl apply -f servicemonitor.yaml
```

### Health Checks

Karl exposes Kubernetes-native probe endpoints:

| Probe | Endpoint | Purpose |
|-------|----------|---------|
| Startup | `/startup` | Wait for application initialization |
| Liveness | `/live` | Detect deadlocks, restart if needed |
| Readiness | `/ready` | Check if ready to accept traffic |
| Health | `/health` | Simple health status |
| Health Detail | `/health/detail` | Detailed component health |

**Probe behavior:**
- Startup probe allows up to 150s for initialization
- Liveness probe restarts pod if 3 consecutive checks fail
- Readiness probe removes pod from service if not ready

### Environment Variables

Karl supports configuration via environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `KARL_CONFIG_PATH` | Config file path | `config/config.json` |
| `KARL_HEALTH_PORT` | Health check port | `:8086` |
| `KARL_METRICS_PORT` | Prometheus metrics port | `:9091` |
| `KARL_API_PORT` | REST API port | `:8080` |
| `KARL_RUN_DIR` | Runtime directory | `./run/karl` |
| `KARL_NG_PORT` | NG protocol UDP port | (from config) |
| `KARL_RTP_MIN_PORT` | RTP port range start | `30000` |
| `KARL_RTP_MAX_PORT` | RTP port range end | `40000` |
| `KARL_MAX_SESSIONS` | Max concurrent sessions | `10000` |
| `KARL_RECORDING_PATH` | Recording storage path | `/var/lib/karl/recordings` |
| `KARL_RECORDING_ENABLED` | Enable recording | (from config) |
| `KARL_MYSQL_DSN` | MySQL connection string | (from config) |
| `KARL_REDIS_ADDR` | Redis address | (from config) |
| `KARL_REDIS_ENABLED` | Enable Redis | (from config) |
| `KARL_MEDIA_IP` | Media IP address | `auto` |
| `KARL_PUBLIC_IP` | Public IP address | (auto-detected) |

Environment variables override config file values.

## Scaling

### Horizontal Scaling

For multiple Karl instances, enable Redis for session sharing:

1. Deploy Redis (or use managed Redis)
2. Update ConfigMap:
   ```json
   "database": {
     "redis_enabled": true,
     "redis_addr": "redis:6379"
   }
   ```
3. Increase replicas in `deployment.yaml`

### Resource Tuning

Adjust resources based on expected load:

| Concurrent Calls | CPU Request | Memory Request |
|-----------------|-------------|----------------|
| < 100           | 250m        | 256Mi          |
| 100-500         | 500m        | 512Mi          |
| 500-1000        | 1000m       | 1Gi            |
| > 1000          | 2000m       | 2Gi            |

## Integration with SIP Proxies

### OpenSIPS (in same cluster)

```opensips
modparam("rtpengine", "rtpengine_sock", "udp:karl:22222")
```

### Kamailio (in same cluster)

```kamailio
modparam("rtpengine", "rtpengine_sock", "udp:karl:22222")
```

### External SIP Proxy

Use the NodePort or LoadBalancer service:

```
rtpengine_sock = "udp:<node-ip>:32222"
```

## Troubleshooting

### Check pod status
```bash
kubectl get pods -l app=karl
kubectl describe pod -l app=karl
```

### View logs
```bash
kubectl logs -l app=karl -f
```

### Test NG protocol
```bash
kubectl exec -it <pod-name> -- sh
echo -n "d7:command4:pinge" | nc -u localhost 22222
```

### Check metrics
```bash
kubectl port-forward svc/karl 9091:9091
curl localhost:9091/metrics
```
