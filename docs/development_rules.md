# prism-api Development Rules

## 1. Purpose

This document defines the development rules for the `prism-api` project.

Goals:
- Maintain a consistent implementation style.
- Avoid large-scale refactoring later.
- Provide a shared judgment basis for both Claude Code and humans.
- Preserve quality on the assumption that the first code written becomes the project-wide template.

These rules apply to the Phase 1 (v0.1.0) scope.

---

## 2. Guiding Principles

- Start small.
- Prefer simplicity.
- Avoid increasing operational cost.
- Standard library first.
- Clearly separate responsibilities.
- Do not over-engineer.
- Optimize for ease of change.

---

## 3. Technical Stack

### Language
- Go 1.26.1 or later.

### HTTP
- Standard `net/http`.

### Logging
- Standard `log/slog`.

### Configuration
- Environment variables.

### Test
- Standard `testing`.

### JSON
- Standard `encoding/json`.

### Dependencies
- `github.com/hidetzu/prism@v0.3.0` (use `pkg/prism` pinned to a fixed version).
- `golang.org/x/sync/semaphore` (for concurrency limit).
- `golang.org/x/time/rate` (for rate limit).

### Principle
Do not add external dependencies until they are actually needed.

---

## 4. Package Layout

```text
cmd/prism-api/
internal/
  app/
  config/
  logging/
  httpapi/
    handler/
    middleware/
    response/
  usecase/
  validation/
```

### cmd/prism-api
- Entry point only.
- Wiring only.
- No business logic.

### internal/app
- Service startup.
- Dependency injection.
- Lifecycle management.
- Graceful shutdown.

### internal/httpapi/handler
- HTTP handlers.
- Request parsing.
- Validation invocation.
- Usecase invocation.
- Response writing.

### internal/httpapi/middleware
- request_id
- recover
- logging
- body_limit
- rate_limit
- concurrency_limit
- timeout

### internal/httpapi/response
- Unifies success/error JSON shape.
- Maps error codes to HTTP status.

### internal/config
- Reads environment variables.
- Holds default values.
- Loaded once at startup.

### internal/logging
- Initializes slog.

### internal/usecase
- Application-layer orchestration.
- Imports `pkg/prism` directly.
- Called by handlers via interfaces.

### internal/validation
- Shared input validation logic.
- Example: GitHub PR URL validation.

### pkg/
- Not created in Phase 1.
- prism-api is an application that exposes HTTP endpoints; it is not intended to be imported as a library by external code.

---

## 5. main.go Rules

Keep `main.go` thin.

```go
func main() {
    cfg := config.Load()
    logger := logging.New(cfg)
    app := app.New(cfg, logger)
    if err := app.Run(); err != nil {
        os.Exit(1)
    }
}
```

### Forbidden
- Handler implementation.
- Direct env reads.
- Middleware definition.
- Large blocks of conditional logic.

---

## 6. Middleware Order

The order is fixed as follows, from outermost to innermost:

1. request_id
2. recover
3. logging
4. body_limit
5. rate_limit
6. concurrency_limit
7. timeout
8. router / handler

Reasoning:
- request_id is at the outermost layer so it appears in every log entry and response.
- recover sits inside request_id so panics are logged with the request_id attached.
- logging observes every request.
- body_limit performs cheap early rejection (Content-Length check) for oversized payloads.
- rate_limit defends per IP.
- concurrency_limit protects the server's overall capacity, applied only to requests that passed rate_limit.
- timeout applies to the actual handler work.
- validation runs inside the handler (it is endpoint-specific and is not implemented as middleware).

---

## 7. Logging Rules

### Logger
- `slog.Logger`.

### Output Format
- JSON.

### Common Fields
- request_id
- method
- path
- status
- duration_ms
- remote_ip
- error

### Example

```json
{
  "level": "INFO",
  "msg": "request completed",
  "request_id": "01H...",
  "method": "POST",
  "path": "/v1/analyze",
  "status": 200,
  "duration_ms": 52
}
```

### Forbidden
- Do not log tokens.
- Do not log full request bodies.
- Do not always emit stack traces.

---

## 8. Request ID Rules

- Attached to every request.
- Header: `X-Request-Id`.
- Stored in the request context.
- Used in logs and responses.
- The middleware must remain panic-free (ULID generation, context value, header set only).

---

## 9. Response Rules

### Success

