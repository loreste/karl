# Comment intégrer Karl avec OpenSIPS

Ce guide couvre l'intégration de Karl Media Server avec OpenSIPS pour le proxy RTP, la traversée NAT et la manipulation média.

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

- OpenSIPS 3.x ou ultérieur
- Karl Media Server en cours d'exécution et accessible
- Module `rtpengine` compilé avec OpenSIPS

Vérifiez que le module est disponible :

```bash
opensips -C -f /etc/opensips/opensips.cfg 2>&1 | grep rtpengine
```

---

## Intégration de base

### Étape 1 : Charger le module

Ajoutez à votre `opensips.cfg` :

```opensips
# Charger le module rtpengine
loadmodule "rtpengine.so"
```

### Étape 2 : Configurer le module

```opensips
# Pointer vers Karl Media Server
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")
```

### Étape 3 : Utiliser dans la logique de routage

```opensips
route {
    # Gérer INVITE
    if (is_method("INVITE")) {
        if (has_body("application/sdp")) {
            # Engager le proxy RTP pour l'offre
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
    }

    # Gérer BYE
    if (is_method("BYE")) {
        rtpengine_delete();
    }

    # Router la requête
    t_relay();
}

# Gérer les réponses
onreply_route[MANAGE_REPLY] {
    if (has_body("application/sdp")) {
        rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
    }
}
```

### Étape 4 : Redémarrer OpenSIPS

```bash
systemctl restart opensips
```

---

## Options de configuration

### Paramètres du module

| Paramètre | Défaut | Description |
|-----------|--------|-------------|
| `rtpengine_sock` | (aucun) | Adresse du serveur Karl |
| `rtpengine_tout` | 1 | Délai d'expiration du socket (secondes) |
| `rtpengine_retr` | 5 | Nombre de tentatives |
| `queued_msgs_threshold` | 64 | Messages en file d'attente maximum |
| `db_url` | (aucun) | Base de données pour la liste rtpengine |

### Exemple de configuration

```opensips
# Paramètres du module
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")
modparam("rtpengine", "rtpengine_tout", 2)
modparam("rtpengine", "rtpengine_retr", 3)
```

---

## Cas d'utilisation courants

### Traversée NAT

Gérer les appels où un ou les deux terminaux sont derrière un NAT :

```opensips
route {
    if (is_method("INVITE")) {
        if (has_body("application/sdp")) {
            # Détecter le NAT et appliquer les drapeaux appropriés
            if (nat_uac_test("19")) {
                # Le client est derrière un NAT
                rtpengine_manage("RTP/AVP replace-origin replace-session-connection SIP-source-address");
            } else {
                rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
            }
        }
    }
}
```

### Pont WebRTC vers SIP

Faire le pont entre les clients WebRTC et les terminaux SIP :

```opensips
route {
    if (is_method("INVITE")) {
        if ($ru =~ "transport=ws") {
            # Client WebRTC - nécessite la suppression ICE et la gestion DTLS-SRTP
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection ICE=remove DTLS=off SDES-off");
        } else {
            # Terminal SIP classique
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
    }
}
```

### Enregistrement des appels

Activer l'enregistrement pour des appels spécifiques :

```opensips
route {
    if (is_method("INVITE")) {
        # Vérifier si l'enregistrement est requis (par ex., basé sur l'appelant)
        if ($fU == "recorded_user") {
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection record-call");
        } else {
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
    }
}
```

### Filtrage de codecs

Forcer des codecs spécifiques :

