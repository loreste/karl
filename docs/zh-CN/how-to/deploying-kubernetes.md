# 如何在 Kubernetes 上部署 Karl

本指南涵盖在 Kubernetes 上部署 Karl 媒体服务器，包括配置、扩展和生产环境最佳实践。

## 目录

- [前提条件](#前提条件)
- [快速部署](#快速部署)
- [了解部署结构](#了解部署结构)
- [配置](#配置)
- [网络注意事项](#网络注意事项)
- [健康探针](#健康探针)
- [扩展](#扩展)
- [监控](#监控)
- [生产环境清单](#生产环境清单)
- [故障排除](#故障排除)

---

## 前提条件

- Kubernetes 集群（1.19+）
- 已配置 `kubectl` 访问集群
- 了解 Kubernetes 基本概念
- 生产环境：Redis（可选，用于会话共享）

---

## 快速部署

```bash
# 克隆仓库
git clone https://github.com/loreste/karl.git
cd karl

# 使用 kustomize 部署
kubectl apply -k deploy/kubernetes/

# 验证部署
kubectl get pods -l app=karl
kubectl get svc -l app=karl
```

---

## 了解部署结构

`deploy/kubernetes/` 中的 Kubernetes 清单包括：

| 文件 | 用途 |
|------|------|
| `deployment.yaml` | Karl Pod 定义，含探针和资源限制 |
| `service.yaml` | ClusterIP、Headless 和 NodePort 服务 |
| `configmap.yaml` | Karl 配置 |
| `pvc.yaml` | 录音用持久卷 |
| `servicemonitor.yaml` | Prometheus 集成 |
| `kustomization.yaml` | Kustomize 配置 |

### 部署架构

```
┌─────────────────────────────────────────────────────────────┐
│                     Kubernetes 集群                          │
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

## 配置

### 使用 ConfigMap

编辑 ConfigMap 以自定义 Karl 的配置：

```bash
# 直接编辑 ConfigMap
kubectl edit configmap karl-config

# 或应用新的 ConfigMap
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

### 使用环境变量

在部署中通过环境变量覆盖配置：

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

### 使用 Secret 存储敏感数据

```yaml
# 创建 Secret
kubectl create secret generic karl-secrets \
  --from-literal=mysql-dsn='karl:password@tcp(mysql:3306)/karl'

# 在部署中引用
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

## 网络注意事项

### 主机网络模式（默认）

默认部署使用 `hostNetwork: true`，推荐用于媒体服务器：

```yaml
spec:
  hostNetwork: true
  dnsPolicy: ClusterFirstWithHostNet
```

**优点：**
- 直接访问主机网络接口
- RTP/RTCP 流量无需 NAT
- 完整端口范围可用（30000-40000）
- 媒体处理性能最佳

**限制：**
- 每个节点只能运行一个 Karl Pod
- Pod 使用节点的 IP 地址

### 非主机网络模式

如需每个节点运行多个 Pod：

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

**所需变更：**
1. 为 NG 协议使用 NodePort 服务
2. 将 RTP 端口范围限制在可用的 NodePort 范围内
3. 配置 SIP 代理使用 NodePort

```yaml
# 外部访问的 NodePort 服务
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

### 负载均衡器（云服务商）

适用于 AWS、GCP 或 Azure：

```yaml
apiVersion: v1
kind: Service
metadata:
  name: karl-lb
  annotations:
    # AWS NLB
    service.beta.kubernetes.io/aws-load-balancer-type: "nlb"
    # 保留客户端 IP
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

## 健康探针

Karl 实现了 Kubernetes 原生健康探针：

### 启动探针

允许缓慢启动而不被终止：

```yaml
startupProbe:
  httpGet:
    path: /startup
    port: 8086
  initialDelaySeconds: 5
  periodSeconds: 5
  timeoutSeconds: 3
  failureThreshold: 30  # 最大启动时间 150 秒
```

### 存活探针

检测死锁并触发重启：

```yaml
livenessProbe:
  httpGet:
    path: /live
    port: 8086
  periodSeconds: 15
  timeoutSeconds: 5
  failureThreshold: 3
```

### 就绪探针

控制流量路由：

```yaml
readinessProbe:
  httpGet:
    path: /ready
    port: 8086
  periodSeconds: 5
  timeoutSeconds: 3
  failureThreshold: 3
```

### 探针端点

| 端点 | 用途 | 成功条件 |
|------|------|----------|
| `/startup` | 初始化完成 | 应用已初始化 |
| `/live` | 进程健康 | 未死锁 |
| `/ready` | 准备接收流量 | NG 监听器运行中 |
| `/health` | 总体健康状态 | 返回状态信息 |
| `/health/detail` | 详细状态 | 各组件状态明细 |

---

## 扩展

### 使用 Redis 水平扩展

多个 Karl 实例需要启用 Redis 进行会话共享：

**步骤一：部署 Redis**

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

**步骤二：配置 Karl**

```yaml
# 在 ConfigMap 中
{
  "database": {
    "redis_enabled": true,
    "redis_addr": "redis:6379"
  }
}

# 或通过环境变量
env:
- name: KARL_REDIS_ENABLED
  value: "true"
- name: KARL_REDIS_ADDR
  value: "redis:6379"
```

**步骤三：扩展部署**

```bash
kubectl scale deployment karl --replicas=3
```

### 资源推荐

| 并发通话数 | CPU 请求 | 内存请求 | 副本数 |
|-----------|----------|----------|--------|
| < 100 | 250m | 256Mi | 1 |
| 100-500 | 500m | 512Mi | 1-2 |
| 500-1000 | 1000m | 1Gi | 2-3 |
| 1000-5000 | 2000m | 2Gi | 3-5 |
| > 5000 | 2000m | 2Gi | 5+ |

### Pod 中断预算

确保更新期间的可用性：

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

## 监控

### Prometheus 集成

应用 Prometheus Operator 的 ServiceMonitor：

```bash
kubectl apply -f deploy/kubernetes/servicemonitor.yaml
```

或手动创建：

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

### Prometheus 抓取配置

非 Operator 场景：

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

### 关键监控指标

```promql
# 活动会话数
karl_sessions_active

# 会话创建速率
rate(karl_sessions_total[5m])

# 丢包率
karl_rtcp_packet_loss_fraction

# 命令延迟
histogram_quantile(0.99, rate(karl_ng_command_duration_seconds_bucket[5m]))
```

---

## 生产环境清单

### 部署前

- [ ] 配置资源请求和限制
- [ ] 设置健康探针
- [ ] 为录音配置持久存储
- [ ] 多实例部署时设置 Redis
- [ ] 配置 Prometheus 监控
- [ ] 设置日志聚合
- [ ] 如有需要配置网络策略

### 部署配置

- [ ] 使用指定的镜像标签，不使用 `latest`
- [ ] 配置 Pod 中断预算
- [ ] 为媒体工作负载设置节点亲和性
- [ ] 配置反亲和性以实现高可用
- [ ] 设置适当的安全上下文

### 网络

- [ ] 验证 RTP 端口范围可访问
- [ ] 配置 UDP 流量的防火墙规则
- [ ] 测试 SIP 代理到 Karl 的 NG 协议连通性
- [ ] 验证健康端点可访问

### 监控

- [ ] Prometheus 正在抓取指标
- [ ] Grafana 仪表板已配置
- [ ] 告警规则已定义
- [ ] 日志聚合正常工作

---

## 故障排除

### Pod 无法启动

```bash
# 检查 Pod 状态
kubectl describe pod -l app=karl

# 检查日志
kubectl logs -l app=karl

# 检查事件
kubectl get events --sort-by='.lastTimestamp'
```

### 健康检查失败

```bash
# 测试启动探针
kubectl exec -it <pod-name> -- curl localhost:8086/startup

# 测试存活探针
kubectl exec -it <pod-name> -- curl localhost:8086/live

# 测试就绪探针
kubectl exec -it <pod-name> -- curl localhost:8086/ready
```

### NG 协议无法工作

```bash
# 从集群内部测试
kubectl run test --rm -it --image=alpine -- sh
apk add netcat-openbsd
echo -n "d7:command4:pinge" | nc -u karl 22222

# 检查服务端点
kubectl get endpoints karl
```

### 通话中没有音频

1. 验证 RTP 端口可访问：
```bash
# 检查端口绑定
kubectl exec -it <pod-name> -- ss -ulnp | grep 30000
```

2. 检查防火墙规则是否阻止了 UDP

3. 验证 media_ip 配置：
```bash
kubectl exec -it <pod-name> -- env | grep KARL_MEDIA_IP
```

### 查看详细日志

```bash
# 实时查看日志
kubectl logs -l app=karl -f

# 启用调试级别
kubectl set env deployment/karl KARL_LOG_LEVEL=debug
```

---

## 下一步

- [配置录音](./setting-up-recording.md)
- [设置监控](./monitoring-prometheus.md)
- [水平扩展](./scaling-horizontally.md)
