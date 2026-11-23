# Como Escalar Karl Horizontalmente

Esta guia cubre la ejecucion de multiples instancias de Karl para alta disponibilidad y mayor capacidad.

## Tabla de Contenidos

- [Resumen](#resumen)
- [Requisitos Previos](#requisitos-previos)
- [Arquitectura](#arquitectura)
- [Funciones del Cluster](#funciones-del-cluster)
- [Configuracion de Redis](#configuracion-de-redis)
- [Configurar Instancias de Karl](#configurar-instancias-de-karl)
- [Balanceo de Carga](#balanceo-de-carga)
- [Failover Automatico](#failover-automatico)
- [Escalado en Kubernetes](#escalado-en-kubernetes)
- [Monitoreo de Clusters](#monitoreo-de-clusters)
- [Solucion de Problemas](#solucion-de-problemas)

---

## Resumen

Karl soporta escalado horizontal mediante:

- **Sesiones compartidas respaldadas por Redis**: Todas las instancias comparten el estado de sesiones
- **Hashing consistente**: Enrutamiento deterministico de sesiones con redistribucion minima
- **Failover automatico**: Las sesiones migran automaticamente cuando los nodos fallan
- **Deteccion de split-brain**: Tolerancia a particiones basada en quorum
- **Integracion con load balancer**: Distribuir trafico entre instancias

Beneficios:
- Manejar mas llamadas concurrentes
- Alta disponibilidad con failover automatico
- Actualizaciones continuas sin tiempo de inactividad
- Sin perdida de sesiones durante fallos de nodos

---

## Requisitos Previos

- Servidor Redis (standalone o cluster)
- Load balancer o multiples conexiones de proxy SIP
- Almacenamiento compartido para grabaciones (si esta habilitado)

---

## Arquitectura

### Instancia Unica

```
┌─────────────┐         ┌─────────────┐
│  Proxy SIP  │────────▶│    Karl     │
└─────────────┘         └─────────────┘
```

### Escalado Horizontal

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

## Funciones del Cluster

Karl incluye capacidades de clustering de nivel empresarial:

### Hashing Consistente

Las sesiones se distribuyen usando hashing consistente con 150 nodos virtuales por nodo fisico:

- **Redistribucion minima**: Cuando los nodos se unen/salen, solo ~1/N sesiones se mueven
- **Enrutamiento deterministico**: La misma sesion siempre se enruta al mismo nodo
- **Balanceo de carga**: Los nodos virtuales aseguran una distribucion uniforme

### Toma de Control Automatica de Sesiones

Cuando un nodo falla, sus sesiones son tomadas automaticamente:

1. **Fallo de nodo detectado** via timeout de heartbeat (3 heartbeats perdidos)
2. **Anillo hash actualizado** para excluir el nodo fallido
3. **Sesiones redistribuidas** usando hashing consistente
4. **Puertos reasignados** en el nuevo nodo
5. **Flujo de medios reanudado** sin intervencion del proxy SIP

### Deteccion de Split-Brain

Karl maneja particiones de red de forma inteligente:

- **Decisiones basadas en quorum**: Los nodos sin quorum entran en modo solo lectura
- **Fencing automatico**: La particion minoritaria deja de aceptar nuevas llamadas
- **Manejo de recuperacion**: Re-fusion elegante cuando la particion sana
- **Tamano de quorum configurable**: El predeterminado es 2 nodos

### Arquitectura del Cluster

```
┌─────────────────────────────────────────────────────────────────┐
│                      ClusterManager                              │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐ │
│  │ RedisSession    │  │   HashRing /    │  │  SplitBrain     │ │
│  │    Store        │  │ SessionRouter   │  │   Detector      │ │
│  │                 │  │                 │  │                 │ │
│  │ - Session CRUD  │  │ - Hashing       │  │ - Deteccion de  │ │
│  │ - Indexacion    │  │   consistente   │  │   particion     │ │
│  │   por nodo      │  │ - Balanceo de   │  │ - Gestion de    │ │
│  │ - Ops batch     │  │   carga         │  │   quorum        │ │
│  │ - Pub/Sub       │  │ - Sesiones      │  │ - Fencing de    │ │
│  │ - Pool workers  │  │   sticky        │  │   nodos         │ │
│  │                 │  │                 │  │ - Heartbeats    │ │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

---

## Configuracion de Redis

### Opcion 1: Docker

```bash
docker run -d \
  --name redis \
  -p 6379:6379 \
  redis:7-alpine
```

### Opcion 2: Docker Compose

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

### Opcion 3: Redis Administrado

Use servicios administrados para produccion:
- AWS ElastiCache
- Google Cloud Memorystore
- Azure Cache for Redis

### Verificar Redis

```bash
redis-cli ping
# Debe devolver: PONG
```

---

## Configurar Instancias de Karl

### Archivo de Configuracion

Cada instancia usa la misma configuracion con Redis habilitado:

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

### Variables de Entorno

```bash
# Instancia 1
KARL_REDIS_ENABLED=true
KARL_REDIS_ADDR=redis:6379
KARL_MEDIA_IP=192.168.1.101

# Instancia 2
KARL_REDIS_ENABLED=true
KARL_REDIS_ADDR=redis:6379
KARL_MEDIA_IP=192.168.1.102

# Instancia 3
KARL_REDIS_ENABLED=true
KARL_REDIS_ADDR=redis:6379
KARL_MEDIA_IP=192.168.1.103
```

### Asignacion de Puertos

Evitar conflictos de puertos asignando diferentes rangos de puertos RTP:

| Instancia | Puertos RTP |
|-----------|-------------|
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

### Docker Compose Multi-Instancia

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

## Balanceo de Carga

### Configuracion del Proxy SIP

#### OpenSIPS

```opensips
# Multiples instancias de Karl con pesos
modparam("rtpengine", "rtpengine_sock",
    "udp:karl1:22222 udp:karl2:22222 udp:karl3:22222")

# O con prioridad de failover
modparam("rtpengine", "rtpengine_sock",
    "1 == udp:karl1:22222")
modparam("rtpengine", "rtpengine_sock",
    "2 == udp:karl2:22222")
modparam("rtpengine", "rtpengine_sock",
    "3 == udp:karl3:22222")
```

#### Kamailio

```kamailio
# Con balanceo de carga
modparam("rtpengine", "rtpengine_sock",
    "udp:karl1:22222 udp:karl2:22222 udp:karl3:22222")

# Distribucion ponderada
modparam("rtpengine", "rtpengine_sock",
    "2 == udp:karl1:22222 2 == udp:karl2:22222 1 == udp:karl3:22222")
```

### HAProxy para API

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

## Failover Automatico

Karl maneja automaticamente los fallos de nodos sin intervencion manual.

### Como Funciona el Failover

1. **Monitoreo de Heartbeat**: Cada nodo envia heartbeats cada 1 segundo
2. **Deteccion de Fallo**: Nodo marcado como fallido tras 3 heartbeats perdidos (3s)
3. **Descubrimiento de Sesiones**: Sesiones del nodo fallido recuperadas del indice Redis
4. **Redistribucion Consistente**: El anillo hash determina los nuevos propietarios
5. **Asignacion de Puertos**: Nuevos puertos asignados del pool pre-calentado
6. **Transferencia de Estado**: Propiedad de sesion actualizada atomicamente en Redis

### Configuracion de Failover

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

| Opcion | Descripcion | Predeterminado |
|--------|-------------|----------------|
| `heartbeat_interval` | Tiempo entre heartbeats | 1s |
| `failure_threshold` | Heartbeats perdidos antes del fallo | 3 |
| `quorum_size` | Nodos minimos para quorum | 2 |
| `replication_factor` | Nodos virtuales por nodo fisico | 150 |
| `enable_fencing` | Aislar nodos sin quorum | true |

### Rendimiento del Failover

El failover de Karl esta optimizado para velocidad:

| Metrica | Valor |
|---------|-------|
| Tiempo de deteccion | 3 segundos |
| Toma de sesion | Paralela (2xCPU workers) |
| Asignacion de puertos | Pool pre-calentado (< 1ms) |
| Failover tipico | < 5 segundos en total |

### Monitoreo del Failover

```promql
# Sesiones tomadas
karl_cluster_takeovers_done

# Fallos de toma de control
karl_cluster_takeovers_failed

# Fallos de nodo detectados
karl_cluster_node_failures

# Eventos de particion
karl_cluster_partition_events
```

---

## Escalado en Kubernetes

### Escalado Basico

```bash
# Escalar a 3 replicas
kubectl scale deployment karl --replicas=3

# Verificar estado
kubectl get pods -l app=karl
```

### Horizontal Pod Autoscaler

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

### Redis para Kubernetes

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
    metadata:
      labels:
        app: redis
    spec:
      containers:
      - name: redis
        image: redis:7-alpine
        ports:
        - containerPort: 6379
        resources:
          requests:
            memory: "128Mi"
            cpu: "100m"
          limits:
            memory: "256Mi"
            cpu: "250m"
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

### Deployment de Karl con Redis

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: karl
spec:
  replicas: 3
  selector:
    matchLabels:
      app: karl
  template:
    metadata:
      labels:
        app: karl
    spec:
      hostNetwork: true
      containers:
      - name: karl
        image: loreste/karl:latest
        env:
        - name: KARL_REDIS_ENABLED
          value: "true"
        - name: KARL_REDIS_ADDR
          value: "redis:6379"
        - name: KARL_MEDIA_IP
          valueFrom:
            fieldRef:
              fieldPath: status.hostIP
```

### Pod Disruption Budget

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

## Monitoreo de Clusters

### Metricas Agregadas

```promql
# Total de sesiones activas en todas las instancias
sum(karl_sessions_active)

# Sesiones por instancia
karl_sessions_active

# Distribucion de sesiones (debe ser uniforme)
karl_sessions_active / ignoring(instance) group_left sum(karl_sessions_active)
```

### Salud de Instancias

```promql
# Instancias activas
count(up{job="karl"} == 1)

# Instancia con mas sesiones
topk(1, karl_sessions_active)

# Instancia con menos sesiones
bottomk(1, karl_sessions_active)
```

### Monitoreo de Redis

```promql
# Estado de conexion de Redis
karl_redis_connected

# Operaciones de Redis
rate(karl_redis_operations_total[5m])
```

### Alertas para Clusters

```yaml
groups:
  - name: karl-cluster
    rules:
      - alert: KarlInstanceDown
        expr: up{job="karl"} == 0
        for: 1m
        labels:
          severity: warning
        annotations:
          summary: "La instancia de Karl {{ $labels.instance }} esta caida"

      - alert: KarlClusterDegraded
        expr: count(up{job="karl"} == 1) < 2
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "El cluster de Karl tiene menos de 2 instancias saludables"

      - alert: KarlUnevenDistribution
        expr: stddev(karl_sessions_active) / avg(karl_sessions_active) > 0.5
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "La distribucion de sesiones es desigual entre instancias de Karl"

      - alert: KarlRedisDisconnected
        expr: karl_redis_connected == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "La instancia de Karl {{ $labels.instance }} perdio la conexion Redis"

      - alert: KarlQuorumLost
        expr: karl_cluster_has_quorum == 0
        for: 30s
        labels:
          severity: critical
        annotations:
          summary: "El cluster de Karl perdio quorum - nuevas sesiones bloqueadas"

      - alert: KarlNodeFenced
        expr: karl_cluster_fenced == 1
        for: 1m
        labels:
          severity: warning
        annotations:
          summary: "La instancia de Karl {{ $labels.instance }} esta aislada"

      - alert: KarlTakeoverFailures
        expr: rate(karl_cluster_takeovers_failed[5m]) > 0
        for: 1m
        labels:
          severity: warning
        annotations:
          summary: "Fallos de toma de control de sesiones de Karl detectados"

      - alert: KarlHighTakeoverLatency
        expr: karl_cluster_takeover_latency_seconds > 10
        for: 1m
        labels:
          severity: warning
        annotations:
          summary: "La toma de control de sesiones de Karl tarda demasiado"
```

---

## Solucion de Problemas

### Las Sesiones No Se Comparten

1. **Verificar conectividad de Redis**:
```bash
# Desde el contenedor/host de Karl
redis-cli -h redis ping
```

2. **Verificar que Redis esta habilitado**:
```bash
curl http://karl1:8080/api/v1/stats | jq .redis_enabled
```

3. **Verificar claves en Redis**:
```bash
redis-cli keys "karl:session:*"
```

### Distribucion de Carga Desigual

1. **Verificar configuracion del proxy SIP**
2. **Verificar que todas las instancias estan saludables**
3. **Verificar conectividad de red**

### Fallos de Busqueda de Sesion

```bash
# Verificar que la sesion existe en Redis
redis-cli get "karl:session:abc123"

# Listar todas las sesiones
redis-cli keys "karl:session:*"
```

### Problemas de Actualizacion Continua

1. **Asegurar que el PodDisruptionBudget esta configurado**
2. **Usar terminacion elegante**:
```yaml
spec:
  terminationGracePeriodSeconds: 30
```

### Rendimiento de Redis

```bash
# Verificar informacion de Redis
redis-cli info stats

# Monitorear comandos de Redis
redis-cli monitor
```

### Problemas de Estado del Cluster

**Verificar estadisticas del cluster**:
```bash
curl http://karl1:8080/api/v1/cluster/stats | jq
```

**Ver estado del anillo hash**:
```bash
curl http://karl1:8080/api/v1/cluster/ring | jq
```

**Verificar salud de nodos**:
```bash
curl http://karl1:8080/api/v1/cluster/nodes | jq
```

### Fallos de Toma de Control de Sesiones

1. **Verificar disponibilidad de puertos**:
```bash
# Asegurar suficientes puertos en el rango
curl http://karl1:8080/api/v1/stats | jq .port_allocator
```

2. **Verificar conectividad de Redis**:
```bash
redis-cli -h redis ping
```

3. **Verificar indice de sesiones del nodo**:
```bash
redis-cli smembers "karl:session:node:failed-node-id"
```

### Recuperacion de Split Brain

1. **Verificar estado del quorum**:
```bash
curl http://karl1:8080/api/v1/cluster/stats | jq .has_quorum
```

2. **Verificar estado de aislamiento**:
```bash
curl http://karl1:8080/api/v1/cluster/stats | jq .fenced
```

3. **Forzar desaislamiento** (usar con precaucion):
```bash
curl -X POST http://karl1:8080/api/v1/cluster/unfence
```

---

## Mejores Practicas

### Dimensionamiento del Cluster

1. **Usar numero impar de instancias** (3, 5, 7) para decisiones de quorum
2. **Minimo 3 nodos** para produccion con failover automatico
3. **Planificar para capacidad N-1** - el cluster debe manejar la carga con un nodo caido

### Rendimiento

4. **Pre-calentar pools de puertos** - 500 pares de puertos predeterminados por nodo
5. **Usar Redis Cluster** para despliegues grandes (>10 nodos)
6. **Habilitar persistencia de Redis** (AOF o RDB) para durabilidad
7. **Co-ubicar Redis** con nodos Karl para menor latencia

### Fiabilidad

8. **Establecer timeouts apropiados** - 3s deteccion, 30s timeout de toma de control
9. **Monitorear estado del quorum** - alertar inmediatamente al perder quorum
10. **Probar failover regularmente** - validar que la toma de control funciona correctamente

### Operaciones

11. **Usar actualizaciones continuas** - actualizar un nodo a la vez
12. **Drenar antes de apagar** - dejar que las sesiones migren elegantemente
13. **Monitorear distribucion de sesiones** - alertar sobre carga desigual

---

## Caracteristicas de Rendimiento

| Tamano del Cluster | Tiempo de Toma | Sesiones/Nodo | Sobrecarga de Memoria |
|--------------------|----------------|---------------|----------------------|
| 3 nodos | < 3s | 3,000-5,000 | ~50MB |
| 5 nodos | < 4s | 2,000-3,000 | ~80MB |
| 10 nodos | < 5s | 1,000-2,000 | ~150MB |

---

## Siguientes Pasos

- [Despliegue en Kubernetes](./deploying-kubernetes.md)
- [Configuracion de Monitoreo](./monitoring-prometheus.md)
- [Guia de Solucion de Problemas](./troubleshooting.md)
