package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/lukuku-dev/go-chi-bp/internal/httpapi/apigen"
	"github.com/lukuku-dev/go-chi-bp/internal/platform/httpkit"
)

const (
	// corsPreflightMaxAge is how long a browser may reuse one preflight result.
	corsPreflightMaxAge = 300
	// readinessCheckTimeout bounds every readiness check together. A probe that
	// hangs is a failed probe: an orchestrator that never gets an answer keeps
	// routing to an instance that already cannot serve.
	readinessCheckTimeout = time.Second
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
	// RequestTimeout bounds how long one handler may run. A zero value leaves
	// requests unbounded, which suits tests that assemble a router directly;
	// configuration validation keeps the running application from doing so.
	RequestTimeout time.Duration
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
	router.Use(requestID)
	router.Use(clientIPMiddleware(options.ClientIP))
	router.Use(securityHeaders)
	router.Use(logRequest(logger))
	router.Use(recoverPanic(logger))
	// After the panic guard so a handler that gives up on its deadline is still
	// covered, and before the routes so every one of them inherits the deadline.
	if options.RequestTimeout > 0 {
		router.Use(requestTimeout(options.RequestTimeout))
	}
	if len(options.CORS.AllowedOrigins) > 0 {
		if options.CORS.AllowCredentials && slices.Contains(options.CORS.AllowedOrigins, "*") {
			panic("credentialed CORS must not allow wildcard origins")
		}
		router.Use(cors.Handler(cors.Options{
			AllowedOrigins:   options.CORS.AllowedOrigins,
			AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
			AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", requestIDHeader},
			ExposedHeaders:   []string{requestIDHeader},
			AllowCredentials: options.CORS.AllowCredentials,
			MaxAge:           corsPreflightMaxAge,
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
		ctx, cancel := context.WithTimeout(r.Context(), readinessCheckTimeout)
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
