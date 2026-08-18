// Package config loads and validates process configuration.
package config

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config contains all process configuration.
type Config struct {
	Environment string
	HTTP        HTTP
	Database    Database
	// ShutdownDrainDelay keeps the server serving after readiness starts
	// failing, so routers can stop sending traffic before shutdown begins.
	ShutdownDrainDelay time.Duration
	ShutdownTimeout    time.Duration
}

// HTTP contains inbound HTTP server settings.
type HTTP struct {
	Address           string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	MaxRequestBytes   int64
	CORS              CORS
	ClientIP          ClientIP
}

// CORS contains browser cross-origin policy settings.
type CORS struct {
	AllowedOrigins   []string
	AllowCredentials bool
}

// ClientIP describes how the trusted client address is derived.
type ClientIP struct {
	Mode              string
	TrustedHeader     string
	TrustedProxyCIDRs []string
	TrustedProxyCount int
}

// Database contains PostgreSQL pool settings.
type Database struct {
	URL             string
	MaxConnections  int32
	MinConnections  int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
	// StatementTimeout bounds how long one statement may run. Server read and
	// write timeouts do not cancel a request's context, so without a server-side
	// limit a slow query holds its pooled connection until the client goes away.
	StatementTimeout time.Duration
}

// Load reads and validates configuration from the process environment.
func Load() (Config, error) {
	reader := &envReader{}
	cfg := Config{
		Environment: reader.Text("APP_ENV", "development"),
		HTTP: HTTP{
			Address:           reader.Text("HTTP_ADDR", ":8080"),
			ReadHeaderTimeout: reader.Duration("HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
			ReadTimeout:       reader.Duration("HTTP_READ_TIMEOUT", 15*time.Second),
			WriteTimeout:      reader.Duration("HTTP_WRITE_TIMEOUT", 30*time.Second),
			IdleTimeout:       reader.Duration("HTTP_IDLE_TIMEOUT", 60*time.Second),
			MaxRequestBytes:   reader.Int64("HTTP_MAX_REQUEST_BYTES", 1<<20),
			CORS: CORS{
				AllowedOrigins:   reader.List("HTTP_CORS_ALLOWED_ORIGINS"),
				AllowCredentials: reader.Bool("HTTP_CORS_ALLOW_CREDENTIALS", false),
			},
			ClientIP: ClientIP{
				Mode:              reader.Text("HTTP_CLIENT_IP_MODE", "remote"),
				TrustedHeader:     reader.Text("HTTP_CLIENT_IP_HEADER", "X-Real-IP"),
				TrustedProxyCIDRs: reader.List("HTTP_TRUSTED_PROXY_CIDRS"),
				TrustedProxyCount: reader.Int("HTTP_TRUSTED_PROXY_COUNT", 1),
			},
		},
		Database: Database{
			URL:              reader.Text("DATABASE_URL", ""),
			MaxConnections:   reader.Int32("DB_MAX_CONNS", 10),
			MinConnections:   reader.Int32("DB_MIN_CONNS", 2),
			MaxConnLifetime:  reader.Duration("DB_MAX_CONN_LIFETIME", 30*time.Minute),
			MaxConnIdleTime:  reader.Duration("DB_MAX_CONN_IDLE_TIME", 5*time.Minute),
			StatementTimeout: reader.Duration("DB_STATEMENT_TIMEOUT", 5*time.Second),
		},
		ShutdownDrainDelay: reader.Duration("SHUTDOWN_DRAIN_DELAY", 5*time.Second),
		ShutdownTimeout:    reader.Duration("SHUTDOWN_TIMEOUT", 10*time.Second),
	}

	// Report unparsable values on their own. Continuing to validate would judge
	// the fallbacks that replaced them and describe the wrong problem.
	if err := errors.Join(reader.errs...); err != nil {
		return Config{}, err
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (cfg Config) validate() error {
	var errs []error

	if cfg.HTTP.Address == "" {
		errs = append(errs, errors.New("HTTP_ADDR must not be empty"))
	}
	if cfg.HTTP.ReadHeaderTimeout <= 0 {
		errs = append(errs, errors.New("HTTP_READ_HEADER_TIMEOUT must be a positive duration"))
	}
	if cfg.HTTP.ReadTimeout <= 0 {
		errs = append(errs, errors.New("HTTP_READ_TIMEOUT must be a positive duration"))
	}
	if cfg.HTTP.WriteTimeout <= 0 {
		errs = append(errs, errors.New("HTTP_WRITE_TIMEOUT must be a positive duration"))
	}
	if cfg.HTTP.IdleTimeout <= 0 {
		errs = append(errs, errors.New("HTTP_IDLE_TIMEOUT must be a positive duration"))
	}
	if cfg.HTTP.MaxRequestBytes < 1 {
		errs = append(errs, errors.New("HTTP_MAX_REQUEST_BYTES must be at least 1"))
	}
	if cfg.HTTP.CORS.AllowCredentials && slicesContain(cfg.HTTP.CORS.AllowedOrigins, "*") {
		errs = append(errs, errors.New("HTTP_CORS_ALLOWED_ORIGINS must not contain * when credentials are allowed"))
	}
	switch cfg.HTTP.ClientIP.Mode {
	case "remote":
	case "header":
		if cfg.HTTP.ClientIP.TrustedHeader == "" {
			errs = append(errs, errors.New("HTTP_CLIENT_IP_HEADER must not be empty in header mode"))
		}
	case "xff-cidrs":
		if len(cfg.HTTP.ClientIP.TrustedProxyCIDRs) == 0 {
			errs = append(errs, errors.New("HTTP_TRUSTED_PROXY_CIDRS must not be empty in xff-cidrs mode"))
		}
		for _, prefix := range cfg.HTTP.ClientIP.TrustedProxyCIDRs {
			if _, err := netip.ParsePrefix(prefix); err != nil {
				errs = append(errs, fmt.Errorf("HTTP_TRUSTED_PROXY_CIDRS contains invalid prefix %q", prefix))
			}
		}
	case "xff-count":
		if cfg.HTTP.ClientIP.TrustedProxyCount < 1 {
			errs = append(errs, errors.New("HTTP_TRUSTED_PROXY_COUNT must be at least 1 in xff-count mode"))
		}
	default:
		errs = append(errs, errors.New("HTTP_CLIENT_IP_MODE must be one of remote, header, xff-cidrs, or xff-count"))
	}
	if cfg.Database.URL == "" {
		errs = append(errs, errors.New("DATABASE_URL must not be empty"))
	}
	if cfg.Database.MaxConnections < 1 {
		errs = append(errs, errors.New("DB_MAX_CONNS must be at least 1"))
	}
	if cfg.Database.MinConnections < 0 {
		errs = append(errs, errors.New("DB_MIN_CONNS must not be negative"))
	}
	if cfg.Database.MinConnections > cfg.Database.MaxConnections {
		errs = append(errs, errors.New("DB_MIN_CONNS must not exceed DB_MAX_CONNS"))
	}
	if cfg.Database.MaxConnLifetime <= 0 {
		errs = append(errs, errors.New("DB_MAX_CONN_LIFETIME must be a positive duration"))
	}
	if cfg.Database.MaxConnIdleTime <= 0 {
		errs = append(errs, errors.New("DB_MAX_CONN_IDLE_TIME must be a positive duration"))
	}
	// PostgreSQL takes statement_timeout in whole milliseconds and treats zero as
	// unbounded, so a sub-millisecond value would round down to no limit at all.
	if cfg.Database.StatementTimeout < time.Millisecond {
		errs = append(errs, errors.New("DB_STATEMENT_TIMEOUT must be at least 1ms"))
	}
	if cfg.ShutdownDrainDelay < 0 {
		errs = append(errs, errors.New("SHUTDOWN_DRAIN_DELAY must not be negative"))
	}
	if cfg.ShutdownTimeout <= 0 {
		errs = append(errs, errors.New("SHUTDOWN_TIMEOUT must be positive"))
	}

	return errors.Join(errs...)
}

// envReader reads environment values and records the ones it cannot parse.
// Returning a sentinel instead would surface later as a range violation and
// blame the value's bounds for what is really a typo.
type envReader struct {
	errs []error
}

// Text reads a string value, or the fallback when the variable is unset.
func (reader *envReader) Text(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}

	return fallback
}

// Duration reads a Go duration such as "5s".
func (reader *envReader) Duration(key string, fallback time.Duration) time.Duration {
	return parseEnv(reader, key, fallback, "duration", time.ParseDuration)
}

// Int reads a decimal integer.
func (reader *envReader) Int(key string, fallback int) int {
	return parseEnv(reader, key, fallback, "integer", strconv.Atoi)
}

// Int32 reads a decimal integer that must fit in 32 bits.
func (reader *envReader) Int32(key string, fallback int32) int32 {
	return parseEnv(reader, key, fallback, "32-bit integer", func(value string) (int32, error) {
		parsed, err := strconv.ParseInt(value, 10, 32)

		return int32(parsed), err
	})
}

// Int64 reads a decimal integer that must fit in 64 bits.
func (reader *envReader) Int64(key string, fallback int64) int64 {
	return parseEnv(reader, key, fallback, "64-bit integer", func(value string) (int64, error) {
		return strconv.ParseInt(value, 10, 64)
	})
}

// Bool reads a boolean such as "true" or "0".
func (reader *envReader) Bool(key string, fallback bool) bool {
	return parseEnv(reader, key, fallback, "boolean", strconv.ParseBool)
}

// List reads a comma-separated list, discarding surrounding and empty entries.
func (reader *envReader) List(key string) []string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return nil
	}

	items := strings.Split(value, ",")
	result := make([]string, 0, len(items))
	for _, item := range items {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
}

// parseEnv applies parse to the variable, recording a failure and falling back
// so that later validation sees a usable value rather than a sentinel.
func parseEnv[T any](reader *envReader, key string, fallback T, kind string, parse func(string) (T, error)) T {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}

	parsed, err := parse(value)
	if err != nil {
		reader.errs = append(reader.errs, fmt.Errorf("%s is not a valid %s", key, kind))

		return fallback
	}

	return parsed
}

func slicesContain(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}

	return false
}

// String returns a safe summary that intentionally omits credentials.
func (cfg Config) String() string {
	return fmt.Sprintf("environment=%s httpAddress=%s", cfg.Environment, cfg.HTTP.Address)
}
