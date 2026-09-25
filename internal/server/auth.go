package server

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	sessionCookie  = "lanroom_session"
	sessionTTL     = 30 * 24 * time.Hour
	authFailLimit  = 5
	authFailWindow = time.Minute
)

// authState 房间口令（LANROOM_PIN）。pin 为空时不启用，所有请求放行。
// 会话只保存在内存里，Hub 重启后需要重新输入口令。
type authState struct {
	pin     string
	proxies proxyTrust

	mu       sync.Mutex
	sessions map[string]time.Time // token -> 过期时间
	fails    map[string]failWindow
}

type failWindow struct {
	count int
	start time.Time
}

func newAuthState(pin string, proxies proxyTrust) *authState {
	return &authState{
		pin:      pin,
		proxies:  proxies,
		sessions: make(map[string]time.Time),
		fails:    make(map[string]failWindow),
	}
}

func (a *authState) enabled() bool {
	return a.pin != ""
}

func (a *authState) authorized(r *http.Request) bool {
	if !a.enabled() {
		return true
	}
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	exp, ok := a.sessions[c.Value]
	return ok && time.Now().Before(exp)
}

// requireAuth 启用口令时拒绝未认证的请求。
func (a *authState) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.authorized(r) {
			http.Error(w, "pin required", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// handleAuth POST /api/auth {pin}：口令正确则下发会话 cookie。
func (a *authState) handleAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !a.enabled() {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}

	// 限速按真实客户端地址：反代后面所有请求的 TCP 对端都是代理，
	// 只信任可信代理给出的转发头，客户端自己伪造的无效
	peer := a.proxies.rawClientIP(r)
	if a.tooManyFails(peer) {
		http.Error(w, "too many attempts, retry later", http.StatusTooManyRequests)
		return
	}

	var req struct {
		PIN string `json:"pin"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if subtle.ConstantTimeCompare([]byte(req.PIN), []byte(a.pin)) != 1 {
		a.recordFail(peer)
		http.Error(w, "wrong pin", http.StatusUnauthorized)
		return
	}

	token, err := randomID()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	a.mu.Lock()
	a.sessions[token] = time.Now().Add(sessionTTL)
	delete(a.fails, peer)
	a.mu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   a.proxies.isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *authState) tooManyFails(peer string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	f, ok := a.fails[peer]
	if !ok {
		return false
	}
	if time.Since(f.start) > authFailWindow {
		delete(a.fails, peer)
		return false
	}
	return f.count >= authFailLimit
}

func (a *authState) recordFail(peer string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	f := a.fails[peer]
	if f.count == 0 || time.Since(f.start) > authFailWindow {
		f = failWindow{start: time.Now()}
	}
	f.count++
	a.fails[peer] = f
}

// prune 清理过期会话与失败记录，由定时清理任务调用。
func (a *authState) prune() {
	now := time.Now()
	a.mu.Lock()
	defer a.mu.Unlock()
	for token, exp := range a.sessions {
		if now.After(exp) {
			delete(a.sessions, token)
		}
	}
	for peer, f := range a.fails {
		if now.Sub(f.start) > authFailWindow {
			delete(a.fails, peer)
		}
	}
}
