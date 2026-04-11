// Package app wires the HTTP server and manages process lifecycle including
// graceful shutdown on SIGINT and SIGTERM.
package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/hidetzu/prism-api/internal/config"
	"github.com/hidetzu/prism-api/internal/httpapi/handler"
	"github.com/hidetzu/prism-api/internal/httpapi/middleware"
	"github.com/hidetzu/prism-api/internal/usecase"
)

// App holds runtime dependencies and the configured HTTP server.
type App struct {
	cfg    *config.Config
	logger *slog.Logger
	server *http.Server
}

// New wires the HTTP server: routes, middleware, timeouts.
func New(cfg *config.Config, logger *slog.Logger) *App {
	mux := http.NewServeMux()

	health := handler.NewHealthHandler()
	mux.HandleFunc("GET /healthz", health.Live)
	mux.HandleFunc("GET /readyz", health.Ready)
	mux.HandleFunc("GET /version", health.Version)

	analyzeHandler := handler.NewAnalyzeHandler(usecase.NewAnalyzer())
	mux.HandleFunc("POST /v1/analyze", analyzeHandler.Handle)

	chain := middleware.Chain(
		mux,
		middleware.RequestID(),
		middleware.Recover(logger),
		middleware.Logger(logger),
		middleware.BodyLimit(cfg.MaxRequestBytes),
		middleware.RateLimit(cfg.RateLimitRPM, cfg.RateLimitBurst),
		middleware.ConcurrencyLimit(cfg.MaxConcurrentRequests),
		middleware.Timeout(cfg.RequestTimeout),
	)

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           chain,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return &App{
		cfg:    cfg,
		logger: logger,
		server: server,
	}
}

// Run starts the HTTP server and blocks until SIGINT or SIGTERM is received,
// then performs graceful shutdown bounded by cfg.ShutdownTimeout.
func (a *App) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return a.run(ctx)
}

// run launches ListenAndServe in a goroutine and blocks until either the
// server exits or ctx is canceled, then performs graceful shutdown bounded
// by cfg.ShutdownTimeout. It is unexported so that tests can drive the
// lifecycle with a controllable context.
func (a *App) run(ctx context.Context) error {
	srvErrCh := make(chan error, 1)
	go func() {
		a.logger.Info("server starting", "addr", a.server.Addr)
		err := a.server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			srvErrCh <- err
			return
		}
		srvErrCh <- nil
	}()

	select {
	case err := <-srvErrCh:
		return err
	case <-ctx.Done():
		a.logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
	defer cancel()

	if err := a.server.Shutdown(shutdownCtx); err != nil {
		return err
	}
	a.logger.Info("server stopped")
	return nil
}
