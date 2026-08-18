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
	options := ShutdownOptions{Timeout: time.Second, BeginDrain: func() { drained = true }}
	if err := Run(ctx, server, options, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if !drained {
		t.Fatal("Run() did not begin draining before shutdown")
	}
}

func TestRunKeepsServingForTheDrainDelay(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var drainedAt time.Time
	options := ShutdownOptions{
		DrainDelay: 150 * time.Millisecond,
		Timeout:    time.Second,
		BeginDrain: func() { drainedAt = time.Now() },
	}

	startedAt := time.Now()
	if err := Run(ctx, testServer("127.0.0.1:0"), options, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	if drainedAt.IsZero() {
		t.Fatal("Run() did not begin draining before shutdown")
	}
	if elapsed := time.Since(startedAt); elapsed < options.DrainDelay {
		t.Fatalf("Run() returned after %s, want at least the %s drain delay", elapsed, options.DrainDelay)
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

	options := ShutdownOptions{DrainDelay: time.Minute, Timeout: time.Second}
	err = Run(context.Background(), testServer(listener.Addr().String()), options, slog.New(slog.DiscardHandler))
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
