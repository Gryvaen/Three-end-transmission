package netutil

import (
	"net"
	"os"
	"strings"
)

// EnvAdvertiseIPs 解析 LANROOM_ADVERTISE_IP（逗号/空格/分号分隔）。
func EnvAdvertiseIPs() []string {
	raw := strings.TrimSpace(os.Getenv("LANROOM_ADVERTISE_IP"))
	if raw == "" {
		return nil
	}
	return strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == ';'
	})
}

// CollectLANIPv4 扫描本机网卡，返回 RFC1918 局域网 IPv4（排除 Docker 172.16/12 等）。
func CollectLANIPv4() []string {
	var addrs []string

	ifaces, err := net.Interfaces()
	if err != nil {
		return addrs
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if IsVirtualInterface(iface.Name) {
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
			if ip == nil || !IsLANIPv4(ip) {
				continue
			}
			addrs = append(addrs, ip.String())
		}
	}

	return addrs
}

// FilterLANIPv4 去重并只保留合法局域网 IPv4。
func FilterLANIPv4(raw []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, ip := range raw {
		ip = strings.TrimSpace(ip)
		parsed := net.ParseIP(ip)
		if parsed == nil {
			continue
		}
		parsed = parsed.To4()
		if parsed == nil || !IsLANIPv4(parsed) {
			continue
		}
		n := parsed.String()
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out
}

// IsVirtualInterface 是否为应忽略的虚拟/隧道网卡。
func IsVirtualInterface(name string) bool {
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

// IsLANIPv4 是否为手机可访问的局域网 IPv4（172.16/12 多为 Docker，排除）。
func IsLANIPv4(ip net.IP) bool {
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

// MulticastInterfaces 返回可用于 mDNS 组播的网卡。
func MulticastInterfaces() []net.Interface {
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
		if IsVirtualInterface(iface.Name) {
			continue
		}
		out = append(out, iface)
	}
	return out
}
