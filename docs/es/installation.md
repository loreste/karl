# Guia de Instalacion

Esta guia cubre todos los metodos de instalacion del Servidor de Medios Karl.

## Tabla de Contenidos

- [Requisitos del Sistema](#requisitos-del-sistema)
- [Desde el Codigo Fuente](#desde-el-codigo-fuente)
- [Docker](#docker)
- [Docker Compose](#docker-compose)
- [Kubernetes](#kubernetes)
- [Servicio Systemd](#servicio-systemd)
- [Compilacion desde el Codigo Fuente](#compilacion-desde-el-codigo-fuente)

---

## Requisitos del Sistema

### Requisitos Minimos

| Recurso | Minimo | Recomendado |
|---------|--------|-------------|
| CPU | 2 nucleos | 4+ nucleos |
| RAM | 1 GB | 4+ GB |
| Disco | 100 MB | + almacenamiento de grabaciones |
| Red | 100 Mbps | 1 Gbps |
| SO | Linux (kernel 4.x+) | Linux (kernel 5.x+) |

### Plataformas Soportadas

- Linux (amd64, arm64)
- macOS (amd64, arm64) - solo desarrollo
- Windows (amd64) - solo desarrollo

### Requisitos de Red

| Puerto | Protocolo | Proposito |
|--------|-----------|-----------|
| 22222 | UDP | Protocolo NG (comunicacion con proxy SIP) |
| 30000-40000 | UDP | Medios RTP/RTCP (rango configurable) |
| 8080 | TCP | API REST |
| 8086 | TCP | Health checks |
| 9091 | TCP | Metricas de Prometheus |

---

## Desde el Codigo Fuente

### Requisitos Previos

- Go 1.26 o superior
- Git

### Pasos

```bash
# Clonar repositorio
git clone https://github.com/loreste/karl.git
cd karl

# Descargar dependencias
go mod download

# Compilar
go build -o karl

# Verificar compilacion
./karl --version
```

### Instalar a Nivel del Sistema

```bash
# Copiar binario
sudo cp karl /usr/local/bin/

# Crear directorios
sudo mkdir -p /etc/karl
sudo mkdir -p /var/lib/karl/recordings
sudo mkdir -p /var/run/karl

# Crear usuario
sudo useradd -r -s /bin/false karl

# Establecer permisos
sudo chown -R karl:karl /var/lib/karl
sudo chown -R karl:karl /var/run/karl

# Copiar configuracion
sudo cp config/config.json /etc/karl/
sudo chown karl:karl /etc/karl/config.json
```

---

## Docker

### Inicio Rapido

```bash
# Descargar la imagen mas reciente
docker pull loreste/karl:latest

# Ejecutar con red del host (recomendado)
docker run -d \
  --name karl \
  --network host \
  --restart unless-stopped \
  loreste/karl:latest
```

### Con Configuracion Personalizada

```bash
# Crear directorio de configuracion
mkdir -p /opt/karl/config
mkdir -p /opt/karl/recordings

# Copiar su configuracion
cp config.json /opt/karl/config/

# Ejecutar con volumenes montados
docker run -d \
  --name karl \
  --network host \
  -v /opt/karl/config:/etc/karl:ro \
  -v /opt/karl/recordings:/var/lib/karl/recordings \
  -e KARL_CONFIG_PATH=/etc/karl/config.json \
  --restart unless-stopped \
  loreste/karl:latest
```

### Con Mapeo de Puertos (Rango RTP Limitado)

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

### Compilar Imagen Docker

```bash
# Clonar repositorio
git clone https://github.com/loreste/karl.git
cd karl

# Compilar imagen
docker build -t karl:local .

# Ejecutar imagen local
docker run -d --name karl --network host karl:local
```

---

## Docker Compose

### Configuracion Basica

Crear `docker-compose.yml`:

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

### Stack Completo con Redis y MySQL

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

### Iniciar el Stack

```bash
# Iniciar todos los servicios
docker-compose up -d

# Ver logs
docker-compose logs -f karl

# Detener todos los servicios
docker-compose down
```

---

## Kubernetes

Consulte la guia dedicada de [Despliegue en Kubernetes](./how-to/deploying-kubernetes.md) para instrucciones completas.

### Despliegue Rapido

```bash
# Clonar repositorio
git clone https://github.com/loreste/karl.git
cd karl

# Desplegar usando kustomize
kubectl apply -k deploy/kubernetes/

# Verificar estado
kubectl get pods -l app=karl
```

---

## Servicio Systemd

### Crear Archivo de Servicio

Crear `/etc/systemd/system/karl.service`:

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

# Endurecimiento de seguridad
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/karl /var/run/karl
PrivateTmp=true

# Limites de recursos
LimitNOFILE=65535
LimitNPROC=4096

[Install]
WantedBy=multi-user.target
```

### Habilitar e Iniciar

```bash
# Recargar systemd
sudo systemctl daemon-reload

# Habilitar servicio
sudo systemctl enable karl

# Iniciar servicio
sudo systemctl start karl

# Verificar estado
sudo systemctl status karl

# Ver logs
sudo journalctl -u karl -f
```

---

## Compilacion desde el Codigo Fuente

### Compilacion de Desarrollo

```bash
# Clonar
git clone https://github.com/loreste/karl.git
cd karl

# Instalar dependencias
go mod download

# Ejecutar pruebas
go test ./...

# Compilar con simbolos de depuracion
go build -o karl

# Ejecutar
./karl
```

### Compilacion de Produccion

```bash
# Compilar binario optimizado
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -ldflags="-s -w" \
  -o karl

# Verificar binario
file karl
```

### Compilacion Cruzada

```bash
# Linux ARM64
GOOS=linux GOARCH=arm64 go build -o karl-linux-arm64

# macOS ARM64
GOOS=darwin GOARCH=arm64 go build -o karl-darwin-arm64

# Windows
GOOS=windows GOARCH=amd64 go build -o karl.exe
```

---

## Verificar la Instalacion

Despues de instalar Karl usando cualquier metodo:

### 1. Verificar el Proceso

```bash
# Usando systemctl
sudo systemctl status karl

# Usando Docker
docker ps | grep karl

# Usando ps
ps aux | grep karl
```

### 2. Probar el Protocolo NG

```bash
echo -n "d7:command4:pinge" | nc -u localhost 22222
```

### 3. Verificar la Salud

```bash
curl http://localhost:8086/health
```

### 4. Ver Metricas

```bash
curl http://localhost:9091/metrics | head
```

---

## Siguientes Pasos

- [Referencia de Configuracion](./configuration.md)
- [Integrar con OpenSIPS](./how-to/integrating-opensips.md)
- [Integrar con Kamailio](./how-to/integrating-kamailio.md)
- [Configurar Monitoreo](./how-to/monitoring-prometheus.md)
