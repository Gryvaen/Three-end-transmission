#!/usr/bin/env bash
# 在 Linux Hub 宿主机上一次性配置，尽量保证 myarch.local 长期可用。
# 用法: sudo ./deploy/setup-host-mdns.sh [主机名，默认取 hostname]
set -euo pipefail

HOSTNAME="${1:-$(hostname)}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "==> LanRoom 宿主机 mDNS 配置 (hostname=${HOSTNAME})"

if ! command -v avahi-daemon >/dev/null 2>&1; then
	echo "请先安装 avahi: pacman -S avahi  或  apt install avahi-daemon"
	exit 1
fi

# nss-mdns：让 glibc ping/浏览器能解析 *.local
if command -v pacman >/dev/null 2>&1; then
	pacman -S --needed --noconfirm avahi nss-mdns 2>/dev/null || pacman -S --needed avahi nss-mdns
elif command -v apt-get >/dev/null 2>&1; then
	apt-get install -y avahi-daemon libnss-mdns
fi

NSS_FILE="/etc/nsswitch.conf"
if [ -f "$NSS_FILE" ] && ! grep -q 'mdns4_minimal' "$NSS_FILE"; then
	echo "==> 在 nsswitch.conf 的 hosts 行加入 mdns4_minimal（需人工确认）"
	echo "    建议 hosts 行类似:"
	echo "    hosts: mymachines mdns4_minimal [NOTFOUND=return] resolve [!UNAVAIL=return] files myhostname dns"
fi

install -d /etc/avahi/services
install -m 644 "$SCRIPT_DIR/avahi/lanroom.service" /etc/avahi/services/lanroom.service

systemctl enable avahi-daemon
systemctl restart avahi-daemon

echo "==> Avahi 服务文件已安装，验证:"
avahi-resolve -n "${HOSTNAME}.local" || true
ping -c 1 -W 2 "${HOSTNAME}.local" || true

echo ""
echo "==> 请用 host 网络启动 Hub（在项目目录）:"
echo "    LANROOM_HOSTNAME=${HOSTNAME} docker compose -f docker-compose.host.yml up -d --build"
echo ""
echo "访问: http://${HOSTNAME}.local:8787"
echo "备用: http://<局域网IP>:8787"