```opensips
route {
    if (is_method("INVITE")) {
        # Autoriser uniquement les codecs G.711
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

**Routage OpenSIPS :**
```opensips
route {
    if (is_method("INVITE")) {
        if (has_body("application/sdp")) {
            if (is_carrier_call()) {
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

### Gestion du délai d'expiration média

Configurer les délais d'expiration de session :

```opensips
route {
    if (is_method("INVITE")) {
        # Définir le délai d'expiration média à 60 secondes
        rtpengine_manage("RTP/AVP replace-origin replace-session-connection media-timeout=60");
    }
}
```

---

## Configuration avancée

### Exemple complet de opensips.cfg

```opensips
####### Paramètres globaux #########
log_level=3
log_stderror=no
log_facility=LOG_LOCAL0

listen=udp:0.0.0.0:5060

####### Section des modules ########

# Modules principaux
loadmodule "signaling.so"
loadmodule "sl.so"
loadmodule "tm.so"
loadmodule "rr.so"
loadmodule "maxfwd.so"
loadmodule "textops.so"
loadmodule "sipmsgops.so"
loadmodule "mi_fifo.so"

# Gestion NAT
loadmodule "nathelper.so"

# Proxy RTP
loadmodule "rtpengine.so"

####### Paramètres des modules ########

# Module de transaction
modparam("tm", "fr_timeout", 5)
modparam("tm", "fr_inv_timeout", 30)

# Record-Route
modparam("rr", "enable_full_lr", 1)

# Aide NAT
modparam("nathelper", "ping_nated_only", 1)
modparam("nathelper", "sipping_bflag", "SIP_PING_FLAG")
modparam("nathelper", "natping_interval", 30)

# RTPEngine (Karl)
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")
modparam("rtpengine", "rtpengine_tout", 2)
modparam("rtpengine", "rtpengine_retr", 3)

####### Logique de routage ########

route {
    # Vérification du max forwards
    if (!mf_process_maxfwd_header(10)) {
        sl_send_reply(483, "Too Many Hops");
        exit;
    }

    # Record-Route pour les requêtes initiant un dialogue
    if (is_method("INVITE|SUBSCRIBE")) {
        record_route();
    }

    # Gérer les requêtes séquentielles
    if (has_totag()) {
        if (loose_route()) {
            if (is_method("BYE")) {
                # Supprimer la session RTP sur BYE
                rtpengine_delete();
            } else if (is_method("INVITE")) {
                # Gestion du re-INVITE
                if (has_body("application/sdp")) {
                    route(RTPENGINE);
                }
            }
            route(RELAY);
            exit;
        }
    }

    # Gérer CANCEL
    if (is_method("CANCEL")) {
        if (t_check_trans()) {
            rtpengine_delete();
            t_relay();
        }
        exit;
    }

    # Gérer l'INVITE initial
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

    # Détection NAT
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

# Route d'échec - nettoyage en cas d'échec d'appel
failure_route[CALL_FAILURE] {
    if (t_was_cancelled()) {
        rtpengine_delete();
        exit;
    }
}
```

### Référence des drapeaux

Drapeaux courants de rtpengine_manage :

| Drapeau | Description |
|---------|-------------|
| `RTP/AVP` | Utiliser le profil RTP/AVP |
| `RTP/SAVP` | Utiliser le profil RTP/SAVP (SRTP) |
| `replace-origin` | Remplacer l'origine SDP |
| `replace-session-connection` | Remplacer la connexion de niveau session |
| `SIP-source-address` | Utiliser la source SIP pour le média |
| `ICE=remove` | Supprimer les candidats ICE |
| `ICE=force` | Forcer ICE |
| `DTLS=off` | Désactiver DTLS |
| `SDES-off` | Désactiver SDES |
| `record-call` | Activer l'enregistrement |
| `media-timeout=N` | Définir le délai d'expiration média (secondes) |
| `direction=X` | Indication d'interface (fournir deux fois : from, to) pour la sélection IP interne/externe |
| `codec-strip-all` | Supprimer tous les codecs |
| `codec-offer-X` | Proposer le codec X |

---

## Haute disponibilité

### Plusieurs instances de Karl

Configurer plusieurs serveurs Karl :

```opensips
# Principal et secours
modparam("rtpengine", "rtpengine_sock",
    "udp:karl1.example.com:22222 udp:karl2.example.com:22222")
```

### Distribution pondérée

```opensips
# 70% vers le principal, 30% vers le secondaire
modparam("rtpengine", "rtpengine_sock",
    "7 == udp:karl1.example.com:22222 3 == udp:karl2.example.com:22222")
```

### Configuration par base de données

Stocker les serveurs Karl dans la base de données :

```opensips
modparam("rtpengine", "db_url", "mysql://opensips:password@localhost/opensips")
modparam("rtpengine", "db_table", "rtpengine")
```

Schéma SQL :

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

## Dépannage

### OpenSIPS ne peut pas se connecter à Karl

```bash
# Vérifier que Karl fonctionne
echo -n "d7:command4:pinge" | nc -u 127.0.0.1 22222

# Vérifier les journaux OpenSIPS
tail -f /var/log/opensips.log | grep rtpengine

# Activer la journalisation de débogage
modparam("rtpengine", "rtpengine_disable", 0)
```

### Pas d'audio dans les appels

1. Vérifier que le SDP est modifié :
```opensips
# Ajouter à votre route
xlog("L_INFO", "SDP avant : $rb\n");
rtpengine_manage("...");
xlog("L_INFO", "SDP après : $rb\n");
```

2. Vérifier que les ports RTP sont accessibles entre les terminaux

3. Vérifier les journaux de Karl pour la création de session

### Audio unidirectionnel

Causes courantes :
- NAT mal détecté
- Routage asymétrique
- Pare-feu bloquant le trafic de retour

Solution :
```opensips
# Forcer le RTP symétrique
rtpengine_manage("RTP/AVP replace-origin replace-session-connection symmetric");
```

### L'enregistrement ne fonctionne pas

1. Vérifier que l'enregistrement est activé dans la configuration de Karl
2. Vérifier les permissions du répertoire d'enregistrement
3. Confirmer que le drapeau `record-call` est envoyé :

```opensips
rtpengine_manage("RTP/AVP replace-origin replace-session-connection record-call");
```

### Surveiller les sessions actives

```bash
# Via l'API de Karl
curl http://127.0.0.1:8080/api/v1/sessions

# Via OpenSIPS MI
opensips-cli -x mi rtpengine_show_all
```

---

## Prochaines étapes

- [Configurer l'enregistrement des appels](./setting-up-recording.md)
- [Configurer la surveillance](./monitoring-prometheus.md)
- [Pont WebRTC vers SIP](./webrtc-sip-bridging.md)
