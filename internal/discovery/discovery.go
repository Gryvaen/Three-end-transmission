// Package discovery implements link-local IPv4 discovery without advertising tunnel addresses.
package discovery

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/net/ipv4"
	"three-end-transmission/internal/netutil"
)

const DefaultPort = 8788

type Packet struct {
	Type      string `json:"type"`
	Version   int    `json:"version"`
	RequestID string `json:"requestId"`
	ServiceID string `json:"serviceId,omitempty"`
	Port      int    `json:"port,omitempty"`
}

// Identity uses an exclusive create so concurrent servers share one persisted UUID.
func Identity(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "service-id")
	for i := 0; i < 20; i++ {
		b, err := os.ReadFile(path)
		if err == nil {
			if id, e := uuid.Parse(string(b)); e == nil {
				return id.String(), nil
			}
			time.Sleep(10 * time.Millisecond)
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		id := uuid.NewString()
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		_, writeErr := f.WriteString(id)
		closeErr := f.Close()
		if writeErr != nil {
			return "", writeErr
		}
		if closeErr != nil {
			return "", closeErr
		}
		return id, nil
	}
	return "", fmt.Errorf("invalid service identity in %s", path)
}

func Reply(data []byte, serviceID string, port int) []byte {
	var req Packet
	if len(data) > 1024 || json.Unmarshal(data, &req) != nil || req.Type != "lanroom.discover" || req.Version != 1 || len(req.RequestID) < 16 || len(req.RequestID) > 64 {
		return nil
	}
	for _, c := range req.RequestID {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
			return nil
		}
	}
	b, _ := json.Marshal(Packet{Type: "lanroom.offer", Version: 1, RequestID: req.RequestID, ServiceID: serviceID, Port: port})
	return b
}

// LocalAddress refreshes interface addresses for every request and only replies to
// directly attached peers, preventing discovery from becoming an Internet reflector.
func LocalAddress(iface net.Interface, peer net.IP, addrs []net.Addr) net.IP {
	if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagBroadcast == 0 || netutil.IsVirtualInterface(iface.Name) {
		return nil
	}
	for _, addr := range addrs {
		n, ok := addr.(*net.IPNet)
		if ok && n.IP.To4() != nil && n.IP.IsPrivate() && n.Contains(peer) {
			return n.IP.To4()
		}
	}
	return nil
}

type Service struct {
	conn *ipv4.PacketConn
	once sync.Once
	done chan struct{}
}

func (s *Service) Close() error {
	var err error
	s.once.Do(func() { err = s.conn.Close(); <-s.done })
	return err
}
func (s *Service) Addr() net.Addr { return s.conn.LocalAddr() }

func Start(listen string, httpPort int, serviceID string) (*Service, error) {
	if httpPort < 1 || httpPort > 65535 {
		return nil, fmt.Errorf("invalid HTTP port")
	}
	if _, err := uuid.Parse(serviceID); err != nil {
		return nil, err
	}
	raw, err := net.ListenPacket("udp4", listen)
	if err != nil {
		return nil, err
	}
	conn := ipv4.NewPacketConn(raw)
	if err := conn.SetControlMessage(ipv4.FlagInterface|ipv4.FlagDst, true); err != nil {
		raw.Close()
		return nil, err
	}
	s := &Service{conn: conn, done: make(chan struct{})}
	go func() {
		defer close(s.done)
		buf := make([]byte, 1025)
		// A global cap bounds work even when many source IPs are spoofed.
		window := time.Now()
		count := 0
		for {
			n, cm, peer, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			if time.Since(window) > time.Second {
				window = time.Now()
				count = 0
			}
			count++
			if count > 40 {
				continue
			}
			if cm == nil || cm.IfIndex == 0 {
				continue
			}
			addr, ok := peer.(*net.UDPAddr)
			if !ok {
				continue
			}
			iface, err := net.InterfaceByIndex(cm.IfIndex)
			if err != nil {
				continue
			}
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			ip := LocalAddress(*iface, addr.IP, addrs)
			if ip == nil {
				continue
			}
			reply := Reply(buf[:n], serviceID, httpPort)
			if reply == nil {
				continue
			}
			_, _ = conn.WriteTo(reply, &ipv4.ControlMessage{IfIndex: cm.IfIndex, Src: ip}, peer)
		}
	}()
	return s, nil
}
