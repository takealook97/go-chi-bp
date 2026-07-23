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
	setValidEnvironment(t)
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

func TestLoadRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		value     string
		wantError string
	}{
		{name: "empty HTTP address", key: "HTTP_ADDR", value: "", wantError: "HTTP_ADDR"},
		{name: "invalid read header timeout", key: "HTTP_READ_HEADER_TIMEOUT", value: "invalid", wantError: "HTTP_READ_HEADER_TIMEOUT"},
		{name: "invalid read timeout", key: "HTTP_READ_TIMEOUT", value: "0s", wantError: "HTTP_READ_TIMEOUT"},
		{name: "invalid write timeout", key: "HTTP_WRITE_TIMEOUT", value: "-1s", wantError: "HTTP_WRITE_TIMEOUT"},
		{name: "invalid idle timeout", key: "HTTP_IDLE_TIMEOUT", value: "invalid", wantError: "HTTP_IDLE_TIMEOUT"},
		{name: "invalid maximum connections", key: "DB_MAX_CONNS", value: "0", wantError: "DB_MAX_CONNS"},
		{name: "invalid minimum connections", key: "DB_MIN_CONNS", value: "-1", wantError: "DB_MIN_CONNS"},
		{name: "minimum exceeds maximum", key: "DB_MIN_CONNS", value: "11", wantError: "must not exceed"},
		{name: "invalid connection lifetime", key: "DB_MAX_CONN_LIFETIME", value: "0s", wantError: "DB_MAX_CONN_LIFETIME"},
		{name: "invalid idle time", key: "DB_MAX_CONN_IDLE_TIME", value: "invalid", wantError: "DB_MAX_CONN_IDLE_TIME"},
		{name: "invalid shutdown timeout", key: "SHUTDOWN_TIMEOUT", value: "0s", wantError: "SHUTDOWN_TIMEOUT"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setValidEnvironment(t)
			t.Setenv(test.key, test.value)

			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Load() error = %v, want error containing %q", err, test.wantError)
			}
		})
	}
}

func TestConfigStringOmitsDatabaseURL(t *testing.T) {
	t.Parallel()

	cfg := Config{
		Environment: "production",
		HTTP:        HTTP{Address: ":8080"},
		Database:    Database{URL: "opaque-database-configuration"},
	}

	result := cfg.String()
	if strings.Contains(result, cfg.Database.URL) {
		t.Fatalf("Config.String() exposes database credentials: %q", result)
	}
}

func setValidEnvironment(t *testing.T) {
	t.Helper()

	for key, value := range map[string]string{
		"DATABASE_URL":             "postgres://example.invalid/app",
		"HTTP_ADDR":                ":8080",
		"HTTP_READ_HEADER_TIMEOUT": "5s",
		"HTTP_READ_TIMEOUT":        "15s",
		"HTTP_WRITE_TIMEOUT":       "30s",
		"HTTP_IDLE_TIMEOUT":        "60s",
		"DB_MAX_CONNS":             "10",
		"DB_MIN_CONNS":             "2",
		"DB_MAX_CONN_LIFETIME":     "30m",
		"DB_MAX_CONN_IDLE_TIME":    "5m",
		"SHUTDOWN_TIMEOUT":         "10s",
	} {
		t.Setenv(key, value)
	}
}
