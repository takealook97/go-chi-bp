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

// ShutdownOptions controls how the server stops serving traffic.
type ShutdownOptions struct {
	// BeginDrain marks the application unready. It is optional.
	BeginDrain func()
	// DrainDelay is how long the server keeps accepting requests after
	// BeginDrain runs. Readiness probes are polled on an interval, so a router
	// only stops sending traffic some time after the probe starts failing.
	// Shutting down immediately would drop the requests routed in that window.
	DrainDelay time.Duration
	// Timeout bounds the graceful shutdown of requests still in flight. It is
	// spent after DrainDelay, so the two together bound total stop time.
	Timeout time.Duration
}

// Run serves requests until the context is canceled, then shuts down cleanly.
func Run(ctx context.Context, server *http.Server, options ShutdownOptions, logger *slog.Logger) error {
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
		if options.BeginDrain != nil {
			options.BeginDrain()
		}
		if err := drain(errChannel, options.DrainDelay, logger); err != nil {
			return err
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), options.Timeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shut down HTTP server: %w", err)
	}

	logger.Info("HTTP server stopped")

	return nil
}

// drain keeps serving for the configured delay so routers can observe the
// failing readiness probe before in-flight shutdown begins.
func drain(errChannel <-chan error, delay time.Duration, logger *slog.Logger) error {
	if delay <= 0 {
		return nil
	}

	logger.Info("HTTP server draining", "delay", delay)

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case err := <-errChannel:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
	case <-timer.C:
	}

	return nil
}
