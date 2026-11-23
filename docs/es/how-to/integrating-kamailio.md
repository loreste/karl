# Como Integrar Karl con Kamailio

Esta guia cubre la integracion del Servidor de Medios Karl con Kamailio para proxying RTP, traversal NAT y manejo de medios.

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

- Kamailio 5.x o posterior
- Servidor de Medios Karl ejecutandose y accesible
- Modulo `rtpengine` compilado con Kamailio

Verifique que el modulo esta disponible:

```bash
kamailio -l | grep rtpengine
```

---

## Integracion Basica

### Paso 1: Cargar el Modulo

Agregue a su `kamailio.cfg`:

```kamailio
# Cargar modulo rtpengine
loadmodule "rtpengine.so"
```

### Paso 2: Configurar el Modulo

```kamailio
# Apuntar al Servidor de Medios Karl
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")
```

### Paso 3: Usar en la Logica de Enrutamiento

```kamailio
request_route {
    # Manejar INVITE
    if (is_method("INVITE")) {
        if (has_body("application/sdp")) {
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
        t_on_reply("MANAGE_REPLY");
    }

    # Manejar BYE
    if (is_method("BYE")) {
        rtpengine_delete();
    }

    # Enrutar la solicitud
    route(RELAY);
}

onreply_route[MANAGE_REPLY] {
    if (has_body("application/sdp")) {
        rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
    }
}

route[RELAY] {
    if (!t_relay()) {
        sl_reply_error();
    }
    exit;
}
```

### Paso 4: Reiniciar Kamailio

```bash
systemctl restart kamailio
```

---

## Opciones de Configuracion

### Parametros del Modulo

| Parametro | Predeterminado | Descripcion |
|-----------|----------------|-------------|
| `rtpengine_sock` | (ninguno) | Direccion del servidor Karl |
| `rtpengine_disable_tout` | 60 | Timeout de deshabilitacion tras fallo (segundos) |
| `rtpengine_tout_ms` | 1000 | Timeout del socket (milisegundos) |
| `rtpengine_retr` | 5 | Numero de reintentos |
| `queried_nodes_limit` | 0 | Nodos maximos a consultar (0=todos) |
| `setid_default` | 0 | ID de conjunto predeterminado |

### Ejemplo de Configuracion

```kamailio
# Parametros del modulo
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")
modparam("rtpengine", "rtpengine_tout_ms", 2000)
modparam("rtpengine", "rtpengine_retr", 3)
modparam("rtpengine", "rtpengine_disable_tout", 30)
```

---

## Casos de Uso Comunes

### Traversal NAT

Manejar endpoints detras de NAT:

```kamailio
request_route {
    if (is_method("INVITE")) {
        # Detectar NAT
        if (nat_uac_test(64)) {
            # Forzar relay y corregir NAT
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection SIP-source-address");
            setbflag(FLB_NATB);
        } else {
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
    }
}
```

### Gateway WebRTC

Puentear WebRTC a SIP:

```kamailio
request_route {
    if (is_method("INVITE")) {
        if ($proto == "ws" || $proto == "wss") {
            # Endpoint WebRTC
            xlog("L_INFO", "Llamada WebRTC desde $fU\n");
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection ICE=remove DTLS=off SDES-off");
        } else if (has_body("application/sdp")) {
            # Endpoint SIP regular
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
    }
}
```

### Grabacion de Llamadas

Habilitar grabacion bajo demanda:

```kamailio
request_route {
    if (is_method("INVITE")) {
        # Grabar llamadas de numeros especificos
        if ($fU =~ "^record") {
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection record-call");
        } else {
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
    }
}
```

### Transcodificacion de Codecs

Forzar negociacion de codecs:

