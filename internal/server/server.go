package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"three-end-transmission/internal/config"
	"three-end-transmission/internal/hub"

	"github.com/gorilla/websocket"
	"github.com/skip2/go-qrcode"
)

type Config struct {
	Port           int
	StaticFS       http.FileSystem
	UploadDir      string
	MaxUploadBytes int64
	// Retention 消息历史与上传文件的保留时长，0 表示用 config.Retention()。
	Retention time.Duration
	// PIN 房间口令，为空表示不启用。
	PIN string
	// PublicURL 公网访问地址（如 https://drop.example.com），为空表示局域网模式。
	PublicURL string
	// TrustedProxies 可信反向代理，只有来自这些地址的请求才读取 X-Forwarded-* 头。
	TrustedProxies []*net.IPNet
}

type Server struct {
	cfg      Config
	hub      *hub.Hub
	auth     *authState
	proxies  proxyTrust
	upgrader websocket.Upgrader
	files    sync.Map // fileID -> fileRecord
	uploads  sync.Map // uploadID -> *uploadSession
}

type fileRecord struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	Mime      string    `json:"mime"`
	Path      string    `json:"-"`
	CreatedAt time.Time `json:"createdAt"`
}

type infoResponse struct {
	JoinURL      string   `json:"joinUrl"`
	PublicURL    string   `json:"publicUrl,omitempty"`
	PINRequired  bool     `json:"pinRequired"`
	Authorized   bool     `json:"authorized"`
	RetentionSec int64    `json:"retentionSec"`
	Port         int      `json:"port"`
	LocalIPs     []string `json:"localIps"`
	URLs         []string `json:"urls"`
	ClientCount  int      `json:"clientCount"`
	MaxUploadMB  int      `json:"maxUploadMb"`
}

func New(cfg Config) *Server {
	if cfg.UploadDir == "" {
		cfg.UploadDir = filepath.Join(os.TempDir(), "three-end-transmission-uploads")
	}
	_ = os.MkdirAll(cfg.UploadDir, 0o755)
	removeOrphanUploads(cfg.UploadDir)

	if cfg.MaxUploadBytes <= 0 {
		cfg.MaxUploadBytes = config.MaxUploadBytes()
	}
	if cfg.Retention <= 0 {
		cfg.Retention = config.Retention()
	}

	proxies := proxyTrust(cfg.TrustedProxies)
	s := &Server{
		cfg:     cfg,
		hub:     hub.New(cfg.Retention),
		auth:    newAuthState(cfg.PIN, proxies),
		proxies: proxies,
	}
	s.upgrader = websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		CheckOrigin:     s.sameOrigin,
	}
	return s
}

// sameOrigin 拒绝其他网站发起的 WebSocket（防止借用口令 cookie）；无 Origin 的非浏览器客户端放行。
// 公网模式下也接受 PublicURL 的 host，反代没有透传 Host 头时仍能连接。
func (s *Server) sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if strings.EqualFold(u.Host, r.Host) {
		return true
	}
	if s.cfg.PublicURL != "" {
		pub, err := url.Parse(s.cfg.PublicURL)
		return err == nil && strings.EqualFold(u.Host, pub.Host)
	}
	return false
}

func (s *Server) StartFileCleanup() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			s.purgeExpiredFiles(s.cfg.Retention)
			s.auth.prune()
		}
	}()
}

func (s *Server) purgeExpiredFiles(ttl time.Duration) {
	cutoff := time.Now().Add(-ttl)
	s.files.Range(func(key, value any) bool {
		record := value.(fileRecord)
		if record.CreatedAt.Before(cutoff) {
			s.files.Delete(key)
			if err := os.Remove(record.Path); err != nil && !os.IsNotExist(err) {
				slog.Warn("remove expired upload failed", "id", key, "err", err)
			}
		}
		return true
	})
	s.purgeStaleUploads(cutoff)
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	protect := s.auth.requireAuth

	mux.HandleFunc("/api/info", s.handleInfo)
	mux.HandleFunc("/api/qrcode", s.handleQRCode)
	mux.HandleFunc("/api/auth", s.auth.handleAuth)
	mux.HandleFunc("/api/upload", protect(s.handleUpload))
	mux.HandleFunc("/api/uploads", protect(s.handleUploadCreate))
	mux.HandleFunc("/api/uploads/", protect(s.handleUploadSession))
	mux.HandleFunc("/api/files/", protect(s.handleDownload))
	mux.HandleFunc("/ws", protect(s.handleWebSocket))

	if s.cfg.StaticFS != nil {
		fileServer := http.FileServer(s.cfg.StaticFS)
		mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 避免手机浏览器长期缓存旧的 app.js/index.html：embed.FS 没有真实 mtime，
			// 默认没有任何缓存校验头，部署新版本后手机可能一直跑旧脚本。
			w.Header().Set("Cache-Control", "no-cache")
			fileServer.ServeHTTP(w, r)
		}))
	}

	return mux
}

