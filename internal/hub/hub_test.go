package hub

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// dialTestConn 建立一个真实可用的 WebSocket 连接，仅用作 deliverSlow 放弃投递时
// 调用 conn.Close() 的占位对象；测试不关心它的读写内容。
func dialTestConn(t *testing.T) *websocket.Conn {
	t.Helper()
	var upgrader websocket.Upgrader
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = upgrader.Upgrade(w, r, nil)
	}))
	t.Cleanup(ts.Close)

	u := "ws" + strings.TrimPrefix(ts.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("dial test conn: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// newTestClient 绕过 Register()（不需要真实的 handshake/welcome 流程），
// 直接把一个 send 缓冲区大小可控的 Client 挂到 Hub 上，方便精确制造"缓冲区打满"场景。
func newTestClient(t *testing.T, h *Hub, buf int) *Client {
	t.Helper()
	c := &Client{
		hub:    h,
		conn:   dialTestConn(t),
		send:   make(chan []byte, buf),
		device: Device{ID: "11111111-1111-1111-1111-111111111111"},
	}
	h.mu.Lock()
	h.clients[c] = true
	h.mu.Unlock()
	return c
}

func withRetryTiming(window, delay time.Duration) func() {
	origWindow, origDelay := slowClientRetryWindow, slowClientRetryDelay
	slowClientRetryWindow, slowClientRetryDelay = window, delay
	return func() { slowClientRetryWindow, slowClientRetryDelay = origWindow, origDelay }
}

// 发送队列瞬时打满不应立即断线：这正是手机端（时延/吞吐通常比同局域网电脑差）
// 之前"频繁掉线重连"的根因——缓冲区一满就被 deliver() 立刻踢掉。
func TestDeliverSlowClientRecoversWithinRetryWindow(t *testing.T) {
	defer withRetryTiming(500*time.Millisecond, 10*time.Millisecond)()

	h := New(time.Minute)
	c := newTestClient(t, h, 1)
	c.send <- []byte("first") // 占满容量为 1 的缓冲区

	h.broadcast([]byte("second")) // 缓冲区已满，应走 deliverSlow 补发路径而非立刻断线

	time.Sleep(50 * time.Millisecond)
	h.mu.RLock()
	_, stillThere := h.clients[c]
	h.mu.RUnlock()
	if !stillThere {
		t.Fatal("client should not be disconnected immediately when its send buffer is briefly full")
	}

	<-c.send // 消费占位消息，腾出空间供 deliverSlow 补发

	select {
	case msg := <-c.send:
		if string(msg) != "second" {
			t.Fatalf("expected retried broadcast, got %q", msg)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("deliverSlow did not retry delivery after the buffer drained")
	}

	h.mu.RLock()
	_, stillThere = h.clients[c]
	h.mu.RUnlock()
	if !stillThere {
		t.Fatal("client should still be registered after a successful retry")
	}
}

// 如果客户端在整个补发窗口内始终无法消费（真正掉线/卡死），仍然要断开，
// 否则慢客户端会无限占用服务端资源。
func TestDeliverSlowClientDisconnectsAfterRetryWindow(t *testing.T) {
	defer withRetryTiming(200*time.Millisecond, 10*time.Millisecond)()

	h := New(time.Minute)
	c := newTestClient(t, h, 1)
	c.send <- []byte("first") // 占满缓冲区且全程不消费

	h.broadcast([]byte("second"))

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		h.mu.RLock()
		_, stillThere := h.clients[c]
		h.mu.RUnlock()
		if !stillThere {
			return // 通过：补发窗口耗尽后客户端被断开
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("client stuck with a permanently full send buffer should eventually be disconnected")
}

func TestDeviceIDFromKey(t *testing.T) {
	key := strings.Repeat("ab", 32)
	id := DeviceID(key)
	if _, err := uuid.Parse(id); err != nil {
		t.Fatalf("not a uuid: %q", id)
	}
	if DeviceID(key) != id || DeviceID(strings.ToUpper(key)) != id {
		t.Fatal("same key must map to the same device ID")
	}
	// 用公开的设备 ID 当密钥得不到同一个 ID
	if DeviceID(strings.ReplaceAll(id, "-", "")) == id {
		t.Fatal("device ID must not be usable as its own key")
	}
	for _, bad := range []string{"", "xyz", id, strings.Repeat("a", 63)} {
		if a, b := DeviceID(bad), DeviceID(bad); a == b {
			t.Fatalf("invalid key %q should get a fresh random ID", bad)
		}
	}
}
