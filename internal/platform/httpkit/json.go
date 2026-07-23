// Package httpkit contains framework-neutral HTTP transport helpers.
package httpkit

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

const maxRequestBodyBytes = 1 << 20

// ErrorResponse is the stable public error envelope.
type ErrorResponse struct {
	Error APIError `json:"error"`
}

// APIError describes a client-safe API error.
type APIError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// DecodeJSON decodes exactly one JSON object using strict field matching.
func DecodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode JSON body: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON value")
	}

	return nil
}

// WriteJSON writes a JSON response with the supplied HTTP status.
func WriteJSON(w http.ResponseWriter, status int, payload any) {
	if status == http.StatusNoContent || payload == nil {
		w.WriteHeader(status)

		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	_ = json.NewEncoder(w).Encode(payload)
}

// WriteError writes the stable public error envelope.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, ErrorResponse{
		Error: APIError{
			Code:    code,
			Message: message,
		},
	})
}
