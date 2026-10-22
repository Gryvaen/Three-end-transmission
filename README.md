# LanRoom

**在手机与电脑之间发送文字、图片和文件，支持网页和 Android App。**

简体中文 | [English](README.en.md)

LanRoom 由 Go 服务端、内嵌网页和可选的 Flutter Android 客户端组成。一台电脑运行服务端，其他设备打开网页或使用 App 加入同一个房间，由服务端中转消息和文件。

网页适用于 Android、iOS、Windows、macOS 和 Linux 浏览器，无需安装客户端。原生 App 首版面向 Android，不支持手机充当服务端，也不提供跨公网自动发现。

## 功能

- 群聊、指定设备私信、在线设备列表。
- 文字、图片、文件互传，分片上传、进度和取消；中断后可以继续上传。
- 网页支持粘贴图片、拖入文件和文件夹；文件夹打包为 ZIP 上传。
- Android 使用局域网 UDP 广播自动发现服务，记忆服务 UUID，IP 改变后重新寻找。
- 网页二维码加入、可选房间口令、断线重连。
- 支持局域网、Nginx HTTPS 和 Tailscale 部署。

## 快速开始：先让两台设备互通

需要 Go **1.26.2 或更新版本**；或在 Linux 上使用 Docker Engine 和 Compose。默认 HTTP/WebSocket 使用 TCP `8787`，Android 自动发现使用 UDP `8788`。

### 方式一：直接运行

```bash
git clone https://github.com/Gryvaen/Three-end-transmission.git
cd Three-end-transmission
go run .
```

保持终端运行，在电脑打开 <http://127.0.0.1:8787>。其他设备需与电脑处于允许互访的同一局域网，打开页面上的「让其他设备加入」，扫描二维码或输入电脑的局域网地址，例如 `http://192.168.1.10:8787`。示例 IP 要替换成实际地址；手机上的 `127.0.0.1` 指向手机自己。

需要口令时，在启动前设置环境变量（以下为 Linux/macOS shell 示例）：

```bash
LANROOM_PIN='请替换成自己的口令' go run .
```

### 方式二：Linux Docker

在仓库根目录运行：

```bash
docker compose -f docker-compose.host.yml up -d --build
docker compose -f docker-compose.host.yml logs --tail 50
```

同样打开电脑的 `8787` 端口。Linux host 网络用于保留真实局域网接口，适合 Android 自动发现。Docker Desktop 用户请看[部署指南](docs/deployment.md)，不要直接假定其广播行为与 Linux 相同。

### 使用 Android App

电脑保持新版 Go 服务运行，手机连接同一局域网，然后安装自行构建的 APK。构建方法见 [Android 指南](mobile/README.md)，仓库不保证提供现成的 APK 下载。

打开 App：首次发现一个服务时自动连接，多个时选择；需要口令时输入口令。看到设备列表后，先与电脑网页互发一条文字，再尝试小文件。没找到服务时进入「连接排查」，也可用「手动连接」验证 HTTP 是否可达。

## 开始使用

1. 两台设备加入同一个服务，设置易识别的昵称，确认双方出现在在线设备列表。
2. 默认发给所有设备；选中目标设备后发送私信，发送前检查接收者。
3. 网页用「添加文件」，App 用「发送文件」；接收端点击文件消息下载。
4. Android 下载完成后仍需在系统保存界面选择位置并确认，才能导出文件。

详细图景和操作步骤见[使用手册](docs/usage.md)。

## 数据与网络边界

默认单文件上限为 **500 MiB**，消息与文件保留 **1 小时**。服务端消息、文件索引和登录会话仅存在内存中；重启会清空它们，并删除上传目录中的旧文件。Docker 上传卷也不等于长期存档，请及时保存收到的文件。

Android 主要面向前台使用，不保证锁屏后持续在线。UDP 广播不跨公网，访客 Wi-Fi、AP 隔离、不同 VLAN、防火墙和代理路由可能阻止发现。Mihomo TUN 开启不代表一定失败，但仍需真实局域网路径可达；程序不会修改路由或防火墙。局域网 HTTP 不加密，不应在不可信网络上传输敏感内容。

## 文档导航

- [使用手册](docs/usage.md)：首次连接、群聊私信、文件收发、恢复连接。
- [部署指南](docs/deployment.md)：直接运行、Docker、更新维护、Nginx、Tailscale。
- [配置参考](docs/configuration.md)：环境变量、端口、服务 UUID、限制。
- [Android 指南](mobile/README.md)：构建安装、真机运行、客户端行为。
- [排错指南](docs/troubleshooting.md)：发现失败、Mihomo、构建和传输问题。
- [API 文档](docs/api.md)：HTTP、WebSocket 和 UDP 发现协议。
- [历史测试记录](mobile/TEST_RESULTS.md)：已通过及受环境限制未验证的项目，不代表所有网络环境均已通过。

参与开发见 [贡献指南](CONTRIBUTING.md)，安全问题见 [SECURITY.md](SECURITY.md)。项目使用 [MIT 许可证](LICENSE)。
