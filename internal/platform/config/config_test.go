package config

import (
	"strings"
	"testing"
)

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("Load() error = %v, want DATABASE_URL validation error", err)
	}
}

func TestLoadValidConfiguration(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example.invalid/app")
	t.Setenv("HTTP_ADDR", ":9090")
	t.Setenv("DB_MAX_CONNS", "8")
	t.Setenv("DB_MIN_CONNS", "1")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.HTTP.Address != ":9090" {
		t.Fatalf("HTTP address = %q, want %q", cfg.HTTP.Address, ":9090")
	}
	if cfg.Database.MaxConnections != 8 {
		t.Fatalf("max connections = %d, want 8", cfg.Database.MaxConnections)
	}
}
