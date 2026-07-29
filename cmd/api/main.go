// Package main composes and runs the API process.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/lukuku-dev/go-chi-bp/internal/httpapi"
	"github.com/lukuku-dev/go-chi-bp/internal/platform/config"
	"github.com/lukuku-dev/go-chi-bp/internal/platform/database"
	"github.com/lukuku-dev/go-chi-bp/internal/platform/httpkit"
	"github.com/lukuku-dev/go-chi-bp/internal/platform/httpserver"
	"github.com/lukuku-dev/go-chi-bp/internal/widget"
	dbgen "github.com/lukuku-dev/go-chi-bp/internal/widget/dbgen"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	logger := newLogger(cfg.Environment)
	logger.Info("application starting", "environment", cfg.Environment)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.Open(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer pool.Close()

	queries := dbgen.New(pool)
	widgetRepository := widget.NewPostgresRepository(queries)
	widgetService := widget.NewService(widgetRepository)
	jsonDecoder := httpkit.NewJSONDecoder(cfg.HTTP.MaxRequestBytes)
	widgetHandler := widget.NewHandler(widgetService, logger, jsonDecoder)

	router := httpapi.NewRouter(logger, pool.Ping, widgetHandler, httpapi.Options{
		CORS: httpapi.CORSOptions{
			AllowedOrigins:   cfg.HTTP.CORS.AllowedOrigins,
			AllowCredentials: cfg.HTTP.CORS.AllowCredentials,
		},
		ClientIP: httpapi.ClientIPOptions{
			Mode:              cfg.HTTP.ClientIP.Mode,
			TrustedHeader:     cfg.HTTP.ClientIP.TrustedHeader,
			TrustedProxyCIDRs: cfg.HTTP.ClientIP.TrustedProxyCIDRs,
			TrustedProxyCount: cfg.HTTP.ClientIP.TrustedProxyCount,
		},
	})
	server := httpserver.New(cfg.HTTP, router, logger)

	if err := httpserver.Run(ctx, server, cfg.ShutdownTimeout, logger); err != nil {
		return err
	}

	return nil
}

func newLogger(environment string) *slog.Logger {
	level := slog.LevelInfo
	if environment == "development" {
		level = slog.LevelDebug
	}

	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}
