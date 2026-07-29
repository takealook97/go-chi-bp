// Package httpkit contains framework-neutral HTTP transport helpers.
package httpkit

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

const (
	internalErrorJSON = "{\"error\":{\"code\":\"internal_error\",\"message\":\"An internal error occurred.\"}}\n"
)

// ErrUnsupportedMediaType indicates that a JSON endpoint received another media type.
var ErrUnsupportedMediaType = errors.New("request content type must be application/json")

// JSONDecoder strictly decodes and validates JSON request bodies.
type JSONDecoder struct {
	maxRequestBytes int64
	validator       *validator.Validate
}

// FieldViolation describes one client-safe request validation failure.
type FieldViolation struct {
	Field string `json:"field"`
	Rule  string `json:"rule"`
}

// ValidationError contains stable field-level validation failures.
type ValidationError struct {
	Fields []FieldViolation
}

// Error implements error without exposing request values.
func (validationError *ValidationError) Error() string {
	return "request validation failed"
}

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

// NewJSONDecoder constructs a strict request decoder with a body-size limit.
func NewJSONDecoder(maxRequestBytes int64) *JSONDecoder {
	if maxRequestBytes < 1 {
		panic("maximum request bytes must be at least 1")
	}

	validate := validator.New(validator.WithRequiredStructEnabled())
	validate.RegisterTagNameFunc(func(field reflect.StructField) string {
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			return field.Name
		}

		return name
	})

	return &JSONDecoder{maxRequestBytes: maxRequestBytes, validator: validate}
}

// Decode decodes exactly one JSON object and validates its transport constraints.
func (decoder *JSONDecoder) Decode(w http.ResponseWriter, r *http.Request, destination any) error {
	if !isJSONContentType(r.Header.Get("Content-Type")) {
		return ErrUnsupportedMediaType
	}

	r.Body = http.MaxBytesReader(w, r.Body, decoder.maxRequestBytes)

	jsonDecoder := json.NewDecoder(r.Body)
	jsonDecoder.DisallowUnknownFields()

	if err := jsonDecoder.Decode(destination); err != nil {
		return fmt.Errorf("decode JSON body: %w", err)
	}
	if err := jsonDecoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON value")
	}
	if err := decoder.validator.Struct(destination); err != nil {
		var validationErrors validator.ValidationErrors
		if !errors.As(err, &validationErrors) {
			return fmt.Errorf("validate JSON body: %w", err)
		}

		fields := make([]FieldViolation, 0, len(validationErrors))
		for _, fieldError := range validationErrors {
			fields = append(fields, FieldViolation{Field: fieldError.Field(), Rule: fieldError.Tag()})
		}

		return &ValidationError{Fields: fields}
	}

	return nil
}

func isJSONContentType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return false
	}

	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}

// WriteJSON writes a JSON response or a safe internal error when encoding fails.
func WriteJSON(w http.ResponseWriter, status int, payload any) error {
	if status == http.StatusNoContent || payload == nil {
		w.WriteHeader(status)

		return nil
	}

	body, err := json.Marshal(payload)
	if err != nil {
		writeErr := writeJSONBytes(w, http.StatusInternalServerError, []byte(internalErrorJSON))

		return errors.Join(fmt.Errorf("encode JSON response: %w", err), writeErr)
	}
	body = append(body, '\n')

	return writeJSONBytes(w, status, body)
}

func writeJSONBytes(w http.ResponseWriter, status int, body []byte) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("write JSON response: %w", err)
	}

	return nil
}

// WriteError writes the stable public error envelope.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteErrorDetails(w, status, code, message, nil)
}

// WriteErrorDetails writes the stable public error envelope with client-safe details.
func WriteErrorDetails(w http.ResponseWriter, status int, code, message string, details map[string]any) {
	_ = WriteJSON(w, status, ErrorResponse{
		Error: APIError{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}
