package config

import "testing"

func TestParsePublicURL(t *testing.T) {
	ok := map[string]string{
		"":                           "",
		"https://drop.example.com":   "https://drop.example.com",
		"https://drop.example.com/ ": "https://drop.example.com",
		"http://203.0.113.5:8787":    "http://203.0.113.5:8787",
	}
	for in, want := range ok {
		got, err := ParsePublicURL(in)
		if err != nil || got != want {
			t.Fatalf("%q: got %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"drop.example.com", "ftp://x", "https://", "https://x.com/lanroom", "https://x.com/?a=1"} {
		if _, err := ParsePublicURL(in); err == nil {
			t.Fatalf("%q should be rejected", in)
		}
	}
}

func TestParseTrustedProxies(t *testing.T) {
	nets, err := ParseTrustedProxies("127.0.0.1, ::1;172.16.0.0/12")
	if err != nil || len(nets) != 3 {
		t.Fatalf("got %v, %v", nets, err)
	}
	if !nets[2].Contains([]byte{172, 17, 0, 1}) || nets[0].Contains([]byte{127, 0, 0, 2}) {
		t.Fatalf("unexpected ranges: %v", nets)
	}
	if _, err := ParseTrustedProxies("localhost"); err == nil {
		t.Fatal("hostnames should be rejected")
	}
}
