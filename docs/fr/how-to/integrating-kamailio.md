# Comment intégrer Karl avec Kamailio

Ce guide couvre l'intégration de Karl Media Server avec Kamailio pour le proxy RTP, la traversée NAT et la gestion média.

## Table des matières

- [Prérequis](#prérequis)
- [Intégration de base](#intégration-de-base)
- [Options de configuration](#options-de-configuration)
- [Cas d'utilisation courants](#cas-dutilisation-courants)
- [Configuration avancée](#configuration-avancée)
- [Haute disponibilité](#haute-disponibilité)
- [Dépannage](#dépannage)

---

## Prérequis

- Kamailio 5.x ou ultérieur
- Karl Media Server en cours d'exécution et accessible
- Module `rtpengine` compilé avec Kamailio

Vérifiez que le module est disponible :

```bash
kamailio -l | grep rtpengine
```

---

## Intégration de base

### Étape 1 : Charger le module

Ajoutez à votre `kamailio.cfg` :

```kamailio
# Charger le module rtpengine
loadmodule "rtpengine.so"
```

### Étape 2 : Configurer le module

```kamailio
# Pointer vers Karl Media Server
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")
```

### Étape 3 : Utiliser dans la logique de routage

```kamailio
request_route {
    # Gérer INVITE
    if (is_method("INVITE")) {
        if (has_body("application/sdp")) {
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
        t_on_reply("MANAGE_REPLY");
    }

    # Gérer BYE
    if (is_method("BYE")) {
        rtpengine_delete();
    }

    # Router la requête
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

### Étape 4 : Redémarrer Kamailio

```bash
systemctl restart kamailio
```

---

## Options de configuration

### Paramètres du module

| Paramètre | Défaut | Description |
|-----------|--------|-------------|
| `rtpengine_sock` | (aucun) | Adresse du serveur Karl |
| `rtpengine_disable_tout` | 60 | Délai de désactivation après échec (secondes) |
| `rtpengine_tout_ms` | 1000 | Délai d'expiration du socket (millisecondes) |
| `rtpengine_retr` | 5 | Nombre de tentatives |
| `queried_nodes_limit` | 0 | Nombre maximum de noeuds à interroger (0=tous) |
| `setid_default` | 0 | ID d'ensemble par défaut |

### Exemple de configuration

```kamailio
# Paramètres du module
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")
modparam("rtpengine", "rtpengine_tout_ms", 2000)
modparam("rtpengine", "rtpengine_retr", 3)
modparam("rtpengine", "rtpengine_disable_tout", 30)
```

---

## Cas d'utilisation courants

### Traversée NAT

Gérer les terminaux derrière un NAT :

```kamailio
request_route {
    if (is_method("INVITE")) {
        # Détecter le NAT
        if (nat_uac_test(64)) {
            # Forcer le relais et corriger le NAT
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection SIP-source-address");
            setbflag(FLB_NATB);
        } else {
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
    }
}
```

### Passerelle WebRTC

Faire le pont entre WebRTC et SIP :

```kamailio
request_route {
    if (is_method("INVITE")) {
        if ($proto == "ws" || $proto == "wss") {
            # Terminal WebRTC
            xlog("L_INFO", "Appel WebRTC de $fU\n");
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection ICE=remove DTLS=off SDES-off");
        } else if (has_body("application/sdp")) {
            # Terminal SIP classique
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
    }
}
```

### Enregistrement des appels

Activer l'enregistrement à la demande :

```kamailio
request_route {
    if (is_method("INVITE")) {
        # Enregistrer les appels de numéros spécifiques
        if ($fU =~ "^record") {
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection record-call");
        } else {
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
    }
}
```

### Transcodage de codecs

Forcer la négociation de codecs :

```kamailio
request_route {
    if (is_method("INVITE")) {
        # Supprimer tous les codecs et ne proposer que G.711
        rtpengine_manage("RTP/AVP replace-origin replace-session-connection codec-strip-all codec-offer-PCMU codec-offer-PCMA");
    }
}
```

### Sélection d'interface basée sur la direction (interne vs externe)

Lorsque votre serveur média dispose d'interfaces réseau internes et externes séparées (par ex., une IP privée pour votre PBX et une IP élastique pour les opérateurs), utilisez le drapeau `direction` pour indiquer à Karl quelle IP annoncer dans le SDP pour chaque leg d'appel.

**Configuration Karl :**
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

**Routage Kamailio :**
```kamailio
request_route {
    if (is_method("INVITE")) {
        if (has_body("application/sdp")) {
            if ($rd =~ "carrier\.example\.com") {
                # PBX interne → opérateur externe
                rtpengine_manage("RTP/AVP replace-origin replace-session-connection direction=internal direction=external");
            } else {
                # Interne vers interne
                rtpengine_manage("RTP/AVP replace-origin replace-session-connection direction=internal direction=internal");
            }
        }
    }
}
```

Avec cette configuration, les appels vers les opérateurs externes reçoivent `198.51.100.10` (l'IP élastique) dans leur SDP, tandis que les appels internes reçoivent `10.0.0.5`. Les sockets se lient toujours sur `10.0.0.5`.

### Application de SRTP

Forcer SRTP pour tous les appels :

```kamailio
request_route {
    if (is_method("INVITE")) {
        # Forcer SRTP
        rtpengine_manage("RTP/SAVP replace-origin replace-session-connection SDES-on");
    }
}
```

---

## Configuration avancée

### Exemple complet de kamailio.cfg

```kamailio
#!KAMAILIO

####### Paramètres globaux #########

#!define WITH_NAT
#!define WITH_RTPENGINE

debug=2
log_stderror=no
log_facility=LOG_LOCAL0
fork=yes
children=4

listen=udp:0.0.0.0:5060
listen=tcp:0.0.0.0:5060

####### Section des modules ########

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

####### Paramètres des modules ########

# Module de transaction
modparam("tm", "fr_timer", 5000)
modparam("tm", "fr_inv_timer", 30000)

# Record-Route
modparam("rr", "enable_full_lr", 1)
modparam("rr", "append_fromtag", 1)

# Aide NAT
modparam("nathelper", "natping_interval", 30)
modparam("nathelper", "ping_nated_only", 1)
modparam("nathelper", "sipping_bflag", "FLB_NATSIPPING")

# RTPEngine (Karl)
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")
modparam("rtpengine", "rtpengine_tout_ms", 2000)
modparam("rtpengine", "rtpengine_retr", 3)

####### Logique de routage ########

# Routage principal des requêtes
request_route {
    # Vérifications initiales par requête
    route(REQINIT);

    # Détection NAT
    route(NATDETECT);

    # Traitement CANCEL
    if (is_method("CANCEL")) {
        if (t_check_trans()) {
            route(RTPENGINE_DELETE);
            t_relay();
        }
        exit;
    }

    # Gérer les requêtes au sein des dialogues
    if (has_totag()) {
        route(WITHINDLG);
        exit;
    }

    # Record routing pour les requêtes initiant un dialogue
    if (is_method("INVITE|SUBSCRIBE")) {
        record_route();
    }

    # Gérer INVITE
    if (is_method("INVITE")) {
        route(RTPENGINE_OFFER);
        t_on_reply("MANAGE_REPLY");
        t_on_failure("CALL_FAILURE");
    }

    route(RELAY);
}

# Vérifications initiales par requête
route[REQINIT] {
    if (!mf_process_maxfwd_header("10")) {
        sl_send_reply("483", "Too Many Hops");
        exit;
    }

    if (!sanity_check("1511", "7")) {
        xlog("L_WARN", "Message SIP malformé de $si:$sp\n");
        exit;
    }
}

# Détection NAT
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

# Gérer les requêtes dans le dialogue
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

# Gestion de l'offre RTPEngine
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

# Suppression RTPEngine
route[RTPENGINE_DELETE] {
    rtpengine_delete();
}

# Gestion NAT
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

# Route de relais
route[RELAY] {
    if (!t_relay()) {
        sl_reply_error();
    }
    exit;
}

# Route de réponse pour gérer le SDP dans les réponses
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

# Route d'échec
failure_route[CALL_FAILURE] {
    if (t_was_cancelled()) {
        route(RTPENGINE_DELETE);
        exit;
    }

    # Gérer les autres cas d'échec selon les besoins
}
```

### Référence des drapeaux

Drapeaux courants de rtpengine_manage :

| Drapeau | Description |
|---------|-------------|
| `RTP/AVP` | Profil RTP standard |
| `RTP/SAVP` | Profil SRTP |
| `RTP/AVPF` | RTP avec retour |
| `RTP/SAVPF` | SRTP avec retour |
| `replace-origin` | Remplacer l'origine SDP |
| `replace-session-connection` | Remplacer la connexion de session |
| `SIP-source-address` | Utiliser la source SIP pour le média |
| `symmetric` | Forcer le RTP symétrique |
| `asymmetric` | Autoriser le RTP asymétrique |
| `ICE=remove` | Supprimer les attributs ICE |
| `ICE=force` | Forcer ICE |
| `DTLS=off` | Désactiver DTLS |
| `DTLS=passive` | Mode passif DTLS |
| `SDES-off` | Désactiver SDES |
| `SDES-on` | Activer SDES |
| `record-call` | Activer l'enregistrement des appels |
| `media-timeout=N` | Délai d'expiration média en secondes |
| `direction=X` | Indication d'interface (fournir deux fois : from, to) pour la sélection IP interne/externe |

---

## Haute disponibilité

### Plusieurs instances de Karl

Configurer le basculement :

```kamailio
# Serveurs principal et de secours
modparam("rtpengine", "rtpengine_sock",
    "udp:karl1.example.com:22222 udp:karl2.example.com:22222")
```

### Répartition de charge pondérée

```kamailio
# Distribution 70/30
modparam("rtpengine", "rtpengine_sock",
    "7 == udp:karl1.example.com:22222 3 == udp:karl2.example.com:22222")
```

### Ensembles multiples

```kamailio
# Définir des ensembles pour différentes régions
modparam("rtpengine", "rtpengine_sock", "1 == udp:karl-us.example.com:22222")
modparam("rtpengine", "rtpengine_sock", "2 == udp:karl-eu.example.com:22222")

# Utiliser dans le routage
request_route {
    if ($rd =~ "\.us\.") {
        set_rtpengine_set("1");
    } else {
        set_rtpengine_set("2");
    }
    rtpengine_manage("...");
}
```

### Configuration par base de données

Stocker les serveurs Karl dans la base de données :

```kamailio
modparam("rtpengine", "db_url", "mysql://kamailio:password@localhost/kamailio")
modparam("rtpengine", "table_name", "rtpengine")
```

---

## Dépannage

### Problèmes de connexion

```bash
# Tester la connectivité de Karl
echo -n "d7:command4:pinge" | nc -u 127.0.0.1 22222

# Vérifier les journaux Kamailio
tail -f /var/log/kamailio.log | grep rtpengine

# Activer le débogage
kamctl debug 4
```

### Pas d'audio

1. Vérifier la modification du SDP :
```kamailio
request_route {
    xlog("L_INFO", "SDP avant : $rb\n");
    rtpengine_manage("...");
    xlog("L_INFO", "SDP après : $rb\n");
}
```

2. Vérifier le pare-feu pour les ports UDP

3. Vérifier la création de session Karl :
```bash
curl http://127.0.0.1:8080/api/v1/sessions
```

### Audio unidirectionnel

```kamailio
# Forcer le RTP symétrique
rtpengine_manage("RTP/AVP replace-origin replace-session-connection symmetric");
```

### Surveiller les sessions

```bash
# API Karl
curl http://127.0.0.1:8080/api/v1/sessions

# RPC Kamailio
kamcmd rtpengine.show_all
```

---

## Prochaines étapes

- [Configurer l'enregistrement des appels](./setting-up-recording.md)
- [Configurer la surveillance](./monitoring-prometheus.md)
- [Pont WebRTC vers SIP](./webrtc-sip-bridging.md)
