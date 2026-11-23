[English](README.md) | [中文](README.zh-CN.md) | **Français** | [Español](README.es.md)

# Karl Media Server

## Proxy RTP Open Source et Serveur Multimédia pour la VoIP et le WebRTC

[![Go Report Card](https://goreportcard.com/badge/github.com/loreste/karl)](https://goreportcard.com/report/github.com/loreste/karl)
[![Go Reference](https://pkg.go.dev/badge/github.com/loreste/karl.svg)](https://pkg.go.dev/github.com/loreste/karl)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL%203.0-blue.svg)](https://opensource.org/licenses/GPL-3.0)

Karl est un proxy multimédia RTP haute performance et cloud-native, ainsi qu'un serveur multimédia écrit en Go. Il constitue un remplacement moderne et direct de rtpengine, conçu spécifiquement pour les déploiements cloud, les environnements Kubernetes et les infrastructures VoIP modernes.

**Points clés :**
- Déploiement en un seul binaire sans aucune dépendance au noyau
- Compatibilité complète avec le protocole NG de rtpengine
- Support natif de Kubernetes avec sondes de santé
- Passerelle WebRTC-vers-SIP prête à l'emploi
- Métriques Prometheus et observabilité de niveau production

---

## Table des matières

- [Le problème que nous résolvons](#le-problème-que-nous-résolvons)
- [Pourquoi choisir Karl](#pourquoi-choisir-karl)
- [Fonctionnalités](#fonctionnalités)
- [Démarrage rapide](#démarrage-rapide)
- [Documentation](#documentation)
- [Installation](#installation)
- [Configuration](#configuration)
- [Déploiement Kubernetes](#déploiement-kubernetes)
- [Intégration avec les proxys SIP](#intégration-avec-les-proxys-sip)
- [Référence API](#référence-api)
- [Surveillance et observabilité](#surveillance-et-observabilité)
- [Performance](#performance)
- [Architecture](#architecture)
- [Feuille de route](#feuille-de-route)
- [Contribuer](#contribuer)
- [Licence](#licence)

---

## Le problème que nous résolvons

### Le défi des proxys RTP traditionnels

L'exploitation de serveurs multimédias dans les environnements VoIP de production a toujours été pénible. Si vous avez exploité rtpengine ou des proxys RTP similaires, vous avez probablement rencontré ces défis :

**Dépendances aux modules noyau**
Les proxys RTP traditionnels nécessitent des modules noyau pour la performance. Ces modules se cassent à chaque mise à jour du noyau, nécessitent une recompilation et sont incompatibles avec les environnements conteneurisés. Les faire fonctionner sur des services Kubernetes managés comme EKS, GKE ou AKS est soit impossible, soit nécessite des conteneurs privilégiés avec un accès à l'hôte.

**Déploiement complexe**
Installer rtpengine signifie gérer plusieurs dépôts, compiler des modules noyau, résoudre des dépendances de bibliothèques C et maintenir des pipelines de build complexes. Une simple mise à jour peut nécessiter des heures d'indisponibilité.

**Compatibilité cloud limitée**
La plupart des proxys RTP ont été conçus pour des serveurs bare-metal. Ils peinent avec :
- Les adresses IP dynamiques dans les environnements cloud
- Les groupes d'auto-scaling et l'orchestration de conteneurs
- La traversée de NAT dans les réseaux VPC
- Les vérifications de santé des répartiteurs de charge

**Observabilité insuffisante**
Le débogage des problèmes de qualité média en production nécessite une visibilité approfondie sur la perte de paquets, la gigue et les performances des codecs. Les outils traditionnels offrent des métriques limitées et aucune intégration native avec les piles d'observabilité modernes comme Prometheus et Grafana.

**WebRTC comme une réflexion tardive**
Avec l'adoption croissante du WebRTC, le pontage des appels basés sur navigateur vers l'infrastructure SIP traditionnelle nécessite une configuration complexe et souvent des composants séparés.

### Comment Karl résout ces problèmes

Karl a été construit de zéro pour répondre à chacun de ces défis :

| Défi | Proxy RTP traditionnel | Karl |
|------|----------------------|------|
| Modules noyau | Requis, se cassent aux mises à jour | Aucun requis |
| Déploiement | Des heures de configuration | Un seul binaire, déploiement en secondes |
| Cloud/Kubernetes | Contournements nécessaires | Support natif |
| Observabilité | Limitée | Plus de 50 métriques Prometheus |
| WebRTC | Composant séparé | Intégré |
| Support des conteneurs | Difficile | Citoyen de première classe |

---

## Pourquoi choisir Karl

### Zéro dépendance au noyau

Karl fonctionne entièrement en espace utilisateur. Aucun module noyau à compiler, aucun conteneur privilégié, aucun problème de compatibilité avec les services Kubernetes managés. Déployez sur n'importe quelle distribution Linux, n'importe quel fournisseur cloud, n'importe quel orchestrateur de conteneurs.

```bash
# C'est toute l'installation
go build -o karl && ./karl
```

### Compatibilité complète avec rtpengine

Karl implémente le protocole complet NG (Next Generation) utilisé par rtpengine. Votre configuration existante OpenSIPS ou Kamailio fonctionne sans modification. Aucun changement à votre proxy SIP, aucune complexité de migration.

### Architecture cloud-native

Conçu pour l'infrastructure moderne :
- **Sondes de santé natives Kubernetes** : points de terminaison Startup, Liveness et Readiness
- **Configuration par variables d'environnement** : intégration facile avec les ConfigMaps et Secrets
- **Mise à l'échelle horizontale** : partage de sessions adossé à Redis pour les déploiements multi-instances
- **Arrêt gracieux** : gestion correcte de SIGTERM pour des déploiements sans interruption

### Observabilité de niveau production

Sachez exactement ce qui se passe dans votre infrastructure média :
- **Plus de 50 métriques Prometheus** couvrant les sessions, la qualité RTCP, la récupération FEC et les performances API
- **Journalisation structurée JSON** pour une ingestion facile dans ELK, Splunk ou CloudWatch
- **Métriques de qualité en temps réel** incluant les scores MOS, la gigue et la perte de paquets par appel
- **Enregistrements détaillés des appels (CDR)** avec statistiques de qualité complètes

### WebRTC natif

Le WebRTC n'est pas une pièce rapportée — c'est une fonctionnalité centrale :
- Support **ICE/STUN/TURN** pour la traversée de NAT
- Chiffrement **DTLS-SRTP** pour le pontage entre WebRTC et SIP
- **Transcodage du codec Opus** vers G.711 et retour
- **Estimation de la bande passante** avec support Transport-CC

---

## Fonctionnalités

### Commandes du protocole NG

Compatibilité complète avec le protocole NG de rtpengine :

| Commande | Description |
|----------|-------------|
| `ping` | Vérification de santé et keepalive |
| `offer` | Traiter l'offre SDP, allouer les ports média |
| `answer` | Traiter la réponse SDP, finaliser la configuration de session |
| `delete` | Terminer la session, libérer les ressources |
| `query` | Obtenir les statistiques de session en temps réel |
| `list` | Énumérer tous les appels actifs |
| `start recording` | Démarrer l'enregistrement d'appel |
| `stop recording` | Arrêter l'enregistrement d'appel |
| `block media` | Couper/rétablir le son des flux média |
| `play DTMF` | Injecter des tonalités DTMF |

### Traitement multimédia

- **Tampon de gigue adaptatif** : mise en mémoire tampon dynamique (20-200ms) avec ajustement automatique selon les conditions réseau
- **Correction d'erreur directe (FEC)** : FEC basée sur XOR avec redondance adaptative (10-50%) selon la perte de paquets en temps réel
- **Traitement RTCP** : RFC 3550 complète avec rapports SR/RR, RTT, gigue, calcul MOS (modèle E ITU-T G.107)
- **Transfert RTCP** : rapports RTCP relayés entre les segments avec chiffrement SRTCP optionnel
- **SRTP/SRTCP** : chiffrement/déchiffrement par segment câblé dans le relais média — les clés SDES du SDP créent les contextes cryptographiques, clés DTLS via `SetDTLSKeys()`. Support SRTCP complet pour le RTCP chiffré.
- **Agent ICE** : ICE par session avec collecte de candidats server-reflexive STUN et relais TURN, vérifications de connectivité, sélection de paires de candidats et fraîcheur du consentement via pion/ice
- **Négociation de codecs** : intersection RFC 3264 — codecs de la réponse intersectés avec l'ensemble de l'offrant, ordonnés selon la préférence de l'offrant
- **Support des codecs** : transcodage G.711 (PCMU/PCMA) intégré ; Opus, G.729, AMR disponibles via les tags de build CGO (`-tags opus`, `-tags g729`, `-tags amr`) liant respectivement libopus, bcg729, opencore-amrnb
- **Passerelle fax T.38** : relais UDPTL complet avec analyse de paquets IFP, redondance, analyse/génération SDP, gestion du cycle de vie des sessions et transfert de sockets UDP
- **Sélection d'interface basée sur la direction** : interfaces réseau nommées avec des IP de liaison/annonce séparées — le proxy SIP envoie un tableau `direction` pour sélectionner les adresses média internes vs externes
- **SIPREC** : analyse des métadonnées RFC 7865/7866 et structures de session (sortie d'enregistrement câblée via le relais média)
- **Multi-flux** : flux vidéo et médias supplémentaires supportés via des entrées `StreamPair` par session, chacune avec un relais indépendant

### Enregistrement d'appels

Capacités d'enregistrement professionnelles :

| Mode | Description |
|------|-------------|
| Mixed | Fichier mono unique avec les deux parties |
| Stereo | Canal gauche appelant, canal droit appelé |
| Separate | Fichiers individuels par segment d'appel |
| SIPREC | Enregistrement de session conforme RFC 7865/7866 |

- **Format** : WAV (PCM 16 bits) à des taux d'échantillonnage configurables
- **Stockage** : système de fichiers local ou stockage réseau
- **Rétention** : nettoyage automatique selon des politiques configurables
- **Basculement** : continuité d'enregistrement en cas de défaillance de nœud

### Clustering et haute disponibilité

Support de clustering de niveau entreprise :

- **État de session adossé à Redis** : partage de sessions distribué entre les nœuds
- **Hachage cohérent** : placement de sessions persistantes avec basculement
- **Détection de split-brain** : tolérance de partition basée sur le quorum
- **Réallocation de ports** : attribution de ports cohérente lors du basculement
- **Coordination CDR** : agrégation distribuée des enregistrements détaillés d'appels
- **Notification de proxy** : notification automatique du proxy SIP en cas de basculement

### Sécurité

Fonctionnalités de sécurité complètes :

- **TLS/HTTPS** : interfaces API et de gestion sécurisées
- **Authentification** : authentification par clé API et par jeton
- **Autorisation** : contrôle d'accès basé sur les rôles pour les opérations
- **Limitation de débit** : limites configurables par IP et par appel
- **Protection DoS** : blocage automatique des sources abusives
- **Validation des entrées** : validation stricte de toutes les entrées protocolaires
- **Gestion des secrets** : traitement sécurisé des identifiants

### API REST

Contrôle programmatique de toutes les fonctions du serveur :

```bash
# Lister les sessions actives
curl http://localhost:8080/api/v1/sessions

# Obtenir les statistiques du serveur
curl http://localhost:8080/api/v1/stats

# Démarrer l'enregistrement d'un appel
curl -X POST http://localhost:8080/api/v1/recording/start \
  -H "Content-Type: application/json" \
  -d '{"session_id": "abc123", "mode": "stereo"}'

# Arrêter l'enregistrement
curl -X POST http://localhost:8080/api/v1/recording/stop \
  -d '{"session_id": "abc123"}'
```

---

## Démarrage rapide

### Prérequis

- Go 1.26 ou supérieur
- MySQL/MariaDB (optionnel, pour le stockage CDR)
- Redis (optionnel, pour le cache de sessions distribué)

### Prêt en 30 secondes

```bash
# Cloner le dépôt
git clone https://github.com/loreste/karl.git
cd karl

# Compiler et exécuter
go build -o karl
./karl
```

Karl écoute maintenant sur :
- Port UDP 22222 pour le protocole NG (communication avec le proxy SIP)
- Ports UDP 30000-40000 pour le média RTP
- Port TCP 8080 pour l'API REST
- Port TCP 8086 pour les vérifications de santé
- Port TCP 9091 pour les métriques Prometheus

### Tester la connexion

```bash
# Test de ping
echo -n "d7:command4:pinge" | nc -u localhost 22222
```

---

## Documentation

Une documentation complète est disponible dans le répertoire [`docs/`](./docs).

### Pour commencer

- [Guide de démarrage rapide](./docs/getting-started.md) - Soyez opérationnel en 5 minutes
- [Guide d'installation](./docs/installation.md) - Options d'installation détaillées
- [Référence de configuration](./docs/configuration.md) - Toutes les options de configuration

### Guides pratiques

Guides étape par étape pour les tâches courantes :

| Guide | Description |
|-------|-------------|
| [Déployer sur Kubernetes](./docs/how-to/deploying-kubernetes.md) | Déploiement K8s en production avec sondes, mise à l'échelle et surveillance |
| [Intégrer avec OpenSIPS](./docs/how-to/integrating-opensips.md) | Connecter Karl à OpenSIPS avec NAT, WebRTC et enregistrement |
| [Intégrer avec Kamailio](./docs/how-to/integrating-kamailio.md) | Connecter Karl à Kamailio avec des exemples complets |
| [Configurer l'enregistrement d'appels](./docs/how-to/setting-up-recording.md) | Configurer les modes d'enregistrement, le stockage et la rétention |
| [Surveiller avec Prometheus](./docs/how-to/monitoring-prometheus.md) | Métriques, tableaux de bord Grafana et règles d'alerte |
| [Mise à l'échelle horizontale](./docs/how-to/scaling-horizontally.md) | Clustering Redis et répartition de charge |
| [Passerelle WebRTC vers SIP](./docs/how-to/webrtc-sip-bridging.md) | Connecter les clients navigateur à l'infrastructure SIP |
| [Sécuriser avec TLS](./docs/how-to/securing-with-tls.md) | HTTPS pour les interfaces API et de gestion |
| [Dépannage](./docs/how-to/troubleshooting.md) | Diagnostiquer et résoudre les problèmes courants |

### Référence

- [Référence du protocole NG](./docs/reference/ng-protocol.md) - Spécification complète du protocole
- [Variables d'environnement](./docs/reference/environment-variables.md) - Toutes les variables supportées

### Outil CLI

Karl est livré avec `karl-ctl`, une interface en ligne de commande pour gérer les sessions, consulter les statistiques et contrôler l'enregistrement :

```bash
# Compiler le CLI
go build -o karl-ctl ./cmd/karl-ctl/

# Utilisation
karl-ctl ping                      # Vérification de santé
karl-ctl sessions                  # Lister les sessions actives
karl-ctl session <id>              # Détails de la session
karl-ctl stats                     # Statistiques du serveur
karl-ctl delete <id>               # Supprimer une session
karl-ctl recording start <id>      # Démarrer l'enregistrement
karl-ctl recording stop <id>       # Arrêter l'enregistrement

# Options
karl-ctl -addr http://host:8080 sessions   # Adresse API personnalisée
karl-ctl -json stats                        # Sortie JSON brute
```

---

## Installation

### Depuis les sources

```bash
git clone https://github.com/loreste/karl.git
cd karl
go build -o karl
sudo mv karl /usr/local/bin/
```

### Docker

```bash
docker run -d \
  --name karl \
  --network host \
  loreste/karl:latest
```

Ou avec le mappage de ports (plage de ports RTP limitée) :

```bash
docker run -d \
  --name karl \
  -p 22222:22222/udp \
  -p 30000-30100:30000-30100/udp \
  -p 8080:8080 \
  -p 8086:8086 \
  -p 9091:9091 \
  -v /path/to/config.json:/etc/karl/config.json \
  -v /path/to/recordings:/var/lib/karl/recordings \
  loreste/karl:latest
```

### Docker Compose

```yaml
version: '3.8'
services:
  karl:
    image: loreste/karl:latest
    network_mode: host
    volumes:
      - ./config.json:/etc/karl/config.json
      - ./recordings:/var/lib/karl/recordings
    environment:
      - KARL_CONFIG_PATH=/etc/karl/config.json
      - KARL_LOG_LEVEL=info
    restart: unless-stopped
```

---

## Configuration

Karl utilise un fichier de configuration JSON avec des valeurs par défaut raisonnables. Tous les paramètres peuvent également être surchargés via des variables d'environnement.

### Fichier de configuration

```json
{
  "ng_protocol": {
    "enabled": true,
    "udp_port": 22222,
    "timeout": 30
  },
  "sessions": {
    "max_sessions": 10000,
    "session_ttl": 3600,
    "min_port": 30000,
    "max_port": 40000
  },
  "jitter_buffer": {
    "enabled": true,
    "min_delay": 20,
    "max_delay": 200,
    "adaptive_mode": true
  },
  "fec": {
    "enabled": true,
    "redundancy": 0.30,
    "adaptive_mode": true
  },
  "recording": {
    "enabled": true,
    "base_path": "/var/lib/karl/recordings",
    "format": "wav",
    "mode": "stereo",
    "retention_days": 30
  },
  "api": {
    "enabled": true,
    "address": ":8080",
    "auth_enabled": false
  },
  "webrtc": {
    "enabled": true,
    "stun_servers": ["stun:stun.l.google.com:19302"]
  },
  "database": {
    "mysql_dsn": "",
    "redis_enabled": false,
    "redis_addr": "redis:6379"
  },
  "integration": {
    "media_ip": "10.0.0.5",
    "interfaces": {
      "internal": { "address": "10.0.0.5", "advertise_addr": "10.0.0.5" },
      "external": { "address": "10.0.0.5", "advertise_addr": "198.51.100.10" }
    }
  },
  "hep": {
    "hep_enabled": false,
    "hep_server": "homer:9060"
  },
  "cluster": {
    "enabled": false,
    "node_id": "karl-1"
  }
}
```

### Variables d'environnement

Toute la configuration peut être définie via des variables d'environnement :

| Variable | Description | Défaut |
|----------|-------------|--------|
| `KARL_CONFIG_PATH` | Chemin vers le fichier de configuration | `config/config.json` |
| `KARL_HEALTH_PORT` | Port du point de terminaison de santé | `:8086` |
| `KARL_METRICS_PORT` | Port des métriques Prometheus | `:9091` |
| `KARL_API_PORT` | Port de l'API REST | `:8080` |
| `KARL_NG_PORT` | Port UDP du protocole NG | `22222` |
| `KARL_RTP_MIN_PORT` | Début de la plage de ports RTP | `30000` |
| `KARL_RTP_MAX_PORT` | Fin de la plage de ports RTP | `40000` |
| `KARL_MAX_SESSIONS` | Sessions concurrentes maximales | `10000` |
| `KARL_RECORDING_PATH` | Chemin de stockage des enregistrements | `/var/lib/karl/recordings` |
| `KARL_RECORDING_ENABLED` | Activer l'enregistrement d'appels | `true` |
| `KARL_MYSQL_DSN` | Chaîne de connexion MySQL | (vide) |
| `KARL_REDIS_ADDR` | Adresse du serveur Redis | (vide) |
| `KARL_REDIS_ENABLED` | Activer le cache de sessions Redis | `false` |
| `KARL_MEDIA_IP` | Adresse IP média | `auto` |
| `KARL_PUBLIC_IP` | IP publique pour le SDP | (détection automatique) |

---

## Déploiement Kubernetes

Karl est conçu pour Kubernetes dès le départ. Des manifestes complets sont fournis dans le répertoire `deploy/kubernetes/`.

### Déploiement rapide

```bash
kubectl apply -k deploy/kubernetes/
```

### Sondes de santé

Karl expose des points de terminaison de sondes natifs Kubernetes :

| Sonde | Point de terminaison | Objectif |
|-------|---------------------|----------|
| Startup | `/startup` | Attendre l'initialisation (jusqu'à 150s) |
| Liveness | `/live` | Détecter les deadlocks, déclencher un redémarrage |
| Readiness | `/ready` | Vérifier si prêt à recevoir du trafic |

### Architecture de déploiement

Le déploiement par défaut utilise `hostNetwork: true` pour des performances RTP optimales :

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: karl
spec:
  replicas: 1
  template:
    spec:
      hostNetwork: true
      containers:
      - name: karl
        image: loreste/karl:latest
        ports:
        - containerPort: 22222
          protocol: UDP
        - containerPort: 8080
          protocol: TCP
        - containerPort: 8086
          protocol: TCP
        - containerPort: 9091
          protocol: TCP
        livenessProbe:
          httpGet:
            path: /live
            port: 8086
          periodSeconds: 15
        readinessProbe:
          httpGet:
            path: /ready
            port: 8086
          periodSeconds: 5
        startupProbe:
          httpGet:
            path: /startup
            port: 8086
          failureThreshold: 30
          periodSeconds: 5
```

### Mise à l'échelle

Pour la mise à l'échelle horizontale, activez Redis pour le partage de sessions :

1. Déployez Redis (ou utilisez un Redis managé)
2. Définissez `KARL_REDIS_ENABLED=true` et `KARL_REDIS_ADDR=redis:6379`
3. Augmentez le nombre de réplicas du déploiement

---

## Intégration avec les proxys SIP

### OpenSIPS

```opensips
# opensips.cfg
loadmodule "rtpengine.so"
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")

route {
    # ... votre logique de routage ...

    if (has_body("application/sdp")) {
        rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
    }
}
```

### Kamailio

```kamailio
# kamailio.cfg
loadmodule "rtpengine.so"
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")

request_route {
    # ... votre logique de routage ...

    if (has_body("application/sdp")) {
        rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
    }
}
```

### FreeSWITCH

Utilisez le module `mod_rtpengine` avec :

```xml
<param name="rtpengine-ip" value="127.0.0.1"/>
<param name="rtpengine-port" value="22222"/>
```

---

## Référence API

### Sessions

**Lister toutes les sessions**
```bash
GET /api/v1/sessions
```

**Obtenir les détails d'une session**
```bash
GET /api/v1/sessions/{session_id}
```

**Supprimer une session**
```bash
DELETE /api/v1/sessions/{session_id}
```

### Statistiques

**Obtenir les statistiques du serveur**
```bash
GET /api/v1/stats
```

Réponse :
```json
{
  "active_sessions": 150,
  "total_sessions": 10542,
  "packets_processed": 15234567,
  "packets_dropped": 234,
  "uptime_seconds": 86400
}
```

### Enregistrement

**Démarrer l'enregistrement**
```bash
POST /api/v1/recording/start
Content-Type: application/json

{
  "session_id": "abc123",
  "mode": "stereo"
}
```

**Arrêter l'enregistrement**
```bash
POST /api/v1/recording/stop
Content-Type: application/json

{
  "session_id": "abc123"
}
```

### Santé

**Vérification de santé simple**
```bash
GET /health
```

**État de santé détaillé**
```bash
GET /health/detail
```

---

## Surveillance et observabilité

### Métriques Prometheus

Karl expose des métriques complètes sur `:9091/metrics` :

**Métriques de session**
```
karl_sessions_active          # Sessions actives actuelles
karl_sessions_total           # Total des sessions créées
karl_session_duration_seconds # Histogramme de durée des sessions
```

**Métriques de qualité média**
```
karl_rtcp_rtt_seconds         # Temps aller-retour RTCP
karl_rtcp_jitter_seconds      # Gigue rapportée
karl_rtcp_packet_loss_ratio   # Pourcentage de perte de paquets
karl_fec_recoveries_total     # Paquets récupérés via FEC
karl_jitter_buffer_latency    # Délai du tampon de gigue
```

**Métriques du protocole NG**
```
karl_ng_commands_total        # Commandes traitées par type
karl_ng_command_duration      # Temps de traitement des commandes
karl_ng_active_calls          # Nombre d'appels en cours
```

### Tableau de bord Grafana

Importez le tableau de bord fourni depuis `deploy/grafana/karl-dashboard.json` pour :
- Surveillance des sessions en temps réel
- Visualisation de la qualité média
- Suivi de l'utilisation des ressources
- Configuration des alertes

### Points de terminaison de santé

| Point de terminaison | Objectif | Réponse |
|---------------------|----------|---------|
| `/health` | État simple | `{"status":"UP"}` |
| `/health/detail` | État des composants | Détail complet des composants |
| `/live` | Liveness Kubernetes | 200 si actif |
| `/ready` | Readiness Kubernetes | 200 si prêt pour le trafic |
| `/startup` | Startup Kubernetes | 200 une fois initialisé |

---

## Performance

Karl est optimisé pour le traitement multimédia à haut débit :

| Métrique | Performance |
|----------|-------------|
| Création de session | 4M opérations/seconde |
| Opérations du tampon de gigue | 4,4M opérations/seconde |
| Encodage FEC | 10,3M opérations/seconde |
| Transcodage G.711 | 3,3M opérations/seconde |
| Encodage iLBC | 1,3M opérations/seconde |
| Opérations du pool de tampons | 58M opérations/seconde |
| Mémoire par session | ~624 octets |
| Sessions concurrentes testées | 10 000+ |

### Fonctionnalités de performance

- **Transfert zéro-copie** : copies mémoire minimisées sur le chemin rapide
- **Partitionnement de sockets** : sockets par cœur pour la scalabilité
- **Pool de tampons** : tampons réutilisables réduisant la pression du ramasse-miettes
- **Pools de workers** : traitement concurrent efficace des paquets
- **RTCP par lots** : traitement RTCP agrégé
- **Enregistrement asynchrone** : écritures d'enregistrement non bloquantes

### Recommandations de ressources

| Appels simultanés | CPU | Mémoire |
|-------------------|-----|---------|
| < 100 | 250m | 256Mi |
| 100-500 | 500m | 512Mi |
| 500-1000 | 1000m | 1Gi |
| > 1000 | 2000m | 2Gi |

---

## Architecture

```
                                    ┌──────────────────┐
                                    │   Prometheus     │
                                    │   :9091/metrics  │
                                    └────────┬─────────┘
                                             │
┌─────────────┐     NG Protocol      ┌───────┴────────┐     RTP/RTCP      ┌────────────┐
│  OpenSIPS   │ ◄──────────────────► │                │ ◄───────────────► │  Endpoints │
│  Kamailio   │     UDP:22222        │      Karl      │   UDP:30000-40000 │ (SIP/WebRTC)│
└─────────────┘                      │                │                   └────────────┘
                                     └───────┬────────┘
                                             │
                                     ┌───────┴────────┐
                                     │   REST API     │
                                     │   :8080        │
                                     └────────────────┘
```

### Vue d'ensemble des composants

- **Gestionnaire du protocole NG** (`ng_socket_listener.go`) : traite les commandes bencode des proxys SIP via socket UDP/Unix
- **Gestionnaires de commandes** (`ng_protocol/commands/`) : Offer, Answer, Delete, Query — créent les sessions, allouent les ports, construisent le SDP
- **Gestionnaire de sessions** (`session_manager.go`) : suit l'état des appels, les segments, les index SSRC, le nettoyage par TTL
- **Relais média** (`media_relay.go`) : transfert RTP/RTCP par session entre les segments d'appel avec chiffrement/déchiffrement SRTP/SRTCP, gestion du NAT (RTP symétrique, verrouillage de port, transfert média, source stricte), blocage/silence média, hooks d'enregistrement et détection de timeout média
- **Agent ICE** (`ice_agent.go`) : négociation ICE par session avec collecte de candidats STUN/TURN, vérifications de connectivité et sélection de paires de candidats via pion/ice
- **Gestionnaire d'enregistrement** (`recording/`) : écriture de fichiers WAV en modes mixed/stereo/separate, câblé au relais via le hook OnPacket
- **Gestionnaire RTCP** (`rtcp_handler.go`) : surveillance de la qualité avec gigue, perte, RTT, calcul MOS (ITU-T G.107)
- **Passerelle T.38** (`t38_gateway.go`) : relais UDPTL avec analyse IFP, redondance, génération SDP et cycle de vie des sessions fax
- **Exportateur HEP** (`hep.go`) : export de paquets HEPv3 vers les serveurs de capture Homer/SIPVicious
- **Gestionnaire de cluster** (`cluster_manager.go`) : basculement de sessions adossé à Redis avec détection de split-brain et hachage cohérent
- **Gestionnaire FEC** (`fec_handler.go`) : correction d'erreur directe basée sur XOR
- **Tampon de gigue** (`jitter_buffer.go`) : mise en mémoire tampon adaptative (20-200ms) avec réordonnancement des séquences
- **Outil CLI** (`cmd/karl-ctl/`) : interface en ligne de commande pour la gestion des sessions, les statistiques, l'enregistrement et les vérifications de santé

---

## Feuille de route

Karl est un remplacement complet de rtpengine. Consultez la [ROADMAP.md](./ROADMAP.md) détaillée pour les détails d'implémentation.

### Phase 1 : Parité du plan de contrôle ✅ Terminée
- [x] Support complet des drapeaux du protocole NG (100+ drapeaux)
- [x] Analyse bencode, corrélation de cookies, listener UDP/Unix socket
- [x] Compatibilité du format de réponse

### Phase 2 : Média de qualité relais ✅ Terminée
- [x] Relais média par session avec sockets UDP par segment
- [x] Traversée de NAT : RTP symétrique, verrouillage de port, transfert média, source stricte
- [x] Véritable empreinte DTLS (certificat ECDSA auto-signé) et identifiants ICE aléatoires
- [x] Véritables clés crypto SDES (conforme RFC 4568)
- [x] Blocage, silence média, suppression automatique par timeout média
- [x] Hook d'enregistrement câblé dans le chemin du relais média

### Phase 3 : Fonctionnalités entreprise ✅ Terminée
- [x] Enregistrement d'appels : sortie WAV, modes mixed/stereo/separate, câblé au chemin média
- [x] Transcodage G.711 PCMU↔PCMA (table de correspondance)
- [x] Transcodage Opus/G.729/AMR via tags de build CGO (libopus, bcg729, opencore-amrnb)
- [x] Passerelle SRTP↔RTP : chiffrement/déchiffrement par segment câblé dans le relais média (chemins de clés SDES + DTLS)
- [x] Chiffrement SRTCP pour le transfert RTCP
- [x] Passerelle fax T.38 : relais UDPTL, analyse IFP, analyse/génération SDP, transfert de sockets UDP
- [x] Agent ICE complet : collecte de candidats STUN/TURN, vérifications de connectivité, sélection de paires de candidats
- [x] Négociation de codecs (RFC 3264) : intersection avec ordonnancement par préférence de l'offrant
- [x] Calcul MOS (modèle E ITU-T G.107) à partir de la gigue/perte/RTT RTCP
- [x] Sélection d'interface basée sur la direction : IP de liaison vs annonce, routage interne/externe
- [x] Support multi-flux : vidéo et médias supplémentaires via StreamPair
- [x] Cluster Redis : stockage de sessions + pub/sub + détection de split-brain + basculement
- [x] HEP/Homer : exportateur HEPv3 pour la capture SIP/RTCP
- [x] Outil CLI : `karl-ctl` pour la gestion des sessions, les statistiques, l'enregistrement, la santé

### Phase 4 : Maturité opérationnelle ✅ Terminée
- [x] Plus de 1300 tests avec détection de courses, zéro échec
- [x] Récupération de panique dans les goroutines de relais
- [x] Protection contre les fuites de ressources (nettoyage de ports en cas d'erreur, application de MaxSessions)
- [x] Sécurité : limitation de débit, authentification API, protection DoS, disjoncteurs
- [x] Métriques Prometheus (50+), sondes de santé, journalisation structurée

### Phase 5 : Tests et validation (en cours)
- [x] Tests unitaires et tests d'intégration pour le relais média (offre→réponse→flux RTP→suppression)
- [ ] Tests d'intégration avec des proxys SIP en production (OpenSIPS, Kamailio)
- [ ] Tests de chaos, tests de charge soutenue

---

## Contribuer

Les contributions sont les bienvenues ! Veuillez consulter notre [Guide de contribution](CONTRIBUTING.md) pour :

- Les directives de style de code
- Le processus de pull request
- La configuration de l'environnement de développement
- Les exigences de test

### Compiler depuis les sources

```bash
git clone https://github.com/loreste/karl.git
cd karl
go mod download
go build -o karl
go test ./...
```

---

## Licence

Karl Media Server est distribué sous la [Licence Publique Générale GNU v3.0](LICENSE).

---

## Remerciements

Karl s'appuie sur le travail de la communauté open source :

- [Pion](https://github.com/pion) - Bibliothèques WebRTC, DTLS, SRTP et RTP
- [rtpengine](https://github.com/sipwise/rtpengine) - Spécification du protocole et référence de compatibilité

---

## Support

- **Documentation** : [Documentation complète](./docs/README.md)
- **Guides pratiques** : [Guides étape par étape](./docs/how-to)
- **Problèmes** : [GitHub Issues](https://github.com/loreste/karl/issues)
- **Discussions** : [GitHub Discussions](https://github.com/loreste/karl/discussions)

---

**Karl Media Server** — Conçu pour les ingénieurs VoIP qui ont besoin d'une infrastructure média fiable, observable et cloud-native.
