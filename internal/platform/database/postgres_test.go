package database

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lukuku-dev/go-chi-bp/internal/platform/config"
)

func TestOpenRejectsInvalidURL(t *testing.T) {
	t.Parallel()

	_, err := Open(context.Background(), config.Database{URL: "://invalid"})
	if err == nil || !strings.Contains(err.Error(), "parse database configuration") {
		t.Fatalf("Open() error = %v, want configuration parsing error", err)
	}
}

func TestOpenIntegration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := Open(ctx, config.Database{
		URL:             databaseURL,
		MaxConnections:  4,
		MinConnections:  1,
		MaxConnLifetime: time.Minute,
		MaxConnIdleTime: time.Minute,
	})
	if err != nil {
		t.Fatalf("Open() unexpected error: %v", err)
	}
	defer pool.Close()

	if pool.Config().MaxConns != 4 || pool.Config().MinConns != 1 {
		t.Fatalf("pool bounds = %d..%d, want 1..4", pool.Config().MinConns, pool.Config().MaxConns)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("Ping() unexpected error: %v", err)
	}
}
