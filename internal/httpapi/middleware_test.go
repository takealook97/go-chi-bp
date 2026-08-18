package httpapi

import (
	"bytes"
	"context"
	"errors"
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

func TestRecoverPanicDoesNotLogPanicValue(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	handler := recoverPanic(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("sensitive panic value")
	}))
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if strings.Contains(output.String(), "sensitive panic value") {
		t.Fatalf("panic log contains recovered value: %s", output.String())
	}
	if !strings.Contains(output.String(), "HTTP handler panicked") {
		t.Fatalf("panic log = %q, want recovery event", output.String())
	}
}

func TestRecoverPanicPropagatesAbortHandler(t *testing.T) {
	t.Parallel()

	recovered := func() (recovered any) {
		defer func() { recovered = recover() }()

		handler := recoverPanic(slog.New(slog.DiscardHandler))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			panic(http.ErrAbortHandler)
		}))
		request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		handler.ServeHTTP(httptest.NewRecorder(), request)

		return nil
	}()

	err, ok := recovered.(error)
	if !ok || !errors.Is(err, http.ErrAbortHandler) {
		t.Fatalf("recovered = %v, want http.ErrAbortHandler to propagate", recovered)
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
		"Content-Security-Policy": "default-src 'none'; frame-ancestors 'none'",
		"Referrer-Policy":         "no-referrer",
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
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
