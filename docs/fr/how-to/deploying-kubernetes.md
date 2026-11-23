# Comment déployer Karl sur Kubernetes

Ce guide couvre le déploiement de Karl Media Server sur Kubernetes, y compris la configuration, la mise à l'échelle et les bonnes pratiques de production.

## Table des matières

- [Prérequis](#prérequis)
- [Déploiement rapide](#déploiement-rapide)
- [Comprendre le déploiement](#comprendre-le-déploiement)
- [Configuration](#configuration)
- [Considérations réseau](#considérations-réseau)
- [Sondes de santé](#sondes-de-santé)
- [Mise à l'échelle](#mise-à-léchelle)
- [Surveillance](#surveillance)
- [Checklist de production](#checklist-de-production)
- [Dépannage](#dépannage)

---

## Prérequis

- Cluster Kubernetes (1.19+)
- `kubectl` configuré pour accéder à votre cluster
- Compréhension de base des concepts Kubernetes
- Pour la production : Redis (optionnel, pour le partage de sessions)

---

## Déploiement rapide

```bash
# Cloner le dépôt
git clone https://github.com/loreste/karl.git
cd karl

# Déployer avec kustomize
kubectl apply -k deploy/kubernetes/

# Vérifier le déploiement
kubectl get pods -l app=karl
kubectl get svc -l app=karl
```

---

## Comprendre le déploiement

Les manifestes Kubernetes dans `deploy/kubernetes/` incluent :

| Fichier | Objectif |
|---------|----------|
| `deployment.yaml` | Spécification du pod Karl avec sondes et ressources |
| `service.yaml` | Services ClusterIP, Headless et NodePort |
| `configmap.yaml` | Configuration de Karl |
| `pvc.yaml` | Volume persistant pour les enregistrements |
| `servicemonitor.yaml` | Intégration Prometheus |
| `kustomization.yaml` | Configuration Kustomize |

### Architecture de déploiement

```
┌─────────────────────────────────────────────────────────────┐
│                     Cluster Kubernetes                       │
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
│                    │   Pod Karl      │                     │
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

## Configuration

### Utilisation du ConfigMap

Modifiez le ConfigMap pour personnaliser la configuration de Karl :

```bash
# Modifier le ConfigMap directement
kubectl edit configmap karl-config

# Ou appliquer un nouveau ConfigMap
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

### Utilisation des variables d'environnement

Remplacez la configuration via des variables d'environnement dans le déploiement :

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

### Utilisation des Secrets pour les données sensibles

```yaml
# Créer un secret
kubectl create secret generic karl-secrets \
  --from-literal=mysql-dsn='karl:password@tcp(mysql:3306)/karl'

# Référencer dans le déploiement
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

## Considérations réseau

### Mode réseau hôte (par défaut)

Le déploiement par défaut utilise `hostNetwork: true`, ce qui est recommandé pour les serveurs média :

```yaml
spec:
  hostNetwork: true
  dnsPolicy: ClusterFirstWithHostNet
```

**Avantages :**
- Accès direct à l'interface réseau de l'hôte
- Pas de NAT pour le trafic RTP/RTCP
- Plage de ports complète disponible (30000-40000)
- Meilleures performances pour le média

**Limitations :**
- Un seul pod Karl par noeud
- Le pod utilise l'adresse IP du noeud

### Mode réseau non-hôte

Pour les environnements nécessitant plusieurs pods par noeud :

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

**Modifications requises :**
1. Utiliser un service NodePort pour le protocole NG
2. Limiter la plage de ports RTP aux NodePorts disponibles
3. Configurer votre proxy SIP pour utiliser le NodePort

```yaml
# Service NodePort pour l'accès externe
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

### Load Balancer (fournisseurs cloud)

Pour AWS, GCP ou Azure :

```yaml
apiVersion: v1
kind: Service
metadata:
  name: karl-lb
  annotations:
    # AWS NLB
    service.beta.kubernetes.io/aws-load-balancer-type: "nlb"
    # Préserver l'IP du client
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

## Sondes de santé

Karl implémente des sondes de santé natives Kubernetes :

### Sonde de démarrage

Permet un démarrage lent sans être tué :

```yaml
startupProbe:
  httpGet:
    path: /startup
    port: 8086
  initialDelaySeconds: 5
  periodSeconds: 5
  timeoutSeconds: 3
  failureThreshold: 30  # 150 secondes de démarrage maximum
```

### Sonde de vivacité

Détecte les interblocages et déclenche un redémarrage :

```yaml
livenessProbe:
  httpGet:
    path: /live
    port: 8086
  periodSeconds: 15
  timeoutSeconds: 5
  failureThreshold: 3
```

### Sonde de disponibilité

Contrôle le routage du trafic :

```yaml
readinessProbe:
  httpGet:
    path: /ready
    port: 8086
  periodSeconds: 5
  timeoutSeconds: 3
  failureThreshold: 3
```

### Points de terminaison des sondes

| Point de terminaison | Objectif | Critère de succès |
|---------------------|----------|-------------------|
| `/startup` | Initialisation terminée | L'application est initialisée |
| `/live` | Le processus est sain | Pas d'interblocage |
| `/ready` | Prêt pour le trafic | Écouteur NG en fonctionnement |
| `/health` | Santé générale | Retourne le statut |
| `/health/detail` | Statut détaillé | Détail par composant |

---

## Mise à l'échelle

### Mise à l'échelle horizontale avec Redis

Pour plusieurs instances de Karl, activez Redis pour le partage de sessions :

**Étape 1 : Déployer Redis**

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

**Étape 2 : Configurer Karl**

```yaml
# Dans le ConfigMap
{
  "database": {
    "redis_enabled": true,
    "redis_addr": "redis:6379"
  }
}

# Ou via les variables d'environnement
env:
- name: KARL_REDIS_ENABLED
  value: "true"
- name: KARL_REDIS_ADDR
  value: "redis:6379"
```

**Étape 3 : Mettre à l'échelle le déploiement**

```bash
kubectl scale deployment karl --replicas=3
```

### Recommandations de ressources

| Appels simultanés | CPU demandé | Mémoire demandée | Réplicas |
|-------------------|-------------|-------------------|----------|
| < 100 | 250m | 256Mi | 1 |
| 100-500 | 500m | 512Mi | 1-2 |
| 500-1000 | 1000m | 1Gi | 2-3 |
| 1000-5000 | 2000m | 2Gi | 3-5 |
| > 5000 | 2000m | 2Gi | 5+ |

### Budget de perturbation des pods

Assurer la disponibilité pendant les mises à jour :

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

## Surveillance

### Intégration Prometheus

Appliquez le ServiceMonitor pour Prometheus Operator :

```bash
kubectl apply -f deploy/kubernetes/servicemonitor.yaml
```

Ou créez manuellement :

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

### Configuration de scrape Prometheus

Pour les configurations sans Operator :

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

### Métriques clés à surveiller

```promql
# Sessions actives
karl_sessions_active

# Taux de création de sessions
rate(karl_sessions_total[5m])

# Perte de paquets
karl_rtcp_packet_loss_fraction

# Latence des commandes
histogram_quantile(0.99, rate(karl_ng_command_duration_seconds_bucket[5m]))
```

---

## Checklist de production

### Avant le déploiement

- [ ] Configurer les demandes et limites de ressources
- [ ] Configurer les sondes de santé
- [ ] Configurer le stockage persistant pour les enregistrements
- [ ] Configurer Redis pour les déploiements multi-instances
- [ ] Configurer la surveillance Prometheus
- [ ] Configurer l'agrégation des journaux
- [ ] Configurer les politiques réseau si nécessaire

### Configuration du déploiement

- [ ] Utiliser des tags d'image spécifiques, pas `latest`
- [ ] Configurer le budget de perturbation des pods
- [ ] Configurer l'affinité de noeud pour les charges de travail média
- [ ] Configurer l'anti-affinité pour la haute disponibilité
- [ ] Définir le contexte de sécurité approprié

### Réseau

- [ ] Vérifier que la plage de ports RTP est accessible
- [ ] Configurer les règles de pare-feu pour le trafic UDP
- [ ] Tester la connectivité du protocole NG depuis le proxy SIP
- [ ] Vérifier que les points de terminaison de santé sont accessibles

### Surveillance

- [ ] Prometheus scrape les métriques
- [ ] Tableau de bord Grafana configuré
- [ ] Règles d'alerte définies
- [ ] Agrégation des journaux fonctionnelle

---

## Dépannage

### Le pod ne démarre pas

```bash
# Vérifier le statut du pod
kubectl describe pod -l app=karl

# Vérifier les journaux
kubectl logs -l app=karl

# Vérifier les événements
kubectl get events --sort-by='.lastTimestamp'
```

### Les vérifications de santé échouent

```bash
# Tester la sonde de démarrage
kubectl exec -it <pod-name> -- curl localhost:8086/startup

# Tester la sonde de vivacité
kubectl exec -it <pod-name> -- curl localhost:8086/live

# Tester la sonde de disponibilité
kubectl exec -it <pod-name> -- curl localhost:8086/ready
```

### Le protocole NG ne fonctionne pas

```bash
# Tester depuis l'intérieur du cluster
kubectl run test --rm -it --image=alpine -- sh
apk add netcat-openbsd
echo -n "d7:command4:pinge" | nc -u karl 22222

# Vérifier les points de terminaison du service
kubectl get endpoints karl
```

### Pas d'audio dans les appels

1. Vérifier que les ports RTP sont accessibles :
```bash
# Vérifier la liaison des ports
kubectl exec -it <pod-name> -- ss -ulnp | grep 30000
```

2. Vérifier les règles de pare-feu bloquant l'UDP

3. Vérifier la configuration de media_ip :
```bash
kubectl exec -it <pod-name> -- env | grep KARL_MEDIA_IP
```

### Afficher les journaux détaillés

```bash
# Suivre les journaux en temps réel
kubectl logs -l app=karl -f

# Avec le niveau debug
kubectl set env deployment/karl KARL_LOG_LEVEL=debug
```

---

## Prochaines étapes

- [Configurer l'enregistrement](./setting-up-recording.md)
- [Configurer la surveillance](./monitoring-prometheus.md)
- [Mise à l'échelle horizontale](./scaling-horizontally.md)
