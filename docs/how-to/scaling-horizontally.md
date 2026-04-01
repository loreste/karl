# How to Scale Karl Horizontally

This guide covers running multiple Karl instances for high availability and increased capacity.

## Table of Contents

- [Overview](#overview)
- [Prerequisites](#prerequisites)
- [Architecture](#architecture)
- [Cluster Features](#cluster-features)
- [Redis Setup](#redis-setup)
- [Configure Karl Instances](#configure-karl-instances)
- [Load Balancing](#load-balancing)
- [Automatic Failover](#automatic-failover)
- [Kubernetes Scaling](#kubernetes-scaling)
- [Monitoring Clusters](#monitoring-clusters)
- [Troubleshooting](#troubleshooting)

---

## Overview

Karl supports horizontal scaling through:

- **Redis-backed session sharing**: All instances share session state
- **Consistent hashing**: Deterministic session routing with minimal redistribution
- **Automatic failover**: Sessions automatically migrate when nodes fail
- **Split-brain detection**: Quorum-based partition tolerance
- **Load balancer integration**: Distribute traffic across instances

Benefits:
- Handle more concurrent calls
- High availability with automatic failover
- Rolling updates without downtime
- No session loss during node failures

---

## Prerequisites

- Redis server (standalone or cluster)
- Load balancer or multiple SIP proxy connections
- Shared storage for recordings (if enabled)

---

## Architecture

### Single Instance

```
┌─────────────┐         ┌─────────────┐
│  SIP Proxy  │────────▶│    Karl     │
└─────────────┘         └─────────────┘
```

### Horizontally Scaled

```
                        ┌─────────────┐
                   ┌───▶│   Karl 1    │───┐
┌─────────────┐    │    └─────────────┘   │    ┌─────────────┐
│  SIP Proxy  │────┤                      ├───▶│    Redis    │
└─────────────┘    │    ┌─────────────┐   │    └─────────────┘
                   ├───▶│   Karl 2    │───┤
                   │    └─────────────┘   │
                   │    ┌─────────────┐   │
                   └───▶│   Karl 3    │───┘
                        └─────────────┘
```

---

## Cluster Features

Karl includes enterprise-grade clustering capabilities:

### Consistent Hashing

Sessions are distributed using consistent hashing with 150 virtual nodes per physical node:

- **Minimal redistribution**: When nodes join/leave, only ~1/N sessions move
- **Deterministic routing**: Same session always routes to same node
- **Load balancing**: Virtual nodes ensure even distribution

### Automatic Session Takeover

When a node fails, its sessions are automatically taken over:

1. **Node failure detected** via heartbeat timeout (3 missed heartbeats)
2. **Hash ring updated** to exclude failed node
3. **Sessions redistributed** using consistent hashing
4. **Ports reallocated** on the new node
5. **Media flow resumes** without SIP proxy intervention

### Split-Brain Detection

Karl handles network partitions intelligently:

- **Quorum-based decisions**: Nodes without quorum enter read-only mode
- **Automatic fencing**: Minority partition stops accepting new calls
- **Recovery handling**: Graceful re-merge when partition heals
- **Configurable quorum size**: Default is 2 nodes

### Cluster Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                      ClusterManager                              │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐ │
│  │ RedisSession    │  │   HashRing /    │  │  SplitBrain     │ │
│  │    Store        │  │ SessionRouter   │  │   Detector      │ │
│  │                 │  │                 │  │                 │ │
│  │ - Session CRUD  │  │ - Consistent    │  │ - Partition     │ │
│  │ - Node indexing │  │   hashing       │  │   detection     │ │
│  │ - Batch ops     │  │ - Load balance  │  │ - Quorum mgmt   │ │
│  │ - Pub/Sub       │  │ - Sticky        │  │ - Node fencing  │ │
│  │ - Worker pool   │  │   sessions      │  │ - Heartbeats    │ │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

---

## Redis Setup

### Option 1: Docker

```bash
docker run -d \
  --name redis \
  -p 6379:6379 \
  redis:7-alpine
```

### Option 2: Docker Compose

```yaml
version: '3.8'
services:
  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data
    command: redis-server --appendonly yes
    restart: unless-stopped

volumes:
  redis_data:
```

### Option 3: Managed Redis

Use managed services for production:
- AWS ElastiCache
- Google Cloud Memorystore
- Azure Cache for Redis

### Verify Redis

```bash
redis-cli ping
# Should return: PONG
```

---

## Configure Karl Instances

### Configuration File

Each instance uses the same configuration with Redis enabled:

```json
{
  "database": {
    "redis_enabled": true,
    "redis_addr": "redis:6379",
    "redis_cleanup_interval": 3600
  },
  "ng_protocol": {
    "enabled": true,
    "udp_port": 22222
  },
  "sessions": {
    "max_sessions": 5000,
    "min_port": 30000,
    "max_port": 35000
  },
  "integration": {
    "media_ip": "auto"
  }
}
```

### Environment Variables

```bash
# Instance 1
KARL_REDIS_ENABLED=true
KARL_REDIS_ADDR=redis:6379
KARL_MEDIA_IP=192.168.1.101

# Instance 2
KARL_REDIS_ENABLED=true
KARL_REDIS_ADDR=redis:6379
KARL_MEDIA_IP=192.168.1.102

# Instance 3
KARL_REDIS_ENABLED=true
KARL_REDIS_ADDR=redis:6379
KARL_MEDIA_IP=192.168.1.103
```

### Port Allocation

Avoid port conflicts by assigning different RTP port ranges:

| Instance | RTP Ports |
|----------|-----------|
| Karl 1 | 30000-33333 |
| Karl 2 | 33334-36666 |
| Karl 3 | 36667-40000 |

```json
// Karl 1
{
  "sessions": {
    "min_port": 30000,
    "max_port": 33333
  }
}

// Karl 2
{
  "sessions": {
    "min_port": 33334,
    "max_port": 36666
  }
}
```

### Docker Compose Multi-Instance

```yaml
version: '3.8'

services:
  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data

  karl1:
    image: loreste/karl:latest
    network_mode: host
    environment:
      - KARL_REDIS_ENABLED=true
      - KARL_REDIS_ADDR=localhost:6379
      - KARL_NG_PORT=22222
      - KARL_RTP_MIN_PORT=30000
      - KARL_RTP_MAX_PORT=33333
      - KARL_API_PORT=:8080
      - KARL_METRICS_PORT=:9091
    depends_on:
      - redis

  karl2:
    image: loreste/karl:latest
    network_mode: host
    environment:
      - KARL_REDIS_ENABLED=true
      - KARL_REDIS_ADDR=localhost:6379
      - KARL_NG_PORT=22223
      - KARL_RTP_MIN_PORT=33334
      - KARL_RTP_MAX_PORT=36666
      - KARL_API_PORT=:8081
      - KARL_METRICS_PORT=:9092
    depends_on:
      - redis

  karl3:
    image: loreste/karl:latest
    network_mode: host
    environment:
      - KARL_REDIS_ENABLED=true
      - KARL_REDIS_ADDR=localhost:6379
      - KARL_NG_PORT=22224
      - KARL_RTP_MIN_PORT=36667
      - KARL_RTP_MAX_PORT=40000
      - KARL_API_PORT=:8082
      - KARL_METRICS_PORT=:9093
    depends_on:
      - redis

volumes:
  redis_data:
```

---

## Load Balancing

### SIP Proxy Configuration

#### OpenSIPS

```opensips
# Multiple Karl instances with weights
modparam("rtpengine", "rtpengine_sock",
    "udp:karl1:22222 udp:karl2:22222 udp:karl3:22222")

# Or with failover priority
modparam("rtpengine", "rtpengine_sock",
    "1 == udp:karl1:22222")
modparam("rtpengine", "rtpengine_sock",
    "2 == udp:karl2:22222")
modparam("rtpengine", "rtpengine_sock",
    "3 == udp:karl3:22222")
```

#### Kamailio

```kamailio
# Load balanced
modparam("rtpengine", "rtpengine_sock",
    "udp:karl1:22222 udp:karl2:22222 udp:karl3:22222")

# Weighted distribution
modparam("rtpengine", "rtpengine_sock",
    "2 == udp:karl1:22222 2 == udp:karl2:22222 1 == udp:karl3:22222")
```

### HAProxy for API

```haproxy
frontend karl_api
    bind *:8080
    default_backend karl_servers

backend karl_servers
    balance roundrobin
    option httpchk GET /health
    http-check expect status 200
    server karl1 192.168.1.101:8080 check
    server karl2 192.168.1.102:8080 check
    server karl3 192.168.1.103:8080 check
```

---

## Automatic Failover

Karl automatically handles node failures without manual intervention.

### How Failover Works

1. **Heartbeat Monitoring**: Each node sends heartbeats every 1 second
2. **Failure Detection**: Node marked failed after 3 missed heartbeats (3s)
3. **Session Discovery**: Failed node's sessions retrieved from Redis index
4. **Consistent Redistribution**: Hash ring determines new owners
5. **Port Allocation**: New ports allocated from pre-warmed pool
6. **State Transfer**: Session ownership updated atomically in Redis

### Failover Configuration

```json
{
  "cluster": {
    "enabled": true,
    "heartbeat_interval": "1s",
    "failure_threshold": 3,
    "quorum_size": 2,
    "replication_factor": 150,
    "enable_fencing": true
  }
}
```

| Setting | Description | Default |
|---------|-------------|---------|
| `heartbeat_interval` | Time between heartbeats | 1s |
| `failure_threshold` | Missed heartbeats before failure | 3 |
| `quorum_size` | Minimum nodes for quorum | 2 |
| `replication_factor` | Virtual nodes per physical node | 150 |
| `enable_fencing` | Fence nodes without quorum | true |

### Failover Performance

Karl's failover is optimized for speed:

| Metric | Value |
|--------|-------|
| Detection time | 3 seconds |
| Session takeover | Parallel (2×CPU workers) |
| Port allocation | Pre-warmed pool (< 1ms) |
| Typical failover | < 5 seconds total |

### Monitoring Failover

```promql
# Sessions taken over
karl_cluster_takeovers_done

# Takeover failures
karl_cluster_takeovers_failed

# Node failures detected
karl_cluster_node_failures

# Partition events
karl_cluster_partition_events
```

---

## Kubernetes Scaling

### Basic Scaling

```bash
# Scale to 3 replicas
kubectl scale deployment karl --replicas=3

# Check status
kubectl get pods -l app=karl
```

### Horizontal Pod Autoscaler

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: karl-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: karl
  minReplicas: 2
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Pods
    pods:
      metric:
        name: karl_sessions_active
      target:
        type: AverageValue
        averageValue: "500"
```

### Redis for Kubernetes

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
    metadata:
      labels:
        app: redis
    spec:
      containers:
      - name: redis
        image: redis:7-alpine
        ports:
        - containerPort: 6379
        resources:
          requests:
            memory: "128Mi"
            cpu: "100m"
          limits:
            memory: "256Mi"
            cpu: "250m"
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

### Karl Deployment with Redis

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: karl
spec:
  replicas: 3
  selector:
    matchLabels:
      app: karl
  template:
    metadata:
      labels:
        app: karl
    spec:
      hostNetwork: true
      containers:
      - name: karl
        image: loreste/karl:latest
        env:
        - name: KARL_REDIS_ENABLED
          value: "true"
        - name: KARL_REDIS_ADDR
          value: "redis:6379"
        - name: KARL_MEDIA_IP
          valueFrom:
            fieldRef:
              fieldPath: status.hostIP
```

### Pod Disruption Budget

```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: karl-pdb
spec:
  minAvailable: 2
  selector:
    matchLabels:
      app: karl
```

---

## Monitoring Clusters

### Aggregate Metrics

```promql
# Total active sessions across all instances
sum(karl_sessions_active)

# Sessions per instance
karl_sessions_active

# Session distribution (should be even)
karl_sessions_active / ignoring(instance) group_left sum(karl_sessions_active)
```

### Instance Health

```promql
# Instances up
count(up{job="karl"} == 1)

# Instance with most sessions
topk(1, karl_sessions_active)

# Instance with fewest sessions
bottomk(1, karl_sessions_active)
```

### Redis Monitoring

```promql
# Redis connection status
karl_redis_connected

# Redis operations
rate(karl_redis_operations_total[5m])
```

### Alerting for Clusters

```yaml
groups:
  - name: karl-cluster
    rules:
      - alert: KarlInstanceDown
        expr: up{job="karl"} == 0
        for: 1m
        labels:
          severity: warning
        annotations:
          summary: "Karl instance {{ $labels.instance }} is down"

      - alert: KarlClusterDegraded
        expr: count(up{job="karl"} == 1) < 2
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Karl cluster has fewer than 2 healthy instances"

      - alert: KarlUnevenDistribution
        expr: stddev(karl_sessions_active) / avg(karl_sessions_active) > 0.5
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Session distribution is uneven across Karl instances"

      - alert: KarlRedisDisconnected
        expr: karl_redis_connected == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Karl instance {{ $labels.instance }} lost Redis connection"

      - alert: KarlQuorumLost
        expr: karl_cluster_has_quorum == 0
        for: 30s
        labels:
          severity: critical
        annotations:
          summary: "Karl cluster lost quorum - new sessions blocked"

      - alert: KarlNodeFenced
        expr: karl_cluster_fenced == 1
        for: 1m
        labels:
          severity: warning
        annotations:
          summary: "Karl instance {{ $labels.instance }} is fenced"

      - alert: KarlTakeoverFailures
        expr: rate(karl_cluster_takeovers_failed[5m]) > 0
        for: 1m
        labels:
          severity: warning
        annotations:
          summary: "Karl session takeover failures detected"

      - alert: KarlHighTakeoverLatency
        expr: karl_cluster_takeover_latency_seconds > 10
        for: 1m
        labels:
          severity: warning
        annotations:
          summary: "Karl session takeover taking too long"
```

---

## Troubleshooting

### Sessions Not Shared

1. **Check Redis connectivity**:
```bash
# From Karl container/host
redis-cli -h redis ping
```

2. **Verify Redis is enabled**:
```bash
curl http://karl1:8080/api/v1/stats | jq .redis_enabled
```

3. **Check Redis keys**:
```bash
redis-cli keys "karl:session:*"
```

### Uneven Load Distribution

1. **Check SIP proxy configuration**
2. **Verify all instances are healthy**
3. **Check network connectivity**

### Session Lookup Failures

```bash
# Check session exists in Redis
redis-cli get "karl:session:abc123"

# List all sessions
redis-cli keys "karl:session:*"
```

### Rolling Update Issues

1. **Ensure PodDisruptionBudget is configured**
2. **Use graceful termination**:
```yaml
spec:
  terminationGracePeriodSeconds: 30
```

### Redis Performance

```bash
# Check Redis info
redis-cli info stats

# Monitor Redis commands
redis-cli monitor
```

### Cluster State Issues

**Check cluster stats**:
```bash
curl http://karl1:8080/api/v1/cluster/stats | jq
```

**View hash ring state**:
```bash
curl http://karl1:8080/api/v1/cluster/ring | jq
```

**Check node health**:
```bash
curl http://karl1:8080/api/v1/cluster/nodes | jq
```

### Session Takeover Failures

1. **Check port availability**:
```bash
# Ensure enough ports in range
curl http://karl1:8080/api/v1/stats | jq .port_allocator
```

2. **Check Redis connectivity**:
```bash
redis-cli -h redis ping
```

3. **Check node session index**:
```bash
redis-cli smembers "karl:session:node:failed-node-id"
```

### Split Brain Recovery

1. **Check quorum status**:
```bash
curl http://karl1:8080/api/v1/cluster/stats | jq .has_quorum
```

2. **Check fenced status**:
```bash
curl http://karl1:8080/api/v1/cluster/stats | jq .fenced
```

3. **Force unfence** (use with caution):
```bash
curl -X POST http://karl1:8080/api/v1/cluster/unfence
```

---

## Best Practices

### Cluster Sizing

1. **Use odd number of instances** (3, 5, 7) for quorum decisions
2. **Minimum 3 nodes** for production with automatic failover
3. **Plan for N-1 capacity** - cluster should handle load with one node down

### Performance

4. **Pre-warm port pools** - default 500 port pairs per node
5. **Use Redis Cluster** for large deployments (>10 nodes)
6. **Enable Redis persistence** (AOF or RDB) for durability
7. **Co-locate Redis** with Karl nodes for lower latency

### Reliability

8. **Set appropriate timeouts** - 3s detection, 30s takeover timeout
9. **Monitor quorum status** - alert immediately on quorum loss
10. **Test failover regularly** - validate takeover works correctly

### Operations

11. **Use rolling updates** - update one node at a time
12. **Drain before shutdown** - let sessions migrate gracefully
13. **Monitor session distribution** - alert on uneven load

---

## Performance Characteristics

| Cluster Size | Takeover Time | Sessions/Node | Memory Overhead |
|--------------|---------------|---------------|-----------------|
| 3 nodes | < 3s | 3,000-5,000 | ~50MB |
| 5 nodes | < 4s | 2,000-3,000 | ~80MB |
| 10 nodes | < 5s | 1,000-2,000 | ~150MB |

---

## Next Steps

- [Kubernetes Deployment](./deploying-kubernetes.md)
- [Monitoring Setup](./monitoring-prometheus.md)
- [Troubleshooting Guide](./troubleshooting.md)
