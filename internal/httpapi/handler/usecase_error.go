package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/hidetzu/prism/pkg/prism"

	"github.com/hidetzu/prism-api/internal/httpapi/response"
)

// writeUsecaseError maps an error returned by a pkg/prism-backed usecase
// call to the canonical response.Code and writes the standard error
// envelope. Used by every handler that delegates to the usecase layer
// so the mapping stays consistent across endpoints.
//
// Mapping rules (first match wins):
//
//  1. context.DeadlineExceeded or context.Canceled → CodeTimeout (504).
//     The timeout middleware canceled the request context while the
//     upstream call was still in flight. Clients see a 504 so they can
//     distinguish "we gave up" from "upstream was unhealthy".
//  2. prism.ErrInvalidInput          → CodeInvalidInput         (400)
//  3. prism.ErrUnsupportedProvider   → CodeUnsupportedProvider  (400)
//  4. prism.ErrAuthRequired          → CodeAuthRequired         (401)
//  5. prism.ErrUpstreamFailure       → CodeUpstreamFailure      (502)
//  6. Anything else                  → CodeInternalError        (500)
//
// Client-facing messages are fixed strings. The raw error text is never
// exposed to the client so wrapped pkg/prism errors cannot leak
// implementation details.
func writeUsecaseError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		response.WriteError(w, requestID, response.CodeTimeout,
			"request exceeded the configured processing timeout")
	case errors.Is(err, prism.ErrInvalidInput):
		response.WriteError(w, requestID, response.CodeInvalidInput,
			"the pull request input could not be processed")
	case errors.Is(err, prism.ErrUnsupportedProvider):
		response.WriteError(w, requestID, response.CodeUnsupportedProvider,
			"the requested provider is not supported")
	case errors.Is(err, prism.ErrAuthRequired):
		response.WriteError(w, requestID, response.CodeAuthRequired,
			"authentication is required to access this repository")
	case errors.Is(err, prism.ErrUpstreamFailure):
		response.WriteError(w, requestID, response.CodeUpstreamFailure,
			"upstream service is temporarily unavailable")
	default:
		response.WriteError(w, requestID, response.CodeInternalError,
			"internal server error")
	}
}
