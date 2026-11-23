# 安装指南

本指南涵盖 Karl 媒体服务器的所有安装方式。

## 目录

- [系统要求](#系统要求)
- [从源码安装](#从源码安装)
- [Docker](#docker)
- [Docker Compose](#docker-compose)
- [Kubernetes](#kubernetes)
- [Systemd 服务](#systemd-服务)
- [从源码构建](#从源码构建)

---

## 系统要求

### 最低要求

| 资源 | 最低配置 | 推荐配置 |
|------|----------|----------|
| CPU | 2 核 | 4+ 核 |
| 内存 | 1 GB | 4+ GB |
| 磁盘 | 100 MB | + 录音存储空间 |
| 网络 | 100 Mbps | 1 Gbps |
| 操作系统 | Linux（内核 4.x+） | Linux（内核 5.x+） |

### 支持的平台

- Linux（amd64、arm64）
- macOS（amd64、arm64）- 仅用于开发
- Windows（amd64）- 仅用于开发

### 网络要求

| 端口 | 协议 | 用途 |
|------|------|------|
| 22222 | UDP | NG 协议（SIP 代理通信） |
| 30000-40000 | UDP | RTP/RTCP 媒体（端口范围可配置） |
| 8080 | TCP | REST API |
| 8086 | TCP | 健康检查 |
| 9091 | TCP | Prometheus 指标 |

---

## 从源码安装

### 前提条件

- Go 1.26 或更高版本
- Git

### 步骤

```bash
# 克隆仓库
git clone https://github.com/loreste/karl.git
cd karl

# 下载依赖
go mod download

# 构建
go build -o karl

# 验证构建
./karl --version
```

### 系统级安装

```bash
# 复制二进制文件
sudo cp karl /usr/local/bin/

# 创建目录
sudo mkdir -p /etc/karl
sudo mkdir -p /var/lib/karl/recordings
sudo mkdir -p /var/run/karl

# 创建用户
sudo useradd -r -s /bin/false karl

# 设置权限
sudo chown -R karl:karl /var/lib/karl
sudo chown -R karl:karl /var/run/karl

# 复制配置文件
sudo cp config/config.json /etc/karl/
sudo chown karl:karl /etc/karl/config.json
```

---

## Docker

### 快速开始

```bash
# 拉取最新镜像
docker pull loreste/karl:latest

# 使用 host 网络运行（推荐）
docker run -d \
  --name karl \
  --network host \
  --restart unless-stopped \
  loreste/karl:latest
```

### 自定义配置

```bash
# 创建配置目录
mkdir -p /opt/karl/config
mkdir -p /opt/karl/recordings

# 复制您的配置文件
cp config.json /opt/karl/config/

# 使用卷挂载运行
docker run -d \
  --name karl \
  --network host \
  -v /opt/karl/config:/etc/karl:ro \
  -v /opt/karl/recordings:/var/lib/karl/recordings \
  -e KARL_CONFIG_PATH=/etc/karl/config.json \
  --restart unless-stopped \
  loreste/karl:latest
```

### 使用端口映射（限制 RTP 范围）

```bash
docker run -d \
  --name karl \
  -p 22222:22222/udp \
  -p 30000-30100:30000-30100/udp \
  -p 8080:8080 \
  -p 8086:8086 \
  -p 9091:9091 \
  -v /opt/karl/config:/etc/karl:ro \
  -v /opt/karl/recordings:/var/lib/karl/recordings \
  --restart unless-stopped \
  loreste/karl:latest
```

### 构建 Docker 镜像

```bash
# 克隆仓库
git clone https://github.com/loreste/karl.git
cd karl

# 构建镜像
docker build -t karl:local .

# 运行本地镜像
docker run -d --name karl --network host karl:local
```

---

## Docker Compose

### 基本设置

创建 `docker-compose.yml`：

```yaml
version: '3.8'

services:
  karl:
    image: loreste/karl:latest
    network_mode: host
    volumes:
      - ./config.json:/etc/karl/config.json:ro
      - recordings:/var/lib/karl/recordings
    environment:
      - KARL_CONFIG_PATH=/etc/karl/config.json
      - KARL_LOG_LEVEL=info
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8086/health"]
      interval: 30s
      timeout: 10s
      retries: 3

volumes:
  recordings:
```

### 完整堆栈（含 Redis 和 MySQL）

```yaml
version: '3.8'

services:
  karl:
    image: loreste/karl:latest
    network_mode: host
    volumes:
      - ./config.json:/etc/karl/config.json:ro
      - recordings:/var/lib/karl/recordings
    environment:
      - KARL_CONFIG_PATH=/etc/karl/config.json
      - KARL_MYSQL_DSN=karl:password@tcp(127.0.0.1:3306)/karl
      - KARL_REDIS_ENABLED=true
      - KARL_REDIS_ADDR=127.0.0.1:6379
    depends_on:
      mysql:
        condition: service_healthy
      redis:
        condition: service_healthy
    restart: unless-stopped

  mysql:
    image: mariadb:10.11
    environment:
      MYSQL_ROOT_PASSWORD: rootpassword
      MYSQL_DATABASE: karl
      MYSQL_USER: karl
      MYSQL_PASSWORD: password
    volumes:
      - mysql_data:/var/lib/mysql
    ports:
      - "3306:3306"
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "localhost"]
      interval: 10s
      timeout: 5s
      retries: 5
    restart: unless-stopped

  redis:
    image: redis:7-alpine
    volumes:
      - redis_data:/data
    ports:
      - "6379:6379"
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5
    restart: unless-stopped

  prometheus:
    image: prom/prometheus:latest
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml:ro
      - prometheus_data:/prometheus
    ports:
      - "9090:9090"
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.path=/prometheus'
    restart: unless-stopped

  grafana:
    image: grafana/grafana:latest
    volumes:
      - grafana_data:/var/lib/grafana
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
    restart: unless-stopped

volumes:
  recordings:
  mysql_data:
  redis_data:
  prometheus_data:
  grafana_data:
```

### 启动堆栈

```bash
# 启动所有服务
docker-compose up -d

# 查看日志
docker-compose logs -f karl

# 停止所有服务
docker-compose down
```

---

## Kubernetes

完整说明请参阅专门的 [Kubernetes 部署指南](./how-to/deploying-kubernetes.md)。

### 快速部署

```bash
# 克隆仓库
git clone https://github.com/loreste/karl.git
cd karl

# 使用 kustomize 部署
kubectl apply -k deploy/kubernetes/

# 检查状态
kubectl get pods -l app=karl
```

---

## Systemd 服务

### 创建服务文件

创建 `/etc/systemd/system/karl.service`：

```ini
[Unit]
Description=Karl Media Server
Documentation=https://github.com/loreste/karl
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=karl
Group=karl
ExecStart=/usr/local/bin/karl -config /etc/karl/config.json
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

# 安全加固
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/karl /var/run/karl
PrivateTmp=true

# 资源限制
LimitNOFILE=65535
LimitNPROC=4096

[Install]
WantedBy=multi-user.target
```

### 启用并启动

```bash
# 重新加载 systemd
sudo systemctl daemon-reload

# 启用服务
sudo systemctl enable karl

# 启动服务
sudo systemctl start karl

# 检查状态
sudo systemctl status karl

# 查看日志
sudo journalctl -u karl -f
```

---

## 从源码构建

### 开发构建

```bash
# 克隆
git clone https://github.com/loreste/karl.git
cd karl

# 安装依赖
go mod download

# 运行测试
go test ./...

# 带调试符号构建
go build -o karl

# 运行
./karl
```

### 生产构建

```bash
# 构建优化的二进制文件
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -ldflags="-s -w" \
  -o karl

# 验证二进制文件
file karl
```

### 交叉编译

```bash
# Linux ARM64
GOOS=linux GOARCH=arm64 go build -o karl-linux-arm64

# macOS ARM64
GOOS=darwin GOARCH=arm64 go build -o karl-darwin-arm64

# Windows
GOOS=windows GOARCH=amd64 go build -o karl.exe
```

---

## 验证安装

无论使用哪种方式安装 Karl：

### 1. 检查进程

```bash
# 使用 systemctl
sudo systemctl status karl

# 使用 Docker
docker ps | grep karl

# 使用 ps
ps aux | grep karl
```

### 2. 测试 NG 协议

```bash
echo -n "d7:command4:pinge" | nc -u localhost 22222
```

### 3. 检查健康状态

```bash
curl http://localhost:8086/health
```

### 4. 查看指标

```bash
curl http://localhost:9091/metrics | head
```

---

## 下一步

- [配置参考](./configuration.md)
- [与 OpenSIPS 集成](./how-to/integrating-opensips.md)
- [与 Kamailio 集成](./how-to/integrating-kamailio.md)
- [设置监控](./how-to/monitoring-prometheus.md)
