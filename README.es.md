[English](README.md) | [中文](README.zh-CN.md) | [Français](README.fr.md) | **Español**

# Karl Media Server

## Proxy RTP de Código Abierto y Servidor de Medios para VoIP y WebRTC

[![Go Report Card](https://goreportcard.com/badge/github.com/loreste/karl)](https://goreportcard.com/report/github.com/loreste/karl)
[![Go Reference](https://pkg.go.dev/badge/github.com/loreste/karl.svg)](https://pkg.go.dev/github.com/loreste/karl)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL%203.0-blue.svg)](https://opensource.org/licenses/GPL-3.0)

Karl es un proxy de medios RTP y servidor de medios de alto rendimiento, nativo en la nube, escrito en Go. Funciona como un reemplazo moderno y directo de rtpengine, diseñado específicamente para despliegues en la nube, entornos Kubernetes e infraestructura VoIP moderna.

**Aspectos destacados:**
- Despliegue con un solo binario sin dependencias de kernel
- Compatibilidad completa con el protocolo NG de rtpengine
- Soporte nativo de Kubernetes con sondas de salud
- Puente WebRTC-a-SIP listo para usar
- Métricas Prometheus y observabilidad de nivel producción

---

## Tabla de Contenidos

- [El Problema que Resolvemos](#el-problema-que-resolvemos)
- [Por Qué Elegir Karl](#por-qué-elegir-karl)
- [Características](#características)
- [Inicio Rápido](#inicio-rápido)
- [Documentación](#documentación)
- [Instalación](#instalación)
- [Configuración](#configuración)
- [Despliegue en Kubernetes](#despliegue-en-kubernetes)
- [Integración con Proxies SIP](#integración-con-proxies-sip)
- [Referencia de API](#referencia-de-api)
- [Monitoreo y Observabilidad](#monitoreo-y-observabilidad)
- [Rendimiento](#rendimiento)
- [Arquitectura](#arquitectura)
- [Hoja de Ruta](#hoja-de-ruta)
- [Contribuir](#contribuir)
- [Licencia](#licencia)

---

## El Problema que Resolvemos

### El Desafío con los Proxies RTP Tradicionales

Ejecutar servidores de medios en entornos VoIP de producción ha sido tradicionalmente difícil. Si has operado rtpengine o proxies RTP similares, probablemente te hayas encontrado con estos desafíos:

**Dependencias de Módulos de Kernel**
Los proxies RTP tradicionales requieren módulos de kernel para rendimiento. Estos módulos se rompen con cada actualización de kernel, requieren recompilación y son incompatibles con entornos en contenedores. Ejecutarlos en servicios Kubernetes administrados como EKS, GKE o AKS es imposible o requiere contenedores privilegiados con acceso al host.

**Despliegue Complejo**
Instalar rtpengine significa gestionar múltiples repositorios, compilar módulos de kernel, resolver dependencias de bibliotecas C y mantener pipelines de compilación complejos. Una simple actualización puede llevar horas de inactividad.

**Compatibilidad Limitada con la Nube**
La mayoría de los proxies RTP fueron diseñados para servidores bare-metal. Tienen dificultades con:
- Direcciones IP dinámicas en entornos de nube
- Grupos de autoescalado y orquestación de contenedores
- Traversal de NAT en redes VPC
- Health checks de balanceadores de carga

**Observabilidad Deficiente**
Depurar problemas de calidad de medios en producción requiere una visibilidad profunda en la pérdida de paquetes, jitter y rendimiento de codecs. Las herramientas tradicionales ofrecen métricas limitadas y ninguna integración nativa con stacks de observabilidad modernos como Prometheus y Grafana.

**WebRTC como Algo Secundario**
A medida que crece la adopción de WebRTC, conectar llamadas basadas en navegador con infraestructura SIP tradicional requiere configuración compleja y, a menudo, componentes separados.

### Cómo Karl Resuelve Estos Problemas

Karl fue construido desde cero para abordar cada uno de estos desafíos:

| Desafío | Proxy RTP Tradicional | Karl |
|---------|----------------------|------|
| Módulos de kernel | Requeridos, se rompen con actualizaciones | No requeridos |
| Despliegue | Horas de configuración | Un solo binario, segundos para desplegar |
| Nube/Kubernetes | Se necesitan soluciones alternativas | Soporte nativo |
| Observabilidad | Limitada | Más de 50 métricas Prometheus |
| WebRTC | Componente separado | Integrado |
| Soporte de contenedores | Difícil | Ciudadano de primera clase |

---

## Por Qué Elegir Karl

### Cero Dependencias de Kernel

Karl se ejecuta completamente en espacio de usuario. Sin módulos de kernel que compilar, sin contenedores privilegiados, sin problemas de compatibilidad con servicios Kubernetes administrados. Despliega en cualquier distribución Linux, cualquier proveedor de nube, cualquier orquestador de contenedores.

```bash
# Esta es toda la instalación
go build -o karl && ./karl
```

### Compatibilidad Completa con rtpengine

Karl implementa el protocolo NG (Next Generation) completo utilizado por rtpengine. Tu configuración existente de OpenSIPS o Kamailio funciona sin modificación. Sin cambios en tu proxy SIP, sin complejidad de migración.

### Arquitectura Nativa en la Nube

Construido para infraestructura moderna:
- **Sondas de salud nativas de Kubernetes**: Endpoints de startup, liveness y readiness
- **Configuración por variables de entorno**: Fácil integración con ConfigMaps y Secrets
- **Escalado horizontal**: Sesiones compartidas respaldadas por Redis para despliegues multi-instancia
- **Apagado graceful**: Manejo adecuado de SIGTERM para despliegues sin tiempo de inactividad

### Observabilidad de Nivel Producción

Sabe exactamente lo que está pasando en tu infraestructura de medios:
- **Más de 50 métricas Prometheus** cubriendo sesiones, calidad RTCP, recuperación FEC y rendimiento de API
- **Logging estructurado en JSON** para fácil ingestión en ELK, Splunk o CloudWatch
- **Métricas de calidad en tiempo real** incluyendo puntajes MOS, jitter y pérdida de paquetes por llamada
- **Registros de Detalle de Llamada (CDR)** con estadísticas completas de calidad

### WebRTC Nativo

WebRTC no es un añadido—es una característica central:
- Soporte **ICE/STUN/TURN** para traversal de NAT
- Puente de cifrado **DTLS-SRTP** entre WebRTC y SIP
- **Transcodificación de codec Opus** a G.711 y viceversa
- **Estimación de ancho de banda** con soporte Transport-CC

---

## Características

### Comandos del Protocolo NG

Compatibilidad completa con el protocolo NG de rtpengine:

| Comando | Descripción |
|---------|-------------|
| `ping` | Health check y keepalive |
| `offer` | Procesar oferta SDP, asignar puertos de medios |
| `answer` | Procesar respuesta SDP, completar configuración de sesión |
| `delete` | Terminar sesión, liberar recursos |
| `query` | Obtener estadísticas de sesión en tiempo real |
| `list` | Enumerar todas las llamadas activas |
| `start recording` | Iniciar grabación de llamada |
| `stop recording` | Detener grabación de llamada |
| `block media` | Silenciar/activar flujos de medios |
| `play DTMF` | Inyectar tonos DTMF |

### Procesamiento de Medios

- **Jitter Buffer Adaptativo**: Buffering dinámico (20-200ms) con ajuste automático basado en condiciones de red
- **Forward Error Correction**: FEC basado en XOR con redundancia adaptativa (10-50%) basada en pérdida de paquetes en tiempo real
- **Procesamiento RTCP**: RFC 3550 completo con reportes SR/RR, RTT, jitter, cálculo de MOS (ITU-T G.107 E-model)
- **Reenvío RTCP**: Reportes RTCP retransmitidos entre legs con cifrado SRTCP opcional
- **SRTP/SRTCP**: Cifrado/descifrado por leg conectado al relay de medios — las claves SDES del SDP crean contextos criptográficos, claves DTLS vía `SetDTLSKeys()`. Soporte completo de SRTCP para RTCP cifrado.
- **ICE Agent**: ICE por sesión con recolección de candidatos server-reflexive de STUN y relay de TURN, verificaciones de conectividad, selección de par de candidatos y consent freshness vía pion/ice
- **Negociación de Codecs**: Intersección RFC 3264 — codecs de respuesta intersectados con el conjunto del oferente, ordenados por preferencia del oferente
- **Soporte de Codecs**: Transcodificación G.711 (PCMU/PCMA) integrada; Opus, G.729, AMR disponibles vía build tags CGO (`-tags opus`, `-tags g729`, `-tags amr`) enlazando libopus, bcg729, opencore-amrnb respectivamente
- **T.38 Fax Gateway**: Relay UDPTL completo con análisis de paquetes IFP, redundancia, análisis/generación SDP, gestión de ciclo de vida de sesión y reenvío de sockets UDP
- **Selección de Interfaz Basada en Dirección**: Interfaces de red nombradas con IPs separadas de bind/advertise — el proxy SIP envía un array `direction` para seleccionar direcciones de medios internas vs externas
- **SIPREC**: Análisis de metadatos RFC 7865/7866 y estructuras de sesión (salida de grabación conectada vía relay de medios)
- **Multi-Stream**: Streams de video y medios adicionales soportados vía entradas `StreamPair` por sesión, cada una con relay independiente

### Grabación de Llamadas

Capacidades profesionales de grabación:

| Modo | Descripción |
|------|-------------|
| Mixed | Archivo mono único con ambas partes |
| Stereo | Canal izquierdo llamante, canal derecho llamado |
| Separate | Archivos individuales por leg de llamada |
| SIPREC | Grabación de sesión compatible con RFC 7865/7866 |

- **Formato**: WAV (PCM 16-bit) a tasas de muestreo configurables
- **Almacenamiento**: Sistema de archivos local o almacenamiento en red
- **Retención**: Limpieza automática basada en políticas configurables
- **Failover**: Continuidad de grabación ante fallos de nodo

### Clustering y Alta Disponibilidad

Soporte de clustering de nivel empresarial:

- **Estado de sesión respaldado por Redis**: Compartición distribuida de sesiones entre nodos
- **Hashing consistente**: Ubicación sticky de sesiones con failover
- **Detección de split-brain**: Tolerancia a particiones basada en quórum
- **Reasignación de puertos**: Asignación consistente de puertos durante failover
- **Coordinación de CDR**: Agregación distribuida de registros de detalle de llamada
- **Notificación a proxy**: Notificación automática al proxy SIP ante failover

### Seguridad

Características de seguridad integrales:

- **TLS/HTTPS**: Interfaces API y de gestión seguras
- **Autenticación**: Autenticación basada en claves API y tokens
- **Autorización**: Control de acceso basado en roles para operaciones
- **Limitación de tasa**: Límites configurables por IP y por llamada
- **Protección DoS**: Bloqueo automático de fuentes abusivas
- **Validación de entrada**: Validación estricta de todas las entradas del protocolo
- **Gestión de secretos**: Manejo seguro de credenciales

### REST API

Control programático sobre todas las funciones del servidor:

```bash
# Listar sesiones activas
curl http://localhost:8080/api/v1/sessions

# Obtener estadísticas del servidor
curl http://localhost:8080/api/v1/stats

# Iniciar grabación de una llamada
curl -X POST http://localhost:8080/api/v1/recording/start \
  -H "Content-Type: application/json" \
  -d '{"session_id": "abc123", "mode": "stereo"}'

# Detener grabación
curl -X POST http://localhost:8080/api/v1/recording/stop \
  -d '{"session_id": "abc123"}'
```

---

## Inicio Rápido

### Requisitos Previos

- Go 1.26 o superior
- MySQL/MariaDB (opcional, para almacenamiento de CDR)
- Redis (opcional, para caché de sesiones distribuido)

### Ejecutar en 30 Segundos

```bash
# Clonar el repositorio
git clone https://github.com/loreste/karl.git
cd karl

# Compilar y ejecutar
go build -o karl
./karl
```

Karl ahora está escuchando en:
- Puerto UDP 22222 para protocolo NG (comunicación con proxy SIP)
- Puertos UDP 30000-40000 para medios RTP
- Puerto TCP 8080 para REST API
- Puerto TCP 8086 para health checks
- Puerto TCP 9091 para métricas Prometheus

### Probar la Conexión

```bash
# Prueba de ping
echo -n "d7:command4:pinge" | nc -u localhost 22222
```

---

## Documentación

La documentación completa está disponible en el directorio [`docs/`](./docs).

### Primeros Pasos

- [Guía de Inicio Rápido](./docs/getting-started.md) - Comienza a funcionar en 5 minutos
- [Guía de Instalación](./docs/installation.md) - Opciones detalladas de instalación
- [Referencia de Configuración](./docs/configuration.md) - Todas las opciones de configuración

### Guías Prácticas

Guías paso a paso para tareas comunes:

| Guía | Descripción |
|------|-------------|
| [Desplegar en Kubernetes](./docs/how-to/deploying-kubernetes.md) | Despliegue K8s en producción con sondas, escalado y monitoreo |
| [Integrar con OpenSIPS](./docs/how-to/integrating-opensips.md) | Conectar Karl a OpenSIPS con NAT, WebRTC y grabación |
| [Integrar con Kamailio](./docs/how-to/integrating-kamailio.md) | Conectar Karl a Kamailio con ejemplos completos |
| [Configurar Grabación de Llamadas](./docs/how-to/setting-up-recording.md) | Configurar modos de grabación, almacenamiento y retención |
| [Monitorear con Prometheus](./docs/how-to/monitoring-prometheus.md) | Métricas, dashboards de Grafana y reglas de alerta |
| [Escalar Horizontalmente](./docs/how-to/scaling-horizontally.md) | Clustering con Redis y balanceo de carga |
| [Puente WebRTC a SIP](./docs/how-to/webrtc-sip-bridging.md) | Conectar clientes de navegador a infraestructura SIP |
| [Asegurar con TLS](./docs/how-to/securing-with-tls.md) | HTTPS para interfaces API y de gestión |
| [Solución de Problemas](./docs/how-to/troubleshooting.md) | Diagnosticar y resolver problemas comunes |

### Referencia

- [Referencia del Protocolo NG](./docs/reference/ng-protocol.md) - Especificación completa del protocolo
- [Variables de Entorno](./docs/reference/environment-variables.md) - Todas las variables soportadas

### Herramienta CLI

Karl incluye `karl-ctl`, una interfaz de línea de comandos para gestionar sesiones, ver estadísticas y controlar la grabación:

```bash
# Compilar el CLI
go build -o karl-ctl ./cmd/karl-ctl/

# Uso
karl-ctl ping                      # Health check
karl-ctl sessions                  # Listar sesiones activas
karl-ctl session <id>              # Detalles de sesión
karl-ctl stats                     # Estadísticas del servidor
karl-ctl delete <id>               # Eliminar una sesión
karl-ctl recording start <id>      # Iniciar grabación
karl-ctl recording stop <id>       # Detener grabación

# Flags
karl-ctl -addr http://host:8080 sessions   # Dirección API personalizada
karl-ctl -json stats                        # Salida JSON sin procesar
```

---

## Instalación

### Desde Código Fuente

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

O con mapeo de puertos (rango de puertos RTP limitado):

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

## Configuración

Karl utiliza un archivo de configuración JSON con valores predeterminados razonables. Todos los ajustes también se pueden sobreescribir mediante variables de entorno.

### Archivo de Configuración

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

### Variables de Entorno

Toda la configuración se puede establecer mediante variables de entorno:

| Variable | Descripción | Valor Predeterminado |
|----------|-------------|----------------------|
| `KARL_CONFIG_PATH` | Ruta al archivo de configuración | `config/config.json` |
| `KARL_HEALTH_PORT` | Puerto del endpoint de health check | `:8086` |
| `KARL_METRICS_PORT` | Puerto de métricas Prometheus | `:9091` |
| `KARL_API_PORT` | Puerto de REST API | `:8080` |
| `KARL_NG_PORT` | Puerto UDP del protocolo NG | `22222` |
| `KARL_RTP_MIN_PORT` | Inicio del rango de puertos RTP | `30000` |
| `KARL_RTP_MAX_PORT` | Fin del rango de puertos RTP | `40000` |
| `KARL_MAX_SESSIONS` | Sesiones concurrentes máximas | `10000` |
| `KARL_RECORDING_PATH` | Ruta de almacenamiento de grabaciones | `/var/lib/karl/recordings` |
| `KARL_RECORDING_ENABLED` | Habilitar grabación de llamadas | `true` |
| `KARL_MYSQL_DSN` | Cadena de conexión MySQL | (vacío) |
| `KARL_REDIS_ADDR` | Dirección del servidor Redis | (vacío) |
| `KARL_REDIS_ENABLED` | Habilitar caché de sesiones Redis | `false` |
| `KARL_MEDIA_IP` | Dirección IP de medios | `auto` |
| `KARL_PUBLIC_IP` | IP pública para SDP | (auto-detectada) |

---

## Despliegue en Kubernetes

Karl está diseñado para Kubernetes desde cero. Los manifiestos completos se proporcionan en el directorio `deploy/kubernetes/`.

### Despliegue Rápido

```bash
kubectl apply -k deploy/kubernetes/
```

### Sondas de Salud

Karl expone endpoints de sondas nativos de Kubernetes:

| Sonda | Endpoint | Propósito |
|-------|----------|-----------|
| Startup | `/startup` | Esperar inicialización (hasta 150s) |
| Liveness | `/live` | Detectar deadlocks, disparar reinicio |
| Readiness | `/ready` | Verificar si está listo para tráfico |

### Arquitectura de Despliegue

El despliegue predeterminado usa `hostNetwork: true` para rendimiento RTP óptimo:

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

### Escalado

Para escalado horizontal, habilita Redis para compartición de sesiones:

1. Despliega Redis (o usa Redis administrado)
2. Establece `KARL_REDIS_ENABLED=true` y `KARL_REDIS_ADDR=redis:6379`
3. Incrementa las réplicas del deployment

---

## Integración con Proxies SIP

### OpenSIPS

```opensips
# opensips.cfg
loadmodule "rtpengine.so"
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")

route {
    # ... tu lógica de enrutamiento ...

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
    # ... tu lógica de enrutamiento ...

    if (has_body("application/sdp")) {
        rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
    }
}
```

### FreeSWITCH

Usa el módulo `mod_rtpengine` con:

```xml
<param name="rtpengine-ip" value="127.0.0.1"/>
<param name="rtpengine-port" value="22222"/>
```

---

## Referencia de API

### Sesiones

**Listar todas las sesiones**
```bash
GET /api/v1/sessions
```

**Obtener detalles de sesión**
```bash
GET /api/v1/sessions/{session_id}
```

**Eliminar una sesión**
```bash
DELETE /api/v1/sessions/{session_id}
```

### Estadísticas

**Obtener estadísticas del servidor**
```bash
GET /api/v1/stats
```

Respuesta:
```json
{
  "active_sessions": 150,
  "total_sessions": 10542,
  "packets_processed": 15234567,
  "packets_dropped": 234,
  "uptime_seconds": 86400
}
```

### Grabación

**Iniciar grabación**
```bash
POST /api/v1/recording/start
Content-Type: application/json

{
  "session_id": "abc123",
  "mode": "stereo"
}
```

**Detener grabación**
```bash
POST /api/v1/recording/stop
Content-Type: application/json

{
  "session_id": "abc123"
}
```

### Salud

**Health check simple**
```bash
GET /health
```

**Estado de salud detallado**
```bash
GET /health/detail
```

---

## Monitoreo y Observabilidad

### Métricas Prometheus

Karl expone métricas completas en `:9091/metrics`:

**Métricas de Sesión**
```
karl_sessions_active          # Sesiones activas actuales
karl_sessions_total           # Total de sesiones creadas
karl_session_duration_seconds # Histograma de duración de sesión
```

**Métricas de Calidad de Medios**
```
karl_rtcp_rtt_seconds         # Tiempo de ida y vuelta RTCP
karl_rtcp_jitter_seconds      # Jitter reportado
karl_rtcp_packet_loss_ratio   # Porcentaje de pérdida de paquetes
karl_fec_recoveries_total     # Paquetes recuperados vía FEC
karl_jitter_buffer_latency    # Retardo del jitter buffer
```

**Métricas del Protocolo NG**
```
karl_ng_commands_total        # Comandos procesados por tipo
karl_ng_command_duration      # Tiempo de procesamiento de comandos
karl_ng_active_calls          # Conteo actual de llamadas
```

### Dashboard de Grafana

Importa el dashboard proporcionado desde `deploy/grafana/karl-dashboard.json` para:
- Monitoreo de sesiones en tiempo real
- Visualización de calidad de medios
- Seguimiento de utilización de recursos
- Configuración de alertas

### Endpoints de Salud

| Endpoint | Propósito | Respuesta |
|----------|-----------|-----------|
| `/health` | Estado simple | `{"status":"UP"}` |
| `/health/detail` | Estado de componentes | Desglose completo de componentes |
| `/live` | Liveness de Kubernetes | 200 si está activo |
| `/ready` | Readiness de Kubernetes | 200 si está listo para tráfico |
| `/startup` | Startup de Kubernetes | 200 cuando está inicializado |

---

## Rendimiento

Karl está optimizado para procesamiento de medios de alto rendimiento:

| Métrica | Rendimiento |
|---------|-------------|
| Creación de sesión | 4M operaciones/segundo |
| Operaciones de jitter buffer | 4.4M operaciones/segundo |
| Codificación FEC | 10.3M operaciones/segundo |
| Transcodificación G.711 | 3.3M operaciones/segundo |
| Codificación iLBC | 1.3M operaciones/segundo |
| Operaciones de buffer pool | 58M operaciones/segundo |
| Memoria por sesión | ~624 bytes |
| Sesiones concurrentes probadas | 10,000+ |

### Características de Rendimiento

- **Reenvío zero-copy**: Copias de memoria minimizadas en la ruta rápida
- **Socket sharding**: Sockets por núcleo para escalabilidad
- **Buffer pooling**: Buffers reutilizables reducen la presión del GC
- **Worker pools**: Procesamiento concurrente eficiente de paquetes
- **RTCP por lotes**: Procesamiento RTCP agregado
- **Grabación asíncrona**: Escrituras de grabación no bloqueantes

### Recomendaciones de Recursos

| Llamadas Concurrentes | CPU | Memoria |
|----------------------|-----|---------|
| < 100 | 250m | 256Mi |
| 100-500 | 500m | 512Mi |
| 500-1000 | 1000m | 1Gi |
| > 1000 | 2000m | 2Gi |

---

## Arquitectura

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

### Resumen de Componentes

- **NG Protocol Handler** (`ng_socket_listener.go`): Procesa comandos bencode de proxies SIP sobre UDP/Unix socket
- **Command Handlers** (`ng_protocol/commands/`): Offer, Answer, Delete, Query — crean sesiones, asignan puertos, construyen SDP
- **Session Manager** (`session_manager.go`): Rastrea estado de llamadas, legs, índices SSRC, limpieza por TTL
- **Media Relay** (`media_relay.go`): Reenvío RTP/RTCP por sesión entre legs de llamada con cifrado/descifrado SRTP/SRTCP, manejo de NAT (RTP simétrico, port latching, media handover, strict-source), bloqueo/silenciamiento de medios, hooks de grabación y detección de timeout de medios
- **ICE Agent** (`ice_agent.go`): Negociación ICE por sesión con recolección de candidatos STUN/TURN, verificaciones de conectividad y selección de par de candidatos vía pion/ice
- **Recording Manager** (`recording/`): Escritura de archivos WAV con modos mixed/stereo/separate, conectado al relay vía hook OnPacket
- **RTCP Handler** (`rtcp_handler.go`): Monitoreo de calidad con jitter, pérdida, RTT, cálculo de MOS (ITU-T G.107)
- **T.38 Gateway** (`t38_gateway.go`): Relay UDPTL con análisis IFP, redundancia, generación SDP y ciclo de vida de sesión de fax
- **HEP Exporter** (`hep.go`): Exportador de paquetes HEPv3 a servidores de captura Homer/SIPVicious
- **Cluster Manager** (`cluster_manager.go`): Failover de sesiones respaldado por Redis con detección de split-brain y hashing consistente
- **FEC Handler** (`fec_handler.go`): Forward error correction basado en XOR
- **Jitter Buffer** (`jitter_buffer.go`): Buffering adaptativo (20-200ms) con reordenamiento de secuencia
- **Herramienta CLI** (`cmd/karl-ctl/`): Interfaz de línea de comandos para gestión de sesiones, estadísticas, grabación y health checks

---

## Hoja de Ruta

Karl es un reemplazo completo de rtpengine. Consulta el [ROADMAP.md](./ROADMAP.md) detallado para detalles de implementación.

### Fase 1: Paridad de Plano de Control ✅ Completa
- [x] Soporte completo de flags del protocolo NG (100+ flags)
- [x] Parsing bencode, correlación de cookies, listener UDP/Unix socket
- [x] Compatibilidad de formato de respuesta

### Fase 2: Medios de Nivel Relay ✅ Completa
- [x] Relay de medios por sesión con sockets UDP por leg
- [x] Traversal de NAT: RTP simétrico, port latching, media handover, strict-source
- [x] Fingerprint DTLS real (certificado ECDSA autofirmado) y credenciales ICE aleatorias
- [x] Claves crypto SDES reales (compatible con RFC 4568)
- [x] Bloqueo de medios, silenciamiento, auto-eliminación por timeout de medios
- [x] Hook de grabación conectado a la ruta del relay de medios

### Fase 3: Características Empresariales ✅ Completa
- [x] Grabación de llamadas: salida WAV, modos mixed/stereo/separate, conectado a la ruta de medios
- [x] Transcodificación G.711 PCMU↔PCMA (tabla de búsqueda)
- [x] Transcodificación Opus/G.729/AMR vía build tags CGO (libopus, bcg729, opencore-amrnb)
- [x] Gateway SRTP↔RTP: cifrado/descifrado por leg conectado al relay de medios (rutas de claves SDES + DTLS)
- [x] Cifrado SRTCP para reenvío RTCP
- [x] T.38 fax gateway: relay UDPTL, análisis IFP, análisis/generación SDP, reenvío de sockets UDP
- [x] ICE agent completo: recolección de candidatos STUN/TURN, verificaciones de conectividad, selección de par de candidatos
- [x] Negociación de codecs (RFC 3264): intersección con ordenamiento por preferencia del oferente
- [x] Cálculo de MOS (ITU-T G.107 E-model) desde jitter/pérdida/RTT de RTCP
- [x] Selección de interfaz basada en dirección: IP de bind vs advertise, enrutamiento interno/externo
- [x] Soporte multi-stream: video y medios adicionales vía StreamPair
- [x] Cluster Redis: almacén de sesiones + pub/sub + detección de split-brain + failover
- [x] HEP/Homer: exportador HEPv3 para captura SIP/RTCP
- [x] Herramienta CLI: `karl-ctl` para gestión de sesiones, estadísticas, grabación, salud

### Fase 4: Madurez Operativa ✅ Completa
- [x] Más de 1300 tests con detección de race conditions, cero fallos
- [x] Recuperación de panic en goroutines del relay
- [x] Protección contra fugas de recursos (limpieza de puertos en error, enforcement de MaxSessions)
- [x] Seguridad: limitación de tasa, autenticación API, protección DoS, circuit breakers
- [x] Métricas Prometheus (50+), sondas de salud, logging estructurado

### Fase 5: Pruebas y Validación (En Curso)
- [x] Tests unitarios y de integración para relay de medios (flujo offer→answer→RTP→delete)
- [ ] Tests de integración con proxies SIP en vivo (OpenSIPS, Kamailio)
- [ ] Pruebas de caos, pruebas de carga sostenida

---

## Contribuir

¡Las contribuciones son bienvenidas! Consulta nuestra [Guía de Contribución](CONTRIBUTING.md) para:

- Directrices de estilo de código
- Proceso de pull requests
- Configuración del entorno de desarrollo
- Requisitos de testing

### Compilar desde Código Fuente

```bash
git clone https://github.com/loreste/karl.git
cd karl
go mod download
go build -o karl
go test ./...
```

---

## Licencia

Karl Media Server está licenciado bajo la [Licencia Pública General de GNU v3.0](LICENSE).

---

## Agradecimientos

Karl se apoya en el trabajo de la comunidad de código abierto:

- [Pion](https://github.com/pion) - Bibliotecas WebRTC, DTLS, SRTP y RTP
- [rtpengine](https://github.com/sipwise/rtpengine) - Especificación del protocolo y referencia de compatibilidad

---

## Soporte

- **Documentación**: [Documentación completa](./docs/README.md)
- **Guías Prácticas**: [Guías paso a paso](./docs/how-to)
- **Issues**: [GitHub Issues](https://github.com/loreste/karl/issues)
- **Discusiones**: [GitHub Discussions](https://github.com/loreste/karl/discussions)

---

**Karl Media Server** — Construido para ingenieros VoIP que necesitan infraestructura de medios confiable, observable y nativa en la nube.
