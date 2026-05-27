package hub

import "strings"

// SendRequest 为 HTTP /api/send 与 CLI 共用请求体。
type SendRequest struct {
	Name     string         `json:"name"`
	Platform string         `json:"platform"`
	Payload  MessagePayload `json:"payload"`
}

type welcomeMessage struct {
	Type   string `json:"type"`
	Device Device `json:"device"`
}

type historyMessage struct {
	Type     string        `json:"type"`
	Messages []ChatMessage `json:"messages"`
}

type presenceMessage struct {
	Type  string   `json:"type"`
	Users []Device `json:"users"`
}

// PayloadKindForMIME 根据 MIME 判断消息类型。
func PayloadKindForMIME(mime string) string {
	if strings.HasPrefix(mime, "image/") {
		return "image"
	}
	return "file"
}
