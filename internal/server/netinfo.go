package server

import (
	"net"
	"strings"
)

func LocalIPv4Addresses() []string {
	return collectIPv4(false)
}

func LANIPv4Addresses() []string {
	return collectIPv4(true)
}

func collectIPv4(lanOnly bool) []string {
	var addrs []string

	ifaces, err := net.Interfaces()
	if err != nil {
		return addrs
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if lanOnly && isVirtualInterface(iface.Name) {
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
			if lanOnly && !isLANIPv4(ip) {
				continue
			}
			addrs = append(addrs, ip.String())
		}
	}

	return addrs
}

func isVirtualInterface(name string) bool {
	name = strings.ToLower(name)
	virtualPrefixes := []string{
		"docker", "br-", "veth", "tun", "tap", "wg", "utun",
		"virbr", "vmnet", "vboxnet", "lo", "cni", "flannel",
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
		// 常见 Docker 网桥，手机无法访问
		if ip4[1] >= 17 && ip4[1] <= 19 {
			return false
		}
		return true
	default:
		return false
	}
}
