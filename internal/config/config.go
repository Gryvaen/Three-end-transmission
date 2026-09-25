package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultPort = 8787

	// DefaultMaxUploadMB 单文件上传默认上限（局域网互传，适当放宽）。
	DefaultMaxUploadMB = 500
	// MaxUploadCapMB 环境变量可设置的上限封顶，防止误配占满磁盘。
	MaxUploadCapMB = 4096
)

// MaxUploadBytes 返回允许的单文件上传字节数。
// 环境变量 LANROOM_MAX_UPLOAD_MB，例如 1024 表示 1 GiB。
func MaxUploadBytes() int64 {
	mb := DefaultMaxUploadMB
	if raw := strings.TrimSpace(os.Getenv("LANROOM_MAX_UPLOAD_MB")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			mb = n
		}
	}
	if mb > MaxUploadCapMB {
		mb = MaxUploadCapMB
	}
	return int64(mb) << 20
}

// MaxUploadMB 与 MaxUploadBytes 对应的 MiB 数（用于 API / 日志展示）。
func MaxUploadMB() int {
	return int(MaxUploadBytes() >> 20)
}

const (
	// DefaultRetention 消息历史与上传文件的默认保留时长。
	DefaultRetention = time.Hour
	minRetention     = time.Minute
)

// Retention 返回消息与文件的保留时长。
// 环境变量 LANROOM_RETENTION，Go duration 格式，例如 30m、24h。
func Retention() time.Duration {
	raw := strings.TrimSpace(os.Getenv("LANROOM_RETENTION"))
	if raw == "" {
		return DefaultRetention
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return DefaultRetention
	}
	if d < minRetention {
		return minRetention
	}
	return d
}

// PIN 返回房间口令（LANROOM_PIN），为空表示不启用口令。
func PIN() string {
	return strings.TrimSpace(os.Getenv("LANROOM_PIN"))
}

// PublicURL 返回公网访问地址（LANROOM_PUBLIC_URL），例如 https://drop.example.com。
// 为空表示局域网模式；设置后加入地址与二维码都使用它。
func PublicURL() (string, error) {
	return ParsePublicURL(os.Getenv("LANROOM_PUBLIC_URL"))
}

// ParsePublicURL 校验公网地址：必须是带 host 的 http/https URL，不能带路径（不支持子路径部署）。
func ParsePublicURL(raw string) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("LANROOM_PUBLIC_URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("LANROOM_PUBLIC_URL must start with http:// or https://, got %q", raw)
	}
	if u.Host == "" {
		return "", fmt.Errorf("LANROOM_PUBLIC_URL has no host: %q", raw)
	}
	if u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("LANROOM_PUBLIC_URL must not contain a path, query or fragment: %q", raw)
	}
	return u.Scheme + "://" + u.Host, nil
}

// ListenAddr 返回监听地址（LANROOM_LISTEN），为空时监听所有网卡的 port。
// 放在 Nginx 后面时设为 127.0.0.1:8787，只让反代访问。
func ListenAddr(port int) string {
	if addr := strings.TrimSpace(os.Getenv("LANROOM_LISTEN")); addr != "" {
		return addr
	}
	return fmt.Sprintf(":%d", port)
}

// TrustedProxies 返回可信反向代理（LANROOM_TRUSTED_PROXIES，逗号/空格分隔的 IP 或 CIDR）。
// 只有来自这些地址的请求才会读取 X-Forwarded-For / X-Real-IP / X-Forwarded-Proto。
func TrustedProxies() ([]*net.IPNet, error) {
	return ParseTrustedProxies(os.Getenv("LANROOM_TRUSTED_PROXIES"))
}

func ParseTrustedProxies(raw string) ([]*net.IPNet, error) {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == ';'
	})
	nets := make([]*net.IPNet, 0, len(fields))
	for _, f := range fields {
		if !strings.Contains(f, "/") {
			ip := net.ParseIP(f)
			if ip == nil {
				return nil, fmt.Errorf("LANROOM_TRUSTED_PROXIES: invalid IP %q", f)
			}
			bits := 128
			if ip.To4() != nil {
				ip, bits = ip.To4(), 32
			}
			nets = append(nets, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
			continue
		}
		_, n, err := net.ParseCIDR(f)
		if err != nil {
			return nil, fmt.Errorf("LANROOM_TRUSTED_PROXIES: invalid CIDR %q", f)
		}
		nets = append(nets, n)
	}
	return nets, nil
}
