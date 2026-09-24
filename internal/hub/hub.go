package hub

import (
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	MaxMessageSize = 512 * 1024
	// MaxRecipients 单条定向消息最多的接收设备数。
	MaxRecipients = 16
)

type Platform string

const (
	PlatformAndroid Platform = "android"
	PlatformWindows Platform = "windows"
	PlatformLinux   Platform = "linux"
	PlatformIOS     Platform = "ios"
	PlatformMacOS   Platform = "macos"
	PlatformUnknown Platform = "unknown"
)

type Device struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Platform Platform `json:"platform"`
	IP       string   `json:"ip"`
}

type MessagePayload struct {
	Kind    string    `json:"kind"`
	Content string    `json:"content,omitempty"`
	FileID  string    `json:"fileId,omitempty"`
	Meta    *FileMeta `json:"meta,omitempty"`
}

type FileMeta struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	Mime string `json:"mime"`
}

// ChatMessage 为空 To 表示群发；非空时只投递给 To 中的设备与发送者。
// Recipients 由服务端根据 To 填充，便于前端在对方离线时仍能显示名称。
type ChatMessage struct {
	Type       string         `json:"type"`
	From       *Device        `json:"from,omitempty"`
	To         []string       `json:"to,omitempty"`
	Recipients []Device       `json:"recipients,omitempty"`
	Payload    MessagePayload `json:"payload,omitempty"`
	Timestamp  int64          `json:"timestamp,omitempty"`
}

// visibleTo 该消息是否应出现在指定设备的聊天记录里。
func (m ChatMessage) visibleTo(deviceID string) bool {
	if len(m.To) == 0 || (m.From != nil && m.From.ID == deviceID) {
		return true
	}
	for _, id := range m.To {
		if id == deviceID {
			return true
		}
	}
	return false
}

type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	send   chan []byte
	device Device
}

type Hub struct {
	mu         sync.RWMutex
	clients    map[*Client]bool
	history    []ChatMessage
	historyTTL time.Duration
	// known 记录出现过的设备（按 ID），用于给定向消息填充接收者名称。
	// 局域网设备数量很少，不做淘汰。
	known map[string]Device
}

func New(historyTTL time.Duration) *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		history:    make([]ChatMessage, 0, 64),
		historyTTL: historyTTL,
		known:      make(map[string]Device),
	}
}

// Register 加入新连接；同一设备 ID 的旧连接（刷新、重连残留）会被替换掉。
func (h *Hub) Register(client *Client) {
	var stale []*Client
	h.mu.Lock()
	for c := range h.clients {
		if c.device.ID == client.device.ID {
			stale = append(stale, c)
			delete(h.clients, c)
			close(c.send)
		}
	}
	h.clients[client] = true
	h.known[client.device.ID] = client.device
	h.mu.Unlock()

	for _, c := range stale {
		_ = c.conn.Close()
	}

	h.sendWelcome(client)
	h.sendHistory(client)
	h.broadcastPresence()
	slog.Info("client joined", "name", client.device.Name, "ip", client.device.IP, "id", client.device.ID)
}

func (h *Hub) sendWelcome(client *Client) {
	client.enqueueJSON(welcomeMessage{
		Type:   "welcome",
		Device: client.device,
	})
}

func (h *Hub) sendHistory(client *Client) {
	cutoff := time.Now().Add(-h.historyTTL).Unix()
	var msgs []ChatMessage
	h.mu.RLock()
	for _, m := range h.history {
		if m.Timestamp >= cutoff && m.visibleTo(client.device.ID) {
			msgs = append(msgs, m)
		}
	}
	h.mu.RUnlock()

	if len(msgs) == 0 {
		return
	}

	client.enqueueJSON(historyMessage{
		Type:     "history",
		Messages: msgs,
	})
}

func (h *Hub) addToHistory(msg ChatMessage) {
	h.mu.Lock()
	defer h.mu.Unlock()

	cutoff := time.Now().Add(-h.historyTTL).Unix()
	kept := h.history[:0]
	for _, m := range h.history {
		if m.Timestamp >= cutoff {
			kept = append(kept, m)
		}
	}
	h.history = append(kept, msg)
}

func (h *Hub) Unregister(client *Client) {
	h.mu.Lock()
	_, ok := h.clients[client]
	if ok {
		delete(h.clients, client)
		close(client.send)
	}
	h.mu.Unlock()
	if !ok {
		return // 已被 Register 替换或已注销
	}

	h.broadcastPresence()
	slog.Info("client left", "name", client.device.Name, "id", client.device.ID)
}