#### /v1/analyze
```json
{
  "result": {
    "pull_request": { },
    "analysis": { },
    "changed_files": [ ]
  }
}
```

The `result` field contains a JSON serialization of `pkg/prism.Result`.
It is not byte-identical to the prism CLI's JSON output; the API maintains its own contract.

#### /v1/prompt
```json
{
  "prompt": "..."
}
```

### Error

```json
{
  "error": {
    "code": "invalid_input",
    "message": "message",
    "request_id": "01H..."
  }
}
```

### Headers
- `Content-Type: application/json`
- `X-Request-Id` (set on every response)

---

## 10. Handler Rules

Keep handler responsibilities narrow.

### Do
- JSON decode.
- Validate (call the request type's `Validate() error`).
- Call usecase.
- Encode response.

### Do Not
- Complex branching.
- Provider implementation.
- Log aggregation.
- Scattered config checks.

### Guideline
- Keep each handler under 100 lines.

### Dependency on usecase
- Handlers receive a usecase via an **interface**.
- The interface is defined in the handler's package (Go's "accept interfaces, return structs" idiom).
- The implementation lives in `internal/usecase/`.

---

## 11. Error Handling

### Principles
- Never panic.
- Use sentinel errors or typed errors.
- Map to HTTP status at the HTTP layer.

### Error Code to HTTP Status Mapping

| error code | HTTP |
|---|---|
| `invalid_input` | 400 |
| `auth_required` | 401 |
| `payload_too_large` | 413 |
| `rate_limited` | 429 |
| `service_unavailable` | 503 |
| `timeout` | 504 |
| `upstream_failure` | 502 |
| `internal_error` | 500 |

---

## 12. Configuration Rules

All configuration is read from environment variables.

### Environment Variables

| Variable | Default | Purpose |
|---|---|---|
| `PORT` | `8080` | listen port |
| `LOG_LEVEL` | `info` | slog level |
| `MAX_REQUEST_BYTES` | `262144` (256KB) | maximum request body size |
| `REQUEST_TIMEOUT` | `30s` | per-request processing timeout |
| `RATE_LIMIT_RPM` | `10` | per-IP rate limit |
| `RATE_LIMIT_BURST` | `20` | per-IP burst |
| `MAX_CONCURRENT_REQUESTS` | `50` | server-wide concurrency cap |
| `MAX_CHANGED_FILES` | `50` | maximum number of changed files per response |
| `MAX_DIFF_BYTES` | `204800` (200KB) | maximum diff size |
| `MAX_RESPONSE_BYTES` | `512000` (500KB) | maximum response body size |
| `ALLOWED_PROVIDERS` | `github` | allowlist of providers |
| `SHUTDOWN_TIMEOUT` | `25s` | maximum graceful shutdown wait |

### Rules
- Every variable has a default.
- Loaded once at startup.
- Never re-read at runtime.

---

## 13. Test Rules

### Unit Test Targets
- middleware
- response writer
- validator
- config loader
- usecase

### HTTP Tests
- Test handlers with `httptest`.
- Substitute usecases with fakes to verify handler input/output.

### Naming
- `*_test.go`

### Principles
- Do not call external APIs.
- Prioritize reproducibility.

---

## 14. Comment Rules

### Principles
- Express intent through naming.
- Comments explain "why".
- Code expresses "what is happening".

### Avoid
```go
// increment i
i++
```

---

## 15. Security Rules

- Do not store tokens.
- Do not log tokens.
- Validate URLs strictly (host limited to `github.com`).
- timeout is mandatory.
- size limit is mandatory.
- recover is mandatory.
- No cache (Phase 1).
- Do not fetch arbitrary URLs (only via prism's GitHub provider).

---

## 16. Fly.io Assumptions

- Single binary.
- Graceful shutdown (must finish within `SHUTDOWN_TIMEOUT`, default 25s, on SIGINT/SIGTERM).
- Honor the `PORT` environment variable.
- Stateless.
- Liveness endpoint: `/healthz`.
- Readiness endpoint: `/readyz`.
- Enable `auto_stop_machines` to suppress idle billing.

---

## 17. Implementation Guidelines

1. Follow these rules with the highest priority.
2. Do not introduce personal stylistic preferences during skeleton generation.
3. Split changes into small PR units.
4. Each PR must address a single responsibility.
5. When in doubt, choose the simpler option.

---

## 18. Most Important

**The first code you write becomes the future template. Do not start carelessly.**
