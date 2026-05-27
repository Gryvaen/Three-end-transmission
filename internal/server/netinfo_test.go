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

func TestClientIPFromRequest(t *testing.T) {
	r := &http.Request{RemoteAddr: "192.168.50.2:54321"}
	if got := ClientIP(r); got != "192.168.50.2" {
		t.Fatalf("got %q", got)
	}
}
