#!/usr/bin/env bash
# 将 myarch.local 写入 /etc/hosts，绕过 Meta/Clash 等代理对 mDNS 的干扰。
# 用法: sudo LANROOM_HOSTNAME=myarch ./deploy/sync-lanroom-hosts.sh
set -euo pipefail

export PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

HOSTNAME="${LANROOM_HOSTNAME:-$(hostname 2>/dev/null || uname -n)}"
MARK_BEGIN="# BEGIN LANROOM"
MARK_END="# END LANROOM"

is_lan_ip() {
	case "$1" in
		192.168.*|10.*) return 0 ;;
		*) return 1 ;;
	esac
}

pick_lan_ip() {
	local ip iface cidr

	if [ -n "${LANROOM_ADVERTISE_IP:-}" ]; then
		echo "$LANROOM_ADVERTISE_IP"
		return
	fi

	# 优先常见网卡（避免 Meta TUN 默认路由返回 198.18.x）
	for iface in wlan0 wlp0s20f3 eth0 enp0s3 eno1; do
		ip=$(ip -4 -o addr show dev "$iface" 2>/dev/null | awk '{print $4}' | head -1 | cut -d/ -f1)
		if [ -n "$ip" ] && is_lan_ip "$ip"; then
			echo "$ip"
			return
		fi
	done

	for cidr in $(ip -4 -o addr show scope global 2>/dev/null | awk '{print $4}'); do
		ip=${cidr%/*}
		if is_lan_ip "$ip"; then
			echo "$ip"
			return
		fi
	done

	# Hub 已运行时从 API 读取
	if command -v curl >/dev/null; then
		ip=$(curl -sf -m 2 http://127.0.0.1:8787/api/info 2>/dev/null \
			| sed -n 's/.*"localIps":\["\([^"]*\)".*/\1/p')
		if [ -n "$ip" ] && is_lan_ip "$ip"; then
			echo "$ip"
			return
		fi
	fi

	return 1
}

IP="$(pick_lan_ip || true)"
if [ -z "$IP" ]; then
	echo "sync-lanroom-hosts: no LAN IPv4, skip (可指定: sudo LANROOM_ADVERTISE_IP=192.168.x.x LANROOM_HOSTNAME=myarch $0)" >&2
	exit 1
fi

TMP=$(mktemp)
awk -v b="$MARK_BEGIN" -v e="$MARK_END" '
	$0 == b { skip=1; next }
	$0 == e { skip=0; next }
	!skip { print }
' /etc/hosts > "$TMP"

{
	cat "$TMP"
	echo "$MARK_BEGIN"
	echo "$IP ${HOSTNAME}.local ${HOSTNAME}"
	echo "$MARK_END"
} > /etc/hosts.new

mv /etc/hosts.new /etc/hosts
echo "sync-lanroom-hosts: ${HOSTNAME}.local -> ${IP}"
