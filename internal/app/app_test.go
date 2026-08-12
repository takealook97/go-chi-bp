package app

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lukuku-dev/go-chi-bp/internal/httpapi"
	"github.com/lukuku-dev/go-chi-bp/internal/platform/config"
	"github.com/lukuku-dev/go-chi-bp/internal/widget"
)

type serviceStub struct{}

func (serviceStub) Create(context.Context, string) (widget.Widget, error) {
	return widget.Widget{}, nil
}

func (serviceStub) Get(context.Context, int64) (widget.Widget, error) {
	return widget.Widget{}, widget.ErrNotFound
}

func (serviceStub) List(context.Context, widget.ListOptions) (widget.Page, error) {
	return widget.Page{Items: []widget.Widget{}}, nil
}

func (serviceStub) Delete(context.Context, int64) error {
	return widget.ErrNotFound
}

func TestBuildProvidesInProcessApplicationHarness(t *testing.T) {
	t.Parallel()

	application := Build(config.Config{HTTP: config.HTTP{MaxRequestBytes: 1 << 20}}, slog.New(slog.DiscardHandler), Dependencies{
		WidgetService:   serviceStub{},
		ReadinessChecks: []httpapi.ReadinessCheck{func(context.Context) error { return nil }},
	})

	assertStatus(t, application.Handler(), "/health/ready", http.StatusOK)
	assertStatus(t, application.Handler(), "/v1/widgets", http.StatusOK)
	application.BeginDrain()
	assertStatus(t, application.Handler(), "/health/ready", http.StatusServiceUnavailable)
}

func assertStatus(t *testing.T, handler http.Handler, target string, want int) {
	t.Helper()

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != want {
		t.Fatalf("GET %s status = %d, want %d", target, response.Code, want)
	}
}
