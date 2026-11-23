# Como Desplegar Karl en Kubernetes

Esta guia cubre el despliegue del Servidor de Medios Karl en Kubernetes, incluyendo configuracion, escalado y mejores practicas de produccion.

## Tabla de Contenidos

- [Requisitos Previos](#requisitos-previos)
- [Despliegue Rapido](#despliegue-rapido)
- [Comprendiendo el Despliegue](#comprendiendo-el-despliegue)
- [Configuracion](#configuracion)
- [Consideraciones de Red](#consideraciones-de-red)
- [Sondas de Salud](#sondas-de-salud)
- [Escalado](#escalado)
- [Monitoreo](#monitoreo)
- [Lista de Verificacion de Produccion](#lista-de-verificacion-de-produccion)
- [Solucion de Problemas](#solucion-de-problemas)

---

## Requisitos Previos

- Cluster de Kubernetes (1.19+)
- `kubectl` configurado para acceder a su cluster
- Comprension basica de los conceptos de Kubernetes
- Para produccion: Redis (opcional, para compartir sesiones)

---

## Despliegue Rapido

```bash
# Clonar el repositorio
git clone https://github.com/loreste/karl.git
cd karl

# Desplegar usando kustomize
kubectl apply -k deploy/kubernetes/

# Verificar despliegue
kubectl get pods -l app=karl
kubectl get svc -l app=karl
```

---

## Comprendiendo el Despliegue

Los manifiestos de Kubernetes en `deploy/kubernetes/` incluyen:

| Archivo | Proposito |
|---------|-----------|
| `deployment.yaml` | Especificacion del pod de Karl con sondas y recursos |
| `service.yaml` | Servicios ClusterIP, Headless y NodePort |
| `configmap.yaml` | Configuracion de Karl |
| `pvc.yaml` | Volumen persistente para grabaciones |
| `servicemonitor.yaml` | Integracion con Prometheus |
| `kustomization.yaml` | Configuracion de Kustomize |

### Arquitectura del Despliegue

```
┌─────────────────────────────────────────────────────────────┐
│                     Cluster de Kubernetes                     │
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
│                    │   Pod de Karl   │                     │
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

## Configuracion

### Usando ConfigMap

Edite el ConfigMap para personalizar la configuracion de Karl:

```bash
# Editar ConfigMap directamente
kubectl edit configmap karl-config

# O aplicar un nuevo ConfigMap
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

### Usando Variables de Entorno

Sobreescriba la configuracion mediante variables de entorno en el deployment:

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

### Usando Secrets para Datos Sensibles

```yaml
# Crear secret
kubectl create secret generic karl-secrets \
  --from-literal=mysql-dsn='karl:password@tcp(mysql:3306)/karl'

# Referenciar en el deployment
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

## Consideraciones de Red

### Modo Host Network (Predeterminado)

El despliegue predeterminado usa `hostNetwork: true`, lo cual es recomendado para servidores de medios:

```yaml
spec:
  hostNetwork: true
  dnsPolicy: ClusterFirstWithHostNet
```

**Ventajas:**
- Acceso directo a la interfaz de red del host
- Sin NAT para trafico RTP/RTCP
- Rango completo de puertos disponible (30000-40000)
- Mejor rendimiento para medios

**Limitaciones:**
- Solo un pod de Karl por nodo
- El pod usa la direccion IP del nodo

### Modo Sin Host Network

Para entornos que requieren multiples pods por nodo:

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

**Cambios requeridos:**
1. Usar servicio NodePort para el protocolo NG
2. Limitar el rango de puertos RTP a los NodePorts disponibles
3. Configurar su proxy SIP para usar NodePort

```yaml
# Servicio NodePort para acceso externo
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

### Load Balancer (Proveedores Cloud)

Para AWS, GCP o Azure:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: karl-lb
  annotations:
    # AWS NLB
    service.beta.kubernetes.io/aws-load-balancer-type: "nlb"
    # Preservar IP del cliente
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

## Sondas de Salud

Karl implementa sondas de salud nativas de Kubernetes:

### Sonda de Inicio

Permite un inicio lento sin ser eliminado:

```yaml
startupProbe:
  httpGet:
    path: /startup
    port: 8086
  initialDelaySeconds: 5
  periodSeconds: 5
  timeoutSeconds: 3
  failureThreshold: 30  # 150 segundos maximo de inicio
```

### Sonda de Vida

Detecta deadlocks y activa un reinicio:

```yaml
livenessProbe:
  httpGet:
    path: /live
    port: 8086
  periodSeconds: 15
  timeoutSeconds: 5
  failureThreshold: 3
```

### Sonda de Disponibilidad

Controla el enrutamiento de trafico:

```yaml
readinessProbe:
  httpGet:
    path: /ready
    port: 8086
  periodSeconds: 5
  timeoutSeconds: 3
  failureThreshold: 3
```

### Endpoints de Sondas

| Endpoint | Proposito | Criterio de Exito |
|----------|-----------|-------------------|
| `/startup` | Inicializacion completa | La aplicacion esta inicializada |
| `/live` | El proceso esta sano | No hay deadlock |
| `/ready` | Listo para trafico | El listener NG esta activo |
| `/health` | Salud general | Devuelve estado |
| `/health/detail` | Estado detallado | Desglose por componente |

---

## Escalado

### Escalado Horizontal con Redis

Para multiples instancias de Karl, habilite Redis para compartir sesiones:

**Paso 1: Desplegar Redis**

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

**Paso 2: Configurar Karl**

```yaml
# En ConfigMap
{
  "database": {
    "redis_enabled": true,
    "redis_addr": "redis:6379"
  }
}

# O mediante entorno
env:
- name: KARL_REDIS_ENABLED
  value: "true"
- name: KARL_REDIS_ADDR
  value: "redis:6379"
```

**Paso 3: Escalar el Deployment**

```bash
kubectl scale deployment karl --replicas=3
```

### Recomendaciones de Recursos

| Llamadas Concurrentes | CPU Request | Memory Request | Replicas |
|-----------------------|-------------|----------------|----------|
| < 100 | 250m | 256Mi | 1 |
| 100-500 | 500m | 512Mi | 1-2 |
| 500-1000 | 1000m | 1Gi | 2-3 |
| 1000-5000 | 2000m | 2Gi | 3-5 |
| > 5000 | 2000m | 2Gi | 5+ |

### Pod Disruption Budget

Asegurar disponibilidad durante actualizaciones:

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

## Monitoreo

### Integracion con Prometheus

Aplique el ServiceMonitor para Prometheus Operator:

```bash
kubectl apply -f deploy/kubernetes/servicemonitor.yaml
```

O cree manualmente:

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

### Configuracion de Scrape de Prometheus

Para configuraciones sin Operator:

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

### Metricas Clave a Monitorear

```promql
# Sesiones activas
karl_sessions_active

# Tasa de creacion de sesiones
rate(karl_sessions_total[5m])

# Perdida de paquetes
karl_rtcp_packet_loss_fraction

# Latencia de comandos
histogram_quantile(0.99, rate(karl_ng_command_duration_seconds_bucket[5m]))
```

---

## Lista de Verificacion de Produccion

### Antes de Desplegar

- [ ] Configurar requests y limits de recursos
- [ ] Configurar sondas de salud
- [ ] Configurar almacenamiento persistente para grabaciones
- [ ] Configurar Redis para despliegues multi-instancia
- [ ] Configurar monitoreo con Prometheus
- [ ] Configurar agregacion de logs
- [ ] Configurar politicas de red si es necesario

### Configuracion del Deployment

- [ ] Usar tags de imagen especificos, no `latest`
- [ ] Configurar Pod Disruption Budget
- [ ] Configurar afinidad de nodo para cargas de trabajo de medios
- [ ] Configurar anti-afinidad para alta disponibilidad
- [ ] Establecer contexto de seguridad apropiado

### Red

- [ ] Verificar que el rango de puertos RTP es accesible
- [ ] Configurar reglas de firewall para trafico UDP
- [ ] Probar conectividad del protocolo NG desde el proxy SIP
- [ ] Verificar que los endpoints de salud son accesibles

### Monitoreo

- [ ] Prometheus recopilando metricas
- [ ] Dashboard de Grafana configurado
- [ ] Reglas de alerta definidas
- [ ] Agregacion de logs funcionando

---

## Solucion de Problemas

### El Pod No Inicia

```bash
# Verificar estado del pod
kubectl describe pod -l app=karl

# Verificar logs
kubectl logs -l app=karl

# Verificar eventos
kubectl get events --sort-by='.lastTimestamp'
```

### Fallan las Verificaciones de Salud

```bash
# Probar sonda de inicio
kubectl exec -it <pod-name> -- curl localhost:8086/startup

# Probar sonda de vida
kubectl exec -it <pod-name> -- curl localhost:8086/live

# Probar sonda de disponibilidad
kubectl exec -it <pod-name> -- curl localhost:8086/ready
```

### El Protocolo NG No Funciona

```bash
# Probar desde dentro del cluster
kubectl run test --rm -it --image=alpine -- sh
apk add netcat-openbsd
echo -n "d7:command4:pinge" | nc -u karl 22222

# Verificar endpoints del servicio
kubectl get endpoints karl
```

### Sin Audio en las Llamadas

1. Verificar que los puertos RTP son accesibles:
```bash
# Verificar binding de puertos
kubectl exec -it <pod-name> -- ss -ulnp | grep 30000
```

2. Verificar reglas de firewall que bloqueen UDP

3. Verificar la configuracion de media_ip:
```bash
kubectl exec -it <pod-name> -- env | grep KARL_MEDIA_IP
```

### Ver Logs Detallados

```bash
# Transmitir logs
kubectl logs -l app=karl -f

# Con nivel debug
kubectl set env deployment/karl KARL_LOG_LEVEL=debug
```

---

## Siguientes Pasos

- [Configurar Grabacion](./setting-up-recording.md)
- [Configurar Monitoreo](./monitoring-prometheus.md)
- [Escalar Horizontalmente](./scaling-horizontally.md)