func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

func (h *Hub) devices() []Device {
	h.mu.RLock()
	defer h.mu.RUnlock()

	list := make([]Device, 0, len(h.clients))
	for c := range h.clients {
		list = append(list, c.device)
	}
	return list
}

func (h *Hub) broadcastPresence() {
	body, err := json.Marshal(presenceMessage{
		Type:  "presence",
		Users: h.devices(),
	})
	if err != nil {
		return
	}
	h.broadcast(body)
}

// SendMessage 群发（to 为空）或定向投递给 to 中的设备与发送者本人。
func (h *Hub) SendMessage(from Device, to []string, payload MessagePayload) {
	msg := ChatMessage{
		Type:      "message",
		From:      &from,
		To:        to,
		Payload:   payload,
		Timestamp: time.Now().Unix(),
	}
	if len(to) > 0 {
		h.mu.RLock()
		for _, id := range to {
			if d, ok := h.known[id]; ok {
				msg.Recipients = append(msg.Recipients, d)
			} else {
				msg.Recipients = append(msg.Recipients, Device{ID: id})
			}
		}
		h.mu.RUnlock()
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return
	}
	h.addToHistory(msg)
	h.deliver(body, func(c *Client) bool { return msg.visibleTo(c.device.ID) })
}

func (h *Hub) broadcast(message []byte) {
	h.deliver(message, nil)
}

// deliver 发给 filter 返回 true 的客户端（filter 为 nil 表示全部）；发送队列满的客户端会被断开。
func (h *Hub) deliver(message []byte, filter func(*Client) bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		if filter != nil && !filter(client) {
			continue
		}
		select {
		case client.send <- message:
		default:
			go func(c *Client) {
				h.Unregister(c)
				_ = c.conn.Close()
			}(client)
		}
	}
}

// NewClient 创建客户端；id 为浏览器持久化的 UUID，不合法时由服务端生成。
func NewClient(h *Hub, conn *websocket.Conn, id, name string, platform Platform, ip string) *Client {
	if parsed, err := uuid.Parse(id); err == nil {
		id = parsed.String()
	} else {
		id = uuid.New().String()
	}
	if name == "" {
		name = "匿名设备"
	}
	if ip == "" {
		ip = "未知"
	}
	return &Client{
		hub:  h,
		conn: conn,
		send: make(chan []byte, 64),
		device: Device{
			ID:       id,
			Name:     name,
			Platform: platform,
			IP:       ip,
		},
	}
}

// enqueueJSON 投递给单个客户端；持读锁确认仍在线，避免向已关闭的 send 通道写入。
func (c *Client) enqueueJSON(v any) {
	body, err := json.Marshal(v)
	if err != nil {
		return
	}
	c.hub.mu.RLock()
	defer c.hub.mu.RUnlock()
	if !c.hub.clients[c] {
		return
	}
	select {
	case c.send <- body:
	default:
	}
}

func (c *Client) ReadPump() {
	defer func() {
		c.hub.Unregister(c)
		_ = c.conn.Close()
	}()

	c.conn.SetReadLimit(MaxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			break
		}

		var incoming ChatMessage
		if err := json.Unmarshal(data, &incoming); err != nil {
			continue
		}
		if incoming.Type != "message" {
			continue
		}
		if incoming.Payload.Kind == "" {
			continue
		}

		to, ok := sanitizeRecipients(incoming.To, c.device.ID)
		if !ok {
			continue // 指定了接收者但全部无效：丢弃，避免误变成群发
		}
		c.hub.SendMessage(c.device, to, incoming.Payload)
	}
}

func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			if _, err := w.Write(message); err != nil {
				return
			}
			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// sanitizeRecipients 规范化接收者 ID（去重、去掉自己、校验 UUID、限制数量）。
// 第二个返回值为 false 表示请求了定向发送但没有有效接收者。
func sanitizeRecipients(raw []string, self string) ([]string, bool) {
	if len(raw) == 0 {
		return nil, true
	}
	seen := make(map[string]struct{}, len(raw))
	var out []string
	for _, id := range raw {
		parsed, err := uuid.Parse(id)
		if err != nil {
			continue
		}
		id = parsed.String()
		if id == self {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
		if len(out) == MaxRecipients {
			break
		}
	}
	return out, len(out) > 0
}
