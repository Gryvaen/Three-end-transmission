# 部署

- [直接运行](#直接运行)
- [局域网：Docker](#局域网docker)
- [公网服务器：Nginx + HTTPS](#公网服务器nginx--https)
- [Tailscale：tailscale serve + HTTPS](#tailscaletailscale-serve--https)

## 直接运行

```bash
go build -o lanroom .
./lanroom -port 8787
```

前端已嵌入二进制，复制这一个文件即可运行。上传目录默认在系统临时目录下，长期运行建议设置 `LANROOM_UPLOAD_DIR`。

## 局域网：Docker

Linux 使用 host 网络，自动识别局域网 IP：

```bash
docker compose -f docker-compose.host.yml up -d --build
```

macOS / Windows 的 Docker Desktop 不支持 host 网络，使用端口映射，并手动指定宿主机 IP：

```bash
LANROOM_ADVERTISE_IP=192.168.1.10 docker compose up -d --build
```

其他设置写在项目目录的 `.env` 中，例如：

```dotenv
LANROOM_PIN=2468
LANROOM_RETENTION=24h
LANROOM_MAX_UPLOAD_MB=2048
```

上传文件保存在 Docker 卷 `lanroom-uploads`。

## 公网服务器：Nginx + HTTPS

```
浏览器 ──HTTPS/WSS──▶ Nginx :443 ──HTTP──▶ LanRoom 127.0.0.1:8787
```

设置 `LANROOM_PUBLIC_URL` 后，加入地址与二维码改用公网地址，并且必须设置 `LANROOM_PIN`，否则拒绝启动。

### 1. 证书

域名解析到服务器，防火墙只放行 80 与 443：

```bash
sudo certbot certonly --nginx -d drop.example.com
```

### 2. Nginx

把 [`deploy/nginx/lanroom.conf`](../deploy/nginx/lanroom.conf) 中的 `drop.example.com` 替换为你的域名：

```bash
sudo cp deploy/nginx/lanroom.conf /etc/nginx/conf.d/lanroom.conf
sudo nginx -t && sudo systemctl reload nginx
```

Nginx 早于 1.25.1 时，删除 `http2 on;`，改为 `listen 443 ssl http2;`。

自己编写配置时，以下几项不能少：

| 配置 | 用途 |
|------|------|
| `proxy_http_version 1.1`，转发 `Upgrade` / `Connection` | WebSocket |
| `proxy_set_header Host $host` | WebSocket Origin 校验 |
| `X-Forwarded-For $remote_addr`，`X-Forwarded-Proto $scheme` | 真实客户端 IP、识别 HTTPS |
| `client_max_body_size 0`，`proxy_request_buffering off`，`proxy_buffering off` | 大文件流式传输 |
| `proxy_read_timeout 3600s` | 慢速上传、空闲 WebSocket |

### 3. 启动

```bash
cat >> .env <<'EOF'
LANROOM_PUBLIC_URL=https://drop.example.com
LANROOM_PIN=<至少 8 位的口令>
EOF
docker compose -f docker-compose.server.yml up -d --build
```

`docker-compose.server.yml` 已设置只监听 `127.0.0.1:8787` 并信任本机代理。不用 Docker 时：

```bash
LANROOM_PUBLIC_URL=https://drop.example.com \
LANROOM_PIN=<口令> \
LANROOM_LISTEN=127.0.0.1:8787 \
LANROOM_TRUSTED_PROXIES=127.0.0.1,::1 \
LANROOM_UPLOAD_DIR=/var/lib/lanroom \
./lanroom
```

### 4. 验证

```bash
curl -s https://drop.example.com/api/info | jq '{joinUrl, pinRequired}'
# { "joinUrl": "https://drop.example.com", "pinRequired": true }
```

出现问题见 [常见问题](troubleshooting.md#公网部署)。

## Tailscale：tailscale serve + HTTPS

```
tailnet 设备 ──HTTPS/WSS──▶ tailscaled :443 ──HTTP──▶ LanRoom 127.0.0.1:8788
```

设备不在同一个局域网、但都装了 Tailscale 时使用。`tailscale serve` 为 `<机器名>.<tailnet>.ts.net` 签发证书并反代到本机，作用相当于上一节的 Nginx。LanRoom 同样以公网模式运行，必须设置 `LANROOM_PIN`。只有 tailnet 内的设备能访问，每台设备都要安装 Tailscale 并登录同一个 tailnet。

这个实例可以和局域网实例（`docker-compose.host.yml`，端口 8787）同时运行：两者是独立的房间，消息不互通。

### 1. 前提

在 Tailscale 管理后台的 DNS 页面开启 **MagicDNS** 和 **HTTPS Certificates**。tailnet 第一次使用 Serve 时，`tailscale serve` 会打印一个授权链接，用管理员账号打开并开启即可。

查看本机域名（去掉末尾的 `.`）：

```bash
tailscale status --json | jq -r .Self.DNSName
```

### 2. 启动

```bash
cat >> .env <<'EOF'
LANROOM_TS_URL=https://lanroom.tail1234.ts.net
LANROOM_TS_PIN=<至少 8 位的口令>
EOF
docker compose -f docker-compose.tailscale.yml up -d --build
tailscale serve --bg 8788
```

[`docker-compose.tailscale.yml`](../docker-compose.tailscale.yml) 使用独立的项目名、端口和数据卷；变量带 `TS_` 前缀，不会给局域网实例加上口令。

不用 Docker 时：

```bash
LANROOM_PUBLIC_URL=https://lanroom.tail1234.ts.net \
LANROOM_PIN=<口令> \
LANROOM_LISTEN=127.0.0.1:8788 \
LANROOM_TRUSTED_PROXIES=127.0.0.1,::1 \
./lanroom
```

`tailscale serve status` 查看当前转发，`tailscale serve reset` 关闭。`tailscale serve` 不限制请求体大小，大文件不需要额外配置。

### 3. 验证

```bash
curl -s https://lanroom.tail1234.ts.net/api/info | jq '{joinUrl, pinRequired}'
# { "joinUrl": "https://lanroom.tail1234.ts.net", "pinRequired": true }
```

需要让 tailnet 外的人访问时可以改用 `tailscale funnel --bg 8788`。此时服务真正暴露在公网上，口令务必足够长。

出现问题见 [常见问题](troubleshooting.md#tailscale)。
