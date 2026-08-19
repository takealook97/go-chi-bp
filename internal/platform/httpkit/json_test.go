package httpkit

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteContextError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		err         error
		wantHandled bool
		wantStatus  int
		wantBody    string
	}{
		{
			name:        "deadline exceeded",
			err:         fmt.Errorf("get widget: %w", context.DeadlineExceeded),
			wantHandled: true,
			wantStatus:  http.StatusGatewayTimeout,
			wantBody:    `"code":"request_timeout"`,
		},
		{
			name:        "canceled",
			err:         fmt.Errorf("get widget: %w", context.Canceled),
			wantHandled: true,
			wantStatus:  http.StatusOK,
		},
		{
			name: "unrelated failure",
			err:  errors.New("repository failure"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			response := httptest.NewRecorder()

			handled := WriteContextError(response, test.err)

			if handled != test.wantHandled {
				t.Fatalf("handled = %v, want %v", handled, test.wantHandled)
			}
			if test.wantStatus != 0 && response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
			if test.wantBody == "" {
				if response.Body.Len() != 0 {
					t.Fatalf("body = %q, want empty", response.Body.String())
				}

				return
			}
			if !strings.Contains(response.Body.String(), test.wantBody) {
				t.Fatalf("body = %q, want to contain %q", response.Body.String(), test.wantBody)
			}
		})
	}
}

func TestDecodeJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{name: "unknown field", body: `{"name":"example","extra":true}`},
		{name: "multiple values", body: `{"name":"first"} {"name":"second"}`},
		{name: "empty body", body: ``},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			var destination struct {
				Name string `json:"name"`
			}

			if err := NewJSONDecoder(1<<20).Decode(response, request, &destination); err == nil {
				t.Fatal("Decode() error = nil, want decoding error")
			}
		})
	}
}

func TestDecodeJSONAcceptsOneStrictValue(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", strings.NewReader(`{"name":"example"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	var destination struct {
		Name string `json:"name"`
	}

	if err := NewJSONDecoder(1<<20).Decode(response, request, &destination); err != nil {
		t.Fatalf("Decode() unexpected error: %v", err)
	}
	if destination.Name != "example" {
		t.Fatalf("name = %q, want example", destination.Name)
	}
}

func TestJSONDecoderRejectsUnsupportedMediaType(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", strings.NewReader(`{"name":"example"}`))
	request.Header.Set("Content-Type", "text/plain")
	response := httptest.NewRecorder()

	err := NewJSONDecoder(1<<20).Decode(response, request, &struct{}{})
	if !errors.Is(err, ErrUnsupportedMediaType) {
		t.Fatalf("Decode() error = %v, want ErrUnsupportedMediaType", err)
	}
}

func TestJSONDecoderRejectsOversizedBody(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", strings.NewReader(`{"name":"example"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	err := NewJSONDecoder(4).Decode(response, request, &struct{}{})
	var maxBytesError *http.MaxBytesError
	if !errors.As(err, &maxBytesError) {
		t.Fatalf("Decode() error = %v, want MaxBytesError", err)
	}
}

func TestJSONDecoderReturnsStableFieldViolations(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", strings.NewReader(`{"name":""}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	var destination struct {
		Name string `json:"name" validate:"required"`
	}

	err := NewJSONDecoder(1<<20).Decode(response, request, &destination)
	var validationError *ValidationError
	if !errors.As(err, &validationError) {
		t.Fatalf("Decode() error = %v, want ValidationError", err)
	}
	if len(validationError.Fields) != 1 ||
		validationError.Fields[0] != (FieldViolation{Field: "name", Rule: "required"}) {
		t.Fatalf("violations = %+v, want name required", validationError.Fields)
	}
}

func TestWriteJSONNoContentOmitsContentType(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()

	if err := WriteJSON(response, http.StatusNoContent, nil); err != nil {
		t.Fatalf("WriteJSON() unexpected error: %v", err)
	}

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
	if got := response.Header().Get("Content-Type"); got != "" {
		t.Fatalf("Content-Type = %q, want empty", got)
	}
	if response.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", response.Body.String())
	}
}

func TestWriteJSONReturnsSafeErrorResponseWhenEncodingFails(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()

	err := WriteJSON(response, http.StatusCreated, map[string]any{
		"unsupported": make(chan int),
	})

	if err == nil {
		t.Fatal("WriteJSON() error = nil, want an error")
	}
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if response.Body.String() != internalErrorJSON {
		t.Fatalf("body = %q, want %q", response.Body.String(), internalErrorJSON)
	}
}

func TestWriteErrorUsesStableEnvelope(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()

	WriteError(response, http.StatusBadRequest, "invalid_request", "The request is invalid.")

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want JSON", got)
	}
	if !strings.Contains(response.Body.String(), `"code":"invalid_request"`) {
		t.Fatalf("body = %q, want stable error envelope", response.Body.String())
	}
}
