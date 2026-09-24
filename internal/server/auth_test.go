package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
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
