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

	"three-end-transmission/internal/config"
	"three-end-transmission/internal/mdns"
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
		Mdns:      nil,
		UploadDir: uploadDir,
	})
	srv.StartFileCleanup()

	keeper := mdns.NewKeeper(*port, func() []string {
		return server.AdvertiseIPv4Addresses(nil)
	})
	go keeper.Run(srv.SetMdns)

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", *port),
		Handler: srv.Handler(),
	}

	go func() {
		slog.Info("hub started", "addr", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server stopped", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	keeper.Shutdown()
	_ = httpServer.Close()
}
