package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/lukuku-dev/go-chi-bp/internal/httpapi/apigen"
	"github.com/lukuku-dev/go-chi-bp/internal/platform/httpkit"
)

// ReadinessCheck verifies a dependency required to serve traffic.
type ReadinessCheck func(ctx context.Context) error

// RouteMount describes one capability-owned HTTP route tree.
type RouteMount struct {
	Pattern string
	Handler http.Handler
}

// Options configures browser and trusted-proxy HTTP behavior.
type Options struct {
	CORS     CORSOptions
	ClientIP ClientIPOptions
}

// CORSOptions configures browser cross-origin access.
type CORSOptions struct {
	AllowedOrigins   []string
	AllowCredentials bool
}

// ClientIPOptions configures the trusted source of client addresses.
type ClientIPOptions struct {
	Mode              string
	TrustedHeader     string
	TrustedProxyCIDRs []string
	TrustedProxyCount int
}

// NewRouter composes all HTTP middleware and module routes.
func NewRouter(
	logger *slog.Logger,
	readinessChecks []ReadinessCheck,
	routes []RouteMount,
	options Options,
) http.Handler {
	if logger == nil || len(readinessChecks) == 0 {
		panic("HTTP router dependencies must not be nil")
	}
	for _, check := range readinessChecks {
		if check == nil {
			panic("HTTP readiness checks must not be nil")
		}
	}
	for _, route := range routes {
		if route.Pattern == "" || route.Handler == nil {
			panic("HTTP route mount must have a pattern and handler")
		}
	}

	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(clientIPMiddleware(options.ClientIP))
	router.Use(securityHeaders)
	router.Use(logRequest(logger))
	router.Use(recoverPanic(logger))
	if len(options.CORS.AllowedOrigins) > 0 {
		if options.CORS.AllowCredentials && contains(options.CORS.AllowedOrigins, "*") {
			panic("credentialed CORS must not allow wildcard origins")
		}
		router.Use(cors.Handler(cors.Options{
			AllowedOrigins:   options.CORS.AllowedOrigins,
			AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
			AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
			ExposedHeaders:   []string{"X-Request-ID"},
			AllowCredentials: options.CORS.AllowCredentials,
			MaxAge:           300,
		}))
	}

	router.Get("/health/live", liveness)
	router.Get("/health/ready", readiness(readinessChecks))
	for _, route := range routes {
		router.Mount(route.Pattern, route.Handler)
	}

	router.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		httpkit.WriteError(w, http.StatusNotFound, "route_not_found", "Route was not found.")
	})
	router.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		httpkit.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method is not allowed.")
	})

	return router
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}

	return false
}

func clientIPMiddleware(options ClientIPOptions) func(http.Handler) http.Handler {
	switch options.Mode {
	case "", "remote":
		return middleware.ClientIPFromRemoteAddr
	case "header":
		return middleware.ClientIPFromHeader(options.TrustedHeader)
	case "xff-cidrs":
		return middleware.ClientIPFromXFF(options.TrustedProxyCIDRs...)
	case "xff-count":
		return middleware.ClientIPFromXFFTrustedProxies(options.TrustedProxyCount)
	default:
		panic("unsupported client IP mode")
	}
}

func liveness(w http.ResponseWriter, _ *http.Request) {
	_ = httpkit.WriteJSON(w, http.StatusOK, apigen.HealthResponse{Status: apigen.Ok})
}

func readiness(checks []ReadinessCheck) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()

		for _, check := range checks {
			if err := check(ctx); err != nil {
				httpkit.WriteError(w, http.StatusServiceUnavailable, "not_ready", "Service is not ready.")

				return
			}
		}

		_ = httpkit.WriteJSON(w, http.StatusOK, apigen.HealthResponse{Status: apigen.Ok})
	}
}
