# 配置

配置在进程启动时读取。直接运行 Go 不自动加载 `.env`；Compose 仅将 YAML 中显式引用的 `.env` 值传入容器。变更后需要重启服务，传输和会话会中断。操作步骤见[部署指南](deployment.md)。

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
| `LANROOM_DATA_DIR` | 服务 UUID 目录，应独立持久化 | `<用户配置目录>/lanroom` |
| `LANROOM_DISCOVERY` | 精确设为 `false` 关闭 UDP 发现；公网模式不开启 | 局域网开启 |
| `LANROOM_DISCOVERY_PORT` | 发现 UDP 端口，范围 `1..65535` | `8788` |
| `LANROOM_PORT` | 仅 `docker-compose.yml`：宿主机端口 | `8787` |

## 限制

| 项目 | 值 |
|------|-----|
| 上传分片 | 8 MiB |
| WebSocket 单条消息 | 512 KiB |
| 私信接收者 | 最多 16 台设备 |
| 口令错误 | 同一客户端 IP 每分钟 5 次 |
| 登录会话 | 30 天 |

消息历史、文件索引与登录会话都在内存中，重启后清空，上传目录中遗留的文件也会被删除。

## 安全

- 口令是整个房间共享的，没有用户账号。启用后，除首页、`/api/info`、`/api/qrcode`、`/api/auth` 外的接口都需要登录。
- 反向代理后面必须配置 `LANROOM_TRUSTED_PROXIES`，否则所有人共用代理 IP，一人输错口令会锁住所有人。
- 设备 ID 由浏览器或 App 本地保存的密钥在服务端推导，看到别人的 ID 也无法冒充对方。
- 局域网模式是明文 HTTP，公网部署务必使用 HTTPS。
- 已登录设备展示的二维码包含口令，不要公开展示。

## 局域网服务发现

新版 Android 客户端通过 IPv4 UDP 自动发现服务，网页访问方式保持不变。

- `LANROOM_DISCOVERY=false`：关闭发现；公网模式默认不启动发现。
- `LANROOM_DISCOVERY_PORT`：UDP 监听端口，默认 `8788`，范围 `1..65535`；无效配置仅关闭发现并记录警告。
- `LANROOM_DATA_DIR`：服务身份数据目录，默认 `<用户配置目录>/lanroom`。`service-id` 保存持久化 UUID，请勿与上传临时文件一起清理；Docker 部署应将此目录挂载持久卷。

发现请求 `{"type":"lanroom.discover","version":1,"requestId":"<16至64位字母数字或连字符>"}`；回复为 `lanroom.offer`，回显版本、请求标识，并包含 `serviceId` 与 HTTP `port`。客户端从回复来源获取服务地址，再通过 `/api/info` 校验 `serviceId` 和 `discoveryVersion`。身份标识不是认证凭据。

仅向真实广播接口同子网内的来源回复，每秒最多处理 40 个报文。发现端口占用或接口控制信息不可用时不影响 HTTP 服务。HTTP 需监听局域网可达地址，防火墙需放行 HTTP 和 UDP 发现端口。Mihomo TUN 环境仍需确保局域网路由可达。

App 自动发现固定使用 UDP `8788`，自定义端口后需手动连接。`LANROOM_ADVERTISE_IP` 只改变展示地址，不改变 HTTP 监听、路由或发现接口选择。现有 Compose 未传入发现及身份目录变量，需显式添加 override，见[身份持久化示例](deployment.md#服务身份持久化)。
