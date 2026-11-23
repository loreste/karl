# Guide d'installation

Ce guide couvre toutes les méthodes d'installation du serveur multimédia Karl.

## Table des matières

- [Configuration requise](#configuration-requise)
- [Depuis les sources](#depuis-les-sources)
- [Docker](#docker)
- [Docker Compose](#docker-compose)
- [Kubernetes](#kubernetes)
- [Service systemd](#service-systemd)
- [Compilation depuis les sources](#compilation-depuis-les-sources)

---

## Configuration requise

### Exigences minimales

| Ressource | Minimum | Recommandé |
|-----------|---------|------------|
| CPU | 2 cœurs | 4+ cœurs |
| RAM | 1 Go | 4+ Go |
| Disque | 100 Mo | + stockage des enregistrements |
| Réseau | 100 Mbps | 1 Gbps |
| OS | Linux (noyau 4.x+) | Linux (noyau 5.x+) |

### Plateformes prises en charge

- Linux (amd64, arm64)
- macOS (amd64, arm64) - développement uniquement
- Windows (amd64) - développement uniquement

### Configuration réseau requise

| Port | Protocole | Utilisation |
|------|-----------|-------------|
| 22222 | UDP | Protocole NG (communication avec le proxy SIP) |
| 30000-40000 | UDP | Média RTP/RTCP (plage configurable) |
| 8080 | TCP | API REST |
| 8086 | TCP | Vérifications de santé |
| 9091 | TCP | Métriques Prometheus |

---

## Depuis les sources

### Prérequis

- Go 1.26 ou supérieur
- Git

### Étapes

```bash
# Cloner le dépôt
git clone https://github.com/loreste/karl.git
cd karl

# Télécharger les dépendances
go mod download

# Compiler
go build -o karl

# Vérifier la compilation
./karl --version
```

### Installation à l'échelle du système

```bash
# Copier le binaire
sudo cp karl /usr/local/bin/

# Créer les répertoires
sudo mkdir -p /etc/karl
sudo mkdir -p /var/lib/karl/recordings
sudo mkdir -p /var/run/karl

# Créer l'utilisateur
sudo useradd -r -s /bin/false karl

# Définir les permissions
sudo chown -R karl:karl /var/lib/karl
sudo chown -R karl:karl /var/run/karl

# Copier la configuration
sudo cp config/config.json /etc/karl/
sudo chown karl:karl /etc/karl/config.json
```

---

## Docker

### Démarrage rapide

```bash
# Télécharger la dernière image
docker pull loreste/karl:latest

# Exécuter avec le réseau hôte (recommandé)
docker run -d \
  --name karl \
  --network host \
  --restart unless-stopped \
  loreste/karl:latest
```

### Avec une configuration personnalisée

```bash
# Créer le répertoire de configuration
mkdir -p /opt/karl/config
mkdir -p /opt/karl/recordings

# Copier votre configuration
cp config.json /opt/karl/config/

# Exécuter avec les montages de volumes
docker run -d \
  --name karl \
  --network host \
  -v /opt/karl/config:/etc/karl:ro \
  -v /opt/karl/recordings:/var/lib/karl/recordings \
  -e KARL_CONFIG_PATH=/etc/karl/config.json \
  --restart unless-stopped \
  loreste/karl:latest
```

### Avec mappage de ports (plage RTP limitée)

```bash
docker run -d \
  --name karl \
  -p 22222:22222/udp \
  -p 30000-30100:30000-30100/udp \
  -p 8080:8080 \
  -p 8086:8086 \
  -p 9091:9091 \
  -v /opt/karl/config:/etc/karl:ro \
  -v /opt/karl/recordings:/var/lib/karl/recordings \
  --restart unless-stopped \
  loreste/karl:latest
```

### Construire l'image Docker

```bash
# Cloner le dépôt
git clone https://github.com/loreste/karl.git
cd karl

# Construire l'image
docker build -t karl:local .

# Exécuter l'image locale
docker run -d --name karl --network host karl:local
```

---

## Docker Compose

### Configuration de base

Créez `docker-compose.yml` :

```yaml
version: '3.8'

services:
  karl:
    image: loreste/karl:latest
    network_mode: host
    volumes:
      - ./config.json:/etc/karl/config.json:ro
      - recordings:/var/lib/karl/recordings
    environment:
      - KARL_CONFIG_PATH=/etc/karl/config.json
      - KARL_LOG_LEVEL=info
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8086/health"]
      interval: 30s
      timeout: 10s
      retries: 3

volumes:
  recordings:
```

### Pile complète avec Redis et MySQL

```yaml
version: '3.8'

services:
  karl:
    image: loreste/karl:latest
    network_mode: host
    volumes:
      - ./config.json:/etc/karl/config.json:ro
      - recordings:/var/lib/karl/recordings
    environment:
      - KARL_CONFIG_PATH=/etc/karl/config.json
      - KARL_MYSQL_DSN=karl:password@tcp(127.0.0.1:3306)/karl
      - KARL_REDIS_ENABLED=true
      - KARL_REDIS_ADDR=127.0.0.1:6379
    depends_on:
      mysql:
        condition: service_healthy
      redis:
        condition: service_healthy
    restart: unless-stopped

  mysql:
    image: mariadb:10.11
    environment:
      MYSQL_ROOT_PASSWORD: rootpassword
      MYSQL_DATABASE: karl
      MYSQL_USER: karl
      MYSQL_PASSWORD: password
    volumes:
      - mysql_data:/var/lib/mysql
    ports:
      - "3306:3306"
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "localhost"]
      interval: 10s
      timeout: 5s
      retries: 5
    restart: unless-stopped

  redis:
    image: redis:7-alpine
    volumes:
      - redis_data:/data
    ports:
      - "6379:6379"
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5
    restart: unless-stopped

  prometheus:
    image: prom/prometheus:latest
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml:ro
      - prometheus_data:/prometheus
    ports:
      - "9090:9090"
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.path=/prometheus'
    restart: unless-stopped

  grafana:
    image: grafana/grafana:latest
    volumes:
      - grafana_data:/var/lib/grafana
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
    restart: unless-stopped

volumes:
  recordings:
  mysql_data:
  redis_data:
  prometheus_data:
  grafana_data:
```

### Démarrer la pile

```bash
# Démarrer tous les services
docker-compose up -d

# Afficher les journaux
docker-compose logs -f karl

# Arrêter tous les services
docker-compose down
```

---

## Kubernetes

Consultez le [Guide de déploiement Kubernetes](./how-to/deploying-kubernetes.md) dédié pour des instructions complètes.

### Déploiement rapide

```bash
# Cloner le dépôt
git clone https://github.com/loreste/karl.git
cd karl

# Déployer avec kustomize
kubectl apply -k deploy/kubernetes/

# Vérifier l'état
kubectl get pods -l app=karl
```

---

## Service systemd

### Créer le fichier de service

Créez `/etc/systemd/system/karl.service` :

```ini
[Unit]
Description=Karl Media Server
Documentation=https://github.com/loreste/karl
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=karl
Group=karl
ExecStart=/usr/local/bin/karl -config /etc/karl/config.json
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

# Renforcement de la sécurité
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/karl /var/run/karl
PrivateTmp=true

# Limites de ressources
LimitNOFILE=65535
LimitNPROC=4096

[Install]
WantedBy=multi-user.target
```

### Activer et démarrer

```bash
# Recharger systemd
sudo systemctl daemon-reload

# Activer le service
sudo systemctl enable karl

# Démarrer le service
sudo systemctl start karl

# Vérifier l'état
sudo systemctl status karl

# Afficher les journaux
sudo journalctl -u karl -f
```

---

## Compilation depuis les sources

### Compilation de développement

```bash
# Cloner
git clone https://github.com/loreste/karl.git
cd karl

# Installer les dépendances
go mod download

# Exécuter les tests
go test ./...

# Compiler avec les symboles de débogage
go build -o karl

# Exécuter
./karl
```

### Compilation de production

```bash
# Compiler le binaire optimisé
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -ldflags="-s -w" \
  -o karl

# Vérifier le binaire
file karl
```

### Compilation croisée

```bash
# Linux ARM64
GOOS=linux GOARCH=arm64 go build -o karl-linux-arm64

# macOS ARM64
GOOS=darwin GOARCH=arm64 go build -o karl-darwin-arm64

# Windows
GOOS=windows GOARCH=amd64 go build -o karl.exe
```

---

## Vérifier l'installation

Après avoir installé Karl par n'importe quelle méthode :

### 1. Vérifier le processus

```bash
# Avec systemctl
sudo systemctl status karl

# Avec Docker
docker ps | grep karl

# Avec ps
ps aux | grep karl
```

### 2. Tester le protocole NG

```bash
echo -n "d7:command4:pinge" | nc -u localhost 22222
```

### 3. Vérifier la santé

```bash
curl http://localhost:8086/health
```

### 4. Afficher les métriques

```bash
curl http://localhost:9091/metrics | head
```

---

## Étapes suivantes

- [Référence de configuration](./configuration.md)
- [Intégrer avec OpenSIPS](./how-to/integrating-opensips.md)
- [Intégrer avec Kamailio](./how-to/integrating-kamailio.md)
- [Configurer la surveillance](./how-to/monitoring-prometheus.md)
