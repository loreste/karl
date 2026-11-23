# 如何将 Karl 与 OpenSIPS 集成

本指南涵盖将 Karl 媒体服务器与 OpenSIPS 集成，实现 RTP 代理、NAT 穿越和媒体操控。

## 目录

- [前提条件](#前提条件)
- [基本集成](#基本集成)
- [配置选项](#配置选项)
- [常见用例](#常见用例)
- [高级配置](#高级配置)
- [高可用](#高可用)
- [故障排除](#故障排除)

---

## 前提条件

- OpenSIPS 3.x 或更高版本
- Karl 媒体服务器已运行且可访问
- OpenSIPS 已编译 `rtpengine` 模块

验证模块是否可用：

```bash
opensips -C -f /etc/opensips/opensips.cfg 2>&1 | grep rtpengine
```

---

## 基本集成

### 步骤一：加载模块

在 `opensips.cfg` 中添加：

```opensips
# 加载 rtpengine 模块
loadmodule "rtpengine.so"
```

### 步骤二：配置模块

```opensips
# 指向 Karl 媒体服务器
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")
```

### 步骤三：在路由逻辑中使用

```opensips
route {
    # 处理 INVITE
    if (is_method("INVITE")) {
        if (has_body("application/sdp")) {
            # 为 offer 启用 RTP 代理
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
    }

    # 处理 BYE
    if (is_method("BYE")) {
        rtpengine_delete();
    }

    # 路由请求
    t_relay();
}

# 处理应答
onreply_route[MANAGE_REPLY] {
    if (has_body("application/sdp")) {
        rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
    }
}
```

### 步骤四：重启 OpenSIPS

```bash
systemctl restart opensips
```

---

## 配置选项

### 模块参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `rtpengine_sock` | （无） | Karl 服务器地址 |
| `rtpengine_tout` | 1 | 套接字超时时间（秒） |
| `rtpengine_retr` | 5 | 重试次数 |
| `queued_msgs_threshold` | 64 | 最大排队消息数 |
| `db_url` | （无） | rtpengine 列表的数据库 |

### 配置示例

```opensips
# 模块参数
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")
modparam("rtpengine", "rtpengine_tout", 2)
modparam("rtpengine", "rtpengine_retr", 3)
```

---

## 常见用例

### NAT 穿越

处理一方或双方位于 NAT 后面的通话：

```opensips
route {
    if (is_method("INVITE")) {
        if (has_body("application/sdp")) {
            # 检测 NAT 并应用相应标志
            if (nat_uac_test("19")) {
                # 客户端在 NAT 后面
                rtpengine_manage("RTP/AVP replace-origin replace-session-connection SIP-source-address");
            } else {
                rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
            }
        }
    }
}
```

### WebRTC 到 SIP 桥接

将 WebRTC 客户端桥接到 SIP 端点：

```opensips
route {
    if (is_method("INVITE")) {
        if ($ru =~ "transport=ws") {
            # WebRTC 客户端 — 需要移除 ICE 和处理 DTLS-SRTP
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection ICE=remove DTLS=off SDES-off");
        } else {
            # 普通 SIP 端点
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
    }
}
```

### 通话录音

为特定通话启用录音：

```opensips
route {
    if (is_method("INVITE")) {
        # 检查是否需要录音（例如基于主叫方）
        if ($fU == "recorded_user") {
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection record-call");
        } else {
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
    }
}
```

### 编解码器过滤

强制使用特定编解码器：

```opensips
route {
    if (is_method("INVITE")) {
        # 仅允许 G.711 编解码器
        rtpengine_manage("RTP/AVP replace-origin replace-session-connection codec-strip-all codec-offer-PCMU codec-offer-PCMA");
    }
}
```

### 基于方向的接口选择（内部与外部）

当媒体服务器有独立的内部和外部网络接口时（例如 PBX 使用私有 IP，运营商使用弹性公网 IP），使用 `direction` 标志告诉 Karl 在每个呼叫腿的 SDP 中公布哪个 IP。

**Karl 配置：**
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

**OpenSIPS 路由：**
```opensips
route {
    if (is_method("INVITE")) {
        if (has_body("application/sdp")) {
            if (is_carrier_call()) {
                # 内部 PBX → 外部运营商
                rtpengine_manage("RTP/AVP replace-origin replace-session-connection direction=internal direction=external");
            } else {
                # 内部到内部
                rtpengine_manage("RTP/AVP replace-origin replace-session-connection direction=internal direction=internal");
            }
        }
    }
}
```

通过此设置，发往外部运营商的通话在 SDP 中使用 `198.51.100.10`（弹性公网 IP），而内部通话使用 `10.0.0.5`。套接字始终绑定在 `10.0.0.5` 上。

### 媒体超时处理

配置会话超时：

```opensips
route {
    if (is_method("INVITE")) {
        # 设置媒体超时为 60 秒
        rtpengine_manage("RTP/AVP replace-origin replace-session-connection media-timeout=60");
    }
}
```

---

## 高级配置

### 完整 opensips.cfg 示例

```opensips
####### 全局参数 #########
log_level=3
log_stderror=no
log_facility=LOG_LOCAL0

listen=udp:0.0.0.0:5060

####### 模块加载 ########

# 核心模块
loadmodule "signaling.so"
loadmodule "sl.so"
loadmodule "tm.so"
loadmodule "rr.so"
loadmodule "maxfwd.so"
loadmodule "textops.so"
loadmodule "sipmsgops.so"
loadmodule "mi_fifo.so"

# NAT 处理
loadmodule "nathelper.so"

# RTP 代理
loadmodule "rtpengine.so"

####### 模块参数 ########

# 事务模块
modparam("tm", "fr_timeout", 5)
modparam("tm", "fr_inv_timeout", 30)

# Record-Route
modparam("rr", "enable_full_lr", 1)

# NAT 助手
modparam("nathelper", "ping_nated_only", 1)
modparam("nathelper", "sipping_bflag", "SIP_PING_FLAG")
modparam("nathelper", "natping_interval", 30)

# RTPEngine (Karl)
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")
modparam("rtpengine", "rtpengine_tout", 2)
modparam("rtpengine", "rtpengine_retr", 3)

####### 路由逻辑 ########

route {
    # 最大跳数检查
    if (!mf_process_maxfwd_header(10)) {
        sl_send_reply(483, "Too Many Hops");
        exit;
    }

    # 为建立对话的请求添加 Record-Route
    if (is_method("INVITE|SUBSCRIBE")) {
        record_route();
    }

    # 处理序列请求
    if (has_totag()) {
        if (loose_route()) {
            if (is_method("BYE")) {
                # 在 BYE 时删除 RTP 会话
                rtpengine_delete();
            } else if (is_method("INVITE")) {
                # 处理 Re-INVITE
                if (has_body("application/sdp")) {
                    route(RTPENGINE);
                }
            }
            route(RELAY);
            exit;
        }
    }

    # 处理 CANCEL
    if (is_method("CANCEL")) {
        if (t_check_trans()) {
            rtpengine_delete();
            t_relay();
        }
        exit;
    }

    # 处理初始 INVITE
    if (is_method("INVITE")) {
        if (has_body("application/sdp")) {
            route(RTPENGINE);
        }
        t_on_reply("MANAGE_REPLY");
    }

    route(RELAY);
}

route[RTPENGINE] {
    $var(rtpflags) = "RTP/AVP replace-origin replace-session-connection";

    # NAT 检测
    if (nat_uac_test("19")) {
        $var(rtpflags) = $var(rtpflags) + " SIP-source-address";
    }

    rtpengine_manage("$var(rtpflags)");
}

route[RELAY] {
    if (!t_relay()) {
        sl_reply_error();
    }
}

onreply_route[MANAGE_REPLY] {
    if (has_body("application/sdp")) {
        route(RTPENGINE);
    }
}

# 失败路由 — 通话失败时清理
failure_route[CALL_FAILURE] {
    if (t_was_cancelled()) {
        rtpengine_delete();
        exit;
    }
}
```

### 标志参考

常用 rtpengine_manage 标志：

| 标志 | 说明 |
|------|------|
| `RTP/AVP` | 使用 RTP/AVP 配置文件 |
| `RTP/SAVP` | 使用 RTP/SAVP（SRTP）配置文件 |
| `replace-origin` | 替换 SDP origin |
| `replace-session-connection` | 替换会话级连接 |
| `SIP-source-address` | 使用 SIP 源作为媒体地址 |
| `ICE=remove` | 移除 ICE 候选 |
| `ICE=force` | 强制 ICE |
| `DTLS=off` | 禁用 DTLS |
| `SDES-off` | 禁用 SDES |
| `record-call` | 启用录音 |
| `media-timeout=N` | 设置媒体超时（秒） |
| `direction=X` | 接口提示（提供两次：from、to）用于内部/外部 IP 选择 |
| `codec-strip-all` | 移除所有编解码器 |
| `codec-offer-X` | 提供编解码器 X |

---

## 高可用

### 多个 Karl 实例

配置多个 Karl 服务器：

```opensips
# 主备配置
modparam("rtpengine", "rtpengine_sock",
    "udp:karl1.example.com:22222 udp:karl2.example.com:22222")
```

### 加权分配

```opensips
# 70% 到主服务器，30% 到备服务器
modparam("rtpengine", "rtpengine_sock",
    "7 == udp:karl1.example.com:22222 3 == udp:karl2.example.com:22222")
```

### 数据库配置

将 Karl 服务器存储在数据库中：

```opensips
modparam("rtpengine", "db_url", "mysql://opensips:password@localhost/opensips")
modparam("rtpengine", "db_table", "rtpengine")
```

SQL 表结构：

```sql
CREATE TABLE rtpengine (
    id INT PRIMARY KEY AUTO_INCREMENT,
    socket VARCHAR(128) NOT NULL,
    set_id INT DEFAULT 0,
    weight INT DEFAULT 1,
    disabled INT DEFAULT 0
);

INSERT INTO rtpengine (socket, set_id, weight) VALUES
    ('udp:karl1.example.com:22222', 0, 10),
    ('udp:karl2.example.com:22222', 0, 10);
```

---

## 故障排除

### OpenSIPS 无法连接到 Karl

```bash
# 检查 Karl 是否在运行
echo -n "d7:command4:pinge" | nc -u 127.0.0.1 22222

# 检查 OpenSIPS 日志
tail -f /var/log/opensips.log | grep rtpengine

# 启用调试日志
modparam("rtpengine", "rtpengine_disable", 0)
```

### 通话中没有音频

1. 检查 SDP 是否被修改：
```opensips
# 在路由中添加
xlog("L_INFO", "SDP before: $rb\n");
rtpengine_manage("...");
xlog("L_INFO", "SDP after: $rb\n");
```

2. 验证端点之间的 RTP 端口可达

3. 检查 Karl 日志中的会话创建记录

### 单向音频

常见原因：
- NAT 检测不正确
- 非对称路由
- 防火墙阻止了回程流量

解决方案：
```opensips
# 强制对称 RTP
rtpengine_manage("RTP/AVP replace-origin replace-session-connection symmetric");
```

### 录音不工作

1. 验证 Karl 配置中录音已启用
2. 检查录音目录权限
3. 确认发送了 `record-call` 标志：

```opensips
rtpengine_manage("RTP/AVP replace-origin replace-session-connection record-call");
```

### 监控活动会话

```bash
# 通过 Karl API
curl http://127.0.0.1:8080/api/v1/sessions

# 通过 OpenSIPS MI
opensips-cli -x mi rtpengine_show_all
```

---

## 下一步

- [配置通话录音](./setting-up-recording.md)
- [设置监控](./monitoring-prometheus.md)
- [WebRTC 到 SIP 桥接](./webrtc-sip-bridging.md)
