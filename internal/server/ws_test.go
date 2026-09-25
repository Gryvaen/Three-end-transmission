package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"three-end-transmission/internal/hub"

	"github.com/gorilla/websocket"
)

type wsPeer struct {
	t    *testing.T
	id   string
	conn *websocket.Conn
}

// newDeviceKey 模拟浏览器生成的设备密钥。
func newDeviceKey() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// dialPeer 以设备密钥 key 连接；设备 ID 由服务端推导，从 welcome 中读取。
func dialPeer(t *testing.T, ts *httptest.Server, key, name string) *wsPeer {
	t.Helper()
	q := url.Values{"key": {key}, "name": {name}, "platform": {"linux"}}
	u := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws?" + q.Encode()
	conn, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("dial %s: %v", name, err)
	}
	t.Cleanup(func() { conn.Close() })
	p := &wsPeer{t: t, conn: conn}
	// welcome 在 Register 之后发出，收到即表示已加入房间
	welcome, ok := p.next("welcome", 2*time.Second)
	if !ok || welcome.Device.ID == "" {
		t.Fatalf("%s: no welcome", name)
	}
	p.id = welcome.Device.ID
	return p
}

type wsEnvelope struct {
	Type     string            `json:"type"`
	Users    []hub.Device      `json:"users"`
	Messages []hub.ChatMessage `json:"messages"`
	Device   hub.Device        `json:"device"`
	hub.ChatMessage
}

// next 读取下一条指定类型的消息；超时返回 false。
func (p *wsPeer) next(typ string, timeout time.Duration) (wsEnvelope, bool) {
	deadline := time.Now().Add(timeout)
	for {
		_ = p.conn.SetReadDeadline(deadline)
		_, data, err := p.conn.ReadMessage()
		if err != nil {
			return wsEnvelope{}, false
		}
		var env wsEnvelope
		if json.Unmarshal(data, &env) == nil && env.Type == typ {
			return env, true
		}
	}
}

func (p *wsPeer) send(to []string, text string) {
	msg := map[string]any{"type": "message", "payload": map[string]string{"kind": "text", "content": text}}
	if to != nil {
		msg["to"] = to
	}
	if err := p.conn.WriteJSON(msg); err != nil {
		p.t.Fatal(err)
	}
}

func TestDirectMessageDelivery(t *testing.T) {
	_, ts := newTestServer(t, Config{})

	a := dialPeer(t, ts, newDeviceKey(), "A")
	bKey := newDeviceKey()
	b := dialPeer(t, ts, bKey, "B")
	c := dialPeer(t, ts, newDeviceKey(), "C")

	// 私信后紧跟一条群发：C 收到的下一条必须是群发（读超时会让 gorilla 连接失效，不能靠超时判断"没收到"）
	a.send([]string{b.id}, "secret")
	a.send(nil, "hello all")

	for _, p := range []*wsPeer{a, b} {
		msg, ok := p.next("message", 2*time.Second)
		if !ok || msg.Payload.Content != "secret" || len(msg.Recipients) != 1 || msg.Recipients[0].Name != "B" {
			t.Fatalf("peer %s: ok=%v msg=%+v", p.id, ok, msg.ChatMessage)
		}
	}
	if msg, ok := c.next("message", 2*time.Second); !ok || msg.Payload.Content != "hello all" {
		t.Fatalf("C should only see the broadcast: ok=%v msg=%+v", ok, msg.ChatMessage)
	}

	// 新设备只能在历史里看到群发
	d := dialPeer(t, ts, newDeviceKey(), "D")
	hist, ok := d.next("history", 2*time.Second)
	if !ok || len(hist.Messages) != 1 || hist.Messages[0].Payload.Content != "hello all" {
		t.Fatalf("D history should only contain broadcast: %+v", hist.Messages)
	}

	// B 重连后仍能在历史里看到发给自己的私信
	b.conn.Close()
	b2 := dialPeer(t, ts, bKey, "B")
	hist, ok = b2.next("history", 2*time.Second)
	if b2.id != b.id || !ok || len(hist.Messages) != 2 {
		t.Fatalf("B history should contain private + broadcast: %+v", hist.Messages)
	}

	// 拿 B 公开的设备 ID 当密钥连接，冒充不了 B，看不到 B 的私信
	e := dialPeer(t, ts, strings.ReplaceAll(b.id, "-", ""), "E")
	hist, ok = e.next("history", 2*time.Second)
	if e.id == b.id || !ok || len(hist.Messages) != 1 {
		t.Fatalf("impersonation: id=%s history=%+v", e.id, hist.Messages)
	}

	// 接收者全部无效：丢弃而不是变成群发
	a.send([]string{"not-a-uuid", a.id}, "nobody")
	a.send(nil, "marker")
	if msg, ok := c.next("message", 2*time.Second); !ok || msg.Payload.Content != "marker" {
		t.Fatalf("invalid recipients must not broadcast: ok=%v msg=%+v", ok, msg.ChatMessage)
	}
}

// 同一浏览器的多个标签页共用设备 ID：都应保持在线、都能收到私信，不能互相踢掉
// （之前的"替换旧连接"会让两个标签页无限互踢重连）。
func TestSameDeviceIDMultipleConnections(t *testing.T) {
	_, ts := newTestServer(t, Config{})

	key := newDeviceKey()
	tab1 := dialPeer(t, ts, key, "pc")
	tab2 := dialPeer(t, ts, key, "pc")
	phone := dialPeer(t, ts, newDeviceKey(), "phone")
	id := tab1.id

	pres, ok := phone.next("presence", 2*time.Second)
	if !ok || len(pres.Users) != 2 {
		t.Fatalf("presence should list pc once plus phone: %+v", pres.Users)
	}
	var info infoResponse
	doJSON(t, http.MethodGet, ts.URL+"/api/info", nil, &info)
	if info.ClientCount != 2 {
		t.Fatalf("clientCount should count devices, got %d", info.ClientCount)
	}

	// 两个标签页都没被关闭，且都收到发给该设备的私信
	phone.send([]string{id}, "hi-tabs")
	for i, tab := range []*wsPeer{tab1, tab2} {
		if msg, ok := tab.next("message", 2*time.Second); !ok || msg.Payload.Content != "hi-tabs" {
			t.Fatalf("tab%d should stay connected and receive the private message: ok=%v", i+1, ok)
		}
	}

	// 关闭一个标签页后设备仍在线
	tab1.conn.Close()
	pres, ok = phone.next("presence", 2*time.Second)
	if !ok || len(pres.Users) != 2 {
		t.Fatalf("device should stay online while another tab is connected: %+v", pres.Users)
	}
}

func TestWebSocketRejectsCrossOrigin(t *testing.T) {
	_, ts := newTestServer(t, Config{})

	u := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws?name=x"
	_, resp, err := websocket.DefaultDialer.Dial(u, http.Header{"Origin": {"http://evil.example"}})
	if err == nil {
		t.Fatal("cross-origin websocket should be rejected")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("want 403, got %v", resp)
	}
}

func TestWebSocketAllowsPublicURLOrigin(t *testing.T) {
	_, ts := newTestServer(t, Config{PublicURL: "https://drop.example.com"})

	u := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws?name=x"
	conn, _, err := websocket.DefaultDialer.Dial(u, http.Header{"Origin": {"https://drop.example.com"}})
	if err != nil {
		t.Fatalf("public URL origin should be accepted: %v", err)
	}
	conn.Close()
	if _, _, err := websocket.DefaultDialer.Dial(u, http.Header{"Origin": {"https://evil.example"}}); err == nil {
		t.Fatal("other origins must still be rejected")
	}
}
