# Como Monitorear Karl con Prometheus

Esta guia cubre la configuracion de monitoreo integral para el Servidor de Medios Karl usando Prometheus y Grafana.

## Tabla de Contenidos

- [Resumen](#resumen)
- [Configuracion Rapida](#configuracion-rapida)
- [Configuracion de Prometheus](#configuracion-de-prometheus)
- [Metricas Clave](#metricas-clave)
- [Dashboard de Grafana](#dashboard-de-grafana)
- [Reglas de Alerta](#reglas-de-alerta)
- [Mejores Practicas](#mejores-practicas)

---

## Resumen

Karl expone metricas en formato Prometheus en `:9091/metrics`. Estas metricas cubren:

- Sesiones activas y estadisticas de llamadas
- Metricas de calidad RTCP (jitter, perdida de paquetes, RTT)
- Estadisticas de Correccion de Errores hacia Adelante
- Rendimiento del jitter buffer
- Metricas de solicitudes API
- Metricas de comandos del protocolo NG

---

## Configuracion Rapida

### Verificar el Endpoint de Metricas

```bash
curl http://localhost:9091/metrics | head -50
```

### Configuracion Minima de Prometheus

```yaml
# prometheus.yml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'karl'
    static_configs:
      - targets: ['localhost:9091']
```

### Iniciar Prometheus

```bash
# Docker
docker run -d \
  --name prometheus \
  -p 9090:9090 \
  -v $(pwd)/prometheus.yml:/etc/prometheus/prometheus.yml \
  prom/prometheus

# O nativo
prometheus --config.file=prometheus.yml
```

---

## Configuracion de Prometheus

### Configuracion Basica

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

### Multiples Instancias de Karl

```yaml
scrape_configs:
  - job_name: 'karl'
    static_configs:
      - targets:
        - 'karl1.example.com:9091'
        - 'karl2.example.com:9091'
        - 'karl3.example.com:9091'
```

### Descubrimiento de Servicios en Kubernetes

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

## Metricas Clave

### Metricas de Sesion

| Metrica | Tipo | Descripcion |
|---------|------|-------------|
| `karl_sessions_active` | Gauge | Sesiones activas actualmente |
| `karl_sessions_total` | Counter | Total de sesiones creadas |
| `karl_session_duration_seconds` | Histogram | Distribucion de duracion de sesiones |

**Consultas de Ejemplo**:

```promql
# Sesiones activas
karl_sessions_active

# Sesiones creadas por minuto
rate(karl_sessions_total[5m]) * 60

# Percentil 95 de duracion de sesion
histogram_quantile(0.95, rate(karl_session_duration_seconds_bucket[5m]))
```

### Metricas de Calidad RTCP

| Metrica | Tipo | Descripcion |
|---------|------|-------------|
| `karl_rtcp_rtt_seconds` | Gauge | Tiempo de ida y vuelta |
| `karl_rtcp_jitter_seconds` | Gauge | Jitter reportado |
| `karl_rtcp_packet_loss_fraction` | Gauge | Ratio de perdida de paquetes (0-1) |
| `karl_rtcp_sr_sent_total` | Counter | Reportes de emisor enviados |
| `karl_rtcp_rr_sent_total` | Counter | Reportes de receptor enviados |

**Consultas de Ejemplo**:

```promql
# RTT promedio
avg(karl_rtcp_rtt_seconds)

# Jitter promedio en milisegundos
avg(karl_rtcp_jitter_seconds) * 1000

# Sesiones con alta perdida de paquetes (>5%)
count(karl_rtcp_packet_loss_fraction > 0.05)
```

### Metricas FEC

| Metrica | Tipo | Descripcion |
|---------|------|-------------|
| `karl_fec_packets_sent_total` | Counter | Paquetes FEC enviados |
| `karl_fec_recoveries_total` | Counter | Recuperaciones exitosas de paquetes |
| `karl_fec_recovery_failures_total` | Counter | Intentos fallidos de recuperacion |
| `karl_fec_redundancy_ratio` | Gauge | Ratio de redundancia actual |

**Consultas de Ejemplo**:

```promql
# Tasa de recuperacion FEC
rate(karl_fec_recoveries_total[5m]) / rate(karl_fec_packets_sent_total[5m])

# Tasa de fallos de recuperacion
rate(karl_fec_recovery_failures_total[5m])
```

### Metricas del Jitter Buffer

| Metrica | Tipo | Descripcion |
|---------|------|-------------|
| `karl_jitter_buffer_size` | Gauge | Tamano actual del buffer |
| `karl_jitter_buffer_latency_seconds` | Gauge | Latencia del buffer |
| `karl_jitter_buffer_packets_dropped_total` | Counter | Paquetes descartados |

**Consultas de Ejemplo**:

```promql
# Latencia promedio del buffer
avg(karl_jitter_buffer_latency_seconds) * 1000

# Paquetes descartados por minuto
rate(karl_jitter_buffer_packets_dropped_total[5m]) * 60
```

### Metricas del Protocolo NG

| Metrica | Tipo | Descripcion |
|---------|------|-------------|
| `karl_ng_commands_total` | Counter | Comandos por tipo y resultado |
| `karl_ng_command_duration_seconds` | Histogram | Tiempo de procesamiento de comandos |
| `karl_ng_active_calls` | Gauge | Llamadas activas via protocolo NG |

**Consultas de Ejemplo**:

```promql
# Comandos por segundo por tipo
sum(rate(karl_ng_commands_total[5m])) by (command)

# Latencia percentil 99 de comandos
histogram_quantile(0.99, rate(karl_ng_command_duration_seconds_bucket[5m]))

# Tasa de errores
sum(rate(karl_ng_commands_total{result="error"}[5m])) / sum(rate(karl_ng_commands_total[5m]))
```

### Metricas de API

| Metrica | Tipo | Descripcion |
|---------|------|-------------|
| `karl_api_requests_total` | Counter | Solicitudes API por endpoint |
| `karl_api_request_duration_seconds` | Histogram | Duracion de solicitudes |

---

## Dashboard de Grafana

### Importar Dashboard

1. Abrir Grafana
2. Ir a Dashboards > Import
3. Subir el archivo JSON desde `deploy/grafana/karl-dashboard.json`

### Paneles del Dashboard

**Fila de Resumen**:
- Sesiones Activas (gauge)
- Llamadas por Minuto (grafico)
- Duracion Promedio de Sesion (stat)
- Tasa de Errores (stat)

**Fila de Calidad**:
- Distribucion de RTT (heatmap)
- Jitter a lo Largo del Tiempo (grafico)
- Perdida de Paquetes (grafico)
- Tasa de Recuperacion FEC (stat)

**Fila de Rendimiento**:
- Latencia de Comandos NG (histograma)
- Tasa de Solicitudes API (grafico)
- Latencia del Jitter Buffer (grafico)

### Ejemplo de JSON del Dashboard

```json
{
  "dashboard": {
    "title": "Karl Media Server",
    "panels": [
      {
        "title": "Sesiones Activas",
        "type": "stat",
        "targets": [
          {
            "expr": "karl_sessions_active",
            "legendFormat": "Sesiones"
          }
        ],
        "gridPos": { "x": 0, "y": 0, "w": 6, "h": 4 }
      },
      {
        "title": "Sesiones a lo Largo del Tiempo",
        "type": "graph",
        "targets": [
          {
            "expr": "karl_sessions_active",
            "legendFormat": "Activas"
          },
          {
            "expr": "rate(karl_sessions_total[5m]) * 60",
            "legendFormat": "Creadas/min"
          }
        ],
        "gridPos": { "x": 6, "y": 0, "w": 18, "h": 8 }
      },
      {
        "title": "Perdida de Paquetes",
        "type": "graph",
        "targets": [
          {
            "expr": "avg(karl_rtcp_packet_loss_fraction) * 100",
            "legendFormat": "Perdida Promedio %"
          },
          {
            "expr": "max(karl_rtcp_packet_loss_fraction) * 100",
            "legendFormat": "Perdida Maxima %"
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
            "legendFormat": "RTT Promedio"
          }
        ],
        "gridPos": { "x": 12, "y": 8, "w": 12, "h": 8 }
      }
    ]
  }
}
```

---

## Reglas de Alerta

### Reglas de Alerta de Prometheus

Crear `karl-alerts.yml`:

```yaml
groups:
  - name: karl
    rules:
      # Alta perdida de paquetes
      - alert: KarlHighPacketLoss
        expr: avg(karl_rtcp_packet_loss_fraction) > 0.05
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Alta perdida de paquetes detectada"
          description: "La perdida promedio de paquetes es {{ $value | humanizePercentage }}"

      # Perdida de paquetes muy alta
      - alert: KarlCriticalPacketLoss
        expr: avg(karl_rtcp_packet_loss_fraction) > 0.15
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "Perdida de paquetes critica"
          description: "Perdida de paquetes en {{ $value | humanizePercentage }}, las llamadas pueden verse afectadas"

      # Alto jitter
      - alert: KarlHighJitter
        expr: avg(karl_rtcp_jitter_seconds) * 1000 > 50
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Alto jitter detectado"
          description: "El jitter promedio es {{ $value }}ms"

      # Limite de sesiones acercandose
      - alert: KarlSessionLimitApproaching
        expr: karl_sessions_active / 10000 > 0.8
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Limite de sesiones acercandose"
          description: "{{ $value | humanizePercentage }} del limite de sesiones usado"

      # Sin sesiones activas (podria indicar problemas)
      - alert: KarlNoSessions
        expr: karl_sessions_active == 0
        for: 30m
        labels:
          severity: info
        annotations:
          summary: "Sin sesiones activas"
          description: "Karl no tiene sesiones activas durante 30 minutos"

      # Errores del protocolo NG
      - alert: KarlNGErrors
        expr: sum(rate(karl_ng_commands_total{result="error"}[5m])) > 1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Errores del protocolo NG"
          description: "{{ $value }} errores por segundo"

      # Alta latencia de comandos
      - alert: KarlHighCommandLatency
        expr: histogram_quantile(0.99, rate(karl_ng_command_duration_seconds_bucket[5m])) > 0.5
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Alta latencia de comandos NG"
          description: "Latencia P99 es {{ $value }}s"

      # Instancia de Karl caida
      - alert: KarlDown
        expr: up{job="karl"} == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Instancia de Karl caida"
          description: "La instancia de Karl {{ $labels.instance }} no responde"
```

### Cargar Reglas de Alerta

```yaml
# prometheus.yml
rule_files:
  - "karl-alerts.yml"
```

### Integracion con Alertmanager

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

## Mejores Practicas

### Intervalo de Scrape

- Produccion: 15s (predeterminado)
- Monitoreo de alta frecuencia: 5s
- Sistemas de bajo trafico: 30s

### Retencion

```yaml
# prometheus.yml
global:
  scrape_interval: 15s

# Linea de comandos
prometheus --storage.tsdb.retention.time=30d
```

### Reglas de Grabacion

Pre-calcular consultas costosas:

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

### Etiquetas

Agregar etiquetas significativas:

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

## Siguientes Pasos

- [Escalar Horizontalmente](./scaling-horizontally.md)
- [Guia de Solucion de Problemas](./troubleshooting.md)
- [Referencia de Configuracion](../configuration.md)
