# Comment surveiller Karl avec Prometheus

Ce guide couvre la mise en place d'une surveillance complète du serveur multimédia Karl à l'aide de Prometheus et Grafana.

## Table des matières

- [Aperçu](#aperçu)
- [Configuration rapide](#configuration-rapide)
- [Configuration de Prometheus](#configuration-de-prometheus)
- [Métriques clés](#métriques-clés)
- [Tableau de bord Grafana](#tableau-de-bord-grafana)
- [Règles d'alerte](#règles-dalerte)
- [Bonnes pratiques](#bonnes-pratiques)

---

## Aperçu

Karl expose les métriques au format Prometheus sur `:9091/metrics`. Ces métriques couvrent :

- Sessions actives et statistiques d'appels
- Métriques de qualité RTCP (gigue, perte de paquets, RTT)
- Statistiques de correction d'erreurs sans retransmission
- Performance du tampon de gigue
- Métriques de requêtes API
- Métriques de commandes du protocole NG

---

## Configuration rapide

### Vérifier le point de terminaison des métriques

```bash
curl http://localhost:9091/metrics | head -50
```

### Configuration Prometheus minimale

```yaml
# prometheus.yml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'karl'
    static_configs:
      - targets: ['localhost:9091']
```

### Démarrer Prometheus

```bash
# Docker
docker run -d \
  --name prometheus \
  -p 9090:9090 \
  -v $(pwd)/prometheus.yml:/etc/prometheus/prometheus.yml \
  prom/prometheus

# Ou natif
prometheus --config.file=prometheus.yml
```

---

## Configuration de Prometheus

### Configuration de base

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

### Plusieurs instances de Karl

```yaml
scrape_configs:
  - job_name: 'karl'
    static_configs:
      - targets:
        - 'karl1.example.com:9091'
        - 'karl2.example.com:9091'
        - 'karl3.example.com:9091'
```

### Découverte de services Kubernetes

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

## Métriques clés

### Métriques de session

| Métrique | Type | Description |
|----------|------|-------------|
| `karl_sessions_active` | Gauge | Sessions actuellement actives |
| `karl_sessions_total` | Counter | Total des sessions créées |
| `karl_session_duration_seconds` | Histogram | Distribution de la durée des sessions |

**Exemples de requêtes** :

```promql
# Sessions actives
karl_sessions_active

# Sessions créées par minute
rate(karl_sessions_total[5m]) * 60

# 95e percentile de la durée des sessions
histogram_quantile(0.95, rate(karl_session_duration_seconds_bucket[5m]))
```

### Métriques de qualité RTCP

| Métrique | Type | Description |
|----------|------|-------------|
| `karl_rtcp_rtt_seconds` | Gauge | Temps aller-retour |
| `karl_rtcp_jitter_seconds` | Gauge | Gigue rapportée |
| `karl_rtcp_packet_loss_fraction` | Gauge | Ratio de perte de paquets (0-1) |
| `karl_rtcp_sr_sent_total` | Counter | Rapports d'émetteur envoyés |
| `karl_rtcp_rr_sent_total` | Counter | Rapports de récepteur envoyés |

**Exemples de requêtes** :

```promql
# RTT moyen
avg(karl_rtcp_rtt_seconds)

# Gigue moyenne en millisecondes
avg(karl_rtcp_jitter_seconds) * 1000

# Sessions avec forte perte de paquets (>5%)
count(karl_rtcp_packet_loss_fraction > 0.05)
```

### Métriques FEC

| Métrique | Type | Description |
|----------|------|-------------|
| `karl_fec_packets_sent_total` | Counter | Paquets FEC envoyés |
| `karl_fec_recoveries_total` | Counter | Récupérations de paquets réussies |
| `karl_fec_recovery_failures_total` | Counter | Tentatives de récupération échouées |
| `karl_fec_redundancy_ratio` | Gauge | Ratio de redondance actuel |

**Exemples de requêtes** :

```promql
# Taux de récupération FEC
rate(karl_fec_recoveries_total[5m]) / rate(karl_fec_packets_sent_total[5m])

# Taux d'échec de récupération
rate(karl_fec_recovery_failures_total[5m])
```

### Métriques du tampon de gigue

| Métrique | Type | Description |
|----------|------|-------------|
| `karl_jitter_buffer_size` | Gauge | Taille actuelle du tampon |
| `karl_jitter_buffer_latency_seconds` | Gauge | Latence du tampon |
| `karl_jitter_buffer_packets_dropped_total` | Counter | Paquets supprimés |

**Exemples de requêtes** :

```promql
# Latence moyenne du tampon
avg(karl_jitter_buffer_latency_seconds) * 1000

# Paquets supprimés par minute
rate(karl_jitter_buffer_packets_dropped_total[5m]) * 60
```

### Métriques du protocole NG

| Métrique | Type | Description |
|----------|------|-------------|
| `karl_ng_commands_total` | Counter | Commandes par type et résultat |
| `karl_ng_command_duration_seconds` | Histogram | Temps de traitement des commandes |
| `karl_ng_active_calls` | Gauge | Appels actifs via le protocole NG |

**Exemples de requêtes** :

```promql
# Commandes par seconde par type
sum(rate(karl_ng_commands_total[5m])) by (command)

# 99e percentile de latence des commandes
histogram_quantile(0.99, rate(karl_ng_command_duration_seconds_bucket[5m]))

# Taux d'erreur
sum(rate(karl_ng_commands_total{result="error"}[5m])) / sum(rate(karl_ng_commands_total[5m]))
```

### Métriques API

| Métrique | Type | Description |
|----------|------|-------------|
| `karl_api_requests_total` | Counter | Requêtes API par point de terminaison |
| `karl_api_request_duration_seconds` | Histogram | Durée des requêtes |

---

## Tableau de bord Grafana

### Importer le tableau de bord

1. Ouvrez Grafana
2. Allez dans Tableaux de bord > Importer
3. Téléchargez le fichier JSON depuis `deploy/grafana/karl-dashboard.json`

### Panneaux du tableau de bord

**Ligne d'aperçu** :
- Sessions actives (jauge)
- Appels par minute (graphique)
- Durée moyenne des sessions (statistique)
- Taux d'erreur (statistique)

**Ligne de qualité** :
- Distribution RTT (heatmap)
- Gigue dans le temps (graphique)
- Perte de paquets (graphique)
- Taux de récupération FEC (statistique)

**Ligne de performance** :
- Latence des commandes NG (histogramme)
- Taux de requêtes API (graphique)
- Latence du tampon de gigue (graphique)

### Exemple de JSON de tableau de bord

```json
{
  "dashboard": {
    "title": "Karl Media Server",
    "panels": [
      {
        "title": "Sessions actives",
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
        "title": "Sessions dans le temps",
        "type": "graph",
        "targets": [
          {
            "expr": "karl_sessions_active",
            "legendFormat": "Actives"
          },
          {
            "expr": "rate(karl_sessions_total[5m]) * 60",
            "legendFormat": "Créées/min"
          }
        ],
        "gridPos": { "x": 6, "y": 0, "w": 18, "h": 8 }
      },
      {
        "title": "Perte de paquets",
        "type": "graph",
        "targets": [
          {
            "expr": "avg(karl_rtcp_packet_loss_fraction) * 100",
            "legendFormat": "Perte moy. %"
          },
          {
            "expr": "max(karl_rtcp_packet_loss_fraction) * 100",
            "legendFormat": "Perte max %"
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
            "legendFormat": "RTT moyen"
          }
        ],
        "gridPos": { "x": 12, "y": 8, "w": 12, "h": 8 }
      }
    ]
  }
}
```

---

## Règles d'alerte

### Règles d'alerte Prometheus

Créez `karl-alerts.yml` :

```yaml
groups:
  - name: karl
    rules:
      # Forte perte de paquets
      - alert: KarlHighPacketLoss
        expr: avg(karl_rtcp_packet_loss_fraction) > 0.05
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Forte perte de paquets détectée"
          description: "La perte de paquets moyenne est de {{ $value | humanizePercentage }}"

      # Perte de paquets critique
      - alert: KarlCriticalPacketLoss
        expr: avg(karl_rtcp_packet_loss_fraction) > 0.15
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "Perte de paquets critique"
          description: "Perte de paquets à {{ $value | humanizePercentage }}, les appels peuvent être affectés"

      # Forte gigue
      - alert: KarlHighJitter
        expr: avg(karl_rtcp_jitter_seconds) * 1000 > 50
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Forte gigue détectée"
          description: "La gigue moyenne est de {{ $value }}ms"

      # Limite de sessions approchant
      - alert: KarlSessionLimitApproaching
        expr: karl_sessions_active / 10000 > 0.8
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Limite de sessions approchant"
          description: "{{ $value | humanizePercentage }} de la limite de sessions utilisée"

      # Pas de sessions actives (peut indiquer un problème)
      - alert: KarlNoSessions
        expr: karl_sessions_active == 0
        for: 30m
        labels:
          severity: info
        annotations:
          summary: "Aucune session active"
          description: "Karl n'a aucune session active depuis 30 minutes"

      # Erreurs du protocole NG
      - alert: KarlNGErrors
        expr: sum(rate(karl_ng_commands_total{result="error"}[5m])) > 1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Erreurs du protocole NG"
          description: "{{ $value }} erreurs par seconde"

      # Latence élevée des commandes
      - alert: KarlHighCommandLatency
        expr: histogram_quantile(0.99, rate(karl_ng_command_duration_seconds_bucket[5m])) > 0.5
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Latence élevée des commandes NG"
          description: "Latence P99 de {{ $value }}s"

      # Instance Karl hors service
      - alert: KarlDown
        expr: up{job="karl"} == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Instance Karl hors service"
          description: "L'instance Karl {{ $labels.instance }} ne répond pas"
```

### Charger les règles d'alerte

```yaml
# prometheus.yml
rule_files:
  - "karl-alerts.yml"
```

### Intégration Alertmanager

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

## Bonnes pratiques

### Intervalle de scrape

- Production : 15s (défaut)
- Surveillance haute fréquence : 5s
- Systèmes à faible trafic : 30s

### Rétention

```yaml
# prometheus.yml
global:
  scrape_interval: 15s

# Ligne de commande
prometheus --storage.tsdb.retention.time=30d
```

### Règles d'enregistrement

Pré-calculer les requêtes coûteuses :

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

Ajouter des labels significatifs :

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

## Étapes suivantes

- [Mise à l'échelle horizontale](./scaling-horizontally.md)
- [Guide de dépannage](./troubleshooting.md)
- [Référence de configuration](../configuration.md)
