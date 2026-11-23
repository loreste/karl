# Comment sécuriser Karl avec TLS

Ce guide couvre l'activation du chiffrement TLS pour l'API REST et les interfaces de gestion de Karl.

## Table des matières

- [Aperçu](#aperçu)
- [Générer des certificats](#générer-des-certificats)
- [Configurer TLS](#configurer-tls)
- [TLS sur Kubernetes](#tls-sur-kubernetes)
- [Gestion des certificats](#gestion-des-certificats)
- [Dépannage](#dépannage)

---

## Aperçu

Karl prend en charge le chiffrement TLS pour :

- API REST (`:8080`)
- Points de terminaison de santé (`:8086`)
- Métriques Prometheus (`:9091`) - via un proxy inverse

---

## Générer des certificats

### Auto-signé (développement)

```bash
# Générer la clé privée
openssl genrsa -out server.key 4096

# Générer la demande de signature de certificat
openssl req -new -key server.key -out server.csr \
  -subj "/CN=karl.example.com/O=Karl Media Server"

# Générer le certificat auto-signé (valide 365 jours)
openssl x509 -req -days 365 -in server.csr \
  -signkey server.key -out server.crt

# Copier dans le répertoire de configuration de Karl
sudo mkdir -p /etc/karl/certs
sudo cp server.key server.crt /etc/karl/certs/
sudo chmod 600 /etc/karl/certs/server.key
```

### Let's Encrypt (production)

Avec certbot :

```bash
# Installer certbot
sudo apt install certbot

# Générer le certificat
sudo certbot certonly --standalone -d karl.example.com

# Les certificats seront dans :
# /etc/letsencrypt/live/karl.example.com/fullchain.pem
# /etc/letsencrypt/live/karl.example.com/privkey.pem

# Copier ou créer un lien symbolique
sudo ln -s /etc/letsencrypt/live/karl.example.com/fullchain.pem /etc/karl/certs/server.crt
sudo ln -s /etc/letsencrypt/live/karl.example.com/privkey.pem /etc/karl/certs/server.key
```

### Depuis une autorité de certification

Si vous avez des certificats signés par une CA :

```bash
# Combiner la chaîne de certificats
cat your_certificate.crt intermediate.crt root.crt > /etc/karl/certs/server.crt

# Copier la clé privée
cp your_private.key /etc/karl/certs/server.key
chmod 600 /etc/karl/certs/server.key
```

---

## Configurer TLS

### API REST

```json
{
  "api": {
    "enabled": true,
    "address": ":8080",
    "tls_enabled": true,
    "tls_cert": "/etc/karl/certs/server.crt",
    "tls_key": "/etc/karl/certs/server.key"
  }
}
```

### Variables d'environnement

```bash
export KARL_API_TLS_ENABLED=true
export KARL_API_TLS_CERT=/etc/karl/certs/server.crt
export KARL_API_TLS_KEY=/etc/karl/certs/server.key
```

### Vérifier TLS

```bash
# Tester la connexion HTTPS
curl -k https://localhost:8080/api/v1/stats

# Vérifier le certificat
openssl s_client -connect localhost:8080 -showcerts

# Sans -k (nécessite un certificat valide)
curl --cacert /etc/karl/certs/server.crt https://localhost:8080/api/v1/stats
```

---

## TLS sur Kubernetes

### Utilisation des Secrets

```bash
# Créer un secret TLS
kubectl create secret tls karl-tls \
  --cert=/path/to/server.crt \
  --key=/path/to/server.key
```

### Déploiement avec TLS

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: karl
spec:
  template:
    spec:
      containers:
      - name: karl
        env:
        - name: KARL_API_TLS_ENABLED
          value: "true"
        - name: KARL_API_TLS_CERT
          value: "/etc/karl/certs/tls.crt"
        - name: KARL_API_TLS_KEY
          value: "/etc/karl/certs/tls.key"
        volumeMounts:
        - name: tls-certs
          mountPath: /etc/karl/certs
          readOnly: true
      volumes:
      - name: tls-certs
        secret:
          secretName: karl-tls
```

### Utilisation de cert-manager

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: karl-cert
spec:
  secretName: karl-tls
  issuerRef:
    name: letsencrypt-prod
    kind: ClusterIssuer
  commonName: karl.example.com
  dnsNames:
  - karl.example.com
```

### Ingress avec TLS

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: karl-ingress
  annotations:
    kubernetes.io/ingress.class: nginx
    cert-manager.io/cluster-issuer: letsencrypt-prod
spec:
  tls:
  - hosts:
    - karl.example.com
    secretName: karl-tls
  rules:
  - host: karl.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: karl
            port:
              number: 8080
```

---

## Gestion des certificats

### Renouvellement automatique avec Let's Encrypt

```bash
# Ajouter au crontab
0 0 1 * * certbot renew --quiet && systemctl reload karl
```

### Surveillance de l'expiration des certificats

Alerte Prometheus :

```yaml
groups:
  - name: tls
    rules:
      - alert: KarlCertExpiringSoon
        expr: probe_ssl_earliest_cert_expiry - time() < 86400 * 30
        for: 24h
        labels:
          severity: warning
        annotations:
          summary: "Le certificat TLS de Karl expire dans moins de 30 jours"
```

### Recharger les certificats

Karl nécessite un redémarrage pour recharger les certificats :

```bash
# Systemd
sudo systemctl restart karl

# Docker
docker restart karl

# Kubernetes
kubectl rollout restart deployment karl
```

---

## Proxy inverse (alternative)

Utilisez nginx ou HAProxy pour la terminaison TLS :

### Nginx

```nginx
server {
    listen 443 ssl;
    server_name karl.example.com;

    ssl_certificate /etc/ssl/certs/karl.crt;
    ssl_certificate_key /etc/ssl/private/karl.key;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

### HAProxy

```haproxy
frontend karl_https
    bind *:443 ssl crt /etc/ssl/karl.pem
    default_backend karl_backend

backend karl_backend
    server karl1 127.0.0.1:8080
```

---

## Dépannage

### Erreurs de certificat

```bash
# Vérifier la validité du certificat
openssl x509 -in /etc/karl/certs/server.crt -noout -dates

# Vérifier que la clé correspond au certificat
openssl x509 -noout -modulus -in server.crt | openssl md5
openssl rsa -noout -modulus -in server.key | openssl md5
# Les deux doivent correspondre
```

### Problèmes de permissions

```bash
# Vérifier les permissions des fichiers
ls -la /etc/karl/certs/

# Corriger les permissions
sudo chown karl:karl /etc/karl/certs/server.key
sudo chmod 600 /etc/karl/certs/server.key
```

### Connexion refusée

```bash
# Vérifier que Karl écoute en HTTPS
ss -tlnp | grep 8080

# Vérifier les journaux pour les erreurs TLS
journalctl -u karl | grep -i tls
```

---

## Étapes suivantes

- [Référence de configuration](../configuration.md)
- [Déploiement Kubernetes](./deploying-kubernetes.md)
- [Configuration de la surveillance](./monitoring-prometheus.md)
