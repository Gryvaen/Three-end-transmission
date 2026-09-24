package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"three-end-transmission/internal/config"
	"three-end-transmission/internal/server"
)

//go:embed web/*
var webFS embed.FS

func main() {
	port := flag.Int("port", config.DefaultPort, "HTTP listen port")
	flag.Parse()

	static, err := fs.Sub(webFS, "web")
	if err != nil {
		slog.Error("load static files failed", "err", err)
		os.Exit(1)
	}

	uploadDir := os.Getenv("LANROOM_UPLOAD_DIR")

	srv := server.New(server.Config{
		Port:      *port,
		StaticFS:  http.FS(static),
		UploadDir: uploadDir,
		PIN:       config.PIN(),
	})
	srv.StartFileCleanup()

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", *port),
		Handler: srv.Handler(),
		// 只限制请求头；大文件上传时长不设上限
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("hub started", "addr", httpServer.Addr, "maxUploadMiB", config.MaxUploadMB(),
			"retention", config.Retention().String(), "pin", config.PIN() != "")
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
