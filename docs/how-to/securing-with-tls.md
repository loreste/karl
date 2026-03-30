# How to Secure Karl with TLS

This guide covers enabling TLS encryption for Karl's REST API and management interfaces.

## Table of Contents

- [Overview](#overview)
- [Generate Certificates](#generate-certificates)
- [Configure TLS](#configure-tls)
- [Kubernetes TLS](#kubernetes-tls)
- [Certificate Management](#certificate-management)
- [Troubleshooting](#troubleshooting)

---

## Overview

Karl supports TLS encryption for:

- REST API (`:8080`)
- Health endpoints (`:8086`)
- Prometheus metrics (`:9091`) - via reverse proxy

---

## Generate Certificates

### Self-Signed (Development)

```bash
# Generate private key
openssl genrsa -out server.key 4096

# Generate certificate signing request
openssl req -new -key server.key -out server.csr \
  -subj "/CN=karl.example.com/O=Karl Media Server"

# Generate self-signed certificate (valid for 365 days)
openssl x509 -req -days 365 -in server.csr \
  -signkey server.key -out server.crt

# Copy to Karl config directory
sudo mkdir -p /etc/karl/certs
sudo cp server.key server.crt /etc/karl/certs/
sudo chmod 600 /etc/karl/certs/server.key
```

### Let's Encrypt (Production)

Using certbot:

```bash
# Install certbot
sudo apt install certbot

# Generate certificate
sudo certbot certonly --standalone -d karl.example.com

# Certificates will be in:
# /etc/letsencrypt/live/karl.example.com/fullchain.pem
# /etc/letsencrypt/live/karl.example.com/privkey.pem

# Copy or symlink
sudo ln -s /etc/letsencrypt/live/karl.example.com/fullchain.pem /etc/karl/certs/server.crt
sudo ln -s /etc/letsencrypt/live/karl.example.com/privkey.pem /etc/karl/certs/server.key
```

### From Certificate Authority

If you have CA-signed certificates:

```bash
# Combine certificate chain
cat your_certificate.crt intermediate.crt root.crt > /etc/karl/certs/server.crt

# Copy private key
cp your_private.key /etc/karl/certs/server.key
chmod 600 /etc/karl/certs/server.key
```

---

## Configure TLS

### REST API

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

### Environment Variables

```bash
export KARL_API_TLS_ENABLED=true
export KARL_API_TLS_CERT=/etc/karl/certs/server.crt
export KARL_API_TLS_KEY=/etc/karl/certs/server.key
```

### Verify TLS

```bash
# Test HTTPS connection
curl -k https://localhost:8080/api/v1/stats

# Verify certificate
openssl s_client -connect localhost:8080 -showcerts

# Without -k (requires valid cert)
curl --cacert /etc/karl/certs/server.crt https://localhost:8080/api/v1/stats
```

---

## Kubernetes TLS

### Using Secrets

```bash
# Create TLS secret
kubectl create secret tls karl-tls \
  --cert=/path/to/server.crt \
  --key=/path/to/server.key
```

### Deployment with TLS

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

### Using cert-manager

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

### Ingress with TLS

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

## Certificate Management

### Auto-Renewal with Let's Encrypt

```bash
# Add to crontab
0 0 1 * * certbot renew --quiet && systemctl reload karl
```

### Certificate Expiry Monitoring

Prometheus alert:

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
          summary: "Karl TLS certificate expires in < 30 days"
```

### Reload Certificates

Karl requires restart to reload certificates:

```bash
# Systemd
sudo systemctl restart karl

# Docker
docker restart karl

# Kubernetes
kubectl rollout restart deployment karl
```

---

## Reverse Proxy (Alternative)

Use nginx or HAProxy for TLS termination:

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

## Troubleshooting

### Certificate Errors

```bash
# Check certificate validity
openssl x509 -in /etc/karl/certs/server.crt -noout -dates

# Verify key matches certificate
openssl x509 -noout -modulus -in server.crt | openssl md5
openssl rsa -noout -modulus -in server.key | openssl md5
# Both should match
```

### Permission Issues

```bash
# Check file permissions
ls -la /etc/karl/certs/

# Fix permissions
sudo chown karl:karl /etc/karl/certs/server.key
sudo chmod 600 /etc/karl/certs/server.key
```

### Connection Refused

```bash
# Check Karl is listening on HTTPS
ss -tlnp | grep 8080

# Check logs for TLS errors
journalctl -u karl | grep -i tls
```

---

## Next Steps

- [Configuration Reference](../configuration.md)
- [Kubernetes Deployment](./deploying-kubernetes.md)
- [Monitoring Setup](./monitoring-prometheus.md)
