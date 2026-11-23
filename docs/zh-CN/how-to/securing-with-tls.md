# 如何使用 TLS 保护 Karl

本指南涵盖为 Karl 的 REST API 和管理接口启用 TLS 加密。

## 目录

- [概述](#概述)
- [生成证书](#生成证书)
- [配置 TLS](#配置-tls)
- [Kubernetes TLS](#kubernetes-tls)
- [证书管理](#证书管理)
- [故障排除](#故障排除)

---

## 概述

Karl 支持以下接口的 TLS 加密：

- REST API（`:8080`）
- 健康端点（`:8086`）
- Prometheus 指标（`:9091`）— 通过反向代理

---

## 生成证书

### 自签名证书（开发环境）

```bash
# 生成私钥
openssl genrsa -out server.key 4096

# 生成证书签名请求
openssl req -new -key server.key -out server.csr \
  -subj "/CN=karl.example.com/O=Karl Media Server"

# 生成自签名证书（有效期 365 天）
openssl x509 -req -days 365 -in server.csr \
  -signkey server.key -out server.crt

# 复制到 Karl 配置目录
sudo mkdir -p /etc/karl/certs
sudo cp server.key server.crt /etc/karl/certs/
sudo chmod 600 /etc/karl/certs/server.key
```

### Let's Encrypt（生产环境）

使用 certbot：

```bash
# 安装 certbot
sudo apt install certbot

# 生成证书
sudo certbot certonly --standalone -d karl.example.com

# 证书位于：
# /etc/letsencrypt/live/karl.example.com/fullchain.pem
# /etc/letsencrypt/live/karl.example.com/privkey.pem

# 复制或创建符号链接
sudo ln -s /etc/letsencrypt/live/karl.example.com/fullchain.pem /etc/karl/certs/server.crt
sudo ln -s /etc/letsencrypt/live/karl.example.com/privkey.pem /etc/karl/certs/server.key
```

### 来自证书颁发机构

如果您有 CA 签发的证书：

```bash
# 合并证书链
cat your_certificate.crt intermediate.crt root.crt > /etc/karl/certs/server.crt

# 复制私钥
cp your_private.key /etc/karl/certs/server.key
chmod 600 /etc/karl/certs/server.key
```

---

## 配置 TLS

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

### 环境变量

```bash
export KARL_API_TLS_ENABLED=true
export KARL_API_TLS_CERT=/etc/karl/certs/server.crt
export KARL_API_TLS_KEY=/etc/karl/certs/server.key
```

### 验证 TLS

```bash
# 测试 HTTPS 连接
curl -k https://localhost:8080/api/v1/stats

# 验证证书
openssl s_client -connect localhost:8080 -showcerts

# 不使用 -k（需要有效证书）
curl --cacert /etc/karl/certs/server.crt https://localhost:8080/api/v1/stats
```

---

## Kubernetes TLS

### 使用 Secret

```bash
# 创建 TLS Secret
kubectl create secret tls karl-tls \
  --cert=/path/to/server.crt \
  --key=/path/to/server.key
```

### 带 TLS 的部署

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

### 使用 cert-manager

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

### 带 TLS 的 Ingress

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

## 证书管理

### Let's Encrypt 自动续期

```bash
# 添加到 crontab
0 0 1 * * certbot renew --quiet && systemctl reload karl
```

### 证书到期监控

Prometheus 告警：

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
          summary: "Karl TLS 证书将在 30 天内过期"
```

### 重新加载证书

Karl 需要重启以重新加载证书：

```bash
# Systemd
sudo systemctl restart karl

# Docker
docker restart karl

# Kubernetes
kubectl rollout restart deployment karl
```

---

## 反向代理（替代方案）

使用 nginx 或 HAProxy 进行 TLS 终止：

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

## 故障排除

### 证书错误

```bash
# 检查证书有效期
openssl x509 -in /etc/karl/certs/server.crt -noout -dates

# 验证密钥是否与证书匹配
openssl x509 -noout -modulus -in server.crt | openssl md5
openssl rsa -noout -modulus -in server.key | openssl md5
# 两者应该匹配
```

### 权限问题

```bash
# 检查文件权限
ls -la /etc/karl/certs/

# 修复权限
sudo chown karl:karl /etc/karl/certs/server.key
sudo chmod 600 /etc/karl/certs/server.key
```

### 连接被拒绝

```bash
# 检查 Karl 是否在 HTTPS 上监听
ss -tlnp | grep 8080

# 检查日志中的 TLS 错误
journalctl -u karl | grep -i tls
```

---

## 下一步

- [配置参考](../configuration.md)
- [Kubernetes 部署](./deploying-kubernetes.md)
- [监控设置](./monitoring-prometheus.md)
