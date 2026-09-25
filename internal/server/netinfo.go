package server

import (
	"net"
	"net/http"
	"strings"

	"three-end-transmission/internal/netutil"
)

// AdvertiseIPv4Addresses 返回应对外展示的局域网 IP（Docker 内会过滤 172.x 容器网段）。
// 优先级：LANROOM_ADVERTISE_IP > 本机网卡 > HTTP Host 头中的 IP，取第一个非空来源，不合并。
func AdvertiseIPv4Addresses(r *http.Request) []string {
	var host string
	if r != nil {
		host = r.Host
	}
	return pickAdvertiseIPs(netutil.EnvAdvertiseIPs(), netutil.CollectLANIPv4(), ipsFromHTTPHost(host))
}

// pickAdvertiseIPs 按优先级返回第一个过滤后非空的来源；合并多个来源会让过期配置与实时网卡同时出现。
func pickAdvertiseIPs(sources ...[]string) []string {
	for _, src := range sources {
		if ips := netutil.FilterLANIPv4(src); len(ips) > 0 {
			return ips
		}
	}
	return nil
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

// proxyTrust 可信反向代理列表（LANROOM_TRUSTED_PROXIES）。
// 转发头任何人都能伪造，只有 TCP 对端在列表内时才读取。
type proxyTrust []*net.IPNet

func (p proxyTrust) contains(ip net.IP) bool {
	for _, n := range p {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func peerIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return strings.Trim(host, "[]")
}

// fromProxy 请求是否直接来自可信代理。
func (p proxyTrust) fromProxy(r *http.Request) bool {
	ip := net.ParseIP(peerIP(r))
	return ip != nil && p.contains(ip)
}

// ClientIP 返回用于展示的客户端 IP。
func (p proxyTrust) ClientIP(r *http.Request) string {
	return normalizeClientIP(p.rawClientIP(r))
}

// rawClientIP 返回真实客户端地址：不经可信代理时就是 TCP 对端；
// 经可信代理时取 X-Forwarded-For 从右往左第一个非可信地址（左边的部分客户端可以随意伪造）。
func (p proxyTrust) rawClientIP(r *http.Request) string {
	peer := peerIP(r)
	if !p.fromProxy(r) {
		return peer
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		hops := strings.Split(xff, ",")
		for i := len(hops) - 1; i >= 0; i-- {
			hop := strings.TrimSpace(hops[i])
			ip := net.ParseIP(hop)
			if ip == nil {
				break
			}
			if !p.contains(ip) || i == 0 {
				return hop
			}
		}
	}
	if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); net.ParseIP(xri) != nil {
		return xri
	}
	return peer
}

// isHTTPS 请求在浏览器侧是否为 HTTPS（直连 TLS，或可信代理声明 X-Forwarded-Proto: https）。
func (p proxyTrust) isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return p.fromProxy(r) && strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
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
