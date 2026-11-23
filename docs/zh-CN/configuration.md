# 配置参考

Karl 使用 JSON 配置文件，并提供合理的默认值。所有设置均可通过环境变量覆盖。

## 目录

- [配置文件位置](#配置文件位置)
- [完整配置示例](#完整配置示例)
- [配置章节](#配置章节)
  - [NG 协议](#ng-协议)
  - [会话](#会话)
  - [抖动缓冲区](#抖动缓冲区)
  - [RTCP](#rtcp)
  - [前向纠错](#前向纠错)
  - [录音](#录音)
  - [REST API](#rest-api)
  - [WebRTC](#webrtc)
  - [集成](#集成)
  - [数据库](#数据库)
  - [SRTP](#srtp)
  - [告警](#告警)
  - [HEP / Homer](#hep--homer)
  - [集群](#集群)
- [环境变量](#环境变量)

---

## 配置文件位置

Karl 按以下顺序查找配置文件：

1. 通过 `-config` 参数指定的路径：`./karl -config /path/to/config.json`
2. 通过 `KARL_CONFIG_PATH` 环境变量指定的路径
3. 默认路径：`./config/config.json`

## 动态配置更新

Karl 会监视配置文件的变更，并自动重新加载某些设置，无需重启。

**立即生效的设置：**
- WebRTC 设置（STUN/TURN 服务器、录音路径）
- 录音设置（保留期限、格式）
- 告警阈值
- 集成设置（SIP 代理注册）
- RTP 质量设置（应用于新会话）

**需要重启的设置：**
- 传输设置（UDP/TCP/TLS 端口和监听器）
- NG 协议端口
- 会话端口范围
- 数据库连接设置

当检测到配置变更时，Karl 会将应用的更改记录到日志中。请检查日志以验证更新是否生效。

---

## 完整配置示例

```json
{
  "version": "1.0.0",
  "environment": "production",

  "transport": {
    "udp_enabled": true,
    "udp_port": 12000,
    "tcp_enabled": false,
    "tcp_port": 12001,
    "tls_enabled": false,
    "tls_port": 12002,
    "tls_cert": "/etc/karl/certs/server.crt",
    "tls_key": "/etc/karl/certs/server.key",
    "ipv6_enabled": false,
    "mtu": 1500
  },

  "ng_protocol": {
    "enabled": true,
    "socket_path": "/var/run/karl/karl.sock",
    "udp_port": 22222,
    "timeout": 30
  },

  "sessions": {
    "max_sessions": 10000,
    "session_ttl": 3600,
    "cleanup_interval": 60,
    "min_port": 30000,
    "max_port": 40000
  },

  "jitter_buffer": {
    "enabled": true,
    "min_delay": 20,
    "max_delay": 200,
    "target_delay": 50,
    "adaptive_mode": true,
    "max_size": 100
  },

  "rtcp": {
    "enabled": true,
    "interval": 5,
    "reduced_size": false,
    "mux_enabled": true
  },

  "fec": {
    "enabled": true,
    "block_size": 48,
    "redundancy": 0.30,
    "adaptive_mode": true,
    "max_redundancy": 0.50,
    "min_redundancy": 0.10
  },

  "recording": {
    "enabled": true,
    "base_path": "/var/lib/karl/recordings",
    "format": "wav",
    "mode": "stereo",
    "sample_rate": 8000,
    "bits_per_sample": 16,
    "max_file_size": 104857600,
    "retention_days": 30
  },

  "api": {
    "enabled": true,
    "address": ":8080",
    "auth_enabled": false,
    "rate_limit_per_min": 60,
    "cors_enabled": false,
    "cors_origins": "*",
    "tls_enabled": false,
    "tls_cert": "",
    "tls_key": ""
  },

  "webrtc": {
    "enabled": true,
    "webrtc_port": 8443,
    "stun_servers": [
      "stun:stun.l.google.com:19302",
      "stun:stun1.l.google.com:19302"
    ],
    "turn_servers": [],
    "max_bitrate": 2000000,
    "start_bitrate": 1000000,
    "bw_estimation": true,
    "tcc_enabled": true
  },

  "integration": {
    "opensips_ip": "",
    "opensips_port": 0,
    "kamailio_ip": "",
    "kamailio_port": 0,
    "media_ip": "auto",
    "public_ip": "",
    "keepalive_interval": 30,
    "interfaces": {
      "internal": {
        "name": "internal",
        "address": "10.0.0.5",
        "advertise_addr": "10.0.0.5"
      },
      "external": {
        "name": "external",
        "address": "10.0.0.5",
        "advertise_addr": "198.51.100.10"
      }
    }
  },

  "database": {
    "mysql_dsn": "",
    "redis_enabled": false,
    "redis_addr": "localhost:6379",
    "redis_cleanup_interval": 3600,
    "max_connections": 10,
    "connection_timeout": 30
  },

  "srtp": {
    "srtp_key": "",
    "srtp_salt": ""
  },

  "alert_settings": {
    "packet_loss_threshold": 0.05,
    "jitter_threshold": 50.0,
    "bandwidth_threshold": 1000000,
    "notify_admin": false,
    "admin_email": "",
    "slack_webhook": ""
  },

  "hep": {
    "hep_enabled": false,
    "hep_server": "homer:9060",
    "hep_capture_id": 1,
    "hep_proto": "udp"
  },

  "cluster": {
    "enabled": false,
    "node_id": "karl-1",
    "node_address": "10.0.0.5:22222",
    "quorum_size": 2,
    "replication_factor": 2,
    "enable_fencing": false
  }
}
```

---

## 配置章节

### NG 协议

控制兼容 rtpengine 的 NG 协议接口。

```json
{
  "ng_protocol": {
    "enabled": true,
    "socket_path": "/var/run/karl/karl.sock",
    "udp_port": 22222,
    "timeout": 30
  }
}
```

| 设置 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | `true` | 启用 NG 协议监听器 |
| `socket_path` | string | `/var/run/karl/karl.sock` | 用于本地通信的 Unix 套接字路径 |
| `udp_port` | int | `22222` | NG 协议的 UDP 端口 |
| `timeout` | int | `30` | 请求超时时间（秒） |

### 会话

控制会话管理和 RTP 端口分配。

```json
{
  "sessions": {
    "max_sessions": 10000,
    "session_ttl": 3600,
    "cleanup_interval": 60,
    "min_port": 30000,
    "max_port": 40000
  }
}
```

| 设置 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `max_sessions` | int | `10000` | 最大并发会话数 |
| `session_ttl` | int | `3600` | 会话存活时间（秒） |
| `cleanup_interval` | int | `60` | 清理过期会话的间隔时间（秒） |
| `min_port` | int | `30000` | 最小 RTP 端口号 |
| `max_port` | int | `40000` | 最大 RTP 端口号 |

**端口范围计算：**

每个会话需要 4 个端口（每个方向各需 RTP + RTCP）。使用默认范围 30000-40000：
- 可用端口：10,000
- 最大会话数：2,500

请根据预期的并发通话量调整 `min_port` 和 `max_port`。

### 抖动缓冲区

控制自适应抖动缓冲区，用于平滑音频播放。

```json
{
  "jitter_buffer": {
    "enabled": true,
    "min_delay": 20,
    "max_delay": 200,
    "target_delay": 50,
    "adaptive_mode": true,
    "max_size": 100
  }
}
```

| 设置 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | `true` | 启用抖动缓冲区 |
| `min_delay` | int | `20` | 最小缓冲延迟（毫秒） |
| `max_delay` | int | `200` | 最大缓冲延迟（毫秒） |
| `target_delay` | int | `50` | 自适应模式的目标延迟（毫秒） |
| `adaptive_mode` | bool | `true` | 根据网络状况自动调整缓冲区大小 |
| `max_size` | int | `100` | 最大缓冲区大小（数据包数） |

**调优指南：**

- **低延迟网络**：`min_delay: 10`，`target_delay: 30`
- **标准网络**：`min_delay: 20`，`target_delay: 50`（默认值）
- **高抖动网络**：`min_delay: 40`，`target_delay: 100`

### RTCP

控制 RTCP（RTP 控制协议），用于质量监控。

```json
{
  "rtcp": {
    "enabled": true,
    "interval": 5,
    "reduced_size": false,
    "mux_enabled": true
  }
}
```

| 设置 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | `true` | 启用 RTCP 处理 |
| `interval` | int | `5` | RTCP 报告间隔（秒） |
| `reduced_size` | bool | `false` | 使用精简 RTCP（RFC 5506） |
| `mux_enabled` | bool | `true` | 启用 RTCP-mux（RTP 和 RTCP 共用同一端口） |

### 前向纠错

控制 FEC，用于丢包恢复。

```json
{
  "fec": {
    "enabled": true,
    "block_size": 48,
    "redundancy": 0.30,
    "adaptive_mode": true,
    "max_redundancy": 0.50,
    "min_redundancy": 0.10
  }
}
```

| 设置 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | `true` | 启用前向纠错 |
| `block_size` | int | `48` | 每个 FEC 块的数据包数 |
| `redundancy` | float | `0.30` | 基础冗余比率（0.0-1.0） |
| `adaptive_mode` | bool | `true` | 根据丢包率调整冗余度 |
| `max_redundancy` | float | `0.50` | 自适应时的最大冗余度 |
| `min_redundancy` | float | `0.10` | 自适应时的最小冗余度 |

**冗余度影响：**

| 冗余度 | 带宽开销 | 恢复能力 |
|--------|----------|----------|
| 0.10（10%） | 低 | 最高可恢复约 8% 丢包 |
| 0.30（30%） | 中 | 最高可恢复约 20% 丢包 |
| 0.50（50%） | 高 | 最高可恢复约 35% 丢包 |

### 录音

控制通话录音功能。

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

| 设置 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | `false` | 启用录音系统 |
| `base_path` | string | `/var/lib/karl/recordings` | 录音文件目录 |
| `format` | string | `wav` | 输出格式：`wav` 或 `pcm` |
| `mode` | string | `stereo` | 录音模式：`mixed`、`stereo` 或 `separate` |
| `sample_rate` | int | `8000` | 采样率（Hz）（8000、16000、48000） |
| `bits_per_sample` | int | `16` | 每样本位数（8 或 16） |
| `max_file_size` | int | `104857600` | 文件轮转前的最大大小（字节） |
| `retention_days` | int | `30` | 录音保留天数 |

**录音模式：**

| 模式 | 说明 | 生成的文件数 |
|------|------|-------------|
| `mixed` | 双方混合为单声道 | 1 个文件 |
| `stereo` | 主叫在左声道，被叫在右声道 | 1 个文件 |
| `separate` | 每方各自独立文件 | 2 个文件 |

**存储空间计算：**

WAV 格式 8kHz/16-bit：每通道每分钟约 1 MB
- 混合模式：约 1 MB/分钟
- 立体声模式：约 2 MB/分钟
- 分离模式：约 2 MB/分钟（2 个文件）

### REST API

控制 REST API 服务器。

```json
{
  "api": {
    "enabled": true,
    "address": ":8080",
    "auth_enabled": false,
    "rate_limit_per_min": 60,
    "cors_enabled": false,
    "cors_origins": "*",
    "tls_enabled": false,
    "tls_cert": "",
    "tls_key": ""
  }
}
```

| 设置 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | `true` | 启用 REST API |
| `address` | string | `:8080` | 监听地址和端口 |
| `auth_enabled` | bool | `false` | 要求 API 密钥认证 |
| `rate_limit_per_min` | int | `60` | 每个客户端每分钟最大请求数 |
| `cors_enabled` | bool | `false` | 启用 CORS 头 |
| `cors_origins` | string | `*` | 允许的 CORS 源 |
| `tls_enabled` | bool | `false` | 启用 HTTPS |
| `tls_cert` | string | | TLS 证书路径 |
| `tls_key` | string | | TLS 私钥路径 |

### WebRTC

控制基于浏览器客户端的 WebRTC 功能。

```json
{
  "webrtc": {
    "enabled": true,
    "webrtc_port": 8443,
    "stun_servers": [
      "stun:stun.l.google.com:19302"
    ],
    "turn_servers": [],
    "max_bitrate": 2000000,
    "start_bitrate": 1000000,
    "bw_estimation": true,
    "tcc_enabled": true
  }
}
```

| 设置 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | `true` | 启用 WebRTC 支持 |
| `webrtc_port` | int | `8443` | WebRTC 信令端口 |
| `stun_servers` | array | Google STUN | 用于 NAT 穿越的 STUN 服务器 |
| `turn_servers` | array | `[]` | 用于中继的 TURN 服务器 |
| `max_bitrate` | int | `2000000` | 最大比特率（bps） |
| `start_bitrate` | int | `1000000` | 初始比特率（bps） |
| `bw_estimation` | bool | `true` | 启用带宽估算 |
| `tcc_enabled` | bool | `true` | 启用 Transport-CC 反馈 |

### 集成

控制与 SIP 代理的集成以及网络接口选择。

```json
{
  "integration": {
    "opensips_ip": "",
    "opensips_port": 0,
    "kamailio_ip": "",
    "kamailio_port": 0,
    "media_ip": "10.0.0.5",
    "public_ip": "198.51.100.10",
    "keepalive_interval": 30,
    "interfaces": {
      "internal": {
        "name": "internal",
        "address": "10.0.0.5",
        "advertise_addr": "10.0.0.5"
      },
      "external": {
        "name": "external",
        "address": "10.0.0.5",
        "advertise_addr": "198.51.100.10"
      }
    }
  }
}
```

| 设置 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `opensips_ip` | string | | OpenSIPS 服务器 IP（用于注册） |
| `opensips_port` | int | | OpenSIPS 服务器端口 |
| `kamailio_ip` | string | | Kamailio 服务器 IP（用于注册） |
| `kamailio_port` | int | | Kamailio 服务器端口 |
| `media_ip` | string | `auto` | 媒体套接字绑定的 IP 地址。使用 `auto` 自动检测 |
| `public_ip` | string | | NAT 场景下的公网 IP（当未设置每接口 `advertise_addr` 时用于 SDP） |
| `keepalive_interval` | int | `30` | 向 SIP 代理发送心跳的间隔（秒） |
| `interfaces` | map | | 命名网络接口，用于基于方向的媒体路由（见下文） |

#### 命名接口

命名接口允许 Karl 根据通话方向在 SDP 中公布不同的 IP。当您的媒体服务器有独立的内部和外部网络路径时（例如，PBX 使用私有 IP，运营商使用弹性公网 IP），这一功能非常重要。

每个接口条目包含：

| 设置 | 类型 | 说明 |
|------|------|------|
| `name` | string | 接口标识符（如 `internal`、`external`） |
| `address` | string | 套接字绑定的本地 IP |
| `advertise_addr` | string | 放入 SDP `c=` 行和流信息中的 IP。如果为空，回退到 `address` |
| `port` | int | 可选的端口覆盖 |

**方向如何选择接口：**

SIP 代理在 NG 协议请求中发送 `direction` 数组（例如 `["internal", "external"]`）。Karl 使用此数组选择正确的接口：

- `direction[0]` = 发起方（"from"）
- `direction[1]` = 终止方（"to"）

对于 **offer**（主叫方 SDP），Karl 使用 `direction[0]` 选择接口。对于 **answer**（被叫方 SDP），Karl 使用 `direction[1]`。所选接口的 `advertise_addr` 会写入 SDP，而 `address` 用于实际的套接字绑定。

**示例 — 运营商获取公网 IP，内部端点获取私有 IP：**

```json
{
  "integration": {
    "media_ip": "10.0.0.5",
    "interfaces": {
      "internal": {
        "address": "10.0.0.5",
        "advertise_addr": "10.0.0.5"
      },
      "external": {
        "address": "10.0.0.5",
        "advertise_addr": "198.51.100.10"
      }
    }
  }
}
```

然后在 SIP 代理中，对于发往外部运营商的呼叫：
```
rtpengine_manage("... direction=internal direction=external");
```

Karl 会在 `10.0.0.5` 上绑定套接字，但在面向运营商的呼叫腿 SDP 中放入 `198.51.100.10`。

如果未配置 `interfaces`，Karl 回退到使用 `media_ip` 进行绑定，使用 `public_ip` 进行 SDP 公布（如果 `public_ip` 为空，则使用 `media_ip`）。

### 数据库

控制 CDR 和会话存储的数据库连接。

```json
{
  "database": {
    "mysql_dsn": "",
    "redis_enabled": false,
    "redis_addr": "localhost:6379",
    "redis_cleanup_interval": 3600,
    "max_connections": 10,
    "connection_timeout": 30
  }
}
```

| 设置 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `mysql_dsn` | string | | MySQL 连接字符串 |
| `redis_enabled` | bool | `false` | 启用 Redis 进行会话缓存 |
| `redis_addr` | string | `localhost:6379` | Redis 服务器地址 |
| `redis_cleanup_interval` | int | `3600` | Redis 清理间隔（秒） |
| `max_connections` | int | `10` | 最大数据库连接数 |
| `connection_timeout` | int | `30` | 连接超时时间（秒） |

**MySQL DSN 格式：**

```
user:password@tcp(host:port)/database?parseTime=true
```

示例：
```
karl:secretpassword@tcp(127.0.0.1:3306)/karl?parseTime=true
```

### SRTP

控制 SRTP 加密设置。

```json
{
  "srtp": {
    "srtp_key": "",
    "srtp_salt": ""
  }
}
```

| 设置 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `srtp_key` | string | | SRTP 主密钥（base64 编码） |
| `srtp_salt` | string | | SRTP 主盐值（base64 编码） |

### 告警

控制质量告警阈值。

```json
{
  "alert_settings": {
    "packet_loss_threshold": 0.05,
    "jitter_threshold": 50.0,
    "bandwidth_threshold": 1000000,
    "notify_admin": false,
    "admin_email": "",
    "slack_webhook": ""
  }
}
```

| 设置 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `packet_loss_threshold` | float | `0.05` | 丢包率超过 5% 时告警 |
| `jitter_threshold` | float | `50.0` | 抖动超过 50ms 时告警 |
| `bandwidth_threshold` | int | `1000000` | 带宽超过阈值时告警 |
| `notify_admin` | bool | `false` | 发送告警通知 |
| `admin_email` | string | | 告警邮箱地址 |
| `slack_webhook` | string | | Slack Webhook URL |

### HEP / Homer

控制 HEPv3 导出器，用于向 Homer 抓包服务器发送 SIP 和 RTCP 数据包。

```json
{
  "hep": {
    "hep_enabled": true,
    "hep_server": "homer.example.com:9060",
    "hep_capture_id": 1,
    "hep_proto": "udp"
  }
}
```

| 设置 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `hep_enabled` | bool | `false` | 启用 HEP 导出器 |
| `hep_server` | string | | Homer 抓包服务器地址（host:port） |
| `hep_capture_id` | int | `0` | HEP 数据包中的抓包代理 ID |
| `hep_proto` | string | `udp` | 到 Homer 的传输协议（`udp` 或 `tcp`） |

### 集群

控制多节点集群，支持 Redis 支撑的会话故障转移、脑裂检测和一致性哈希。

```json
{
  "cluster": {
    "enabled": true,
    "node_id": "karl-1",
    "node_address": "10.0.0.5:22222",
    "quorum_size": 2,
    "replication_factor": 2,
    "enable_fencing": true
  }
}
```

| 设置 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | `false` | 启用集群 |
| `node_id` | string | | 本节点的唯一标识符 |
| `node_address` | string | | 本节点的地址（其他节点可达） |
| `quorum_size` | int | `2` | 仲裁所需的最小节点数 |
| `replication_factor` | int | `2` | 哈希环上的会话复制因子 |
| `enable_fencing` | bool | `false` | 在分区检测时启用节点隔离 |

**前提条件**：必须在 `database` 配置节中启用 Redis。集群管理器使用 Redis 进行会话存储、发布/订阅协调、心跳和脑裂检测。

**故障转移工作原理**：
1. 每个节点每秒向 Redis 发送心跳
2. 如果节点连续丢失 3 次心跳，则标记为故障
3. 脑裂检测器在允许接管之前检查仲裁
4. 存活节点通过一致性哈希重新分配故障节点的会话
5. 在新的所有者上重新分配端口并重启媒体中继

---

## 环境变量

所有配置选项均可通过环境变量覆盖：

| 变量 | 配置路径 | 说明 |
|------|----------|------|
| `KARL_CONFIG_PATH` | - | 配置文件路径 |
| `KARL_LOG_LEVEL` | - | 日志级别（debug、info、warn、error） |
| `KARL_HEALTH_PORT` | - | 健康检查端口（默认：`:8086`） |
| `KARL_METRICS_PORT` | - | Prometheus 指标端口（默认：`:9091`） |
| `KARL_API_PORT` | `api.address` | REST API 端口 |
| `KARL_NG_PORT` | `ng_protocol.udp_port` | NG 协议 UDP 端口 |
| `KARL_RTP_MIN_PORT` | `sessions.min_port` | 最小 RTP 端口 |
| `KARL_RTP_MAX_PORT` | `sessions.max_port` | 最大 RTP 端口 |
| `KARL_MAX_SESSIONS` | `sessions.max_sessions` | 最大并发会话数 |
| `KARL_RECORDING_PATH` | `recording.base_path` | 录音存储路径 |
| `KARL_RECORDING_ENABLED` | `recording.enabled` | 启用录音 |
| `KARL_MYSQL_DSN` | `database.mysql_dsn` | MySQL 连接字符串 |
| `KARL_REDIS_ENABLED` | `database.redis_enabled` | 启用 Redis |
| `KARL_REDIS_ADDR` | `database.redis_addr` | Redis 地址 |
| `KARL_MEDIA_IP` | `integration.media_ip` | 媒体 IP 地址 |
| `KARL_PUBLIC_IP` | `integration.public_ip` | 公网 IP 地址 |
| `KARL_RUN_DIR` | - | 运行时目录 |

环境变量优先于配置文件中的值。

---

## 下一步

- [快速入门](./getting-started.md)
- [Kubernetes 部署](./how-to/deploying-kubernetes.md)
- [监控设置](./how-to/monitoring-prometheus.md)
