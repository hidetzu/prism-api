// Package app wires the HTTP server and manages process lifecycle including
// graceful shutdown on SIGINT and SIGTERM.
package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hidetzu/prism-api/internal/config"
	"github.com/hidetzu/prism-api/internal/httpapi/handler"
	"github.com/hidetzu/prism-api/internal/httpapi/middleware"
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

	chain := middleware.Chain(
		mux,
		middleware.RequestID(),
		middleware.Recover(logger),
		middleware.Logger(logger),
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

// Run starts the HTTP server and blocks until a shutdown signal is received,
// then performs graceful shutdown bounded by cfg.ShutdownTimeout.
func (a *App) Run() error {
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

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	select {
	case err := <-srvErrCh:
		return err
	case sig := <-sigCh:
		a.logger.Info("shutdown signal received", "signal", sig.String())
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
	defer cancel()

	if err := a.server.Shutdown(shutdownCtx); err != nil {
		return err
	}
	a.logger.Info("server stopped")
	return nil
}
