# Referencia de Variables de Entorno

Lista completa de variables de entorno soportadas por el Servidor de Medios Karl.

## Tabla de Contenidos

- [Configuracion Principal](#configuracion-principal)
- [Configuracion de Red](#configuracion-de-red)
- [Configuracion de Sesiones](#configuracion-de-sesiones)
- [Configuracion de Grabacion](#configuracion-de-grabacion)
- [Configuracion de Base de Datos](#configuracion-de-base-de-datos)
- [Configuracion de Monitoreo](#configuracion-de-monitoreo)
- [Configuracion de WebRTC](#configuracion-de-webrtc)
- [Configuracion Avanzada](#configuracion-avanzada)

---

## Configuracion Principal

| Variable | Predeterminado | Descripcion |
|----------|----------------|-------------|
| `KARL_CONFIG_PATH` | `./config/config.json` | Ruta al archivo de configuracion JSON |
| `KARL_LOG_LEVEL` | `info` | Nivel de logging: `debug`, `info`, `warn`, `error` |
| `KARL_RUN_DIR` | `./run/karl` | Directorio de ejecucion para sockets y archivos PID |
| `KARL_ENVIRONMENT` | `production` | Nombre del entorno: `production`, `staging`, `development` |

### Ejemplos

```bash
# Usar archivo de configuracion personalizado
export KARL_CONFIG_PATH=/etc/karl/config.json

# Habilitar logging de depuracion
export KARL_LOG_LEVEL=debug

# Establecer directorio de ejecucion
export KARL_RUN_DIR=/var/run/karl
```

---

## Configuracion de Red

| Variable | Predeterminado | Descripcion |
|----------|----------------|-------------|
| `KARL_NG_PORT` | `22222` | Puerto UDP para el protocolo NG |
| `KARL_API_PORT` | `:8080` | Puerto TCP para la API REST |
| `KARL_HEALTH_PORT` | `:8086` | Puerto TCP para endpoints de salud |
| `KARL_METRICS_PORT` | `:9091` | Puerto TCP para metricas de Prometheus |
| `KARL_MEDIA_IP` | `auto` | Direccion IP para medios (SDP). Use `auto` para auto-deteccion |
| `KARL_PUBLIC_IP` | (auto) | IP publica para escenarios NAT |

### Ejemplos

```bash
# Establecer puertos especificos
export KARL_NG_PORT=22223
export KARL_API_PORT=:8081
export KARL_HEALTH_PORT=:8087
export KARL_METRICS_PORT=:9092

# Establecer IP de medios explicitamente
export KARL_MEDIA_IP=192.168.1.100

# Establecer IP publica para NAT
export KARL_PUBLIC_IP=203.0.113.50
```

---

## Configuracion de Sesiones

| Variable | Predeterminado | Descripcion |
|----------|----------------|-------------|
| `KARL_RTP_MIN_PORT` | `30000` | Puerto minimo para medios RTP |
| `KARL_RTP_MAX_PORT` | `40000` | Puerto maximo para medios RTP |
| `KARL_MAX_SESSIONS` | `10000` | Sesiones concurrentes maximas |
| `KARL_SESSION_TTL` | `3600` | Timeout de sesion en segundos |
| `KARL_CLEANUP_INTERVAL` | `60` | Intervalo de limpieza de sesiones obsoletas (segundos) |

### Ejemplos

```bash
# Limitar rango de puertos RTP
export KARL_RTP_MIN_PORT=30000
export KARL_RTP_MAX_PORT=35000

# Aumentar capacidad de sesiones
export KARL_MAX_SESSIONS=20000

# Timeout de sesion mas corto
export KARL_SESSION_TTL=1800
```

### Calculo del Rango de Puertos

Cada sesion requiere 4 puertos (RTP + RTCP x 2 direcciones):
```
Sesiones maximas = (MAX_PORT - MIN_PORT + 1) / 4
```

Predeterminado: (40000 - 30000 + 1) / 4 = 2,500 sesiones

---

## Configuracion de Grabacion

| Variable | Predeterminado | Descripcion |
|----------|----------------|-------------|
| `KARL_RECORDING_ENABLED` | `false` | Habilitar/deshabilitar grabacion de llamadas |
| `KARL_RECORDING_PATH` | `/var/lib/karl/recordings` | Directorio para archivos de grabacion |
| `KARL_RECORDING_FORMAT` | `wav` | Formato de salida: `wav`, `pcm` |
| `KARL_RECORDING_MODE` | `stereo` | Modo de grabacion: `mixed`, `stereo`, `separate` |
| `KARL_RECORDING_SAMPLE_RATE` | `8000` | Tasa de muestreo en Hz |
| `KARL_RECORDING_RETENTION_DAYS` | `30` | Dias para conservar grabaciones |

### Ejemplos

```bash
# Habilitar grabacion
export KARL_RECORDING_ENABLED=true
export KARL_RECORDING_PATH=/mnt/recordings

# Configurar grabacion stereo
export KARL_RECORDING_MODE=stereo
export KARL_RECORDING_SAMPLE_RATE=16000

# Establecer retencion a 7 dias
export KARL_RECORDING_RETENTION_DAYS=7
```

---

## Configuracion de Base de Datos

| Variable | Predeterminado | Descripcion |
|----------|----------------|-------------|
| `KARL_MYSQL_DSN` | (vacio) | Cadena de conexion MySQL |
| `KARL_REDIS_ENABLED` | `false` | Habilitar Redis para compartir sesiones |
| `KARL_REDIS_ADDR` | `localhost:6379` | Direccion del servidor Redis |
| `KARL_REDIS_PASSWORD` | (vacio) | Contrasena de Redis |
| `KARL_REDIS_DB` | `0` | Numero de base de datos Redis |

### Ejemplos

```bash
# Configuracion de MySQL
export KARL_MYSQL_DSN="karl:password@tcp(mysql:3306)/karl?parseTime=true"

# Configuracion de Redis para clustering
export KARL_REDIS_ENABLED=true
export KARL_REDIS_ADDR=redis.example.com:6379
export KARL_REDIS_PASSWORD=secretpassword
export KARL_REDIS_DB=0
```

### Formato DSN de MySQL

```
user:password@tcp(host:port)/database?parseTime=true
```

---

## Configuracion de Monitoreo

| Variable | Predeterminado | Descripcion |
|----------|----------------|-------------|
| `KARL_METRICS_ENABLED` | `true` | Habilitar metricas de Prometheus |
| `KARL_HEALTH_CHECK_INTERVAL` | `30` | Intervalo de verificacion de salud (segundos) |

### Ejemplos

```bash
# Deshabilitar metricas (no recomendado)
export KARL_METRICS_ENABLED=false

# Verificaciones de salud mas frecuentes
export KARL_HEALTH_CHECK_INTERVAL=15
```

---

## Configuracion de WebRTC

| Variable | Predeterminado | Descripcion |
|----------|----------------|-------------|
| `KARL_WEBRTC_ENABLED` | `true` | Habilitar soporte WebRTC |
| `KARL_STUN_SERVERS` | `stun:stun.l.google.com:19302` | Servidores STUN (separados por coma) |
| `KARL_TURN_SERVERS` | (vacio) | Servidores TURN (formato JSON) |
| `KARL_WEBRTC_MAX_BITRATE` | `2000000` | Bitrate maximo (bps) |

### Ejemplos

```bash
# Habilitar WebRTC con STUN personalizado
export KARL_WEBRTC_ENABLED=true
export KARL_STUN_SERVERS="stun:stun1.example.com:3478,stun:stun2.example.com:3478"

# Configurar TURN (formato JSON)
export KARL_TURN_SERVERS='[{"url":"turn:turn.example.com:3478","username":"user","credential":"pass"}]'

# Limitar bitrate
export KARL_WEBRTC_MAX_BITRATE=1000000
```

---

## Configuracion Avanzada

### Jitter Buffer

| Variable | Predeterminado | Descripcion |
|----------|----------------|-------------|
| `KARL_JITTER_BUFFER_ENABLED` | `true` | Habilitar jitter buffer |
| `KARL_JITTER_BUFFER_MIN_DELAY` | `20` | Retraso minimo (ms) |
| `KARL_JITTER_BUFFER_MAX_DELAY` | `200` | Retraso maximo (ms) |
| `KARL_JITTER_BUFFER_TARGET_DELAY` | `50` | Retraso objetivo (ms) |

### Correccion de Errores hacia Adelante

| Variable | Predeterminado | Descripcion |
|----------|----------------|-------------|
| `KARL_FEC_ENABLED` | `true` | Habilitar FEC |
| `KARL_FEC_REDUNDANCY` | `0.30` | Ratio de redundancia (0.0-1.0) |
| `KARL_FEC_ADAPTIVE_MODE` | `true` | Habilitar FEC adaptativo |

### Seguridad de API

| Variable | Predeterminado | Descripcion |
|----------|----------------|-------------|
| `KARL_API_AUTH_ENABLED` | `false` | Habilitar autenticacion de API |
| `KARL_API_RATE_LIMIT` | `60` | Solicitudes por minuto por cliente |
| `KARL_API_CORS_ENABLED` | `false` | Habilitar CORS |
| `KARL_API_CORS_ORIGINS` | `*` | Origenes CORS permitidos |
| `KARL_API_TLS_ENABLED` | `false` | Habilitar HTTPS |
| `KARL_API_TLS_CERT` | (vacio) | Ruta al certificado TLS |
| `KARL_API_TLS_KEY` | (vacio) | Ruta a la clave privada TLS |

### Ejemplos

```bash
# Ajustar jitter buffer para red de alta latencia
export KARL_JITTER_BUFFER_MIN_DELAY=40
export KARL_JITTER_BUFFER_MAX_DELAY=300
export KARL_JITTER_BUFFER_TARGET_DELAY=100

# Aumentar FEC para red con perdidas
export KARL_FEC_REDUNDANCY=0.50
export KARL_FEC_ADAPTIVE_MODE=true

# Habilitar seguridad de API
export KARL_API_AUTH_ENABLED=true
export KARL_API_RATE_LIMIT=120
export KARL_API_TLS_ENABLED=true
export KARL_API_TLS_CERT=/etc/karl/certs/server.crt
export KARL_API_TLS_KEY=/etc/karl/certs/server.key
```

---

## Alertas

| Variable | Predeterminado | Descripcion |
|----------|----------------|-------------|
| `KARL_ALERT_PACKET_LOSS_THRESHOLD` | `0.05` | Alertar cuando la perdida de paquetes excede el 5% |
| `KARL_ALERT_JITTER_THRESHOLD` | `50` | Alertar cuando el jitter excede 50ms |
| `KARL_ALERT_NOTIFY_ADMIN` | `false` | Enviar notificaciones de alerta |
| `KARL_ALERT_ADMIN_EMAIL` | (vacio) | Email del administrador para alertas |
| `KARL_ALERT_SLACK_WEBHOOK` | (vacio) | URL del webhook de Slack |

---

## Orden de Prioridad

Las variables de entorno sobreescriben los valores del archivo de configuracion:

1. Variables de entorno (prioridad mas alta)
2. Archivo de configuracion
3. Valores predeterminados (prioridad mas baja)

---

## Ejemplo de ConfigMap de Kubernetes

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

## Ejemplo de Docker Compose

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

## Siguientes Pasos

- [Referencia de Configuracion](../configuration.md)
- [Guia de Instalacion](../installation.md)
- [Despliegue en Kubernetes](../how-to/deploying-kubernetes.md)
