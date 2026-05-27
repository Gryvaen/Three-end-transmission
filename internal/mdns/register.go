package mdns

import (
	"fmt"
	"log/slog"
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

func Register(port int) (*Registration, error) {
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

	server, err := zeroconf.Register(
		hostname,
		ServiceType,
		Domain,
		port,
		[]string{"app=ThreeEndTransmission", "ver=1"},
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("mdns register: %w", err)
	}

	slog.Info("mDNS registered", "hostname", hostname, "url", fmt.Sprintf("http://%s.local:%d", hostname, port))

	return &Registration{
		server:   server,
		hostname: hostname,
		port:     port,
	}, nil
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
