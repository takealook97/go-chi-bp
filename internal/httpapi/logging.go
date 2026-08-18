package httpapi

import (
	"context"
	"log/slog"

	"github.com/go-chi/chi/v5/middleware"
)

// requestIDLogHandler adds the current request's ID to every record logged with
// its context. Handlers already log through the request context, but slog
// handlers ignore context by default, so a failure logged deep in a request
// could not be tied to the request that caused it.
type requestIDLogHandler struct {
	slog.Handler
}

// NewLogHandler wraps handler so that records logged with a request context
// carry that request's ID.
func NewLogHandler(handler slog.Handler) slog.Handler {
	if handler == nil {
		panic("log handler must not be nil")
	}

	return requestIDLogHandler{Handler: handler}
}

func (handler requestIDLogHandler) Handle(ctx context.Context, record slog.Record) error {
	requestID := middleware.GetReqID(ctx)
	if requestID == "" {
		return handler.Handler.Handle(ctx, record)
	}

	// Clone before adding: the caller still owns the record, and slog reuses a
	// record's attribute storage across handlers in a chain.
	record = record.Clone()
	record.AddAttrs(slog.String("requestID", requestID))

	return handler.Handler.Handle(ctx, record)
}

// WithAttrs and WithGroup must rewrap. Returning the inner handler would keep
// the annotated logger working while silently dropping request IDs from it.

func (handler requestIDLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return requestIDLogHandler{Handler: handler.Handler.WithAttrs(attrs)}
}

func (handler requestIDLogHandler) WithGroup(name string) slog.Handler {
	return requestIDLogHandler{Handler: handler.Handler.WithGroup(name)}
}
