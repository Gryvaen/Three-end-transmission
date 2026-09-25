package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"

	"three-end-transmission/internal/config"
)

func TestAuthDisabledAllowsAll(t *testing.T) {
	_, ts := newTestServer(t, Config{})

	if code := doJSON(t, http.MethodPost, ts.URL+"/api/uploads", map[string]any{"name": "a", "size": 1}, nil); code != http.StatusCreated {
		t.Fatalf("want 201 without pin, got %d", code)
	}
	var info infoResponse
	doJSON(t, http.MethodGet, ts.URL+"/api/info", nil, &info)
	if info.PINRequired || !info.Authorized {
		t.Fatalf("unexpected info: %+v", info)
	}
}

func TestAuthPIN(t *testing.T) {
	_, ts := newTestServer(t, Config{PIN: "2468"})

	for _, path := range []string{"/api/files/" + strings.Repeat("a", 32), "/ws", "/api/uploads/" + strings.Repeat("b", 32)} {
		if code := doJSON(t, http.MethodGet, ts.URL+path, nil, nil); code != http.StatusUnauthorized {
			t.Fatalf("%s: want 401, got %d", path, code)
		}
	}

	var info infoResponse
	doJSON(t, http.MethodGet, ts.URL+"/api/info", nil, &info)
	if !info.PINRequired || info.Authorized || strings.Contains(info.JoinURL, "pin=") {
		t.Fatalf("unauthorized info leaked pin or wrong flags: %+v", info)
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	auth := func(pin string) int {
		body, _ := json.Marshal(map[string]string{"pin": pin})
		resp, err := client.Post(ts.URL+"/api/auth", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	if code := auth("2468"); code != http.StatusOK {
		t.Fatalf("correct pin: got %d", code)
	}
	resp, err := client.Post(ts.URL+"/api/uploads", "application/json", strings.NewReader(`{"name":"a","size":1}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("authorized upload: got %d", resp.StatusCode)
	}

	resp, _ = client.Get(ts.URL + "/api/info")
	_ = json.NewDecoder(resp.Body).Decode(&info)
	resp.Body.Close()
	if !info.Authorized || (info.JoinURL != "" && !strings.HasSuffix(info.JoinURL, "#pin=2468")) {
		t.Fatalf("authorized info: %+v", info)
	}

	for i := 0; i < authFailLimit; i++ {
		if code := auth("0000"); code != http.StatusUnauthorized {
			t.Fatalf("wrong pin #%d: got %d", i+1, code)
		}
	}
	if code := auth("2468"); code != http.StatusTooManyRequests {
		t.Fatalf("after %d failures: want 429, got %d", authFailLimit, code)
	}
}

// 放在反代后面：Secure cookie 取决于可信代理的 X-Forwarded-Proto，
// 限速按真实客户端 IP，一人输错不能把经同一代理的其他人锁住。
func TestAuthBehindProxy(t *testing.T) {
	trusted, _ := config.ParseTrustedProxies("127.0.0.1,::1")
	_, ts := newTestServer(t, Config{PIN: "correct-horse", TrustedProxies: trusted})

	auth := func(pin, clientIP string) *http.Response {
		body, _ := json.Marshal(map[string]string{"pin": pin})
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/auth", bytes.NewReader(body))
		req.Header.Set("X-Forwarded-For", clientIP)
		req.Header.Set("X-Forwarded-Proto", "https")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp
	}

	resp := auth("correct-horse", "198.51.100.1")
	if resp.StatusCode != http.StatusOK || len(resp.Cookies()) != 1 || !resp.Cookies()[0].Secure {
		t.Fatalf("want Secure session cookie, got %d %+v", resp.StatusCode, resp.Cookies())
	}

	for i := 0; i < authFailLimit; i++ {
		auth("wrong", "203.0.113.66")
	}
	if code := auth("correct-horse", "203.0.113.66").StatusCode; code != http.StatusTooManyRequests {
		t.Fatalf("attacker should be rate limited, got %d", code)
	}
	if code := auth("correct-horse", "198.51.100.2").StatusCode; code != http.StatusOK {
		t.Fatalf("other clients behind the same proxy must not be locked out, got %d", code)
	}
}

func TestAuthCookieNotSecureOnPlainHTTP(t *testing.T) {
	_, ts := newTestServer(t, Config{PIN: "2468"})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/auth", strings.NewReader(`{"pin":"2468"}`))
	// 未配置可信代理：伪造的 X-Forwarded-Proto 无效
	req.Header.Set("X-Forwarded-Proto", "https")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(resp.Cookies()) != 1 || resp.Cookies()[0].Secure {
		t.Fatalf("LAN http cookie must not be Secure: %+v", resp.Cookies())
	}
}
