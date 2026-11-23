# 如何将 WebRTC 桥接到 SIP

本指南涵盖使用 Karl 将 WebRTC 浏览器客户端桥接到传统 SIP 基础设施。

## 目录

- [概述](#概述)
- [工作原理](#工作原理)
- [前提条件](#前提条件)
- [配置](#配置)
- [SIP 代理设置](#sip-代理设置)
- [常见场景](#常见场景)
- [编解码器处理](#编解码器处理)
- [故障排除](#故障排除)

---

## 概述

Karl 作为 WebRTC 和 SIP 之间的媒体网关，处理：

- **ICE/STUN/TURN**：WebRTC 客户端的 NAT 穿越
- **DTLS-SRTP 到 RTP**：加密桥接
- **编解码器转码**：Opus 到 G.711 转换
- **SDP 转换**：WebRTC 到 SIP 兼容的 SDP

---

## 工作原理

```
┌──────────────┐                    ┌──────────────┐                    ┌──────────────┐
│    浏览器    │                    │     Karl     │                    │  SIP 话机    │
│   (WebRTC)   │                    │              │                    │              │
└──────┬───────┘                    └──────┬───────┘                    └──────┬───────┘
       │                                   │                                   │
       │  DTLS-SRTP (Opus)                │  RTP (G.711)                      │
       │  ICE candidates                   │  Plain RTP                        │
       │◄─────────────────────────────────►│◄─────────────────────────────────►│
       │                                   │                                   │
```

### 执行的转换

| WebRTC 侧 | SIP 侧 |
|-----------|---------|
| DTLS-SRTP | RTP 或 SRTP |
| ICE candidates | 直接 IP |
| Opus 编解码器 | G.711（PCMU/PCMA） |
| Transport-CC | RTCP |
| Bundle | 独立 m-lines |

---

## 前提条件

- Karl 媒体服务器已运行
- SIP 代理（OpenSIPS 或 Kamailio）支持 WebSocket
- 支持 WebRTC 的浏览器应用
- STUN/TURN 服务器（用于 NAT 穿越）

---

## 配置

### Karl 配置

```json
{
  "webrtc": {
    "enabled": true,
    "stun_servers": [
      "stun:stun.l.google.com:19302",
      "stun:stun1.l.google.com:19302"
    ],
    "turn_servers": [
      {
        "url": "turn:turn.example.com:3478",
        "username": "user",
        "credential": "password"
      }
    ],
    "max_bitrate": 2000000,
    "bw_estimation": true,
    "tcc_enabled": true
  },
  "ng_protocol": {
    "enabled": true,
    "udp_port": 22222
  }
}
```

### 环境变量

```bash
export KARL_WEBRTC_ENABLED=true
export KARL_STUN_SERVERS="stun:stun.l.google.com:19302"
```

---

## SIP 代理设置

### OpenSIPS 配合 WebSocket

```opensips
# 加载 WebSocket 模块
loadmodule "proto_ws.so"
loadmodule "proto_wss.so"

# 监听 WebSocket
listen=ws:0.0.0.0:80
listen=wss:0.0.0.0:443

# RTPEngine 模块
loadmodule "rtpengine.so"
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")

route {
    if (is_method("INVITE")) {
        if ($proto == "ws" || $proto == "wss") {
            # WebRTC 到 SIP
            # 移除 ICE，禁用 DTLS，转换为 RTP
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection ICE=remove DTLS=off SDES-off");
        } else if ($ru =~ "transport=ws") {
            # SIP 到 WebRTC（不常见但需处理）
            rtpengine_manage("RTP/SAVPF replace-origin replace-session-connection ICE=force DTLS=passive");
        } else {
            # SIP 到 SIP
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
    }

    if (is_method("BYE")) {
        rtpengine_delete();
    }

    t_relay();
}

onreply_route[MANAGE_REPLY] {
    if (has_body("application/sdp")) {
        if ($proto == "ws" || $proto == "wss") {
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection ICE=remove DTLS=off SDES-off");
        } else {
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
    }
}
```

### Kamailio 配合 WebSocket

```kamailio
# 加载 WebSocket 模块
loadmodule "xhttp.so"
loadmodule "websocket.so"

# 监听 WebSocket
listen=tcp:0.0.0.0:80
listen=tls:0.0.0.0:443

# RTPEngine 模块
loadmodule "rtpengine.so"
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")

request_route {
    if (is_method("INVITE")) {
        if ($proto == "ws" || $proto == "wss") {
            # WebRTC 客户端
            $var(rtpflags) = "RTP/AVP replace-origin replace-session-connection ICE=remove DTLS=off SDES-off";
        } else {
            # 标准 SIP
            $var(rtpflags) = "RTP/AVP replace-origin replace-session-connection";
        }

        rtpengine_manage("$var(rtpflags)");
        t_on_reply("MANAGE_REPLY");
    }

    if (is_method("BYE")) {
        rtpengine_delete();
    }

    route(RELAY);
}

onreply_route[MANAGE_REPLY] {
    if (has_body("application/sdp")) {
        if ($proto == "ws" || $proto == "wss") {
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection ICE=remove DTLS=off SDES-off");
        } else {
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
    }
}

event_route[xhttp:request] {
    if ($hdr(Upgrade) =~ "websocket") {
        if (ws_handle_handshake()) {
            exit;
        }
    }
    xhttp_reply("404", "Not Found", "", "");
}
```

---

## 常见场景

### WebRTC 浏览器到 SIP 话机

浏览器发起对 SIP 话机的呼叫：

```
浏览器 (WebRTC) ──► SIP 代理 ──► Karl ──► SIP 话机
     │                  │           │           │
     │ INVITE (WS)      │           │           │
     │ SDP: Opus, ICE   │           │           │
     │─────────────────►│           │           │
     │                  │ INVITE    │           │
     │                  │ SDP 修改  │           │
     │                  │──────────►│ INVITE    │
     │                  │           │ SDP: G.711│
     │                  │           │──────────►│
```

**使用的标志**：
```
ICE=remove    # 移除 ICE candidates
DTLS=off      # 禁用 DTLS
SDES-off      # 禁用 SDES
RTP/AVP       # 使用纯 RTP
```

### SIP 话机到 WebRTC 浏览器

SIP 话机呼叫 WebRTC 客户端：

```
SIP 话机 ──► Karl ──► SIP 代理 ──► 浏览器 (WebRTC)
     │           │          │              │
     │ INVITE    │          │              │
     │ SDP: G.711│          │              │
     │──────────►│ INVITE   │              │
     │           │ SDP 修改 │ INVITE (WS)  │
     │           │─────────►│ SDP: Opus    │
     │           │          │─────────────►│
```

**使用的标志**：
```
ICE=force     # 添加 ICE candidates
DTLS=passive  # 以被动模式启用 DTLS
RTP/SAVPF     # 使用带反馈的 SRTP
```

### WebRTC 到 WebRTC（通过 SIP 代理）

双方都是 WebRTC：

```
浏览器 A ──► SIP 代理 ──► Karl ──► SIP 代理 ──► 浏览器 B
```

**标志**：Karl 在双方都处理 ICE 和 DTLS。

---

## 编解码器处理

### 自动转码

Karl 自动在以下编解码器之间转码：

| WebRTC 编解码器 | SIP 编解码器 |
|----------------|-------------|
| Opus | PCMU（G.711 μ-law） |
| Opus | PCMA（G.711 A-law） |

### 强制特定编解码器

```opensips
# 移除所有编解码器，仅保留 G.711
rtpengine_manage("... codec-strip-all codec-offer-PCMU codec-offer-PCMA");
```

### 编解码器优先级

```opensips
# 优先 PCMU 而非 PCMA
rtpengine_manage("... codec-mask-PCMA");
```

---

## NAT 穿越

### STUN 配置

STUN 服务器帮助发现公网 IP：

```json
{
  "webrtc": {
    "stun_servers": [
      "stun:stun.l.google.com:19302",
      "stun:stun.example.com:3478"
    ]
  }
}
```

### TURN 配置

当直接连接失败时，TURN 服务器中继流量：

```json
{
  "webrtc": {
    "turn_servers": [
      {
        "url": "turn:turn.example.com:3478",
        "username": "webrtc",
        "credential": "secret123"
      },
      {
        "url": "turns:turn.example.com:5349",
        "username": "webrtc",
        "credential": "secret123"
      }
    ]
  }
}
```

### ICE 处理标志

| 标志 | 说明 |
|------|------|
| `ICE=remove` | 移除所有 ICE 属性 |
| `ICE=force` | 强制 ICE 协商 |
| `ICE=force-relay` | 强制 TURN 中继 |

---

## 安全注意事项

### SRTP 模式

| 模式 | 说明 | 适用场景 |
|------|------|----------|
| `DTLS=off` | 禁用 DTLS-SRTP | WebRTC 到纯 RTP |
| `DTLS=passive` | DTLS 被动角色 | SIP 到 WebRTC |
| `DTLS=active` | DTLS 主动角色 | Karl 发起 DTLS |
| `SDES-off` | 禁用 SDES | 不提供 SDES |
| `SDES-on` | 启用 SDES | 提供 SDES 密钥 |

### 推荐配置

```opensips
# WebRTC 到 SIP（移除加密）
rtpengine_manage("RTP/AVP ... DTLS=off SDES-off");

# SIP 到 WebRTC（添加加密）
rtpengine_manage("RTP/SAVPF ... DTLS=passive ICE=force");

# WebRTC 到 WebRTC（保持加密）
rtpengine_manage("RTP/SAVPF ... DTLS=passive ICE=force");
```

---

## 故障排除

### WebRTC 客户端没有音频

**诊断**：
```bash
# 检查 ICE 是否被处理
grep -i "ICE\|DTLS" /var/log/karl.log

# 验证会话 SDP
curl http://localhost:8080/api/v1/sessions | jq .
```

**常见原因**：
1. SIP 侧未移除 ICE
2. DTLS 握手失败
3. STUN/TURN 不可达

**解决方案**：
```opensips
# 确保移除了 ICE
rtpengine_manage("... ICE=remove");

# 检查 TURN 连通性
# 浏览器控制台应显示 ICE gathering complete
```

### 单向音频

**WebRTC 能听到 SIP 但反之不行**：
- DTLS 握手未完成
- ICE 连通性检查失败

**SIP 能听到 WebRTC 但反之不行**：
- SIP 端点的 NAT 未处理
- 防火墙阻止了回程流量

**解决方案**：
```bash
# 启用对称 RTP
rtpengine_manage("... symmetric");
```

### 编解码器协商失败

**症状**：没有共同编解码器，通话失败

**诊断**：
```bash
# 检查日志中提供的编解码器
grep -i "codec\|offer" /var/log/karl.log
```

**解决方案**：
```opensips
# 始终转码为 G.711
rtpengine_manage("... transcode-PCMU transcode-PCMA");
```

### DTLS 握手超时

**症状**：通话建立延迟，最终失败

**诊断**：
```bash
# 检查 DTLS 日志
grep -i DTLS /var/log/karl.log
```

**解决方案**：
1. 验证证书配置
2. 检查 DTLS 端口的 UDP 连通性
3. 确保防火墙允许 UDP 流量

### ICE 收集缓慢

**症状**：通话建立时间过长

**解决方案**：
1. 使用距离更近的 STUN/TURN 服务器
2. 减少收集的 ICE candidates
3. 检查到 STUN 服务器的网络延迟

---

## WebRTC 客户端示例

### JavaScript SIP.js 示例

```javascript
const ua = new SIP.UA({
  uri: 'sip:user@example.com',
  transportOptions: {
    wsServers: ['wss://proxy.example.com:443']
  },
  sessionDescriptionHandlerFactoryOptions: {
    peerConnectionOptions: {
      iceServers: [
        { urls: 'stun:stun.l.google.com:19302' },
        {
          urls: 'turn:turn.example.com:3478',
          username: 'user',
          credential: 'password'
        }
      ]
    }
  }
});

// 发起通话
const session = ua.invite('sip:destination@example.com', {
  sessionDescriptionHandlerOptions: {
    constraints: {
      audio: true,
      video: false
    }
  }
});
```

### JsSIP 示例

```javascript
const socket = new JsSIP.WebSocketInterface('wss://proxy.example.com:443');
const configuration = {
  sockets: [socket],
  uri: 'sip:user@example.com',
  password: 'secret'
};

const ua = new JsSIP.UA(configuration);

ua.start();

// 发起通话
const eventHandlers = {
  'progress': (e) => console.log('call is in progress'),
  'failed': (e) => console.log('call failed'),
  'ended': (e) => console.log('call ended'),
  'confirmed': (e) => console.log('call confirmed')
};

const options = {
  eventHandlers: eventHandlers,
  mediaConstraints: { audio: true, video: false },
  pcConfig: {
    iceServers: [
      { urls: 'stun:stun.l.google.com:19302' }
    ]
  }
};

ua.call('sip:destination@example.com', options);
```

---

## 下一步

- [OpenSIPS 集成](./integrating-opensips.md)
- [Kamailio 集成](./integrating-kamailio.md)
- [故障排除指南](./troubleshooting.md)
