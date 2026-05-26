package mdns

import (
	"fmt"
	"log/slog"
	"os"

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
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "lan-room"
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
