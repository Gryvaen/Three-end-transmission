package server

import (
	"net/http"
	"testing"

	"three-end-transmission/internal/config"
)

func TestNormalizeClientIP(t *testing.T) {
	t.Run("ipv4 passthrough", func(t *testing.T) {
		got := normalizeClientIP("192.168.1.10")
		if got != "192.168.1.10" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("ipv4 mapped ipv6", func(t *testing.T) {
		got := normalizeClientIP("::ffff:192.168.1.10")
		if got != "192.168.1.10" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("loopback", func(t *testing.T) {
		got := normalizeClientIP("::1")
		if got != "127.0.0.1" && got == "::1" {
			t.Fatalf("expected loopback v4 or unchanged, got %q", got)
		}
	})
}

func TestPickAdvertiseIPs(t *testing.T) {
	env := []string{"192.168.1.5"}
	nic := []string{"192.168.1.9", "172.17.0.1"}
	host := []string{"10.0.0.2"}

	cases := []struct {
		name string
		srcs [][]string
		want string
	}{
		{"env wins and is not merged", [][]string{env, nic, host}, "192.168.1.5"},
		{"nic when env empty", [][]string{nil, nic, host}, "192.168.1.9"},
		{"host header as last resort", [][]string{nil, {"172.17.0.2"}, host}, "10.0.0.2"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := pickAdvertiseIPs(c.srcs...)
			if len(got) != 1 || got[0] != c.want {
				t.Fatalf("got %v want [%s]", got, c.want)
			}
		})
	}

	if got := pickAdvertiseIPs(nil, nil, nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestClientIPFromRequest(t *testing.T) {
	var none proxyTrust
	r := &http.Request{RemoteAddr: "192.168.50.2:54321", Header: http.Header{}}
	if got := none.ClientIP(r); got != "192.168.50.2" {
		t.Fatalf("got %q", got)
	}
}

func TestClientIPTrustedProxies(t *testing.T) {
	trusted, err := config.ParseTrustedProxies("127.0.0.1, 10.0.0.0/8")
	if err != nil {
		t.Fatal(err)
	}
	p := proxyTrust(trusted)
	req := func(remote string, h map[string]string) *http.Request {
		r := &http.Request{RemoteAddr: remote, Header: http.Header{}}
		for k, v := range h {
			r.Header.Set(k, v)
		}
		return r
	}

	cases := []struct {
		name string
		r    *http.Request
		want string
	}{
		{"untrusted peer cannot spoof XFF", req("203.0.113.9:1", map[string]string{"X-Forwarded-For": "1.2.3.4"}), "203.0.113.9"},
		{"untrusted peer cannot spoof X-Real-IP", req("203.0.113.9:1", map[string]string{"X-Real-IP": "1.2.3.4"}), "203.0.113.9"},
		{"trusted proxy XFF", req("127.0.0.1:1", map[string]string{"X-Forwarded-For": "198.51.100.7"}), "198.51.100.7"},
		{"client-forged left part ignored", req("127.0.0.1:1", map[string]string{"X-Forwarded-For": "1.2.3.4, 198.51.100.7"}), "198.51.100.7"},
		{"skip trusted hops", req("127.0.0.1:1", map[string]string{"X-Forwarded-For": "198.51.100.7, 10.1.2.3"}), "198.51.100.7"},
		{"X-Real-IP fallback", req("127.0.0.1:1", map[string]string{"X-Real-IP": "198.51.100.8"}), "198.51.100.8"},
		{"no headers", req("127.0.0.1:1", nil), "127.0.0.1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := p.rawClientIP(c.r); got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}

	if p.isHTTPS(req("203.0.113.9:1", map[string]string{"X-Forwarded-Proto": "https"})) {
		t.Fatal("untrusted X-Forwarded-Proto must be ignored")
	}
	if !p.isHTTPS(req("127.0.0.1:1", map[string]string{"X-Forwarded-Proto": "https"})) {
		t.Fatal("trusted X-Forwarded-Proto https should count as https")
	}
}

func TestInfoPublicMode(t *testing.T) {
	_, ts := newTestServer(t, Config{PublicURL: "https://drop.example.com"})

	var info infoResponse
	doJSON(t, http.MethodGet, ts.URL+"/api/info", nil, &info)
	if info.JoinURL != "https://drop.example.com" || info.PublicURL != info.JoinURL ||
		len(info.URLs) != 1 || info.URLs[0] != info.JoinURL || len(info.LocalIPs) != 0 {
		t.Fatalf("public mode info: %+v", info)
	}
}
