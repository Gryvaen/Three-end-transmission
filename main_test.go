package main

import (
	"io"
	"os"
	"testing"
	"time"
)

func TestWatchStdinStopsOnEOF(t *testing.T) {
	r, w := io.Pipe()
	stop := make(chan os.Signal, 1)
	go watchStdin(r, stop)

	select {
	case <-stop:
		t.Fatal("stopped before stdin closed")
	case <-time.After(50 * time.Millisecond):
	}

	_ = w.Close()
	select {
	case <-stop:
	case <-time.After(time.Second):
		t.Fatal("did not stop after stdin closed")
	}
}
