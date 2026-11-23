# Como Integrar Karl con OpenSIPS

Esta guia cubre la integracion del Servidor de Medios Karl con OpenSIPS para proxying RTP, traversal NAT y manipulacion de medios.

## Tabla de Contenidos

- [Requisitos Previos](#requisitos-previos)
- [Integracion Basica](#integracion-basica)
- [Opciones de Configuracion](#opciones-de-configuracion)
- [Casos de Uso Comunes](#casos-de-uso-comunes)
- [Configuracion Avanzada](#configuracion-avanzada)
- [Alta Disponibilidad](#alta-disponibilidad)
- [Solucion de Problemas](#solucion-de-problemas)

---

## Requisitos Previos

- OpenSIPS 3.x o posterior
- Servidor de Medios Karl ejecutandose y accesible
- Modulo `rtpengine` compilado con OpenSIPS

Verifique que el modulo esta disponible:

```bash
opensips -C -f /etc/opensips/opensips.cfg 2>&1 | grep rtpengine
```

---

## Integracion Basica

### Paso 1: Cargar el Modulo

Agregue a su `opensips.cfg`:

```opensips
# Cargar modulo rtpengine
loadmodule "rtpengine.so"
```

### Paso 2: Configurar el Modulo

```opensips
# Apuntar al Servidor de Medios Karl
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")
```

### Paso 3: Usar en la Logica de Enrutamiento

```opensips
route {
    # Manejar INVITE
    if (is_method("INVITE")) {
        if (has_body("application/sdp")) {
            # Activar proxy RTP para el offer
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
    }

    # Manejar BYE
    if (is_method("BYE")) {
        rtpengine_delete();
    }

    # Enrutar la solicitud
    t_relay();
}

# Manejar respuestas
onreply_route[MANAGE_REPLY] {
    if (has_body("application/sdp")) {
        rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
    }
}
```

### Paso 4: Reiniciar OpenSIPS

```bash
systemctl restart opensips
```

---

## Opciones de Configuracion

### Parametros del Modulo

| Parametro | Predeterminado | Descripcion |
|-----------|----------------|-------------|
| `rtpengine_sock` | (ninguno) | Direccion del servidor Karl |
| `rtpengine_tout` | 1 | Tiempo de espera del socket (segundos) |
| `rtpengine_retr` | 5 | Numero de reintentos |
| `queued_msgs_threshold` | 64 | Mensajes maximos en cola |
| `db_url` | (ninguno) | Base de datos para lista de rtpengine |

### Ejemplo de Configuracion

```opensips
# Parametros del modulo
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")
modparam("rtpengine", "rtpengine_tout", 2)
modparam("rtpengine", "rtpengine_retr", 3)
```

---

## Casos de Uso Comunes

### Traversal NAT

Manejar llamadas donde uno o ambos endpoints estan detras de NAT:

```opensips
route {
    if (is_method("INVITE")) {
        if (has_body("application/sdp")) {
            # Detectar NAT y aplicar flags apropiados
            if (nat_uac_test("19")) {
                # El cliente esta detras de NAT
                rtpengine_manage("RTP/AVP replace-origin replace-session-connection SIP-source-address");
            } else {
                rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
            }
        }
    }
}
```

### Puente WebRTC a SIP

Puentear clientes WebRTC a endpoints SIP:

```opensips
route {
    if (is_method("INVITE")) {
        if ($ru =~ "transport=ws") {
            # Cliente WebRTC - necesita eliminacion de ICE y manejo DTLS-SRTP
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection ICE=remove DTLS=off SDES-off");
        } else {
            # Endpoint SIP regular
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
    }
}
```

### Grabacion de Llamadas

Habilitar grabacion para llamadas especificas:

```opensips
route {
    if (is_method("INVITE")) {
        # Verificar si se requiere grabacion (ej., basado en el llamante)
        if ($fU == "recorded_user") {
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection record-call");
        } else {
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
    }
}
```

### Filtrado de Codecs

Forzar codecs especificos:

```opensips
route {
    if (is_method("INVITE")) {
        # Solo permitir codecs G.711
        rtpengine_manage("RTP/AVP replace-origin replace-session-connection codec-strip-all codec-offer-PCMU codec-offer-PCMA");
    }
}
```

### Seleccion de Interfaz Basada en Direccion (Interna vs Externa)

Cuando su servidor de medios tiene interfaces de red internas y externas separadas (ej., una IP privada para su PBX y una IP Elastica para carriers), use el flag `direction` para indicar a Karl que IP anunciar en SDP para cada tramo de llamada.

**Configuracion de Karl:**
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

**Enrutamiento en OpenSIPS:**
```opensips
route {
    if (is_method("INVITE")) {
        if (has_body("application/sdp")) {
            if (is_carrier_call()) {
                # PBX interna → carrier externo
                rtpengine_manage("RTP/AVP replace-origin replace-session-connection direction=internal direction=external");
            } else {
                # Interno a interno
                rtpengine_manage("RTP/AVP replace-origin replace-session-connection direction=internal direction=internal");
            }
        }
    }
}
```

Con esta configuracion, las llamadas hacia carriers externos obtienen `198.51.100.10` (la IP Elastica) en su SDP, mientras que las llamadas internas obtienen `10.0.0.5`. Los sockets siempre se vinculan en `10.0.0.5`.

### Manejo de Timeout de Medios

Configurar timeouts de sesion:

```opensips
route {
    if (is_method("INVITE")) {
        # Establecer timeout de medios a 60 segundos
        rtpengine_manage("RTP/AVP replace-origin replace-session-connection media-timeout=60");
    }
}
```

---

## Configuracion Avanzada

### Ejemplo Completo de opensips.cfg

```opensips
####### Parametros Globales #########
log_level=3
log_stderror=no
log_facility=LOG_LOCAL0

listen=udp:0.0.0.0:5060

####### Seccion de Modulos ########

# Modulos base
loadmodule "signaling.so"
loadmodule "sl.so"
loadmodule "tm.so"
loadmodule "rr.so"
loadmodule "maxfwd.so"
loadmodule "textops.so"
loadmodule "sipmsgops.so"
loadmodule "mi_fifo.so"

# Manejo de NAT
loadmodule "nathelper.so"

# Proxy RTP
loadmodule "rtpengine.so"

####### Parametros de Modulos ########

# Modulo de transacciones
modparam("tm", "fr_timeout", 5)
modparam("tm", "fr_inv_timeout", 30)

# Record-Route
modparam("rr", "enable_full_lr", 1)

# NAT helper
modparam("nathelper", "ping_nated_only", 1)
modparam("nathelper", "sipping_bflag", "SIP_PING_FLAG")
modparam("nathelper", "natping_interval", 30)

# RTPEngine (Karl)
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")
modparam("rtpengine", "rtpengine_tout", 2)
modparam("rtpengine", "rtpengine_retr", 3)

####### Logica de Enrutamiento ########

route {
    # Verificacion de max forwards
    if (!mf_process_maxfwd_header(10)) {
        sl_send_reply(483, "Too Many Hops");
        exit;
    }

    # Record-Route para solicitudes que forman dialogo
    if (is_method("INVITE|SUBSCRIBE")) {
        record_route();
    }

    # Manejar solicitudes secuenciales
    if (has_totag()) {
        if (loose_route()) {
            if (is_method("BYE")) {
                # Eliminar sesion RTP en BYE
                rtpengine_delete();
            } else if (is_method("INVITE")) {
                # Manejo de re-INVITE
                if (has_body("application/sdp")) {
                    route(RTPENGINE);
                }
            }
            route(RELAY);
            exit;
        }
    }

    # Manejar CANCEL
    if (is_method("CANCEL")) {
        if (t_check_trans()) {
            rtpengine_delete();
            t_relay();
        }
        exit;
    }

    # Manejar INVITE inicial
    if (is_method("INVITE")) {
        if (has_body("application/sdp")) {
            route(RTPENGINE);
        }
        t_on_reply("MANAGE_REPLY");
    }

    route(RELAY);
}

route[RTPENGINE] {
    $var(rtpflags) = "RTP/AVP replace-origin replace-session-connection";

    # Deteccion de NAT
    if (nat_uac_test("19")) {
        $var(rtpflags) = $var(rtpflags) + " SIP-source-address";
    }

    rtpengine_manage("$var(rtpflags)");
}

route[RELAY] {
    if (!t_relay()) {
        sl_reply_error();
    }
}

onreply_route[MANAGE_REPLY] {
    if (has_body("application/sdp")) {
        route(RTPENGINE);
    }
}

# Ruta de fallo - limpieza en fallo de llamada
failure_route[CALL_FAILURE] {
    if (t_was_cancelled()) {
        rtpengine_delete();
        exit;
    }
}
```

### Referencia de Flags

Flags comunes de rtpengine_manage:

| Flag | Descripcion |
|------|-------------|
| `RTP/AVP` | Usar perfil RTP/AVP |
| `RTP/SAVP` | Usar perfil RTP/SAVP (SRTP) |
| `replace-origin` | Reemplazar origen SDP |
| `replace-session-connection` | Reemplazar conexion a nivel de sesion |
| `SIP-source-address` | Usar origen SIP para medios |
| `ICE=remove` | Eliminar candidatos ICE |
| `ICE=force` | Forzar ICE |
| `DTLS=off` | Deshabilitar DTLS |
| `SDES-off` | Deshabilitar SDES |
| `record-call` | Habilitar grabacion |
| `media-timeout=N` | Establecer timeout de medios (segundos) |
| `direction=X` | Indicacion de interfaz (proporcionar dos veces: from, to) para seleccion de IP interna/externa |
| `codec-strip-all` | Eliminar todos los codecs |
| `codec-offer-X` | Ofrecer codec X |

---

## Alta Disponibilidad

### Multiples Instancias de Karl

Configurar multiples servidores Karl:

```opensips
# Primario y respaldo
modparam("rtpengine", "rtpengine_sock",
    "udp:karl1.example.com:22222 udp:karl2.example.com:22222")
```

### Distribucion Ponderada

```opensips
# 70% al primario, 30% al secundario
modparam("rtpengine", "rtpengine_sock",
    "7 == udp:karl1.example.com:22222 3 == udp:karl2.example.com:22222")
```

### Configuracion Respaldada por Base de Datos

Almacenar servidores Karl en base de datos:

```opensips
modparam("rtpengine", "db_url", "mysql://opensips:password@localhost/opensips")
modparam("rtpengine", "db_table", "rtpengine")
```

Esquema SQL:

```sql
CREATE TABLE rtpengine (
    id INT PRIMARY KEY AUTO_INCREMENT,
    socket VARCHAR(128) NOT NULL,
    set_id INT DEFAULT 0,
    weight INT DEFAULT 1,
    disabled INT DEFAULT 0
);

INSERT INTO rtpengine (socket, set_id, weight) VALUES
    ('udp:karl1.example.com:22222', 0, 10),
    ('udp:karl2.example.com:22222', 0, 10);
```

---

## Solucion de Problemas

### OpenSIPS No Puede Conectar a Karl

```bash
# Verificar que Karl esta ejecutandose
echo -n "d7:command4:pinge" | nc -u 127.0.0.1 22222

# Verificar logs de OpenSIPS
tail -f /var/log/opensips.log | grep rtpengine

# Habilitar logging de depuracion
modparam("rtpengine", "rtpengine_disable", 0)
```

### Sin Audio en las Llamadas

1. Verificar que el SDP esta siendo modificado:
```opensips
# Agregar a su ruta
xlog("L_INFO", "SDP antes: $rb\n");
rtpengine_manage("...");
xlog("L_INFO", "SDP despues: $rb\n");
```

2. Verificar que los puertos RTP son alcanzables entre endpoints

3. Verificar logs de Karl para la creacion de sesiones

### Audio Unidireccional

Causas comunes:
- NAT no detectado correctamente
- Enrutamiento asimetrico
- Firewall bloqueando trafico de retorno

Solucion:
```opensips
# Forzar RTP simetrico
rtpengine_manage("RTP/AVP replace-origin replace-session-connection symmetric");
```

### La Grabacion No Funciona

1. Verificar que la grabacion esta habilitada en la configuracion de Karl
2. Verificar permisos del directorio de grabacion
3. Confirmar que el flag `record-call` se esta enviando:

```opensips
rtpengine_manage("RTP/AVP replace-origin replace-session-connection record-call");
```

### Monitorear Sesiones Activas

```bash
# Via API de Karl
curl http://127.0.0.1:8080/api/v1/sessions

# Via MI de OpenSIPS
opensips-cli -x mi rtpengine_show_all
```

---

## Siguientes Pasos

- [Configurar Grabacion de Llamadas](./setting-up-recording.md)
- [Configurar Monitoreo](./monitoring-prometheus.md)
- [Puente WebRTC a SIP](./webrtc-sip-bridging.md)
