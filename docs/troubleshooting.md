# 常见问题

## 局域网

**手机打不开页面**

- 手机与服务端连同一个 Wi‑Fi，不要用蜂窝数据。
- 关闭路由器的 AP 隔离或访客网络。
- 放行 8787 端口：`sudo ufw allow 8787`；Windows 将网络设为「专用网络」。
- 服务端执行 `ss -tlnp 'sport = :8787'` 确认在监听。

**连接信息显示 `172.x` 或没有地址**

运行在 Docker bridge 网络中。改用 `docker-compose.host.yml`，或设置 `LANROOM_ADVERTISE_IP`。

**局域网 IP 变了**

网页重新扫描「让其他设备加入」里的二维码。Android App 可重新发现同 UUID 服务，不要求固定 IP；找不到原 UUID 时检查服务身份目录是否保留，再决定是否「切换服务器」。

## 公网部署

**WebSocket 一直显示未连接**

检查 Nginx 是否转发了 `Upgrade` / `Connection` 头，以及 `proxy_set_header Host $host`。日志中出现 `request origin not allowed` 说明 Host 没有透传。

**所有人都提示 `too many attempts`**

没有设置 `LANROOM_TRUSTED_PROXIES`。

**启动即退出**

查看日志：公网模式未设置 `LANROOM_PIN`，或 `LANROOM_PUBLIC_URL` 格式不对（必须以 `http://` / `https://` 开头且不带路径）。

## Tailscale

**`tailscale serve` 提示 `Serve is not enabled on your tailnet`**

用管理员账号打开它打印的链接，开启 Serve 后重新运行。

**打开 ts.net 地址提示证书错误或一直加载**

管理后台没有开启 HTTPS Certificates；首次访问时证书需要几秒签发，稍后刷新。

**设备列表里所有人的 IP 都是 127.0.0.1**

没有设置 `LANROOM_TRUSTED_PROXIES=127.0.0.1,::1`。

**手机打不开地址**

确认手机上的 Tailscale 已连接，并且登录的是同一个 tailnet。

## 上传

- 页面需显示「已连接」，否则上传后无法发出消息。
- 超过上限会被拒绝，调大 `LANROOM_MAX_UPLOAD_MB`；当前值：`curl -s <地址>/api/info | jq .maxUploadMb`。
- Docker 中报 `cannot save file`：重建容器，入口脚本会修正目录权限。
- 查看日志：`docker logs lanroom --tail 50`。

## Android 自动发现与 Mihomo TUN

`ApkAssets: Deleting an ApkAssets object` 是 Android 资源清理日志，单凭这些行不能判断 UDP 发现失败原因。先看 App 状态、错误提示和服务端日志，分别检查以下路径，不要把 HTTP 成功当作 UDP 成功。

1. 电脑确认新版接口：`curl --fail http://127.0.0.1:8787/api/info`，应有 `serviceId` 和 `discoveryVersion: 1`。缺少字段可能仍在运行旧容器；按部署指南重新构建并启动，`restart` 不更新镜像。
2. 手机浏览器打开 `http://电脑真实局域网IP:8787/api/info`。打不开时先检查 Wi-Fi、AP 隔离、防火墙和监听地址；App「手动连接」也可检查这条 HTTP 路径。
3. Linux 检查监听：`ss -ulnp 'sport = :8788'`。服务端日志应有 `UDP discovery started`；若有端口占用、无效配置或初始化警告，先处理对应原因。公网模式、`LANROOM_DISCOVERY=false` 不开启广播发现。
4. 确认手机与服务端真实网卡同子网；Linux Docker 优先 host。bridge 或 Docker Desktop 不保证入站接口及广播路径满足发现条件。`LANROOM_ADVERTISE_IP` 不能修复 UDP 路由。
5. Mihomo 开启时检查到手机地址的路由，例如 `ip route get 192.168.1.20`（替换为手机 IP），必要时结合 `ip rule`、策略路由和抓包检查。真实网卡应承担局域网通信，不能仅依赖规则中的 `DIRECT` 就认定没有被 TUN 接管。

有抓包权限时，可在真实网卡观察请求和回复（替换 `wlan0`，按本机策略授权）：

```bash
sudo tcpdump -ni wlan0 udp port 8788
```

重新点击发现：没有请求到达电脑，检查手机、AP、广播和路由；请求到达但无回复，检查服务日志和接口/同子网过滤；回复已发出但 App 未发现，再检查返程路由及客户端错误。请求和响应匹配且 `/api/info` 校验成功才算完整自动发现成功。

防火墙需允许局域网来源访问 TCP `8787` 和 UDP `8788`，不要无条件向公网放行。Mihomo 的 `allow-lan` 控制代理入站，不是 LanRoom 服务开关；如确认 TUN 截获局域网路径，再由用户按实际网段检查 `tun.route-exclude-address` 等配置。程序不会自动修改代理、路由或防火墙。

## 构建失败

### Flutter SDK 下的 `processResources` 无法删除目录

若错误路径是 `/opt/dev/flutter/packages/flutter_tools/gradle/build/...`，通常涉及共享 SDK 中旧构建文件的权限。当前项目会将 Flutter Gradle 插件源码复制到项目的 `mobile/android/.gradle/flutter-tools/` 后构建，避免写共享 SDK。先确认代码已更新，再从 `mobile/` 重试 `flutter pub get`、`flutter run -d <Android设备ID>`。不要用 `sudo flutter run`。

若仍失败，检查本项目构建目录是否由其他用户创建，并保留完整 Gradle 错误及 `flutter doctor -v` 输出；不要直接递归修改整个 SDK 权限。

### Docker 在下载 Go 工具链时超时

当前 Dockerfile 使用 `golang:1.26.2-alpine` 和 `GOTOOLCHAIN=local`，不再从旧版本基础镜像自动下载 Go 1.26.2。若仍出现 `go: downloading go1.26.2`，确认构建使用的是当前 Dockerfile 和目录。

模块依赖下载仍需要网络，可按[部署指南](deployment.md)传入 `--build-arg GOPROXY=...`。出现 `198.18.x.x` 等地址时结合本机 Mihomo DNS/Fake-IP、容器 DNS 和路由排查；该地址本身不能证明具体是哪一环节失败。更换模块代理不保证解决基础镜像拉取或所有容器网络问题。

## 文件丢失或任务无法继续

默认保留期是 1 小时，服务端重启会清空索引并清理旧上传文件。Docker 卷不会改变这个行为；这不是永久网盘。App 保留的待传副本可重新上传，但已完成上传并清理本地副本的文件应从原始来源重新选择。

App 下载完成后必须在系统保存入口确认导出；只下载到临时目录并不代表文件已出现在公共下载目录。下载失败从头重试，不支持下载断点。
