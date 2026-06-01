package mdns

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"

	"github.com/grandcat/zeroconf"
)

const (
	ServiceType = "_lanroom._tcp"
	Domain      = "local."
)

type Registration struct {
	server   *zeroconf.Server
	hostname string
	port     int
}

// Register 在局域网注册 mDNS 服务。advertiseIPs 应为真实 LAN IPv4（如 192.168.x.x），
// 避免 Docker 网桥 172.x 被解析导致 myarch.local 打不开。
func Register(port int, advertiseIPs []string) (*Registration, error) {
	hostname := strings.TrimSpace(os.Getenv("LANROOM_HOSTNAME"))
	if hostname == "" {
		var err error
		hostname, err = os.Hostname()
		if err != nil || hostname == "" {
			hostname = "lan-room"
		}
	}

	if shouldSkipMDNS(hostname) {
		return nil, fmt.Errorf("mdns skipped in docker (set LANROOM_HOSTNAME or use host network)")
	}

	ips := normalizeAdvertiseIPs(advertiseIPs)
	if len(ips) == 0 {
		return nil, fmt.Errorf("no LAN IPv4 for mDNS (set LANROOM_ADVERTISE_IP or check Wi-Fi)")
	}

	ifaces := lanMulticastInterfaces()
	if len(ifaces) == 0 {
		return nil, fmt.Errorf("no multicast interface for mDNS")
	}

	txt := []string{"app=ThreeEndTransmission", "ver=1"}
	server, err := zeroconf.RegisterProxy(
		hostname,
		ServiceType,
		Domain,
		port,
		hostname,
		ips,
		txt,
		ifaces,
	)
	if err != nil {
		return nil, fmt.Errorf("mdns register: %w", err)
	}

	slog.Info("mDNS registered", "hostname", hostname, "ips", strings.Join(ips, ","), "url", fmt.Sprintf("http://%s.local:%d", hostname, port))

	return &Registration{
		server:   server,
		hostname: hostname,
		port:     port,
	}, nil
}

func normalizeAdvertiseIPs(raw []string) []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(ip string) {
		ip = strings.TrimSpace(ip)
		parsed := net.ParseIP(ip)
		if parsed == nil {
			return
		}
		parsed = parsed.To4()
		if parsed == nil || !isLANIPv4(parsed) {
			return
		}
		n := parsed.String()
		if _, ok := seen[n]; ok {
			return
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}

	for _, ip := range raw {
		add(ip)
	}
	for _, ip := range envAdvertiseIPs() {
		add(ip)
	}
	if len(out) == 0 {
		for _, ip := range collectLANIPv4() {
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
	return strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == ';'
	})
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
			if ip == nil || !isLANIPv4(ip) {
				continue
			}
			addrs = append(addrs, ip.String())
		}
	}
	return addrs
}

func lanMulticastInterfaces() []net.Interface {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	var out []net.Interface
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagMulticast == 0 {
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if isVirtualInterface(iface.Name) {
			continue
		}
		out = append(out, iface)
	}
	return out
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
		return false
	default:
		return false
	}
}

func (r *Registration) Hostname() string {
	return r.hostname
}

func (r *Registration) URL() string {
	return fmt.Sprintf("http://%s.local:%d", r.hostname, r.port)
}

func (r *Registration) Shutdown() {
	if r.server != nil {
		r.server.Shutdown()
	}
}

func shouldSkipMDNS(hostname string) bool {
	if _, err := os.Stat("/.dockerenv"); err != nil {
		return false
	}
	if strings.TrimSpace(os.Getenv("LANROOM_HOSTNAME")) != "" {
		return false
	}
	return isContainerHostname(hostname)
}

func isContainerHostname(name string) bool {
	if len(name) != 12 {
		return false
	}
	for _, c := range name {
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			continue
		}
		return false
	}
	return true
}
