# 如何使用 Prometheus 监控 Karl

本指南涵盖使用 Prometheus 和 Grafana 为 Karl 媒体服务器设置全面监控。

## 目录

- [概述](#概述)
- [快速设置](#快速设置)
- [Prometheus 配置](#prometheus-配置)
- [关键指标](#关键指标)
- [Grafana 仪表板](#grafana-仪表板)
- [告警规则](#告警规则)
- [最佳实践](#最佳实践)

---

## 概述

Karl 在 `:9091/metrics` 端点以 Prometheus 格式暴露指标。这些指标涵盖：

- 活动会话和通话统计
- RTCP 质量指标（抖动、丢包、RTT）
- 前向纠错统计
- 抖动缓冲区性能
- API 请求指标
- NG 协议命令指标

---

## 快速设置

### 验证指标端点

```bash
curl http://localhost:9091/metrics | head -50
```

### 最小化 Prometheus 配置

```yaml
# prometheus.yml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'karl'
    static_configs:
      - targets: ['localhost:9091']
```

### 启动 Prometheus

```bash
# Docker
docker run -d \
  --name prometheus \
  -p 9090:9090 \
  -v $(pwd)/prometheus.yml:/etc/prometheus/prometheus.yml \
  prom/prometheus

# 或原生运行
prometheus --config.file=prometheus.yml
```

---

## Prometheus 配置

### 基本配置

```yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: 'karl'
    static_configs:
      - targets: ['karl:9091']
    relabel_configs:
      - source_labels: [__address__]
        target_label: instance
        regex: '([^:]+):\d+'
        replacement: '${1}'
```

### 多个 Karl 实例

```yaml
scrape_configs:
  - job_name: 'karl'
    static_configs:
      - targets:
        - 'karl1.example.com:9091'
        - 'karl2.example.com:9091'
        - 'karl3.example.com:9091'
```

### Kubernetes 服务发现

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
      - source_labels: [__meta_kubernetes_namespace]
        target_label: namespace
      - source_labels: [__meta_kubernetes_pod_name]
        target_label: pod
```

### Prometheus Operator（ServiceMonitor）

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

---

## 关键指标

### 会话指标

| 指标 | 类型 | 说明 |
|------|------|------|
| `karl_sessions_active` | Gauge | 当前活动会话数 |
| `karl_sessions_total` | Counter | 已创建的总会话数 |
| `karl_session_duration_seconds` | Histogram | 会话时长分布 |

**查询示例**：

```promql
# 活动会话数
karl_sessions_active

# 每分钟创建的会话数
rate(karl_sessions_total[5m]) * 60

# 95 百分位会话时长
histogram_quantile(0.95, rate(karl_session_duration_seconds_bucket[5m]))
```

### RTCP 质量指标

| 指标 | 类型 | 说明 |
|------|------|------|
| `karl_rtcp_rtt_seconds` | Gauge | 往返时间 |
| `karl_rtcp_jitter_seconds` | Gauge | 报告的抖动 |
| `karl_rtcp_packet_loss_fraction` | Gauge | 丢包比率（0-1） |
| `karl_rtcp_sr_sent_total` | Counter | 已发送的发送者报告 |
| `karl_rtcp_rr_sent_total` | Counter | 已发送的接收者报告 |

**查询示例**：

```promql
# 平均 RTT
avg(karl_rtcp_rtt_seconds)

# 平均抖动（毫秒）
avg(karl_rtcp_jitter_seconds) * 1000

# 高丢包率（>5%）的会话数
count(karl_rtcp_packet_loss_fraction > 0.05)
```

### FEC 指标

| 指标 | 类型 | 说明 |
|------|------|------|
| `karl_fec_packets_sent_total` | Counter | 已发送的 FEC 数据包 |
| `karl_fec_recoveries_total` | Counter | 成功恢复的数据包 |
| `karl_fec_recovery_failures_total` | Counter | 恢复失败次数 |
| `karl_fec_redundancy_ratio` | Gauge | 当前冗余比率 |

**查询示例**：

```promql
# FEC 恢复率
rate(karl_fec_recoveries_total[5m]) / rate(karl_fec_packets_sent_total[5m])

# 恢复失败率
rate(karl_fec_recovery_failures_total[5m])
```

### 抖动缓冲区指标

| 指标 | 类型 | 说明 |
|------|------|------|
| `karl_jitter_buffer_size` | Gauge | 当前缓冲区大小 |
| `karl_jitter_buffer_latency_seconds` | Gauge | 缓冲区延迟 |
| `karl_jitter_buffer_packets_dropped_total` | Counter | 丢弃的数据包 |

**查询示例**：

```promql
# 平均缓冲区延迟
avg(karl_jitter_buffer_latency_seconds) * 1000

# 每分钟丢弃的数据包
rate(karl_jitter_buffer_packets_dropped_total[5m]) * 60
```

### NG 协议指标

| 指标 | 类型 | 说明 |
|------|------|------|
| `karl_ng_commands_total` | Counter | 按类型和结果分类的命令数 |
| `karl_ng_command_duration_seconds` | Histogram | 命令处理时间 |
| `karl_ng_active_calls` | Gauge | 通过 NG 协议的活动通话数 |

**查询示例**：

```promql
# 按类型分类的每秒命令数
sum(rate(karl_ng_commands_total[5m])) by (command)

# 99 百分位命令延迟
histogram_quantile(0.99, rate(karl_ng_command_duration_seconds_bucket[5m]))

# 错误率
sum(rate(karl_ng_commands_total{result="error"}[5m])) / sum(rate(karl_ng_commands_total[5m]))
```

### API 指标

| 指标 | 类型 | 说明 |
|------|------|------|
| `karl_api_requests_total` | Counter | 按端点分类的 API 请求数 |
| `karl_api_request_duration_seconds` | Histogram | 请求处理时间 |

---

## Grafana 仪表板

### 导入仪表板

1. 打开 Grafana
2. 进入 Dashboards > Import
3. 上传 `deploy/grafana/karl-dashboard.json` 中的 JSON 文件

### 仪表板面板

**概览行**：
- 活动会话数（仪表盘）
- 每分钟通话数（图表）
- 平均会话时长（统计值）
- 错误率（统计值）

**质量行**：
- RTT 分布（热力图）
- 抖动随时间变化（图表）
- 丢包率（图表）
- FEC 恢复率（统计值）

**性能行**：
- NG 命令延迟（直方图）
- API 请求速率（图表）
- 抖动缓冲区延迟（图表）

### 仪表板 JSON 示例

```json
{
  "dashboard": {
    "title": "Karl Media Server",
    "panels": [
      {
        "title": "Active Sessions",
        "type": "stat",
        "targets": [
          {
            "expr": "karl_sessions_active",
            "legendFormat": "Sessions"
          }
        ],
        "gridPos": { "x": 0, "y": 0, "w": 6, "h": 4 }
      },
      {
        "title": "Sessions Over Time",
        "type": "graph",
        "targets": [
          {
            "expr": "karl_sessions_active",
            "legendFormat": "Active"
          },
          {
            "expr": "rate(karl_sessions_total[5m]) * 60",
            "legendFormat": "Created/min"
          }
        ],
        "gridPos": { "x": 6, "y": 0, "w": 18, "h": 8 }
      },
      {
        "title": "Packet Loss",
        "type": "graph",
        "targets": [
          {
            "expr": "avg(karl_rtcp_packet_loss_fraction) * 100",
            "legendFormat": "Avg Loss %"
          },
          {
            "expr": "max(karl_rtcp_packet_loss_fraction) * 100",
            "legendFormat": "Max Loss %"
          }
        ],
        "gridPos": { "x": 0, "y": 8, "w": 12, "h": 8 }
      },
      {
        "title": "RTT (ms)",
        "type": "graph",
        "targets": [
          {
            "expr": "avg(karl_rtcp_rtt_seconds) * 1000",
            "legendFormat": "Avg RTT"
          }
        ],
        "gridPos": { "x": 12, "y": 8, "w": 12, "h": 8 }
      }
    ]
  }
}
```

---

## 告警规则

### Prometheus 告警规则

创建 `karl-alerts.yml`：

```yaml
groups:
  - name: karl
    rules:
      # 高丢包率
      - alert: KarlHighPacketLoss
        expr: avg(karl_rtcp_packet_loss_fraction) > 0.05
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "检测到高丢包率"
          description: "平均丢包率为 {{ $value | humanizePercentage }}"

      # 极高丢包率
      - alert: KarlCriticalPacketLoss
        expr: avg(karl_rtcp_packet_loss_fraction) > 0.15
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "严重丢包"
          description: "丢包率达到 {{ $value | humanizePercentage }}，通话可能受到影响"

      # 高抖动
      - alert: KarlHighJitter
        expr: avg(karl_rtcp_jitter_seconds) * 1000 > 50
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "检测到高抖动"
          description: "平均抖动为 {{ $value }}ms"

      # 会话限制接近
      - alert: KarlSessionLimitApproaching
        expr: karl_sessions_active / 10000 > 0.8
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "会话限制即将达到"
          description: "已使用 {{ $value | humanizePercentage }} 的会话限制"

      # 无活动会话（可能表示存在问题）
      - alert: KarlNoSessions
        expr: karl_sessions_active == 0
        for: 30m
        labels:
          severity: info
        annotations:
          summary: "无活动会话"
          description: "Karl 在 30 分钟内没有活动会话"

      # NG 协议错误
      - alert: KarlNGErrors
        expr: sum(rate(karl_ng_commands_total{result="error"}[5m])) > 1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "NG 协议错误"
          description: "每秒 {{ $value }} 个错误"

      # 高命令延迟
      - alert: KarlHighCommandLatency
        expr: histogram_quantile(0.99, rate(karl_ng_command_duration_seconds_bucket[5m])) > 0.5
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "NG 命令延迟过高"
          description: "P99 延迟为 {{ $value }}s"

      # Karl 实例宕机
      - alert: KarlDown
        expr: up{job="karl"} == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Karl 实例宕机"
          description: "Karl 实例 {{ $labels.instance }} 无响应"
```

### 加载告警规则

```yaml
# prometheus.yml
rule_files:
  - "karl-alerts.yml"
```

### Alertmanager 集成

```yaml
# alertmanager.yml
route:
  receiver: 'karl-alerts'
  group_by: ['alertname']
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 3h

receivers:
  - name: 'karl-alerts'
    email_configs:
      - to: 'ops@example.com'
        from: 'alertmanager@example.com'
        smarthost: 'smtp.example.com:587'
    slack_configs:
      - api_url: 'https://hooks.slack.com/services/xxx'
        channel: '#alerts'
```

---

## 最佳实践

### 抓取间隔

- 生产环境：15s（默认）
- 高频监控：5s
- 低流量系统：30s

### 数据保留

```yaml
# prometheus.yml
global:
  scrape_interval: 15s

# 命令行
prometheus --storage.tsdb.retention.time=30d
```

### 预计算规则

预计算开销较大的查询：

```yaml
groups:
  - name: karl_recording_rules
    rules:
      - record: karl:session_rate:5m
        expr: rate(karl_sessions_total[5m])

      - record: karl:avg_packet_loss:5m
        expr: avg(karl_rtcp_packet_loss_fraction)

      - record: karl:p99_command_latency:5m
        expr: histogram_quantile(0.99, rate(karl_ng_command_duration_seconds_bucket[5m]))
```

### 标签

添加有意义的标签：

```yaml
scrape_configs:
  - job_name: 'karl'
    static_configs:
      - targets: ['karl1:9091']
        labels:
          datacenter: 'us-east'
          environment: 'production'
```

---

## 下一步

- [水平扩展](./scaling-horizontally.md)
- [故障排除指南](./troubleshooting.md)
- [配置参考](../configuration.md)
