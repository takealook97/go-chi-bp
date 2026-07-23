package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
	router := NewRouter(logger, func(context.Context) error { return nil }, widget.NewHandler(service, logger))
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

func testRouter(check ReadinessCheck) http.Handler {
	logger := slog.New(slog.DiscardHandler)
	service := widget.NewService(emptyWidgetRepository{})

	return NewRouter(logger, check, widget.NewHandler(service, logger))
}
