package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/lukuku-dev/go-chi-bp/internal/platform/httpkit"
	"github.com/lukuku-dev/go-chi-bp/internal/widget"
)

type emptyWidgetRepository struct{}

func (emptyWidgetRepository) Create(_ context.Context, _ string) (widget.Widget, error) {
	return widget.Widget{}, nil
}

func (emptyWidgetRepository) Get(_ context.Context, _ int64) (widget.Widget, error) {
	return widget.Widget{}, widget.ErrNotFound
}

func (emptyWidgetRepository) List(_ context.Context, _ widget.ListOptions) ([]widget.Widget, error) {
	return []widget.Widget{}, nil
}

func (emptyWidgetRepository) Delete(_ context.Context, _ int64) error {
	return widget.ErrNotFound
}

func TestLiveness(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.DiscardHandler)
	service := widget.NewService(emptyWidgetRepository{})
	router := NewRouter(
		logger,
		func(context.Context) error { return nil },
		widget.NewHandler(service, logger, httpkit.NewJSONDecoder(1<<20)),
		Options{},
	)
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health/live", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
}

func TestReadinessFailure(t *testing.T) {
	t.Parallel()

	router := testRouter(func(context.Context) error { return errors.New("dependency unavailable") })
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health/ready", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	if !strings.Contains(response.Body.String(), `"code":"not_ready"`) {
		t.Fatalf("body = %q, want readiness error", response.Body.String())
	}
}

func TestUnknownRouteUsesErrorEnvelope(t *testing.T) {
	t.Parallel()

	router := testRouter(func(context.Context) error { return nil })
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/unknown", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
	if !strings.Contains(response.Body.String(), `"code":"route_not_found"`) {
		t.Fatalf("body = %q, want route error", response.Body.String())
	}
}

func TestCORSAllowsConfiguredOrigin(t *testing.T) {
	t.Parallel()

	router := testRouterWithOptions(
		func(context.Context) error { return nil },
		Options{CORS: CORSOptions{AllowedOrigins: []string{"https://app.example.com"}}},
	)
	request := httptest.NewRequestWithContext(context.Background(), http.MethodOptions, "/v1/widgets", nil)
	request.Header.Set("Origin", "https://app.example.com")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want configured origin", got)
	}
	if got := response.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
}

func TestClientIPUsesOnlyConfiguredTrustedHeader(t *testing.T) {
	t.Parallel()

	var clientIP string
	handler := clientIPMiddleware(ClientIPOptions{
		Mode:          "header",
		TrustedHeader: "CF-Connecting-IP",
	})(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		clientIP = middleware.GetClientIP(request.Context())
	}))
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	request.Header.Set("X-Forwarded-For", "198.51.100.1")
	request.Header.Set("CF-Connecting-IP", "203.0.113.7")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if clientIP != "203.0.113.7" {
		t.Fatalf("client IP = %q, want trusted header value", clientIP)
	}
}

func testRouter(check ReadinessCheck) http.Handler {
	return testRouterWithOptions(check, Options{})
}

func testRouterWithOptions(check ReadinessCheck, options Options) http.Handler {
	logger := slog.New(slog.DiscardHandler)
	service := widget.NewService(emptyWidgetRepository{})

	return NewRouter(
		logger,
		check,
		widget.NewHandler(service, logger, httpkit.NewJSONDecoder(1<<20)),
		options,
	)
}
