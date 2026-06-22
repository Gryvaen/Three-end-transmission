package hub

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
