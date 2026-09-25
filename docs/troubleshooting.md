# 常见问题

## 局域网

**手机打不开页面**

- 手机与服务端连同一个 Wi‑Fi，不要用蜂窝数据。
- 关闭路由器的 AP 隔离或访客网络。
- 放行 8787 端口：`sudo ufw allow 8787`；Windows 将网络设为「专用网络」。
- 服务端执行 `ss -tlnp | grep 8787` 确认在监听。

**连接信息显示 `172.x` 或没有地址**

运行在 Docker bridge 网络中。改用 `docker-compose.host.yml`，或设置 `LANROOM_ADVERTISE_IP`。

**局域网 IP 变了**

重新扫「连接信息」里的二维码。建议在路由器上给服务端绑定静态 DHCP 地址。

## 公网部署

**WebSocket 一直显示未连接**

检查 Nginx 是否转发了 `Upgrade` / `Connection` 头，以及 `proxy_set_header Host $host`。日志中出现 `request origin not allowed` 说明 Host 没有透传。

**所有人都提示 `too many attempts`**

没有设置 `LANROOM_TRUSTED_PROXIES`。

**启动即退出**

查看日志：公网模式未设置 `LANROOM_PIN`，或 `LANROOM_PUBLIC_URL` 格式不对（必须以 `http://` / `https://` 开头且不带路径）。

## 上传

- 页面需显示「已连接」，否则上传后无法发出消息。
- 超过上限会被拒绝，调大 `LANROOM_MAX_UPLOAD_MB`；当前值：`curl -s <地址>/api/info | jq .maxUploadMb`。
- Docker 中报 `cannot save file`：重建容器，入口脚本会修正目录权限。
- 查看日志：`docker logs lanroom --tail 50`。
