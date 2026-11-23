# Karl 媒体服务器快速入门

本指南将帮助您在 5 分钟内启动并运行 Karl。

## 前提条件

- Go 1.26 或更高版本（从源码构建时需要）
- Docker（可选，用于容器化部署）
- SIP 代理（OpenSIPS 或 Kamailio）用于生产环境

## 方式一：从源码运行

```bash
# 克隆仓库
git clone https://github.com/loreste/karl.git
cd karl

# 构建
go build -o karl

# 使用默认设置运行
./karl
```

Karl 现在已使用默认配置运行：
- NG 协议：UDP 端口 22222
- RTP 端口：30000-40000
- REST API：TCP 端口 8080
- 健康检查：TCP 端口 8086
- 指标：TCP 端口 9091

## 方式二：使用 Docker 运行

```bash
# 使用 host 网络模式运行（媒体服务器推荐方式）
docker run -d \
  --name karl \
  --network host \
  loreste/karl:latest
```

或使用显式端口映射：

```bash
docker run -d \
  --name karl \
  -p 22222:22222/udp \
  -p 30000-30100:30000-30100/udp \
  -p 8080:8080 \
  -p 8086:8086 \
  -p 9091:9091 \
  loreste/karl:latest
```

## 验证安装

### 测试 NG 协议

```bash
# 发送 ping 命令
echo -n "d7:command4:pinge" | nc -u localhost 22222
```

预期响应包含 `pong`。

### 检查健康端点

```bash
curl http://localhost:8086/health
```

预期响应：
```json
{"status":"UP"}
```

### 查看指标

```bash
curl http://localhost:9091/metrics | head -20
```

## 连接到 SIP 代理

### OpenSIPS

在您的 `opensips.cfg` 中添加：

```opensips
loadmodule "rtpengine.so"
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")
```

### Kamailio

在您的 `kamailio.cfg` 中添加：

```kamailio
loadmodule "rtpengine.so"
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")
```

## 进行测试通话

1. 在 SIP 代理上注册两个 SIP 话机
2. 在它们之间发起通话
3. 验证媒体流通过 Karl：

```bash
# 检查活跃会话
curl http://localhost:8080/api/v1/sessions
```

## 下一步

- [配置参考](./configuration.md) - 为您的环境自定义 Karl
- [Kubernetes 部署](./how-to/deploying-kubernetes.md) - 部署到 Kubernetes
- [设置监控](./how-to/monitoring-prometheus.md) - 添加 Prometheus 和 Grafana
- [启用录音](./how-to/setting-up-recording.md) - 录制通话

## 常见问题

### 端口 22222 "connection refused"

Karl 未运行或绑定到了其他网络接口。检查：

```bash
# Karl 是否在运行？
ps aux | grep karl

# 监听的是哪个端口？
ss -ulnp | grep 22222
```

### 通话中没有音频

1. 检查 RTP 端口（30000-40000）是否可访问
2. 验证防火墙规则允许 UDP 流量
3. 检查 Karl 日志是否有错误：

```bash
# 如果以服务方式运行
journalctl -u karl -f

# 如果在 Docker 中运行
docker logs -f karl
```

更多帮助请参阅[故障排除指南](./how-to/troubleshooting.md)。
