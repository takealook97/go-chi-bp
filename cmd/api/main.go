// Package main composes and runs the API process.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/lukuku-dev/go-chi-bp/internal/app"
	"github.com/lukuku-dev/go-chi-bp/internal/platform/config"
	"github.com/lukuku-dev/go-chi-bp/internal/platform/database"
	"github.com/lukuku-dev/go-chi-bp/internal/platform/httpserver"
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

	application := app.New(cfg, logger, pool)
	server := httpserver.New(cfg.HTTP, application.Handler(), logger)

	shutdown := httpserver.ShutdownOptions{
		BeginDrain: application.BeginDrain,
		DrainDelay: cfg.ShutdownDrainDelay,
		Timeout:    cfg.ShutdownTimeout,
	}
	if err := httpserver.Run(ctx, server, shutdown, logger); err != nil {
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
