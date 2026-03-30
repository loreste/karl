# How to Deploy Karl on Kubernetes

This guide covers deploying Karl Media Server on Kubernetes, including configuration, scaling, and production best practices.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Quick Deploy](#quick-deploy)
- [Understanding the Deployment](#understanding-the-deployment)
- [Configuration](#configuration)
- [Networking Considerations](#networking-considerations)
- [Health Probes](#health-probes)
- [Scaling](#scaling)
- [Monitoring](#monitoring)
- [Production Checklist](#production-checklist)
- [Troubleshooting](#troubleshooting)

---

## Prerequisites

- Kubernetes cluster (1.19+)
- `kubectl` configured to access your cluster
- Basic understanding of Kubernetes concepts
- For production: Redis (optional, for session sharing)

---

## Quick Deploy

```bash
# Clone the repository
git clone https://github.com/loreste/karl.git
cd karl

# Deploy using kustomize
kubectl apply -k deploy/kubernetes/

# Verify deployment
kubectl get pods -l app=karl
kubectl get svc -l app=karl
```

---

## Understanding the Deployment

The Kubernetes manifests in `deploy/kubernetes/` include:

| File | Purpose |
|------|---------|
| `deployment.yaml` | Karl pod specification with probes and resources |
| `service.yaml` | ClusterIP, Headless, and NodePort services |
| `configmap.yaml` | Karl configuration |
| `pvc.yaml` | Persistent volume for recordings |
| `servicemonitor.yaml` | Prometheus integration |
| `kustomization.yaml` | Kustomize configuration |

### Deployment Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Kubernetes Cluster                       │
│                                                             │
│  ┌─────────────┐     ┌─────────────┐     ┌─────────────┐  │
│  │   Service   │     │   Service   │     │   Service   │  │
│  │   (karl)    │     │(karl-headless)│   │(karl-external)│ │
│  │  ClusterIP  │     │   Headless  │     │  NodePort   │  │
│  └──────┬──────┘     └──────┬──────┘     └──────┬──────┘  │
│         │                   │                   │          │
│         └───────────────────┼───────────────────┘          │
│                             │                              │
│                    ┌────────▼────────┐                     │
│                    │   Karl Pod      │                     │
│                    │  (hostNetwork)  │                     │
│                    └────────┬────────┘                     │
│                             │                              │
│                    ┌────────▼────────┐                     │
│                    │   ConfigMap     │                     │
│                    │   (karl-config) │                     │
│                    └─────────────────┘                     │
└─────────────────────────────────────────────────────────────┘
```

---

## Configuration

### Using ConfigMap

Edit the ConfigMap to customize Karl's configuration:

```bash
# Edit ConfigMap directly
kubectl edit configmap karl-config

# Or apply a new ConfigMap
kubectl apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: karl-config
data:
  config.json: |
    {
      "ng_protocol": {
        "enabled": true,
        "udp_port": 22222
      },
      "sessions": {
        "max_sessions": 10000,
        "min_port": 30000,
        "max_port": 40000
      },
      "recording": {
        "enabled": true,
        "base_path": "/var/lib/karl/recordings"
      }
    }
EOF
```

### Using Environment Variables

Override configuration via environment variables in the deployment:

```yaml
spec:
  containers:
  - name: karl
    env:
    - name: KARL_LOG_LEVEL
      value: "info"
    - name: KARL_MAX_SESSIONS
      value: "5000"
    - name: KARL_REDIS_ENABLED
      value: "true"
    - name: KARL_REDIS_ADDR
      value: "redis:6379"
```

### Using Secrets for Sensitive Data

```yaml
# Create secret
kubectl create secret generic karl-secrets \
  --from-literal=mysql-dsn='karl:password@tcp(mysql:3306)/karl'

# Reference in deployment
spec:
  containers:
  - name: karl
    env:
    - name: KARL_MYSQL_DSN
      valueFrom:
        secretKeyRef:
          name: karl-secrets
          key: mysql-dsn
```

---

## Networking Considerations

### Host Network Mode (Default)

The default deployment uses `hostNetwork: true`, which is recommended for media servers:

```yaml
spec:
  hostNetwork: true
  dnsPolicy: ClusterFirstWithHostNet
```

**Advantages:**
- Direct access to host network interface
- No NAT for RTP/RTCP traffic
- Full port range available (30000-40000)
- Best performance for media

**Limitations:**
- Only one Karl pod per node
- Pod uses node's IP address

### Non-Host Network Mode

For environments requiring multiple pods per node:

```yaml
spec:
  hostNetwork: false
  containers:
  - name: karl
    ports:
    - containerPort: 22222
      protocol: UDP
    - containerPort: 8080
      protocol: TCP
```

**Required changes:**
1. Use NodePort service for NG protocol
2. Limit RTP port range to available NodePorts
3. Configure your SIP proxy to use NodePort

```yaml
# NodePort service for external access
apiVersion: v1
kind: Service
metadata:
  name: karl-external
spec:
  type: NodePort
  ports:
  - name: ng-protocol
    port: 22222
    targetPort: 22222
    nodePort: 32222
    protocol: UDP
  selector:
    app: karl
```

### Load Balancer (Cloud Providers)

For AWS, GCP, or Azure:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: karl-lb
  annotations:
    # AWS NLB
    service.beta.kubernetes.io/aws-load-balancer-type: "nlb"
    # Preserve client IP
    service.beta.kubernetes.io/aws-load-balancer-target-group-attributes: preserve_client_ip.enabled=true
spec:
  type: LoadBalancer
  ports:
  - name: ng-protocol
    port: 22222
    targetPort: 22222
    protocol: UDP
  selector:
    app: karl
```

---

## Health Probes

Karl implements Kubernetes-native health probes:

### Startup Probe

Allows slow startup without being killed:

```yaml
startupProbe:
  httpGet:
    path: /startup
    port: 8086
  initialDelaySeconds: 5
  periodSeconds: 5
  timeoutSeconds: 3
  failureThreshold: 30  # 150 seconds max startup
```

### Liveness Probe

Detects deadlocks and triggers restart:

```yaml
livenessProbe:
  httpGet:
    path: /live
    port: 8086
  periodSeconds: 15
  timeoutSeconds: 5
  failureThreshold: 3
```

### Readiness Probe

Controls traffic routing:

```yaml
readinessProbe:
  httpGet:
    path: /ready
    port: 8086
  periodSeconds: 5
  timeoutSeconds: 3
  failureThreshold: 3
```

### Probe Endpoints

| Endpoint | Purpose | Success Criteria |
|----------|---------|------------------|
| `/startup` | Initialization complete | App is initialized |
| `/live` | Process is healthy | Not deadlocked |
| `/ready` | Ready for traffic | NG listener running |
| `/health` | General health | Returns status |
| `/health/detail` | Detailed status | Component breakdown |

---

## Scaling

### Horizontal Scaling with Redis

For multiple Karl instances, enable Redis for session sharing:

**Step 1: Deploy Redis**

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: redis
spec:
  replicas: 1
  selector:
    matchLabels:
      app: redis
  template:
    spec:
      containers:
      - name: redis
        image: redis:7-alpine
        ports:
        - containerPort: 6379
---
apiVersion: v1
kind: Service
metadata:
  name: redis
spec:
  ports:
  - port: 6379
  selector:
    app: redis
```

**Step 2: Configure Karl**

```yaml
# In ConfigMap
{
  "database": {
    "redis_enabled": true,
    "redis_addr": "redis:6379"
  }
}

# Or via environment
env:
- name: KARL_REDIS_ENABLED
  value: "true"
- name: KARL_REDIS_ADDR
  value: "redis:6379"
```

**Step 3: Scale Deployment**

```bash
kubectl scale deployment karl --replicas=3
```

### Resource Recommendations

| Concurrent Calls | CPU Request | Memory Request | Replicas |
|-----------------|-------------|----------------|----------|
| < 100 | 250m | 256Mi | 1 |
| 100-500 | 500m | 512Mi | 1-2 |
| 500-1000 | 1000m | 1Gi | 2-3 |
| 1000-5000 | 2000m | 2Gi | 3-5 |
| > 5000 | 2000m | 2Gi | 5+ |

### Pod Disruption Budget

Ensure availability during updates:

```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: karl-pdb
spec:
  minAvailable: 1
  selector:
    matchLabels:
      app: karl
```

---

## Monitoring

### Prometheus Integration

Apply the ServiceMonitor for Prometheus Operator:

```bash
kubectl apply -f deploy/kubernetes/servicemonitor.yaml
```

Or create manually:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: karl
  labels:
    app: karl
spec:
  selector:
    matchLabels:
      app: karl
  endpoints:
  - port: metrics
    interval: 15s
    path: /metrics
```

### Prometheus Scrape Config

For non-Operator setups:

```yaml
scrape_configs:
  - job_name: 'karl'
    kubernetes_sd_configs:
      - role: pod
    relabel_configs:
      - source_labels: [__meta_kubernetes_pod_label_app]
        action: keep
        regex: karl
      - source_labels: [__meta_kubernetes_pod_container_port_name]
        action: keep
        regex: metrics
```

### Key Metrics to Monitor

```promql
# Active sessions
karl_sessions_active

# Session creation rate
rate(karl_sessions_total[5m])

# Packet loss
karl_rtcp_packet_loss_fraction

# Command latency
histogram_quantile(0.99, rate(karl_ng_command_duration_seconds_bucket[5m]))
```

---

## Production Checklist

### Before Deploying

- [ ] Configure resource requests and limits
- [ ] Set up health probes
- [ ] Configure persistent storage for recordings
- [ ] Set up Redis for multi-instance deployments
- [ ] Configure Prometheus monitoring
- [ ] Set up log aggregation
- [ ] Configure network policies if required

### Deployment Configuration

- [ ] Use specific image tags, not `latest`
- [ ] Configure Pod Disruption Budget
- [ ] Set up node affinity for media workloads
- [ ] Configure anti-affinity for high availability
- [ ] Set appropriate security context

### Networking

- [ ] Verify RTP port range is accessible
- [ ] Configure firewall rules for UDP traffic
- [ ] Test NG protocol connectivity from SIP proxy
- [ ] Verify health endpoints are accessible

### Monitoring

- [ ] Prometheus scraping metrics
- [ ] Grafana dashboard configured
- [ ] Alerting rules defined
- [ ] Log aggregation working

---

## Troubleshooting

### Pod Not Starting

```bash
# Check pod status
kubectl describe pod -l app=karl

# Check logs
kubectl logs -l app=karl

# Check events
kubectl get events --sort-by='.lastTimestamp'
```

### Health Checks Failing

```bash
# Test startup probe
kubectl exec -it <pod-name> -- curl localhost:8086/startup

# Test liveness probe
kubectl exec -it <pod-name> -- curl localhost:8086/live

# Test readiness probe
kubectl exec -it <pod-name> -- curl localhost:8086/ready
```

### NG Protocol Not Working

```bash
# Test from within cluster
kubectl run test --rm -it --image=alpine -- sh
apk add netcat-openbsd
echo -n "d7:command4:pinge" | nc -u karl 22222

# Check service endpoints
kubectl get endpoints karl
```

### No Audio in Calls

1. Verify RTP ports are accessible:
```bash
# Check port binding
kubectl exec -it <pod-name> -- ss -ulnp | grep 30000
```

2. Check for firewall rules blocking UDP

3. Verify media_ip configuration:
```bash
kubectl exec -it <pod-name> -- env | grep KARL_MEDIA_IP
```

### View Detailed Logs

```bash
# Stream logs
kubectl logs -l app=karl -f

# With debug level
kubectl set env deployment/karl KARL_LOG_LEVEL=debug
```

---

## Next Steps

- [Configure Recording](./setting-up-recording.md)
- [Set Up Monitoring](./monitoring-prometheus.md)
- [Scale Horizontally](./scaling-horizontally.md)
