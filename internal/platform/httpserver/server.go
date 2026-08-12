// Package httpserver configures and runs the inbound HTTP server.
package httpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/lukuku-dev/go-chi-bp/internal/platform/config"
)

// New constructs an HTTP server with explicit production timeouts.
func New(cfg config.HTTP, handler http.Handler, logger *slog.Logger) *http.Server {
	return &http.Server{
		Addr:              cfg.Address,
		Handler:           handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}
}

// Run serves requests until the context is canceled, then shuts down cleanly.
func Run(
	ctx context.Context,
	server *http.Server,
	shutdownTimeout time.Duration,
	beforeShutdown func(),
	logger *slog.Logger,
) error {
	errChannel := make(chan error, 1)
	go func() {
		logger.Info("HTTP server started", "address", server.Addr)
		errChannel <- server.ListenAndServe()
	}()

	select {
	case err := <-errChannel:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
	case <-ctx.Done():
		if beforeShutdown != nil {
			beforeShutdown()
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shut down HTTP server: %w", err)
	}

	logger.Info("HTTP server stopped")

	return nil
}
