#!/bin/sh
# 修正上传目录权限后，以 lanroom 用户启动 Hub。
set -e

export PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

mkdir -p /data/uploads
if [ "$(id -u)" = "0" ]; then
	chown -R lanroom:lanroom /data/uploads /app 2>/dev/null || true
	RUN_USER=lanroom
else
	RUN_USER=""
fi

# 不在这里固化 LANROOM_ADVERTISE_IP：host 网络下 Hub 会实时扫描网卡，
# 启动时写死的 IP 在 DHCP 变更后会过期，导致连接信息出现两个地址。
if [ -n "${LANROOM_ADVERTISE_IP:-}" ]; then
	echo "lanroom: advertise IP ${LANROOM_ADVERTISE_IP}"
fi

if [ -n "$RUN_USER" ] && command -v su-exec >/dev/null; then
	exec su-exec "$RUN_USER" /app/lanroom "$@"
fi
exec /app/lanroom "$@"
