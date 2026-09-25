# 配置

## 命令行参数

| 参数 | 说明 | 默认 |
|------|------|------|
| `-port` | 监听端口 | `8787` |
| `-addr` | 完整监听地址，如 `127.0.0.1:8787`，优先于 `-port` 与 `LANROOM_LISTEN` | 空 |

## 环境变量

| 变量 | 说明 | 默认 |
|------|------|------|
| `LANROOM_PIN` | 房间口令，为空表示不启用 | 空 |
| `LANROOM_RETENTION` | 消息与文件保留时长（`30m`、`24h`），最短 `1m` | `1h` |
| `LANROOM_MAX_UPLOAD_MB` | 单文件上限（MiB），最大 `4096` | `500` |
| `LANROOM_UPLOAD_DIR` | 上传目录 | `<临时目录>/three-end-transmission-uploads` |
| `LANROOM_ADVERTISE_IP` | 局域网模式下展示的 IP（逗号分隔），用于 Docker bridge 或多网卡 | 自动检测 |
| `LANROOM_PUBLIC_URL` | 公网地址，如 `https://drop.example.com`，不能带路径；设置后进入公网模式 | 空 |
| `LANROOM_LISTEN` | 监听地址 | `:<port>` |
| `LANROOM_TRUSTED_PROXIES` | 可信反向代理 IP / CIDR（逗号分隔），只有它们发来的 `X-Forwarded-*` 头会被采用 | 空 |
| `LANROOM_PORT` | 仅 `docker-compose.yml`：宿主机端口 | `8787` |

## 限制

| 项目 | 值 |
|------|-----|
| 上传分片 | 8 MiB |
| WebSocket 单条消息 | 512 KB |
| 私信接收者 | 最多 16 台设备 |
| 口令错误 | 同一客户端 IP 每分钟 5 次 |
| 登录会话 | 30 天 |

消息历史、文件索引与登录会话都在内存中，重启后清空，上传目录中遗留的文件也会被删除。

## 安全

- 口令是整个房间共享的，没有用户账号。启用后，除首页、`/api/info`、`/api/qrcode`、`/api/auth` 外的接口都需要登录。
- 反向代理后面必须配置 `LANROOM_TRUSTED_PROXIES`，否则所有人共用代理 IP，一人输错口令会锁住所有人。
- 设备 ID 由浏览器本地保存的密钥在服务端推导，看到别人的 ID 也无法冒充对方。
- 局域网模式是明文 HTTP，公网部署务必使用 HTTPS。
- 已登录设备展示的二维码包含口令，不要公开展示。
