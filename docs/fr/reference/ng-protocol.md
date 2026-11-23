# Référence du protocole NG

Référence complète du protocole NG (Next Generation) compatible rtpengine implémenté par Karl.

## Table des matières

- [Aperçu du protocole](#aperçu-du-protocole)
- [Format des messages](#format-des-messages)
- [Commandes](#commandes)
- [Référence des drapeaux](#référence-des-drapeaux)
- [Format des réponses](#format-des-réponses)
- [Gestion des erreurs](#gestion-des-erreurs)

---

## Aperçu du protocole

Le protocole NG est un protocole basé sur UDP utilisant l'encodage bencode pour la communication entre les proxys SIP et les serveurs multimédia.

### Connexion

- **Port par défaut** : UDP 22222
- **Encodage** : Bencode
- **Requête/Réponse** : Synchrone

### Flux de base

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

## Format des messages

### Format de requête

```
<cookie> <dictionnaire-bencodé>
```

- **cookie** : Identifiant unique pour faire correspondre les réponses (chaîne)
- **dictionnaire-bencodé** : Commande et paramètres

### Exemple

```
abc123 d7:command5:offer7:call-id10:call-567898:from-tag9:tag-12345 3:sdp...e
```

### Bases du bencode

| Type | Format | Exemple |
|------|--------|---------|
| Chaîne | `<longueur>:<données>` | `4:ping` |
| Entier | `i<nombre>e` | `i42e` |
| Liste | `l<éléments>e` | `l4:spam4:eggse` |
| Dictionnaire | `d<clé><valeur>...e` | `d3:cow3:mooe` |

---

## Commandes

### ping

Commande de vérification de santé.

**Requête** :
```
d7:command4:pinge
```

**Réponse** :
```
d6:result4:ponge
```

**Utilisation** : Keepalive, surveillance de santé

---

### offer

Traiter l'offre SDP pour un appel nouveau ou existant.

**Paramètres requis** :

| Paramètre | Type | Description |
|-----------|------|-------------|
| `command` | string | `offer` |
| `call-id` | string | Identifiant unique de l'appel |
| `from-tag` | string | SIP From-tag |
| `sdp` | string | Corps de l'offre SDP |

**Paramètres optionnels** :

| Paramètre | Type | Description |
|-----------|------|-------------|
| `to-tag` | string | SIP To-tag (pour re-INVITE) |
| `flags` | list | Drapeaux de traitement |
| `replace` | list | Éléments SDP à remplacer |
| `direction` | list | Indications de sélection d'interface (voir [Sélection d'interface basée sur la direction](#sélection-dinterface-basée-sur-la-direction)) |
| `ICE` | string | Mode de gestion ICE |
| `DTLS` | string | Mode de gestion DTLS |
| `SDES` | string | Mode de gestion SDES |
| `transport-protocol` | string | Forcer le protocole de transport |
| `media-address` | string | Remplacer l'adresse multimédia |

**Réponse** :
```
d6:result2:ok3:sdpX:v=0...e
```

---

### answer

Traiter la réponse SDP pour un appel existant.

**Paramètres requis** :

| Paramètre | Type | Description |
|-----------|------|-------------|
| `command` | string | `answer` |
| `call-id` | string | Identifiant de l'appel |
| `from-tag` | string | SIP From-tag |
| `to-tag` | string | SIP To-tag |
| `sdp` | string | Corps de la réponse SDP |

**Paramètres optionnels** : Identiques à `offer`.

---

### delete

Terminer l'appel et libérer les ressources.

**Paramètres requis** :

| Paramètre | Type | Description |
|-----------|------|-------------|
| `command` | string | `delete` |
| `call-id` | string | Identifiant de l'appel |

**Paramètres optionnels** :

| Paramètre | Type | Description |
|-----------|------|-------------|
| `from-tag` | string | Supprimer un segment spécifique |
| `to-tag` | string | Supprimer un segment spécifique |
| `flags` | list | Drapeaux de traitement |

---

### query

Obtenir les statistiques en temps réel d'un appel.

**Paramètres requis** :

| Paramètre | Type | Description |
|-----------|------|-------------|
| `command` | string | `query` |
| `call-id` | string | Identifiant de l'appel |

**Champs de réponse** :

| Champ | Type | Description |
|-------|------|-------------|
| `result` | string | `ok` ou erreur |
| `totals` | dict | Statistiques agrégées |
| `tags` | dict | Statistiques par tag |
| `created` | int | Horodatage de création |
| `last-signal` | int | Horodatage de la dernière signalisation |

---

### list

Lister tous les appels actifs.

**Paramètres requis** :

| Paramètre | Type | Description |
|-----------|------|-------------|
| `command` | string | `list` |

**Paramètres optionnels** :

| Paramètre | Type | Description |
|-----------|------|-------------|
| `limit` | int | Résultats maximum |

---

### start recording

Démarrer l'enregistrement d'appel.

**Paramètres requis** :

| Paramètre | Type | Description |
|-----------|------|-------------|
| `command` | string | `start recording` |
| `call-id` | string | Identifiant de l'appel |

---

### stop recording

Arrêter l'enregistrement d'appel.

**Paramètres requis** :

| Paramètre | Type | Description |
|-----------|------|-------------|
| `command` | string | `stop recording` |
| `call-id` | string | Identifiant de l'appel |

---

### block media

Couper le son du média dans une ou les deux directions.

**Paramètres requis** :

| Paramètre | Type | Description |
|-----------|------|-------------|
| `command` | string | `block media` |
| `call-id` | string | Identifiant de l'appel |

---

### unblock media

Rétablir le son du média.

**Paramètres** : Identiques à `block media`.

---

### play DTMF

Injecter des tonalités DTMF.

**Paramètres requis** :

| Paramètre | Type | Description |
|-----------|------|-------------|
| `command` | string | `play DTMF` |
| `call-id` | string | Identifiant de l'appel |
| `dtmf` | string | Chiffres DTMF (0-9, *, #) |

**Paramètres optionnels** :

| Paramètre | Type | Description |
|-----------|------|-------------|
| `from-tag` | string | Injecter vers ce segment |
| `duration` | int | Durée de la tonalité (ms) |
| `pause` | int | Pause inter-chiffres (ms) |

---

### statistics

Obtenir les statistiques à l'échelle du serveur.

**Paramètres requis** :

| Paramètre | Type | Description |
|-----------|------|-------------|
| `command` | string | `statistics` |

**Champs de réponse** :

| Champ | Description |
|-------|-------------|
| `currentstatistics` | Compteurs actuels |
| `totalstatistics` | Compteurs sur la durée de vie |
| `sessions` | Nombre de sessions actives |
| `uptime` | Durée de fonctionnement du serveur (secondes) |

---

## Sélection d'interface basée sur la direction

Le paramètre `direction` est une liste de deux chaînes qui indique à Karl quelle interface réseau nommée utiliser pour chaque segment d'appel.

### Format

```
"direction": ["<interface-source>", "<interface-destination>"]
```

- `direction[0]` -- le nom d'interface pour le côté d'origine (utilisé pendant l'`offer`)
- `direction[1]` -- le nom d'interface pour le côté de terminaison (utilisé pendant l'`answer`)

### Exemple

```
d7:command5:offer ... 9:directionl8:internal8:externale ... e
```

Cela indique à Karl : "l'appelant est sur le réseau `internal`, l'appelé est sur le réseau `external`." Karl va :

1. Lier les sockets multimédia sur l'`address` de l'interface (l'IP locale/privée)
2. Annoncer l'`advertise_addr` de l'interface dans la ligne `c=` du SDP (par ex., l'IP élastique publique pour `external`)

### Ordre de résolution d'interface

Lors de la sélection d'une interface, Karl vérifie dans cet ordre :

1. Drapeau `interface` ou `from-interface`/`to-interface` explicite
2. Tableau `direction` (comme décrit ci-dessus)
3. Auto-détection de l'adresse du pair (privée -> `internal`, publique -> `external`)
4. Interface par défaut depuis la configuration (`media_ip` / `public_ip`)

### Exemple OpenSIPS

```opensips
# PBX interne appelant un opérateur externe
rtpengine_manage("RTP/AVP replace-origin replace-session-connection direction=internal direction=external");
```

### Exemple Kamailio

```kamailio
# PBX interne appelant un opérateur externe
rtpengine_manage("RTP/AVP replace-origin replace-session-connection direction=internal direction=external");
```

Voir [Référence de configuration -- Interfaces nommées](../configuration.md#interfaces-nommées) pour savoir comment définir les interfaces.

---

## Référence des drapeaux

### Drapeaux de protocole de transport

| Drapeau | Description |
|---------|-------------|
| `RTP/AVP` | RTP simple (profil Audio/Vidéo) |
| `RTP/AVPF` | RTP avec retour |
| `RTP/SAVP` | SRTP (Secure AVP) |
| `RTP/SAVPF` | SRTP avec retour |
| `UDP/TLS/RTP/SAVP` | DTLS-SRTP |
| `UDP/TLS/RTP/SAVPF` | DTLS-SRTP avec retour |

### Drapeaux de manipulation SDP

| Drapeau | Description |
|---------|-------------|
| `replace-origin` | Remplacer la ligne d'origine SDP |
| `replace-session-connection` | Remplacer la connexion de niveau session |
| `trust-address` | Faire confiance aux adresses SDP |
| `SIP-source-address` | Utiliser la source SIP comme adresse multimédia |
| `symmetric` | Forcer le RTP symétrique |
| `asymmetric` | Autoriser le RTP asymétrique |

### Drapeaux ICE

| Drapeau | Description |
|---------|-------------|
| `ICE=remove` | Supprimer tous les attributs ICE |
| `ICE=force` | Forcer la négociation ICE |
| `ICE=force-relay` | Forcer le relais TURN |

### Drapeaux DTLS

| Drapeau | Description |
|---------|-------------|
| `DTLS=off` | Désactiver DTLS |
| `DTLS=passive` | Rôle passif DTLS |
| `DTLS=active` | Rôle actif DTLS |

### Drapeaux SDES

| Drapeau | Description |
|---------|-------------|
| `SDES-off` | Désactiver SDES |
| `SDES-on` | Activer SDES |
| `SDES-unencrypted_srtp` | Autoriser SRTP non chiffré |
| `SDES-unencrypted_srtcp` | Autoriser SRTCP non chiffré |

### Drapeaux de codec

| Drapeau | Description |
|---------|-------------|
| `codec-strip-all` | Supprimer tous les codecs |
| `codec-offer-XXXX` | Ajouter le codec XXXX à l'offre |
| `codec-mask-XXXX` | Supprimer le codec XXXX |
| `transcode-XXXX` | Transcoder vers le codec XXXX |

### Drapeaux d'enregistrement

| Drapeau | Description |
|---------|-------------|
| `record-call` | Activer l'enregistrement d'appel |

### Drapeaux d'interface / direction

| Drapeau | Description |
|---------|-------------|
| `direction=X` | Indication de sélection d'interface (fournir deux fois : de, vers). Voir [Sélection d'interface basée sur la direction](#sélection-dinterface-basée-sur-la-direction) |
| `interface=X` | Nom d'interface explicite pour ce segment |
| `in-iface=X` / `from-interface=X` | Interface pour le côté d'origine (offer) |
| `out-iface=X` / `to-interface=X` | Interface pour le côté de terminaison (answer) |
| `media-address=X` | Remplacer entièrement l'IP multimédia (contourne la sélection d'interface) |

### Drapeaux de contrôle multimédia

| Drapeau | Description |
|---------|-------------|
| `media-timeout=N` | Délai d'expiration multimédia en secondes |
| `receive-only` | Mode réception uniquement |
| `send-only` | Mode émission uniquement |

---

## Format des réponses

### Réponse de succès

```
d6:result2:ok...e
```

### Réponse d'erreur

```
d6:result5:error12:error-reason18:Invalid parameterse
```

### Raisons d'erreur courantes

| Erreur | Description |
|--------|-------------|
| `Unknown call-id` | Session introuvable |
| `Invalid SDP` | SDP malformé |
| `Port allocation failed` | Pas de ports disponibles |
| `Codec negotiation failed` | Pas de codecs communs |
| `Session limit reached` | Nombre maximal de sessions dépassé |

---

## Gestion des erreurs

### Gestion des délais d'expiration

Si pas de réponse dans le délai :
1. Renvoyer la requête (jusqu'au nombre de tentatives configuré)
2. Marquer l'instance comme défaillante
3. Basculer vers l'instance de secours

### Erreurs d'analyse bencode

Un bencode invalide résulte en :
```
d6:result5:error12:error-reason14:Parse error: ...e
```

---

## Tests

### Tests manuels

```bash
# Test ping
echo -n "test123 d7:command4:pinge" | nc -u localhost 22222

# La réponse attendue contient "pong"
```

### Tests OpenSIPS/Kamailio

```bash
# Dans la configuration du proxy SIP, activez la journalisation de débogage pour le module rtpengine
# Puis passez un appel test et vérifiez les journaux
```

---

## Étapes suivantes

- [Référence de l'API REST](./rest-api.md)
- [Référence des métriques](./metrics.md)
- [Référence de configuration](../configuration.md)
