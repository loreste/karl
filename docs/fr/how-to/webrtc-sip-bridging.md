# Comment faire le pont entre WebRTC et SIP

Ce guide couvre l'utilisation de Karl pour relier les clients WebRTC du navigateur à l'infrastructure SIP traditionnelle.

## Table des matières

- [Aperçu](#aperçu)
- [Fonctionnement](#fonctionnement)
- [Prérequis](#prérequis)
- [Configuration](#configuration)
- [Configuration du proxy SIP](#configuration-du-proxy-sip)
- [Scénarios courants](#scénarios-courants)
- [Gestion des codecs](#gestion-des-codecs)
- [Dépannage](#dépannage)

---

## Aperçu

Karl agit comme une passerelle multimédia entre WebRTC et SIP, gérant :

- **ICE/STUN/TURN** : Traversée NAT pour les clients WebRTC
- **DTLS-SRTP vers RTP** : Pont de chiffrement
- **Transcodage de codecs** : Conversion Opus vers G.711
- **Traduction SDP** : SDP compatible WebRTC vers SIP

---

## Fonctionnement

```
┌──────────────┐                    ┌──────────────┐                    ┌──────────────┐
│  Navigateur  │                    │     Karl     │                    │ Téléphone SIP│
│   (WebRTC)   │                    │              │                    │              │
└──────┬───────┘                    └──────┬───────┘                    └──────┬───────┘
       │                                   │                                   │
       │  DTLS-SRTP (Opus)                │  RTP (G.711)                      │
       │  Candidats ICE                    │  RTP simple                       │
       │◄─────────────────────────────────►│◄─────────────────────────────────►│
       │                                   │                                   │
```

### Traductions effectuées

| Côté WebRTC | Côté SIP |
|-------------|----------|
| DTLS-SRTP | RTP ou SRTP |
| Candidats ICE | IP directe |
| Codec Opus | G.711 (PCMU/PCMA) |
| Transport-CC | RTCP |
| Bundle | Lignes m séparées |

---

## Prérequis

- Serveur multimédia Karl en cours d'exécution
- Proxy SIP (OpenSIPS ou Kamailio) avec support WebSocket
- Application navigateur compatible WebRTC
- Serveur STUN/TURN (pour la traversée NAT)

---

## Configuration

### Configuration de Karl

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

### Variables d'environnement

```bash
export KARL_WEBRTC_ENABLED=true
export KARL_STUN_SERVERS="stun:stun.l.google.com:19302"
```

---

## Configuration du proxy SIP

### OpenSIPS avec WebSocket

```opensips
# Charger le module WebSocket
loadmodule "proto_ws.so"
loadmodule "proto_wss.so"

# Écouter sur WebSocket
listen=ws:0.0.0.0:80
listen=wss:0.0.0.0:443

# Module RTPEngine
loadmodule "rtpengine.so"
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")

route {
    if (is_method("INVITE")) {
        if ($proto == "ws" || $proto == "wss") {
            # WebRTC vers SIP
            # Supprimer ICE, désactiver DTLS, convertir en RTP
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection ICE=remove DTLS=off SDES-off");
        } else if ($ru =~ "transport=ws") {
            # SIP vers WebRTC (peu probable mais géré)
            rtpengine_manage("RTP/SAVPF replace-origin replace-session-connection ICE=force DTLS=passive");
        } else {
            # SIP vers SIP
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

### Kamailio avec WebSocket

```kamailio
# Charger les modules WebSocket
loadmodule "xhttp.so"
loadmodule "websocket.so"

# Écouter sur WebSocket
listen=tcp:0.0.0.0:80
listen=tls:0.0.0.0:443

# Module RTPEngine
loadmodule "rtpengine.so"
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")

request_route {
    if (is_method("INVITE")) {
        if ($proto == "ws" || $proto == "wss") {
            # Client WebRTC
            $var(rtpflags) = "RTP/AVP replace-origin replace-session-connection ICE=remove DTLS=off SDES-off";
        } else {
            # SIP standard
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

## Scénarios courants

### Navigateur WebRTC vers téléphone SIP

**Drapeaux utilisés** :
```
ICE=remove    # Supprimer les candidats ICE
DTLS=off      # Désactiver DTLS
SDES-off      # Désactiver SDES
RTP/AVP       # Utiliser RTP simple
```

### Téléphone SIP vers navigateur WebRTC

**Drapeaux utilisés** :
```
ICE=force     # Ajouter les candidats ICE
DTLS=passive  # Activer DTLS en mode passif
RTP/SAVPF     # Utiliser SRTP avec retour
```

### WebRTC vers WebRTC via proxy SIP

Les deux terminaux sont WebRTC. Karl gère ICE et DTLS des deux côtés.

---

## Gestion des codecs

### Transcodage automatique

Karl transcode automatiquement entre :

| Codec WebRTC | Codec SIP |
|--------------|-----------|
| Opus | PCMU (G.711 loi-mu) |
| Opus | PCMA (G.711 loi-A) |

### Forcer un codec spécifique

```opensips
# Supprimer tous les codecs sauf G.711
rtpengine_manage("... codec-strip-all codec-offer-PCMU codec-offer-PCMA");
```

---

## Dépannage

### Pas d'audio depuis le client WebRTC

**Diagnostic** :
```bash
# Vérifier que ICE est géré
grep -i "ICE\|DTLS" /var/log/karl.log

# Vérifier le SDP de la session
curl http://localhost:8080/api/v1/sessions | jq .
```

**Causes courantes** :
1. ICE non supprimé côté SIP
2. Échec du handshake DTLS
3. STUN/TURN non accessible

### Audio unidirectionnel

**Solution** :
```bash
# Activer le RTP symétrique
rtpengine_manage("... symmetric");
```

### Échec de négociation de codecs

**Solution** :
```opensips
# Toujours transcoder vers G.711
rtpengine_manage("... transcode-PCMU transcode-PCMA");
```

### Délai d'expiration du handshake DTLS

**Solutions** :
1. Vérifier la configuration des certificats
2. Vérifier la connectivité UDP sur les ports DTLS
3. S'assurer que le pare-feu autorise le trafic UDP

---

## Étapes suivantes

- [Intégration OpenSIPS](./integrating-opensips.md)
- [Intégration Kamailio](./integrating-kamailio.md)
- [Guide de dépannage](./troubleshooting.md)
