// Package widgethttp exposes widget use cases through HTTP.
package widgethttp

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/lukuku-dev/go-chi-bp/internal/httpapi/apigen"
	"github.com/lukuku-dev/go-chi-bp/internal/platform/httpkit"
	"github.com/lukuku-dev/go-chi-bp/internal/widget"
)

// minimumListLimit mirrors the minimum published for the limit query parameter.
const minimumListLimit = 1

// Service describes the widget use cases consumed by the HTTP adapter.
type Service interface {
	Create(ctx context.Context, name string) (widget.Widget, error)
	Get(ctx context.Context, id int64) (widget.Widget, error)
	List(ctx context.Context, options widget.ListOptions) (widget.Page, error)
	Delete(ctx context.Context, id int64) error
}

// Handler exposes widget use cases over HTTP.
type Handler struct {
	service Service
	logger  *slog.Logger
	decoder *httpkit.JSONDecoder
}

// NewHandler constructs a widget HTTP handler.
func NewHandler(service Service, logger *slog.Logger, decoder *httpkit.JSONDecoder) *Handler {
	if service == nil || logger == nil || decoder == nil {
		panic("widget handler dependencies must not be nil")
	}

	return &Handler{service: service, logger: logger, decoder: decoder}
}

// Router returns the widget HTTP routes.
func (handler *Handler) Router() chi.Router {
	router := chi.NewRouter()
	router.Get("/", handler.list)
	router.Post("/", handler.create)
	router.Get("/{widgetID}", handler.get)
	router.Delete("/{widgetID}", handler.delete)

	return router
}

func (handler *Handler) create(w http.ResponseWriter, r *http.Request) {
	var request apigen.CreateWidgetJSONRequestBody
	if err := handler.decoder.Decode(w, r, &request); err != nil {
		var (
			maxBytesError   *http.MaxBytesError
			validationError *httpkit.ValidationError
		)
		switch {
		case errors.Is(err, httpkit.ErrUnsupportedMediaType):
			httpkit.WriteError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "Content-Type must be application/json.")
		case errors.As(err, &maxBytesError):
			httpkit.WriteError(w, http.StatusRequestEntityTooLarge, "request_too_large", "The request body is too large.")
		case errors.As(err, &validationError):
			httpkit.WriteErrorDetails(
				w,
				http.StatusUnprocessableEntity,
				"validation_failed",
				"Request validation failed.",
				map[string]any{"fields": validationError.Fields},
			)
		default:
			httpkit.WriteError(w, http.StatusBadRequest, "invalid_request", "The request body is invalid.")
		}

		return
	}

	result, err := handler.service.Create(r.Context(), request.Name)
	if errors.Is(err, widget.ErrInvalidName) {
		httpkit.WriteError(w, http.StatusUnprocessableEntity, "invalid_widget_name", "Name must contain 1 to 120 characters.")

		return
	}
	if err != nil {
		handler.internalError(w, r, "create widget", err)

		return
	}

	handler.writeJSON(w, r, http.StatusCreated, widgetToResponse(result))
}

func (handler *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		httpkit.WriteError(w, http.StatusNotFound, "widget_not_found", "Widget was not found.")

		return
	}

	result, err := handler.service.Get(r.Context(), id)
	if errors.Is(err, widget.ErrNotFound) {
		httpkit.WriteError(w, http.StatusNotFound, "widget_not_found", "Widget was not found.")

		return
	}
	if err != nil {
		handler.internalError(w, r, "get widget", err)

		return
	}

	handler.writeJSON(w, r, http.StatusOK, widgetToResponse(result))
}

func (handler *Handler) list(w http.ResponseWriter, r *http.Request) {
	limit, err := listLimit(r)
	if err != nil {
		httpkit.WriteError(w, http.StatusBadRequest, "invalid_pagination", "Pagination parameters are invalid.")

		return
	}
	cursor, err := decodeListCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		httpkit.WriteError(w, http.StatusBadRequest, "invalid_pagination", "Pagination parameters are invalid.")

		return
	}

	page, err := handler.service.List(r.Context(), widget.ListOptions{Limit: limit, Cursor: cursor})
	if errors.Is(err, widget.ErrInvalidPagination) {
		httpkit.WriteError(w, http.StatusBadRequest, "invalid_pagination", "Pagination parameters are invalid.")

		return
	}
	if err != nil {
		handler.internalError(w, r, "list widgets", err)

		return
	}

	items := make([]apigen.Widget, 0, len(page.Items))
	for _, result := range page.Items {
		items = append(items, widgetToResponse(result))
	}

	var nextCursor *string
	if page.NextCursor != nil {
		encoded := encodeListCursor(*page.NextCursor)
		nextCursor = &encoded
	}

	handler.writeJSON(w, r, http.StatusOK, apigen.WidgetList{Items: items, Limit: page.Limit, NextCursor: nextCursor})
}

func (handler *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		httpkit.WriteError(w, http.StatusNotFound, "widget_not_found", "Widget was not found.")

		return
	}

	if err := handler.service.Delete(r.Context(), id); errors.Is(err, widget.ErrNotFound) {
		httpkit.WriteError(w, http.StatusNotFound, "widget_not_found", "Widget was not found.")

		return
	} else if err != nil {
		handler.internalError(w, r, "delete widget", err)

		return
	}

	handler.writeJSON(w, r, http.StatusNoContent, nil)
}

func (handler *Handler) internalError(w http.ResponseWriter, r *http.Request, operation string, err error) {
	handler.logger.ErrorContext(r.Context(), "HTTP request failed", "operation", operation, "error", err)
	httpkit.WriteError(w, http.StatusInternalServerError, "internal_error", "An internal error occurred.")
}

func (handler *Handler) writeJSON(w http.ResponseWriter, r *http.Request, status int, payload any) {
	if err := httpkit.WriteJSON(w, status, payload); err != nil {
		handler.logger.ErrorContext(r.Context(), "HTTP response failed", "status", status, "error", err)
	}
}

func pathID(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "widgetID"), 10, 64)
}

// listLimit reads the page size within the bounds published by the OpenAPI
// contract. An absent parameter selects the documented default; a present one
// must satisfy the contract's minimum.
func listLimit(r *http.Request) (int32, error) {
	value := r.URL.Query().Get("limit")
	if value == "" {
		return widget.DefaultListLimit, nil
	}

	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("parse limit: %w", err)
	}
	if parsed < minimumListLimit {
		return 0, errors.New("limit is below the documented minimum")
	}

	return int32(parsed), nil
}

func encodeListCursor(cursor widget.ListCursor) string {
	payload := cursor.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + strconv.FormatInt(cursor.ID, 10)

	return base64.RawURLEncoding.EncodeToString([]byte(payload))
}

func decodeListCursor(value string) (*widget.ListCursor, error) {
	if value == "" {
		return nil, nil
	}

	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("decode cursor: %w", err)
	}

	createdAtValue, idValue, found := strings.Cut(string(payload), "|")
	if !found {
		return nil, errors.New("cursor separator is missing")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, createdAtValue)
	if err != nil {
		return nil, fmt.Errorf("parse cursor timestamp: %w", err)
	}
	id, err := strconv.ParseInt(idValue, 10, 64)
	if err != nil || id < 1 {
		return nil, errors.New("cursor identifier is invalid")
	}

	return &widget.ListCursor{CreatedAt: createdAt.UTC(), ID: id}, nil
}

func widgetToResponse(value widget.Widget) apigen.Widget {
	return apigen.Widget{
		ID:        value.ID,
		Name:      value.Name,
		CreatedAt: value.CreatedAt,
		UpdatedAt: value.UpdatedAt,
	}
}
