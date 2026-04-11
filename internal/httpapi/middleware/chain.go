// Package middleware contains HTTP middleware used by prism-api.
//
// Middleware order (outermost to innermost) is defined in
// docs/development_rules.md §6. The canonical chain is:
//
//  1. request_id
//  2. recover
//  3. logging
//  4. body_limit          (added with /v1 endpoints)
//  5. rate_limit          (added with /v1 endpoints)
//  6. concurrency_limit   (added with /v1 endpoints)
//  7. timeout
//  8. router / handler
//
// Phase 1 skeleton wires request_id, recover, logging, and timeout. The
// remaining defense middleware will be added alongside the analyze/prompt
// handlers in a subsequent PR.
package middleware

import "net/http"

// Middleware wraps an http.Handler.
type Middleware func(http.Handler) http.Handler

// Chain applies the given middlewares to the handler. The first middleware in
// the slice becomes the outermost layer; the last becomes the innermost.
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}
