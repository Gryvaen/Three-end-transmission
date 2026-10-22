package discovery

import (
	"encoding/json"
	"net"
	"testing"
	"time"
)

func TestIdentity(t *testing.T) {
	dir := t.TempDir()
	a, e := Identity(dir)
	if e != nil {
		t.Fatal(e)
	}
	b, e := Identity(dir)
	if e != nil || a != b {
		t.Fatalf("identity changed: %q %q %v", a, b, e)
	}
}
func TestReply(t *testing.T) {
	valid := `{"type":"lanroom.discover","version":1,"requestId":"0123456789abcdef"}`
	for _, data := range []string{`{`, `{}`, `{"type":"lanroom.discover","version":2,"requestId":"0123456789abcdef"}`, `{"type":"lanroom.discover","version":1,"requestId":"short"}`} {
		if Reply([]byte(data), "server", 8787) != nil {
			t.Errorf("accepted %s", data)
		}
	}
	var p Packet
	if e := json.Unmarshal(Reply([]byte(valid), "server", 8787), &p); e != nil || p.ServiceID != "server" || p.Port != 8787 || p.RequestID != "0123456789abcdef" {
		t.Fatalf("bad reply: %+v %v", p, e)
	}
}
func TestLocalAddress(t *testing.T) {
	_, n, _ := net.ParseCIDR("192.168.1.10/24")
	n.IP = net.ParseIP("192.168.1.10")
	iface := net.Interface{Name: "wlan0", Flags: net.FlagUp | net.FlagBroadcast}
	if ip := LocalAddress(iface, net.ParseIP("192.168.1.20"), []net.Addr{n}); !ip.Equal(n.IP) {
		t.Fatal(ip)
	}
	if LocalAddress(iface, net.ParseIP("8.8.8.8"), []net.Addr{n}) != nil {
		t.Fatal("non-local peer accepted")
	}
	iface.Name = "Meta"
	if LocalAddress(iface, n.IP, []net.Addr{n}) != nil {
		t.Fatal("TUN accepted")
	}
	iface.Name = "docker0"
	if LocalAddress(iface, n.IP, []net.Addr{n}) != nil {
		t.Fatal("Docker accepted")
	}
}
func TestLifecycle(t *testing.T) {
	id, _ := Identity(t.TempDir())
	if _, e := Start("127.0.0.1:0", 0, id); e == nil {
		t.Fatal("invalid port accepted")
	}
	s, e := Start("127.0.0.1:0", 8787, id)
	if e != nil {
		t.Fatal(e)
	}
	addr := s.Addr().String()
	if _, e := Start(addr, 8787, id); e == nil {
		t.Fatal("duplicate bind succeeded")
	}
	done := make(chan struct{})
	go func() { s.Close(); s.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("close blocked")
	}
	s, e = Start(addr, 8787, id)
	if e != nil {
		t.Fatal(e)
	}
	s.Close()
}
