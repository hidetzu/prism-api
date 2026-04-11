# prism-api

HTTP API that wraps [`prism`](https://github.com/hidetzu/prism), a tool that
decomposes Pull Requests into structured, AI-review-ready context.

> **Status:** Phase 2 (v0.2.0). Provides health, analyze, and prompt
> endpoints for public GitHub Pull Requests.

## Who is this for?

- **Multi-language CI/CD pipelines** — call from Python, Node, or Ruby
  without shipping a Go binary to your CI image.
- **GitHub Actions and bot implementations** — automate PR analysis from
  workflows, Slack/Teams bots, or any service that already speaks HTTP.
- **Distribution layer for the `prism` CLI** — one hosted endpoint,
  rate-limited and bounded, serving multiple callers.

## Endpoints

| Method | Path            | Description                                    |
|--------|-----------------|------------------------------------------------|
| `GET`  | `/healthz`      | Liveness probe                                 |
| `GET`  | `/readyz`       | Readiness probe                                |
| `GET`  | `/version`      | API and Go runtime version                     |
| `POST` | `/v1/analyze`   | Decompose a GitHub PR into structured context  |
| `POST` | `/v1/prompt`    | Generate an AI review prompt from a GitHub PR  |

### `POST /v1/analyze`

Request:

```json
{
  "pull_request_url": "https://github.com/owner/repo/pull/123"
}
```

Response:

```json
{
  "result": {
    "pull_request":  { "provider": "github", "repository": "owner/repo", "id": "123", "title": "...", "author": "...", "url": "..." },
    "analysis":      { "change_type": "feature", "risk_level": "low", "review_axes": ["..."], "summary": "..." },
    "changed_files": [
      { "path": "a.go", "status": "modified", "additions": 10, "deletions": 2, "language": "go" }
    ]
  }
}
```

Example:

```bash
curl -sS -X POST http://localhost:8080/v1/analyze \
  -H 'Content-Type: application/json' \
  -d '{"pull_request_url":"https://github.com/owner/repo/pull/123"}'
```

### `POST /v1/prompt`

Request:

```json
{
  "pull_request_url": "https://github.com/owner/repo/pull/123",
  "mode":             "light",
  "language":         "en"
}
```

`mode` and `language` are optional. When omitted, `prism` applies its own
defaults (`light` mode, `en` language). Accepted modes: `light`, `detailed`,
`cross`. Accepted languages: `en`, `ja`.

Response:

```json
{
  "prompt": "You are a code reviewer performing a quick screening..."
}
```

Example (Japanese detailed mode):

```bash
curl -sS -X POST http://localhost:8080/v1/prompt \
  -H 'Content-Type: application/json' \
  -d '{"pull_request_url":"https://github.com/owner/repo/pull/123","mode":"detailed","language":"ja"}'
```

### Error format

Every error response follows this envelope:

```json
{
  "error": {
    "code":       "invalid_input",
    "message":    "pull_request_url: must be a github.com URL",
    "request_id": "01HXXXXXXXX"
  }
}
```

The `X-Request-Id` response header carries the same id. Log the header
(or the body field) when reporting issues so the server-side entry can be
correlated quickly.

| HTTP | Code                     | When it fires                                  |
|------|--------------------------|------------------------------------------------|
| 400  | `invalid_input`          | Request body is malformed or fails validation  |
| 400  | `unsupported_provider`   | Provider URL parses but is not allowed         |
| 401  | `auth_required`          | Upstream requires authentication               |
| 413  | `payload_too_large`      | Request body exceeds `MAX_REQUEST_BYTES`       |
| 429  | `rate_limited`           | Per-IP rate limit exceeded                     |
| 502  | `upstream_failure`       | Upstream provider (GitHub) returned an error   |
| 503  | `service_unavailable`    | Server-wide concurrency cap exceeded           |
| 504  | `timeout`                | Processing exceeded `REQUEST_TIMEOUT`          |
| 500  | `internal_error`         | Unknown failure                                |

## Quick Start

```bash
make build
make run
```

Test the health endpoints in another terminal:

```bash
curl -sS http://localhost:8080/healthz
curl -sS http://localhost:8080/readyz
curl -sS http://localhost:8080/version
```

## Make Targets

| Target       | Description                                    |
|--------------|------------------------------------------------|
| `make build` | Build the binary into `bin/prism-api`          |
| `make run`   | Build and run the binary locally               |
| `make test`  | Run unit tests with the race detector          |
| `make vet`   | Run `go vet`                                   |
| `make lint`  | Run `golangci-lint`                            |
| `make tidy`  | Run `go mod tidy`                              |
| `make clean` | Remove build artifacts                         |

## Configuration

All configuration is supplied via environment variables. Values are
validated at startup; invalid inputs cause `prism-api` to exit loudly
rather than silently start with broken state.

| Variable                  | Default  | Purpose                                           |
|---------------------------|----------|---------------------------------------------------|
| `PORT`                    | `8080`   | Listen port (0–65535)                             |
| `LOG_LEVEL`               | `info`   | slog level (`debug` / `info` / `warn` / `error`)  |
| `REQUEST_TIMEOUT`         | `30s`    | Per-request processing timeout                    |
| `SHUTDOWN_TIMEOUT`        | `25s`    | Graceful shutdown wait                            |
| `ALLOWED_PROVIDERS`       | `github` | Comma-separated provider allowlist                |
| `MAX_REQUEST_BYTES`       | `262144` | Request body size limit (256 KiB). `0` disables   |
| `RATE_LIMIT_RPM`          | `10`     | Per-IP rate limit (requests/minute). `0` disables |
| `RATE_LIMIT_BURST`        | `20`     | Per-IP burst capacity                             |
| `MAX_CONCURRENT_REQUESTS` | `50`     | Global in-flight request cap. `0` disables        |

See [`docs/development_rules.md`](docs/development_rules.md) §12 for the
full configuration surface including variables reserved for future phases.

## Known constraints (v0.2.0)

- **No authentication.** Any caller who can reach the deployed URL can
  submit any public GitHub PR for analysis. Do not deploy the public
  image for private-repository workflows.
- **Public GitHub PRs only.** Per-request GitHub token passthrough is on
  the roadmap but not yet wired.
- **GitHub unauthenticated rate limit.** The upstream GitHub API limits
  unauthenticated callers to **60 requests per hour per source IP**.
  Heavy CI/CD usage will hit this limit before `prism-api`'s own
  per-IP `RATE_LIMIT_RPM` fires.
- **`related_files` is heuristic.** The `analysis.related_files` list in
  `/v1/analyze` responses can include paths that are not present in the
  PR; they are heuristic review-time suggestions from `prism`'s
  classifier, not authoritative statements about the PR contents. Treat
  them as hints.
- **No caching.** Re-analyzing the same PR re-runs the full GitHub
  fetch. Caching is on the roadmap but not in Phase 2.
- **Rate limit state grows unbounded.** The per-IP limiter map is
  append-only for the lifetime of a single `prism-api` process. Fly.io
  machines auto-stop on idle so this rarely matters in practice, but
  long-running deployments should plan to restart periodically.

## Deployment (Fly.io)

`fly.toml` is pre-configured for a single `shared-cpu-1x` / 256MB machine
in `nrt` (Tokyo) with `auto_stop_machines` enabled so idle instances stop
billing.

```bash
fly launch --copy-config --no-deploy
fly deploy
```

## Development

See [`docs/development_rules.md`](docs/development_rules.md) for the
project's development rules (package layout, middleware order, error
handling, logging, test strategy, security requirements).

## License

See [`LICENSE`](LICENSE).
