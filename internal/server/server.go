package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"three-end-transmission/internal/hub"
	"three-end-transmission/internal/mdns"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/skip2/go-qrcode"
)

type Config struct {
	Port       int
	StaticFS   http.FileSystem
	Mdns       *mdns.Registration
	UploadDir  string
}

type Server struct {
	cfg      Config
	hub      *hub.Hub
	upgrader websocket.Upgrader
	files    sync.Map
}

type fileRecord struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	Mime      string    `json:"mime"`
	Path      string    `json:"-"`
	CreatedAt time.Time `json:"createdAt"`
}

type infoResponse struct {
	Hostname    string   `json:"hostname"`
	MdnsURL     string   `json:"mdnsUrl"`
	JoinURL     string   `json:"joinUrl"`
	Port        int      `json:"port"`
	LocalIPs    []string `json:"localIps"`
	URLs        []string `json:"urls"`
	ClientCount int      `json:"clientCount"`
}

func New(cfg Config) *Server {
	if cfg.UploadDir == "" {
		cfg.UploadDir = filepath.Join(os.TempDir(), "three-end-transmission-uploads")
	}
	_ = os.MkdirAll(cfg.UploadDir, 0o755)

	return &Server{
		cfg: cfg,
		hub: hub.New(),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/info", s.handleInfo)
	mux.HandleFunc("/api/qrcode", s.handleQRCode)
	mux.HandleFunc("/api/send", s.handleSend)
	mux.HandleFunc("/api/upload", s.handleUpload)
	mux.HandleFunc("/api/files/", s.handleDownload)
	mux.HandleFunc("/ws", s.handleWebSocket)

	if s.cfg.StaticFS != nil {
		mux.Handle("/", http.FileServer(s.cfg.StaticFS))
	}

	return mux
}

func (s *Server) joinURLs() []string {
	urls := make([]string, 0)

	for _, ip := range LANIPv4Addresses() {
		urls = append(urls, fmt.Sprintf("http://%s:%d", ip, s.cfg.Port))
	}
	if s.cfg.Mdns != nil {
		urls = append(urls, s.cfg.Mdns.URL())
	}
	return urls
}

func (s *Server) preferredJoinURL() string {
	lanIPs := LANIPv4Addresses()
	if len(lanIPs) > 0 {
		return fmt.Sprintf("http://%s:%d", lanIPs[0], s.cfg.Port)
	}
	if s.cfg.Mdns != nil {
		return s.cfg.Mdns.URL()
	}
	return ""
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	ips := LANIPv4Addresses()
	urls := s.joinURLs()

	resp := infoResponse{
		Port:        s.cfg.Port,
		LocalIPs:    ips,
		URLs:        urls,
		JoinURL:     s.preferredJoinURL(),
		ClientCount: s.hub.ClientCount(),
	}
	if s.cfg.Mdns != nil {
		resp.Hostname = s.cfg.Mdns.Hostname()
		resp.MdnsURL = s.cfg.Mdns.URL()
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleQRCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	target := strings.TrimSpace(r.URL.Query().Get("url"))
	if target == "" {
		target = s.preferredJoinURL()
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

type sendRequest struct {
	Name     string             `json:"name"`
	Platform string             `json:"platform"`
	Payload  hub.MessagePayload `json:"payload"`
}

func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req sendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "CLI"
	}
	platform := hub.Platform(parsePlatform(req.Platform, ""))
	if req.Payload.Kind == "" {
		http.Error(w, "missing payload.kind", http.StatusBadRequest)
		return
	}

	device := hub.Device{
		ID:       uuid.New().String(),
		Name:     name,
		Platform: platform,
		IP:       ClientIP(r),
	}
	s.hub.BroadcastMessage(device, req.Payload)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("websocket upgrade failed", "err", err)
		return
	}

	name := strings.TrimSpace(r.URL.Query().Get("name"))
	platform := hub.Platform(parsePlatform(r.URL.Query().Get("platform"), r.UserAgent()))

	client := hub.NewClient(s.hub, conn, name, platform, ClientIP(r))
	s.hub.Register(client)

	go client.WritePump()
	go client.ReadPump()
}

func parsePlatform(explicit, userAgent string) string {
	if explicit != "" {
		return strings.ToLower(explicit)
	}
	ua := strings.ToLower(userAgent)
	switch {
	case strings.Contains(ua, "android"):
		return string(hub.PlatformAndroid)
	case strings.Contains(ua, "iphone") || strings.Contains(ua, "ipad"):
		return string(hub.PlatformIOS)
	case strings.Contains(ua, "windows"):
		return string(hub.PlatformWindows)
	case strings.Contains(ua, "mac os") || strings.Contains(ua, "macintosh"):
		return string(hub.PlatformMacOS)
	case strings.Contains(ua, "linux"):
		return string(hub.PlatformLinux)
	default:
		return string(hub.PlatformUnknown)
	}
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	const maxUpload = 100 << 20 // 100 MiB
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)

	if err := r.ParseMultipartForm(maxUpload); err != nil {
		http.Error(w, "file too large or invalid form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file field", http.StatusBadRequest)
		return
	}
	defer file.Close()

	id, err := randomID()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	destPath := filepath.Join(s.cfg.UploadDir, id)
	dest, err := os.Create(destPath)
	if err != nil {
		http.Error(w, "cannot save file", http.StatusInternalServerError)
		return
	}

	written, err := io.Copy(dest, file)
	_ = dest.Close()
	if err != nil {
		_ = os.Remove(destPath)
		http.Error(w, "upload failed", http.StatusInternalServerError)
		return
	}

	mime := header.Header.Get("Content-Type")
	if mime == "" {
		mime = "application/octet-stream"
	}

	record := fileRecord{
		Name:      header.Filename,
		Size:      written,
		Mime:      mime,
		Path:      destPath,
		CreatedAt: time.Now(),
	}
	s.files.Store(id, record)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"fileId": id,
		"name":   record.Name,
		"size":   record.Size,
		"mime":   record.Mime,
	})
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/files/")
	if id == "" || strings.Contains(id, "/") {
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
