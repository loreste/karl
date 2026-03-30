# How to Monitor Karl with Prometheus

This guide covers setting up comprehensive monitoring for Karl Media Server using Prometheus and Grafana.

## Table of Contents

- [Overview](#overview)
- [Quick Setup](#quick-setup)
- [Prometheus Configuration](#prometheus-configuration)
- [Key Metrics](#key-metrics)
- [Grafana Dashboard](#grafana-dashboard)
- [Alerting Rules](#alerting-rules)
- [Best Practices](#best-practices)

---

## Overview

Karl exposes metrics in Prometheus format at `:9091/metrics`. These metrics cover:

- Active sessions and call statistics
- RTCP quality metrics (jitter, packet loss, RTT)
- Forward Error Correction statistics
- Jitter buffer performance
- API request metrics
- NG protocol command metrics

---

## Quick Setup

### Verify Metrics Endpoint

```bash
curl http://localhost:9091/metrics | head -50
```

### Minimal Prometheus Config

```yaml
# prometheus.yml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'karl'
    static_configs:
      - targets: ['localhost:9091']
```

### Start Prometheus

```bash
# Docker
docker run -d \
  --name prometheus \
  -p 9090:9090 \
  -v $(pwd)/prometheus.yml:/etc/prometheus/prometheus.yml \
  prom/prometheus

# Or native
prometheus --config.file=prometheus.yml
```

---

## Prometheus Configuration

### Basic Configuration

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

### Multiple Karl Instances

```yaml
scrape_configs:
  - job_name: 'karl'
    static_configs:
      - targets:
        - 'karl1.example.com:9091'
        - 'karl2.example.com:9091'
        - 'karl3.example.com:9091'
```

### Kubernetes Service Discovery

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

### Prometheus Operator (ServiceMonitor)

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

## Key Metrics

### Session Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `karl_sessions_active` | Gauge | Currently active sessions |
| `karl_sessions_total` | Counter | Total sessions created |
| `karl_session_duration_seconds` | Histogram | Session duration distribution |

**Example Queries**:

```promql
# Active sessions
karl_sessions_active

# Sessions created per minute
rate(karl_sessions_total[5m]) * 60

# 95th percentile session duration
histogram_quantile(0.95, rate(karl_session_duration_seconds_bucket[5m]))
```

### RTCP Quality Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `karl_rtcp_rtt_seconds` | Gauge | Round-trip time |
| `karl_rtcp_jitter_seconds` | Gauge | Reported jitter |
| `karl_rtcp_packet_loss_fraction` | Gauge | Packet loss ratio (0-1) |
| `karl_rtcp_sr_sent_total` | Counter | Sender reports sent |
| `karl_rtcp_rr_sent_total` | Counter | Receiver reports sent |

**Example Queries**:

```promql
# Average RTT
avg(karl_rtcp_rtt_seconds)

# Average jitter in milliseconds
avg(karl_rtcp_jitter_seconds) * 1000

# Sessions with high packet loss (>5%)
count(karl_rtcp_packet_loss_fraction > 0.05)
```

### FEC Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `karl_fec_packets_sent_total` | Counter | FEC packets sent |
| `karl_fec_recoveries_total` | Counter | Successful packet recoveries |
| `karl_fec_recovery_failures_total` | Counter | Failed recovery attempts |
| `karl_fec_redundancy_ratio` | Gauge | Current redundancy ratio |

**Example Queries**:

```promql
# FEC recovery rate
rate(karl_fec_recoveries_total[5m]) / rate(karl_fec_packets_sent_total[5m])

# Recovery failure rate
rate(karl_fec_recovery_failures_total[5m])
```

### Jitter Buffer Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `karl_jitter_buffer_size` | Gauge | Current buffer size |
| `karl_jitter_buffer_latency_seconds` | Gauge | Buffer latency |
| `karl_jitter_buffer_packets_dropped_total` | Counter | Dropped packets |

**Example Queries**:

```promql
# Average buffer latency
avg(karl_jitter_buffer_latency_seconds) * 1000

# Packets dropped per minute
rate(karl_jitter_buffer_packets_dropped_total[5m]) * 60
```

### NG Protocol Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `karl_ng_commands_total` | Counter | Commands by type and result |
| `karl_ng_command_duration_seconds` | Histogram | Command processing time |
| `karl_ng_active_calls` | Gauge | Active calls via NG protocol |

**Example Queries**:

```promql
# Commands per second by type
sum(rate(karl_ng_commands_total[5m])) by (command)

# 99th percentile command latency
histogram_quantile(0.99, rate(karl_ng_command_duration_seconds_bucket[5m]))

# Error rate
sum(rate(karl_ng_commands_total{result="error"}[5m])) / sum(rate(karl_ng_commands_total[5m]))
```

### API Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `karl_api_requests_total` | Counter | API requests by endpoint |
| `karl_api_request_duration_seconds` | Histogram | Request duration |

---

## Grafana Dashboard

### Import Dashboard

1. Open Grafana
2. Go to Dashboards > Import
3. Upload the JSON file from `deploy/grafana/karl-dashboard.json`

### Dashboard Panels

**Overview Row**:
- Active Sessions (gauge)
- Calls per Minute (graph)
- Average Session Duration (stat)
- Error Rate (stat)

**Quality Row**:
- RTT Distribution (heatmap)
- Jitter Over Time (graph)
- Packet Loss (graph)
- FEC Recovery Rate (stat)

**Performance Row**:
- NG Command Latency (histogram)
- API Request Rate (graph)
- Jitter Buffer Latency (graph)

### Sample Dashboard JSON

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

## Alerting Rules

### Prometheus Alert Rules

Create `karl-alerts.yml`:

```yaml
groups:
  - name: karl
    rules:
      # High packet loss
      - alert: KarlHighPacketLoss
        expr: avg(karl_rtcp_packet_loss_fraction) > 0.05
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High packet loss detected"
          description: "Average packet loss is {{ $value | humanizePercentage }}"

      # Very high packet loss
      - alert: KarlCriticalPacketLoss
        expr: avg(karl_rtcp_packet_loss_fraction) > 0.15
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "Critical packet loss"
          description: "Packet loss at {{ $value | humanizePercentage }}, calls may be affected"

      # High jitter
      - alert: KarlHighJitter
        expr: avg(karl_rtcp_jitter_seconds) * 1000 > 50
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High jitter detected"
          description: "Average jitter is {{ $value }}ms"

      # Session limit approaching
      - alert: KarlSessionLimitApproaching
        expr: karl_sessions_active / 10000 > 0.8
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Session limit approaching"
          description: "{{ $value | humanizePercentage }} of session limit used"

      # No active sessions (might indicate issues)
      - alert: KarlNoSessions
        expr: karl_sessions_active == 0
        for: 30m
        labels:
          severity: info
        annotations:
          summary: "No active sessions"
          description: "Karl has no active sessions for 30 minutes"

      # NG protocol errors
      - alert: KarlNGErrors
        expr: sum(rate(karl_ng_commands_total{result="error"}[5m])) > 1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "NG protocol errors"
          description: "{{ $value }} errors per second"

      # High command latency
      - alert: KarlHighCommandLatency
        expr: histogram_quantile(0.99, rate(karl_ng_command_duration_seconds_bucket[5m])) > 0.5
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High NG command latency"
          description: "P99 latency is {{ $value }}s"

      # Karl instance down
      - alert: KarlDown
        expr: up{job="karl"} == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Karl instance down"
          description: "Karl instance {{ $labels.instance }} is not responding"
```

### Load Alert Rules

```yaml
# prometheus.yml
rule_files:
  - "karl-alerts.yml"
```

### Alertmanager Integration

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

## Best Practices

### Scrape Interval

- Production: 15s (default)
- High-frequency monitoring: 5s
- Low-traffic systems: 30s

### Retention

```yaml
# prometheus.yml
global:
  scrape_interval: 15s

# Command line
prometheus --storage.tsdb.retention.time=30d
```

### Recording Rules

Pre-compute expensive queries:

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

### Labels

Add meaningful labels:

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

## Next Steps

- [Scale Horizontally](./scaling-horizontally.md)
- [Troubleshooting Guide](./troubleshooting.md)
- [Configuration Reference](../configuration.md)
