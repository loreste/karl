# 环境变量参考

Karl 媒体服务器支持的完整环境变量列表。

## 目录

- [核心配置](#核心配置)
- [网络配置](#网络配置)
- [会话配置](#会话配置)
- [录音配置](#录音配置)
- [数据库配置](#数据库配置)
- [监控配置](#监控配置)
- [WebRTC 配置](#webrtc-配置)
- [高级配置](#高级配置)

---

## 核心配置

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `KARL_CONFIG_PATH` | `./config/config.json` | JSON 配置文件路径 |
| `KARL_LOG_LEVEL` | `info` | 日志级别：`debug`、`info`、`warn`、`error` |
| `KARL_RUN_DIR` | `./run/karl` | 运行时目录，用于套接字和 PID 文件 |
| `KARL_ENVIRONMENT` | `production` | 环境名称：`production`、`staging`、`development` |

### 示例

```bash
# 使用自定义配置文件
export KARL_CONFIG_PATH=/etc/karl/config.json

# 启用调试日志
export KARL_LOG_LEVEL=debug

# 设置运行时目录
export KARL_RUN_DIR=/var/run/karl
```

---

## 网络配置

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `KARL_NG_PORT` | `22222` | NG 协议的 UDP 端口 |
| `KARL_API_PORT` | `:8080` | REST API 的 TCP 端口 |
| `KARL_HEALTH_PORT` | `:8086` | 健康端点的 TCP 端口 |
| `KARL_METRICS_PORT` | `:9091` | Prometheus 指标的 TCP 端口 |
| `KARL_MEDIA_IP` | `auto` | 媒体（SDP）的 IP 地址。使用 `auto` 自动检测 |
| `KARL_PUBLIC_IP` | （自动） | NAT 场景下的公网 IP |

### 示例

```bash
# 设置特定端口
export KARL_NG_PORT=22223
export KARL_API_PORT=:8081
export KARL_HEALTH_PORT=:8087
export KARL_METRICS_PORT=:9092

# 显式设置媒体 IP
export KARL_MEDIA_IP=192.168.1.100

# 设置 NAT 公网 IP
export KARL_PUBLIC_IP=203.0.113.50
```

---

## 会话配置

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `KARL_RTP_MIN_PORT` | `30000` | RTP 媒体最小端口 |
| `KARL_RTP_MAX_PORT` | `40000` | RTP 媒体最大端口 |
| `KARL_MAX_SESSIONS` | `10000` | 最大并发会话数 |
| `KARL_SESSION_TTL` | `3600` | 会话超时时间（秒） |
| `KARL_CLEANUP_INTERVAL` | `60` | 清理过期会话的间隔（秒） |

### 示例

```bash
# 限制 RTP 端口范围
export KARL_RTP_MIN_PORT=30000
export KARL_RTP_MAX_PORT=35000

# 增加会话容量
export KARL_MAX_SESSIONS=20000

# 缩短会话超时
export KARL_SESSION_TTL=1800
```

### 端口范围计算

每个会话需要 4 个端口（RTP + RTCP x 2 个方向）：
```
最大会话数 = (MAX_PORT - MIN_PORT + 1) / 4
```

默认值：(40000 - 30000 + 1) / 4 = 2,500 个会话

---

## 录音配置

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `KARL_RECORDING_ENABLED` | `false` | 启用/禁用通话录音 |
| `KARL_RECORDING_PATH` | `/var/lib/karl/recordings` | 录音文件目录 |
| `KARL_RECORDING_FORMAT` | `wav` | 输出格式：`wav`、`pcm` |
| `KARL_RECORDING_MODE` | `stereo` | 录音模式：`mixed`、`stereo`、`separate` |
| `KARL_RECORDING_SAMPLE_RATE` | `8000` | 采样率（Hz） |
| `KARL_RECORDING_RETENTION_DAYS` | `30` | 录音保留天数 |

### 示例

```bash
# 启用录音
export KARL_RECORDING_ENABLED=true
export KARL_RECORDING_PATH=/mnt/recordings

# 配置立体声录音
export KARL_RECORDING_MODE=stereo
export KARL_RECORDING_SAMPLE_RATE=16000

# 设置保留期为 7 天
export KARL_RECORDING_RETENTION_DAYS=7
```

---

## 数据库配置

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `KARL_MYSQL_DSN` | （空） | MySQL 连接字符串 |
| `KARL_REDIS_ENABLED` | `false` | 启用 Redis 进行会话共享 |
| `KARL_REDIS_ADDR` | `localhost:6379` | Redis 服务器地址 |
| `KARL_REDIS_PASSWORD` | （空） | Redis 密码 |
| `KARL_REDIS_DB` | `0` | Redis 数据库编号 |

### 示例

```bash
# MySQL 配置
export KARL_MYSQL_DSN="karl:password@tcp(mysql:3306)/karl?parseTime=true"

# Redis 集群配置
export KARL_REDIS_ENABLED=true
export KARL_REDIS_ADDR=redis.example.com:6379
export KARL_REDIS_PASSWORD=secretpassword
export KARL_REDIS_DB=0
```

### MySQL DSN 格式

```
user:password@tcp(host:port)/database?parseTime=true
```

---

## 监控配置

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `KARL_METRICS_ENABLED` | `true` | 启用 Prometheus 指标 |
| `KARL_HEALTH_CHECK_INTERVAL` | `30` | 健康检查间隔（秒） |

### 示例

```bash
# 禁用指标（不推荐）
export KARL_METRICS_ENABLED=false

# 加快健康检查频率
export KARL_HEALTH_CHECK_INTERVAL=15
```

---

## WebRTC 配置

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `KARL_WEBRTC_ENABLED` | `true` | 启用 WebRTC 支持 |
| `KARL_STUN_SERVERS` | `stun:stun.l.google.com:19302` | STUN 服务器（逗号分隔） |
| `KARL_TURN_SERVERS` | （空） | TURN 服务器（JSON 格式） |
| `KARL_WEBRTC_MAX_BITRATE` | `2000000` | 最大比特率（bps） |

### 示例

```bash
# 启用 WebRTC 并使用自定义 STUN
export KARL_WEBRTC_ENABLED=true
export KARL_STUN_SERVERS="stun:stun1.example.com:3478,stun:stun2.example.com:3478"

# 配置 TURN（JSON 格式）
export KARL_TURN_SERVERS='[{"url":"turn:turn.example.com:3478","username":"user","credential":"pass"}]'

# 限制比特率
export KARL_WEBRTC_MAX_BITRATE=1000000
```

---

## 高级配置

### 抖动缓冲区

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `KARL_JITTER_BUFFER_ENABLED` | `true` | 启用抖动缓冲区 |
| `KARL_JITTER_BUFFER_MIN_DELAY` | `20` | 最小延迟（毫秒） |
| `KARL_JITTER_BUFFER_MAX_DELAY` | `200` | 最大延迟（毫秒） |
| `KARL_JITTER_BUFFER_TARGET_DELAY` | `50` | 目标延迟（毫秒） |

### 前向纠错

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `KARL_FEC_ENABLED` | `true` | 启用 FEC |
| `KARL_FEC_REDUNDANCY` | `0.30` | 冗余比率（0.0-1.0） |
| `KARL_FEC_ADAPTIVE_MODE` | `true` | 启用自适应 FEC |

### API 安全

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `KARL_API_AUTH_ENABLED` | `false` | 启用 API 认证 |
| `KARL_API_RATE_LIMIT` | `60` | 每个客户端每分钟请求数 |
| `KARL_API_CORS_ENABLED` | `false` | 启用 CORS |
| `KARL_API_CORS_ORIGINS` | `*` | 允许的 CORS 源 |
| `KARL_API_TLS_ENABLED` | `false` | 启用 HTTPS |
| `KARL_API_TLS_CERT` | （空） | TLS 证书路径 |
| `KARL_API_TLS_KEY` | （空） | TLS 私钥路径 |

### 示例

```bash
# 为高延迟网络调优抖动缓冲区
export KARL_JITTER_BUFFER_MIN_DELAY=40
export KARL_JITTER_BUFFER_MAX_DELAY=300
export KARL_JITTER_BUFFER_TARGET_DELAY=100

# 为高丢包网络增加 FEC
export KARL_FEC_REDUNDANCY=0.50
export KARL_FEC_ADAPTIVE_MODE=true

# 启用 API 安全
export KARL_API_AUTH_ENABLED=true
export KARL_API_RATE_LIMIT=120
export KARL_API_TLS_ENABLED=true
export KARL_API_TLS_CERT=/etc/karl/certs/server.crt
export KARL_API_TLS_KEY=/etc/karl/certs/server.key
```

---

## 告警

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `KARL_ALERT_PACKET_LOSS_THRESHOLD` | `0.05` | 丢包率超过 5% 时告警 |
| `KARL_ALERT_JITTER_THRESHOLD` | `50` | 抖动超过 50ms 时告警 |
| `KARL_ALERT_NOTIFY_ADMIN` | `false` | 发送告警通知 |
| `KARL_ALERT_ADMIN_EMAIL` | （空） | 管理员告警邮箱 |
| `KARL_ALERT_SLACK_WEBHOOK` | （空） | Slack Webhook URL |

---

## 优先级顺序

环境变量会覆盖配置文件中的值：

1. 环境变量（最高优先级）
2. 配置文件
3. 默认值（最低优先级）

---

## Kubernetes ConfigMap 示例

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: karl-env
data:
  KARL_LOG_LEVEL: "info"
  KARL_NG_PORT: "22222"
  KARL_RTP_MIN_PORT: "30000"
  KARL_RTP_MAX_PORT: "40000"
  KARL_MAX_SESSIONS: "10000"
  KARL_REDIS_ENABLED: "true"
  KARL_REDIS_ADDR: "redis:6379"
  KARL_RECORDING_ENABLED: "true"
  KARL_RECORDING_PATH: "/var/lib/karl/recordings"
```

---

## Docker Compose 示例

```yaml
services:
  karl:
    image: loreste/karl:latest
    environment:
      - KARL_LOG_LEVEL=info
      - KARL_NG_PORT=22222
      - KARL_RTP_MIN_PORT=30000
      - KARL_RTP_MAX_PORT=40000
      - KARL_REDIS_ENABLED=true
      - KARL_REDIS_ADDR=redis:6379
      - KARL_RECORDING_ENABLED=true
```

---

## 下一步

- [配置参考](../configuration.md)
- [安装指南](../installation.md)
- [Kubernetes 部署](../how-to/deploying-kubernetes.md)
