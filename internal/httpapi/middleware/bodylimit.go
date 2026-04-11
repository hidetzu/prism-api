package middleware

import (
	"net/http"

	"github.com/hidetzu/prism-api/internal/httpapi/response"
)

// BodyLimit returns middleware that rejects requests whose body exceeds
// maxBytes. Enforcement has two layers:
//
//   - Early rejection via r.ContentLength. When the transport has already
//     parsed a Content-Length that exceeds maxBytes, the request is
//     rejected with 413 before the handler runs. This is the cheap path
//     and protects against abusive clients that advertise large bodies.
//   - Read-time enforcement via http.MaxBytesReader. r.Body is wrapped so
//     that reads beyond maxBytes return an *http.MaxBytesError. This
//     catches clients that lie about Content-Length or use chunked
//     transfer encoding. Handlers that read the body are responsible for
//     surfacing that error as 413 in their own error branches (per
//     docs/development_rules.md §6: validation runs inside handlers).
//
// maxBytes <= 0 disables both layers, which is useful for tests and for
// local development where the operator explicitly wants no limit.
func BodyLimit(maxBytes int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if maxBytes <= 0 {
				next.ServeHTTP(w, r)
				return
			}
			if r.ContentLength > maxBytes {
				response.WriteError(
					w,
					RequestIDFrom(r.Context()),
					response.CodePayloadTooLarge,
					"request body exceeds the configured limit",
				)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}
