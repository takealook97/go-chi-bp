package httpkit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
			response := httptest.NewRecorder()
			var destination struct {
				Name string `json:"name"`
			}

			if err := DecodeJSON(response, request, &destination); err == nil {
				t.Fatal("DecodeJSON() error = nil, want decoding error")
			}
		})
	}
}

func TestDecodeJSONAcceptsOneStrictValue(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", strings.NewReader(`{"name":"example"}`))
	response := httptest.NewRecorder()
	var destination struct {
		Name string `json:"name"`
	}

	if err := DecodeJSON(response, request, &destination); err != nil {
		t.Fatalf("DecodeJSON() unexpected error: %v", err)
	}
	if destination.Name != "example" {
		t.Fatalf("name = %q, want example", destination.Name)
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
