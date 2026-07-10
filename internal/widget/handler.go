// Package widget demonstrates a vertical business module.
package widget

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
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
	Items  []widgetResponse `json:"items"`
	Limit  int32            `json:"limit"`
	Offset int32            `json:"offset"`
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

	httpkit.WriteJSON(w, http.StatusCreated, widgetToResponse(result))
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

	httpkit.WriteJSON(w, http.StatusOK, widgetToResponse(result))
}

func (handler *Handler) list(w http.ResponseWriter, r *http.Request) {
	limit, err := queryInt32(r, "limit", defaultListLimit)
	if err != nil {
		httpkit.WriteError(w, http.StatusBadRequest, "invalid_pagination", "Pagination parameters are invalid.")

		return
	}
	offset, err := queryInt32(r, "offset", 0)
	if err != nil {
		httpkit.WriteError(w, http.StatusBadRequest, "invalid_pagination", "Pagination parameters are invalid.")

		return
	}

	results, err := handler.service.List(r.Context(), ListOptions{Limit: limit, Offset: offset})
	if errors.Is(err, ErrInvalidPagination) {
		httpkit.WriteError(w, http.StatusBadRequest, "invalid_pagination", "Pagination parameters are invalid.")

		return
	}
	if err != nil {
		handler.internalError(w, r, "list widgets", err)

		return
	}

	items := make([]widgetResponse, 0, len(results))
	for _, result := range results {
		items = append(items, widgetToResponse(result))
	}

	httpkit.WriteJSON(w, http.StatusOK, widgetListResponse{Items: items, Limit: limit, Offset: offset})
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

	httpkit.WriteJSON(w, http.StatusNoContent, nil)
}

func (handler *Handler) internalError(w http.ResponseWriter, r *http.Request, operation string, err error) {
	handler.logger.ErrorContext(r.Context(), "HTTP request failed", "operation", operation, "error", err)
	httpkit.WriteError(w, http.StatusInternalServerError, "internal_error", "An internal error occurred.")
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

func widgetToResponse(value Widget) widgetResponse {
	return widgetResponse(value)
}
