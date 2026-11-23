# 如何将 Karl 与 Kamailio 集成

本指南涵盖将 Karl 媒体服务器与 Kamailio 集成，实现 RTP 代理、NAT 穿越和媒体处理。

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

- Kamailio 5.x 或更高版本
- Karl 媒体服务器已运行且可访问
- Kamailio 已编译 `rtpengine` 模块

验证模块是否可用：

```bash
kamailio -l | grep rtpengine
```

---

## 基本集成

### 步骤一：加载模块

在 `kamailio.cfg` 中添加：

```kamailio
# 加载 rtpengine 模块
loadmodule "rtpengine.so"
```

### 步骤二：配置模块

```kamailio
# 指向 Karl 媒体服务器
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")
```

### 步骤三：在路由逻辑中使用

```kamailio
request_route {
    # 处理 INVITE
    if (is_method("INVITE")) {
        if (has_body("application/sdp")) {
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
        t_on_reply("MANAGE_REPLY");
    }

    # 处理 BYE
    if (is_method("BYE")) {
        rtpengine_delete();
    }

    # 路由请求
    route(RELAY);
}

onreply_route[MANAGE_REPLY] {
    if (has_body("application/sdp")) {
        rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
    }
}

route[RELAY] {
    if (!t_relay()) {
        sl_reply_error();
    }
    exit;
}
```

### 步骤四：重启 Kamailio

```bash
systemctl restart kamailio
```

---

## 配置选项

### 模块参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `rtpengine_sock` | （无） | Karl 服务器地址 |
| `rtpengine_disable_tout` | 60 | 故障后禁用超时（秒） |
| `rtpengine_tout_ms` | 1000 | 套接字超时（毫秒） |
| `rtpengine_retr` | 5 | 重试次数 |
| `queried_nodes_limit` | 0 | 最大查询节点数（0=全部） |
| `setid_default` | 0 | 默认集合 ID |

### 配置示例

```kamailio
# 模块参数
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")
modparam("rtpengine", "rtpengine_tout_ms", 2000)
modparam("rtpengine", "rtpengine_retr", 3)
modparam("rtpengine", "rtpengine_disable_tout", 30)
```

---

## 常见用例

### NAT 穿越

处理位于 NAT 后面的端点：

```kamailio
request_route {
    if (is_method("INVITE")) {
        # 检测 NAT
        if (nat_uac_test(64)) {
            # 强制中继并修复 NAT
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection SIP-source-address");
            setbflag(FLB_NATB);
        } else {
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
    }
}
```

### WebRTC 网关

将 WebRTC 桥接到 SIP：

```kamailio
request_route {
    if (is_method("INVITE")) {
        if ($proto == "ws" || $proto == "wss") {
            # WebRTC 端点
            xlog("L_INFO", "WebRTC call from $fU\n");
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection ICE=remove DTLS=off SDES-off");
        } else if (has_body("application/sdp")) {
            # 普通 SIP 端点
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
    }
}
```

### 通话录音

按需启用录音：

```kamailio
request_route {
    if (is_method("INVITE")) {
        # 录制来自特定号码的通话
        if ($fU =~ "^record") {
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection record-call");
        } else {
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
    }
}
```

### 编解码器转码

强制编解码器协商：

