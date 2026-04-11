package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/hidetzu/prism-api/internal/httpapi/response"
)

// Recover returns middleware that catches panics from downstream handlers,
// logs them with the request_id from context, and returns a 500 internal
// error response.
//
// Must sit inside the RequestID middleware so that the recovered log entry
// and error response include the request_id.
func Recover(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				requestID := RequestIDFrom(r.Context())
				logger.ErrorContext(r.Context(), "panic recovered",
					"request_id", requestID,
					"panic", rec,
					"stack", string(debug.Stack()),
				)
				response.WriteError(w, requestID, response.CodeInternalError, "internal server error")
			}()
			next.ServeHTTP(w, r)
		})
	}
}
