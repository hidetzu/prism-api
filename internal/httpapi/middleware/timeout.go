package middleware

import (
	"context"
	"net/http"
	"time"
)

// Timeout returns middleware that bounds the request context with d. The
// actual enforcement is the responsibility of downstream handlers that honor
// the request context.
func Timeout(d time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
