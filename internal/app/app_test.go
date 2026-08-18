package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

// failingService stands in for a capability whose dependency is broken.
type failingService struct {
	serviceStub
}

func (failingService) List(context.Context, widget.ListOptions) (widget.Page, error) {
	return widget.Page{}, errors.New("dependency is unavailable")
}

// The assembled application must log a failure against the request that caused
// it. A 500 that cannot be tied to its completion line is not diagnosable.
func TestBuildCorrelatesFailureLogsWithTheirRequest(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	application := Build(
		config.Config{HTTP: config.HTTP{MaxRequestBytes: 1 << 20}},
		slog.New(slog.NewJSONHandler(&output, nil)),
		Dependencies{
			WidgetService:   failingService{},
			ReadinessChecks: []httpapi.ReadinessCheck{func(context.Context) error { return nil }},
		},
	)

	assertStatus(t, application.Handler(), "/v1/widgets", http.StatusInternalServerError)

	failure, completion := loggedRequestID(t, &output, "HTTP request failed"), loggedRequestID(t, &output, "HTTP request completed")
	if failure == "" {
		t.Fatal("the failure log carries no request ID")
	}
	if failure != completion {
		t.Fatalf("failure request ID = %q, want the completion log's %q", failure, completion)
	}
}

// loggedRequestID returns the request ID recorded by the named log message.
func loggedRequestID(t *testing.T, output *bytes.Buffer, message string) string {
	t.Helper()

	for line := range bytes.Lines(output.Bytes()) {
		var record struct {
			Message   string `json:"msg"`
			RequestID string `json:"requestID"`
		}
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatalf("decode log record %q: %v", line, err)
		}
		if record.Message == message {
			return record.RequestID
		}
	}

	t.Fatalf("no log record with message %q in:\n%s", message, output.String())

	return ""
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
