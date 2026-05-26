package server

import (
	"net"
)

func LocalIPv4Addresses() []string {
	var addrs []string

	ifaces, err := net.Interfaces()
	if err != nil {
		return addrs
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
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
			addrs = append(addrs, ip.String())
		}
	}

	return addrs
}
