package netutil

import (
	"net"
	"testing"
)

func TestIsLANIPv4(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"192.168.1.1", true},
		{"10.0.0.1", true},
		{"172.17.0.1", false},
		{"8.8.8.8", false},
	}
	for _, c := range cases {
		got := IsLANIPv4(net.ParseIP(c.ip))
		if got != c.want {
			t.Fatalf("%s: got %v want %v", c.ip, got, c.want)
		}
	}
}

func TestFilterLANIPv4(t *testing.T) {
	got := FilterLANIPv4([]string{"192.168.0.1", "192.168.0.1", "172.17.0.1", "bad"})
	if len(got) != 1 || got[0] != "192.168.0.1" {
		t.Fatalf("got %v", got)
	}
}
