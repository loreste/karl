# Referencia del Protocolo NG

Referencia completa del protocolo NG (Next Generation) compatible con rtpengine implementado por Karl.

## Tabla de Contenidos

- [Resumen del Protocolo](#resumen-del-protocolo)
- [Formato de Mensaje](#formato-de-mensaje)
- [Comandos](#comandos)
- [Referencia de Flags](#referencia-de-flags)
- [Formato de Respuesta](#formato-de-respuesta)
- [Manejo de Errores](#manejo-de-errores)

---

## Resumen del Protocolo

El protocolo NG es un protocolo basado en UDP que usa codificacion bencode para la comunicacion entre proxies SIP y servidores de medios.

### Conexion

- **Puerto Predeterminado**: UDP 22222
- **Codificacion**: Bencode
- **Solicitud/Respuesta**: Sincrono

### Flujo Basico

```
Proxy SIP                    Karl
    │                          │
    │  ping                    │
    │─────────────────────────►│
    │                          │
    │  pong                    │
    │◄─────────────────────────│
    │                          │
```

---

## Formato de Mensaje

### Formato de Solicitud

```
<cookie> <diccionario-bencoded>
```

- **cookie**: Identificador unico para coincidir respuestas (string)
- **diccionario-bencoded**: Comando y parametros

### Ejemplo

```
abc123 d7:command5:offer7:call-id10:call-567898:from-tag9:tag-12345 3:sdp...e
```

### Fundamentos de Bencode

| Tipo | Formato | Ejemplo |
|------|---------|---------|
| String | `<longitud>:<datos>` | `4:ping` |
| Entero | `i<numero>e` | `i42e` |
| Lista | `l<elementos>e` | `l4:spam4:eggse` |
| Diccionario | `d<clave><valor>...e` | `d3:cow3:mooe` |

---

## Comandos

### ping

Comando de verificacion de salud.

**Solicitud**:
```
d7:command4:pinge
```

**Respuesta**:
```
d6:result4:ponge
```

**Uso**: Keepalive, monitoreo de salud

---

### offer

Procesar offer SDP para llamada nueva o existente.

**Parametros Requeridos**:

| Parametro | Tipo | Descripcion |
|-----------|------|-------------|
| `command` | string | `offer` |
| `call-id` | string | Identificador unico de llamada |
| `from-tag` | string | SIP From-tag |
| `sdp` | string | Cuerpo del offer SDP |

**Parametros Opcionales**:

| Parametro | Tipo | Descripcion |
|-----------|------|-------------|
| `to-tag` | string | SIP To-tag (para re-INVITE) |
| `flags` | list | Flags de procesamiento |
| `replace` | list | Elementos SDP a reemplazar |
| `direction` | list | Indicaciones de seleccion de interfaz (ver [Seleccion de Interfaz Basada en Direccion](#seleccion-de-interfaz-basada-en-direccion)) |
| `ICE` | string | Modo de manejo ICE |
| `DTLS` | string | Modo de manejo DTLS |
| `SDES` | string | Modo de manejo SDES |
| `transport-protocol` | string | Forzar protocolo de transporte |
| `media-address` | string | Sobreescribir direccion de medios |

**Ejemplo de Solicitud**:
```
d7:command5:offer7:call-id10:call-567898:from-tag9:tag-123453:sdpX:v=0...e
```

**Respuesta**:
```
d6:result2:ok3:sdpX:v=0...e
```

---

### answer

Procesar answer SDP para llamada existente.

**Parametros Requeridos**:

| Parametro | Tipo | Descripcion |
|-----------|------|-------------|
| `command` | string | `answer` |
| `call-id` | string | Identificador de llamada |
| `from-tag` | string | SIP From-tag |
| `to-tag` | string | SIP To-tag |
| `sdp` | string | Cuerpo del answer SDP |

**Parametros Opcionales**:

Iguales a `offer`.

**Ejemplo de Solicitud**:
```
d7:command6:answer7:call-id10:call-567898:from-tag9:tag-123456:to-tag9:tag-678903:sdpX:v=0...e
```

**Respuesta**:
```
d6:result2:ok3:sdpX:v=0...e
```

---

### delete

Terminar llamada y liberar recursos.

**Parametros Requeridos**:

| Parametro | Tipo | Descripcion |
|-----------|------|-------------|
| `command` | string | `delete` |
| `call-id` | string | Identificador de llamada |

**Parametros Opcionales**:

| Parametro | Tipo | Descripcion |
|-----------|------|-------------|
| `from-tag` | string | Eliminar tramo especifico |
| `to-tag` | string | Eliminar tramo especifico |
| `flags` | list | Flags de procesamiento |

**Ejemplo de Solicitud**:
```
d7:command6:delete7:call-id10:call-56789e
```

**Respuesta**:
```
d6:result2:oke
```

---

### query

Obtener estadisticas en tiempo real de una llamada.

**Parametros Requeridos**:

| Parametro | Tipo | Descripcion |
|-----------|------|-------------|
| `command` | string | `query` |
| `call-id` | string | Identificador de llamada |

**Campos de Respuesta**:

| Campo | Tipo | Descripcion |
|-------|------|-------------|
| `result` | string | `ok` o error |
| `totals` | dict | Estadisticas agregadas |
| `tags` | dict | Estadisticas por tag |
| `created` | int | Timestamp de creacion |
| `last-signal` | int | Timestamp de ultima senalizacion |

**Ejemplo de Respuesta**:
```
d6:result2:ok6:totalsd12:packets-sent i1000e16:packets-receivedi950e...ee
```

---

### list

Listar todas las llamadas activas.

**Parametros Requeridos**:

| Parametro | Tipo | Descripcion |
|-----------|------|-------------|
| `command` | string | `list` |

**Parametros Opcionales**:

| Parametro | Tipo | Descripcion |
|-----------|------|-------------|
| `limit` | int | Resultados maximos |

**Respuesta**:
```
d6:result2:ok5:callsl d7:call-id10:call-12345 8:from-tag9:tag-11111 6:to-tag9:tag-22222 5:state6:active e ...ee
```

---

### start recording

Iniciar grabacion de llamada.

**Parametros Requeridos**:

| Parametro | Tipo | Descripcion |
|-----------|------|-------------|
| `command` | string | `start recording` |
| `call-id` | string | Identificador de llamada |

**Parametros Opcionales**:

| Parametro | Tipo | Descripcion |
|-----------|------|-------------|
| `from-tag` | string | Grabar tramo especifico |
| `to-tag` | string | Grabar tramo especifico |

**Respuesta**:
```
d6:result2:oke
```

---

### stop recording

Finalizar grabacion de llamada.

**Parametros Requeridos**:

| Parametro | Tipo | Descripcion |
|-----------|------|-------------|
| `command` | string | `stop recording` |
| `call-id` | string | Identificador de llamada |

---

### block media

Silenciar medios en una o ambas direcciones.

**Parametros Requeridos**:

| Parametro | Tipo | Descripcion |
|-----------|------|-------------|
| `command` | string | `block media` |
| `call-id` | string | Identificador de llamada |

**Parametros Opcionales**:

| Parametro | Tipo | Descripcion |
|-----------|------|-------------|
| `from-tag` | string | Tramo especifico |
| `direction` | list | `from-tag`, `to-tag` o ambos |

---

### unblock media

Restaurar audio silenciado.

**Parametros**: Iguales a `block media`.

---

### play DTMF

Inyectar tonos DTMF.

**Parametros Requeridos**:

| Parametro | Tipo | Descripcion |
|-----------|------|-------------|
| `command` | string | `play DTMF` |
| `call-id` | string | Identificador de llamada |
| `dtmf` | string | Digitos DTMF (0-9, *, #) |

**Parametros Opcionales**:

| Parametro | Tipo | Descripcion |
|-----------|------|-------------|
| `from-tag` | string | Inyectar hacia este tramo |
| `duration` | int | Duracion del tono (ms) |
| `pause` | int | Pausa entre digitos (ms) |

---

### statistics

Obtener estadisticas a nivel de servidor.

**Parametros Requeridos**:

| Parametro | Tipo | Descripcion |
|-----------|------|-------------|
| `command` | string | `statistics` |

**Campos de Respuesta**:

| Campo | Descripcion |
|-------|-------------|
| `currentstatistics` | Contadores actuales |
| `totalstatistics` | Contadores de por vida |
| `sessions` | Conteo de sesiones activas |
| `uptime` | Tiempo de actividad del servidor (segundos) |

---

## Seleccion de Interfaz Basada en Direccion

El parametro `direction` es una lista de dos strings que indica a Karl que interfaz de red nombrada usar para cada tramo de la llamada. Asi es como los proxies SIP como OpenSIPS y Kamailio seleccionan entre direcciones de medios internas y externas.

### Formato

```
"direction": ["<interfaz-from>", "<interfaz-to>"]
```

- `direction[0]` — el nombre de interfaz para el lado originante (usado durante `offer`)
- `direction[1]` — el nombre de interfaz para el lado terminante (usado durante `answer`)

### Ejemplo

```
d7:command5:offer ... 9:directionl8:internal8:externale ... e
```

Esto le dice a Karl: "el llamante esta en la red `internal`, el llamado esta en la red `external`." Karl:

1. Vinculara los sockets de medios en el `address` de la interfaz (la IP local/privada)
2. Anunciara el `advertise_addr` de la interfaz en la linea `c=` del SDP (ej., la IP Elastica publica para `external`)

### Orden de Resolucion de Interfaz

Al seleccionar una interfaz, Karl verifica en este orden:

1. Flag explicito `interface` o `from-interface`/`to-interface`
2. Array `direction` (como se describio arriba)
3. Auto-deteccion de direccion del peer (privada → `internal`, publica → `external`)
4. Interfaz predeterminada de la configuracion (`media_ip` / `public_ip`)

### Ejemplo con OpenSIPS

```opensips
# PBX interna llamando a un carrier externo
rtpengine_manage("RTP/AVP replace-origin replace-session-connection direction=internal direction=external");
```

### Ejemplo con Kamailio

```kamailio
# PBX interna llamando a un carrier externo
rtpengine_manage("RTP/AVP replace-origin replace-session-connection direction=internal direction=external");
```

Vea [Referencia de Configuracion — Interfaces Nombradas](../configuration.md#interfaces-nombradas) para como definir interfaces.

---

## Referencia de Flags

### Flags de Protocolo de Transporte

| Flag | Descripcion |
|------|-------------|
| `RTP/AVP` | RTP plano (Perfil Audio/Video) |
| `RTP/AVPF` | RTP con feedback |
| `RTP/SAVP` | SRTP (AVP Seguro) |
| `RTP/SAVPF` | SRTP con feedback |
| `UDP/TLS/RTP/SAVP` | DTLS-SRTP |
| `UDP/TLS/RTP/SAVPF` | DTLS-SRTP con feedback |

### Flags de Manipulacion SDP

| Flag | Descripcion |
|------|-------------|
| `replace-origin` | Reemplazar linea de origen SDP |
| `replace-session-connection` | Reemplazar conexion a nivel de sesion |
| `trust-address` | Confiar en direcciones SDP |
| `SIP-source-address` | Usar origen SIP como direccion de medios |
| `symmetric` | Forzar RTP simetrico |
| `asymmetric` | Permitir RTP asimetrico |

### Flags ICE

| Flag | Descripcion |
|------|-------------|
| `ICE=remove` | Eliminar todos los atributos ICE |
| `ICE=force` | Forzar negociacion ICE |
| `ICE=force-relay` | Forzar relay TURN |

### Flags DTLS

| Flag | Descripcion |
|------|-------------|
| `DTLS=off` | Deshabilitar DTLS |
| `DTLS=passive` | Rol pasivo DTLS |
| `DTLS=active` | Rol activo DTLS |

### Flags SDES

| Flag | Descripcion |
|------|-------------|
| `SDES-off` | Deshabilitar SDES |
| `SDES-on` | Habilitar SDES |
| `SDES-unencrypted_srtp` | Permitir SRTP sin cifrar |
| `SDES-unencrypted_srtcp` | Permitir SRTCP sin cifrar |

### Flags de Codec

| Flag | Descripcion |
|------|-------------|
| `codec-strip-all` | Eliminar todos los codecs |
| `codec-offer-XXXX` | Agregar codec XXXX al offer |
| `codec-mask-XXXX` | Eliminar codec XXXX |
| `transcode-XXXX` | Transcodificar a codec XXXX |

### Flags de Grabacion

| Flag | Descripcion |
|------|-------------|
| `record-call` | Habilitar grabacion de llamada |

### Flags de Interfaz / Direccion

| Flag | Descripcion |
|------|-------------|
| `direction=X` | Indicacion de seleccion de interfaz (proporcionar dos veces: from, to). Ver [Seleccion de Interfaz Basada en Direccion](#seleccion-de-interfaz-basada-en-direccion) |
| `interface=X` | Nombre de interfaz explicito para este tramo |
| `in-iface=X` / `from-interface=X` | Interfaz para el lado originante (offer) |
| `out-iface=X` / `to-interface=X` | Interfaz para el lado terminante (answer) |
| `media-address=X` | Sobreescribir la IP de medios completamente (omite seleccion de interfaz) |

### Flags de Control de Medios

| Flag | Descripcion |
|------|-------------|
| `media-timeout=N` | Timeout de medios en segundos |
| `receive-only` | Modo solo recepcion |
| `send-only` | Modo solo envio |

---

## Formato de Respuesta

### Respuesta Exitosa

```
d6:result2:ok...e
```

### Respuesta de Error

```
d6:result5:error12:error-reason18:Invalid parameterse
```

### Razones de Error Comunes

| Error | Descripcion |
|-------|-------------|
| `Unknown call-id` | Sesion no encontrada |
| `Invalid SDP` | SDP malformado |
| `Port allocation failed` | No hay puertos disponibles |
| `Codec negotiation failed` | Sin codecs en comun |
| `Session limit reached` | Sesiones maximas excedidas |

---

## Manejo de Errores

### Manejo de Timeout

Si no hay respuesta dentro del timeout:
1. Reintentar la solicitud (hasta los reintentos configurados)
2. Marcar la instancia como fallida
3. Failover a instancia de respaldo

### Errores de Parseo Bencode

Bencode invalido resulta en:
```
d6:result5:error12:error-reason14:Parse error: ...e
```

### Errores de Estado de Sesion

Intentar operaciones en estado de sesion invalido:
```
d6:result5:error12:error-reason20:Invalid session statee
```

---

## Pruebas

### Pruebas Manuales

```bash
# Prueba de ping
echo -n "test123 d7:command4:pinge" | nc -u localhost 22222

# La respuesta esperada contiene "pong"
```

### Pruebas con OpenSIPS/Kamailio

```bash
# En la configuracion del proxy SIP, habilitar logging de depuracion para el modulo rtpengine
# Luego realizar una llamada de prueba y verificar los logs
```

---

## Siguientes Pasos

- [Referencia de la API REST](./rest-api.md)
- [Referencia de Metricas](./metrics.md)
- [Referencia de Configuracion](../configuration.md)
