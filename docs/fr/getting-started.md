# Prise en main du serveur multimédia Karl

Ce guide vous aidera à lancer Karl en moins de 5 minutes.

## Prérequis

- Go 1.26 ou supérieur (pour la compilation depuis les sources)
- Docker (optionnel, pour le déploiement en conteneur)
- Un proxy SIP (OpenSIPS ou Kamailio) pour une utilisation en production

## Option 1 : Exécuter depuis les sources

```bash
# Cloner le dépôt
git clone https://github.com/loreste/karl.git
cd karl

# Compiler
go build -o karl

# Exécuter avec les paramètres par défaut
./karl
```

Karl fonctionne maintenant avec la configuration par défaut :
- Protocole NG : port UDP 22222
- Ports RTP : 30000-40000
- API REST : port TCP 8080
- Vérification de santé : port TCP 8086
- Métriques : port TCP 9091

## Option 2 : Exécuter avec Docker

```bash
# Exécuter avec le réseau hôte (recommandé pour les serveurs multimédia)
docker run -d \
  --name karl \
  --network host \
  loreste/karl:latest
```

Ou avec un mappage de ports explicite :

```bash
docker run -d \
  --name karl \
  -p 22222:22222/udp \
  -p 30000-30100:30000-30100/udp \
  -p 8080:8080 \
  -p 8086:8086 \
  -p 9091:9091 \
  loreste/karl:latest
```

## Vérifier l'installation

### Tester le protocole NG

```bash
# Envoyer une commande ping
echo -n "d7:command4:pinge" | nc -u localhost 22222
```

La réponse attendue contient `pong`.

### Vérifier le point de terminaison de santé

```bash
curl http://localhost:8086/health
```

Réponse attendue :
```json
{"status":"UP"}
```

### Afficher les métriques

```bash
curl http://localhost:9091/metrics | head -20
```

## Connecter à votre proxy SIP

### OpenSIPS

Ajoutez à votre `opensips.cfg` :

```opensips
loadmodule "rtpengine.so"
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")
```

### Kamailio

Ajoutez à votre `kamailio.cfg` :

```kamailio
loadmodule "rtpengine.so"
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")
```

## Effectuer un appel test

1. Enregistrez deux téléphones SIP avec votre proxy SIP
2. Passez un appel entre eux
3. Vérifiez que le média transite par Karl :

```bash
# Vérifier les sessions actives
curl http://localhost:8080/api/v1/sessions
```

## Étapes suivantes

- [Référence de configuration](./configuration.md) - Personnaliser Karl pour votre environnement
- [Déploiement Kubernetes](./how-to/deploying-kubernetes.md) - Déployer sur Kubernetes
- [Configurer la surveillance](./how-to/monitoring-prometheus.md) - Ajouter Prometheus et Grafana
- [Activer l'enregistrement](./how-to/setting-up-recording.md) - Enregistrer les appels

## Problèmes courants

### "connection refused" sur le port 22222

Karl ne fonctionne pas ou est lié à une interface différente. Vérifiez :

```bash
# Karl est-il en cours d'exécution ?
ps aux | grep karl

# Sur quel port écoute-t-il ?
ss -ulnp | grep 22222
```

### Pas d'audio dans les appels

1. Vérifiez que les ports RTP (30000-40000) sont accessibles
2. Vérifiez que les règles de pare-feu autorisent le trafic UDP
3. Consultez les journaux de Karl pour les erreurs :

```bash
# Si exécuté en tant que service
journalctl -u karl -f

# Si exécuté dans Docker
docker logs -f karl
```

Consultez le [Guide de dépannage](./how-to/troubleshooting.md) pour plus d'aide.
