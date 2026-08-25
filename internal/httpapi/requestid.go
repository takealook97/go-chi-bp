package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
)

const (
	// requestIDHeader is the header a correlation ID is read from and returned
	// on. It is the name published to browsers as a CORS exposed header.
	requestIDHeader = "X-Request-ID"
	// maximumRequestIDLength bounds an ID supplied by a client. The value is
	// written to every log record the request produces, so an unbounded one
	// would let the caller decide how large this server's log lines are.
	maximumRequestIDLength = 64
	// generatedRequestIDBytes is the width of an ID this server generates.
	generatedRequestIDBytes = 16
)

// requestID resolves the request's correlation ID and returns it to the client.
//
// This replaces chi's RequestID middleware, which puts an ID in the context and
// stops there. Two gaps follow from that. The response carries no ID, so a
// client holding a failed request has nothing to quote and the CORS exposed
// header list advertises a header the server never sends. And an inbound
// X-Request-Id is trusted verbatim: it reaches every log record for the request,
// which makes its length and its bytes the caller's choice rather than this
// server's.
func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := acceptableRequestID(r.Header.Get(requestIDHeader))
		if id == "" {
			id = generateRequestID()
		}

		// Set before the handler runs. A header written after the first byte of
		// the body has already missed the response it belongs to.
		w.Header().Set(requestIDHeader, id)

		// chi's context key is kept so that middleware.GetReqID, and with it the
		// log handler wrapping every record, keeps working unchanged.
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), middleware.RequestIDKey, id)))
	})
}

// acceptableRequestID returns value when a client may propagate it, and the
// empty string when this server must supply its own instead. Only unreserved
// URL characters are accepted, which is what keeps an ID from carrying a
// newline into a log record or a control byte into a response header.
func acceptableRequestID(value string) string {
	if value == "" || len(value) > maximumRequestIDLength {
		return ""
	}

	for index := range len(value) {
		character := value[index]
		switch {
		case character >= 'a' && character <= 'z':
		case character >= 'A' && character <= 'Z':
		case character >= '0' && character <= '9':
		case character == '-', character == '_', character == '.':
		default:
			return ""
		}
	}

	return value
}

// generateRequestID returns an opaque ID for a request that arrived without an
// acceptable one.
func generateRequestID() string {
	buffer := make([]byte, generatedRequestIDBytes)
	// crypto/rand.Read fills the buffer or crashes the process, so the error it
	// returns to satisfy io.Reader cannot occur here.
	_, _ = rand.Read(buffer)

	return hex.EncodeToString(buffer)
}
