# Comment configurer l'enregistrement des appels

Ce guide couvre la configuration et la gestion de l'enregistrement des appels dans le serveur multimédia Karl.

## Table des matières

- [Aperçu](#aperçu)
- [Activer l'enregistrement](#activer-lenregistrement)
- [Modes d'enregistrement](#modes-denregistrement)
- [Configuration du stockage](#configuration-du-stockage)
- [Déclencher l'enregistrement](#déclencher-lenregistrement)
- [Gérer les enregistrements](#gérer-les-enregistrements)
- [Considérations de production](#considérations-de-production)
- [Dépannage](#dépannage)

---

## Aperçu

Karl prend en charge l'enregistrement d'appels de qualité professionnelle avec :

- Plusieurs modes d'enregistrement (mixed, stereo, separate)
- Format de sortie WAV (PCM 16 bits)
- Transcodage automatique des codecs
- Gestion de la politique de rétention
- API REST pour le contrôle de l'enregistrement

---

## Activer l'enregistrement

### Fichier de configuration

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

### Variables d'environnement

```bash
export KARL_RECORDING_ENABLED=true
export KARL_RECORDING_PATH=/var/lib/karl/recordings
```

### Vérifier la configuration

```bash
# Vérifier que l'enregistrement est activé
curl http://localhost:8080/api/v1/stats | jq .recording_enabled
```

---

## Modes d'enregistrement

### Mode Mixed

Les deux parties combinées dans un seul fichier mono.

```json
{
  "recording": {
    "mode": "mixed"
  }
}
```

**Sortie** : Fichier WAV mono unique
**Cas d'utilisation** : Archivage simple, conformité

### Mode Stereo

Appelant sur le canal gauche, appelé sur le canal droit.

```json
{
  "recording": {
    "mode": "stereo"
  }
}
```

**Sortie** : Fichier WAV stéréo unique
**Cas d'utilisation** : Analyse de qualité, identification du locuteur

### Mode Separate

Chaque partie dans un fichier séparé.

```json
{
  "recording": {
    "mode": "separate"
  }
}
```

**Sortie** : Deux fichiers WAV mono (un par partie)
**Cas d'utilisation** : Post-traitement, services de transcription

---

## Configuration du stockage

### Structure des répertoires

Les enregistrements sont stockés avec cette structure :

```
/var/lib/karl/recordings/
├── 2024-01-15/
│   ├── call-abc123_1705312200_stereo.wav
│   ├── call-def456_1705312500_stereo.wav
│   └── call-ghi789_1705313000_stereo.wav
└── 2024-01-16/
    └── ...
```

### Nommage des fichiers

Format : `{call_id}_{unix_timestamp}_{mode}.wav`

Exemple : `call-abc123_1705312200_stereo.wav`

### Besoins en stockage

| Mode | Débit | Par minute | Par heure |
|------|-------|------------|-----------|
| Mixed (mono) | 128 kbps | ~1 Mo | ~60 Mo |
| Stereo | 256 kbps | ~2 Mo | ~120 Mo |
| Separate | 256 kbps | ~2 Mo | ~120 Mo |

### Politique de rétention

Configurer le nettoyage automatique :

```json
{
  "recording": {
    "retention_days": 30
  }
}
```

Les enregistrements plus anciens que `retention_days` sont automatiquement supprimés.

### Stockage réseau

Pour les déploiements haute disponibilité, montez un stockage réseau :

```bash
# Montage NFS
mount -t nfs storage.example.com:/recordings /var/lib/karl/recordings

# Ou dans /etc/fstab
storage.example.com:/recordings /var/lib/karl/recordings nfs defaults 0 0
```

Pour Kubernetes, utilisez un PersistentVolumeClaim :

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: karl-recordings
spec:
  accessModes:
    - ReadWriteMany
  resources:
    requests:
      storage: 100Gi
  storageClassName: nfs
```

---

## Déclencher l'enregistrement

### Méthode 1 : Drapeau du protocole NG

Ajoutez le drapeau `record-call` lors de la création d'une session.

**OpenSIPS** :
```opensips
rtpengine_manage("RTP/AVP replace-origin replace-session-connection record-call");
```

**Kamailio** :
```kamailio
rtpengine_manage("RTP/AVP replace-origin replace-session-connection record-call");
```

### Méthode 2 : API REST

Démarrer l'enregistrement pour une session active :

```bash
# Démarrer l'enregistrement
curl -X POST http://localhost:8080/api/v1/recording/start \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "abc123",
    "mode": "stereo"
  }'
```

Arrêter l'enregistrement :

```bash
# Arrêter l'enregistrement
curl -X POST http://localhost:8080/api/v1/recording/stop \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "abc123"
  }'
```

### Méthode 3 : Enregistrer tous les appels

Configurer l'enregistrement par défaut dans votre proxy SIP :

**OpenSIPS** (enregistrer tous les appels) :
```opensips
route {
    if (is_method("INVITE")) {
        rtpengine_manage("RTP/AVP replace-origin replace-session-connection record-call");
    }
}
```

### Méthode 4 : Enregistrement conditionnel

Enregistrer selon des critères :

```opensips
route {
    if (is_method("INVITE")) {
        $var(flags) = "RTP/AVP replace-origin replace-session-connection";

        # Enregistrer les appels d'utilisateurs spécifiques
        if ($fU =~ "^sales_") {
            $var(flags) = $var(flags) + " record-call";
        }

        # Enregistrer les appels vers des numéros spécifiques
        if ($rU =~ "^1800") {
            $var(flags) = $var(flags) + " record-call";
        }

        rtpengine_manage("$var(flags)");
    }
}
```

---

## Gérer les enregistrements

### Lister les enregistrements

```bash
# Via l'API
curl http://localhost:8080/api/v1/recordings

# Réponse
{
  "recordings": [
    {
      "id": "rec-001",
      "session_id": "abc123",
      "call_id": "call-abc123",
      "file_path": "/var/lib/karl/recordings/2024-01-15/call-abc123_1705312200_stereo.wav",
      "mode": "stereo",
      "duration_seconds": 125,
      "file_size": 3932160,
      "created_at": "2024-01-15T10:30:00Z"
    }
  ]
}
```

### Obtenir les détails d'un enregistrement

```bash
curl http://localhost:8080/api/v1/recordings/rec-001
```

### Télécharger un enregistrement

```bash
# Obtenir le chemin du fichier via l'API, puis télécharger
curl http://localhost:8080/api/v1/recordings/rec-001/download -o recording.wav
```

### Supprimer un enregistrement

```bash
curl -X DELETE http://localhost:8080/api/v1/recordings/rec-001
```

### Lister les enregistrements par date

```bash
# Lister les enregistrements d'une date spécifique
ls -la /var/lib/karl/recordings/2024-01-15/

# Trouver les enregistrements d'un appel spécifique
find /var/lib/karl/recordings -name "*abc123*"
```

---

## Considérations de production

### Planification du stockage

Calculer les besoins en stockage :

```
Stockage quotidien = appels_par_jour × durée_moyenne_minutes × Mo_par_minute

Exemple :
- 1000 appels/jour
- 5 minutes de durée moyenne
- 2 Mo/minute (stéréo)
= 1000 × 5 × 2 = 10 Go/jour
= 300 Go/mois
```

### Haute disponibilité

1. **Stockage partagé** : Utiliser NFS, EFS ou similaire pour les déploiements multi-instances
2. **Sauvegarde** : Sauvegarde régulière du répertoire d'enregistrement
3. **Surveillance** : Alerter sur l'utilisation de l'espace disque

### Conformité

Pour les industries réglementées :

1. **Chiffrement au repos** : Utiliser un système de fichiers chiffré
2. **Contrôle d'accès** : Restreindre les permissions du répertoire d'enregistrement
3. **Journalisation d'audit** : Suivre les accès aux enregistrements
4. **Rétention** : Configurer la période de rétention appropriée

### Performance

Impact de l'enregistrement sur le système :

| Enregistrements simultanés | CPU supplémentaire | Mémoire supplémentaire |
|---------------------------|-------------------|----------------------|
| 100 | ~5% | ~50 Mo |
| 500 | ~15% | ~200 Mo |
| 1000 | ~25% | ~400 Mo |

---

## Dépannage

### Enregistrements non créés

1. **Vérifier que l'enregistrement est activé** :
```bash
grep -A 10 '"recording"' /etc/karl/config.json
```

2. **Vérifier les permissions du répertoire** :
```bash
ls -la /var/lib/karl/recordings
# Doit être accessible en écriture par l'utilisateur karl
```

3. **Vérifier l'espace disque** :
```bash
df -h /var/lib/karl/recordings
```

4. **Vérifier que le drapeau record-call est envoyé** :
```bash
# Activer la journalisation de débogage
KARL_LOG_LEVEL=debug ./karl

# Chercher les messages d'enregistrement
grep -i recording /var/log/karl.log
```

### Fichiers d'enregistrement vides

1. **Vérifier que la session a du média** :
```bash
curl http://localhost:8080/api/v1/sessions/abc123
# Vérifier que packets_received > 0
```

2. **Vérifier la prise en charge des codecs** :
Karl prend en charge G.711 (PCMU/PCMA) et Opus. D'autres codecs peuvent ne pas s'enregistrer correctement.

3. **Vérifier les erreurs** :
```bash
grep -i "recording\|codec" /var/log/karl.log
```

### Mauvaise qualité d'enregistrement

1. **Inadéquation du taux d'échantillonnage** : Assurez-vous que sample_rate correspond à votre codec
2. **Problèmes réseau** : Une forte perte de paquets affecte la qualité d'enregistrement
3. **Gigue** : Activez le tampon de gigue pour une meilleure qualité

### Problèmes d'espace disque

1. **Activer la politique de rétention** :
```json
{
  "recording": {
    "retention_days": 7
  }
}
```

2. **Nettoyage manuel** :
```bash
# Supprimer les enregistrements de plus de 7 jours
find /var/lib/karl/recordings -name "*.wav" -mtime +7 -delete
```

3. **Surveiller l'utilisation du disque** :
```bash
# Ajouter au cron
0 * * * * /usr/bin/df -h /var/lib/karl/recordings | mail -s "Karl disk usage" admin@example.com
```

---

## Étapes suivantes

- [Surveiller avec Prometheus](./monitoring-prometheus.md)
- [Intégrer avec OpenSIPS](./integrating-opensips.md)
- [Intégrer avec Kamailio](./integrating-kamailio.md)
