[English](../README.md) | **中文** | [Français](../fr/README.md) | [Español](../es/README.md)

# Karl 媒体服务器文档

欢迎阅读 Karl 媒体服务器文档。本指南将帮助您在生产环境中安装、配置和运维 Karl。

## 文档概览

### 入门指南

- [快速入门](./getting-started.md) - 5 分钟内启动 Karl
- [安装指南](./installation.md) - 详细安装说明
- [配置参考](./configuration.md) - 完整配置选项

### 操作指南

常见任务的分步指南：

- [在 Kubernetes 上部署](./how-to/deploying-kubernetes.md) - 在 Kubernetes 集群中运行 Karl
- [与 OpenSIPS 集成](./how-to/integrating-opensips.md) - 将 Karl 连接到 OpenSIPS
- [与 Kamailio 集成](./how-to/integrating-kamailio.md) - 将 Karl 连接到 Kamailio
- [设置通话录音](./how-to/setting-up-recording.md) - 启用和配置通话录音
- [使用 Prometheus 监控](./how-to/monitoring-prometheus.md) - 设置指标和告警
- [水平扩展](./how-to/scaling-horizontally.md) - 运行多个 Karl 实例
- [WebRTC 到 SIP 桥接](./how-to/webrtc-sip-bridging.md) - 将浏览器客户端连接到 SIP 网络
- [使用 TLS 加密](./how-to/securing-with-tls.md) - 为 API 和控制平面启用加密
- [故障排除指南](./how-to/troubleshooting.md) - 诊断和修复常见问题

### 参考文档

- [NG 协议参考](./reference/ng-protocol.md) - 完整的 NG 协议文档
- [REST API 参考](./reference/rest-api.md) - API 端点和示例
- [指标参考](./reference/metrics.md) - Prometheus 指标文档
- [环境变量](./reference/environment-variables.md) - 所有支持的环境变量

### 架构

- [架构概览](./architecture/overview.md) - Karl 内部工作原理
- [会话生命周期](./architecture/session-lifecycle.md) - 呼叫流程和状态管理
- [媒体处理](./architecture/media-processing.md) - RTP 处理和质量特性

---

## 快速链接

| 任务 | 文档 |
|------|------|
| 安装 Karl | [安装指南](./installation.md) |
| 配置生产环境 | [配置参考](./configuration.md) |
| 部署到 Kubernetes | [Kubernetes 操作指南](./how-to/deploying-kubernetes.md) |
| 设置监控 | [Prometheus 操作指南](./how-to/monitoring-prometheus.md) |
| 调试问题 | [故障排除指南](./how-to/troubleshooting.md) |

---

## 支持

- **GitHub Issues**：[报告问题和提出功能请求](https://github.com/loreste/karl/issues)
- **GitHub Discussions**：[提问和分享想法](https://github.com/loreste/karl/discussions)
