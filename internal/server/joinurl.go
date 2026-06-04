package server

import (
	"net/url"
	"strings"
)

func isMdnsURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return strings.Contains(strings.ToLower(raw), ".local")
	}
	return strings.HasSuffix(strings.ToLower(u.Hostname()), ".local")
}

func firstIPv4JoinURL(urls []string, preferred string) string {
	if preferred != "" && !isMdnsURL(preferred) {
		return preferred
	}
	for _, u := range urls {
		if !isMdnsURL(u) {
			return u
		}
	}
	return ""
}

func firstMdnsJoinURL(urls []string, mdnsURL, preferred string) string {
	if mdnsURL != "" {
		return mdnsURL
	}
	if preferred != "" && isMdnsURL(preferred) {
		return preferred
	}
	for _, u := range urls {
		if isMdnsURL(u) {
			return u
		}
	}
	return ""
}
