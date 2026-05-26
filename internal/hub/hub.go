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
	maxMessageSize = 512 * 1024
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
}

type MessagePayload struct {
	Kind    string `json:"kind"`
	Content string `json:"content,omitempty"`
	FileID  string `json:"fileId,omitempty"`
	Meta    *FileMeta `json:"meta,omitempty"`
}

type FileMeta struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	Mime string `json:"mime"`
}

type ChatMessage struct {
	Type      string         `json:"type"`
	From      *Device        `json:"from,omitempty"`
	Payload   MessagePayload `json:"payload,omitempty"`
	Timestamp int64          `json:"timestamp,omitempty"`
}

type Client struct {
	hub      *Hub
	conn     *websocket.Conn
	send     chan []byte
	device   Device
}

type Hub struct {
	mu      sync.RWMutex
	clients map[*Client]bool
}

func New() *Hub {
	return &Hub{
		clients: make(map[*Client]bool),
	}
}

func (h *Hub) Run() {
	for {
		select {}
	}
}

func (h *Hub) Register(client *Client) {
	h.mu.Lock()
	h.clients[client] = true
	h.mu.Unlock()

	h.broadcastPresence()
	slog.Info("client joined", "name", client.device.Name, "id", client.device.ID)
}

func (h *Hub) Unregister(client *Client) {
	h.mu.Lock()
	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)
		close(client.send)
	}
	h.mu.Unlock()

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
	msg := ChatMessage{
		Type: "presence",
	}
	body, err := json.Marshal(map[string]any{
		"type":  msg.Type,
		"users": h.devices(),
	})
	if err != nil {
		return
	}
	h.broadcast(body)
}

func (h *Hub) BroadcastMessage(from Device, payload MessagePayload) {
	msg := ChatMessage{
		Type:      "message",
		From:      &from,
		Payload:   payload,
		Timestamp: time.Now().Unix(),
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return
	}
	h.broadcast(body)
}

func (h *Hub) broadcast(message []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
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

func NewClient(h *Hub, conn *websocket.Conn, name string, platform Platform) *Client {
	if name == "" {
		name = "匿名设备"
	}
	return &Client{
		hub:  h,
		conn: conn,
		send: make(chan []byte, 64),
		device: Device{
			ID:       uuid.New().String(),
			Name:     name,
			Platform: platform,
		},
	}
}

func (c *Client) ReadPump() {
	defer func() {
		c.hub.Unregister(c)
		_ = c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
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

		c.hub.BroadcastMessage(c.device, incoming.Payload)
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
