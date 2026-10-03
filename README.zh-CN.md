<div align="center">

# LanRoom

**在浏览器里，任意设备之间互传文字和文件。**

[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Docker](https://img.shields.io/badge/Docker-ready-2496ED?logo=docker&logoColor=white)](docs/deployment.md)

[English](README.md) | 简体中文

</div>

LanRoom 是一个 Go 单文件服务端，提供聊天式网页，用于在手机和电脑之间发送消息与文件。Android、iOS、Windows、macOS、Linux 设备打开网址或扫描二维码即可加入，无需安装任何软件。

可以运行在家里或办公室网络中的一台机器上，也可以部署到公网服务器，通过 Nginx 和 HTTPS 访问。

## 功能

- **群聊与私信**：发给所有人，或点击某台设备单独发送。
- **大文件断点续传**：分片上传，断网或刷新页面后从断点继续。
- **扫码加入**：同一网络的设备扫描页面上的二维码即可进入。
- **房间口令**：局域网可选，公网必需。
- **两种部署模式**：零配置的局域网模式；使用域名和 HTTPS 的公网模式。
- **自动清理**：消息与文件在保留期后自动删除（默认 1 小时，可配置）。

## 快速开始

需要 Go 1.26 或更高版本。

```bash
git clone https://github.com/Gryvaen/Three-end-transmission.git
cd Three-end-transmission
go run .
```

打开 <http://127.0.0.1:8787>，输入昵称，其他设备扫描「连接信息」中的二维码加入。

Linux 上使用 Docker：

```bash
docker compose -f docker-compose.host.yml up -d --build
```

## 部署

- **局域网**：在任意常开的机器上运行二进制文件或 Docker 镜像。
- **公网服务器**：放在 Nginx + HTTPS 之后，设置 `LANROOM_PUBLIC_URL` 与 `LANROOM_PIN`。
- **Tailscale**：用 `tailscale serve` 通过 HTTPS 提供给 tailnet 内的设备，配置与公网模式相同。

详细步骤见[部署指南](docs/deployment.md)。

## 文档

| 文档 | 内容 |
|------|------|
| [部署](docs/deployment.md) | 二进制、Docker、公网服务器（Nginx + HTTPS）、Tailscale |
| [配置](docs/configuration.md) | 命令行参数、环境变量、限制、安全说明 |
| [API](docs/api.md) | WebSocket 协议、HTTP 接口、curl 示例 |
| [常见问题](docs/troubleshooting.md) | 常见问题与解决办法 |

## 参与贡献

欢迎贡献代码，开发环境与规范见 [CONTRIBUTING.md](CONTRIBUTING.md)。

## 安全

发现安全漏洞请按 [SECURITY.md](SECURITY.md) 私下报告。

## 许可证

[MIT](LICENSE) © 2026 Gryvaen
