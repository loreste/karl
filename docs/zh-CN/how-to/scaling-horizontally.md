# 如何水平扩展 Karl

本指南涵盖运行多个 Karl 实例以实现高可用和增加容量。

## 目录

- [概述](#概述)
- [前提条件](#前提条件)
- [架构](#架构)
- [集群特性](#集群特性)
- [Redis 设置](#redis-设置)
- [配置 Karl 实例](#配置-karl-实例)
- [负载均衡](#负载均衡)
- [自动故障转移](#自动故障转移)
- [Kubernetes 扩展](#kubernetes-扩展)
- [集群监控](#集群监控)
- [故障排除](#故障排除)

---

## 概述

Karl 通过以下方式支持水平扩展：

- **Redis 支撑的会话共享**：所有实例共享会话状态
- **一致性哈希**：确定性会话路由，最小化重分布
- **自动故障转移**：节点故障时会话自动迁移
- **脑裂检测**：基于仲裁的分区容错
- **负载均衡器集成**：在实例之间分配流量

优势：
- 处理更多并发通话
- 自动故障转移实现高可用
- 滚动更新不停机
- 节点故障不丢失会话

---

## 前提条件

- Redis 服务器（单机或集群）
- 负载均衡器或多个 SIP 代理连接
- 共享存储用于录音（如已启用）

---

## 架构

### 单实例

```
┌─────────────┐         ┌─────────────┐
│  SIP 代理   │────────▶│    Karl     │
└─────────────┘         └─────────────┘
```

### 水平扩展

```
                        ┌─────────────┐
                   ┌───▶│   Karl 1    │───┐
┌─────────────┐    │    └─────────────┘   │    ┌─────────────┐
│  SIP 代理   │────┤                      ├───▶│    Redis    │
└─────────────┘    │    ┌─────────────┐   │    └─────────────┘
                   ├───▶│   Karl 2    │───┤
                   │    └─────────────┘   │
                   │    ┌─────────────┐   │
                   └───▶│   Karl 3    │───┘
                        └─────────────┘
```

---

## 集群特性

Karl 包含企业级集群能力：

### 一致性哈希

会话通过一致性哈希分布，每个物理节点 150 个虚拟节点：

- **最小化重分布**：当节点加入/离开时，仅约 1/N 的会话移动
- **确定性路由**：同一会话始终路由到同一节点
- **负载均衡**：虚拟节点确保均匀分布

### 自动会话接管

当节点故障时，其会话会被自动接管：

1. **检测到节点故障**：通过心跳超时（连续 3 次心跳丢失）
2. **更新哈希环**：排除故障节点
3. **重分布会话**：使用一致性哈希
4. **重新分配端口**：在新节点上
5. **恢复媒体流**：无需 SIP 代理干预

### 脑裂检测

Karl 智能处理网络分区：

- **基于仲裁的决策**：没有仲裁的节点进入只读模式
- **自动隔离**：少数派分区停止接受新通话
- **恢复处理**：分区恢复后优雅地重新合并
- **可配置仲裁大小**：默认 2 个节点

### 集群架构

```
┌─────────────────────────────────────────────────────────────────┐
│                      ClusterManager                              │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐ │
│  │ RedisSession    │  │   HashRing /    │  │  SplitBrain     │ │
│  │    Store        │  │ SessionRouter   │  │   Detector      │ │
│  │                 │  │                 │  │                 │ │
│  │ - 会话 CRUD    │  │ - 一致性哈希   │  │ - 分区检测     │ │
│  │ - 节点索引     │  │ - 负载均衡     │  │ - 仲裁管理     │ │
│  │ - 批量操作     │  │ - 会话粘性     │  │ - 节点隔离     │ │
│  │ - 发布/订阅    │  │                 │  │ - 心跳检测     │ │
│  │ - 工作线程池   │  │                 │  │                 │ │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

---

## Redis 设置

### 方式一：Docker

```bash
docker run -d \
  --name redis \
  -p 6379:6379 \
  redis:7-alpine
```

### 方式二：Docker Compose

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

### 方式三：托管 Redis

生产环境使用托管服务：
- AWS ElastiCache
- Google Cloud Memorystore
- Azure Cache for Redis

### 验证 Redis

```bash
redis-cli ping
# 应返回：PONG
```

---

## 配置 Karl 实例

### 配置文件

每个实例使用相同的配置，启用 Redis：

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

### 环境变量

```bash
# 实例 1
KARL_REDIS_ENABLED=true
KARL_REDIS_ADDR=redis:6379
KARL_MEDIA_IP=192.168.1.101

# 实例 2
KARL_REDIS_ENABLED=true
KARL_REDIS_ADDR=redis:6379
KARL_MEDIA_IP=192.168.1.102

# 实例 3
KARL_REDIS_ENABLED=true
KARL_REDIS_ADDR=redis:6379
KARL_MEDIA_IP=192.168.1.103
```

### 端口分配

分配不同的 RTP 端口范围以避免冲突：

| 实例 | RTP 端口 |
|------|----------|
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

### Docker Compose 多实例

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

## 负载均衡

### SIP 代理配置

#### OpenSIPS

```opensips
# 多个 Karl 实例带权重
modparam("rtpengine", "rtpengine_sock",
    "udp:karl1:22222 udp:karl2:22222 udp:karl3:22222")

# 或按故障转移优先级
modparam("rtpengine", "rtpengine_sock",
    "1 == udp:karl1:22222")
modparam("rtpengine", "rtpengine_sock",
    "2 == udp:karl2:22222")
modparam("rtpengine", "rtpengine_sock",
    "3 == udp:karl3:22222")
```

#### Kamailio

```kamailio
# 负载均衡
modparam("rtpengine", "rtpengine_sock",
    "udp:karl1:22222 udp:karl2:22222 udp:karl3:22222")

# 加权分配
modparam("rtpengine", "rtpengine_sock",
    "2 == udp:karl1:22222 2 == udp:karl2:22222 1 == udp:karl3:22222")
```

### HAProxy 用于 API

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

## 自动故障转移

Karl 自动处理节点故障，无需人工干预。

### 故障转移工作原理

1. **心跳监控**：每个节点每 1 秒发送一次心跳
2. **故障检测**：连续丢失 3 次心跳（3 秒）后标记为故障
3. **会话发现**：从 Redis 索引中获取故障节点的会话
4. **一致性重分布**：哈希环确定新的所有者
5. **端口分配**：从预热池中分配新端口
6. **状态转移**：在 Redis 中原子更新会话所有权

### 故障转移配置

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

| 设置 | 说明 | 默认值 |
|------|------|--------|
| `heartbeat_interval` | 心跳间隔 | 1s |
| `failure_threshold` | 丢失心跳次数阈值 | 3 |
| `quorum_size` | 仲裁最少节点数 | 2 |
| `replication_factor` | 每物理节点的虚拟节点数 | 150 |
| `enable_fencing` | 隔离没有仲裁的节点 | true |

### 故障转移性能

Karl 的故障转移针对速度进行了优化：

| 指标 | 值 |
|------|-----|
| 检测时间 | 3 秒 |
| 会话接管 | 并行（2×CPU 工作线程） |
| 端口分配 | 预热池（< 1ms） |
| 典型故障转移 | 总计 < 5 秒 |

### 监控故障转移

```promql
# 已接管的会话
karl_cluster_takeovers_done

# 接管失败
karl_cluster_takeovers_failed

# 检测到的节点故障
karl_cluster_node_failures

# 分区事件
karl_cluster_partition_events
```

---

## Kubernetes 扩展

### 基本扩展

```bash
# 扩展到 3 个副本
kubectl scale deployment karl --replicas=3

# 检查状态
kubectl get pods -l app=karl
```

### 水平 Pod 自动扩展器

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

### Kubernetes 中的 Redis

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

### 带 Redis 的 Karl 部署

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

### Pod 中断预算

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

## 集群监控

### 聚合指标

```promql
# 所有实例的总活动会话数
sum(karl_sessions_active)

# 每个实例的会话数
karl_sessions_active

# 会话分布（应该均匀）
karl_sessions_active / ignoring(instance) group_left sum(karl_sessions_active)
```

### 实例健康

```promql
# 在线实例数
count(up{job="karl"} == 1)

# 会话最多的实例
topk(1, karl_sessions_active)

# 会话最少的实例
bottomk(1, karl_sessions_active)
```

### Redis 监控

```promql
# Redis 连接状态
karl_redis_connected

# Redis 操作
rate(karl_redis_operations_total[5m])
```

### 集群告警

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
          summary: "Karl 实例 {{ $labels.instance }} 已宕机"

      - alert: KarlClusterDegraded
        expr: count(up{job="karl"} == 1) < 2
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Karl 集群健康实例少于 2 个"

      - alert: KarlUnevenDistribution
        expr: stddev(karl_sessions_active) / avg(karl_sessions_active) > 0.5
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Karl 实例间会话分布不均"

      - alert: KarlRedisDisconnected
        expr: karl_redis_connected == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Karl 实例 {{ $labels.instance }} 失去 Redis 连接"

      - alert: KarlQuorumLost
        expr: karl_cluster_has_quorum == 0
        for: 30s
        labels:
          severity: critical
        annotations:
          summary: "Karl 集群失去仲裁 — 新会话被阻止"

      - alert: KarlNodeFenced
        expr: karl_cluster_fenced == 1
        for: 1m
        labels:
          severity: warning
        annotations:
          summary: "Karl 实例 {{ $labels.instance }} 已被隔离"

      - alert: KarlTakeoverFailures
        expr: rate(karl_cluster_takeovers_failed[5m]) > 0
        for: 1m
        labels:
          severity: warning
        annotations:
          summary: "检测到 Karl 会话接管失败"

      - alert: KarlHighTakeoverLatency
        expr: karl_cluster_takeover_latency_seconds > 10
        for: 1m
        labels:
          severity: warning
        annotations:
          summary: "Karl 会话接管耗时过长"
```

---

## 故障排除

### 会话未共享

1. **检查 Redis 连通性**：
```bash
# 从 Karl 容器/主机
redis-cli -h redis ping
```

2. **验证 Redis 已启用**：
```bash
curl http://karl1:8080/api/v1/stats | jq .redis_enabled
```

3. **检查 Redis 键**：
```bash
redis-cli keys "karl:session:*"
```

### 负载分布不均

1. **检查 SIP 代理配置**
2. **验证所有实例健康**
3. **检查网络连通性**

### 会话查找失败

```bash
# 检查会话是否存在于 Redis 中
redis-cli get "karl:session:abc123"

# 列出所有会话
redis-cli keys "karl:session:*"
```

### 滚动更新问题

1. **确保已配置 PodDisruptionBudget**
2. **使用优雅终止**：
```yaml
spec:
  terminationGracePeriodSeconds: 30
```

### Redis 性能

```bash
# 检查 Redis 信息
redis-cli info stats

# 监控 Redis 命令
redis-cli monitor
```

### 集群状态问题

**检查集群统计**：
```bash
curl http://karl1:8080/api/v1/cluster/stats | jq
```

**查看哈希环状态**：
```bash
curl http://karl1:8080/api/v1/cluster/ring | jq
```

**检查节点健康**：
```bash
curl http://karl1:8080/api/v1/cluster/nodes | jq
```

### 会话接管失败

1. **检查端口可用性**：
```bash
# 确保端口范围有足够端口
curl http://karl1:8080/api/v1/stats | jq .port_allocator
```

2. **检查 Redis 连通性**：
```bash
redis-cli -h redis ping
```

3. **检查节点会话索引**：
```bash
redis-cli smembers "karl:session:node:failed-node-id"
```

### 脑裂恢复

1. **检查仲裁状态**：
```bash
curl http://karl1:8080/api/v1/cluster/stats | jq .has_quorum
```

2. **检查隔离状态**：
```bash
curl http://karl1:8080/api/v1/cluster/stats | jq .fenced
```

3. **强制解除隔离**（谨慎使用）：
```bash
curl -X POST http://karl1:8080/api/v1/cluster/unfence
```

---

## 最佳实践

### 集群规模

1. **使用奇数个实例**（3、5、7）用于仲裁决策
2. **生产环境最少 3 个节点**，实现自动故障转移
3. **按 N-1 容量规划** — 集群应在一个节点宕机时仍能处理负载

### 性能

4. **预热端口池** — 默认每节点 500 个端口对
5. **大规模部署使用 Redis Cluster**（>10 个节点）
6. **启用 Redis 持久化**（AOF 或 RDB）确保持久性
7. **Redis 与 Karl 节点同位部署**以降低延迟

### 可靠性

8. **设置合适的超时** — 3 秒检测，30 秒接管超时
9. **监控仲裁状态** — 仲裁丢失时立即告警
10. **定期测试故障转移** — 验证接管是否正常工作

### 运维

11. **使用滚动更新** — 一次更新一个节点
12. **关闭前排空** — 让会话优雅迁移
13. **监控会话分布** — 负载不均时告警

---

## 性能特征

| 集群大小 | 接管时间 | 每节点会话数 | 内存开销 |
|---------|---------|-------------|---------|
| 3 节点 | < 3s | 3,000-5,000 | 约 50MB |
| 5 节点 | < 4s | 2,000-3,000 | 约 80MB |
| 10 节点 | < 5s | 1,000-2,000 | 约 150MB |

---

## 下一步

- [Kubernetes 部署](./deploying-kubernetes.md)
- [监控设置](./monitoring-prometheus.md)
- [故障排除指南](./troubleshooting.md)
