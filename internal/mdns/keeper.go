package mdns

import (
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"
)

// Keeper 持续维护 mDNS 注册：启动重试、Wi-Fi 晚就绪补注册、DHCP 换 IP 后自动更新。
type Keeper struct {
	port  int
	ipsFn func() []string

	mu     sync.Mutex
	reg    *Registration
	ipsKey string
}

func NewKeeper(port int, ipsFn func() []string) *Keeper {
	return &Keeper{port: port, ipsFn: ipsFn}
}

func ipsKey(ips []string) string {
	if len(ips) == 0 {
		return ""
	}
	cp := append([]string(nil), ips...)
	sort.Strings(cp)
	return strings.Join(cp, ",")
}

func (k *Keeper) Run(onUpdate func(*Registration)) {
	try := func() {
		key := ipsKey(k.ipsFn())
		if key == "" {
			return
		}

		k.mu.Lock()
		if k.reg != nil && key == k.ipsKey {
			k.mu.Unlock()
			return
		}
		old := k.reg
		k.mu.Unlock()

		if old != nil {
			old.Shutdown()
		}

		reg, err := Register(k.port, strings.Split(key, ","))
		if err != nil {
			slog.Warn("mDNS register failed", "err", err)
			return
		}

		k.mu.Lock()
		k.reg = reg
		k.ipsKey = key
		k.mu.Unlock()

		onUpdate(reg)
		slog.Info("mDNS active", "url", reg.URL(), "ips", key)
	}

	try()

	fast := time.NewTicker(3 * time.Second)
	slow := time.NewTicker(90 * time.Second)
	defer fast.Stop()
	defer slow.Stop()

	for {
		select {
		case <-fast.C:
			k.mu.Lock()
			needs := k.reg == nil
			k.mu.Unlock()
			if needs {
				try()
			}
		case <-slow.C:
			try()
		}
	}
}

func (k *Keeper) Shutdown() {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.reg != nil {
		k.reg.Shutdown()
		k.reg = nil
		k.ipsKey = ""
	}
}
