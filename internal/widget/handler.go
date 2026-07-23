// Package widget demonstrates a vertical business module.
package widget

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/lukuku-dev/go-chi-bp/internal/platform/httpkit"
)

// Handler exposes widget use cases over HTTP.
type Handler struct {
	service *Service
	logger  *slog.Logger
}

// NewHandler constructs a widget HTTP handler.
func NewHandler(service *Service, logger *slog.Logger) *Handler {
	if service == nil || logger == nil {
		panic("widget handler dependencies must not be nil")
	}

	return &Handler{service: service, logger: logger}
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

type createWidgetRequest struct {
	Name string `json:"name"`
}

type widgetResponse struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type widgetListResponse struct {
	Items      []widgetResponse `json:"items"`
	Limit      int32            `json:"limit"`
	NextCursor *string          `json:"nextCursor"`
}

func (handler *Handler) create(w http.ResponseWriter, r *http.Request) {
	var request createWidgetRequest
	if err := httpkit.DecodeJSON(w, r, &request); err != nil {
		httpkit.WriteError(w, http.StatusBadRequest, "invalid_request", "The request body is invalid.")

		return
	}

	result, err := handler.service.Create(r.Context(), request.Name)
	if errors.Is(err, ErrInvalidName) {
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
	if errors.Is(err, ErrNotFound) {
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
	limit, err := queryInt32(r, "limit", defaultListLimit)
	if err != nil {
		httpkit.WriteError(w, http.StatusBadRequest, "invalid_pagination", "Pagination parameters are invalid.")

		return
	}
	cursor, err := decodeListCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		httpkit.WriteError(w, http.StatusBadRequest, "invalid_pagination", "Pagination parameters are invalid.")

		return
	}

	page, err := handler.service.List(r.Context(), ListOptions{Limit: limit, Cursor: cursor})
	if errors.Is(err, ErrInvalidPagination) {
		httpkit.WriteError(w, http.StatusBadRequest, "invalid_pagination", "Pagination parameters are invalid.")

		return
	}
	if err != nil {
		handler.internalError(w, r, "list widgets", err)

		return
	}

	items := make([]widgetResponse, 0, len(page.Items))
	for _, result := range page.Items {
		items = append(items, widgetToResponse(result))
	}

	var nextCursor *string
	if page.NextCursor != nil {
		encoded := encodeListCursor(*page.NextCursor)
		nextCursor = &encoded
	}

	handler.writeJSON(w, r, http.StatusOK, widgetListResponse{Items: items, Limit: limit, NextCursor: nextCursor})
}

func (handler *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		httpkit.WriteError(w, http.StatusNotFound, "widget_not_found", "Widget was not found.")

		return
	}

	if err := handler.service.Delete(r.Context(), id); errors.Is(err, ErrNotFound) {
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

func queryInt32(r *http.Request, key string, fallback int32) (int32, error) {
	value := r.URL.Query().Get(key)
	if value == "" {
		return fallback, nil
	}

	parsed, err := strconv.ParseInt(value, 10, 32)

	return int32(parsed), err
}

func encodeListCursor(cursor ListCursor) string {
	payload := cursor.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + strconv.FormatInt(cursor.ID, 10)

	return base64.RawURLEncoding.EncodeToString([]byte(payload))
}

func decodeListCursor(value string) (*ListCursor, error) {
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

	return &ListCursor{CreatedAt: createdAt.UTC(), ID: id}, nil
}

func widgetToResponse(value Widget) widgetResponse {
	return widgetResponse(value)
}
