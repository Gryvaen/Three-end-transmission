# 部署

从仓库根目录执行以下命令。新用户先看[首页](../README.md)与[使用手册](usage.md)。Linux 需要 Android 自动发现时优先直接运行或使用 host Compose；只运行一种局域网部署方式，避免争用 TCP `8787`。

- [直接运行](#直接运行)
- [局域网：Docker](#局域网docker)
- [公网服务器：Nginx + HTTPS](#公网服务器nginx--https)
- [Tailscale：tailscale serve + HTTPS](#tailscaletailscale-serve--https)

## 直接运行

```bash
go build -o lanroom .
./lanroom -port 8787
```

需要 Go 1.26.2 或更新版本。前端已嵌入二进制，同操作系统和架构下可复制二进制运行；终端按 Ctrl+C 停止。修改网页后需要重新构建或重启 `go run .`。

上传目录默认位于系统临时目录。可设置 `LANROOM_UPLOAD_DIR` 指定工作目录，但启动时仍会清理旧文件；不要指向包含其他重要文件的目录。直接运行不会自动读取 `.env`，应使用 shell 环境变量传入配置。

## 局域网：Docker

Linux 使用 host 网络，自动识别局域网 IP：

```bash
docker compose -f docker-compose.host.yml up -d --build
```

Docker 构建使用 Go 1.26.2，直接使用镜像内的工具链，不额外下载 Go 编译器。若当前网络无法连接默认 Go 模块代理，可指定依赖代理后构建：

```bash
GOPROXY=https://goproxy.cn,direct docker compose -f docker-compose.host.yml build lanroom
docker compose -f docker-compose.host.yml up -d lanroom
```

直接使用 Docker 构建时，对应参数为 `--build-arg GOPROXY=https://goproxy.cn,direct`。

macOS / Windows 使用 Docker Desktop 时，可用端口映射并指定宿主机实际局域网 IP（替换下方示例地址）：

```bash
LANROOM_ADVERTISE_IP=192.168.1.10 docker compose up -d --build
```

该 bridge 配置只发布 TCP HTTP 端口，适合网页或 App 手动连接，不保证 UDP 自动发现。Docker Desktop 4.34+ 可选择开启 host networking，但容器不能直接访问宿主机网卡，不能据此认定本项目的广播发现可用，详见 [Docker 官方说明](https://docs.docker.com/engine/network/drivers/host/)。自动发现应单独实测。

Compose 读取项目目录 `.env` 做变量替换；只有 YAML 中显式传入 `environment` 的变量才会进入容器。例如现有局域网配置支持：

```dotenv
LANROOM_PIN=2468
LANROOM_RETENTION=24h
LANROOM_MAX_UPLOAD_MB=2048
```

上传工作文件保存在 Docker 卷 `lanroom-uploads`，服务重启后仍会清理。`LANROOM_PORT` 只影响 bridge 的宿主机 TCP 映射；host 模式默认监听 8787，不使用该映射变量。

所有 Compose 构建均可用下面的通用方式指定模块代理：

```bash
docker compose -f docker-compose.host.yml build --build-arg GOPROXY=https://goproxy.cn,direct
docker compose -f docker-compose.host.yml up -d
```

使用其他部署文件时替换 `-f` 后的文件名。

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
LANROOM_PIN='请替换成至少8位的口令' \
LANROOM_LISTEN=127.0.0.1:8787 \
LANROOM_TRUSTED_PROXIES=127.0.0.1,::1 \
LANROOM_UPLOAD_DIR=/var/lib/lanroom/uploads \
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
LANROOM_PIN='请替换成至少8位的口令' \
LANROOM_LISTEN=127.0.0.1:8788 \
LANROOM_TRUSTED_PROXIES=127.0.0.1,::1 \
./lanroom
```

使用 `tailscale serve status` 查看当前转发。若本实例占用默认 HTTPS 443，可使用 `tailscale serve --https=443 off` 关闭该入口，执行前确认不会影响同入口其他服务。`tailscale serve reset` 会清除全部 Serve 配置，不应作为日常停止单个房间的命令。命令语义见 [Tailscale 官方参考](https://tailscale.com/docs/reference/tailscale-cli/serve)。

### 3. 验证

```bash
curl -s https://lanroom.tail1234.ts.net/api/info | jq '{joinUrl, pinRequired}'
# { "joinUrl": "https://lanroom.tail1234.ts.net", "pinRequired": true }
```

Android App 不会通过局域网广播找到这个 HTTPS 实例，需要「手动连接」输入完整地址；App 重启后可能需要重新手动连接。这里的 8788 是回环 HTTP **TCP** 端口，与局域网发现 **UDP** 8788 不冲突。

出现问题见 [常见问题](troubleshooting.md#tailscale)。

## 服务身份持久化

Go 使用 `LANROOM_DATA_DIR/service-id` 保存服务 UUID，默认目录为 `<用户配置目录>/lanroom`。现有 Compose 仅挂载上传卷，没有持久化身份目录，因此重建容器可能改变 UUID。App 找不到原 UUID 时请用「切换服务器」选择新实例。

需要稳定身份时，在仓库根目录新建本地 `compose.identity.yml`（以下适用于 Linux host 配置）：

```yaml
services:
  lanroom:
    environment:
      LANROOM_DATA_DIR: /app/state
    volumes:
      - lanroom-state:/app/state
volumes:
  lanroom-state:
```

首次启用或从未持久化的容器迁移时，可能生成新 UUID。此后所有启动、更新和停止命令都要使用同一组文件：

```bash
docker compose -f docker-compose.host.yml -f compose.identity.yml up -d --build
```

`/app` 由镜像入口脚本设置可写权限；身份目录应与 `/data/uploads` 分开，上传目录会被启动清理。不同独立实例应使用各自的身份卷。自定义发现端口或关闭发现也需在 override 的 `environment` 显式配置，仅写 `.env` 不会自动生效。

## 更新、日志与停止

以下以 Linux host 配置为例，其他方式替换文件名；配置身份 override 时每条 Compose 命令都加上它。

```bash
# 确认本地修改已妥善处理，再更新源码
git pull --ff-only
# 重建镜像并重建容器；单纯 restart 不会更新代码
docker compose -f docker-compose.host.yml up -d --build
# 查看状态和日志
docker compose -f docker-compose.host.yml ps
docker compose -f docker-compose.host.yml logs --tail 100
# 检查 HTTP；成功不代表 UDP 发现成功
curl --fail http://127.0.0.1:8787/api/info
# 停止并移除容器，保留命名卷
docker compose -f docker-compose.host.yml down
```

更新、重启和停止都会中断正在进行的传输。即便保留卷，下次启动仍清理旧上传文件；先通知使用者保存需要的文件。不要使用 `down -v` 保留数据，它会删除命名卷（包括已配置的身份卷）。

直接运行时停止旧进程、重新 `go build -o lanroom .`，然后用原环境变量启动。Tailscale 实例停止容器不会自动删除 Serve 转发，需单独检查 Serve 状态。
