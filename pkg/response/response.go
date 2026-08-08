// Package response standardises JSON HTTP responses across the API so clients
// always receive a predictable envelope.
package response

import (
	"encoding/json"
	"errors"
	"net/http"
)

// Envelope is the consistent shape returned by every endpoint.
type Envelope struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *ErrorBody  `json:"error,omitempty"`
	Meta    interface{} `json:"meta,omitempty"`
}

// ErrorBody describes a failure in a machine and human readable way.
type ErrorBody struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

// JSON writes an arbitrary payload with the given status code.
func JSON(w http.ResponseWriter, status int, payload Envelope) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// OK writes a 200 success envelope.
func OK(w http.ResponseWriter, data interface{}) {
	JSON(w, http.StatusOK, Envelope{Success: true, Data: data})
}

// Created writes a 201 success envelope.
func Created(w http.ResponseWriter, data interface{}) {
	JSON(w, http.StatusCreated, Envelope{Success: true, Data: data})
}

// Paginated writes a 200 envelope with pagination metadata.
func Paginated(w http.ResponseWriter, data interface{}, meta interface{}) {
	JSON(w, http.StatusOK, Envelope{Success: true, Data: data, Meta: meta})
}

// NoContent writes a 204 response.
func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// Error inspects err, maps it to an HTTP status, and writes an error envelope.
func Error(w http.ResponseWriter, err error) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		JSON(w, appErr.Status, Envelope{
			Success: false,
			Error: &ErrorBody{
				Code:    appErr.Code,
				Message: appErr.Message,
				Fields:  appErr.Fields,
			},
		})
		return
	}

	JSON(w, http.StatusInternalServerError, Envelope{
		Success: false,
		Error:   &ErrorBody{Code: "internal_error", Message: "an unexpected error occurred"},
	})
}
