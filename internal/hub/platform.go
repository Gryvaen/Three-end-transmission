package hub

import "strings"

// ParsePlatform 从显式参数或 User-Agent 推断客户端平台。
func ParsePlatform(explicit, userAgent string) Platform {
	if explicit != "" {
		return Platform(strings.ToLower(explicit))
	}
	ua := strings.ToLower(userAgent)
	switch {
	case strings.Contains(ua, "android"):
		return PlatformAndroid
	case strings.Contains(ua, "iphone"), strings.Contains(ua, "ipad"), strings.Contains(ua, "ipod"):
		return PlatformIOS
	case strings.Contains(ua, "windows"):
		return PlatformWindows
	case strings.Contains(ua, "mac os"), strings.Contains(ua, "macintosh"):
		return PlatformMacOS
	case strings.Contains(ua, "linux"):
		return PlatformLinux
	default:
		return PlatformUnknown
	}
}
