// Package app composes the application's capability modules and outer adapters.
package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync/atomic"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lukuku-dev/go-chi-bp/internal/httpapi"
	"github.com/lukuku-dev/go-chi-bp/internal/platform/config"
	"github.com/lukuku-dev/go-chi-bp/internal/platform/httpkit"
	"github.com/lukuku-dev/go-chi-bp/internal/widget"
	"github.com/lukuku-dev/go-chi-bp/internal/widget/widgethttp"
	"github.com/lukuku-dev/go-chi-bp/internal/widget/widgetpostgres"
	dbgen "github.com/lukuku-dev/go-chi-bp/internal/widget/widgetpostgres/dbgen"
)

// Dependencies are the capability ports and health checks supplied to the composition harness.
type Dependencies struct {
	WidgetService   widgethttp.Service
	ReadinessChecks []httpapi.ReadinessCheck
}

// App is an assembled HTTP application with explicit lifecycle control.
type App struct {
	handler          http.Handler
	acceptingTraffic atomic.Bool
}

// New assembles production adapters around the application capabilities.
func New(cfg config.Config, logger *slog.Logger, pool *pgxpool.Pool) *App {
	if pool == nil {
		panic("database pool must not be nil")
	}

	widgetRepository := widgetpostgres.NewPostgresRepository(dbgen.New(pool))

	return Build(cfg, logger, Dependencies{
		WidgetService:   widget.NewService(widgetRepository),
		ReadinessChecks: []httpapi.ReadinessCheck{pool.Ping},
	})
}

// Build assembles the HTTP application from replaceable capability ports.
func Build(cfg config.Config, logger *slog.Logger, dependencies Dependencies) *App {
	if logger == nil || dependencies.WidgetService == nil || len(dependencies.ReadinessChecks) == 0 {
		panic("application dependencies must not be nil")
	}

	application := &App{}
	application.acceptingTraffic.Store(true)
	// Wrap once here so that everything assembled below reports the request its
	// records belong to. Components log through the request context already, but
	// slog handlers ignore context on their own.
	requestLogger := slog.New(httpapi.NewLogHandler(logger.Handler()))
	widgetHandler := widgethttp.NewHandler(
		dependencies.WidgetService,
		requestLogger,
		httpkit.NewJSONDecoder(cfg.HTTP.MaxRequestBytes),
	)
	readinessChecks := append([]httpapi.ReadinessCheck{application.checkAcceptingTraffic}, dependencies.ReadinessChecks...)
	application.handler = httpapi.NewRouter(
		requestLogger,
		readinessChecks,
		[]httpapi.RouteMount{{Pattern: "/v1/widgets", Handler: widgetHandler.Router()}},
		httpapi.Options{
			CORS: httpapi.CORSOptions{
				AllowedOrigins:   cfg.HTTP.CORS.AllowedOrigins,
				AllowCredentials: cfg.HTTP.CORS.AllowCredentials,
			},
			RequestTimeout: cfg.HTTP.RequestTimeout,
			ClientIP: httpapi.ClientIPOptions{
				Mode:              cfg.HTTP.ClientIP.Mode,
				TrustedHeader:     cfg.HTTP.ClientIP.TrustedHeader,
				TrustedProxyCIDRs: cfg.HTTP.ClientIP.TrustedProxyCIDRs,
				TrustedProxyCount: cfg.HTTP.ClientIP.TrustedProxyCount,
			},
		},
	)

	return application
}

// Handler returns the fully assembled inbound HTTP handler.
func (application *App) Handler() http.Handler {
	return application.handler
}

// BeginDrain removes the application from readiness before server shutdown.
func (application *App) BeginDrain() {
	application.acceptingTraffic.Store(false)
}

func (application *App) checkAcceptingTraffic(context.Context) error {
	if !application.acceptingTraffic.Load() {
		return errors.New("application is draining")
	}

	return nil
}
