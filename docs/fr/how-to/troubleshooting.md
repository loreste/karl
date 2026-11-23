# Guide de dépannage

Ce guide couvre le diagnostic et la résolution des problèmes courants avec le serveur multimédia Karl.

## Table des matières

- [Outils de diagnostic](#outils-de-diagnostic)
- [Problèmes de démarrage](#problèmes-de-démarrage)
- [Problèmes de connexion](#problèmes-de-connexion)
- [Problèmes multimédia](#problèmes-multimédia)
- [Problèmes d'enregistrement](#problèmes-denregistrement)
- [Problèmes de performance](#problèmes-de-performance)
- [Problèmes Kubernetes](#problèmes-kubernetes)
- [Obtenir de l'aide](#obtenir-de-laide)

---

## Outils de diagnostic

### Vérifier l'état de Karl

```bash
# Processus en cours d'exécution
ps aux | grep karl

# État systemd
systemctl status karl

# État Docker
docker ps | grep karl
docker logs karl

# État Kubernetes
kubectl get pods -l app=karl
kubectl describe pod -l app=karl
```

### Tester le protocole NG

```bash
# Envoyer une commande ping
echo -n "d7:command4:pinge" | nc -u localhost 22222

# La réponse attendue contient "pong"
```

### Vérifier les points de terminaison de santé

```bash
# Santé simple
curl http://localhost:8086/health

# Santé détaillée
curl http://localhost:8086/health/detail

# Sondes Kubernetes
curl http://localhost:8086/startup
curl http://localhost:8086/live
curl http://localhost:8086/ready
```

### Afficher les métriques

```bash
# Toutes les métriques
curl http://localhost:9091/metrics

# Sessions actives
curl -s http://localhost:9091/metrics | grep karl_sessions_active
```

### Consulter les journaux

```bash
# Systemd
journalctl -u karl -f

# Docker
docker logs -f karl

# Kubernetes
kubectl logs -l app=karl -f

# Avec le niveau debug
KARL_LOG_LEVEL=debug ./karl
```

### Requêtes API

```bash
# Lister les sessions
curl http://localhost:8080/api/v1/sessions

# Statistiques du serveur
curl http://localhost:8080/api/v1/stats

# Session spécifique
curl http://localhost:8080/api/v1/sessions/{session_id}
```

---

## Problèmes de démarrage

### "bind: address already in use"

Un autre processus utilise le port.

**Solution** :
```bash
# Trouver le processus
sudo lsof -i :22222
sudo lsof -i :8080

# Terminer si nécessaire
sudo kill <PID>

# Ou changer les ports de Karl
KARL_NG_PORT=22223 ./karl
```

### "permission denied" pour le socket

Permissions insuffisantes pour le socket Unix ou les ports.

**Solution** :
```bash
# Créer le répertoire avec les bonnes permissions
sudo mkdir -p /var/run/karl
sudo chown karl:karl /var/run/karl

# Pour les ports inférieurs à 1024
sudo setcap 'cap_net_bind_service=+ep' /usr/local/bin/karl
```

### "too many open files"

Limite de descripteurs de fichiers trop basse.

**Solution** :
```bash
# Correction temporaire
ulimit -n 65535

# Correction permanente - ajouter à /etc/security/limits.conf
karl soft nofile 65535
karl hard nofile 65535

# Ou dans le service systemd
[Service]
LimitNOFILE=65535
```

### Fichier de configuration introuvable

```bash
# Vérifier que le fichier existe
ls -la /etc/karl/config.json

# Vérifier le chemin
KARL_CONFIG_PATH=/etc/karl/config.json ./karl

# Utiliser la configuration par défaut
./karl  # Utilise ./config/config.json
```

### Configuration invalide

```bash
# Valider le JSON
cat /etc/karl/config.json | jq .

# Vérifier les problèmes courants
# - Virgules manquantes
# - Virgules en fin de ligne
# - Valeurs invalides
```

---

## Problèmes de connexion

### Le proxy SIP ne peut pas se connecter

**Symptômes** : Pas de réponse aux commandes du protocole NG

**Diagnostic** :
```bash
# Vérifier que Karl écoute
ss -ulnp | grep 22222

# Tester depuis le proxy SIP
echo -n "d7:command4:pinge" | nc -u karl-ip 22222

# Vérifier le pare-feu
sudo iptables -L -n | grep 22222
```

**Solutions** :
1. Vérifier que Karl fonctionne et écoute sur la bonne interface
2. Vérifier que les règles de pare-feu autorisent l'UDP 22222
3. Vérifier que la configuration du proxy SIP pointe vers la bonne IP/port

### L'API ne répond pas

**Diagnostic** :
```bash
# Vérifier que l'API écoute
ss -tlnp | grep 8080

# Tester localement
curl http://localhost:8080/api/v1/stats

# Vérifier depuis une machine distante
curl http://karl-ip:8080/api/v1/stats
```

**Solutions** :
1. Vérifier que l'API est activée dans la configuration
2. Vérifier le pare-feu pour TCP 8080
3. Vérifier l'adresse de liaison (utiliser `0.0.0.0` pour toutes les interfaces)

### Échec de connexion Redis

**Symptômes** : Sessions non partagées entre les instances

**Diagnostic** :
```bash
# Tester la connectivité Redis
redis-cli -h redis-host ping

# Vérifier les journaux de Karl
grep -i redis /var/log/karl.log

# Vérifier la configuration
grep -A 5 '"database"' /etc/karl/config.json
```

**Solutions** :
1. Vérifier que Redis fonctionne
2. Vérifier la connectivité réseau
3. Vérifier l'adresse Redis dans la configuration
4. Vérifier l'authentification Redis si activée

---

## Problèmes multimédia

### Pas d'audio (les deux directions)

**Diagnostic** :
```bash
# Vérifier que les sessions sont créées
curl http://localhost:8080/api/v1/sessions

# Vérifier que les ports RTP sont alloués
ss -ulnp | grep -E "3[0-9]{4}"

# Vérifier les compteurs de paquets
curl http://localhost:9091/metrics | grep packets
```

**Causes courantes** :
1. Ports RTP bloqués par le pare-feu
2. Configuration `media_ip` incorrecte
3. Problèmes NAT - IP privée fuitant dans le SDP envoyé aux pairs externes
4. `interfaces` / `direction` manquants ou mal configurés

**Solutions** :
```bash
# Ouvrir les ports RTP
sudo iptables -A INPUT -p udp --dport 30000:40000 -j ACCEPT

# Vérifier media_ip
KARL_MEDIA_IP=192.168.1.100 ./karl

# Pour le NAT, définir public_ip
KARL_PUBLIC_IP=203.0.113.50 ./karl
```

### Audio unidirectionnel

**Solutions** :
```bash
# Dans le proxy SIP, forcer le RTP symétrique
rtpengine_manage("... symmetric");

# Vérifier que les deux directions ont des paquets
curl http://localhost:8080/api/v1/sessions/{id}
# Vérifier que packets_sent et packets_received > 0
```

### Mauvaise qualité audio

**Diagnostic** :
```bash
# Vérifier les métriques RTCP
curl http://localhost:9091/metrics | grep rtcp

# Chercher :
# - Forte perte de paquets (> 5%)
# - Forte gigue (> 50ms)
# - Fort RTT (> 200ms)
```

**Solutions** :

```json
{
  "fec": {
    "enabled": true,
    "redundancy": 0.30,
    "adaptive_mode": true
  },
  "jitter_buffer": {
    "enabled": true,
    "max_delay": 200,
    "adaptive_mode": true
  }
}
```

---

## Problèmes d'enregistrement

### Enregistrements non créés

**Solutions** :
1. Activer l'enregistrement dans la configuration
2. Créer le répertoire avec les bonnes permissions
3. S'assurer que le drapeau `record-call` est envoyé par le proxy SIP

### Fichiers d'enregistrement vides

**Causes** :
1. Pas de paquets multimédia reçus
2. Codec non pris en charge
3. Session terminée trop rapidement

### Problèmes d'espace disque

```bash
# Vérifier l'utilisation du disque
df -h /var/lib/karl/recordings

# Activer la politique de rétention
{
  "recording": {
    "retention_days": 7
  }
}

# Nettoyage manuel
find /var/lib/karl/recordings -name "*.wav" -mtime +7 -delete
```

---

## Problèmes de performance

### Utilisation CPU élevée

**Diagnostic** :
```bash
# Vérifier le CPU du processus
top -p $(pgrep karl)
```

**Solutions** :
1. Mise à l'échelle horizontale
2. Réduire la redondance FEC
3. Augmenter l'allocation CPU

### Utilisation mémoire élevée

**Solutions** :
1. Réduire max_sessions
2. Diminuer la taille du tampon de gigue
3. Ajouter des limites de mémoire et laisser l'orchestrateur gérer les redémarrages

### Création de sessions lente

**Solutions** :
1. Vérifier la latence Redis (si Redis est utilisé)
2. Réduire la plage d'allocation de ports
3. Mise à l'échelle horizontale

---

## Problèmes Kubernetes

### Le pod ne démarre pas

```bash
# Vérifier l'état du pod
kubectl describe pod -l app=karl

# Problèmes courants :
# - Erreurs de téléchargement d'image
# - Limites de ressources
# - Échecs des sondes
# - Problèmes de montage de volume
```

### Échec des sondes

```bash
# Tester les sondes manuellement
kubectl exec -it <pod> -- curl localhost:8086/startup
kubectl exec -it <pod> -- curl localhost:8086/live
kubectl exec -it <pod> -- curl localhost:8086/ready
```

### Pas de connectivité réseau

```bash
# Vérifier les points de terminaison du service
kubectl get endpoints karl

# Tester depuis l'intérieur du cluster
kubectl run test --rm -it --image=alpine -- sh
apk add curl netcat-openbsd
nc -u karl 22222

# Vérifier les politiques réseau
kubectl get networkpolicies
```

---

## Obtenir de l'aide

### Informations à rassembler

Lors du signalement de problèmes, incluez :

1. **Version de Karl**
```bash
./karl --version
```

2. **Configuration** (supprimer les données sensibles)
```bash
cat /etc/karl/config.json
```

3. **Journaux**
```bash
journalctl -u karl --since "1 hour ago" > karl-logs.txt
```

4. **Métriques**
```bash
curl http://localhost:9091/metrics > metrics.txt
```

5. **Environnement**
```bash
uname -a
cat /etc/os-release
```

### Canaux de support

- **GitHub Issues** : [github.com/loreste/karl/issues](https://github.com/loreste/karl/issues)
- **GitHub Discussions** : [github.com/loreste/karl/discussions](https://github.com/loreste/karl/discussions)

### Résumé des commandes utiles

```bash
# Vérification rapide de santé
curl -s http://localhost:8086/health | jq .

# Nombre de sessions
curl -s http://localhost:9091/metrics | grep karl_sessions_active

# Erreurs récentes
grep -i error /var/log/karl.log | tail -20

# Tester le protocole NG
echo -n "d7:command4:pinge" | nc -u localhost 22222

# Vérification des ports
ss -ulnp | grep -E "22222|3[0-9]{4}"
```

---

## Étapes suivantes

- [Référence de configuration](../configuration.md)
- [Configuration de la surveillance](./monitoring-prometheus.md)
- [Déploiement Kubernetes](./deploying-kubernetes.md)
