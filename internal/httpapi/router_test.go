package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
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
