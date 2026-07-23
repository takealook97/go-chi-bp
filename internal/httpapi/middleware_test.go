package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMiddlewarePreservesFlusher(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.DiscardHandler)
	handler := logRequest(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, ok := w.(http.Flusher); !ok {
			t.Error("wrapped response writer does not implement http.Flusher")
		}

		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
}

func TestRecoverPanicWritesErrorBeforeResponseStarts(t *testing.T) {
	t.Parallel()

	handler := productionMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("test panic")
	}))
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(response.Body.String(), `"code":"internal_error"`) {
		t.Fatalf("body = %q, want internal error envelope", response.Body.String())
	}
}

func TestRecoverPanicDoesNotAppendErrorAfterResponseStarts(t *testing.T) {
	t.Parallel()

	handler := productionMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("partial"))
		panic("test panic")
	}))
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if response.Body.String() != "partial" {
		t.Fatalf("body = %q, want partial response without an appended JSON error", response.Body.String())
	}
}

func TestSecurityHeaders(t *testing.T) {
	t.Parallel()

	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	for header, want := range map[string]string{
		"Referrer-Policy":        "no-referrer",
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
	} {
		if got := response.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
}

func productionMiddleware(next http.Handler) http.Handler {
	logger := slog.New(slog.DiscardHandler)

	return logRequest(logger)(recoverPanic(logger)(next))
}
