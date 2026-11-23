# Referencia de Configuracion

Karl utiliza un archivo de configuracion JSON con valores predeterminados razonables. Todas las opciones pueden ser sobreescritas mediante variables de entorno.

## Tabla de Contenidos

- [Ubicacion del Archivo de Configuracion](#ubicacion-del-archivo-de-configuracion)
- [Ejemplo Completo de Configuracion](#ejemplo-completo-de-configuracion)
- [Secciones de Configuracion](#secciones-de-configuracion)
  - [Protocolo NG](#protocolo-ng)
  - [Sesiones](#sesiones)
  - [Jitter Buffer](#jitter-buffer)
  - [RTCP](#rtcp)
  - [Correccion de Errores hacia Adelante](#correccion-de-errores-hacia-adelante)
  - [Grabacion](#grabacion)
  - [API REST](#api-rest)
  - [WebRTC](#webrtc)
  - [Integracion](#integracion)
  - [Base de Datos](#base-de-datos)
  - [SRTP](#srtp)
  - [Alertas](#alertas)
  - [HEP / Homer](#hep--homer)
  - [Cluster](#cluster)
- [Variables de Entorno](#variables-de-entorno)

---

## Ubicacion del Archivo de Configuracion

Karl busca la configuracion en el siguiente orden:

1. Ruta especificada mediante el flag `-config`: `./karl -config /path/to/config.json`
2. Ruta especificada mediante la variable de entorno `KARL_CONFIG_PATH`
3. Predeterminado: `./config/config.json`

## Actualizaciones Dinamicas de Configuracion

Karl monitorea el archivo de configuracion en busca de cambios y recarga automaticamente ciertas opciones sin requerir un reinicio.

**Opciones que se aplican inmediatamente:**
- Configuracion de WebRTC (servidores STUN/TURN, ruta de grabacion)
- Configuracion de grabacion (retencion, formato)
- Umbrales de alertas
- Configuracion de integracion (registro con proxy SIP)
- Configuracion de calidad RTP (aplicada a nuevas sesiones)

**Opciones que requieren un reinicio:**
- Configuracion de transporte (puertos y listeners UDP/TCP/TLS)
- Puerto del protocolo NG
- Rangos de puertos de sesion
- Configuracion de conexion a base de datos

Cuando se detectan cambios en la configuracion, Karl registra los cambios aplicados. Revise los logs para verificar que las actualizaciones se aplicaron correctamente.

---

## Ejemplo Completo de Configuracion

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

## Secciones de Configuracion

### Protocolo NG

Controla la interfaz del protocolo NG compatible con rtpengine.

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

| Opcion | Tipo | Predeterminado | Descripcion |
|--------|------|----------------|-------------|
| `enabled` | bool | `true` | Habilitar listener del protocolo NG |
| `socket_path` | string | `/var/run/karl/karl.sock` | Ruta del socket Unix para comunicacion local |
| `udp_port` | int | `22222` | Puerto UDP para el protocolo NG |
| `timeout` | int | `30` | Tiempo de espera de solicitud en segundos |

### Sesiones

Controla la gestion de sesiones y la asignacion de puertos RTP.

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

| Opcion | Tipo | Predeterminado | Descripcion |
|--------|------|----------------|-------------|
| `max_sessions` | int | `10000` | Sesiones concurrentes maximas |
| `session_ttl` | int | `3600` | Tiempo de vida de la sesion en segundos |
| `cleanup_interval` | int | `60` | Intervalo de limpieza de sesiones obsoletas (segundos) |
| `min_port` | int | `30000` | Numero minimo de puerto RTP |
| `max_port` | int | `40000` | Numero maximo de puerto RTP |

**Calculo del Rango de Puertos:**

Cada sesion requiere 4 puertos (RTP + RTCP para cada direccion). Con el rango predeterminado de 30000-40000:
- Puertos disponibles: 10,000
- Sesiones maximas: 2,500

Ajuste `min_port` y `max_port` segun el volumen esperado de llamadas concurrentes.

### Jitter Buffer

Controla el jitter buffer adaptativo para una reproduccion de audio fluida.

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

| Opcion | Tipo | Predeterminado | Descripcion |
|--------|------|----------------|-------------|
| `enabled` | bool | `true` | Habilitar jitter buffer |
| `min_delay` | int | `20` | Retraso minimo de buffering (ms) |
| `max_delay` | int | `200` | Retraso maximo de buffering (ms) |
| `target_delay` | int | `50` | Retraso objetivo para modo adaptativo (ms) |
| `adaptive_mode` | bool | `true` | Ajustar automaticamente el tamano del buffer segun las condiciones de la red |
| `max_size` | int | `100` | Tamano maximo del buffer en paquetes |

**Guia de Ajuste:**

- **Redes de baja latencia**: `min_delay: 10`, `target_delay: 30`
- **Redes estandar**: `min_delay: 20`, `target_delay: 50` (predeterminado)
- **Redes de alto jitter**: `min_delay: 40`, `target_delay: 100`

### RTCP

Controla RTCP (Protocolo de Control RTP) para monitoreo de calidad.

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

| Opcion | Tipo | Predeterminado | Descripcion |
|--------|------|----------------|-------------|
| `enabled` | bool | `true` | Habilitar procesamiento RTCP |
| `interval` | int | `5` | Intervalo de reportes RTCP en segundos |
| `reduced_size` | bool | `false` | Usar RTCP de tamano reducido (RFC 5506) |
| `mux_enabled` | bool | `true` | Habilitar RTCP-mux (RTP y RTCP en el mismo puerto) |

### Correccion de Errores hacia Adelante

Controla FEC para recuperacion de paquetes perdidos.

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

| Opcion | Tipo | Predeterminado | Descripcion |
|--------|------|----------------|-------------|
| `enabled` | bool | `true` | Habilitar Correccion de Errores hacia Adelante |
| `block_size` | int | `48` | Numero de paquetes por bloque FEC |
| `redundancy` | float | `0.30` | Ratio de redundancia base (0.0-1.0) |
| `adaptive_mode` | bool | `true` | Ajustar redundancia segun la perdida de paquetes |
| `max_redundancy` | float | `0.50` | Redundancia maxima al adaptar |
| `min_redundancy` | float | `0.10` | Redundancia minima al adaptar |

**Impacto de la Redundancia:**

| Redundancia | Sobrecarga de Ancho de Banda | Capacidad de Recuperacion |
|-------------|------------------------------|---------------------------|
| 0.10 (10%) | Baja | Hasta ~8% de perdida de paquetes |
| 0.30 (30%) | Media | Hasta ~20% de perdida de paquetes |
| 0.50 (50%) | Alta | Hasta ~35% de perdida de paquetes |

### Grabacion

Controla la funcionalidad de grabacion de llamadas.

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

| Opcion | Tipo | Predeterminado | Descripcion |
|--------|------|----------------|-------------|
| `enabled` | bool | `false` | Habilitar sistema de grabacion |
| `base_path` | string | `/var/lib/karl/recordings` | Directorio para archivos de grabacion |
| `format` | string | `wav` | Formato de salida: `wav` o `pcm` |
| `mode` | string | `stereo` | Modo de grabacion: `mixed`, `stereo` o `separate` |
| `sample_rate` | int | `8000` | Tasa de muestreo en Hz (8000, 16000, 48000) |
| `bits_per_sample` | int | `16` | Bits por muestra (8 o 16) |
| `max_file_size` | int | `104857600` | Tamano maximo de archivo antes de rotacion (bytes) |
| `retention_days` | int | `30` | Dias para conservar grabaciones antes de la limpieza |

**Modos de Grabacion:**

| Modo | Descripcion | Archivos Creados |
|------|-------------|------------------|
| `mixed` | Ambas partes mezcladas en mono | 1 archivo |
| `stereo` | Llamante en canal izquierdo, llamado en canal derecho | 1 archivo |
| `separate` | Cada parte en archivo separado | 2 archivos |

**Calculo de Almacenamiento:**

WAV a 8kHz/16-bit: ~1 MB por minuto por canal
- Modo mixed: ~1 MB/minuto
- Modo stereo: ~2 MB/minuto
- Modo separate: ~2 MB/minuto (2 archivos)

### API REST

Controla el servidor de API REST.

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

| Opcion | Tipo | Predeterminado | Descripcion |
|--------|------|----------------|-------------|
| `enabled` | bool | `true` | Habilitar API REST |
| `address` | string | `:8080` | Direccion y puerto de escucha |
| `auth_enabled` | bool | `false` | Requerir autenticacion con clave API |
| `rate_limit_per_min` | int | `60` | Solicitudes maximas por minuto por cliente |
| `cors_enabled` | bool | `false` | Habilitar encabezados CORS |
| `cors_origins` | string | `*` | Origenes CORS permitidos |
| `tls_enabled` | bool | `false` | Habilitar HTTPS |
| `tls_cert` | string | | Ruta al certificado TLS |
| `tls_key` | string | | Ruta a la clave privada TLS |

### WebRTC

Controla la funcionalidad WebRTC para clientes basados en navegador.

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

| Opcion | Tipo | Predeterminado | Descripcion |
|--------|------|----------------|-------------|
| `enabled` | bool | `true` | Habilitar soporte WebRTC |
| `webrtc_port` | int | `8443` | Puerto de senalizacion WebRTC |
| `stun_servers` | array | Google STUN | Servidores STUN para traversal NAT |
| `turn_servers` | array | `[]` | Servidores TURN para relay |
| `max_bitrate` | int | `2000000` | Bitrate maximo (bps) |
| `start_bitrate` | int | `1000000` | Bitrate inicial (bps) |
| `bw_estimation` | bool | `true` | Habilitar estimacion de ancho de banda |
| `tcc_enabled` | bool | `true` | Habilitar feedback Transport-CC |

### Integracion

Controla la integracion con proxies SIP y la seleccion de interfaz de red.

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

| Opcion | Tipo | Predeterminado | Descripcion |
|--------|------|----------------|-------------|
| `opensips_ip` | string | | IP del servidor OpenSIPS para registro |
| `opensips_port` | int | | Puerto del servidor OpenSIPS |
| `kamailio_ip` | string | | IP del servidor Kamailio para registro |
| `kamailio_port` | int | | Puerto del servidor Kamailio |
| `media_ip` | string | `auto` | Direccion IP para binding del socket de medios. Use `auto` para deteccion automatica |
| `public_ip` | string | | IP publica para escenarios NAT (usada en SDP cuando no se establece `advertise_addr` por interfaz) |
| `keepalive_interval` | int | `30` | Intervalo de keepalive hacia proxies SIP (segundos) |
| `interfaces` | map | | Interfaces de red nombradas para enrutamiento de medios basado en direccion (ver abajo) |

#### Interfaces Nombradas

Las interfaces nombradas permiten a Karl anunciar diferentes IPs en SDP dependiendo de la direccion a la que se dirige una llamada. Esto es esencial cuando su servidor de medios tiene rutas de red internas y externas separadas (por ejemplo, una IP privada para su PBX y una IP Elastica para carriers).

Cada entrada de interfaz tiene:

| Opcion | Tipo | Descripcion |
|--------|------|-------------|
| `name` | string | Identificador de interfaz (ej., `internal`, `external`) |
| `address` | string | IP local para binding de sockets |
| `advertise_addr` | string | IP para colocar en la linea `c=` del SDP e informacion de stream. Si esta vacia, se usa `address` |
| `port` | int | Sobreescritura opcional de puerto |

**Como la direccion selecciona la interfaz:**

El proxy SIP envia un array `direction` en la solicitud del protocolo NG (ej., `["internal", "external"]`). Karl lo usa para elegir la interfaz correcta:

- `direction[0]` = lado originante ("from")
- `direction[1]` = lado terminante ("to")

Para el **offer** (SDP del lado llamante), Karl usa `direction[0]` para seleccionar la interfaz. Para el **answer** (SDP del lado llamado), Karl usa `direction[1]`. El `advertise_addr` de la interfaz seleccionada se coloca en el SDP, mientras que `address` se usa para el binding real del socket.

**Ejemplo — los carriers obtienen la IP publica, los endpoints internos obtienen la IP privada:**

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

Luego en su proxy SIP, para llamadas hacia un carrier externo:
```
rtpengine_manage("... direction=internal direction=external");
```

Karl vinculara el socket en `10.0.0.5` pero colocara `198.51.100.10` en el SDP para el tramo dirigido al carrier.

Si no se configuran `interfaces`, Karl usa `media_ip` para binding y `public_ip` para anuncio en SDP (o `media_ip` si `public_ip` esta vacia).

### Base de Datos

Controla las conexiones de base de datos para CDR y almacenamiento de sesiones.

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

| Opcion | Tipo | Predeterminado | Descripcion |
|--------|------|----------------|-------------|
| `mysql_dsn` | string | | Cadena de conexion MySQL |
| `redis_enabled` | bool | `false` | Habilitar Redis para cache de sesiones |
| `redis_addr` | string | `localhost:6379` | Direccion del servidor Redis |
| `redis_cleanup_interval` | int | `3600` | Intervalo de limpieza de Redis (segundos) |
| `max_connections` | int | `10` | Conexiones maximas a base de datos |
| `connection_timeout` | int | `30` | Tiempo de espera de conexion (segundos) |

**Formato DSN de MySQL:**

```
user:password@tcp(host:port)/database?parseTime=true
```

Ejemplo:
```
karl:secretpassword@tcp(127.0.0.1:3306)/karl?parseTime=true
```

### SRTP

Controla la configuracion de cifrado SRTP.

```json
{
  "srtp": {
    "srtp_key": "",
    "srtp_salt": ""
  }
}
```

| Opcion | Tipo | Predeterminado | Descripcion |
|--------|------|----------------|-------------|
| `srtp_key` | string | | Clave maestra para SRTP (codificada en base64) |
| `srtp_salt` | string | | Salt maestro para SRTP (codificado en base64) |

### Alertas

Controla los umbrales de alerta de calidad.

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

| Opcion | Tipo | Predeterminado | Descripcion |
|--------|------|----------------|-------------|
| `packet_loss_threshold` | float | `0.05` | Alertar cuando la perdida de paquetes excede el 5% |
| `jitter_threshold` | float | `50.0` | Alertar cuando el jitter excede 50ms |
| `bandwidth_threshold` | int | `1000000` | Alertar cuando el ancho de banda excede el umbral |
| `notify_admin` | bool | `false` | Enviar notificaciones de alerta |
| `admin_email` | string | | Email para alertas |
| `slack_webhook` | string | | URL del webhook de Slack para alertas |

### HEP / Homer

Controla el exportador HEPv3 para enviar paquetes SIP y RTCP a un servidor de captura Homer.

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

| Opcion | Tipo | Predeterminado | Descripcion |
|--------|------|----------------|-------------|
| `hep_enabled` | bool | `false` | Habilitar exportador HEP |
| `hep_server` | string | | Direccion del servidor de captura Homer (host:port) |
| `hep_capture_id` | int | `0` | ID del agente de captura enviado en paquetes HEP |
| `hep_proto` | string | `udp` | Protocolo de transporte hacia Homer (`udp` o `tcp`) |

### Cluster

Controla el clustering multi-nodo con failover de sesiones respaldado por Redis, deteccion de split-brain y hashing consistente.

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

| Opcion | Tipo | Predeterminado | Descripcion |
|--------|------|----------------|-------------|
| `enabled` | bool | `false` | Habilitar clustering |
| `node_id` | string | | Identificador unico para este nodo |
| `node_address` | string | | Direccion de este nodo (accesible por otros nodos) |
| `quorum_size` | int | `2` | Nodos minimos para quorum |
| `replication_factor` | int | `2` | Factor de replicacion de sesiones en el anillo hash |
| `enable_fencing` | bool | `false` | Habilitar fencing de nodos al detectar particion |

**Requisitos previos**: Redis debe estar habilitado en la seccion `database`. El administrador de cluster usa Redis para almacenamiento de sesiones, coordinacion pub/sub, heartbeats y deteccion de split-brain.

**Como funciona el failover**:
1. Cada nodo envia heartbeats a Redis cada segundo
2. Si un nodo pierde 3 heartbeats, se marca como fallido
3. El detector de split-brain verifica el quorum antes de permitir la toma de control
4. Los nodos supervivientes redistribuyen las sesiones del nodo fallido mediante hashing consistente
5. Los puertos se reasignan y los relays de medios se reinician en el nuevo propietario

---

## Variables de Entorno

Todas las opciones de configuracion pueden ser sobreescritas mediante variables de entorno:

| Variable | Ruta de Configuracion | Descripcion |
|----------|----------------------|-------------|
| `KARL_CONFIG_PATH` | - | Ruta al archivo de configuracion |
| `KARL_LOG_LEVEL` | - | Nivel de logging (debug, info, warn, error) |
| `KARL_HEALTH_PORT` | - | Puerto de health check (predeterminado: `:8086`) |
| `KARL_METRICS_PORT` | - | Puerto de metricas Prometheus (predeterminado: `:9091`) |
| `KARL_API_PORT` | `api.address` | Puerto de la API REST |
| `KARL_NG_PORT` | `ng_protocol.udp_port` | Puerto UDP del protocolo NG |
| `KARL_RTP_MIN_PORT` | `sessions.min_port` | Puerto RTP minimo |
| `KARL_RTP_MAX_PORT` | `sessions.max_port` | Puerto RTP maximo |
| `KARL_MAX_SESSIONS` | `sessions.max_sessions` | Sesiones concurrentes maximas |
| `KARL_RECORDING_PATH` | `recording.base_path` | Ruta de almacenamiento de grabaciones |
| `KARL_RECORDING_ENABLED` | `recording.enabled` | Habilitar grabacion |
| `KARL_MYSQL_DSN` | `database.mysql_dsn` | Cadena de conexion MySQL |
| `KARL_REDIS_ENABLED` | `database.redis_enabled` | Habilitar Redis |
| `KARL_REDIS_ADDR` | `database.redis_addr` | Direccion de Redis |
| `KARL_MEDIA_IP` | `integration.media_ip` | Direccion IP de medios |
| `KARL_PUBLIC_IP` | `integration.public_ip` | Direccion IP publica |
| `KARL_RUN_DIR` | - | Directorio de ejecucion |

Las variables de entorno tienen prioridad sobre los valores del archivo de configuracion.

---

## Siguientes Pasos

- [Primeros Pasos](./getting-started.md)
- [Despliegue en Kubernetes](./how-to/deploying-kubernetes.md)
- [Configuracion de Monitoreo](./how-to/monitoring-prometheus.md)
