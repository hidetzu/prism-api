package middleware

import (
	"net/http"

	"golang.org/x/sync/semaphore"

	"github.com/hidetzu/prism-api/internal/httpapi/response"
)

// ConcurrencyLimit returns middleware that caps the number of in-flight
// requests processed by the server to maxInFlight. New requests arriving
// once the cap is reached are rejected immediately with 503 and
// response.CodeServiceUnavailable; they do not queue.
//
// The immediate-reject semantic is deliberate: under load we would rather
// shed traffic fast than let clients hog connections waiting for a slot.
// Per Phase 2 instruction §14 no Retry-After header is emitted.
//
// maxInFlight <= 0 disables the limiter entirely (pass-through), matching
// the convention used by body_limit and rate_limit.
func ConcurrencyLimit(maxInFlight int) Middleware {
	if maxInFlight <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	sem := semaphore.NewWeighted(int64(maxInFlight))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !sem.TryAcquire(1) {
				response.WriteError(
					w,
					RequestIDFrom(r.Context()),
					response.CodeServiceUnavailable,
					"server is at capacity, please retry shortly",
				)
				return
			}
			defer sem.Release(1)
			next.ServeHTTP(w, r)
		})
	}
}
