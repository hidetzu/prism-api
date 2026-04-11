package middleware

import (
	"net"
	"net/http"
	"strings"
)

// clientIP extracts the client IP address from the request. It prefers the
// first entry in X-Forwarded-For when present (behind a trusted proxy such
// as Fly.io's edge) and falls back to RemoteAddr otherwise. The helper is
// package-internal because logging and rate limiting both need the same
// extraction logic and neither wants to drift from the other.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.IndexByte(xff, ','); idx >= 0 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
