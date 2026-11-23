# Comment mettre Karl à l'échelle horizontalement

Ce guide couvre l'exécution de plusieurs instances de Karl pour la haute disponibilité et l'augmentation de capacité.

## Table des matières

- [Aperçu](#aperçu)
- [Prérequis](#prérequis)
- [Architecture](#architecture)
- [Fonctionnalités du cluster](#fonctionnalités-du-cluster)
- [Configuration de Redis](#configuration-de-redis)
- [Configurer les instances Karl](#configurer-les-instances-karl)
- [Répartition de charge](#répartition-de-charge)
- [Basculement automatique](#basculement-automatique)
- [Mise à l'échelle Kubernetes](#mise-à-léchelle-kubernetes)
- [Surveillance des clusters](#surveillance-des-clusters)
- [Dépannage](#dépannage)

---

## Aperçu

Karl prend en charge la mise à l'échelle horizontale grâce à :

- **Partage de sessions via Redis** : Toutes les instances partagent l'état des sessions
- **Hachage cohérent** : Routage déterministe des sessions avec redistribution minimale
- **Basculement automatique** : Les sessions migrent automatiquement en cas de défaillance de nœud
- **Détection de split-brain** : Tolérance aux partitions basée sur le quorum
- **Intégration d'équilibreur de charge** : Distribution du trafic entre les instances

Avantages :
- Gérer plus d'appels simultanés
- Haute disponibilité avec basculement automatique
- Mises à jour progressives sans interruption
- Pas de perte de session lors des défaillances de nœuds

---

## Prérequis

- Serveur Redis (autonome ou cluster)
- Équilibreur de charge ou connexions multiples au proxy SIP
- Stockage partagé pour les enregistrements (si activé)

---

## Architecture

### Instance unique

```
┌─────────────┐         ┌─────────────┐
│  Proxy SIP  │────────▶│    Karl     │
└─────────────┘         └─────────────┘
```

### Mise à l'échelle horizontale

```
                        ┌─────────────┐
                   ┌───▶│   Karl 1    │───┐
┌─────────────┐    │    └─────────────┘   │    ┌─────────────┐
│  Proxy SIP  │────┤                      ├───▶│    Redis    │
└─────────────┘    │    ┌─────────────┐   │    └─────────────┘
                   ├───▶│   Karl 2    │───┤
                   │    └─────────────┘   │
                   │    ┌─────────────┐   │
                   └───▶│   Karl 3    │───┘
                        └─────────────┘
```

---

## Fonctionnalités du cluster

Karl inclut des capacités de clustering de niveau entreprise :

### Hachage cohérent

Les sessions sont distribuées à l'aide du hachage cohérent avec 150 nœuds virtuels par nœud physique :

- **Redistribution minimale** : Lorsque des nœuds rejoignent/quittent, seules ~1/N sessions se déplacent
- **Routage déterministe** : La même session route toujours vers le même nœud
- **Équilibrage de charge** : Les nœuds virtuels assurent une distribution uniforme

### Prise en charge automatique des sessions

Lorsqu'un nœud tombe en panne, ses sessions sont automatiquement reprises :

1. **Défaillance du nœud détectée** via le délai d'expiration des battements de cœur (3 battements manqués)
2. **Anneau de hachage mis à jour** pour exclure le nœud défaillant
3. **Sessions redistribuées** à l'aide du hachage cohérent
4. **Ports réalloués** sur le nouveau nœud
5. **Flux média reprend** sans intervention du proxy SIP

### Détection de split-brain

Karl gère intelligemment les partitions réseau :

- **Décisions basées sur le quorum** : Les nœuds sans quorum passent en mode lecture seule
- **Isolement automatique** : La partition minoritaire arrête d'accepter de nouveaux appels
- **Gestion de la récupération** : Re-fusion gracieuse lorsque la partition guérit
- **Taille de quorum configurable** : Par défaut 2 nœuds

### Architecture du cluster

```
┌─────────────────────────────────────────────────────────────────┐
│                      ClusterManager                              │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐ │
│  │ RedisSession    │  │   HashRing /    │  │  SplitBrain     │ │
│  │    Store        │  │ SessionRouter   │  │   Detector      │ │
│  │                 │  │                 │  │                 │ │
│  │ - CRUD session  │  │ - Hachage       │  │ - Détection     │ │
│  │ - Index nœud    │  │   cohérent      │  │   partition     │ │
│  │ - Ops batch     │  │ - Équilibrage   │  │ - Gestion       │ │
│  │ - Pub/Sub       │  │ - Sessions      │  │   quorum        │ │
│  │ - Pool workers  │  │   persistantes  │  │ - Isolement     │ │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

---

## Configuration de Redis

### Option 1 : Docker

```bash
docker run -d \
  --name redis \
  -p 6379:6379 \
  redis:7-alpine
```

### Option 2 : Docker Compose

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

### Option 3 : Redis géré

Utilisez des services gérés pour la production :
- AWS ElastiCache
- Google Cloud Memorystore
- Azure Cache for Redis

### Vérifier Redis

```bash
redis-cli ping
# Devrait retourner : PONG
```

---

## Configurer les instances Karl

### Fichier de configuration

Chaque instance utilise la même configuration avec Redis activé :

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

### Variables d'environnement

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

### Allocation de ports

Évitez les conflits de ports en attribuant différentes plages de ports RTP :

| Instance | Ports RTP |
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

### Docker Compose multi-instances

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

## Répartition de charge

### Configuration du proxy SIP

#### OpenSIPS

```opensips
# Plusieurs instances Karl avec poids
modparam("rtpengine", "rtpengine_sock",
    "udp:karl1:22222 udp:karl2:22222 udp:karl3:22222")

# Ou avec priorité de basculement
modparam("rtpengine", "rtpengine_sock",
    "1 == udp:karl1:22222")
modparam("rtpengine", "rtpengine_sock",
    "2 == udp:karl2:22222")
modparam("rtpengine", "rtpengine_sock",
    "3 == udp:karl3:22222")
```

#### Kamailio

```kamailio
# Équilibrage de charge
modparam("rtpengine", "rtpengine_sock",
    "udp:karl1:22222 udp:karl2:22222 udp:karl3:22222")

# Distribution pondérée
modparam("rtpengine", "rtpengine_sock",
    "2 == udp:karl1:22222 2 == udp:karl2:22222 1 == udp:karl3:22222")
```

### HAProxy pour l'API

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

## Basculement automatique

Karl gère automatiquement les défaillances de nœuds sans intervention manuelle.

### Fonctionnement du basculement

1. **Surveillance des battements de cœur** : Chaque nœud envoie des battements de cœur toutes les 1 seconde
2. **Détection de défaillance** : Nœud marqué défaillant après 3 battements manqués (3s)
3. **Découverte des sessions** : Sessions du nœud défaillant récupérées depuis l'index Redis
4. **Redistribution cohérente** : L'anneau de hachage détermine les nouveaux propriétaires
5. **Allocation de ports** : Nouveaux ports alloués depuis un pool pré-chauffé
6. **Transfert d'état** : Propriété des sessions mise à jour atomiquement dans Redis

### Configuration du basculement

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

| Paramètre | Description | Défaut |
|-----------|-------------|--------|
| `heartbeat_interval` | Temps entre les battements de cœur | 1s |
| `failure_threshold` | Battements manqués avant défaillance | 3 |
| `quorum_size` | Nœuds minimum pour le quorum | 2 |
| `replication_factor` | Nœuds virtuels par nœud physique | 150 |
| `enable_fencing` | Isoler les nœuds sans quorum | true |

### Performance du basculement

| Métrique | Valeur |
|----------|--------|
| Temps de détection | 3 secondes |
| Reprise de session | Parallèle (2xCPU workers) |
| Allocation de ports | Pool pré-chauffé (< 1ms) |
| Basculement typique | < 5 secondes au total |

### Surveiller le basculement

```promql
# Sessions reprises
karl_cluster_takeovers_done

# Échecs de reprise
karl_cluster_takeovers_failed

# Défaillances de nœuds détectées
karl_cluster_node_failures

# Événements de partition
karl_cluster_partition_events
```

---

## Mise à l'échelle Kubernetes

### Mise à l'échelle de base

```bash
# Mettre à l'échelle à 3 réplicas
kubectl scale deployment karl --replicas=3

# Vérifier l'état
kubectl get pods -l app=karl
```

### Autoscaler horizontal de pods

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

### Budget de perturbation des pods

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

## Surveillance des clusters

### Métriques agrégées

```promql
# Total des sessions actives sur toutes les instances
sum(karl_sessions_active)

# Sessions par instance
karl_sessions_active

# Distribution des sessions (devrait être uniforme)
karl_sessions_active / ignoring(instance) group_left sum(karl_sessions_active)
```

### Santé des instances

```promql
# Instances en service
count(up{job="karl"} == 1)

# Instance avec le plus de sessions
topk(1, karl_sessions_active)

# Instance avec le moins de sessions
bottomk(1, karl_sessions_active)
```

---

## Dépannage

### Sessions non partagées

1. **Vérifier la connectivité Redis** :
```bash
redis-cli -h redis ping
```

2. **Vérifier que Redis est activé** :
```bash
curl http://karl1:8080/api/v1/stats | jq .redis_enabled
```

3. **Vérifier les clés Redis** :
```bash
redis-cli keys "karl:session:*"
```

### Distribution de charge inégale

1. **Vérifier la configuration du proxy SIP**
2. **Vérifier que toutes les instances sont saines**
3. **Vérifier la connectivité réseau**

### Bonnes pratiques

1. **Utiliser un nombre impair d'instances** (3, 5, 7) pour les décisions de quorum
2. **Minimum 3 nœuds** pour la production avec basculement automatique
3. **Planifier une capacité N-1** - le cluster devrait supporter la charge avec un nœud en moins

---

## Étapes suivantes

- [Déploiement Kubernetes](./deploying-kubernetes.md)
- [Configuration de la surveillance](./monitoring-prometheus.md)
- [Guide de dépannage](./troubleshooting.md)
