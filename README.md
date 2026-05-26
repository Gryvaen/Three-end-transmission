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

---

## 环境要求

- **Go 1.22+**（[安装说明](https://go.dev/doc/install)）
- 局域网内设备可互相访问（关闭 AP 隔离）
- Windows 防火墙需允许入站端口（首次运行会提示）

---

## 快速开始（3 步）

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

安装 Avahi：

```bash
# Arch
sudo pacman -S avahi
sudo systemctl enable --now avahi-daemon

# Debian/Ubuntu
sudo apt install avahi-daemon
sudo systemctl enable --now avahi-daemon
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
