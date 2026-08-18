package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"

	"github.com/lukuku-dev/go-chi-bp/internal/platform/httpkit"
	"github.com/lukuku-dev/go-chi-bp/internal/widget"
	"github.com/lukuku-dev/go-chi-bp/internal/widget/widgethttp"
)

// errContractDependency stands in for a failing readiness dependency.
var errContractDependency = errors.New("dependency is unavailable")

// TestResponsesSatisfyTheOpenAPISchemas serves real requests through the router
// and validates each response against the contract.
//
// The route-level contract test cannot see this class of defect: it compares
// method and path sets, so a handler may answer a documented route with a body
// the schema forbids. A limit of zero once produced exactly that, returning
// "limit": 0 against a schema declaring a minimum of one.
func TestResponsesSatisfyTheOpenAPISchemas(t *testing.T) {
	t.Parallel()

	timestamp := time.Date(2026, time.July, 10, 10, 0, 0, 0, time.UTC)
	stored := widget.Widget{ID: 1, Name: "first widget", CreatedAt: timestamp, UpdatedAt: timestamp}

	tests := []struct {
		name       string
		method     string
		target     string
		body       string
		mediaType  string
		repository *contractRepository
		ready      ReadinessCheck
		wantStatus int
	}{
		{
			name:       "liveness",
			method:     http.MethodGet,
			target:     "/health/live",
			wantStatus: http.StatusOK,
		},
		{
			name:       "readiness",
			method:     http.MethodGet,
			target:     "/health/ready",
			wantStatus: http.StatusOK,
		},
		{
			name:       "readiness with a failing dependency",
			method:     http.MethodGet,
			target:     "/health/ready",
			ready:      func(context.Context) error { return errContractDependency },
			wantStatus: http.StatusServiceUnavailable,
		},
		{
			name:       "empty widget page",
			method:     http.MethodGet,
			target:     "/v1/widgets",
			wantStatus: http.StatusOK,
		},
		{
			name:   "widget page with a continuation cursor",
			method: http.MethodGet,
			target: "/v1/widgets?limit=1",
			repository: &contractRepository{list: []widget.Widget{
				stored,
				{ID: 2, Name: "second widget", CreatedAt: timestamp, UpdatedAt: timestamp},
			}},
			wantStatus: http.StatusOK,
		},
		{
			name:       "widget page below the documented minimum",
			method:     http.MethodGet,
			target:     "/v1/widgets?limit=0",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "widget page with an unreadable cursor",
			method:     http.MethodGet,
			target:     "/v1/widgets?cursor=not-a-cursor",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "created widget",
			method:     http.MethodPost,
			target:     "/v1/widgets",
			body:       `{"name":"first widget"}`,
			mediaType:  "application/json",
			repository: &contractRepository{create: stored},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "created widget without a name",
			method:     http.MethodPost,
			target:     "/v1/widgets",
			body:       `{}`,
			mediaType:  "application/json",
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name:       "created widget with a malformed body",
			method:     http.MethodPost,
			target:     "/v1/widgets",
			body:       `{`,
			mediaType:  "application/json",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "created widget with the wrong media type",
			method:     http.MethodPost,
			target:     "/v1/widgets",
			body:       `{"name":"first widget"}`,
			mediaType:  "text/plain",
			wantStatus: http.StatusUnsupportedMediaType,
		},
		{
			name:       "widget that does not exist",
			method:     http.MethodGet,
			target:     "/v1/widgets/999",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "found widget",
			method:     http.MethodGet,
			target:     "/v1/widgets/1",
			repository: &contractRepository{get: stored},
			wantStatus: http.StatusOK,
		},
		{
			name:       "deleted widget",
			method:     http.MethodDelete,
			target:     "/v1/widgets/1",
			repository: &contractRepository{},
			wantStatus: http.StatusNoContent,
		},
	}

	validator := newContractValidator(t)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			repository := test.repository
			if repository == nil {
				repository = &contractRepository{getErr: widget.ErrNotFound, deleteErr: widget.ErrNotFound}
			}
			ready := test.ready
			if ready == nil {
				ready = func(context.Context) error { return nil }
			}

			router := contractRouter(repository, ready)

			response := httptest.NewRecorder()
			router.ServeHTTP(response, contractRequest(test.method, test.target, test.body, test.mediaType))

			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body = %q", response.Code, test.wantStatus, response.Body.String())
			}
			validator.validate(t, test.method, test.target, response)
		})
	}
}

// contractValidator checks responses against api/openapi.yaml.
type contractValidator struct {
	router routers.Router
}

func newContractValidator(t *testing.T) *contractValidator {
	t.Helper()

	router, err := gorillamux.NewRouter(loadContract(t))
	if err != nil {
		t.Fatalf("build contract router: %v", err)
	}

	return &contractValidator{router: router}
}

// validate fails the test when the response violates the documented schema,
// including its status, media type, required headers, and body.
func (validator *contractValidator) validate(t *testing.T, method, target string, response *httptest.ResponseRecorder) {
	t.Helper()

	request := contractRequest(method, target, "", "")
	route, pathParams, err := validator.router.FindRoute(request)
	if err != nil {
		t.Fatalf("find %s %s in the contract: %v", method, target, err)
	}

	input := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{
			Request:    request,
			PathParams: pathParams,
			Route:      route,
		},
		Status:  response.Code,
		Header:  response.Header(),
		Body:    http.NoBody,
		Options: &openapi3filter.Options{IncludeResponseStatus: true},
	}
	if body := response.Body.Bytes(); len(body) > 0 {
		input.SetBodyBytes(body)
	}

	if err := openapi3filter.ValidateResponse(t.Context(), input); err != nil {
		t.Fatalf("%s %s responded %d in violation of the contract: %v\nbody = %s",
			method, target, response.Code, err, response.Body.String())
	}
}

func contractRouter(repository widget.Repository, ready ReadinessCheck) http.Handler {
	logger := contractLogger()
	service := widget.NewService(repository)

	return NewRouter(
		logger,
		[]ReadinessCheck{ready},
		[]RouteMount{{
			Pattern: "/v1/widgets",
			Handler: widgethttp.NewHandler(service, logger, httpkit.NewJSONDecoder(1<<20)).Router(),
		}},
		Options{},
	)
}

func contractRequest(method, target, body, mediaType string) *http.Request {
	request := httptest.NewRequestWithContext(
		context.Background(), method, "http://localhost:8080"+target, strings.NewReader(body))
	if mediaType != "" {
		request.Header.Set("Content-Type", mediaType)
	}

	return request
}

// contractRepository serves fixed widgets so each documented response is
// produced by the real handler rather than assembled by the test.
type contractRepository struct {
	create    widget.Widget
	createErr error
	get       widget.Widget
	getErr    error
	list      []widget.Widget
	listErr   error
	deleteErr error
}

func (repository *contractRepository) Create(context.Context, string) (widget.Widget, error) {
	return repository.create, repository.createErr
}

func (repository *contractRepository) Get(context.Context, int64) (widget.Widget, error) {
	return repository.get, repository.getErr
}

func (repository *contractRepository) List(context.Context, widget.ListOptions) ([]widget.Widget, error) {
	if repository.list == nil {
		return []widget.Widget{}, repository.listErr
	}

	return repository.list, repository.listErr
}

func (repository *contractRepository) Delete(context.Context, int64) error {
	return repository.deleteErr
}

func contractLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}
