package mdns

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/grandcat/zeroconf"
	"three-end-transmission/internal/netutil"
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

// Register 在局域网注册 mDNS 服务。advertiseIPs 应由 server.AdvertiseIPv4Addresses 提供。
func Register(port int, advertiseIPs []string) (*Registration, error) {
	hostname := resolveHostname()
	if shouldSkipMDNS(hostname) {
		return nil, fmt.Errorf("mdns skipped in docker (set LANROOM_HOSTNAME or use host network)")
	}

	ips := netutil.FilterLANIPv4(advertiseIPs)
	if len(ips) == 0 {
		return nil, fmt.Errorf("no LAN IPv4 for mDNS (set LANROOM_ADVERTISE_IP or check Wi-Fi)")
	}

	ifaces := netutil.MulticastInterfaces()
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

func resolveHostname() string {
	hostname := strings.TrimSpace(os.Getenv("LANROOM_HOSTNAME"))
	if hostname != "" {
		return hostname
	}
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "lan-room"
	}
	return h
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