func (s *Server) joinURLs(lanIPs []string) []string {
	urls := make([]string, 0, len(lanIPs))
	for _, ip := range lanIPs {
		urls = append(urls, fmt.Sprintf("http://%s:%d", ip, s.cfg.Port))
	}
	return urls
}

func (s *Server) preferredJoinURL(lanIPs []string) string {
	if len(lanIPs) > 0 {
		return fmt.Sprintf("http://%s:%d", lanIPs[0], s.cfg.Port)
	}
	return ""
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	var ips, urls []string
	var join string
	if s.cfg.PublicURL != "" {
		// 公网模式：只展示公网地址，不暴露服务器内网 IP
		urls = []string{s.cfg.PublicURL}
		join = s.cfg.PublicURL
	} else {
		ips = AdvertiseIPv4Addresses(r)
		urls = s.joinURLs(ips)
		join = s.preferredJoinURL(ips)
	}
	authorized := s.auth.authorized(r)

	// 已认证设备展示的二维码带上口令（URL fragment 不会发给服务器），扫码即可免输入
	if join != "" && s.auth.enabled() && authorized {
		join += "/#pin=" + url.QueryEscape(s.auth.pin)
	}

	resp := infoResponse{
		Port:         s.cfg.Port,
		LocalIPs:     ips,
		URLs:         urls,
		JoinURL:      join,
		PublicURL:    s.cfg.PublicURL,
		PINRequired:  s.auth.enabled(),
		Authorized:   authorized,
		RetentionSec: int64(s.cfg.Retention.Seconds()),
		ClientCount:  s.hub.ClientCount(),
		MaxUploadMB:  int(s.cfg.MaxUploadBytes >> 20),
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleQRCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	target := strings.TrimSpace(r.URL.Query().Get("url"))
	if target == "" {
		target = s.preferredJoinURL(AdvertiseIPv4Addresses(r))
	}
	if target == "" {
		http.Error(w, "no join url available", http.StatusBadRequest)
		return
	}

	png, err := qrcode.Encode(target, qrcode.Medium, 256)
	if err != nil {
		slog.Error("qrcode encode failed", "err", err)
		http.Error(w, "qrcode failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(png)
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("websocket upgrade failed", "err", err)
		return
	}

	q := r.URL.Query()
	name := strings.TrimSpace(q.Get("name"))
	platform := hub.ParsePlatform(q.Get("platform"), r.UserAgent())

	client := hub.NewClient(s.hub, conn, q.Get("key"), name, platform, s.proxies.ClientIP(r))
	s.hub.Register(client)

	go client.WritePump()
	go client.ReadPump()
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/files/")
	if id == "" || strings.Contains(id, "/") || !isHexFileID(id) {
		http.NotFound(w, r)
		return
	}

	raw, ok := s.files.Load(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	record := raw.(fileRecord)

	w.Header().Set("Content-Type", record.Mime)
	// Content-Type 来自上传方，禁止浏览器嗅探成 HTML 执行
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// filename* 支持中文等非 ASCII 文件名
	asciiName := strings.Map(func(r rune) rune {
		if r >= 0x20 && r <= 0x7e && r != '"' && r != '\\' {
			return r
		}
		return '_'
	}, record.Name)
	if asciiName == "" {
		asciiName = "download"
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(
		`attachment; filename="%s"; filename*=UTF-8''%s`,
		asciiName,
		url.PathEscape(record.Name),
	))
	http.ServeFile(w, r, record.Path)
}

func randomID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func isHexFileID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, c := range id {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
