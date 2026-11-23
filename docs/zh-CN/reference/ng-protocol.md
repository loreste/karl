# NG 协议参考

Karl 实现的兼容 rtpengine 的 NG（Next Generation）协议完整参考。

## 目录

- [协议概述](#协议概述)
- [消息格式](#消息格式)
- [命令](#命令)
- [标志参考](#标志参考)
- [响应格式](#响应格式)
- [错误处理](#错误处理)

---

## 协议概述

NG 协议是基于 UDP 的协议，使用 bencode 编码在 SIP 代理和媒体服务器之间通信。

### 连接

- **默认端口**：UDP 22222
- **编码**：Bencode
- **请求/响应**：同步

### 基本流程

```
SIP 代理                     Karl
    │                          │
    │  ping                    │
    │─────────────────────────►│
    │                          │
    │  pong                    │
    │◄─────────────────────────│
    │                          │
```

---

## 消息格式

### 请求格式

```
<cookie> <bencoded-dictionary>
```

- **cookie**：用于匹配响应的唯一标识符（字符串）
- **bencoded-dictionary**：命令和参数

### 示例

```
abc123 d7:command5:offer7:call-id10:call-567898:from-tag9:tag-12345 3:sdp...e
```

### Bencode 基础

| 类型 | 格式 | 示例 |
|------|------|------|
| 字符串 | `<长度>:<数据>` | `4:ping` |
| 整数 | `i<数字>e` | `i42e` |
| 列表 | `l<元素>e` | `l4:spam4:eggse` |
| 字典 | `d<键><值>...e` | `d3:cow3:mooe` |

---

## 命令

### ping

健康检查命令。

**请求**：
```
d7:command4:pinge
```

**响应**：
```
d6:result4:ponge
```

**用途**：心跳保活、健康监控

---

### offer

处理新通话或已有通话的 SDP offer。

**必需参数**：

| 参数 | 类型 | 说明 |
|------|------|------|
| `command` | string | `offer` |
| `call-id` | string | 唯一通话标识符 |
| `from-tag` | string | SIP From-tag |
| `sdp` | string | SDP offer 内容 |

**可选参数**：

| 参数 | 类型 | 说明 |
|------|------|------|
| `to-tag` | string | SIP To-tag（用于 re-INVITE） |
| `flags` | list | 处理标志 |
| `replace` | list | 要替换的 SDP 元素 |
| `direction` | list | 接口选择提示（见[基于方向的接口选择](#基于方向的接口选择)） |
| `ICE` | string | ICE 处理模式 |
| `DTLS` | string | DTLS 处理模式 |
| `SDES` | string | SDES 处理模式 |
| `transport-protocol` | string | 强制传输协议 |
| `media-address` | string | 覆盖媒体地址 |

**请求示例**：
```
d7:command5:offer7:call-id10:call-567898:from-tag9:tag-123453:sdpX:v=0...e
```

**响应**：
```
d6:result2:ok3:sdpX:v=0...e
```

---

### answer

处理已有通话的 SDP answer。

**必需参数**：

| 参数 | 类型 | 说明 |
|------|------|------|
| `command` | string | `answer` |
| `call-id` | string | 通话标识符 |
| `from-tag` | string | SIP From-tag |
| `to-tag` | string | SIP To-tag |
| `sdp` | string | SDP answer 内容 |

**可选参数**：

与 `offer` 相同。

**请求示例**：
```
d7:command6:answer7:call-id10:call-567898:from-tag9:tag-123456:to-tag9:tag-678903:sdpX:v=0...e
```

**响应**：
```
d6:result2:ok3:sdpX:v=0...e
```

---

### delete

终止通话并释放资源。

**必需参数**：

| 参数 | 类型 | 说明 |
|------|------|------|
| `command` | string | `delete` |
| `call-id` | string | 通话标识符 |

**可选参数**：

| 参数 | 类型 | 说明 |
|------|------|------|
| `from-tag` | string | 删除特定腿 |
| `to-tag` | string | 删除特定腿 |
| `flags` | list | 处理标志 |

**请求示例**：
```
d7:command6:delete7:call-id10:call-56789e
```

**响应**：
```
d6:result2:oke
```

---

### query

获取通话的实时统计信息。

**必需参数**：

| 参数 | 类型 | 说明 |
|------|------|------|
| `command` | string | `query` |
| `call-id` | string | 通话标识符 |

**响应字段**：

| 字段 | 类型 | 说明 |
|------|------|------|
| `result` | string | `ok` 或错误 |
| `totals` | dict | 聚合统计 |
| `tags` | dict | 按 tag 分类的统计 |
| `created` | int | 创建时间戳 |
| `last-signal` | int | 最后信令时间戳 |

**响应示例**：
```
d6:result2:ok6:totalsd12:packets-sent i1000e16:packets-receivedi950e...ee
```

---

### list

列出所有活动通话。

**必需参数**：

| 参数 | 类型 | 说明 |
|------|------|------|
| `command` | string | `list` |

**可选参数**：

| 参数 | 类型 | 说明 |
|------|------|------|
| `limit` | int | 最大结果数 |

**响应**：
```
d6:result2:ok5:callsl d7:call-id10:call-12345 8:from-tag9:tag-11111 6:to-tag9:tag-22222 5:state6:active e ...ee
```

---

### start recording

开始通话录音。

**必需参数**：

| 参数 | 类型 | 说明 |
|------|------|------|
| `command` | string | `start recording` |
| `call-id` | string | 通话标识符 |

**可选参数**：

| 参数 | 类型 | 说明 |
|------|------|------|
| `from-tag` | string | 录制特定腿 |
| `to-tag` | string | 录制特定腿 |

**响应**：
```
d6:result2:oke
```

---

### stop recording

停止通话录音。

**必需参数**：

| 参数 | 类型 | 说明 |
|------|------|------|
| `command` | string | `stop recording` |
| `call-id` | string | 通话标识符 |

---

### block media

静音一个或两个方向的媒体。

**必需参数**：

| 参数 | 类型 | 说明 |
|------|------|------|
| `command` | string | `block media` |
| `call-id` | string | 通话标识符 |

**可选参数**：

| 参数 | 类型 | 说明 |
|------|------|------|
| `from-tag` | string | 特定腿 |
| `direction` | list | `from-tag`、`to-tag` 或两者 |

---

### unblock media

取消静音。

**参数**：与 `block media` 相同。

---

### play DTMF

注入 DTMF 音调。

**必需参数**：

| 参数 | 类型 | 说明 |
|------|------|------|
| `command` | string | `play DTMF` |
| `call-id` | string | 通话标识符 |
| `dtmf` | string | DTMF 数字（0-9、*、#） |

**可选参数**：

| 参数 | 类型 | 说明 |
|------|------|------|
| `from-tag` | string | 向此腿注入 |
| `duration` | int | 音调持续时间（毫秒） |
| `pause` | int | 数字间停顿（毫秒） |

---

### statistics

获取服务器全局统计信息。

**必需参数**：

| 参数 | 类型 | 说明 |
|------|------|------|
| `command` | string | `statistics` |

**响应字段**：

| 字段 | 说明 |
|------|------|
| `currentstatistics` | 当前计数器 |
| `totalstatistics` | 生命周期计数器 |
| `sessions` | 活动会话数 |
| `uptime` | 服务器运行时间（秒） |

---

## 基于方向的接口选择

`direction` 参数是一个包含两个字符串的列表，告诉 Karl 为每个呼叫腿使用哪个命名网络接口。这就是 OpenSIPS 和 Kamailio 等 SIP 代理选择内部和外部媒体地址的方式。

### 格式

```
"direction": ["<from-interface>", "<to-interface>"]
```

- `direction[0]` — 发起方的接口名称（在 `offer` 时使用）
- `direction[1]` — 终止方的接口名称（在 `answer` 时使用）

### 示例

```
d7:command5:offer ... 9:directionl8:internal8:externale ... e
```

这告诉 Karl："主叫在 `internal` 网络上，被叫在 `external` 网络上。"Karl 将：

1. 在接口的 `address`（本地/私有 IP）上绑定媒体套接字
2. 在 SDP `c=` 行中公布接口的 `advertise_addr`（例如 `external` 的公网弹性 IP）

### 接口解析顺序

选择接口时，Karl 按以下顺序检查：

1. 显式的 `interface` 或 `from-interface`/`to-interface` 标志
2. `direction` 数组（如上所述）
3. 对端地址自动检测（私有 → `internal`，公网 → `external`）
4. 配置中的默认接口（`media_ip` / `public_ip`）

### OpenSIPS 示例

```opensips
# 内部 PBX 呼叫外部运营商
rtpengine_manage("RTP/AVP replace-origin replace-session-connection direction=internal direction=external");
```

### Kamailio 示例

```kamailio
# 内部 PBX 呼叫外部运营商
rtpengine_manage("RTP/AVP replace-origin replace-session-connection direction=internal direction=external");
```

详见[配置参考 — 命名接口](../configuration.md#命名接口)了解如何定义接口。

---

## 标志参考

### 传输协议标志

| 标志 | 说明 |
|------|------|
| `RTP/AVP` | 纯 RTP（音视频配置文件） |
| `RTP/AVPF` | 带反馈的 RTP |
| `RTP/SAVP` | SRTP（安全 AVP） |
| `RTP/SAVPF` | 带反馈的 SRTP |
| `UDP/TLS/RTP/SAVP` | DTLS-SRTP |
| `UDP/TLS/RTP/SAVPF` | 带反馈的 DTLS-SRTP |

### SDP 操控标志

| 标志 | 说明 |
|------|------|
| `replace-origin` | 替换 SDP origin 行 |
| `replace-session-connection` | 替换会话级连接 |
| `trust-address` | 信任 SDP 地址 |
| `SIP-source-address` | 使用 SIP 源作为媒体地址 |
| `symmetric` | 强制对称 RTP |
| `asymmetric` | 允许非对称 RTP |

### ICE 标志

| 标志 | 说明 |
|------|------|
| `ICE=remove` | 移除所有 ICE 属性 |
| `ICE=force` | 强制 ICE 协商 |
| `ICE=force-relay` | 强制 TURN 中继 |

### DTLS 标志

| 标志 | 说明 |
|------|------|
| `DTLS=off` | 禁用 DTLS |
| `DTLS=passive` | DTLS 被动角色 |
| `DTLS=active` | DTLS 主动角色 |

### SDES 标志

| 标志 | 说明 |
|------|------|
| `SDES-off` | 禁用 SDES |
| `SDES-on` | 启用 SDES |
| `SDES-unencrypted_srtp` | 允许未加密的 SRTP |
| `SDES-unencrypted_srtcp` | 允许未加密的 SRTCP |

### 编解码器标志

| 标志 | 说明 |
|------|------|
| `codec-strip-all` | 移除所有编解码器 |
| `codec-offer-XXXX` | 在 offer 中添加编解码器 XXXX |
| `codec-mask-XXXX` | 移除编解码器 XXXX |
| `transcode-XXXX` | 转码为编解码器 XXXX |

### 录音标志

| 标志 | 说明 |
|------|------|
| `record-call` | 启用通话录音 |

### 接口/方向标志

| 标志 | 说明 |
|------|------|
| `direction=X` | 接口选择提示（提供两次：from、to）。见[基于方向的接口选择](#基于方向的接口选择) |
| `interface=X` | 此腿的显式接口名称 |
| `in-iface=X` / `from-interface=X` | 发起方（offer）侧的接口 |
| `out-iface=X` / `to-interface=X` | 终止方（answer）侧的接口 |
| `media-address=X` | 完全覆盖媒体 IP（绕过接口选择） |

### 媒体控制标志

| 标志 | 说明 |
|------|------|
| `media-timeout=N` | 媒体超时（秒） |
| `receive-only` | 仅接收模式 |
| `send-only` | 仅发送模式 |

---

## 响应格式

### 成功响应

```
d6:result2:ok...e
```

### 错误响应

```
d6:result5:error12:error-reason18:Invalid parameterse
```

### 常见错误原因

| 错误 | 说明 |
|------|------|
| `Unknown call-id` | 会话未找到 |
| `Invalid SDP` | SDP 格式错误 |
| `Port allocation failed` | 无可用端口 |
| `Codec negotiation failed` | 无共同编解码器 |
| `Session limit reached` | 超过最大会话数 |

---

## 错误处理

### 超时处理

如果在超时时间内没有响应：
1. 重试请求（最多配置的重试次数）
2. 将实例标记为失败
3. 故障转移到备用实例

### Bencode 解析错误

无效的 bencode 将返回：
```
d6:result5:error12:error-reason14:Parse error: ...e
```

### 会话状态错误

在无效会话状态上执行操作：
```
d6:result5:error12:error-reason20:Invalid session statee
```

---

## 测试

### 手动测试

```bash
# Ping 测试
echo -n "test123 d7:command4:pinge" | nc -u localhost 22222

# 预期响应包含 "pong"
```

### OpenSIPS/Kamailio 测试

```bash
# 在 SIP 代理配置中启用 rtpengine 模块的调试日志
# 然后进行测试通话并检查日志
```

---

## 下一步

- [REST API 参考](./rest-api.md)
- [指标参考](./metrics.md)
- [配置参考](../configuration.md)
