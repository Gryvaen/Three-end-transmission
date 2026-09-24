package server

import (
	"net/http"
	"testing"
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
	r := &http.Request{RemoteAddr: "192.168.50.2:54321"}
	if got := ClientIP(r); got != "192.168.50.2" {
		t.Fatalf("got %q", got)
	}
}
