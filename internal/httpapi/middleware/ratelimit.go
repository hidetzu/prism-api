package middleware

import (
	"net/http"
	"sync"

	"golang.org/x/time/rate"

	"github.com/hidetzu/prism-api/internal/httpapi/response"
)

// RateLimit returns middleware that enforces a per-IP request rate limit.
// Each unique client IP gets its own token-bucket limiter configured with
// rpm requests per minute and a burst capacity of burst tokens.
//
// The client IP is taken from X-Forwarded-For (first entry) when present,
// otherwise from RemoteAddr. See clientIP for details.
//
// rpm <= 0 disables the limiter entirely, which is useful for tests and for
// opt-in local development where the operator wants no limit.
//
// Memory note: the limiter map grows unbounded over the lifetime of the
// process. Phase 2 explicitly accepts this (see
// docs/development_rules.md §6 and the Phase 2 instruction §14); eviction
// will be added in a later phase if long-lived deployments see meaningful
// growth.
func RateLimit(rpm int, burst int) Middleware {
	if rpm <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	// rate.Limit is tokens per second; convert from requests per minute.
	perSecond := rate.Limit(float64(rpm) / 60.0)
	rl := &ipRateLimiter{
		rate:     perSecond,
		burst:    burst,
		limiters: make(map[string]*rate.Limiter),
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			if !rl.limiterFor(ip).Allow() {
				response.WriteError(
					w,
					RequestIDFrom(r.Context()),
					response.CodeRateLimited,
					"rate limit exceeded for this client",
				)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ipRateLimiter owns a per-IP map of token-bucket limiters. Access is
// serialized through mu because both reads and writes to the map can race.
type ipRateLimiter struct {
	rate     rate.Limit
	burst    int
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
}

// limiterFor returns the limiter for the given IP, creating one on first
// use. Subsequent calls for the same IP return the same limiter so that
// token state accumulates across requests.
func (rl *ipRateLimiter) limiterFor(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	lim, ok := rl.limiters[ip]
	if !ok {
		lim = rate.NewLimiter(rl.rate, rl.burst)
		rl.limiters[ip] = lim
	}
	return lim
}
