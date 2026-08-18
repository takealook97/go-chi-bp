package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

// An error logged from inside a request must name the request that caused it.
// Without that, a 500 in the log cannot be tied to the completion line that
// records its route and status.
func TestErrorsLoggedDuringARequestCarryItsRequestID(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	logger := slog.New(NewLogHandler(slog.NewJSONHandler(&output, nil)))
	router := NewRouter(
		logger,
		[]ReadinessCheck{func(context.Context) error { return nil }},
		[]RouteMount{{Pattern: "/failing", Handler: http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			logger.ErrorContext(r.Context(), "handler failed")
		})}},
		Options{},
	)

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/failing", nil)
	router.ServeHTTP(httptest.NewRecorder(), request)

	requestIDs := loggedRequestIDs(t, output.Bytes(), "handler failed", "HTTP request completed")
	if requestIDs["handler failed"] == "" {
		t.Fatal("the error log carries no request ID")
	}
	if requestIDs["handler failed"] != requestIDs["HTTP request completed"] {
		t.Fatalf("error request ID = %q, want the completion log's %q",
			requestIDs["handler failed"], requestIDs["HTTP request completed"])
	}
}

// slog.Logger.With returns a derived handler. A wrapper that forgets to rewrap
// there silently stops adding request IDs for every component that annotates
// its logger.
func TestDerivedLoggersStillCarryTheRequestID(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	logger := slog.New(NewLogHandler(slog.NewJSONHandler(&output, nil)))
	router := NewRouter(
		logger,
		[]ReadinessCheck{func(context.Context) error { return nil }},
		[]RouteMount{{Pattern: "/failing", Handler: http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			logger.With("operation", "create widget").ErrorContext(r.Context(), "handler failed")
		})}},
		Options{},
	)

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/failing", nil)
	router.ServeHTTP(httptest.NewRecorder(), request)

	if requestIDs := loggedRequestIDs(t, output.Bytes(), "handler failed"); requestIDs["handler failed"] == "" {
		t.Fatal("a logger derived with With() carries no request ID")
	}
}

// Records logged outside a request must stay as they were.
func TestRecordsWithoutARequestAreUnchanged(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	slog.New(NewLogHandler(slog.NewJSONHandler(&output, nil))).Info("application starting")

	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatalf("decode log record: %v", err)
	}
	if _, ok := record["requestID"]; ok {
		t.Fatalf("record logged outside a request has a request ID: %v", record)
	}
}

// loggedRequestIDs maps each wanted log message to the request ID it recorded.
func loggedRequestIDs(t *testing.T, output []byte, wanted ...string) map[string]string {
	t.Helper()

	result := make(map[string]string, len(wanted))
	for line := range bytes.Lines(output) {
		var record struct {
			Message   string `json:"msg"`
			RequestID string `json:"requestID"`
		}
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatalf("decode log record %q: %v", line, err)
		}
		for _, message := range wanted {
			if record.Message == message {
				result[message] = record.RequestID
			}
		}
	}
	for _, message := range wanted {
		if _, ok := result[message]; !ok {
			t.Fatalf("no log record with message %q in:\n%s", message, output)
		}
	}

	return result
}
