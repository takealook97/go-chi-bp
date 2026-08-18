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
	t.Setenv("HTTP_CORS_ALLOWED_ORIGINS", "https://app.example.com, https://admin.example.com")
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
	if len(cfg.HTTP.CORS.AllowedOrigins) != 2 || cfg.HTTP.CORS.AllowedOrigins[1] != "https://admin.example.com" {
		t.Fatalf("allowed origins = %v, want two trimmed origins", cfg.HTTP.CORS.AllowedOrigins)
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
		{name: "invalid maximum request bytes", key: "HTTP_MAX_REQUEST_BYTES", value: "0", wantError: "HTTP_MAX_REQUEST_BYTES"},
		{name: "invalid CORS credentials", key: "HTTP_CORS_ALLOW_CREDENTIALS", value: "sometimes", wantError: "HTTP_CORS_ALLOW_CREDENTIALS"},
		{name: "invalid client IP mode", key: "HTTP_CLIENT_IP_MODE", value: "automatic", wantError: "HTTP_CLIENT_IP_MODE"},
		{name: "invalid maximum connections", key: "DB_MAX_CONNS", value: "0", wantError: "DB_MAX_CONNS"},
		{name: "invalid minimum connections", key: "DB_MIN_CONNS", value: "-1", wantError: "DB_MIN_CONNS"},
		{name: "minimum exceeds maximum", key: "DB_MIN_CONNS", value: "11", wantError: "must not exceed"},
		{name: "invalid connection lifetime", key: "DB_MAX_CONN_LIFETIME", value: "0s", wantError: "DB_MAX_CONN_LIFETIME"},
		{name: "invalid idle time", key: "DB_MAX_CONN_IDLE_TIME", value: "invalid", wantError: "DB_MAX_CONN_IDLE_TIME"},
		{name: "invalid statement timeout", key: "DB_STATEMENT_TIMEOUT", value: "0s", wantError: "DB_STATEMENT_TIMEOUT"},
		{name: "sub-millisecond statement timeout", key: "DB_STATEMENT_TIMEOUT", value: "500us", wantError: "DB_STATEMENT_TIMEOUT"},
		{name: "invalid shutdown timeout", key: "SHUTDOWN_TIMEOUT", value: "0s", wantError: "SHUTDOWN_TIMEOUT"},
		{name: "negative drain delay", key: "SHUTDOWN_DRAIN_DELAY", value: "-1s", wantError: "SHUTDOWN_DRAIN_DELAY"},
		{name: "invalid drain delay", key: "SHUTDOWN_DRAIN_DELAY", value: "invalid", wantError: "SHUTDOWN_DRAIN_DELAY"},
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

func TestLoadReportsParseFailuresAsParseFailures(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "duration", key: "HTTP_READ_TIMEOUT", value: "not-a-duration"},
		{name: "int32", key: "DB_MAX_CONNS", value: "not-a-number"},
		{name: "int64", key: "HTTP_MAX_REQUEST_BYTES", value: "not-a-number"},
		{name: "int", key: "HTTP_TRUSTED_PROXY_COUNT", value: "not-a-number"},
		{name: "bool", key: "HTTP_CORS_ALLOW_CREDENTIALS", value: "not-a-bool"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setValidEnvironment(t)
			t.Setenv(test.key, test.value)

			_, err := Load()
			if err == nil {
				t.Fatalf("Load() error = nil, want a parse failure for %s", test.key)
			}
			if !strings.Contains(err.Error(), test.key) || !strings.Contains(err.Error(), "is not a valid") {
				t.Fatalf("Load() error = %v, want %s reported as an unparsable value", err, test.key)
			}
		})
	}
}

func TestLoadAllowsZeroDrainDelay(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv("SHUTDOWN_DRAIN_DELAY", "0s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.ShutdownDrainDelay != 0 {
		t.Fatalf("shutdown drain delay = %s, want 0", cfg.ShutdownDrainDelay)
	}
}

func TestLoadRejectsCredentialedWildcardCORS(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv("HTTP_CORS_ALLOWED_ORIGINS", "*")
	t.Setenv("HTTP_CORS_ALLOW_CREDENTIALS", "true")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "must not contain *") {
		t.Fatalf("Load() error = %v, want credentialed wildcard rejection", err)
	}
}

func TestLoadValidatesTrustedProxyConfiguration(t *testing.T) {
	tests := []struct {
		name      string
		mode      string
		key       string
		value     string
		wantError string
	}{
		{name: "empty trusted header", mode: "header", key: "HTTP_CLIENT_IP_HEADER", value: "", wantError: "HTTP_CLIENT_IP_HEADER"},
		{name: "empty trusted CIDRs", mode: "xff-cidrs", key: "HTTP_TRUSTED_PROXY_CIDRS", value: "", wantError: "HTTP_TRUSTED_PROXY_CIDRS"},
		{name: "invalid trusted CIDR", mode: "xff-cidrs", key: "HTTP_TRUSTED_PROXY_CIDRS", value: "not-a-cidr", wantError: "invalid prefix"},
		{name: "invalid trusted proxy count", mode: "xff-count", key: "HTTP_TRUSTED_PROXY_COUNT", value: "0", wantError: "HTTP_TRUSTED_PROXY_COUNT"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setValidEnvironment(t)
			t.Setenv("HTTP_CLIENT_IP_MODE", test.mode)
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
		"DATABASE_URL":                "postgres://example.invalid/app",
		"HTTP_ADDR":                   ":8080",
		"HTTP_READ_HEADER_TIMEOUT":    "5s",
		"HTTP_READ_TIMEOUT":           "15s",
		"HTTP_WRITE_TIMEOUT":          "30s",
		"HTTP_IDLE_TIMEOUT":           "60s",
		"HTTP_MAX_REQUEST_BYTES":      "1048576",
		"HTTP_CORS_ALLOWED_ORIGINS":   "",
		"HTTP_CORS_ALLOW_CREDENTIALS": "false",
		"HTTP_CLIENT_IP_MODE":         "remote",
		"HTTP_CLIENT_IP_HEADER":       "X-Real-IP",
		"HTTP_TRUSTED_PROXY_CIDRS":    "",
		"HTTP_TRUSTED_PROXY_COUNT":    "1",
		"DB_MAX_CONNS":                "10",
		"DB_MIN_CONNS":                "2",
		"DB_MAX_CONN_LIFETIME":        "30m",
		"DB_MAX_CONN_IDLE_TIME":       "5m",
		"DB_STATEMENT_TIMEOUT":        "5s",
		"SHUTDOWN_TIMEOUT":            "10s",
		"SHUTDOWN_DRAIN_DELAY":        "0s",
	} {
		t.Setenv(key, value)
	}
}
