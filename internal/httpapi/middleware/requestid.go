package middleware

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"net/http"
)

// RequestIDHeader is the canonical HTTP header name for the request ID.
const RequestIDHeader = "X-Request-Id"

type contextKey struct{ name string }

var requestIDKey = &contextKey{"request_id"}

// RequestID returns middleware that assigns a unique ID to every request,
// stores it in the request context, and sets it on the X-Request-Id header.
//
// The middleware is intentionally panic-free: the ID generation only uses
// crypto/rand plus base32 encoding, and context / header mutation cannot
// panic on a real ResponseWriter.
func RequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := newRequestID()
			w.Header().Set(RequestIDHeader, id)
			ctx := context.WithValue(r.Context(), requestIDKey, id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequestIDFrom returns the request ID stored in ctx by the RequestID
// middleware. It returns an empty string if no ID is present.
func RequestIDFrom(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

var requestIDEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// newRequestID generates a 26-character random identifier.
// 16 random bytes encoded as unpadded base32 yields 26 ASCII characters.
func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Extremely unlikely in practice; return a fixed sentinel so the
		// request still has a (non-empty) identifier.
		return "00000000000000000000000000"
	}
	return requestIDEncoding.EncodeToString(b[:])
}
