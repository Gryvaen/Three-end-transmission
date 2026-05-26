package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"three-end-transmission/internal/hub"
	"three-end-transmission/internal/mdns"
)

type Client struct {
	BaseURL  string
	Name     string
	Platform string
	HTTP     *http.Client
}

func New(baseURL, name string) *Client {
	if name == "" {
		name, _ = os.Hostname()
	}
	if name == "" {
		name = "Linux CLI"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	return &Client{
		BaseURL:  baseURL,
		Name:     name,
		Platform: string(hub.PlatformLinux),
		HTTP: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

type sendRequest struct {
	Name     string             `json:"name"`
	Platform string             `json:"platform"`
	Payload  hub.MessagePayload `json:"payload"`
}

type uploadResult struct {
	FileID string `json:"fileId"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Mime   string `json:"mime"`
}

func (c *Client) SendText(text string) error {
	return c.send(hub.MessagePayload{
		Kind:    "text",
		Content: text,
	})
}

func (c *Client) SendFilePath(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	name := filepath.Base(path)
	return c.SendStream(name, f)
}

func (c *Client) SendStream(filename string, r io.Reader) error {
	uploaded, err := c.upload(filename, r)
	if err != nil {
		return err
	}

	kind := "file"
	if strings.HasPrefix(uploaded.Mime, "image/") {
		kind = "image"
	}

	return c.send(hub.MessagePayload{
		Kind:   kind,
		FileID: uploaded.FileID,
		Meta: &hub.FileMeta{
			Name: uploaded.Name,
			Size: uploaded.Size,
			Mime: uploaded.Mime,
		},
	})
}

func (c *Client) send(payload hub.MessagePayload) error {
	body, err := json.Marshal(sendRequest{
		Name:     c.Name,
		Platform: c.Platform,
		Payload:  payload,
	})
	if err != nil {
		return err
	}

	resp, err := c.HTTP.Post(c.BaseURL+"/api/send", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("连接 Hub 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("发送失败 (%s): %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	return nil
}

func (c *Client) upload(filename string, r io.Reader) (*uploadResult, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, r); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	resp, err := c.HTTP.Post(c.BaseURL+"/api/upload", writer.FormDataContentType(), &body)
	if err != nil {
		return nil, fmt.Errorf("上传失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("上传失败 (%s): %s", resp.Status, strings.TrimSpace(string(msg)))
	}

	var result uploadResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) Info() ([]byte, error) {
	resp, err := c.HTTP.Get(c.BaseURL + "/api/info")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func DefaultHost() string {
	return os.Getenv("LANROOM_HOST")
}

func hubSupportsSend(baseURL string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Post(baseURL+"/api/send", "application/json", strings.NewReader("{}"))
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	// 404 表示旧版 Hub，没有 CLI 接口
	return resp.StatusCode != http.StatusNotFound
}

func discoverWorkingHub() (string, error) {
	urls, err := mdns.DiscoverAll(3 * time.Second)
	if err != nil {
		return "", err
	}
	for _, candidate := range urls {
		if hubSupportsSend(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("发现了 Hub 但不支持命令行发送（请重启最新版 go run . -port 8787）")
}

// ResolveHost 确定 Hub 地址：-host > LANROOM_HOST > mDNS 发现 > 127.0.0.1
func ResolveHost(explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return ParseHost(explicit)
	}

	env := strings.TrimSpace(os.Getenv("LANROOM_HOST"))
	switch {
	case env == "":
		if url, err := discoverWorkingHub(); err == nil {
			return ParseHost(url)
		}
		if hubSupportsSend("http://127.0.0.1:8787") {
			return ParseHost("http://127.0.0.1:8787")
		}
		return "", fmt.Errorf("未找到可用 Hub，请设置 LANROOM_HOST=http://127.0.0.1:8787")
	case strings.EqualFold(env, "auto"):
		url, err := discoverWorkingHub()
		if err != nil {
			return "", err
		}
		return ParseHost(url)
	default:
		return ParseHost(env)
	}
}

func ParseHost(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("无效地址: %s", raw)
	}
	return strings.TrimRight(raw, "/"), nil
}

func IsStdinPipe() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) == 0
}
