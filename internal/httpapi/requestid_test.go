package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
)

// recordRequestID serves one request through the middleware and reports the ID
// the response carried and the one the handler saw in its context.
func recordRequestID(t *testing.T, inbound string) (header string, contextValue string) {
	t.Helper()

	handler := requestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contextValue = middleware.GetReqID(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	if inbound != "" {
		request.Header.Set(requestIDHeader, inbound)
	}
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	return response.Header().Get(requestIDHeader), contextValue
}

func TestRequestIDIsReturnedToTheClient(t *testing.T) {
	t.Parallel()

	header, contextValue := recordRequestID(t, "")

	if header == "" {
		t.Fatal("response carried no request ID, so a client cannot quote one back")
	}
	if header != contextValue {
		t.Fatalf("response %s = %q, want the context value %q", requestIDHeader, header, contextValue)
	}
	if len(header) != generatedRequestIDBytes*2 {
		t.Fatalf("generated ID = %q, want %d hexadecimal characters", header, generatedRequestIDBytes*2)
	}
}

func TestRequestIDPropagatesAnAcceptableClientValue(t *testing.T) {
	t.Parallel()

	const inbound = "edge-7f3c_1.2"

	header, contextValue := recordRequestID(t, inbound)

	if header != inbound {
		t.Fatalf("response %s = %q, want %q", requestIDHeader, header, inbound)
	}
	if contextValue != inbound {
		t.Fatalf("context ID = %q, want %q", contextValue, inbound)
	}
}

// A client-supplied ID reaches every log record for the request, so one that
// carries a newline or a control byte would let a caller forge log lines and
// response headers. Each of these must be replaced rather than propagated.
func TestRequestIDReplacesAnUnacceptableClientValue(t *testing.T) {
	t.Parallel()

	for name, inbound := range map[string]string{
		"newline":      "abc\ndef",
		"carriage":     "abc\rdef",
		"null":         "abc\x00def",
		"space":        "abc def",
		"slash":        "abc/def",
		"non-ASCII":    "abcé",
		"over the cap": strings.Repeat("a", maximumRequestIDLength+1),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			header, contextValue := recordRequestID(t, inbound)

			if header == inbound {
				t.Fatalf("response %s = %q, want a replacement", requestIDHeader, header)
			}
			if header != contextValue {
				t.Fatalf("response %s = %q, want the context value %q", requestIDHeader, header, contextValue)
			}
			if len(header) != generatedRequestIDBytes*2 {
				t.Fatalf("replacement ID = %q, want %d hexadecimal characters", header, generatedRequestIDBytes*2)
			}
		})
	}
}

// The cap is on the value a client may propagate, not on one of the same length
// this server would have generated, so the boundary itself must be accepted.
func TestRequestIDAcceptsTheLongestPermittedClientValue(t *testing.T) {
	t.Parallel()

	inbound := strings.Repeat("a", maximumRequestIDLength)

	if header, _ := recordRequestID(t, inbound); header != inbound {
		t.Fatalf("response %s = %q, want %q", requestIDHeader, header, inbound)
	}
}

func TestRequestIDIsUniquePerRequest(t *testing.T) {
	t.Parallel()

	const requestCount = 128
	seen := make(map[string]struct{}, requestCount)
	for range requestCount {
		header, _ := recordRequestID(t, "")
		if _, duplicate := seen[header]; duplicate {
			t.Fatalf("generated ID %q was issued twice", header)
		}
		seen[header] = struct{}{}
	}
}

// A handler that writes immediately must still produce the header, because a
// header set after the first byte of the body never reaches the client.
func TestRequestIDIsSetBeforeTheHandlerWrites(t *testing.T) {
	t.Parallel()

	handler := requestID(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("body")); err != nil {
			t.Errorf("write response body: %v", err)
		}
	}))
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	// Result snapshots the headers as they stood when the handler wrote, which
	// is what a client would have received; Header would also show a header set
	// too late to reach one.
	result := response.Result()
	defer func() {
		if err := result.Body.Close(); err != nil {
			t.Errorf("close recorded response body: %v", err)
		}
	}()

	if result.Header.Get(requestIDHeader) == "" {
		t.Fatal("response written by the handler carried no request ID")
	}
}

// The router publishes this header to browsers through the CORS exposed header
// list. That list is a promise about every response, so it must name the header
// the middleware actually sets.
func TestRouterExposesTheRequestIDHeaderItSets(t *testing.T) {
	t.Parallel()

	router := NewRouter(
		slog.New(slog.DiscardHandler),
		[]ReadinessCheck{func(context.Context) error { return nil }},
		nil,
		Options{CORS: CORSOptions{AllowedOrigins: []string{"https://example.test"}}},
	)
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health/live", nil)
	request.Header.Set("Origin", "https://example.test")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if got := response.Header().Get(requestIDHeader); got == "" {
		t.Fatal("router response carried no request ID")
	}
	exposed := response.Header().Get("Access-Control-Expose-Headers")
	if !strings.Contains(strings.ToLower(exposed), strings.ToLower(requestIDHeader)) {
		t.Fatalf("Access-Control-Expose-Headers = %q, want it to name %s", exposed, requestIDHeader)
	}
}
