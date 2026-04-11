// Package response provides unified JSON writers for success and error
// responses, along with the canonical error code to HTTP status mapping.
package response

import (
	"encoding/json"
	"net/http"
)

const contentTypeJSON = "application/json; charset=utf-8"

// Code is a machine-readable error code returned in error bodies.
type Code string

// Error codes returned in the body of error responses. Each code maps to a
// canonical HTTP status via (Code).HTTPStatus.
const (
	CodeInvalidInput       Code = "invalid_input"
	CodeAuthRequired       Code = "auth_required"
	CodePayloadTooLarge    Code = "payload_too_large"
	CodeRateLimited        Code = "rate_limited"
	CodeServiceUnavailable Code = "service_unavailable"
	CodeTimeout            Code = "timeout"
	CodeUpstreamFailure    Code = "upstream_failure"
	CodeInternalError      Code = "internal_error"
)

// HTTPStatus returns the HTTP status code corresponding to c.
func (c Code) HTTPStatus() int {
	switch c {
	case CodeInvalidInput:
		return http.StatusBadRequest
	case CodeAuthRequired:
		return http.StatusUnauthorized
	case CodePayloadTooLarge:
		return http.StatusRequestEntityTooLarge
	case CodeRateLimited:
		return http.StatusTooManyRequests
	case CodeServiceUnavailable:
		return http.StatusServiceUnavailable
	case CodeTimeout:
		return http.StatusGatewayTimeout
	case CodeUpstreamFailure:
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}

type errorBody struct {
	Error errorPayload `json:"error"`
}

type errorPayload struct {
	Code      Code   `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

// WriteJSON writes the given body as a JSON response with the given status.
// It returns any encoding error so callers can log it; the response has
// already been partially written in that case.
func WriteJSON(w http.ResponseWriter, status int, body any) error {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(body)
}

// WriteError writes a standardized JSON error response. The HTTP status is
// derived from the code via (Code).HTTPStatus.
func WriteError(w http.ResponseWriter, requestID string, code Code, message string) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(code.HTTPStatus())
	// If encoding fails (e.g. client disconnected), the response has already
	// been committed via WriteHeader; nothing sensible to do here.
	_ = json.NewEncoder(w).Encode(errorBody{
		Error: errorPayload{
			Code:      code,
			Message:   message,
			RequestID: requestID,
		},
	})
}
