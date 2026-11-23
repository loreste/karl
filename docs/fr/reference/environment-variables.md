# Référence des variables d'environnement

Liste complète des variables d'environnement prises en charge par le serveur multimédia Karl.

## Table des matières

- [Configuration de base](#configuration-de-base)
- [Configuration réseau](#configuration-réseau)
- [Configuration des sessions](#configuration-des-sessions)
- [Configuration de l'enregistrement](#configuration-de-lenregistrement)
- [Configuration de la base de données](#configuration-de-la-base-de-données)
- [Configuration de la surveillance](#configuration-de-la-surveillance)
- [Configuration WebRTC](#configuration-webrtc)
- [Configuration avancée](#configuration-avancée)

---

## Configuration de base

| Variable | Défaut | Description |
|----------|--------|-------------|
| `KARL_CONFIG_PATH` | `./config/config.json` | Chemin vers le fichier de configuration JSON |
| `KARL_LOG_LEVEL` | `info` | Niveau de journalisation : `debug`, `info`, `warn`, `error` |
| `KARL_RUN_DIR` | `./run/karl` | Répertoire d'exécution pour les sockets et fichiers PID |
| `KARL_ENVIRONMENT` | `production` | Nom de l'environnement : `production`, `staging`, `development` |

### Exemples

```bash
# Utiliser un fichier de configuration personnalisé
export KARL_CONFIG_PATH=/etc/karl/config.json

# Activer la journalisation de débogage
export KARL_LOG_LEVEL=debug

# Définir le répertoire d'exécution
export KARL_RUN_DIR=/var/run/karl
```

---

## Configuration réseau

| Variable | Défaut | Description |
|----------|--------|-------------|
| `KARL_NG_PORT` | `22222` | Port UDP pour le protocole NG |
| `KARL_API_PORT` | `:8080` | Port TCP pour l'API REST |
| `KARL_HEALTH_PORT` | `:8086` | Port TCP pour les points de terminaison de santé |
| `KARL_METRICS_PORT` | `:9091` | Port TCP pour les métriques Prometheus |
| `KARL_MEDIA_IP` | `auto` | Adresse IP pour le multimédia (SDP). Utilisez `auto` pour la détection automatique |
| `KARL_PUBLIC_IP` | (auto) | IP publique pour les scénarios NAT |

### Exemples

```bash
# Définir des ports spécifiques
export KARL_NG_PORT=22223
export KARL_API_PORT=:8081
export KARL_HEALTH_PORT=:8087
export KARL_METRICS_PORT=:9092

# Définir l'IP multimédia explicitement
export KARL_MEDIA_IP=192.168.1.100

# Définir l'IP publique pour le NAT
export KARL_PUBLIC_IP=203.0.113.50
```

---

## Configuration des sessions

| Variable | Défaut | Description |
|----------|--------|-------------|
| `KARL_RTP_MIN_PORT` | `30000` | Port minimum pour le multimédia RTP |
| `KARL_RTP_MAX_PORT` | `40000` | Port maximum pour le multimédia RTP |
| `KARL_MAX_SESSIONS` | `10000` | Nombre maximum de sessions simultanées |
| `KARL_SESSION_TTL` | `3600` | Délai d'expiration de session en secondes |
| `KARL_CLEANUP_INTERVAL` | `60` | Intervalle de nettoyage des sessions obsolètes (secondes) |

### Exemples

```bash
# Limiter la plage de ports RTP
export KARL_RTP_MIN_PORT=30000
export KARL_RTP_MAX_PORT=35000

# Augmenter la capacité de sessions
export KARL_MAX_SESSIONS=20000

# Délai d'expiration de session plus court
export KARL_SESSION_TTL=1800
```

### Calcul de la plage de ports

Chaque session nécessite 4 ports (RTP + RTCP x 2 directions) :
```
Sessions max = (MAX_PORT - MIN_PORT + 1) / 4
```

Défaut : (40000 - 30000 + 1) / 4 = 2 500 sessions

---

## Configuration de l'enregistrement

| Variable | Défaut | Description |
|----------|--------|-------------|
| `KARL_RECORDING_ENABLED` | `false` | Activer/désactiver l'enregistrement des appels |
| `KARL_RECORDING_PATH` | `/var/lib/karl/recordings` | Répertoire pour les fichiers d'enregistrement |
| `KARL_RECORDING_FORMAT` | `wav` | Format de sortie : `wav`, `pcm` |
| `KARL_RECORDING_MODE` | `stereo` | Mode d'enregistrement : `mixed`, `stereo`, `separate` |
| `KARL_RECORDING_SAMPLE_RATE` | `8000` | Taux d'échantillonnage en Hz |
| `KARL_RECORDING_RETENTION_DAYS` | `30` | Jours de conservation des enregistrements |

### Exemples

```bash
# Activer l'enregistrement
export KARL_RECORDING_ENABLED=true
export KARL_RECORDING_PATH=/mnt/recordings

# Configurer l'enregistrement stéréo
export KARL_RECORDING_MODE=stereo
export KARL_RECORDING_SAMPLE_RATE=16000

# Définir la rétention à 7 jours
export KARL_RECORDING_RETENTION_DAYS=7
```

---

## Configuration de la base de données

| Variable | Défaut | Description |
|----------|--------|-------------|
| `KARL_MYSQL_DSN` | (vide) | Chaîne de connexion MySQL |
| `KARL_REDIS_ENABLED` | `false` | Activer Redis pour le partage de sessions |
| `KARL_REDIS_ADDR` | `localhost:6379` | Adresse du serveur Redis |
| `KARL_REDIS_PASSWORD` | (vide) | Mot de passe Redis |
| `KARL_REDIS_DB` | `0` | Numéro de base de données Redis |

### Exemples

```bash
# Configuration MySQL
export KARL_MYSQL_DSN="karl:password@tcp(mysql:3306)/karl?parseTime=true"

# Configuration Redis pour le clustering
export KARL_REDIS_ENABLED=true
export KARL_REDIS_ADDR=redis.example.com:6379
export KARL_REDIS_PASSWORD=secretpassword
export KARL_REDIS_DB=0
```

### Format du DSN MySQL

```
user:password@tcp(host:port)/database?parseTime=true
```

---

## Configuration de la surveillance

| Variable | Défaut | Description |
|----------|--------|-------------|
| `KARL_METRICS_ENABLED` | `true` | Activer les métriques Prometheus |
| `KARL_HEALTH_CHECK_INTERVAL` | `30` | Intervalle de vérification de santé (secondes) |

### Exemples

```bash
# Désactiver les métriques (non recommandé)
export KARL_METRICS_ENABLED=false

# Vérifications de santé plus rapides
export KARL_HEALTH_CHECK_INTERVAL=15
```

---

## Configuration WebRTC

| Variable | Défaut | Description |
|----------|--------|-------------|
| `KARL_WEBRTC_ENABLED` | `true` | Activer le support WebRTC |
| `KARL_STUN_SERVERS` | `stun:stun.l.google.com:19302` | Serveurs STUN (séparés par des virgules) |
| `KARL_TURN_SERVERS` | (vide) | Serveurs TURN (format JSON) |
| `KARL_WEBRTC_MAX_BITRATE` | `2000000` | Débit binaire maximum (bps) |

### Exemples

```bash
# Activer WebRTC avec STUN personnalisé
export KARL_WEBRTC_ENABLED=true
export KARL_STUN_SERVERS="stun:stun1.example.com:3478,stun:stun2.example.com:3478"

# Configurer TURN (format JSON)
export KARL_TURN_SERVERS='[{"url":"turn:turn.example.com:3478","username":"user","credential":"pass"}]'

# Limiter le débit binaire
export KARL_WEBRTC_MAX_BITRATE=1000000
```

---

## Configuration avancée

### Tampon de gigue

| Variable | Défaut | Description |
|----------|--------|-------------|
| `KARL_JITTER_BUFFER_ENABLED` | `true` | Activer le tampon de gigue |
| `KARL_JITTER_BUFFER_MIN_DELAY` | `20` | Délai minimum (ms) |
| `KARL_JITTER_BUFFER_MAX_DELAY` | `200` | Délai maximum (ms) |
| `KARL_JITTER_BUFFER_TARGET_DELAY` | `50` | Délai cible (ms) |

### Correction d'erreurs sans retransmission

| Variable | Défaut | Description |
|----------|--------|-------------|
| `KARL_FEC_ENABLED` | `true` | Activer FEC |
| `KARL_FEC_REDUNDANCY` | `0.30` | Ratio de redondance (0.0-1.0) |
| `KARL_FEC_ADAPTIVE_MODE` | `true` | Activer le FEC adaptatif |

### Sécurité de l'API

| Variable | Défaut | Description |
|----------|--------|-------------|
| `KARL_API_AUTH_ENABLED` | `false` | Activer l'authentification API |
| `KARL_API_RATE_LIMIT` | `60` | Requêtes par minute par client |
| `KARL_API_CORS_ENABLED` | `false` | Activer CORS |
| `KARL_API_CORS_ORIGINS` | `*` | Origines CORS autorisées |
| `KARL_API_TLS_ENABLED` | `false` | Activer HTTPS |
| `KARL_API_TLS_CERT` | (vide) | Chemin vers le certificat TLS |
| `KARL_API_TLS_KEY` | (vide) | Chemin vers la clé privée TLS |

### Exemples

```bash
# Régler le tampon de gigue pour un réseau à haute latence
export KARL_JITTER_BUFFER_MIN_DELAY=40
export KARL_JITTER_BUFFER_MAX_DELAY=300
export KARL_JITTER_BUFFER_TARGET_DELAY=100

# Augmenter le FEC pour un réseau avec pertes
export KARL_FEC_REDUNDANCY=0.50
export KARL_FEC_ADAPTIVE_MODE=true

# Activer la sécurité de l'API
export KARL_API_AUTH_ENABLED=true
export KARL_API_RATE_LIMIT=120
export KARL_API_TLS_ENABLED=true
export KARL_API_TLS_CERT=/etc/karl/certs/server.crt
export KARL_API_TLS_KEY=/etc/karl/certs/server.key
```

---

## Alertes

| Variable | Défaut | Description |
|----------|--------|-------------|
| `KARL_ALERT_PACKET_LOSS_THRESHOLD` | `0.05` | Alerter lorsque la perte de paquets dépasse 5% |
| `KARL_ALERT_JITTER_THRESHOLD` | `50` | Alerter lorsque la gigue dépasse 50 ms |
| `KARL_ALERT_NOTIFY_ADMIN` | `false` | Envoyer des notifications d'alerte |
| `KARL_ALERT_ADMIN_EMAIL` | (vide) | E-mail de l'administrateur pour les alertes |
| `KARL_ALERT_SLACK_WEBHOOK` | (vide) | URL du webhook Slack |

---

## Ordre de priorité

Les variables d'environnement remplacent les valeurs du fichier de configuration :

1. Variables d'environnement (priorité la plus élevée)
2. Fichier de configuration
3. Valeurs par défaut (priorité la plus basse)

---

## Exemple ConfigMap Kubernetes

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: karl-env
data:
  KARL_LOG_LEVEL: "info"
  KARL_NG_PORT: "22222"
  KARL_RTP_MIN_PORT: "30000"
  KARL_RTP_MAX_PORT: "40000"
  KARL_MAX_SESSIONS: "10000"
  KARL_REDIS_ENABLED: "true"
  KARL_REDIS_ADDR: "redis:6379"
  KARL_RECORDING_ENABLED: "true"
  KARL_RECORDING_PATH: "/var/lib/karl/recordings"
```

---

## Exemple Docker Compose

```yaml
services:
  karl:
    image: loreste/karl:latest
    environment:
      - KARL_LOG_LEVEL=info
      - KARL_NG_PORT=22222
      - KARL_RTP_MIN_PORT=30000
      - KARL_RTP_MAX_PORT=40000
      - KARL_REDIS_ENABLED=true
      - KARL_REDIS_ADDR=redis:6379
      - KARL_RECORDING_ENABLED=true
```

---

## Étapes suivantes

- [Référence de configuration](../configuration.md)
- [Guide d'installation](../installation.md)
- [Déploiement Kubernetes](../how-to/deploying-kubernetes.md)
