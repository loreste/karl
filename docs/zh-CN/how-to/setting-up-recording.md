# 如何设置通话录音

本指南涵盖在 Karl 媒体服务器中配置和管理通话录音。

## 目录

- [概述](#概述)
- [启用录音](#启用录音)
- [录音模式](#录音模式)
- [存储配置](#存储配置)
- [触发录音](#触发录音)
- [管理录音](#管理录音)
- [生产环境注意事项](#生产环境注意事项)
- [故障排除](#故障排除)

---

## 概述

Karl 支持专业级通话录音功能：

- 多种录音模式（混合、立体声、分离）
- WAV 输出格式（16 位 PCM）
- 自动编解码器转码
- 保留策略管理
- REST API 录音控制

---

## 启用录音

### 配置文件

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

### 环境变量

```bash
export KARL_RECORDING_ENABLED=true
export KARL_RECORDING_PATH=/var/lib/karl/recordings
```

### 验证配置

```bash
# 检查录音是否已启用
curl http://localhost:8080/api/v1/stats | jq .recording_enabled
```

---

## 录音模式

### 混合模式

双方合并为单个单声道文件。

```json
{
  "recording": {
    "mode": "mixed"
  }
}
```

**输出**：单个单声道 WAV 文件
**适用场景**：简单归档、合规性

### 立体声模式

主叫在左声道，被叫在右声道。

```json
{
  "recording": {
    "mode": "stereo"
  }
}
```

**输出**：单个立体声 WAV 文件
**适用场景**：质量分析、说话人识别

### 分离模式

每方各自独立文件。

```json
{
  "recording": {
    "mode": "separate"
  }
}
```

**输出**：两个单声道 WAV 文件（每方一个）
**适用场景**：后期处理、语音转录服务

---

## 存储配置

### 目录结构

录音按以下结构存储：

```
/var/lib/karl/recordings/
├── 2024-01-15/
│   ├── call-abc123_1705312200_stereo.wav
│   ├── call-def456_1705312500_stereo.wav
│   └── call-ghi789_1705313000_stereo.wav
└── 2024-01-16/
    └── ...
```

### 文件命名

格式：`{call_id}_{unix_timestamp}_{mode}.wav`

示例：`call-abc123_1705312200_stereo.wav`

### 存储需求

| 模式 | 比特率 | 每分钟 | 每小时 |
|------|--------|--------|--------|
| 混合（单声道） | 128 kbps | 约 1 MB | 约 60 MB |
| 立体声 | 256 kbps | 约 2 MB | 约 120 MB |
| 分离 | 256 kbps | 约 2 MB | 约 120 MB |

### 保留策略

配置自动清理：

```json
{
  "recording": {
    "retention_days": 30
  }
}
```

超过 `retention_days` 天的录音将自动删除。

### 网络存储

对于高可用部署，挂载网络存储：

```bash
# NFS 挂载
mount -t nfs storage.example.com:/recordings /var/lib/karl/recordings

# 或在 /etc/fstab 中
storage.example.com:/recordings /var/lib/karl/recordings nfs defaults 0 0
```

对于 Kubernetes，使用 PersistentVolumeClaim：

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: karl-recordings
spec:
  accessModes:
    - ReadWriteMany
  resources:
    requests:
      storage: 100Gi
  storageClassName: nfs
```

---

## 触发录音

### 方法一：NG 协议标志

在创建会话时添加 `record-call` 标志。

**OpenSIPS**：
```opensips
rtpengine_manage("RTP/AVP replace-origin replace-session-connection record-call");
```

**Kamailio**：
```kamailio
rtpengine_manage("RTP/AVP replace-origin replace-session-connection record-call");
```

### 方法二：REST API

为活动会话启动录音：

```bash
# 开始录音
curl -X POST http://localhost:8080/api/v1/recording/start \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "abc123",
    "mode": "stereo"
  }'
```

停止录音：

```bash
# 停止录音
curl -X POST http://localhost:8080/api/v1/recording/stop \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "abc123"
  }'
```

### 方法三：录制所有通话

在 SIP 代理中配置默认录音：

**OpenSIPS**（录制所有通话）：
```opensips
route {
    if (is_method("INVITE")) {
        rtpengine_manage("RTP/AVP replace-origin replace-session-connection record-call");
    }
}
```

### 方法四：条件录音

根据条件录音：

```opensips
route {
    if (is_method("INVITE")) {
        $var(flags) = "RTP/AVP replace-origin replace-session-connection";

        # 录制特定用户的通话
        if ($fU =~ "^sales_") {
            $var(flags) = $var(flags) + " record-call";
        }

        # 录制拨往特定号码的通话
        if ($rU =~ "^1800") {
            $var(flags) = $var(flags) + " record-call";
        }

        rtpengine_manage("$var(flags)");
    }
}
```

---

## 管理录音

### 列出录音

```bash
# 通过 API
curl http://localhost:8080/api/v1/recordings

# 响应
{
  "recordings": [
    {
      "id": "rec-001",
      "session_id": "abc123",
      "call_id": "call-abc123",
      "file_path": "/var/lib/karl/recordings/2024-01-15/call-abc123_1705312200_stereo.wav",
      "mode": "stereo",
      "duration_seconds": 125,
      "file_size": 3932160,
      "created_at": "2024-01-15T10:30:00Z"
    }
  ]
}
```

### 获取录音详情

```bash
curl http://localhost:8080/api/v1/recordings/rec-001
```

### 下载录音

```bash
# 从 API 获取文件路径，然后下载
curl http://localhost:8080/api/v1/recordings/rec-001/download -o recording.wav
```

### 删除录音

```bash
curl -X DELETE http://localhost:8080/api/v1/recordings/rec-001
```

### 按日期列出录音

```bash
# 列出指定日期的录音
ls -la /var/lib/karl/recordings/2024-01-15/

# 查找特定通话的录音
find /var/lib/karl/recordings -name "*abc123*"
```

---

## 生产环境注意事项

### 存储规划

计算存储需求：

```
每日存储 = 每日通话数 × 平均通话时长（分钟） × 每分钟 MB 数

示例：
- 1000 通通话/天
- 平均 5 分钟通话时长
- 2 MB/分钟（立体声）
= 1000 × 5 × 2 = 10 GB/天
= 300 GB/月
```

### 高可用

1. **共享存储**：多实例部署使用 NFS、EFS 或类似方案
2. **备份**：定期备份录音目录
3. **监控**：对磁盘空间使用设置告警

### 合规性

适用于受监管行业：

1. **静态加密**：使用加密文件系统
2. **访问控制**：限制录音目录权限
3. **审计日志**：记录录音访问行为
4. **保留期限**：配置适当的保留期

### 性能

录音对系统的影响：

| 并发录音数 | 额外 CPU | 额外内存 |
|-----------|----------|----------|
| 100 | 约 5% | 约 50 MB |
| 500 | 约 15% | 约 200 MB |
| 1000 | 约 25% | 约 400 MB |

---

## 故障排除

### 录音未创建

1. **检查录音是否启用**：
```bash
grep -A 10 '"recording"' /etc/karl/config.json
```

2. **验证目录权限**：
```bash
ls -la /var/lib/karl/recordings
# 应该对 karl 用户可写
```

3. **检查磁盘空间**：
```bash
df -h /var/lib/karl/recordings
```

4. **验证 record-call 标志是否已发送**：
```bash
# 启用调试日志
KARL_LOG_LEVEL=debug ./karl

# 查找录音消息
grep -i recording /var/log/karl.log
```

### 录音文件为空

1. **检查会话是否有媒体**：
```bash
curl http://localhost:8080/api/v1/sessions/abc123
# 验证 packets_received > 0
```

2. **验证编解码器支持**：
Karl 支持 G.711（PCMU/PCMA）和 Opus。其他编解码器可能无法正常录音。

3. **检查错误**：
```bash
grep -i "recording\|codec" /var/log/karl.log
```

### 录音质量差

1. **采样率不匹配**：确保 sample_rate 与您的编解码器匹配
2. **网络问题**：高丢包率影响录音质量
3. **抖动**：启用抖动缓冲区以获得更好的质量

### 磁盘空间问题

1. **启用保留策略**：
```json
{
  "recording": {
    "retention_days": 7
  }
}
```

2. **手动清理**：
```bash
# 删除超过 7 天的录音
find /var/lib/karl/recordings -name "*.wav" -mtime +7 -delete
```

3. **监控磁盘使用**：
```bash
# 添加到 cron
0 * * * * /usr/bin/df -h /var/lib/karl/recordings | mail -s "Karl disk usage" admin@example.com
```

---

## 下一步

- [使用 Prometheus 监控](./monitoring-prometheus.md)
- [与 OpenSIPS 集成](./integrating-opensips.md)
- [与 Kamailio 集成](./integrating-kamailio.md)
