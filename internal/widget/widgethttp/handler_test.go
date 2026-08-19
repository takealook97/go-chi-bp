package widgethttp

import (
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lukuku-dev/go-chi-bp/internal/platform/httpkit"
	"github.com/lukuku-dev/go-chi-bp/internal/widget"
)

type repositoryStub struct {
	create    widget.Widget
	createErr error
	get       widget.Widget
	getErr    error
	list      []widget.Widget
	listErr   error
	deleteErr error
}

func (stub *repositoryStub) Create(context.Context, string) (widget.Widget, error) {
	return stub.create, stub.createErr
}

func (stub *repositoryStub) Get(context.Context, int64) (widget.Widget, error) {
	return stub.get, stub.getErr
}

func (stub *repositoryStub) List(context.Context, widget.ListOptions) ([]widget.Widget, error) {
	return stub.list, stub.listErr
}

func (stub *repositoryStub) Delete(context.Context, int64) error {
	return stub.deleteErr
}

func TestHandlerCreate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		repository *repositoryStub
		wantStatus int
		wantBody   string
	}{
		{
			name: "created",
			body: `{"name":"example"}`,
			repository: &repositoryStub{create: widget.Widget{
				ID: 1, Name: "example", CreatedAt: time.Unix(1, 0).UTC(), UpdatedAt: time.Unix(1, 0).UTC(),
			}},
			wantStatus: http.StatusCreated,
			wantBody:   `"name":"example"`,
		},
		{
			name:       "unknown field",
			body:       `{"name":"example","extra":true}`,
			repository: &repositoryStub{},
			wantStatus: http.StatusBadRequest,
			wantBody:   `"code":"invalid_request"`,
		},
		{
			name:       "invalid name",
			body:       `{"name":"   "}`,
			repository: &repositoryStub{},
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   `"code":"invalid_widget_name"`,
		},
		{
			name:       "missing name",
			body:       `{"name":""}`,
			repository: &repositoryStub{},
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   `"fields":[{"field":"name","rule":"required"}]`,
		},
		{
			name:       "repository failure",
			body:       `{"name":"example"}`,
			repository: &repositoryStub{createErr: errors.New("repository failure")},
			wantStatus: http.StatusInternalServerError,
			wantBody:   `"code":"internal_error"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			response := serveWidgetRequest(t, test.repository, http.MethodPost, "/", test.body)

			assertResponse(t, response, test.wantStatus, test.wantBody)
		})
	}
}

func TestHandlerCreateRejectsNonJSONContentType(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.DiscardHandler)
	handler := NewHandler(widget.NewService(&repositoryStub{}), logger, httpkit.NewJSONDecoder(1<<20)).Router()
	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", strings.NewReader(`{"name":"example"}`))
	request.Header.Set("Content-Type", "text/plain")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	assertResponse(t, response, http.StatusUnsupportedMediaType, `"code":"unsupported_media_type"`)
}

func TestHandlerCreateRejectsOversizedBody(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.DiscardHandler)
	handler := NewHandler(widget.NewService(&repositoryStub{}), logger, httpkit.NewJSONDecoder(4)).Router()
	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", strings.NewReader(`{"name":"example"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	assertResponse(t, response, http.StatusRequestEntityTooLarge, `"code":"request_too_large"`)
}

func TestHandlerGet(t *testing.T) {
	t.Parallel()

	createdAt := time.Unix(1, 0).UTC()
	tests := []struct {
		name       string
		target     string
		repository *repositoryStub
		wantStatus int
		wantBody   string
	}{
		{
			name:       "found",
			target:     "/7",
			repository: &repositoryStub{get: widget.Widget{ID: 7, Name: "example", CreatedAt: createdAt, UpdatedAt: createdAt}},
			wantStatus: http.StatusOK,
			wantBody:   `"id":7`,
		},
		{
			name:       "invalid identifier",
			target:     "/invalid",
			repository: &repositoryStub{},
			wantStatus: http.StatusNotFound,
			wantBody:   `"code":"widget_not_found"`,
		},
		{
			name:       "not found",
			target:     "/7",
			repository: &repositoryStub{getErr: widget.ErrNotFound},
			wantStatus: http.StatusNotFound,
			wantBody:   `"code":"widget_not_found"`,
		},
		{
			name:       "repository failure",
			target:     "/7",
			repository: &repositoryStub{getErr: errors.New("repository failure")},
			wantStatus: http.StatusInternalServerError,
			wantBody:   `"code":"internal_error"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			response := serveWidgetRequest(t, test.repository, http.MethodGet, test.target, "")

			assertResponse(t, response, test.wantStatus, test.wantBody)
		})
	}
}

func TestHandlerList(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		target     string
		repository *repositoryStub
		wantStatus int
		wantBody   string
	}{
		{
			name:       "bounded page",
			target:     "/?limit=10",
			repository: &repositoryStub{list: []widget.Widget{}},
			wantStatus: http.StatusOK,
			wantBody:   `"items":[],"limit":10,"nextCursor":null`,
		},
		{
			name:   "continuation cursor",
			target: "/?limit=1",
			repository: &repositoryStub{list: []widget.Widget{
				{ID: 2, CreatedAt: time.Unix(2, 0).UTC()},
				{ID: 1, CreatedAt: time.Unix(1, 0).UTC()},
			}},
			wantStatus: http.StatusOK,
			wantBody:   `"nextCursor":"`,
		},
		{
			name:       "invalid integer",
			target:     "/?limit=many",
			repository: &repositoryStub{},
			wantStatus: http.StatusBadRequest,
			wantBody:   `"code":"invalid_pagination"`,
		},
		{
			name:       "out of bounds",
			target:     "/?limit=101",
			repository: &repositoryStub{},
			wantStatus: http.StatusBadRequest,
			wantBody:   `"code":"invalid_pagination"`,
		},
		{
			name:       "zero limit",
			target:     "/?limit=0",
			repository: &repositoryStub{},
			wantStatus: http.StatusBadRequest,
			wantBody:   `"code":"invalid_pagination"`,
		},
		{
			name:       "absent limit reports the applied default",
			target:     "/",
			repository: &repositoryStub{list: []widget.Widget{}},
			wantStatus: http.StatusOK,
			wantBody:   `"limit":20`,
		},
		{
			name:       "invalid cursor",
			target:     "/?cursor=not-a-valid-cursor",
			repository: &repositoryStub{},
			wantStatus: http.StatusBadRequest,
			wantBody:   `"code":"invalid_pagination"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			response := serveWidgetRequest(t, test.repository, http.MethodGet, test.target, "")

			assertResponse(t, response, test.wantStatus, test.wantBody)
		})
	}
}

func TestHandlerDelete(t *testing.T) {
	t.Parallel()

	t.Run("deleted", func(t *testing.T) {
		t.Parallel()

		response := serveWidgetRequest(t, &repositoryStub{}, http.MethodDelete, "/7", "")

		if response.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
		}
		if response.Body.Len() != 0 {
			t.Fatalf("body = %q, want empty", response.Body.String())
		}
		if got := response.Header().Get("Content-Type"); got != "" {
			t.Fatalf("Content-Type = %q, want empty", got)
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		response := serveWidgetRequest(t, &repositoryStub{deleteErr: widget.ErrNotFound}, http.MethodDelete, "/7", "")

		assertResponse(t, response, http.StatusNotFound, `"code":"widget_not_found"`)
	})

	t.Run("invalid identifier", func(t *testing.T) {
		t.Parallel()

		response := serveWidgetRequest(t, &repositoryStub{}, http.MethodDelete, "/invalid", "")

		assertResponse(t, response, http.StatusNotFound, `"code":"widget_not_found"`)
	})

	t.Run("repository failure", func(t *testing.T) {
		t.Parallel()

		response := serveWidgetRequest(t, &repositoryStub{deleteErr: errors.New("repository failure")}, http.MethodDelete, "/7", "")

		assertResponse(t, response, http.StatusInternalServerError, `"code":"internal_error"`)
	})
}

func TestHandlerReportsContextFailures(t *testing.T) {
	t.Parallel()

	t.Run("deadline exceeded", func(t *testing.T) {
		t.Parallel()

		response := serveWidgetRequest(
			t, &repositoryStub{getErr: context.DeadlineExceeded}, http.MethodGet, "/7", "")

		assertResponse(t, response, http.StatusGatewayTimeout, `"code":"request_timeout"`)
	})

	t.Run("canceled by the client", func(t *testing.T) {
		t.Parallel()

		response := serveWidgetRequest(
			t, &repositoryStub{getErr: context.Canceled}, http.MethodGet, "/7", "")

		// A canceled request has no connection left to answer, so the handler
		// writes nothing and the recorder keeps its unwritten defaults.
		if response.Body.Len() != 0 {
			t.Fatalf("body = %q, want empty", response.Body.String())
		}
		if got := response.Header().Get("Content-Type"); got != "" {
			t.Fatalf("Content-Type = %q, want empty", got)
		}
	})
}

func TestListCursorRoundTrip(t *testing.T) {
	t.Parallel()

	want := widget.ListCursor{CreatedAt: time.Unix(1, 123456000).UTC(), ID: 7}
	result, err := decodeListCursor(encodeListCursor(want))
	if err != nil {
		t.Fatalf("decodeListCursor() unexpected error: %v", err)
	}
	if *result != want {
		t.Fatalf("cursor = %+v, want %+v", result, want)
	}
}

func TestDecodeListCursorRejectsInvalidPayloads(t *testing.T) {
	t.Parallel()

	for _, value := range []string{
		base64.RawURLEncoding.EncodeToString([]byte("missing-separator")),
		base64.RawURLEncoding.EncodeToString([]byte("invalid|1")),
		base64.RawURLEncoding.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano) + "|invalid")),
		base64.RawURLEncoding.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano) + "|0")),
	} {
		if _, err := decodeListCursor(value); err == nil {
			t.Errorf("decodeListCursor(%q) error = nil, want error", value)
		}
	}
}

func serveWidgetRequest(t *testing.T, repository *repositoryStub, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()

	logger := slog.New(slog.DiscardHandler)
	handler := NewHandler(widget.NewService(repository), logger, httpkit.NewJSONDecoder(1<<20)).Router()
	request := httptest.NewRequestWithContext(context.Background(), method, target, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	return response
}

func assertResponse(t *testing.T, response *httptest.ResponseRecorder, wantStatus int, wantBody string) {
	t.Helper()

	if response.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body = %q", response.Code, wantStatus, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), wantBody) {
		t.Fatalf("body = %q, want to contain %q", response.Body.String(), wantBody)
	}
}
