package main

import (
	"embed"
	"flag"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"three-end-transmission/internal/config"
	"three-end-transmission/internal/server"
)

//go:embed web/*
var webFS embed.FS

func main() {
	port := flag.Int("port", config.DefaultPort, "HTTP listen port")
	addr := flag.String("addr", "", "HTTP listen address, e.g. 127.0.0.1:8787 (overrides -port and LANROOM_LISTEN)")
	flag.Parse()

	listen := config.ListenAddr(*port)
	if *addr != "" {
		listen = *addr
	}
	if _, p, err := net.SplitHostPort(listen); err == nil {
		// 局域网模式展示的加入地址需要真实端口
		if n, err := strconv.Atoi(p); err == nil {
			*port = n
		}
	}

	publicURL, err := config.PublicURL()
	if err != nil {
		slog.Error("invalid config", "err", err)
		os.Exit(1)
	}
	trusted, err := config.TrustedProxies()
	if err != nil {
		slog.Error("invalid config", "err", err)
		os.Exit(1)
	}
	pin := config.PIN()
	if publicURL != "" {
		// 公网上没有口令 = 任何人都能进房间、上传文件占满磁盘
		if pin == "" {
			slog.Error("LANROOM_PIN is required when LANROOM_PUBLIC_URL is set")
			os.Exit(1)
		}
		if len([]rune(pin)) < 8 {
			slog.Warn("LANROOM_PIN is short; use at least 8 characters on a public server")
		}
		if strings.HasPrefix(publicURL, "http://") {
			slog.Warn("LANROOM_PUBLIC_URL is plain http; PIN and files travel unencrypted")
		}
	}

	static, err := fs.Sub(webFS, "web")
	if err != nil {
		slog.Error("load static files failed", "err", err)
		os.Exit(1)
	}

	uploadDir := os.Getenv("LANROOM_UPLOAD_DIR")

	srv := server.New(server.Config{
		Port:           *port,
		StaticFS:       http.FS(static),
		UploadDir:      uploadDir,
		PIN:            pin,
		PublicURL:      publicURL,
		TrustedProxies: trusted,
	})
	srv.StartFileCleanup()

	httpServer := &http.Server{
		Addr:    listen,
		Handler: srv.Handler(),
		// 只限制请求头；大文件上传时长不设上限
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("hub started", "addr", httpServer.Addr, "maxUploadMiB", config.MaxUploadMB(),
			"retention", config.Retention().String(), "pin", pin != "", "publicUrl", publicURL)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server stopped", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	_ = httpServer.Close()
}
