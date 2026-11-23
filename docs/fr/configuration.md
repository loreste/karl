# Référence de configuration

Karl utilise un fichier de configuration JSON avec des valeurs par défaut raisonnables. Tous les paramètres peuvent être remplacés via des variables d'environnement.

## Table des matières

- [Emplacement du fichier de configuration](#emplacement-du-fichier-de-configuration)
- [Exemple de configuration complète](#exemple-de-configuration-complète)
- [Sections de configuration](#sections-de-configuration)
  - [Protocole NG](#protocole-ng)
  - [Sessions](#sessions)
  - [Tampon de gigue](#tampon-de-gigue)
  - [RTCP](#rtcp)
  - [Correction d'erreurs sans retransmission](#correction-derreurs-sans-retransmission)
  - [Enregistrement](#enregistrement)
  - [API REST](#api-rest)
  - [WebRTC](#webrtc)
  - [Intégration](#intégration)
  - [Base de données](#base-de-données)
  - [SRTP](#srtp)
  - [Alertes](#alertes)
  - [HEP / Homer](#hep--homer)
  - [Cluster](#cluster)
- [Variables d'environnement](#variables-denvironnement)

---

## Emplacement du fichier de configuration

Karl recherche la configuration dans l'ordre suivant :

1. Chemin spécifié via le drapeau `-config` : `./karl -config /path/to/config.json`
2. Chemin spécifié via la variable d'environnement `KARL_CONFIG_PATH`
3. Par défaut : `./config/config.json`

## Mises à jour dynamiques de la configuration

Karl surveille le fichier de configuration pour détecter les modifications et recharge automatiquement certains paramètres sans nécessiter un redémarrage.

**Paramètres appliqués immédiatement :**
- Paramètres WebRTC (serveurs STUN/TURN, chemin d'enregistrement)
- Paramètres d'enregistrement (rétention, format)
- Seuils d'alerte
- Paramètres d'intégration (enregistrement du proxy SIP)
- Paramètres de qualité RTP (appliqués aux nouvelles sessions)

**Paramètres nécessitant un redémarrage :**
- Paramètres de transport (ports et écouteurs UDP/TCP/TLS)
- Port du protocole NG
- Plages de ports de session
- Paramètres de connexion à la base de données

Lorsque des modifications de configuration sont détectées, Karl enregistre les changements appliqués. Consultez les journaux pour vérifier que les mises à jour ont pris effet.

---

## Exemple de configuration complète

```json
{
  "version": "1.0.0",
  "environment": "production",

  "transport": {
    "udp_enabled": true,
    "udp_port": 12000,
    "tcp_enabled": false,
    "tcp_port": 12001,
    "tls_enabled": false,
    "tls_port": 12002,
    "tls_cert": "/etc/karl/certs/server.crt",
    "tls_key": "/etc/karl/certs/server.key",
    "ipv6_enabled": false,
    "mtu": 1500
  },

  "ng_protocol": {
    "enabled": true,
    "socket_path": "/var/run/karl/karl.sock",
    "udp_port": 22222,
    "timeout": 30
  },

  "sessions": {
    "max_sessions": 10000,
    "session_ttl": 3600,
    "cleanup_interval": 60,
    "min_port": 30000,
    "max_port": 40000
  },

  "jitter_buffer": {
    "enabled": true,
    "min_delay": 20,
    "max_delay": 200,
    "target_delay": 50,
    "adaptive_mode": true,
    "max_size": 100
  },

  "rtcp": {
    "enabled": true,
    "interval": 5,
    "reduced_size": false,
    "mux_enabled": true
  },

  "fec": {
    "enabled": true,
    "block_size": 48,
    "redundancy": 0.30,
    "adaptive_mode": true,
    "max_redundancy": 0.50,
    "min_redundancy": 0.10
  },

  "recording": {
    "enabled": true,
    "base_path": "/var/lib/karl/recordings",
    "format": "wav",
    "mode": "stereo",
    "sample_rate": 8000,
    "bits_per_sample": 16,
    "max_file_size": 104857600,
    "retention_days": 30
  },

  "api": {
    "enabled": true,
    "address": ":8080",
    "auth_enabled": false,
    "rate_limit_per_min": 60,
    "cors_enabled": false,
    "cors_origins": "*",
    "tls_enabled": false,
    "tls_cert": "",
    "tls_key": ""
  },

  "webrtc": {
    "enabled": true,
    "webrtc_port": 8443,
    "stun_servers": [
      "stun:stun.l.google.com:19302",
      "stun:stun1.l.google.com:19302"
    ],
    "turn_servers": [],
    "max_bitrate": 2000000,
    "start_bitrate": 1000000,
    "bw_estimation": true,
    "tcc_enabled": true
  },

  "integration": {
    "opensips_ip": "",
    "opensips_port": 0,
    "kamailio_ip": "",
    "kamailio_port": 0,
    "media_ip": "auto",
    "public_ip": "",
    "keepalive_interval": 30,
    "interfaces": {
      "internal": {
        "name": "internal",
        "address": "10.0.0.5",
        "advertise_addr": "10.0.0.5"
      },
      "external": {
        "name": "external",
        "address": "10.0.0.5",
        "advertise_addr": "198.51.100.10"
      }
    }
  },

  "database": {
    "mysql_dsn": "",
    "redis_enabled": false,
    "redis_addr": "localhost:6379",
    "redis_cleanup_interval": 3600,
    "max_connections": 10,
    "connection_timeout": 30
  },

  "srtp": {
    "srtp_key": "",
    "srtp_salt": ""
  },

  "alert_settings": {
    "packet_loss_threshold": 0.05,
    "jitter_threshold": 50.0,
    "bandwidth_threshold": 1000000,
    "notify_admin": false,
    "admin_email": "",
    "slack_webhook": ""
  },

  "hep": {
    "hep_enabled": false,
    "hep_server": "homer:9060",
    "hep_capture_id": 1,
    "hep_proto": "udp"
  },

  "cluster": {
    "enabled": false,
    "node_id": "karl-1",
    "node_address": "10.0.0.5:22222",
    "quorum_size": 2,
    "replication_factor": 2,
    "enable_fencing": false
  }
}
```

---

## Sections de configuration

### Protocole NG

Contrôle l'interface du protocole NG compatible rtpengine.

```json
{
  "ng_protocol": {
    "enabled": true,
    "socket_path": "/var/run/karl/karl.sock",
    "udp_port": 22222,
    "timeout": 30
  }
}
```

| Paramètre | Type | Défaut | Description |
|-----------|------|--------|-------------|
| `enabled` | bool | `true` | Activer l'écouteur du protocole NG |
| `socket_path` | string | `/var/run/karl/karl.sock` | Chemin du socket Unix pour la communication locale |
| `udp_port` | int | `22222` | Port UDP pour le protocole NG |
| `timeout` | int | `30` | Délai d'expiration des requêtes en secondes |

### Sessions

Contrôle la gestion des sessions et l'allocation des ports RTP.

```json
{
  "sessions": {
    "max_sessions": 10000,
    "session_ttl": 3600,
    "cleanup_interval": 60,
    "min_port": 30000,
    "max_port": 40000
  }
}
```

| Paramètre | Type | Défaut | Description |
|-----------|------|--------|-------------|
| `max_sessions` | int | `10000` | Nombre maximum de sessions simultanées |
| `session_ttl` | int | `3600` | Durée de vie des sessions en secondes |
| `cleanup_interval` | int | `60` | Intervalle de nettoyage des sessions obsolètes (secondes) |
| `min_port` | int | `30000` | Numéro de port RTP minimum |
| `max_port` | int | `40000` | Numéro de port RTP maximum |

**Calcul de la plage de ports :**

Chaque session nécessite 4 ports (RTP + RTCP pour chaque direction). Avec la plage par défaut de 30000-40000 :
- Ports disponibles : 10 000
- Sessions maximales : 2 500

Ajustez `min_port` et `max_port` en fonction du volume d'appels simultanés attendu.

### Tampon de gigue

Contrôle le tampon de gigue adaptatif pour une lecture audio fluide.

```json
{
  "jitter_buffer": {
    "enabled": true,
    "min_delay": 20,
    "max_delay": 200,
    "target_delay": 50,
    "adaptive_mode": true,
    "max_size": 100
  }
}
```

| Paramètre | Type | Défaut | Description |
|-----------|------|--------|-------------|
| `enabled` | bool | `true` | Activer le tampon de gigue |
| `min_delay` | int | `20` | Délai de mise en tampon minimum (ms) |
| `max_delay` | int | `200` | Délai de mise en tampon maximum (ms) |
| `target_delay` | int | `50` | Délai cible pour le mode adaptatif (ms) |
| `adaptive_mode` | bool | `true` | Ajuster automatiquement la taille du tampon selon les conditions réseau |
| `max_size` | int | `100` | Taille maximale du tampon en paquets |

**Guide de réglage :**

- **Réseaux à faible latence** : `min_delay: 10`, `target_delay: 30`
- **Réseaux standard** : `min_delay: 20`, `target_delay: 50` (défaut)
- **Réseaux à forte gigue** : `min_delay: 40`, `target_delay: 100`

### RTCP

Contrôle le RTCP (RTP Control Protocol) pour la surveillance de la qualité.

```json
{
  "rtcp": {
    "enabled": true,
    "interval": 5,
    "reduced_size": false,
    "mux_enabled": true
  }
}
```

| Paramètre | Type | Défaut | Description |
|-----------|------|--------|-------------|
| `enabled` | bool | `true` | Activer le traitement RTCP |
| `interval` | int | `5` | Intervalle des rapports RTCP en secondes |
| `reduced_size` | bool | `false` | Utiliser le RTCP de taille réduite (RFC 5506) |
| `mux_enabled` | bool | `true` | Activer RTCP-mux (RTP et RTCP sur le même port) |

### Correction d'erreurs sans retransmission

Contrôle le FEC pour la récupération des paquets perdus.

```json
{
  "fec": {
    "enabled": true,
    "block_size": 48,
    "redundancy": 0.30,
    "adaptive_mode": true,
    "max_redundancy": 0.50,
    "min_redundancy": 0.10
  }
}
```

| Paramètre | Type | Défaut | Description |
|-----------|------|--------|-------------|
| `enabled` | bool | `true` | Activer la correction d'erreurs sans retransmission |
| `block_size` | int | `48` | Nombre de paquets par bloc FEC |
| `redundancy` | float | `0.30` | Taux de redondance de base (0.0-1.0) |
| `adaptive_mode` | bool | `true` | Ajuster la redondance selon la perte de paquets |
| `max_redundancy` | float | `0.50` | Redondance maximale en mode adaptatif |
| `min_redundancy` | float | `0.10` | Redondance minimale en mode adaptatif |

**Impact de la redondance :**

| Redondance | Surcharge de bande passante | Capacité de récupération |
|------------|----------------------------|--------------------------|
| 0.10 (10%) | Faible | Jusqu'à ~8% de perte de paquets |
| 0.30 (30%) | Moyenne | Jusqu'à ~20% de perte de paquets |
| 0.50 (50%) | Élevée | Jusqu'à ~35% de perte de paquets |

### Enregistrement

Contrôle la fonctionnalité d'enregistrement des appels.

```json
{
  "recording": {
    "enabled": true,
    "base_path": "/var/lib/karl/recordings",
    "format": "wav",
    "mode": "stereo",
    "sample_rate": 8000,
    "bits_per_sample": 16,
    "max_file_size": 104857600,
    "retention_days": 30
  }
}
```

| Paramètre | Type | Défaut | Description |
|-----------|------|--------|-------------|
| `enabled` | bool | `false` | Activer le système d'enregistrement |
| `base_path` | string | `/var/lib/karl/recordings` | Répertoire pour les fichiers d'enregistrement |
| `format` | string | `wav` | Format de sortie : `wav` ou `pcm` |
| `mode` | string | `stereo` | Mode d'enregistrement : `mixed`, `stereo` ou `separate` |
| `sample_rate` | int | `8000` | Taux d'échantillonnage en Hz (8000, 16000, 48000) |
| `bits_per_sample` | int | `16` | Bits par échantillon (8 ou 16) |
| `max_file_size` | int | `104857600` | Taille maximale du fichier avant rotation (octets) |
| `retention_days` | int | `30` | Nombre de jours de conservation des enregistrements avant suppression |

**Modes d'enregistrement :**

| Mode | Description | Fichiers créés |
|------|-------------|----------------|
| `mixed` | Les deux parties mixées en mono | 1 fichier |
| `stereo` | Appelant canal gauche, appelé canal droit | 1 fichier |
| `separate` | Chaque partie dans un fichier séparé | 2 fichiers |

**Calcul du stockage :**

WAV à 8kHz/16-bit : ~1 Mo par minute par canal
- Mode mixed : ~1 Mo/minute
- Mode stereo : ~2 Mo/minute
- Mode separate : ~2 Mo/minute (2 fichiers)

### API REST

Contrôle le serveur d'API REST.

```json
{
  "api": {
    "enabled": true,
    "address": ":8080",
    "auth_enabled": false,
    "rate_limit_per_min": 60,
    "cors_enabled": false,
    "cors_origins": "*",
    "tls_enabled": false,
    "tls_cert": "",
    "tls_key": ""
  }
}
```

| Paramètre | Type | Défaut | Description |
|-----------|------|--------|-------------|
| `enabled` | bool | `true` | Activer l'API REST |
| `address` | string | `:8080` | Adresse et port d'écoute |
| `auth_enabled` | bool | `false` | Exiger l'authentification par clé API |
| `rate_limit_per_min` | int | `60` | Nombre maximum de requêtes par minute par client |
| `cors_enabled` | bool | `false` | Activer les en-têtes CORS |
| `cors_origins` | string | `*` | Origines CORS autorisées |
| `tls_enabled` | bool | `false` | Activer HTTPS |
| `tls_cert` | string | | Chemin vers le certificat TLS |
| `tls_key` | string | | Chemin vers la clé privée TLS |

### WebRTC

Contrôle les fonctionnalités WebRTC pour les clients basés sur navigateur.

```json
{
  "webrtc": {
    "enabled": true,
    "webrtc_port": 8443,
    "stun_servers": [
      "stun:stun.l.google.com:19302"
    ],
    "turn_servers": [],
    "max_bitrate": 2000000,
    "start_bitrate": 1000000,
    "bw_estimation": true,
    "tcc_enabled": true
  }
}
```

| Paramètre | Type | Défaut | Description |
|-----------|------|--------|-------------|
| `enabled` | bool | `true` | Activer le support WebRTC |
| `webrtc_port` | int | `8443` | Port de signalisation WebRTC |
| `stun_servers` | array | Google STUN | Serveurs STUN pour la traversée NAT |
| `turn_servers` | array | `[]` | Serveurs TURN pour le relais |
| `max_bitrate` | int | `2000000` | Débit maximum (bps) |
| `start_bitrate` | int | `1000000` | Débit initial (bps) |
| `bw_estimation` | bool | `true` | Activer l'estimation de la bande passante |
| `tcc_enabled` | bool | `true` | Activer le retour Transport-CC |

### Intégration

Contrôle l'intégration avec les proxys SIP et la sélection des interfaces réseau.

```json
{
  "integration": {
    "opensips_ip": "",
    "opensips_port": 0,
    "kamailio_ip": "",
    "kamailio_port": 0,
    "media_ip": "10.0.0.5",
    "public_ip": "198.51.100.10",
    "keepalive_interval": 30,
    "interfaces": {
      "internal": {
        "name": "internal",
        "address": "10.0.0.5",
        "advertise_addr": "10.0.0.5"
      },
      "external": {
        "name": "external",
        "address": "10.0.0.5",
        "advertise_addr": "198.51.100.10"
      }
    }
  }
}
```

| Paramètre | Type | Défaut | Description |
|-----------|------|--------|-------------|
| `opensips_ip` | string | | IP du serveur OpenSIPS pour l'enregistrement |
| `opensips_port` | int | | Port du serveur OpenSIPS |
| `kamailio_ip` | string | | IP du serveur Kamailio pour l'enregistrement |
| `kamailio_port` | int | | Port du serveur Kamailio |
| `media_ip` | string | `auto` | Adresse IP pour la liaison des sockets média. Utilisez `auto` pour la détection automatique |
| `public_ip` | string | | IP publique pour les scénarios NAT (utilisée dans le SDP lorsqu'aucune `advertise_addr` par interface n'est définie) |
| `keepalive_interval` | int | `30` | Intervalle de keepalive vers les proxys SIP (secondes) |
| `interfaces` | map | | Interfaces réseau nommées pour le routage média basé sur la direction (voir ci-dessous) |

#### Interfaces nommées

Les interfaces nommées permettent à Karl d'annoncer différentes adresses IP dans le SDP selon la direction de l'appel. C'est essentiel lorsque votre serveur média dispose de chemins réseau internes et externes séparés (par exemple, une IP privée pour votre PBX et une IP élastique pour les opérateurs).

Chaque entrée d'interface possède :

| Paramètre | Type | Description |
|-----------|------|-------------|
| `name` | string | Identifiant de l'interface (par ex., `internal`, `external`) |
| `address` | string | IP locale sur laquelle lier les sockets |
| `advertise_addr` | string | IP à mettre dans la ligne `c=` du SDP et les informations de flux. Si vide, retombe sur `address` |
| `port` | int | Remplacement de port optionnel |

**Comment la direction sélectionne l'interface :**

Le proxy SIP envoie un tableau `direction` dans la requête du protocole NG (par ex., `["internal", "external"]`). Karl l'utilise pour choisir l'interface correcte :

- `direction[0]` = côté d'origine ("from")
- `direction[1]` = côté de terminaison ("to")

Pour l'**offer** (SDP côté appelant), Karl utilise `direction[0]` pour sélectionner l'interface. Pour l'**answer** (SDP côté appelé), Karl utilise `direction[1]`. L'`advertise_addr` de l'interface sélectionnée est insérée dans le SDP, tandis que l'`address` est utilisée pour la liaison réelle du socket.

**Exemple — les opérateurs reçoivent l'IP publique, les points de terminaison internes reçoivent l'IP privée :**

```json
{
  "integration": {
    "media_ip": "10.0.0.5",
    "interfaces": {
      "internal": {
        "address": "10.0.0.5",
        "advertise_addr": "10.0.0.5"
      },
      "external": {
        "address": "10.0.0.5",
        "advertise_addr": "198.51.100.10"
      }
    }
  }
}
```

Puis dans votre proxy SIP, pour les appels vers un opérateur externe :
```
rtpengine_manage("... direction=internal direction=external");
```

Karl liera le socket sur `10.0.0.5` mais mettra `198.51.100.10` dans le SDP pour le leg côté opérateur.

Si aucune `interfaces` n'est configurée, Karl retombe sur `media_ip` pour la liaison et `public_ip` pour l'annonce SDP (ou `media_ip` si `public_ip` est vide).

### Base de données

Contrôle les connexions à la base de données pour le CDR et le stockage des sessions.

```json
{
  "database": {
    "mysql_dsn": "",
    "redis_enabled": false,
    "redis_addr": "localhost:6379",
    "redis_cleanup_interval": 3600,
    "max_connections": 10,
    "connection_timeout": 30
  }
}
```

| Paramètre | Type | Défaut | Description |
|-----------|------|--------|-------------|
| `mysql_dsn` | string | | Chaîne de connexion MySQL |
| `redis_enabled` | bool | `false` | Activer Redis pour la mise en cache des sessions |
| `redis_addr` | string | `localhost:6379` | Adresse du serveur Redis |
| `redis_cleanup_interval` | int | `3600` | Intervalle de nettoyage Redis (secondes) |
| `max_connections` | int | `10` | Nombre maximum de connexions à la base de données |
| `connection_timeout` | int | `30` | Délai d'expiration de la connexion (secondes) |

**Format du DSN MySQL :**

```
user:password@tcp(host:port)/database?parseTime=true
```

Exemple :
```
karl:secretpassword@tcp(127.0.0.1:3306)/karl?parseTime=true
```

### SRTP

Contrôle les paramètres de chiffrement SRTP.

```json
{
  "srtp": {
    "srtp_key": "",
    "srtp_salt": ""
  }
}
```

| Paramètre | Type | Défaut | Description |
|-----------|------|--------|-------------|
| `srtp_key` | string | | Clé maître pour SRTP (encodée en base64) |
| `srtp_salt` | string | | Salt maître pour SRTP (encodé en base64) |

### Alertes

Contrôle les seuils d'alerte de qualité.

```json
{
  "alert_settings": {
    "packet_loss_threshold": 0.05,
    "jitter_threshold": 50.0,
    "bandwidth_threshold": 1000000,
    "notify_admin": false,
    "admin_email": "",
    "slack_webhook": ""
  }
}
```

| Paramètre | Type | Défaut | Description |
|-----------|------|--------|-------------|
| `packet_loss_threshold` | float | `0.05` | Alerter lorsque la perte de paquets dépasse 5% |
| `jitter_threshold` | float | `50.0` | Alerter lorsque la gigue dépasse 50ms |
| `bandwidth_threshold` | int | `1000000` | Alerter lorsque la bande passante dépasse le seuil |
| `notify_admin` | bool | `false` | Envoyer des notifications d'alerte |
| `admin_email` | string | | Adresse e-mail pour les alertes |
| `slack_webhook` | string | | URL du webhook Slack pour les alertes |

### HEP / Homer

Contrôle l'exportateur HEPv3 pour l'envoi de paquets SIP et RTCP vers un serveur de capture Homer.

```json
{
  "hep": {
    "hep_enabled": true,
    "hep_server": "homer.example.com:9060",
    "hep_capture_id": 1,
    "hep_proto": "udp"
  }
}
```

| Paramètre | Type | Défaut | Description |
|-----------|------|--------|-------------|
| `hep_enabled` | bool | `false` | Activer l'exportateur HEP |
| `hep_server` | string | | Adresse du serveur de capture Homer (host:port) |
| `hep_capture_id` | int | `0` | ID de l'agent de capture envoyé dans les paquets HEP |
| `hep_proto` | string | `udp` | Protocole de transport vers Homer (`udp` ou `tcp`) |

### Cluster

Contrôle le clustering multi-noeuds avec basculement de session soutenu par Redis, détection de split-brain et hachage cohérent.

```json
{
  "cluster": {
    "enabled": true,
    "node_id": "karl-1",
    "node_address": "10.0.0.5:22222",
    "quorum_size": 2,
    "replication_factor": 2,
    "enable_fencing": true
  }
}
```

| Paramètre | Type | Défaut | Description |
|-----------|------|--------|-------------|
| `enabled` | bool | `false` | Activer le clustering |
| `node_id` | string | | Identifiant unique pour ce noeud |
| `node_address` | string | | Adresse de ce noeud (accessible par les autres noeuds) |
| `quorum_size` | int | `2` | Nombre minimum de noeuds pour le quorum |
| `replication_factor` | int | `2` | Facteur de réplication des sessions sur l'anneau de hachage |
| `enable_fencing` | bool | `false` | Activer l'isolement des noeuds lors de la détection de partition |

**Prérequis** : Redis doit être activé dans la section `database`. Le gestionnaire de cluster utilise Redis pour le stockage des sessions, la coordination pub/sub, les heartbeats et la détection de split-brain.

**Fonctionnement du basculement** :
1. Chaque noeud envoie des heartbeats à Redis toutes les secondes
2. Si un noeud manque 3 heartbeats, il est marqué comme défaillant
3. Le détecteur de split-brain vérifie le quorum avant d'autoriser la prise en charge
4. Les noeuds survivants redistribuent les sessions du noeud défaillant via le hachage cohérent
5. Les ports sont réalloués et les relais média redémarrés sur le nouveau propriétaire

---

## Variables d'environnement

Toutes les options de configuration peuvent être remplacées via des variables d'environnement :

| Variable | Chemin de config | Description |
|----------|-----------------|-------------|
| `KARL_CONFIG_PATH` | - | Chemin vers le fichier de configuration |
| `KARL_LOG_LEVEL` | - | Niveau de journalisation (debug, info, warn, error) |
| `KARL_HEALTH_PORT` | - | Port de vérification de santé (défaut : `:8086`) |
| `KARL_METRICS_PORT` | - | Port des métriques Prometheus (défaut : `:9091`) |
| `KARL_API_PORT` | `api.address` | Port de l'API REST |
| `KARL_NG_PORT` | `ng_protocol.udp_port` | Port UDP du protocole NG |
| `KARL_RTP_MIN_PORT` | `sessions.min_port` | Port RTP minimum |
| `KARL_RTP_MAX_PORT` | `sessions.max_port` | Port RTP maximum |
| `KARL_MAX_SESSIONS` | `sessions.max_sessions` | Nombre maximum de sessions simultanées |
| `KARL_RECORDING_PATH` | `recording.base_path` | Chemin de stockage des enregistrements |
| `KARL_RECORDING_ENABLED` | `recording.enabled` | Activer l'enregistrement |
| `KARL_MYSQL_DSN` | `database.mysql_dsn` | Chaîne de connexion MySQL |
| `KARL_REDIS_ENABLED` | `database.redis_enabled` | Activer Redis |
| `KARL_REDIS_ADDR` | `database.redis_addr` | Adresse Redis |
| `KARL_MEDIA_IP` | `integration.media_ip` | Adresse IP média |
| `KARL_PUBLIC_IP` | `integration.public_ip` | Adresse IP publique |
| `KARL_RUN_DIR` | - | Répertoire d'exécution |

Les variables d'environnement ont priorité sur les valeurs du fichier de configuration.

---

## Prochaines étapes

- [Démarrage rapide](./getting-started.md)
- [Déploiement Kubernetes](./how-to/deploying-kubernetes.md)
- [Configuration de la surveillance](./how-to/monitoring-prometheus.md)
