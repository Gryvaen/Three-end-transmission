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

	"three-end-transmission/internal/mdns"
	"three-end-transmission/internal/server"
)

//go:embed web/*
var webFS embed.FS

func main() {
	port := flag.Int("port", 8787, "HTTP listen port")
	flag.Parse()

	static, err := fs.Sub(webFS, "web")
	if err != nil {
		slog.Error("load static files failed", "err", err)
		os.Exit(1)
	}

	reg, err := mdns.Register(*port)
	if err != nil {
		slog.Warn("mDNS registration failed, QR/IP fallback still works", "err", err)
	}

	srv := server.New(server.Config{
		Port:     *port,
		StaticFS: http.FS(static),
		Mdns:     reg,
	})

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", *port),
		Handler: srv.Handler(),
	}

	go func() {
		slog.Info("hub started", "addr", httpServer.Addr)
		if reg != nil {
			slog.Info("open in browser", "mdns", reg.URL())
		}
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server stopped", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	if reg != nil {
		reg.Shutdown()
	}
	_ = httpServer.Close()
}
