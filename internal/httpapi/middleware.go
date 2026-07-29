// Package httpapi composes the API router and cross-cutting HTTP middleware.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/lukuku-dev/go-chi-bp/internal/platform/httpkit"
)

func logRequest(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			startedAt := time.Now()
			recorder := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			defer func(ctx context.Context) {
				status := recorder.Status()
				if status == 0 {
					status = http.StatusOK
				}

				logger.InfoContext(
					ctx,
					"HTTP request completed",
					"requestID", middleware.GetReqID(ctx),
					"clientIP", middleware.GetClientIP(ctx),
					"method", r.Method,
					"path", r.URL.Path,
					"status", status,
					"bytes", recorder.BytesWritten(),
					"duration", time.Since(startedAt),
				)
			}(r.Context())

			next.ServeHTTP(recorder, r)
		})
	}
}

func recoverPanic(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func(ctx context.Context) {
				if recovered := recover(); recovered != nil {
					logger.ErrorContext(
						ctx,
						"HTTP handler panicked",
						"requestID", middleware.GetReqID(ctx),
						"stack", string(debug.Stack()),
					)
					statusWriter, ok := w.(interface{ Status() int })
					if !ok || statusWriter.Status() == 0 {
						httpkit.WriteError(w, http.StatusInternalServerError, "internal_error", "An internal error occurred.")
					}
				}
			}(r.Context())

			next.ServeHTTP(w, r)
		})
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
