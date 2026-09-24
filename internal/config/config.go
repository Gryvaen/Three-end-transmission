package config

import (
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
