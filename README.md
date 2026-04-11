# prism-api

HTTP API that wraps [prism](https://github.com/hidetzu/prism) — a tool that
decomposes Pull Requests into structured, AI-review-ready context.

> **Status:** Phase 1 (v0.1.0-dev). Currently provides liveness, readiness,
> and version endpoints only. The `/v1/analyze` and `/v1/prompt` endpoints
> are planned for the next iteration.

## Endpoints

| Method | Path       | Description            |
|--------|------------|------------------------|
| `GET`  | `/healthz` | Liveness probe         |
| `GET`  | `/readyz`  | Readiness probe        |
| `GET`  | `/version` | API and Go version     |

## Quick Start

```bash
make build
make run
```

In another terminal:

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

All configuration is supplied via environment variables. The table below
lists the variables wired in Phase 1; see
[`docs/development_rules.md`](docs/development_rules.md) §12 for the full
Phase 1 configuration surface.

| Variable             | Default  | Purpose                              |
|----------------------|----------|--------------------------------------|
| `PORT`               | `8080`   | Listen port                          |
| `LOG_LEVEL`          | `info`   | slog level (debug/info/warn/error)   |
| `REQUEST_TIMEOUT`    | `30s`    | Per-request processing timeout       |
| `SHUTDOWN_TIMEOUT`   | `25s`    | Graceful shutdown wait               |
| `ALLOWED_PROVIDERS`  | `github` | Comma-separated provider allowlist   |

## Provider Support

prism-api currently supports **GitHub** only. For AWS CodeCommit, use the
prism CLI directly or self-host prism-api in your own AWS environment.

## Deployment (Fly.io)

`fly.toml` is pre-configured for a single `shared-cpu-1x` / 256MB machine in
`nrt` (Tokyo) with `auto_stop_machines` enabled so idle instances stop
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
