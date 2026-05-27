package server

import (
	"net"
	"net/http"
	"os"
	"strings"
)

func LANIPv4Addresses() []string {
	return collectLANIPv4()
}

// AdvertiseIPv4Addresses 返回应对外展示的局域网 IP（Docker 内会过滤 172.x 容器网段）。
// 优先级：LANROOM_ADVERTISE_IP > 本机网卡 > HTTP Host 头中的 IP。
func AdvertiseIPv4Addresses(r *http.Request) []string {
	seen := make(map[string]struct{})
	var out []string

	add := func(ip string) {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			return
		}
		parsed := net.ParseIP(ip)
		if parsed == nil {
			return
		}
		parsed = parsed.To4()
		if parsed == nil || !isLANIPv4(parsed) {
			return
		}
		normalized := parsed.String()
		if _, ok := seen[normalized]; ok {
			return
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}

	for _, ip := range envAdvertiseIPs() {
		add(ip)
	}
	for _, ip := range LANIPv4Addresses() {
		add(ip)
	}
	if r != nil {
		for _, ip := range ipsFromHTTPHost(r.Host) {
			add(ip)
		}
	}

	return out
}

func envAdvertiseIPs() []string {
	raw := strings.TrimSpace(os.Getenv("LANROOM_ADVERTISE_IP"))
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == ';'
	})
	return parts
}

func ipsFromHTTPHost(host string) []string {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil
	}
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		h = host
	}
	h = strings.Trim(h, "[]")
	if ip := net.ParseIP(h); ip != nil {
		return []string{h}
	}
	return nil
}

func collectLANIPv4() []string {
	var addrs []string

	ifaces, err := net.Interfaces()
	if err != nil {
		return addrs
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if isVirtualInterface(iface.Name) {
			continue
		}

		entries, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, entry := range entries {
			var ip net.IP
			switch v := entry.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			ip = ip.To4()
			if ip == nil {
				continue
			}
			if !isLANIPv4(ip) {
				continue
			}
			addrs = append(addrs, ip.String())
		}
	}

	return addrs
}

func ClientIP(r *http.Request) string {
	return normalizeClientIP(rawClientIP(r))
}

func rawClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return strings.Trim(host, "[]")
}

// normalizeClientIP 优先展示局域网 IPv4，避免设备列表出现公网 IPv6。
func normalizeClientIP(raw string) string {
	raw = strings.TrimSpace(strings.Trim(raw, "[]"))
	if raw == "" {
		return "未知"
	}

	ip := net.ParseIP(raw)
	if ip == nil {
		return raw
	}

	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}

	// 本机或本机网卡地址（如通过 myarch.local 的 AAAA 连入）→ 展示 Wi‑Fi IPv4
	if ip.IsLoopback() || isLocalInterfaceIP(ip) {
		if ips := LANIPv4Addresses(); len(ips) > 0 {
			return ips[0]
		}
		if ip.IsLoopback() {
			return "127.0.0.1"
		}
	}

	// 其余 IPv6 原样返回（跨设备纯 IPv6 连接时无法推断 IPv4）
	return ip.String()
}

func isLocalInterfaceIP(ip net.IP) bool {
	ifaces, err := net.Interfaces()
	if err != nil {
		return false
	}

	for _, iface := range ifaces {
		entries, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, entry := range entries {
			var netIP net.IP
			switch v := entry.(type) {
			case *net.IPNet:
				netIP = v.IP
			case *net.IPAddr:
				netIP = v.IP
			}
			if netIP != nil && netIP.Equal(ip) {
				return true
			}
		}
	}
	return false
}

func isVirtualInterface(name string) bool {
	name = strings.ToLower(name)
	virtualPrefixes := []string{
		"docker", "br-", "veth", "tun", "tap", "wg", "utun",
		"virbr", "vmnet", "vboxnet", "lo", "cni", "flannel", "meta",
	}
	for _, prefix := range virtualPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func isLANIPv4(ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}

	switch {
	case ip4[0] == 192 && ip4[1] == 168:
		return true
	case ip4[0] == 10:
		return true
	case ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31:
		// 172.16/12 多为 Docker 等虚拟网桥，手机无法访问
		return false
	default:
		return false
	}
}
