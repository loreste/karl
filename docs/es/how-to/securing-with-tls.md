# Como Asegurar Karl con TLS

Esta guia cubre la habilitacion del cifrado TLS para la API REST y las interfaces de gestion de Karl.

## Tabla de Contenidos

- [Resumen](#resumen)
- [Generar Certificados](#generar-certificados)
- [Configurar TLS](#configurar-tls)
- [TLS en Kubernetes](#tls-en-kubernetes)
- [Gestion de Certificados](#gestion-de-certificados)
- [Solucion de Problemas](#solucion-de-problemas)

---

## Resumen

Karl soporta cifrado TLS para:

- API REST (`:8080`)
- Endpoints de salud (`:8086`)
- Metricas de Prometheus (`:9091`) - via proxy reverso

---

## Generar Certificados

### Auto-firmado (Desarrollo)

```bash
# Generar clave privada
openssl genrsa -out server.key 4096

# Generar solicitud de firma de certificado
openssl req -new -key server.key -out server.csr \
  -subj "/CN=karl.example.com/O=Karl Media Server"

# Generar certificado auto-firmado (valido por 365 dias)
openssl x509 -req -days 365 -in server.csr \
  -signkey server.key -out server.crt

# Copiar al directorio de configuracion de Karl
sudo mkdir -p /etc/karl/certs
sudo cp server.key server.crt /etc/karl/certs/
sudo chmod 600 /etc/karl/certs/server.key
```

### Let's Encrypt (Produccion)

Usando certbot:

```bash
# Instalar certbot
sudo apt install certbot

# Generar certificado
sudo certbot certonly --standalone -d karl.example.com

# Los certificados estaran en:
# /etc/letsencrypt/live/karl.example.com/fullchain.pem
# /etc/letsencrypt/live/karl.example.com/privkey.pem

# Copiar o crear enlace simbolico
sudo ln -s /etc/letsencrypt/live/karl.example.com/fullchain.pem /etc/karl/certs/server.crt
sudo ln -s /etc/letsencrypt/live/karl.example.com/privkey.pem /etc/karl/certs/server.key
```

### De una Autoridad Certificadora

Si tiene certificados firmados por una CA:

```bash
# Combinar cadena de certificados
cat your_certificate.crt intermediate.crt root.crt > /etc/karl/certs/server.crt

# Copiar clave privada
cp your_private.key /etc/karl/certs/server.key
chmod 600 /etc/karl/certs/server.key
```

---

## Configurar TLS

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

### Variables de Entorno

```bash
export KARL_API_TLS_ENABLED=true
export KARL_API_TLS_CERT=/etc/karl/certs/server.crt
export KARL_API_TLS_KEY=/etc/karl/certs/server.key
```

### Verificar TLS

```bash
# Probar conexion HTTPS
curl -k https://localhost:8080/api/v1/stats

# Verificar certificado
openssl s_client -connect localhost:8080 -showcerts

# Sin -k (requiere certificado valido)
curl --cacert /etc/karl/certs/server.crt https://localhost:8080/api/v1/stats
```

---

## TLS en Kubernetes

### Usando Secrets

```bash
# Crear secret TLS
kubectl create secret tls karl-tls \
  --cert=/path/to/server.crt \
  --key=/path/to/server.key
```

### Deployment con TLS

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

### Usando cert-manager

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

### Ingress con TLS

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

## Gestion de Certificados

### Renovacion Automatica con Let's Encrypt

```bash
# Agregar a crontab
0 0 1 * * certbot renew --quiet && systemctl reload karl
```

### Monitoreo de Expiracion de Certificados

Alerta de Prometheus:

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
          summary: "El certificado TLS de Karl expira en < 30 dias"
```

### Recargar Certificados

Karl requiere un reinicio para recargar certificados:

```bash
# Systemd
sudo systemctl restart karl

# Docker
docker restart karl

# Kubernetes
kubectl rollout restart deployment karl
```

---

## Proxy Reverso (Alternativa)

Use nginx o HAProxy para terminacion TLS:

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

## Solucion de Problemas

### Errores de Certificado

```bash
# Verificar validez del certificado
openssl x509 -in /etc/karl/certs/server.crt -noout -dates

# Verificar que la clave coincide con el certificado
openssl x509 -noout -modulus -in server.crt | openssl md5
openssl rsa -noout -modulus -in server.key | openssl md5
# Ambos deben coincidir
```

### Problemas de Permisos

```bash
# Verificar permisos de archivos
ls -la /etc/karl/certs/

# Corregir permisos
sudo chown karl:karl /etc/karl/certs/server.key
sudo chmod 600 /etc/karl/certs/server.key
```

### Conexion Rechazada

```bash
# Verificar que Karl esta escuchando en HTTPS
ss -tlnp | grep 8080

# Verificar logs para errores TLS
journalctl -u karl | grep -i tls
```

---

## Siguientes Pasos

- [Referencia de Configuracion](../configuration.md)
- [Despliegue en Kubernetes](./deploying-kubernetes.md)
- [Configuracion de Monitoreo](./monitoring-prometheus.md)