```kamailio
request_route {
    if (is_method("INVITE")) {
        # 移除所有编解码器并仅提供 G.711
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

**Kamailio 路由：**
```kamailio
request_route {
    if (is_method("INVITE")) {
        if (has_body("application/sdp")) {
            if ($rd =~ "carrier\.example\.com") {
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

### 强制 SRTP

为所有通话强制使用 SRTP：

```kamailio
request_route {
    if (is_method("INVITE")) {
        # 强制 SRTP
        rtpengine_manage("RTP/SAVP replace-origin replace-session-connection SDES-on");
    }
}
```

---

## 高级配置

### 完整 kamailio.cfg 示例

```kamailio
#!KAMAILIO

####### 全局参数 #########

#!define WITH_NAT
#!define WITH_RTPENGINE

debug=2
log_stderror=no
log_facility=LOG_LOCAL0
fork=yes
children=4

listen=udp:0.0.0.0:5060
listen=tcp:0.0.0.0:5060

####### 模块加载 ########

loadmodule "tm.so"
loadmodule "sl.so"
loadmodule "rr.so"
loadmodule "pv.so"
loadmodule "maxfwd.so"
loadmodule "textops.so"
loadmodule "siputils.so"
loadmodule "xlog.so"
loadmodule "sanity.so"
loadmodule "nathelper.so"
loadmodule "rtpengine.so"

####### 模块参数 ########

# 事务模块
modparam("tm", "fr_timer", 5000)
modparam("tm", "fr_inv_timer", 30000)

# Record-Route
modparam("rr", "enable_full_lr", 1)
modparam("rr", "append_fromtag", 1)

# NAT 助手
modparam("nathelper", "natping_interval", 30)
modparam("nathelper", "ping_nated_only", 1)
modparam("nathelper", "sipping_bflag", "FLB_NATSIPPING")

# RTPEngine (Karl)
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")
modparam("rtpengine", "rtpengine_tout_ms", 2000)
modparam("rtpengine", "rtpengine_retr", 3)

####### 路由逻辑 ########

# 主请求路由
request_route {
    # 每请求初始检查
    route(REQINIT);

    # NAT 检测
    route(NATDETECT);

    # CANCEL 处理
    if (is_method("CANCEL")) {
        if (t_check_trans()) {
            route(RTPENGINE_DELETE);
            t_relay();
        }
        exit;
    }

    # 处理对话内请求
    if (has_totag()) {
        route(WITHINDLG);
        exit;
    }

    # 为建立对话的请求添加 Record-Route
    if (is_method("INVITE|SUBSCRIBE")) {
        record_route();
    }

    # 处理 INVITE
    if (is_method("INVITE")) {
        route(RTPENGINE_OFFER);
        t_on_reply("MANAGE_REPLY");
        t_on_failure("CALL_FAILURE");
    }

    route(RELAY);
}

# 每请求初始检查
route[REQINIT] {
    if (!mf_process_maxfwd_header("10")) {
        sl_send_reply("483", "Too Many Hops");
        exit;
    }

    if (!sanity_check("1511", "7")) {
        xlog("L_WARN", "Malformed SIP message from $si:$sp\n");
        exit;
    }
}

# NAT 检测
route[NATDETECT] {
    force_rport();
    if (nat_uac_test("19")) {
        if (is_method("REGISTER")) {
            fix_nated_register();
        } else {
            if (is_first_hop()) {
                set_contact_alias();
            }
        }
        setflag(FLT_NATS);
    }
}

# 处理对话内请求
route[WITHINDLG] {
    if (!loose_route()) {
        if (is_method("ACK")) {
            if (!t_check_trans()) {
                exit;
            }
        }
        sl_send_reply("404", "Not Found");
        exit;
    }

    if (is_method("ACK")) {
        route(NATMANAGE);
    } else if (is_method("BYE")) {
        route(RTPENGINE_DELETE);
    } else if (is_method("INVITE")) {
        # Re-INVITE
        route(RTPENGINE_OFFER);
        t_on_reply("MANAGE_REPLY");
    }

    route(RELAY);
}

# RTPEngine offer 处理
route[RTPENGINE_OFFER] {
    if (!has_body("application/sdp")) {
        return;
    }

    $var(rtpflags) = "RTP/AVP replace-origin replace-session-connection";

    if (isflagset(FLT_NATS)) {
        $var(rtpflags) = $var(rtpflags) + " SIP-source-address";
    }

    rtpengine_manage("$var(rtpflags)");
}

# RTPEngine 删除
route[RTPENGINE_DELETE] {
    rtpengine_delete();
}

# NAT 管理
route[NATMANAGE] {
    if (is_request()) {
        if (has_totag()) {
            if (check_route_param("nat=yes")) {
                setbflag(FLB_NATB);
            }
        }
    }

    if (isflagset(FLT_NATS) || isbflagset(FLB_NATB)) {
        if (is_request()) {
            if (!has_totag()) {
                if (t_is_branch_route()) {
                    add_rr_param(";nat=yes");
                }
            }
        }
    }
}

# 中继路由
route[RELAY] {
    if (!t_relay()) {
        sl_reply_error();
    }
    exit;
}

# 应答路由 — 处理响应中的 SDP
onreply_route[MANAGE_REPLY] {
    if (status =~ "[12][0-9][0-9]") {
        route(NATMANAGE);
    }

    if (has_body("application/sdp")) {
        $var(rtpflags) = "RTP/AVP replace-origin replace-session-connection";

        if (isbflagset(FLB_NATB)) {
            $var(rtpflags) = $var(rtpflags) + " SIP-source-address";
        }

        rtpengine_manage("$var(rtpflags)");
    }
}

# 失败路由
failure_route[CALL_FAILURE] {
    if (t_was_cancelled()) {
        route(RTPENGINE_DELETE);
        exit;
    }

    # 根据需要处理其他失败情况
}
```

### 标志参考

常用 rtpengine_manage 标志：

| 标志 | 说明 |
|------|------|
| `RTP/AVP` | 标准 RTP 配置文件 |
| `RTP/SAVP` | SRTP 配置文件 |
| `RTP/AVPF` | 带反馈的 RTP |
| `RTP/SAVPF` | 带反馈的 SRTP |
| `replace-origin` | 替换 SDP origin |
| `replace-session-connection` | 替换会话连接 |
| `SIP-source-address` | 使用 SIP 源作为媒体地址 |
| `symmetric` | 强制对称 RTP |
| `asymmetric` | 允许非对称 RTP |
| `ICE=remove` | 移除 ICE 属性 |
| `ICE=force` | 强制 ICE |
| `DTLS=off` | 禁用 DTLS |
| `DTLS=passive` | DTLS 被动模式 |
| `SDES-off` | 禁用 SDES |
| `SDES-on` | 启用 SDES |
| `record-call` | 启用通话录音 |
| `media-timeout=N` | 媒体超时（秒） |
| `direction=X` | 接口提示（提供两次：from、to）用于内部/外部 IP 选择 |

---

## 高可用

### 多个 Karl 实例

配置故障转移：

```kamailio
# 主备服务器
modparam("rtpengine", "rtpengine_sock",
    "udp:karl1.example.com:22222 udp:karl2.example.com:22222")
```

### 加权负载均衡

```kamailio
# 70/30 分配
modparam("rtpengine", "rtpengine_sock",
    "7 == udp:karl1.example.com:22222 3 == udp:karl2.example.com:22222")
```

### 多集合

```kamailio
# 为不同地区定义集合
modparam("rtpengine", "rtpengine_sock", "1 == udp:karl-us.example.com:22222")
modparam("rtpengine", "rtpengine_sock", "2 == udp:karl-eu.example.com:22222")

# 在路由中使用
request_route {
    if ($rd =~ "\.us\.") {
        set_rtpengine_set("1");
    } else {
        set_rtpengine_set("2");
    }
    rtpengine_manage("...");
}
```

### 数据库配置

将 Karl 服务器存储在数据库中：

```kamailio
modparam("rtpengine", "db_url", "mysql://kamailio:password@localhost/kamailio")
modparam("rtpengine", "table_name", "rtpengine")
```

---

## 故障排除

### 连接问题

```bash
# 测试 Karl 连通性
echo -n "d7:command4:pinge" | nc -u 127.0.0.1 22222

# 检查 Kamailio 日志
tail -f /var/log/kamailio.log | grep rtpengine

# 启用调试
kamctl debug 4
```

### 没有音频

1. 验证 SDP 修改：
```kamailio
request_route {
    xlog("L_INFO", "SDP before: $rb\n");
    rtpengine_manage("...");
    xlog("L_INFO", "SDP after: $rb\n");
}
```

2. 检查防火墙的 UDP 端口

3. 验证 Karl 会话创建：
```bash
curl http://127.0.0.1:8080/api/v1/sessions
```

### 单向音频

```kamailio
# 强制对称 RTP
rtpengine_manage("RTP/AVP replace-origin replace-session-connection symmetric");
```

### 监控会话

```bash
# Karl API
curl http://127.0.0.1:8080/api/v1/sessions

# Kamailio RPC
kamcmd rtpengine.show_all
```

---

## 下一步

- [设置通话录音](./setting-up-recording.md)
- [配置监控](./monitoring-prometheus.md)
- [WebRTC 到 SIP 桥接](./webrtc-sip-bridging.md)
