# Three End Transmission · LanRoom

局域网 **聊天式互传** 骨架：Go Hub + mDNS + 二维码 + WebSocket 群聊。

Android / Windows / Linux / iOS 只需 **打开浏览器**，无需安装客户端 App。

---

## 功能概览

| 功能 | 状态 |
|------|------|
| WebSocket 群聊（文字） | ✅ |
| 在线设备列表（显示平台图标） | ✅ |
| 图片 / 文件上传与下载 | ✅ |
| mDNS（`http://主机名.local:8787`） | ✅ |
| 启动页二维码（IP 变化时扫码） | ✅ |
| 断线自动重连 | ✅ |
| Linux 命令行发送（echo/cat） | ✅ |

---

## Linux 命令行发送（lanroom-cli）

浏览器 📎 按钮在某些 Linux 桌面（如 dwm）下可能无反应，可用命令行代替：

### 编译

```bash
go build -o lanroom-cli ./cmd/lanroom-cli
```

### 用法示例

```bash
# 发文字（echo 管道）
echo "你好" | ./lanroom-cli

# 指定 Hub 地址（手机/其他机器上的 Hub）
echo "hello" | ./lanroom-cli -host http://192.168.47.224:8787

# 直接发文字
./lanroom-cli -t "一条消息"

# 发文件
./lanroom-cli ./photo.png
./lanroom-cli report.pdf

# cat 管道发文件（需 -n 指定文件名）
cat readme.md | ./lanroom-cli -n readme.md
cat image.png | ./lanroom-cli -file -n image.png

# 自定义发送者名称
./lanroom-cli -name "arch-pc" -t "来自 CLI"
```

### 环境变量（支持域名 / 动态 IP）

```bash
# 推荐：用 Hub 机器的 mDNS 主机名，IP 变了也不用改
export LANROOM_HOST=http://myarch.local:8787

# 或自动发现局域网里的 Hub（需安装 avahi）
export LANROOM_HOST=auto

# Hub 就跑在本机时，127.0.0.1 始终可用（与 DHCP 换 IP 无关）
export LANROOM_HOST=http://127.0.0.1:8787

echo "发送" | ./lanroom-cli
```

| 值 | 说明 |
|----|------|
| `http://主机名.local:8787` | **推荐**，适应动态 IP |
| `http://192.168.x.x:8787` | 局域网 IP，IP 变需更新 |
| `auto` | mDNS 自动发现 Hub |
| 未设置 | 先自动发现，失败则用 `127.0.0.1:8787` |