```kamailio
request_route {
    if (is_method("INVITE")) {
        # Eliminar todos los codecs y ofrecer solo G.711
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

**Enrutamiento en Kamailio:**
```kamailio
request_route {
    if (is_method("INVITE")) {
        if (has_body("application/sdp")) {
            if ($rd =~ "carrier\.example\.com") {
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

### Aplicacion de SRTP

Forzar SRTP para todas las llamadas:

```kamailio
request_route {
    if (is_method("INVITE")) {
        # Forzar SRTP
        rtpengine_manage("RTP/SAVP replace-origin replace-session-connection SDES-on");
    }
}
```

---

## Configuracion Avanzada

### Ejemplo Completo de kamailio.cfg

```kamailio
#!KAMAILIO

####### Parametros Globales #########

#!define WITH_NAT
#!define WITH_RTPENGINE

debug=2
log_stderror=no
log_facility=LOG_LOCAL0
fork=yes
children=4

listen=udp:0.0.0.0:5060
listen=tcp:0.0.0.0:5060

####### Seccion de Modulos ########

loadmodule "tm.so"
loadmodule "sl.so"
loadmodule "rr.so"
loadmodule "pv.so"
loadmodule "maxfwd.so"
loadmodule "textops.so"
loadmodule "siputils.so"
loadmodule "xlog.so"
loadmodule "sanity.so"
loadmodule "nathelper.so"
loadmodule "rtpengine.so"

####### Parametros de Modulos ########

# Modulo de transacciones
modparam("tm", "fr_timer", 5000)
modparam("tm", "fr_inv_timer", 30000)

# Record-Route
modparam("rr", "enable_full_lr", 1)
modparam("rr", "append_fromtag", 1)

# NAT helper
modparam("nathelper", "natping_interval", 30)
modparam("nathelper", "ping_nated_only", 1)
modparam("nathelper", "sipping_bflag", "FLB_NATSIPPING")

# RTPEngine (Karl)
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")
modparam("rtpengine", "rtpengine_tout_ms", 2000)
modparam("rtpengine", "rtpengine_retr", 3)

####### Logica de Enrutamiento ########

# Enrutamiento principal de solicitudes
request_route {
    # Verificaciones iniciales por solicitud
    route(REQINIT);

    # Deteccion de NAT
    route(NATDETECT);

    # Procesamiento de CANCEL
    if (is_method("CANCEL")) {
        if (t_check_trans()) {
            route(RTPENGINE_DELETE);
            t_relay();
        }
        exit;
    }

    # Manejar solicitudes dentro de dialogos
    if (has_totag()) {
        route(WITHINDLG);
        exit;
    }

    # Record routing para solicitudes que forman dialogo
    if (is_method("INVITE|SUBSCRIBE")) {
        record_route();
    }

    # Manejar INVITE
    if (is_method("INVITE")) {
        route(RTPENGINE_OFFER);
        t_on_reply("MANAGE_REPLY");
        t_on_failure("CALL_FAILURE");
    }

    route(RELAY);
}

# Verificaciones iniciales por solicitud
route[REQINIT] {
    if (!mf_process_maxfwd_header("10")) {
        sl_send_reply("483", "Too Many Hops");
        exit;
    }

    if (!sanity_check("1511", "7")) {
        xlog("L_WARN", "Mensaje SIP malformado desde $si:$sp\n");
        exit;
    }
}

# Deteccion de NAT
route[NATDETECT] {
    force_rport();
    if (nat_uac_test("19")) {
        if (is_method("REGISTER")) {
            fix_nated_register();
        } else {
            if (is_first_hop()) {
                set_contact_alias();
            }
        }
        setflag(FLT_NATS);
    }
}

# Manejar solicitudes dentro del dialogo
route[WITHINDLG] {
    if (!loose_route()) {
        if (is_method("ACK")) {
            if (!t_check_trans()) {
                exit;
            }
        }
        sl_send_reply("404", "Not Found");
        exit;
    }

    if (is_method("ACK")) {
        route(NATMANAGE);
    } else if (is_method("BYE")) {
        route(RTPENGINE_DELETE);
    } else if (is_method("INVITE")) {
        # Re-INVITE
        route(RTPENGINE_OFFER);
        t_on_reply("MANAGE_REPLY");
    }

    route(RELAY);
}

# Manejo de offer de RTPEngine
route[RTPENGINE_OFFER] {
    if (!has_body("application/sdp")) {
        return;
    }

    $var(rtpflags) = "RTP/AVP replace-origin replace-session-connection";

    if (isflagset(FLT_NATS)) {
        $var(rtpflags) = $var(rtpflags) + " SIP-source-address";
    }

    rtpengine_manage("$var(rtpflags)");
}

# Eliminacion de RTPEngine
route[RTPENGINE_DELETE] {
    rtpengine_delete();
}

# Gestion de NAT
route[NATMANAGE] {
    if (is_request()) {
        if (has_totag()) {
            if (check_route_param("nat=yes")) {
                setbflag(FLB_NATB);
            }
        }
    }

    if (isflagset(FLT_NATS) || isbflagset(FLB_NATB)) {
        if (is_request()) {
            if (!has_totag()) {
                if (t_is_branch_route()) {
                    add_rr_param(";nat=yes");
                }
            }
        }
    }
}

# Ruta de relay
route[RELAY] {
    if (!t_relay()) {
        sl_reply_error();
    }
    exit;
}

# Ruta de respuesta para gestionar SDP en respuestas
onreply_route[MANAGE_REPLY] {
    if (status =~ "[12][0-9][0-9]") {
        route(NATMANAGE);
    }

    if (has_body("application/sdp")) {
        $var(rtpflags) = "RTP/AVP replace-origin replace-session-connection";

        if (isbflagset(FLB_NATB)) {
            $var(rtpflags) = $var(rtpflags) + " SIP-source-address";
        }

        rtpengine_manage("$var(rtpflags)");
    }
}

# Ruta de fallo
failure_route[CALL_FAILURE] {
    if (t_was_cancelled()) {
        route(RTPENGINE_DELETE);
        exit;
    }

    # Manejar otros casos de fallo segun sea necesario
}
```

### Referencia de Flags

Flags comunes de rtpengine_manage:

| Flag | Descripcion |
|------|-------------|
| `RTP/AVP` | Perfil RTP estandar |
| `RTP/SAVP` | Perfil SRTP |
| `RTP/AVPF` | RTP con feedback |
| `RTP/SAVPF` | SRTP con feedback |
| `replace-origin` | Reemplazar origen SDP |
| `replace-session-connection` | Reemplazar conexion de sesion |
| `SIP-source-address` | Usar origen SIP para medios |
| `symmetric` | Forzar RTP simetrico |
| `asymmetric` | Permitir RTP asimetrico |
| `ICE=remove` | Eliminar atributos ICE |
| `ICE=force` | Forzar ICE |
| `DTLS=off` | Deshabilitar DTLS |
| `DTLS=passive` | Modo pasivo DTLS |
| `SDES-off` | Deshabilitar SDES |
| `SDES-on` | Habilitar SDES |
| `record-call` | Habilitar grabacion de llamada |
| `media-timeout=N` | Timeout de medios en segundos |
| `direction=X` | Indicacion de interfaz (proporcionar dos veces: from, to) para seleccion de IP interna/externa |

---

## Alta Disponibilidad

### Multiples Instancias de Karl

Configurar failover:

```kamailio
# Servidores primario y de respaldo
modparam("rtpengine", "rtpengine_sock",
    "udp:karl1.example.com:22222 udp:karl2.example.com:22222")
```

### Balanceo de Carga Ponderado

```kamailio
# Distribucion 70/30
modparam("rtpengine", "rtpengine_sock",
    "7 == udp:karl1.example.com:22222 3 == udp:karl2.example.com:22222")
```

### Multiples Conjuntos

```kamailio
# Definir conjuntos para diferentes regiones
modparam("rtpengine", "rtpengine_sock", "1 == udp:karl-us.example.com:22222")
modparam("rtpengine", "rtpengine_sock", "2 == udp:karl-eu.example.com:22222")

# Usar en enrutamiento
request_route {
    if ($rd =~ "\.us\.") {
        set_rtpengine_set("1");
    } else {
        set_rtpengine_set("2");
    }
    rtpengine_manage("...");
}
```

### Configuracion por Base de Datos

Almacenar servidores Karl en base de datos:

```kamailio
modparam("rtpengine", "db_url", "mysql://kamailio:password@localhost/kamailio")
modparam("rtpengine", "table_name", "rtpengine")
```

---

## Solucion de Problemas

### Problemas de Conexion

```bash
# Probar conectividad de Karl
echo -n "d7:command4:pinge" | nc -u 127.0.0.1 22222

# Verificar logs de Kamailio
tail -f /var/log/kamailio.log | grep rtpengine

# Habilitar depuracion
kamctl debug 4
```

### Sin Audio

1. Verificar modificacion de SDP:
```kamailio
request_route {
    xlog("L_INFO", "SDP antes: $rb\n");
    rtpengine_manage("...");
    xlog("L_INFO", "SDP despues: $rb\n");
}
```

2. Verificar firewall para puertos UDP

3. Verificar creacion de sesion en Karl:
```bash
curl http://127.0.0.1:8080/api/v1/sessions
```

### Audio Unidireccional

```kamailio
# Forzar RTP simetrico
rtpengine_manage("RTP/AVP replace-origin replace-session-connection symmetric");
```

### Monitorear Sesiones

```bash
# API de Karl
curl http://127.0.0.1:8080/api/v1/sessions

# RPC de Kamailio
kamcmd rtpengine.show_all
```

---

## Siguientes Pasos

- [Configurar Grabacion de Llamadas](./setting-up-recording.md)
- [Configurar Monitoreo](./monitoring-prometheus.md)
- [Puente WebRTC a SIP](./webrtc-sip-bridging.md)
