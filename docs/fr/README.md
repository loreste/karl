[English](../README.md) | [中文](../zh-CN/README.md) | **Français** | [Español](../es/README.md)

# Documentation du serveur multimédia Karl

Bienvenue dans la documentation du serveur multimédia Karl. Ce guide vous aidera à installer, configurer et exploiter Karl dans des environnements de production.

## Aperçu de la documentation

### Prise en main

- [Guide de démarrage rapide](./getting-started.md) - Lancez Karl en 5 minutes
- [Guide d'installation](./installation.md) - Instructions d'installation détaillées
- [Référence de configuration](./configuration.md) - Options de configuration complètes

### Guides pratiques

Guides étape par étape pour les tâches courantes :

- [Déployer sur Kubernetes](./how-to/deploying-kubernetes.md) - Exécuter Karl dans des clusters Kubernetes
- [Intégrer avec OpenSIPS](./how-to/integrating-opensips.md) - Connecter Karl à OpenSIPS
- [Intégrer avec Kamailio](./how-to/integrating-kamailio.md) - Connecter Karl à Kamailio
- [Configurer l'enregistrement des appels](./how-to/setting-up-recording.md) - Activer et configurer l'enregistrement des appels
- [Surveiller avec Prometheus](./how-to/monitoring-prometheus.md) - Configurer les métriques et les alertes
- [Mise à l'échelle horizontale](./how-to/scaling-horizontally.md) - Exécuter plusieurs instances de Karl
- [Pont WebRTC vers SIP](./how-to/webrtc-sip-bridging.md) - Connecter les clients navigateur aux réseaux SIP
- [Sécuriser avec TLS](./how-to/securing-with-tls.md) - Activer le chiffrement pour l'API et le plan de contrôle
- [Guide de dépannage](./how-to/troubleshooting.md) - Diagnostiquer et résoudre les problèmes courants

### Référence

- [Référence du protocole NG](./reference/ng-protocol.md) - Documentation complète du protocole NG
- [Référence de l'API REST](./reference/rest-api.md) - Points de terminaison de l'API et exemples
- [Référence des métriques](./reference/metrics.md) - Documentation des métriques Prometheus
- [Variables d'environnement](./reference/environment-variables.md) - Toutes les variables d'environnement prises en charge

### Architecture

- [Aperçu de l'architecture](./architecture/overview.md) - Fonctionnement interne de Karl
- [Cycle de vie d'une session](./architecture/session-lifecycle.md) - Flux d'appel et gestion d'état
- [Traitement multimédia](./architecture/media-processing.md) - Gestion RTP et fonctionnalités de qualité

---

## Liens rapides

| Tâche | Documentation |
|-------|---------------|
| Installer Karl | [Guide d'installation](./installation.md) |
| Configurer pour la production | [Référence de configuration](./configuration.md) |
| Déployer sur Kubernetes | [Guide Kubernetes](./how-to/deploying-kubernetes.md) |
| Configurer la surveillance | [Guide Prometheus](./how-to/monitoring-prometheus.md) |
| Déboguer les problèmes | [Guide de dépannage](./how-to/troubleshooting.md) |

---

## Support

- **GitHub Issues** : [Signaler des bogues et demander des fonctionnalités](https://github.com/loreste/karl/issues)
- **GitHub Discussions** : [Poser des questions et partager des idées](https://github.com/loreste/karl/discussions)
