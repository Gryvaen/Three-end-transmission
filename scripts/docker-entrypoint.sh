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

for cidr in $(ip -4 -o addr show scope global 2>/dev/null | awk '{print $4}'); do
	ip=${cidr%/*}
	case "$ip" in
		192.168.*|10.*)
			if [ -z "${LANROOM_ADVERTISE_IP:-}" ]; then
				export LANROOM_ADVERTISE_IP="$ip"
			fi
			echo "lanroom: LAN IP ${LANROOM_ADVERTISE_IP}"
			break
			;;
	esac
done

if [ -n "$RUN_USER" ] && command -v su-exec >/dev/null; then
	exec su-exec "$RUN_USER" /app/lanroom "$@"
fi
exec /app/lanroom "$@"
