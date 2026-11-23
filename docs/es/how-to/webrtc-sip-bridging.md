# Como Puentear WebRTC a SIP

Esta guia cubre el uso de Karl para puentear clientes WebRTC de navegador a infraestructura SIP tradicional.

## Tabla de Contenidos

- [Resumen](#resumen)
- [Como Funciona](#como-funciona)
- [Requisitos Previos](#requisitos-previos)
- [Configuracion](#configuracion)
- [Configuracion del Proxy SIP](#configuracion-del-proxy-sip)
- [Escenarios Comunes](#escenarios-comunes)
- [Manejo de Codecs](#manejo-de-codecs)
- [Solucion de Problemas](#solucion-de-problemas)

---

## Resumen

Karl actua como gateway de medios entre WebRTC y SIP, manejando:

- **ICE/STUN/TURN**: Traversal NAT para clientes WebRTC
- **DTLS-SRTP a RTP**: Puente de cifrado
- **Transcodificacion de codecs**: Conversion de Opus a G.711
- **Traduccion de SDP**: SDP compatible de WebRTC a SIP

---

## Como Funciona

```
┌──────────────┐                    ┌──────────────┐                    ┌──────────────┐
│   Navegador  │                    │     Karl     │                    │ Telefono SIP │
│   (WebRTC)   │                    │              │                    │              │
└──────┬───────┘                    └──────┬───────┘                    └──────┬───────┘
       │                                   │                                   │
       │  DTLS-SRTP (Opus)                │  RTP (G.711)                      │
       │  Candidatos ICE                   │  RTP plano                        │
       │◄─────────────────────────────────►│◄─────────────────────────────────►│
       │                                   │                                   │
```

### Traduccion Realizada

| Lado WebRTC | Lado SIP |
|-------------|----------|
| DTLS-SRTP | RTP o SRTP |
| Candidatos ICE | IP directa |
| Codec Opus | G.711 (PCMU/PCMA) |
| Transport-CC | RTCP |
| Bundle | m-lines separadas |

---

## Requisitos Previos

- Servidor de Medios Karl ejecutandose
- Proxy SIP (OpenSIPS o Kamailio) con soporte WebSocket
- Aplicacion de navegador compatible con WebRTC
- Servidor STUN/TURN (para traversal NAT)

---

## Configuracion

### Configuracion de Karl

```json
{
  "webrtc": {
    "enabled": true,
    "stun_servers": [
      "stun:stun.l.google.com:19302",
      "stun:stun1.l.google.com:19302"
    ],
    "turn_servers": [
      {
        "url": "turn:turn.example.com:3478",
        "username": "user",
        "credential": "password"
      }
    ],
    "max_bitrate": 2000000,
    "bw_estimation": true,
    "tcc_enabled": true
  },
  "ng_protocol": {
    "enabled": true,
    "udp_port": 22222
  }
}
```

### Variables de Entorno

```bash
export KARL_WEBRTC_ENABLED=true
export KARL_STUN_SERVERS="stun:stun.l.google.com:19302"
```

---

## Configuracion del Proxy SIP

### OpenSIPS con WebSocket

```opensips
# Cargar modulo WebSocket
loadmodule "proto_ws.so"
loadmodule "proto_wss.so"

# Escuchar en WebSocket
listen=ws:0.0.0.0:80
listen=wss:0.0.0.0:443

# Modulo RTPEngine
loadmodule "rtpengine.so"
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")

route {
    if (is_method("INVITE")) {
        if ($proto == "ws" || $proto == "wss") {
            # WebRTC a SIP
            # Eliminar ICE, deshabilitar DTLS, convertir a RTP
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection ICE=remove DTLS=off SDES-off");
        } else if ($ru =~ "transport=ws") {
            # SIP a WebRTC (poco probable pero manejarlo)
            rtpengine_manage("RTP/SAVPF replace-origin replace-session-connection ICE=force DTLS=passive");
        } else {
            # SIP a SIP
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
    }

    if (is_method("BYE")) {
        rtpengine_delete();
    }

    t_relay();
}

onreply_route[MANAGE_REPLY] {
    if (has_body("application/sdp")) {
        if ($proto == "ws" || $proto == "wss") {
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection ICE=remove DTLS=off SDES-off");
        } else {
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
    }
}
```

### Kamailio con WebSocket

```kamailio
# Cargar modulos WebSocket
loadmodule "xhttp.so"
loadmodule "websocket.so"

# Escuchar en WebSocket
listen=tcp:0.0.0.0:80
listen=tls:0.0.0.0:443

# Modulo RTPEngine
loadmodule "rtpengine.so"
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")

request_route {
    if (is_method("INVITE")) {
        if ($proto == "ws" || $proto == "wss") {
            # Cliente WebRTC
            $var(rtpflags) = "RTP/AVP replace-origin replace-session-connection ICE=remove DTLS=off SDES-off";
        } else {
            # SIP estandar
            $var(rtpflags) = "RTP/AVP replace-origin replace-session-connection";
        }

        rtpengine_manage("$var(rtpflags)");
        t_on_reply("MANAGE_REPLY");
    }

    if (is_method("BYE")) {
        rtpengine_delete();
    }

    route(RELAY);
}

onreply_route[MANAGE_REPLY] {
    if (has_body("application/sdp")) {
        if ($proto == "ws" || $proto == "wss") {
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection ICE=remove DTLS=off SDES-off");
        } else {
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
    }
}

event_route[xhttp:request] {
    if ($hdr(Upgrade) =~ "websocket") {
        if (ws_handle_handshake()) {
            exit;
        }
    }
    xhttp_reply("404", "Not Found", "", "");
}
```

---

## Escenarios Comunes

### Navegador WebRTC a Telefono SIP

El navegador inicia llamada a telefono SIP:

```
Navegador (WebRTC) ──► Proxy SIP ──► Karl ──► Telefono SIP
     │                  │           │           │
     │ INVITE (WS)      │           │           │
     │ SDP: Opus, ICE   │           │           │
     │─────────────────►│           │           │
     │                  │ INVITE    │           │
     │                  │ SDP mod   │           │
     │                  │──────────►│ INVITE    │
     │                  │           │ SDP: G.711│
     │                  │           │──────────►│
```

**Flags usados**:
```
ICE=remove    # Eliminar candidatos ICE
DTLS=off      # Deshabilitar DTLS
SDES-off      # Deshabilitar SDES
RTP/AVP       # Usar RTP plano
```

### Telefono SIP a Navegador WebRTC

Telefono SIP llama a cliente WebRTC:

```
Telefono SIP ──► Karl ──► Proxy SIP ──► Navegador (WebRTC)
     │           │          │              │
     │ INVITE    │          │              │
     │ SDP: G.711│          │              │
     │──────────►│ INVITE   │              │
     │           │ SDP mod  │ INVITE (WS)  │
     │           │─────────►│ SDP: Opus    │
     │           │          │─────────────►│
```

**Flags usados**:
```
ICE=force     # Agregar candidatos ICE
DTLS=passive  # Habilitar DTLS en modo pasivo
RTP/SAVPF     # Usar SRTP con feedback
```

### WebRTC a WebRTC via Proxy SIP

Ambos endpoints son WebRTC:

```
Navegador A ──► Proxy SIP ──► Karl ──► Proxy SIP ──► Navegador B
```

**Flags**: Karl maneja ICE y DTLS en ambos lados.

---

## Manejo de Codecs

### Transcodificacion Automatica

Karl transcodifica automaticamente entre:

| Codec WebRTC | Codec SIP |
|--------------|-----------|
| Opus | PCMU (G.711 mu-law) |
| Opus | PCMA (G.711 A-law) |

### Forzar Codec Especifico

```opensips
# Eliminar todos los codecs excepto G.711
rtpengine_manage("... codec-strip-all codec-offer-PCMU codec-offer-PCMA");
```

### Preferencia de Codec

```opensips
# Preferir PCMU sobre PCMA
rtpengine_manage("... codec-mask-PCMA");
```

---

## Traversal NAT

### Configuracion de STUN

Los servidores STUN ayudan a descubrir la IP publica:

```json
{
  "webrtc": {
    "stun_servers": [
      "stun:stun.l.google.com:19302",
      "stun:stun.example.com:3478"
    ]
  }
}
```

### Configuracion de TURN

Los servidores TURN retransmiten trafico cuando la conexion directa falla:

```json
{
  "webrtc": {
    "turn_servers": [
      {
        "url": "turn:turn.example.com:3478",
        "username": "webrtc",
        "credential": "secret123"
      },
      {
        "url": "turns:turn.example.com:5349",
        "username": "webrtc",
        "credential": "secret123"
      }
    ]
  }
}
```

### Flags de Manejo ICE

| Flag | Descripcion |
|------|-------------|
| `ICE=remove` | Eliminar todos los atributos ICE |
| `ICE=force` | Forzar negociacion ICE |
| `ICE=force-relay` | Forzar relay TURN |

---

## Consideraciones de Seguridad

### Modos SRTP

| Modo | Descripcion | Caso de Uso |
|------|-------------|-------------|
| `DTLS=off` | Deshabilitar DTLS-SRTP | WebRTC a RTP plano |
| `DTLS=passive` | Rol pasivo DTLS | SIP a WebRTC |
| `DTLS=active` | Rol activo DTLS | Karl inicia DTLS |
| `SDES-off` | Deshabilitar SDES | No ofrecer SDES |
| `SDES-on` | Habilitar SDES | Ofrecer claves SDES |

### Configuracion Recomendada

```opensips
# WebRTC a SIP (eliminar cifrado)
rtpengine_manage("RTP/AVP ... DTLS=off SDES-off");

# SIP a WebRTC (agregar cifrado)
rtpengine_manage("RTP/SAVPF ... DTLS=passive ICE=force");

# WebRTC a WebRTC (mantener cifrado)
rtpengine_manage("RTP/SAVPF ... DTLS=passive ICE=force");
```

---

## Solucion de Problemas

### Sin Audio desde Cliente WebRTC

**Diagnostico**:
```bash
# Verificar que ICE se esta manejando
grep -i "ICE\|DTLS" /var/log/karl.log

# Verificar SDP de la sesion
curl http://localhost:8080/api/v1/sessions | jq .
```

**Causas Comunes**:
1. ICE no eliminado para el lado SIP
2. Handshake DTLS fallando
3. STUN/TURN no alcanzable

**Soluciones**:
```opensips
# Asegurar eliminacion de ICE
rtpengine_manage("... ICE=remove");

# Verificar conectividad TURN
# La consola del navegador debe mostrar ICE gathering completo
```

### Audio Unidireccional

**WebRTC escucha SIP pero no al reves**:
- Handshake DTLS incompleto
- Verificacion de conectividad ICE fallando

**SIP escucha WebRTC pero no al reves**:
- NAT no manejado para el endpoint SIP
- Firewall bloqueando trafico de retorno

**Solucion**:
```bash
# Habilitar RTP simetrico
rtpengine_manage("... symmetric");
```

### Fallo de Negociacion de Codecs

**Sintomas**: Sin codec comun, la llamada falla

**Diagnostico**:
```bash
# Verificar codecs ofrecidos en logs
grep -i "codec\|offer" /var/log/karl.log
```

**Solucion**:
```opensips
# Siempre transcodificar a G.711
rtpengine_manage("... transcode-PCMU transcode-PCMA");
```

### Timeout de Handshake DTLS

**Sintomas**: Retrasos en el establecimiento de llamada, fallo eventual

**Diagnostico**:
```bash
# Verificar logs DTLS
grep -i DTLS /var/log/karl.log
```

**Soluciones**:
1. Verificar configuracion de certificados
2. Verificar conectividad UDP en puertos DTLS
3. Asegurar que el firewall permite trafico UDP

### Recoleccion ICE Lenta

**Sintomas**: Tiempo largo de establecimiento de llamada

**Soluciones**:
1. Usar servidores STUN/TURN mas cercanos
2. Reducir candidatos ICE recolectados
3. Verificar latencia de red a servidores STUN

---

## Ejemplo de Cliente WebRTC

### Ejemplo con JavaScript SIP.js

```javascript
const ua = new SIP.UA({
  uri: 'sip:user@example.com',
  transportOptions: {
    wsServers: ['wss://proxy.example.com:443']
  },
  sessionDescriptionHandlerFactoryOptions: {
    peerConnectionOptions: {
      iceServers: [
        { urls: 'stun:stun.l.google.com:19302' },
        {
          urls: 'turn:turn.example.com:3478',
          username: 'user',
          credential: 'password'
        }
      ]
    }
  }
});

// Realizar una llamada
const session = ua.invite('sip:destination@example.com', {
  sessionDescriptionHandlerOptions: {
    constraints: {
      audio: true,
      video: false
    }
  }
});
```

### Ejemplo con JsSIP

```javascript
const socket = new JsSIP.WebSocketInterface('wss://proxy.example.com:443');
const configuration = {
  sockets: [socket],
  uri: 'sip:user@example.com',
  password: 'secret'
};

const ua = new JsSIP.UA(configuration);

ua.start();

// Realizar una llamada
const eventHandlers = {
  'progress': (e) => console.log('llamada en progreso'),
  'failed': (e) => console.log('llamada fallida'),
  'ended': (e) => console.log('llamada terminada'),
  'confirmed': (e) => console.log('llamada confirmada')
};

const options = {
  eventHandlers: eventHandlers,
  mediaConstraints: { audio: true, video: false },
  pcConfig: {
    iceServers: [
      { urls: 'stun:stun.l.google.com:19302' }
    ]
  }
};

ua.call('sip:destination@example.com', options);
```

---

## Siguientes Pasos

- [Integracion con OpenSIPS](./integrating-opensips.md)
- [Integracion con Kamailio](./integrating-kamailio.md)
- [Guia de Solucion de Problemas](./troubleshooting.md)
