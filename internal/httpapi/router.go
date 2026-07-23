package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/lukuku-dev/go-chi-bp/internal/platform/httpkit"
	"github.com/lukuku-dev/go-chi-bp/internal/widget"
)

// ReadinessCheck verifies a dependency required to serve traffic.
type ReadinessCheck func(ctx context.Context) error

// NewRouter composes all HTTP middleware and module routes.
func NewRouter(logger *slog.Logger, readinessCheck ReadinessCheck, widgetHandler *widget.Handler) http.Handler {
	if logger == nil || readinessCheck == nil || widgetHandler == nil {
		panic("HTTP router dependencies must not be nil")
	}

	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(securityHeaders)
	router.Use(logRequest(logger))
	router.Use(recoverPanic(logger))

	router.Get("/health/live", liveness)
	router.Get("/health/ready", readiness(readinessCheck))
	router.Mount("/v1/widgets", widgetHandler.Router())

	router.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		httpkit.WriteError(w, http.StatusNotFound, "route_not_found", "Route was not found.")
	})
	router.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		httpkit.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method is not allowed.")
	})

	return router
}

func liveness(w http.ResponseWriter, _ *http.Request) {
	httpkit.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func readiness(check ReadinessCheck) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()

		if err := check(ctx); err != nil {
			httpkit.WriteError(w, http.StatusServiceUnavailable, "not_ready", "Service is not ready.")

			return
		}

		httpkit.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}
