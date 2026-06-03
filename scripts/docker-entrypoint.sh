#!/bin/sh
# 等待 Wi-Fi/有线拿到局域网 IP 后再启动 Hub，避免 mDNS 在启动瞬间失败。
set -e

has_lan_ip() {
	ip -4 -o addr show scope global 2>/dev/null | while read -r _ _ cidr _; do
		ip=${cidr%/*}
		case "$ip" in
			192.168.*|10.*)
				if [ -z "${LANROOM_ADVERTISE_IP:-}" ]; then
					export LANROOM_ADVERTISE_IP="$ip"
				fi
				return 0
				;;
		esac
	done
	return 1
}

i=0
while [ "$i" -lt 90 ]; do
	if has_lan_ip; then
		[ -n "${LANROOM_ADVERTISE_IP:-}" ] && echo "lanroom: LAN IP ready (${LANROOM_ADVERTISE_IP})"
		exec /app/lanroom "$@"
	fi
	i=$((i + 1))
	sleep 2
done

echo "lanroom: warning — no LAN IPv4 after 180s, starting anyway" >&2
exec /app/lanroom "$@"
