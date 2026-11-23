[English](README.md) | **中文** | [Français](README.fr.md) | [Español](README.es.md)

# Karl Media Server

## 开源 RTP 代理与媒体服务器，适用于 VoIP 和 WebRTC

[![Go Report Card](https://goreportcard.com/badge/github.com/loreste/karl)](https://goreportcard.com/report/github.com/loreste/karl)
[![Go Reference](https://pkg.go.dev/badge/github.com/loreste/karl.svg)](https://pkg.go.dev/github.com/loreste/karl)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL%203.0-blue.svg)](https://opensource.org/licenses/GPL-3.0)

Karl 是一个高性能、云原生的 RTP 媒体代理和媒体服务器，使用 Go 语言编写。它是 rtpengine 的现代化替代方案，专为云部署、Kubernetes 环境和现代 VoIP 基础设施而设计。

**核心亮点：**
- 单一二进制文件部署，零内核依赖
- 完全兼容 rtpengine NG protocol
- 原生支持 Kubernetes 健康探针
- 开箱即用的 WebRTC 到 SIP 桥接
- Prometheus 指标和生产级可观测性

---

## 目录

- [我们解决的问题](#我们解决的问题)
- [为什么选择 Karl](#为什么选择-karl)
- [功能特性](#功能特性)
- [快速开始](#快速开始)
- [文档](#文档)
- [安装](#安装)
- [配置](#配置)
- [Kubernetes 部署](#kubernetes-部署)
- [与 SIP 代理集成](#与-sip-代理集成)
- [API 参考](#api-参考)
- [监控与可观测性](#监控与可观测性)
- [性能](#性能)
- [架构](#架构)
- [路线图](#路线图)
- [贡献](#贡献)
- [许可证](#许可证)

---

## 我们解决的问题

### 传统 RTP 代理面临的挑战

在生产环境中运行 VoIP 媒体服务器一直是一件痛苦的事情。如果你曾经运维过 rtpengine 或类似的 RTP 代理，你很可能遇到过以下挑战：

**内核模块依赖**
传统 RTP 代理需要内核模块来保证性能。这些模块在每次内核更新时都会失效，需要重新编译，并且与容器化环境不兼容。在 EKS、GKE 或 AKS 等托管 Kubernetes 服务上运行它们几乎不可能，或者需要具有主机访问权限的特权容器。

**部署复杂**
安装 rtpengine 意味着管理多个仓库、编译内核模块、解决 C 库依赖以及维护复杂的构建流水线。一次简单的升级可能需要数小时的停机时间。

**云兼容性有限**
大多数 RTP 代理是为裸机服务器设计的。它们在以下方面表现不佳：
- 云环境中的动态 IP 地址
- 自动伸缩组和容器编排
- VPC 网络中的 NAT 穿透
- 负载均衡器健康检查

**可观测性差**
在生产环境中调试媒体质量问题需要对丢包、抖动和编解码器性能有深入的洞察。传统工具提供的指标有限，且不支持与 Prometheus 和 Grafana 等现代可观测性栈的原生集成。

**WebRTC 被视为附加功能**
随着 WebRTC 的普及，将基于浏览器的通话桥接到传统 SIP 基础设施需要复杂的配置，通常还需要单独的组件。

### Karl 如何解决这些问题

Karl 从零开始构建，以解决上述每一个挑战：

| 挑战 | 传统 RTP 代理 | Karl |
|------|--------------|------|
| 内核模块 | 必需，更新后易失效 | 无需内核模块 |
| 部署 | 数小时的安装配置 | 单一二进制文件，秒级部署 |
| 云/Kubernetes | 需要变通方案 | 原生支持 |
| 可观测性 | 有限 | 50+ Prometheus 指标 |
| WebRTC | 单独组件 | 内置支持 |
| 容器支持 | 困难 | 一等公民 |

---

## 为什么选择 Karl

### 零内核依赖

Karl 完全运行在用户空间。无需编译内核模块，无需特权容器，不存在与托管 Kubernetes 服务的兼容性问题。可在任何 Linux 发行版、任何云服务商、任何容器编排器上部署。

```bash
# 这就是全部的安装过程
go build -o karl && ./karl
```

### 完全兼容 rtpengine

Karl 实现了 rtpengine 使用的完整 NG (Next Generation) protocol。你现有的 OpenSIPS 或 Kamailio 配置无需修改即可使用。无需更改 SIP 代理，无迁移复杂度。

### 云原生架构

专为现代基础设施构建：
- **Kubernetes 原生健康探针**：Startup、Liveness 和 Readiness 端点
- **环境变量配置**：轻松与 ConfigMaps 和 Secrets 集成
- **水平扩展**：基于 Redis 的会话共享，支持多实例部署
- **优雅关闭**：正确的 SIGTERM 处理，实现零停机部署

### 生产级可观测性

精确了解你的媒体基础设施状态：
- **50+ Prometheus 指标**，覆盖会话、RTCP 质量、FEC 恢复和 API 性能
- **结构化 JSON 日志**，可轻松导入 ELK、Splunk 或 CloudWatch
- **实时质量指标**，包括每通电话的 MOS 评分、抖动和丢包率
- **通话详单 (CDR)**，附带完整的质量统计数据

### 原生 WebRTC 支持

WebRTC 不是后期附加的功能——它是核心特性：
- **ICE/STUN/TURN** 支持 NAT 穿透
- **DTLS-SRTP** 加密，桥接 WebRTC 和 SIP
- **Opus 编解码器转码**，支持与 G.711 双向转换
- **带宽估计**，支持 Transport-CC

---

## 功能特性

### NG Protocol 命令

完全兼容 rtpengine NG protocol：

| 命令 | 描述 |
|------|------|
| `ping` | 健康检查和保活 |
| `offer` | 处理 SDP offer，分配媒体端口 |
| `answer` | 处理 SDP answer，完成会话建立 |
| `delete` | 终止会话，释放资源 |
| `query` | 获取实时会话统计 |
| `list` | 枚举所有活跃通话 |
| `start recording` | 开始通话录制 |
| `stop recording` | 停止通话录制 |
| `block media` | 静音/取消静音媒体流 |
| `play DTMF` | 注入 DTMF 音调 |

### 媒体处理

- **自适应抖动缓冲区**：动态缓冲（20-200ms），根据网络状况自动调整
- **前向纠错 (FEC)**：基于 XOR 的 FEC，自适应冗余度（10-50%），基于实时丢包率调整
- **RTCP 处理**：完整的 RFC 3550 实现，支持 SR/RR 报告、RTT、抖动、MOS 计算（ITU-T G.107 E-model）
- **RTCP 转发**：RTCP 报告在通话双方之间中继，可选 SRTCP 加密
- **SRTP/SRTCP**：每条通话腿的加解密集成到媒体中继中——SDP 中的 SDES 密钥创建加密上下文，DTLS 密钥通过 `SetDTLSKeys()` 配置。完整的 SRTCP 支持加密 RTCP。
- **ICE Agent**：每会话 ICE 协商，支持 STUN server-reflexive 和 TURN relay 候选地址收集、连通性检查、候选对选择以及通过 pion/ice 实现的 consent freshness
- **编解码器协商**：RFC 3264 交集——answer 编解码器与 offerer 集合取交集，按 offerer 偏好排序
- **编解码器支持**：内置 G.711 (PCMU/PCMA) 转码；Opus、G.729、AMR 可通过 CGO 构建标签启用（`-tags opus`、`-tags g729`、`-tags amr`），分别链接 libopus、bcg729、opencore-amrnb
- **T.38 传真网关**：完整的 UDPTL 中继，支持 IFP 包解析、冗余、SDP 解析/生成、会话生命周期管理和 UDP 套接字转发
- **基于方向的接口选择**：命名网络接口，支持独立的绑定/通告 IP——SIP 代理发送 `direction` 数组以选择内部或外部媒体地址
- **SIPREC**：RFC 7865/7866 元数据解析和会话结构（录制输出通过媒体中继连接）
- **多流支持**：通过每会话的 `StreamPair` 条目支持视频和额外媒体流，每个流独立中继

### 通话录制

专业的录制能力：

| 模式 | 描述 |
|------|------|
| Mixed | 单个单声道文件，包含双方音频 |
| Stereo | 左声道为主叫方，右声道为被叫方 |
| Separate | 每条通话腿单独文件 |
| SIPREC | 符合 RFC 7865/7866 的会话录制 |

- **格式**：WAV (16-bit PCM)，可配置采样率
- **存储**：本地文件系统或网络存储
- **保留策略**：基于可配置策略的自动清理
- **故障转移**：节点故障时录制连续性

### 集群与高可用

企业级集群支持：

- **基于 Redis 的会话状态**：跨节点分布式会话共享
- **一致性哈希**：带故障转移的会话固定分配
- **脑裂检测**：基于法定人数的分区容错
- **端口重新分配**：故障转移期间一致的端口分配
- **CDR 协调**：分布式通话详单聚合
- **代理通知**：故障转移时自动通知 SIP 代理

### 安全

全面的安全特性：

- **TLS/HTTPS**：安全的 API 和管理接口
- **认证**：API 密钥和基于令牌的认证
- **授权**：基于角色的操作访问控制
- **速率限制**：可配置的每 IP 和每通话速率限制
- **DoS 防护**：自动阻止滥用来源
- **输入验证**：对所有协议输入进行严格验证
- **密钥管理**：安全处理凭证

### REST API

通过编程方式控制所有服务器功能：

```bash
# 列出活跃会话
curl http://localhost:8080/api/v1/sessions

# 获取服务器统计
curl http://localhost:8080/api/v1/stats

# 开始录制通话
curl -X POST http://localhost:8080/api/v1/recording/start \
  -H "Content-Type: application/json" \
  -d '{"session_id": "abc123", "mode": "stereo"}'

# 停止录制
curl -X POST http://localhost:8080/api/v1/recording/stop \
  -d '{"session_id": "abc123"}'
```

---

## 快速开始

### 前提条件

- Go 1.26 或更高版本
- MySQL/MariaDB（可选，用于 CDR 存储）
- Redis（可选，用于分布式会话缓存）

### 30 秒启动

```bash
# 克隆仓库
git clone https://github.com/loreste/karl.git
cd karl

# 构建并运行
go build -o karl
./karl
```

Karl 现在正在以下端口监听：
- UDP 端口 22222 用于 NG protocol（SIP 代理通信）
- UDP 端口 30000-40000 用于 RTP 媒体
- TCP 端口 8080 用于 REST API
- TCP 端口 8086 用于健康检查
- TCP 端口 9091 用于 Prometheus 指标

### 测试连接

```bash
# Ping 测试
echo -n "d7:command4:pinge" | nc -u localhost 22222
```

---

## 文档

完整文档位于 [`docs/`](./docs) 目录中。

### 入门指南

- [快速入门指南](./docs/getting-started.md) - 5 分钟内启动运行
- [安装指南](./docs/installation.md) - 详细的安装选项
- [配置参考](./docs/configuration.md) - 所有配置选项

### 操作指南

常见任务的分步指南：

| 指南 | 描述 |
|------|------|
| [在 Kubernetes 上部署](./docs/how-to/deploying-kubernetes.md) | 生产级 K8s 部署，包含探针、扩展和监控 |
| [与 OpenSIPS 集成](./docs/how-to/integrating-opensips.md) | 将 Karl 连接到 OpenSIPS，支持 NAT、WebRTC 和录制 |
| [与 Kamailio 集成](./docs/how-to/integrating-kamailio.md) | 将 Karl 连接到 Kamailio，附完整示例 |
| [设置通话录制](./docs/how-to/setting-up-recording.md) | 配置录制模式、存储和保留策略 |
| [使用 Prometheus 监控](./docs/how-to/monitoring-prometheus.md) | 指标、Grafana 仪表盘和告警规则 |
| [水平扩展](./docs/how-to/scaling-horizontally.md) | Redis 集群和负载均衡 |
| [WebRTC 到 SIP 桥接](./docs/how-to/webrtc-sip-bridging.md) | 将浏览器客户端连接到 SIP 基础设施 |
| [TLS 安全配置](./docs/how-to/securing-with-tls.md) | API 和管理接口的 HTTPS 配置 |
| [故障排除](./docs/how-to/troubleshooting.md) | 诊断和解决常见问题 |

### 参考文档

- [NG Protocol 参考](./docs/reference/ng-protocol.md) - 完整的协议规范
- [环境变量](./docs/reference/environment-variables.md) - 所有支持的变量

### CLI 工具

Karl 附带 `karl-ctl`，一个用于管理会话、查看统计和控制录制的命令行工具：

```bash
# 构建 CLI
go build -o karl-ctl ./cmd/karl-ctl/

# 用法
karl-ctl ping                      # 健康检查
karl-ctl sessions                  # 列出活跃会话
karl-ctl session <id>              # 会话详情
karl-ctl stats                     # 服务器统计
karl-ctl delete <id>               # 删除会话
karl-ctl recording start <id>      # 开始录制
karl-ctl recording stop <id>       # 停止录制

# 参数
karl-ctl -addr http://host:8080 sessions   # 自定义 API 地址
karl-ctl -json stats                        # 原始 JSON 输出
```

---

## 安装

### 从源码安装

```bash
git clone https://github.com/loreste/karl.git
cd karl
go build -o karl
sudo mv karl /usr/local/bin/
```

### Docker

```bash
docker run -d \
  --name karl \
  --network host \
  loreste/karl:latest
```

或使用端口映射（限制 RTP 端口范围）：

```bash
docker run -d \
  --name karl \
  -p 22222:22222/udp \
  -p 30000-30100:30000-30100/udp \
  -p 8080:8080 \
  -p 8086:8086 \
  -p 9091:9091 \
  -v /path/to/config.json:/etc/karl/config.json \
  -v /path/to/recordings:/var/lib/karl/recordings \
  loreste/karl:latest
```

### Docker Compose

```yaml
version: '3.8'
services:
  karl:
    image: loreste/karl:latest
    network_mode: host
    volumes:
      - ./config.json:/etc/karl/config.json
      - ./recordings:/var/lib/karl/recordings
    environment:
      - KARL_CONFIG_PATH=/etc/karl/config.json
      - KARL_LOG_LEVEL=info
    restart: unless-stopped
```

---

## 配置

Karl 使用带有合理默认值的 JSON 配置文件。所有设置也可以通过环境变量覆盖。

### 配置文件

```json
{
  "ng_protocol": {
    "enabled": true,
    "udp_port": 22222,
    "timeout": 30
  },
  "sessions": {
    "max_sessions": 10000,
    "session_ttl": 3600,
    "min_port": 30000,
    "max_port": 40000
  },
  "jitter_buffer": {
    "enabled": true,
    "min_delay": 20,
    "max_delay": 200,
    "adaptive_mode": true
  },
  "fec": {
    "enabled": true,
    "redundancy": 0.30,
    "adaptive_mode": true
  },
  "recording": {
    "enabled": true,
    "base_path": "/var/lib/karl/recordings",
    "format": "wav",
    "mode": "stereo",
    "retention_days": 30
  },
  "api": {
    "enabled": true,
    "address": ":8080",
    "auth_enabled": false
  },
  "webrtc": {
    "enabled": true,
    "stun_servers": ["stun:stun.l.google.com:19302"]
  },
  "database": {
    "mysql_dsn": "",
    "redis_enabled": false,
    "redis_addr": "redis:6379"
  },
  "integration": {
    "media_ip": "10.0.0.5",
    "interfaces": {
      "internal": { "address": "10.0.0.5", "advertise_addr": "10.0.0.5" },
      "external": { "address": "10.0.0.5", "advertise_addr": "198.51.100.10" }
    }
  },
  "hep": {
    "hep_enabled": false,
    "hep_server": "homer:9060"
  },
  "cluster": {
    "enabled": false,
    "node_id": "karl-1"
  }
}
```

### 环境变量

所有配置均可通过环境变量设置：

| 变量 | 描述 | 默认值 |
|------|------|--------|
| `KARL_CONFIG_PATH` | 配置文件路径 | `config/config.json` |
| `KARL_HEALTH_PORT` | 健康检查端点端口 | `:8086` |
| `KARL_METRICS_PORT` | Prometheus 指标端口 | `:9091` |
| `KARL_API_PORT` | REST API 端口 | `:8080` |
| `KARL_NG_PORT` | NG protocol UDP 端口 | `22222` |
| `KARL_RTP_MIN_PORT` | RTP 端口范围起始 | `30000` |
| `KARL_RTP_MAX_PORT` | RTP 端口范围结束 | `40000` |
| `KARL_MAX_SESSIONS` | 最大并发会话数 | `10000` |
| `KARL_RECORDING_PATH` | 录制文件存储路径 | `/var/lib/karl/recordings` |
| `KARL_RECORDING_ENABLED` | 启用通话录制 | `true` |
| `KARL_MYSQL_DSN` | MySQL 连接字符串 | （空） |
| `KARL_REDIS_ADDR` | Redis 服务器地址 | （空） |
| `KARL_REDIS_ENABLED` | 启用 Redis 会话缓存 | `false` |
| `KARL_MEDIA_IP` | 媒体 IP 地址 | `auto` |
| `KARL_PUBLIC_IP` | SDP 中的公网 IP | （自动检测） |

---

## Kubernetes 部署

Karl 从设计之初就面向 Kubernetes。完整的部署清单位于 `deploy/kubernetes/` 目录中。

### 快速部署

```bash
kubectl apply -k deploy/kubernetes/
```

### 健康探针

Karl 暴露 Kubernetes 原生探针端点：

| 探针 | 端点 | 用途 |
|------|------|------|
| Startup | `/startup` | 等待初始化完成（最多 150 秒） |
| Liveness | `/live` | 检测死锁，触发重启 |
| Readiness | `/ready` | 检查是否准备好接收流量 |

### 部署架构

默认部署使用 `hostNetwork: true` 以获得最佳 RTP 性能：

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: karl
spec:
  replicas: 1
  template:
    spec:
      hostNetwork: true
      containers:
      - name: karl
        image: loreste/karl:latest
        ports:
        - containerPort: 22222
          protocol: UDP
        - containerPort: 8080
          protocol: TCP
        - containerPort: 8086
          protocol: TCP
        - containerPort: 9091
          protocol: TCP
        livenessProbe:
          httpGet:
            path: /live
            port: 8086
          periodSeconds: 15
        readinessProbe:
          httpGet:
            path: /ready
            port: 8086
          periodSeconds: 5
        startupProbe:
          httpGet:
            path: /startup
            port: 8086
          failureThreshold: 30
          periodSeconds: 5
```

### 扩展

要进行水平扩展，需启用 Redis 进行会话共享：

1. 部署 Redis（或使用托管 Redis）
2. 设置 `KARL_REDIS_ENABLED=true` 和 `KARL_REDIS_ADDR=redis:6379`
3. 增加 Deployment 副本数

---

## 与 SIP 代理集成

### OpenSIPS

```opensips
# opensips.cfg
loadmodule "rtpengine.so"
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")

route {
    # ... 你的路由逻辑 ...

    if (has_body("application/sdp")) {
        rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
    }
}
```

### Kamailio

```kamailio
# kamailio.cfg
loadmodule "rtpengine.so"
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")

request_route {
    # ... 你的路由逻辑 ...

    if (has_body("application/sdp")) {
        rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
    }
}
```

### FreeSWITCH

使用 `mod_rtpengine` 模块：

```xml
<param name="rtpengine-ip" value="127.0.0.1"/>
<param name="rtpengine-port" value="22222"/>
```

---

## API 参考

### 会话

**列出所有会话**
```bash
GET /api/v1/sessions
```

**获取会话详情**
```bash
GET /api/v1/sessions/{session_id}
```

**删除会话**
```bash
DELETE /api/v1/sessions/{session_id}
```

### 统计

**获取服务器统计**
```bash
GET /api/v1/stats
```

响应：
```json
{
  "active_sessions": 150,
  "total_sessions": 10542,
  "packets_processed": 15234567,
  "packets_dropped": 234,
  "uptime_seconds": 86400
}
```

### 录制

**开始录制**
```bash
POST /api/v1/recording/start
Content-Type: application/json

{
  "session_id": "abc123",
  "mode": "stereo"
}
```

**停止录制**
```bash
POST /api/v1/recording/stop
Content-Type: application/json

{
  "session_id": "abc123"
}
```

### 健康检查

**简单健康检查**
```bash
GET /health
```

**详细健康状态**
```bash
GET /health/detail
```

---

## 监控与可观测性

### Prometheus 指标

Karl 在 `:9091/metrics` 暴露全面的指标：

**会话指标**
```
karl_sessions_active          # 当前活跃会话数
karl_sessions_total           # 创建的总会话数
karl_session_duration_seconds # 会话持续时间直方图
```

**媒体质量指标**
```
karl_rtcp_rtt_seconds         # RTCP 往返时间
karl_rtcp_jitter_seconds      # 报告的抖动
karl_rtcp_packet_loss_ratio   # 丢包百分比
karl_fec_recoveries_total     # 通过 FEC 恢复的数据包
karl_jitter_buffer_latency    # 抖动缓冲区延迟
```

**NG Protocol 指标**
```
karl_ng_commands_total        # 按类型统计的已处理命令
karl_ng_command_duration      # 命令处理时间
karl_ng_active_calls          # 当前通话数
```

### Grafana 仪表盘

从 `deploy/grafana/karl-dashboard.json` 导入提供的仪表盘，用于：
- 实时会话监控
- 媒体质量可视化
- 资源利用率跟踪
- 告警配置

### 健康端点

| 端点 | 用途 | 响应 |
|------|------|------|
| `/health` | 简单状态 | `{"status":"UP"}` |
| `/health/detail` | 组件状态 | 完整的组件状态分解 |
| `/live` | Kubernetes 存活探针 | 存活时返回 200 |
| `/ready` | Kubernetes 就绪探针 | 准备好接收流量时返回 200 |
| `/startup` | Kubernetes 启动探针 | 初始化完成时返回 200 |

---

## 性能

Karl 针对高吞吐量媒体处理进行了优化：

| 指标 | 性能 |
|------|------|
| 会话创建 | 400 万次操作/秒 |
| 抖动缓冲区操作 | 440 万次操作/秒 |
| FEC 编码 | 1030 万次操作/秒 |
| G.711 转码 | 330 万次操作/秒 |
| iLBC 编码 | 130 万次操作/秒 |
| 缓冲池操作 | 5800 万次操作/秒 |
| 每会话内存占用 | 约 624 字节 |
| 已测试并发会话数 | 10,000+ |

### 性能特性

- **零拷贝转发**：快速路径中最小化内存拷贝
- **套接字分片**：每核心独立套接字，提升可扩展性
- **缓冲池**：可复用缓冲区减少 GC 压力
- **工作线程池**：高效的并发包处理
- **批量 RTCP**：聚合 RTCP 处理
- **异步录制**：非阻塞的录制写入

### 资源建议

| 并发通话数 | CPU | 内存 |
|-----------|-----|------|
| < 100 | 250m | 256Mi |
| 100-500 | 500m | 512Mi |
| 500-1000 | 1000m | 1Gi |
| > 1000 | 2000m | 2Gi |

---

## 架构

```
                                    ┌──────────────────┐
                                    │   Prometheus     │
                                    │   :9091/metrics  │
                                    └────────┬─────────┘
                                             │
┌─────────────┐     NG Protocol      ┌───────┴────────┐     RTP/RTCP      ┌────────────┐
│  OpenSIPS   │ ◄──────────────────► │                │ ◄───────────────► │  Endpoints │
│  Kamailio   │     UDP:22222        │      Karl      │   UDP:30000-40000 │ (SIP/WebRTC)│
└─────────────┘                      │                │                   └────────────┘
                                     └───────┬────────┘
                                             │
                                     ┌───────┴────────┐
                                     │   REST API     │
                                     │   :8080        │
                                     └────────────────┘
```

### 组件概览

- **NG Protocol 处理器** (`ng_socket_listener.go`)：通过 UDP/Unix 套接字处理来自 SIP 代理的 bencode 命令
- **命令处理器** (`ng_protocol/commands/`)：Offer、Answer、Delete、Query——创建会话、分配端口、构建 SDP
- **会话管理器** (`session_manager.go`)：跟踪通话状态、通话腿、SSRC 索引、TTL 清理
- **媒体中继** (`media_relay.go`)：每会话 RTP/RTCP 转发，支持通话双方之间的 SRTP/SRTCP 加解密、NAT 处理（对称 RTP、端口锁定、媒体切换、严格源验证）、媒体阻断/静音、录制钩子和媒体超时检测
- **ICE Agent** (`ice_agent.go`)：每会话 ICE 协商，支持 STUN/TURN 候选地址收集、连通性检查和候选对选择（通过 pion/ice）
- **录制管理器** (`recording/`)：WAV 文件写入，支持混合/立体声/分离模式，通过 OnPacket 钩子连接到中继
- **RTCP 处理器** (`rtcp_handler.go`)：质量监控，包括抖动、丢包、RTT、MOS 计算（ITU-T G.107）
- **T.38 网关** (`t38_gateway.go`)：UDPTL 中继，支持 IFP 解析、冗余、SDP 生成和传真会话生命周期
- **HEP 导出器** (`hep.go`)：HEPv3 数据包导出到 Homer/SIPVicious 抓包服务器
- **集群管理器** (`cluster_manager.go`)：基于 Redis 的会话故障转移，支持脑裂检测和一致性哈希
- **FEC 处理器** (`fec_handler.go`)：基于 XOR 的前向纠错
- **抖动缓冲区** (`jitter_buffer.go`)：自适应缓冲（20-200ms），支持序列重排序
- **CLI 工具** (`cmd/karl-ctl/`)：用于会话管理、统计、录制和健康检查的命令行工具

---

## 路线图

Karl 是功能完整的 rtpengine 替代方案。详见 [ROADMAP.md](./ROADMAP.md) 了解实现细节。

### 第一阶段：控制面对等 ✅ 已完成
- [x] 完整的 NG protocol 标志支持（100+ 标志）
- [x] Bencode 解析、cookie 关联、UDP/Unix 套接字监听
- [x] 响应格式兼容

### 第二阶段：中继级媒体 ✅ 已完成
- [x] 每会话媒体中继，每通话腿独立 UDP 套接字
- [x] NAT 穿透：对称 RTP、端口锁定、媒体切换、严格源验证
- [x] 真实 DTLS 指纹（自签名 ECDSA 证书）和随机 ICE 凭证
- [x] 真实 SDES 加密密钥（符合 RFC 4568）
- [x] 媒体阻断、静音、媒体超时自动删除
- [x] 录制钩子接入媒体中继路径

### 第三阶段：企业级功能 ✅ 已完成
- [x] 通话录制：WAV 输出，混合/立体声/分离模式，接入媒体路径
- [x] G.711 PCMU↔PCMA 转码（查找表）
- [x] Opus/G.729/AMR 转码，通过 CGO 构建标签（libopus、bcg729、opencore-amrnb）
- [x] SRTP↔RTP 网关：每通话腿加解密集成到媒体中继（SDES + DTLS 密钥路径）
- [x] SRTCP 加密用于 RTCP 转发
- [x] T.38 传真网关：UDPTL 中继、IFP 解析、SDP 解析/生成、UDP 套接字转发
- [x] 完整 ICE agent：STUN/TURN 候选地址收集、连通性检查、候选对选择
- [x] 编解码器协商（RFC 3264）：按 offerer 偏好排序的交集
- [x] MOS 计算（ITU-T G.107 E-model），基于 RTCP 抖动/丢包/RTT
- [x] 基于方向的接口选择：绑定 vs 通告 IP，内部/外部路由
- [x] 多流支持：通过 StreamPair 支持视频和额外媒体
- [x] Redis 集群：会话存储 + 发布/订阅 + 脑裂检测 + 故障转移
- [x] HEP/Homer：HEPv3 导出器用于 SIP/RTCP 抓包
- [x] CLI 工具：`karl-ctl` 用于会话管理、统计、录制、健康检查

### 第四阶段：运维成熟度 ✅ 已完成
- [x] 1300+ 测试，启用竞态检测，零失败
- [x] 中继协程中的 panic 恢复
- [x] 资源泄漏防护（错误时端口清理、MaxSessions 强制执行）
- [x] 安全：速率限制、API 认证、DoS 防护、熔断器
- [x] Prometheus 指标（50+）、健康探针、结构化日志

### 第五阶段：测试与验证（进行中）
- [x] 媒体中继的单元测试和集成测试（offer→answer→RTP 流→delete）
- [ ] 与实际 SIP 代理的集成测试（OpenSIPS、Kamailio）
- [ ] 混沌测试、持续负载测试

---

## 贡献

欢迎贡献！请查看我们的[贡献指南](CONTRIBUTING.md)了解：

- 代码风格指南
- Pull Request 流程
- 开发环境设置
- 测试要求

### 从源码构建

```bash
git clone https://github.com/loreste/karl.git
cd karl
go mod download
go build -o karl
go test ./...
```

---

## 许可证

Karl Media Server 基于 [GNU General Public License v3.0](LICENSE) 许可证发布。

---

## 致谢

Karl 基于开源社区的工作成果构建：

- [Pion](https://github.com/pion) - WebRTC、DTLS、SRTP 和 RTP 库
- [rtpengine](https://github.com/sipwise/rtpengine) - 协议规范和兼容性参考

---

## 支持

- **文档**：[完整文档](./docs/README.md)
- **操作指南**：[分步指南](./docs/how-to)
- **问题反馈**：[GitHub Issues](https://github.com/loreste/karl/issues)
- **讨论交流**：[GitHub Discussions](https://github.com/loreste/karl/discussions)

---

**Karl Media Server** — 为需要可靠、可观测、云原生媒体基础设施的 VoIP 工程师而构建。
