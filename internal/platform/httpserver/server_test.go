package httpserver

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/lukuku-dev/go-chi-bp/internal/platform/config"
)

func TestNewAppliesServerConfiguration(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	logger := slog.New(slog.DiscardHandler)
	cfg := config.HTTP{
		Address:           "127.0.0.1:8080",
		ReadHeaderTimeout: time.Second,
		ReadTimeout:       2 * time.Second,
		WriteTimeout:      3 * time.Second,
		IdleTimeout:       4 * time.Second,
	}

	server := New(cfg, handler, logger)

	if server.Addr != cfg.Address || server.Handler == nil {
		t.Fatalf("server address or handler does not match configuration")
	}
	if server.ReadHeaderTimeout != cfg.ReadHeaderTimeout || server.ReadTimeout != cfg.ReadTimeout ||
		server.WriteTimeout != cfg.WriteTimeout || server.IdleTimeout != cfg.IdleTimeout {
		t.Fatalf("server timeouts do not match configuration: %+v", server)
	}
}

func TestRunStopsCleanlyWhenContextIsCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	server := testServer("127.0.0.1:0")

	drained := false
	if err := Run(ctx, server, time.Second, func() { drained = true }, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if !drained {
		t.Fatal("Run() did not begin draining before shutdown")
	}
}

func TestRunReportsListenFailure(t *testing.T) {
	t.Parallel()

	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve local address: %v", err)
	}
	defer func() {
		if closeErr := listener.Close(); closeErr != nil {
			t.Errorf("release local address: %v", closeErr)
		}
	}()

	err = Run(context.Background(), testServer(listener.Addr().String()), time.Second, nil, slog.New(slog.DiscardHandler))
	if err == nil || !strings.Contains(err.Error(), "serve HTTP") {
		t.Fatalf("Run() error = %v, want listen failure", err)
	}
}

func testServer(address string) *http.Server {
	return &http.Server{
		Addr:              address,
		Handler:           http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		ReadHeaderTimeout: time.Second,
	}
}
