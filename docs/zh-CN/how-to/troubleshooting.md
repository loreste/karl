# 故障排除指南

本指南涵盖 Karl 媒体服务器常见问题的诊断和解决方法。

## 目录

- [诊断工具](#诊断工具)
- [启动问题](#启动问题)
- [连接问题](#连接问题)
- [媒体问题](#媒体问题)
- [录音问题](#录音问题)
- [性能问题](#性能问题)
- [Kubernetes 问题](#kubernetes-问题)
- [获取帮助](#获取帮助)

---

## 诊断工具

### 检查 Karl 状态

```bash
# 进程是否在运行
ps aux | grep karl

# Systemd 状态
systemctl status karl

# Docker 状态
docker ps | grep karl
docker logs karl

# Kubernetes 状态
kubectl get pods -l app=karl
kubectl describe pod -l app=karl
```

### 测试 NG 协议

```bash
# 发送 ping 命令
echo -n "d7:command4:pinge" | nc -u localhost 22222

# 预期响应包含 "pong"
```

### 检查健康端点

```bash
# 简单健康检查
curl http://localhost:8086/health

# 详细健康检查
curl http://localhost:8086/health/detail

# Kubernetes 探针
curl http://localhost:8086/startup
curl http://localhost:8086/live
curl http://localhost:8086/ready
```

### 查看指标

```bash
# 所有指标
curl http://localhost:9091/metrics

# 活动会话数
curl -s http://localhost:9091/metrics | grep karl_sessions_active
```

### 检查日志

```bash
# Systemd
journalctl -u karl -f

# Docker
docker logs -f karl

# Kubernetes
kubectl logs -l app=karl -f

# 启用调试级别
KARL_LOG_LEVEL=debug ./karl
```

### API 查询

```bash
# 列出会话
curl http://localhost:8080/api/v1/sessions

# 服务器统计
curl http://localhost:8080/api/v1/stats

# 特定会话
curl http://localhost:8080/api/v1/sessions/{session_id}
```

---

## 启动问题

### "bind: address already in use"

其他进程正在使用该端口。

**解决方案**：
```bash
# 查找进程
sudo lsof -i :22222
sudo lsof -i :8080

# 必要时终止
sudo kill <PID>

# 或更改 Karl 的端口
KARL_NG_PORT=22223 ./karl
```

### "permission denied"（套接字权限不足）

套接字或端口权限不足。

**解决方案**：
```bash
# 创建具有正确权限的目录
sudo mkdir -p /var/run/karl
sudo chown karl:karl /var/run/karl

# 对于低于 1024 的端口
sudo setcap 'cap_net_bind_service=+ep' /usr/local/bin/karl
```

### "too many open files"

文件描述符限制过低。

**解决方案**：
```bash
# 临时修复
ulimit -n 65535

# 永久修复 — 添加到 /etc/security/limits.conf
karl soft nofile 65535
karl hard nofile 65535

# 或在 systemd 服务中
[Service]
LimitNOFILE=65535
```

### 找不到配置文件

```bash
# 检查文件是否存在
ls -la /etc/karl/config.json

# 验证路径
KARL_CONFIG_PATH=/etc/karl/config.json ./karl

# 使用默认配置
./karl  # 使用 ./config/config.json
```

### 配置无效

```bash
# 验证 JSON 格式
cat /etc/karl/config.json | jq .

# 检查常见问题
# - 缺少逗号
# - 末尾多余逗号
# - 无效的值
```

---

## 连接问题

### SIP 代理无法连接

**症状**：NG 协议命令无响应

**诊断**：
```bash
# 检查 Karl 是否在监听
ss -ulnp | grep 22222

# 从 SIP 代理测试
echo -n "d7:command4:pinge" | nc -u karl-ip 22222

# 检查防火墙
sudo iptables -L -n | grep 22222
```

**解决方案**：
1. 验证 Karl 正在运行并监听正确的接口
2. 检查防火墙规则允许 UDP 22222
3. 验证 SIP 代理配置指向正确的 IP/端口

### API 无响应

**诊断**：
```bash
# 检查 API 是否在监听
ss -tlnp | grep 8080

# 本地测试
curl http://localhost:8080/api/v1/stats

# 远程测试
curl http://karl-ip:8080/api/v1/stats
```

**解决方案**：
1. 验证配置中 API 已启用
2. 检查 TCP 8080 的防火墙规则
3. 验证绑定地址（使用 `0.0.0.0` 监听所有接口）

### Redis 连接失败

**症状**：实例之间会话未共享

**诊断**：
```bash
# 测试 Redis 连通性
redis-cli -h redis-host ping

# 检查 Karl 日志
grep -i redis /var/log/karl.log

# 验证配置
grep -A 5 '"database"' /etc/karl/config.json
```

**解决方案**：
1. 验证 Redis 正在运行
2. 检查网络连通性
3. 验证配置中的 Redis 地址
4. 如启用了 Redis 认证，检查认证配置

---

## 媒体问题

### 双向无音频

**诊断**：
```bash
# 检查会话是否已创建
curl http://localhost:8080/api/v1/sessions

# 验证 RTP 端口已分配
ss -ulnp | grep -E "3[0-9]{4}"

# 检查数据包计数器
curl http://localhost:9091/metrics | grep packets
```

**常见原因**：
1. 防火墙阻止了 RTP 端口
2. `media_ip` 配置错误
3. NAT 问题 — 私有 IP 泄漏到发往外部对端的 SDP 中
4. `interfaces` / `direction` 缺失或配置错误 — 如果 SIP 代理发送 `direction=internal direction=external` 但 Karl 没有匹配的接口配置，则回退到 `media_ip`，这可能是一个私有地址

**解决方案**：
```bash
# 开放 RTP 端口
sudo iptables -A INPUT -p udp --dport 30000:40000 -j ACCEPT

# 检查 media_ip
KARL_MEDIA_IP=192.168.1.100 ./karl

# 对于 NAT，设置 public_ip
KARL_PUBLIC_IP=203.0.113.50 ./karl
```

如果使用独立的内部/外部接口，请验证：
1. Karl 配置中每个命名接口都有正确的 `address`（绑定）和 `advertise_addr`（SDP）
2. SIP 代理发送的 `direction` 标志与这些接口名称匹配
3. 外部接口的 `advertise_addr` 是运营商可达的公网/弹性 IP

详见[配置 — 命名接口](../configuration.md#命名接口)。

### 单向音频

**常见原因**：
1. 非对称 NAT
2. 防火墙阻止了回程流量
3. SDP 操控不正确

**解决方案**：
```bash
# 在 SIP 代理中强制对称 RTP
rtpengine_manage("... symmetric");

# 检查双方向是否都有数据包
curl http://localhost:8080/api/v1/sessions/{id}
# 验证 packets_sent 和 packets_received > 0
```

### 音频质量差

**诊断**：
```bash
# 检查 RTCP 指标
curl http://localhost:9091/metrics | grep rtcp

# 关注：
# - 高丢包率（> 5%）
# - 高抖动（> 50ms）
# - 高 RTT（> 200ms）
```

**解决方案**：
1. 启用/增加 FEC 冗余度
2. 增大抖动缓冲区
3. 检查网络路径问题

```json
{
  "fec": {
    "enabled": true,
    "redundancy": 0.30,
    "adaptive_mode": true
  },
  "jitter_buffer": {
    "enabled": true,
    "max_delay": 200,
    "adaptive_mode": true
  }
}
```

### 会话未创建

**诊断**：
```bash
# 检查 NG 协议日志
grep -i "offer\|answer" /var/log/karl.log

# 验证是否发送了 SDP
tcpdump -i any port 22222 -A
```

**常见原因**：
1. 请求中 SDP 无效
2. 缺少必需参数
3. 达到会话上限

**解决方案**：
1. 检查 SIP 代理是否发送完整请求
2. 验证 call-id 和 from-tag 是否存在
3. 如有需要增加 max_sessions

---

## 录音问题

### 录音未创建

**诊断**：
```bash
# 检查录音是否启用
grep -A 10 '"recording"' /etc/karl/config.json

# 验证目录存在且可写
ls -la /var/lib/karl/recordings
touch /var/lib/karl/recordings/test.txt

# 检查录音消息
grep -i recording /var/log/karl.log
```

**解决方案**：
1. 在配置中启用录音
2. 创建具有正确权限的目录
3. 确保 SIP 代理发送了 `record-call` 标志

### 录音文件为空

**原因**：
1. 未收到媒体数据包
2. 编解码器不支持
3. 会话结束太快

**诊断**：
```bash
# 检查会话是否有媒体
curl http://localhost:8080/api/v1/sessions/{id}
# 验证 packets_received > 0

# 检查编解码器
grep -i codec /var/log/karl.log
```

### 磁盘空间问题

```bash
# 检查磁盘使用
df -h /var/lib/karl/recordings

# 查找大文件
du -sh /var/lib/karl/recordings/*

# 启用保留策略
{
  "recording": {
    "retention_days": 7
  }
}

# 手动清理
find /var/lib/karl/recordings -name "*.wav" -mtime +7 -delete
```

---

## 性能问题

### CPU 使用率高

**诊断**：
```bash
# 检查进程 CPU
top -p $(pgrep karl)

# 性能分析（如果构建时启用了 profiling）
curl http://localhost:9091/debug/pprof/profile?seconds=30 > profile.pb.gz
```

**常见原因**：
1. 并发会话太多
2. FEC 开销过高
3. 资源不足

**解决方案**：
1. 水平扩展
2. 降低 FEC 冗余度
3. 增加 CPU 分配

### 内存使用率高

**诊断**：
```bash
# 检查内存
ps aux | grep karl

# 内存指标
curl http://localhost:9091/metrics | grep memory
```

**解决方案**：
1. 减少 max_sessions
2. 减小抖动缓冲区大小
3. 添加内存限制，让编排系统处理重启

### 会话创建慢

**诊断**：
```bash
# 检查命令延迟
curl http://localhost:9091/metrics | grep ng_command_duration
```

**解决方案**：
1. 检查 Redis 延迟（如使用 Redis）
2. 减少端口分配范围
3. 水平扩展

---

## Kubernetes 问题

### Pod 无法启动

```bash
# 检查 Pod 状态
kubectl describe pod -l app=karl

# 常见问题：
# - 镜像拉取错误
# - 资源限制
# - 探针失败
# - 卷挂载问题
```

### 探针失败

```bash
# 手动测试探针
kubectl exec -it <pod> -- curl localhost:8086/startup
kubectl exec -it <pod> -- curl localhost:8086/live
kubectl exec -it <pod> -- curl localhost:8086/ready

# 检查探针配置
kubectl get pod <pod> -o yaml | grep -A 10 "Probe"
```

### 无网络连通性

```bash
# 检查服务端点
kubectl get endpoints karl

# 从集群内部测试
kubectl run test --rm -it --image=alpine -- sh
apk add curl netcat-openbsd
nc -u karl 22222

# 检查网络策略
kubectl get networkpolicies
```

### hostNetwork 问题

```bash
# 验证是否启用了 hostNetwork
kubectl get pod <pod> -o yaml | grep hostNetwork

# 检查节点 IP
kubectl get pod <pod> -o wide

# 验证节点上没有端口冲突
kubectl exec -it <pod> -- ss -ulnp
```

---

## 获取帮助

### 需要收集的信息

报告问题时请包含：

1. **Karl 版本**
```bash
./karl --version
```

2. **配置**（请脱敏处理敏感数据）
```bash
cat /etc/karl/config.json
```

3. **日志**
```bash
journalctl -u karl --since "1 hour ago" > karl-logs.txt
```

4. **指标**
```bash
curl http://localhost:9091/metrics > metrics.txt
```

5. **环境信息**
```bash
uname -a
cat /etc/os-release
```

### 支持渠道

- **GitHub Issues**：[github.com/loreste/karl/issues](https://github.com/loreste/karl/issues)
- **GitHub Discussions**：[github.com/loreste/karl/discussions](https://github.com/loreste/karl/discussions)

### 常用命令汇总

```bash
# 快速健康检查
curl -s http://localhost:8086/health | jq .

# 会话计数
curl -s http://localhost:9091/metrics | grep karl_sessions_active

# 最近的错误
grep -i error /var/log/karl.log | tail -20

# 测试 NG 协议
echo -n "d7:command4:pinge" | nc -u localhost 22222

# 端口检查
ss -ulnp | grep -E "22222|3[0-9]{4}"
```

---

## 下一步

- [配置参考](../configuration.md)
- [监控设置](./monitoring-prometheus.md)
- [Kubernetes 部署](./deploying-kubernetes.md)
