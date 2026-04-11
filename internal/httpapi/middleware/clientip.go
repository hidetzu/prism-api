package middleware

import (
	"net"
	"net/http"
	"strings"
)

// clientIP extracts the client IP address from the request. It is used by
// both the logging and rate-limiting middleware and must return a
// consistent, non-spoofable value so the per-IP rate limiter cannot be
// bypassed by a malicious X-Forwarded-For header.
//
// Priority order:
//
//  1. Fly-Client-IP — set by Fly.io's edge proxy with the real client
//     address. The client cannot inject or override this header because
//     Fly strips any client-supplied value before setting its own. This
//     is the primary source when running on Fly.io.
//  2. X-Forwarded-For (first entry) — fallback for non-Fly environments
//     (local development, other reverse proxies that are trusted).
//  3. RemoteAddr — direct-connect fallback.
func clientIP(r *http.Request) string {
	// Prefer Fly-Client-IP so rate limiting is resistant to XFF spoofing.
	if fci := r.Header.Get("Fly-Client-IP"); fci != "" {
		return strings.TrimSpace(fci)
	}
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
