package server

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestInfoDiscoveryIdentity(t *testing.T) {
	const id = "049501cd-04c5-4e9f-8a84-11864f0dcc56"
	s := New(Config{Port: 8787, UploadDir: t.TempDir(), ServiceID: id, PIN: "test-secret"})
	r := httptest.NewRequest("GET", "http://192.168.1.2:8787/api/info", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	var result map[string]any
	if e := json.Unmarshal(w.Body.Bytes(), &result); e != nil {
		t.Fatal(e)
	}
	if result["serviceId"] != id || result["discoveryVersion"] != float64(1) || result["pinRequired"] != true || result["authorized"] != false {
		t.Fatalf("unexpected info: %v", result)
	}
	if result["port"] != float64(8787) {
		t.Fatal("existing fields lost")
	}
}
