package response

import (
	"fmt"
	"net/http"
)

// AppError is a transport-agnostic application error carrying an HTTP status, a
// stable machine code, and an optional set of field level validation messages.
//
// Domain and service layers return these so handlers never need to know how to
// translate an error into an HTTP status.
type AppError struct {
	Status  int
	Code    string
	Message string
	Fields  map[string]string
	wrapped error
}

func (e *AppError) Error() string {
	if e.wrapped != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.wrapped)
	}
	return e.Message
}

func (e *AppError) Unwrap() error { return e.wrapped }

// Wrap attaches an underlying cause without exposing it to clients.
func (e *AppError) Wrap(err error) *AppError {
	clone := *e
	clone.wrapped = err
	return &clone
}

// WithFields attaches field level details (used for validation errors).
func (e *AppError) WithFields(fields map[string]string) *AppError {
	clone := *e
	clone.Fields = fields
	return &clone
}

// WithMessage overrides the human-readable message, e.g. to summarise field
// level validation errors into a single self-explanatory sentence.
func (e *AppError) WithMessage(message string) *AppError {
	clone := *e
	clone.Message = message
	return &clone
}

// Constructors for the common error categories.

func NewBadRequest(message string) *AppError {
	return &AppError{Status: http.StatusBadRequest, Code: "bad_request", Message: message}
}

func NewValidation(fields map[string]string) *AppError {
	return &AppError{Status: http.StatusUnprocessableEntity, Code: "validation_error", Message: "validation failed", Fields: fields}
}

func NewUnauthorized(message string) *AppError {
	return &AppError{Status: http.StatusUnauthorized, Code: "unauthorized", Message: message}
}

func NewForbidden(message string) *AppError {
	return &AppError{Status: http.StatusForbidden, Code: "forbidden", Message: message}
}

func NewNotFound(message string) *AppError {
	return &AppError{Status: http.StatusNotFound, Code: "not_found", Message: message}
}

func NewConflict(message string) *AppError {
	return &AppError{Status: http.StatusConflict, Code: "conflict", Message: message}
}

func NewInternal(message string) *AppError {
	return &AppError{Status: http.StatusInternalServerError, Code: "internal_error", Message: message}
}
