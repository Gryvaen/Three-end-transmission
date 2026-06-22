package server

import (
	"net"
	"net/http"
	"strings"

	"three-end-transmission/internal/netutil"
)

// AdvertiseIPv4Addresses 返回应对外展示的局域网 IP（Docker 内会过滤 172.x 容器网段）。
// 优先级：LANROOM_ADVERTISE_IP > 本机网卡 > HTTP Host 头中的 IP。
func AdvertiseIPv4Addresses(r *http.Request) []string {
	var raw []string
	raw = append(raw, netutil.EnvAdvertiseIPs()...)
	raw = append(raw, netutil.CollectLANIPv4()...)
	if r != nil {
		raw = append(raw, ipsFromHTTPHost(r.Host)...)
	}
	return netutil.FilterLANIPv4(raw)
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

	if ip.IsLoopback() || isLocalInterfaceIP(ip) {
		if ips := netutil.CollectLANIPv4(); len(ips) > 0 {
			return ips[0]
		}
		if ip.IsLoopback() {
			return "127.0.0.1"
		}
	}

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
