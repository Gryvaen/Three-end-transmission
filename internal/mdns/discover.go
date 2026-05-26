package mdns

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"three-end-transmission/internal/config"

	"github.com/grandcat/zeroconf"
)

// Discover 在局域网搜索 LanRoom Hub，返回全部候选 HTTP 地址（去重）。
func DiscoverAll(timeout time.Duration) ([]string, error) {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}

	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return nil, fmt.Errorf("mdns resolver: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	entries := make(chan *zeroconf.ServiceEntry, 8)
	go func() {
		_ = resolver.Browse(ctx, ServiceType, Domain, entries)
	}()

	seen := make(map[string]struct{})
	var urls []string

	for {
		select {
		case entry := <-entries:
			if entry == nil {
				continue
			}
			ip := pickIPv4(entry)
			if ip == "" {
				continue
			}
			port := entry.Port
			if port == 0 {
				port = config.DefaultPort
			}
			url := fmt.Sprintf("http://%s:%d", ip, port)
			if _, ok := seen[url]; ok {
				continue
			}
			seen[url] = struct{}{}
			urls = append(urls, url)
		case <-ctx.Done():
			if len(urls) == 0 {
				return nil, fmt.Errorf("未在局域网发现 LanRoom Hub（可设置 LANROOM_HOST 或 -host）")
			}
			sort.Slice(urls, func(i, j int) bool {
				return preferURL(urls[i], urls[j])
			})
			return urls, nil
		}
	}
}

// Discover 返回排序后的第一个候选地址（兼容旧调用）。
func Discover(timeout time.Duration) (string, error) {
	urls, err := DiscoverAll(timeout)
	if err != nil {
		return "", err
	}
	return urls[0], nil
}

func preferURL(a, b string) bool {
	pa := portFromURL(a)
	pb := portFromURL(b)
	if pa == config.DefaultPort && pb != config.DefaultPort {
		return true
	}
	if pb == config.DefaultPort && pa != config.DefaultPort {
		return false
	}
	return pa < pb
}

func portFromURL(raw string) int {
	_, portStr, err := net.SplitHostPort(strings.TrimPrefix(raw, "http://"))
	if err != nil {
		return config.DefaultPort
	}
	var port int
	fmt.Sscanf(portStr, "%d", &port)
	if port == 0 {
		return config.DefaultPort
	}
	return port
}

func pickIPv4(entry *zeroconf.ServiceEntry) string {
	for _, ip := range entry.AddrIPv4 {
		if ip != nil && ip.To4() != nil {
			return ip.String()
		}
	}
	host := strings.TrimSuffix(entry.HostName, ".")
	if host == "" {
		return ""
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return ""
	}
	for _, ip := range ips {
		if v4 := ip.To4(); v4 != nil {
			return v4.String()
		}
	}
	return ""
}