使用 `.local` 或 `auto` 前，请先在 Hub 机器上安装 Avahi，见 [Linux 前置条件：Avahi](#linux-前置条件avahimdns)。

> **注意：** 命令行发送需要先 **重启 Hub**（`go run .`）以加载 `/api/send` 接口。  
> 若报 `404 page not found`，通常是 mDNS 连到了**旧 Hub 进程**（以前测试留在 8790/8799 等端口）。结束旧进程后重试：
> ```bash
> pkill -f 'lanroom|three-end-trans'
> go run . -port 8787
> ```
> 或显式指定：`LANROOM_HOST=http://127.0.0.1:8787 ./lanroom-cli -t "你好"`

---

## 环境要求

- **Go 1.22+**（[安装说明](https://go.dev/doc/install)）
- 局域网内设备可互相访问（关闭 AP 隔离）
- Windows 防火墙需允许入站端口（首次运行会提示）

### Linux 前置条件：Avahi（mDNS）

Hub 所在 Linux 若需使用 `http://主机名.local:8787`、CLI 的 `LANROOM_HOST=auto` 或解析 `.local` 域名，须先安装并启动 **Avahi**：

```bash
# Arch Linux
sudo pacman -S avahi
sudo systemctl enable --now avahi-daemon
```

```bash
# Debian / Ubuntu
sudo apt install avahi-daemon
sudo systemctl enable --now avahi-daemon
```

验证是否生效（将 `myarch` 换成你的主机名）：

```bash
ping -c 1 myarch.local
avahi-resolve -n myarch.local
```

> Hub 只跑在本机、且 CLI 用 `127.0.0.1` 时，可不装 Avahi。  
> 手机浏览器访问建议优先用 **二维码 / 局域网 IP**，不依赖 Avahi。

---

## 快速开始（3 步）

### 0. （Linux）安装 Avahi 前置条件

若使用 mDNS 域名（`.local`）或 CLI 自动发现，请先完成 [Linux 前置条件：Avahi](#linux-前置条件avahimdns) 中的安装步骤。

### 1. 克隆 / 进入项目

```bash
cd /home/user/projs/projects2026/Three_end_transmission
```

### 2. 下载依赖并启动 Hub

```bash
go mod tidy
go run . -port 8787
```

看到类似日志表示成功：

```
level=INFO msg="mDNS registered" hostname=DESKTOP-XXX url=http://DESKTOP-XXX.local:8787
level=INFO msg="hub started" addr=:8787
```

### 3. 各设备打开浏览器

**任选一种方式进入：**

| 方式 | 地址示例 | 说明 |
|------|----------|------|
| mDNS（推荐） | `http://你的电脑名.local:8787` | IP 变了也能用 |
| 局域网 IP | `http://192.168.x.x:8787` | 启动后终端会打印 |
| 二维码 | 页面点「查看连接地址 / 二维码」 | 手机扫一下 |

每台设备：输入昵称 → **进入聊天室** → 即可像群聊一样发文字、图片、文件。

---

## 编译为单文件（可选）

```bash
go build -o lanroom .
./lanroom -port 8787
```

可将 `lanroom` 复制到任意 Windows / Linux 机器运行。

---

## 项目结构

```
Three_end_transmission/
├── main.go                 # 入口：启动 HTTP + mDNS
├── internal/
│   ├── hub/                # WebSocket 群聊 Hub
│   ├── mdns/               # mDNS 服务注册
│   └── server/             # HTTP API、文件上传
├── web/
│   ├── index.html          # 聊天界面
│   └── static/
│       ├── app.js
│       └── style.css
└── README.md
```

---

## API 说明

### WebSocket

```
ws://<host>:8787/ws?name=设备名&platform=android
```

**客户端发送：**

```json
{
  "type": "message",
  "payload": {
    "kind": "text",
    "content": "你好"
  }
}
```

**服务端广播：**

```json
{
  "type": "message",
  "from": { "id": "...", "name": "Windows PC", "platform": "windows" },
  "payload": { "kind": "text", "content": "你好" },
  "timestamp": 1710000000
}
```

**在线列表：**

```json
{
  "type": "presence",
  "users": [
    { "id": "...", "name": "Pixel", "platform": "android" }
  ]
}
```

### HTTP

| 路径 | 方法 | 说明 |
|------|------|------|
| `/api/info` | GET | 返回 mDNS 地址、本机 IP 列表、推荐加入地址 |
| `/api/qrcode` | GET | 返回加入地址的 PNG 二维码（可选 `?url=`） |
| `/api/send` | POST | 命令行/脚本发送消息（JSON） |
| `/api/upload` | POST | `multipart/form-data`，字段名 `file` |
| `/api/files/{id}` | GET | 下载已上传文件 |

---

## 动态 IP 怎么用？

1. **优先收藏 mDNS 地址**：`http://主机名.local:8787`（DHCP 换 IP 不影响）
2. **IP 变了**：在 Hub 所在机器打开页面 → 「连接信息」→ 扫新二维码
3. **进阶（可选）**：在路由器给 Hub 机器做 DHCP 静态绑定

---

## 常见问题

### 手机扫码后打不开？

- 二维码现在优先使用 **局域网 IP**（如 `http://192.168.x.x:8787`），不要用 `.local` 地址
- 确认手机和 Hub **在同一 WiFi**（不是手机流量）
- 路由器关闭 **AP 隔离 / 访客网络隔离**
- Linux 防火墙放行 8787：`sudo firewall-cmd --add-port=8787/tcp` 或 `sudo ufw allow 8787`
- 若仍不行，在 Hub 机器上执行 `ss -tlnp | grep 8787` 确认服务在监听

### 手机搜不到 `.local` 地址？

- 确认手机和 Hub 在 **同一 WiFi**
- 路由器关闭 **AP 隔离 / 访客网络隔离**
- 改用 **二维码** 或 **IP 地址** 进入

### Windows 上其他设备连不上？

- 将 WiFi 设为 **专用网络（Private）**
- 防火墙放行 **8787** 端口，或允许 `lanroom.exe`

### Linux 上 mDNS 不工作？

请先确认已按 [Linux 前置条件：Avahi](#linux-前置条件avahimdns) 安装并启动 `avahi-daemon`，然后检查：

```bash
systemctl status avahi-daemon
ping -c 1 你的主机名.local
```

### iOS Safari 注意事项

- 文件上传、聊天均可用
- 剪贴板自动同步受限（需用户手动粘贴，后续版本可加「发送剪贴板」按钮）
- 可「添加到主屏幕」当 PWA 使用

### 文件存在哪？

临时目录：`<系统临时目录>/three-end-transmission-uploads/`  
Hub 重启后文件记录会丢失（MVP 行为，后续可加持久化）。

---

## 下一步可扩展

- [ ] 房间 PIN 码 / 多房间
- [ ] 剪贴板一键发送
- [ ] 大文件 WebRTC 直传（减轻 Hub 压力）
- [ ] 消息历史持久化
- [ ] Docker 一键部署

---

## 许可证

MIT（可按需修改）
