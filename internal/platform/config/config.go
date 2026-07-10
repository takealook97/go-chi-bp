// Package config loads and validates process configuration.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config contains all process configuration.
type Config struct {
	Environment     string
	HTTP            HTTP
	Database        Database
	ShutdownTimeout time.Duration
}

// HTTP contains inbound HTTP server settings.
type HTTP struct {
	Address           string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
}

// Database contains PostgreSQL pool settings.
type Database struct {
	URL             string
	MaxConnections  int32
	MinConnections  int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

// Load reads and validates configuration from the process environment.
func Load() (Config, error) {
	cfg := Config{
		Environment: envString("APP_ENV", "development"),
		HTTP: HTTP{
			Address:           envString("HTTP_ADDR", ":8080"),
			ReadHeaderTimeout: envDuration("HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
			ReadTimeout:       envDuration("HTTP_READ_TIMEOUT", 15*time.Second),
			WriteTimeout:      envDuration("HTTP_WRITE_TIMEOUT", 30*time.Second),
			IdleTimeout:       envDuration("HTTP_IDLE_TIMEOUT", 60*time.Second),
		},
		Database: Database{
			URL:             os.Getenv("DATABASE_URL"),
			MaxConnections:  envInt32("DB_MAX_CONNS", 10),
			MinConnections:  envInt32("DB_MIN_CONNS", 2),
			MaxConnLifetime: envDuration("DB_MAX_CONN_LIFETIME", 30*time.Minute),
			MaxConnIdleTime: envDuration("DB_MAX_CONN_IDLE_TIME", 5*time.Minute),
		},
		ShutdownTimeout: envDuration("SHUTDOWN_TIMEOUT", 10*time.Second),
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
	if cfg.ShutdownTimeout <= 0 {
		errs = append(errs, errors.New("SHUTDOWN_TIMEOUT must be positive"))
	}

	return errors.Join(errs...)
}

func envString(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}

	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return -1
	}

	return parsed
}

func envInt32(key string, fallback int32) int32 {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}

	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return -1
	}

	return int32(parsed)
}

// String returns a safe summary that intentionally omits credentials.
func (cfg Config) String() string {
	return fmt.Sprintf("environment=%s httpAddress=%s", cfg.Environment, cfg.HTTP.Address)
}
