package app

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/hidetzu/prism-api/internal/config"
)

// newTestConfig returns a Config suitable for lifecycle tests. Port "0"
// lets the OS assign an ephemeral port so tests do not collide.
func newTestConfig(port string) *config.Config {
	return &config.Config{
		Port:            port,
		RequestTimeout:  5 * time.Second,
		ShutdownTimeout: 5 * time.Second,
	}
}

func newSilentLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func TestRun_ShutsDownOnContextCancel(t *testing.T) {
	a := New(newTestConfig("0"), newSilentLogger())

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- a.run(ctx)
	}()

	// Give ListenAndServe time to bind before we trigger shutdown.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("run() after cancel = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("run() did not return within 2s after context cancel")
	}
}

func TestRun_ReturnsErrorOnListenFailure(t *testing.T) {
	// An invalid port string makes net.Listen fail synchronously inside
	// ListenAndServe, so run should surface the error without waiting
	// for a shutdown signal.
	a := New(newTestConfig("not-a-port"), newSilentLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := a.run(ctx); err == nil {
		t.Fatal("run() should return error when ListenAndServe fails")
	}
}
