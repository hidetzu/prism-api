package app

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hidetzu/prism-api/internal/config"
)

// chainConfig returns a Config suitable for chain integration tests. Callers
// override specific fields to exercise particular defensive middleware.
func chainConfig() *config.Config {
	return &config.Config{
		Port:                  "0",
		LogLevel:              "info",
		RequestTimeout:        5 * time.Second,
		ShutdownTimeout:       5 * time.Second,
		MaxRequestBytes:       1 << 16, // 64 KiB
		RateLimitRPM:          10000,   // effectively disabled for positive-path tests
		RateLimitBurst:        10000,
		MaxConcurrentRequests: 1000,
		MaxChangedFiles:       50,
		MaxDiffBytes:          1 << 18,
		MaxResponseBytes:      1 << 19,
		AllowedProviders:      []string{"github"},
	}
}

// serveRequest drives a request through the configured app's middleware
// chain and returns the response recorder. It uses the already-constructed
// server.Handler so the full chain (as wired by New) is exercised.
func serveRequest(a *App, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	a.server.Handler.ServeHTTP(rec, req)
	return rec
}

func TestChain_HealthEndpointsPassThroughAllLayers(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))
	a := New(chainConfig(), logger)

	for _, path := range []string{"/healthz", "/readyz", "/version"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.RemoteAddr = "203.0.113.1:4242"
			rec := serveRequest(a, req)

			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want 200", rec.Code)
			}
			if rec.Header().Get("X-Request-Id") == "" {
				t.Error("X-Request-Id header must be set by the chain")
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
				t.Errorf("Content-Type = %q", ct)
			}
		})
	}

	// The Logger middleware should have emitted one entry per request with
	// the request_id field populated — confirms RequestID → Logger ordering.
	logs := logBuf.String()
	if !strings.Contains(logs, `"request_id"`) {
		t.Error("log output must contain request_id field from the chain")
	}
	if strings.Count(logs, `"msg":"request completed"`) < 3 {
		t.Errorf("expected >= 3 request completed log entries, got logs:\n%s", logs)
	}
}

func TestChain_BodyLimitRejectsOversizedWithRequestID(t *testing.T) {
	// A small MaxRequestBytes so a modest POST body triggers rejection.
	// This test exercises two things at once:
	//   1. body_limit fires before the handler.
	//   2. RequestID is upstream of body_limit — the error body includes
	//      request_id, proving the chain ordering is correct.
	cfg := chainConfig()
	cfg.MaxRequestBytes = 64
	a := New(cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))

	payload := strings.Repeat("a", 1024)
	req := httptest.NewRequest(http.MethodPost, "/healthz", strings.NewReader(payload))
	rec := serveRequest(a, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
	if rec.Header().Get("X-Request-Id") == "" {
		t.Error("X-Request-Id response header must still be set on 413")
	}

	var body struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != "payload_too_large" {
		t.Errorf("error.code = %q, want payload_too_large", body.Error.Code)
	}
	if body.Error.RequestID == "" {
		t.Error("error.request_id must be populated (proves RequestID is upstream of BodyLimit)")
	}
	if body.Error.RequestID != rec.Header().Get("X-Request-Id") {
		t.Errorf("error.request_id %q != X-Request-Id header %q",
			body.Error.RequestID, rec.Header().Get("X-Request-Id"))
	}
}

func TestChain_RateLimitRejectsExcessFromSameIP(t *testing.T) {
	// Low burst + same source IP so the third request trips the limiter.
	// Rate is high enough that recovery is instant but the burst bucket
	// is only 2 tokens at any moment.
	cfg := chainConfig()
	cfg.RateLimitRPM = 60 // 1 rps
	cfg.RateLimitBurst = 2
	a := New(cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))

	send := func() int {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		req.RemoteAddr = "198.51.100.7:1111"
		return serveRequest(a, req).Code
	}

	// First two consume the burst.
	if code := send(); code != http.StatusOK {
		t.Fatalf("request 1 status = %d, want 200", code)
	}
	if code := send(); code != http.StatusOK {
		t.Fatalf("request 2 status = %d, want 200", code)
	}
	// Third is rate limited.
	if code := send(); code != http.StatusTooManyRequests {
		t.Errorf("request 3 status = %d, want 429", code)
	}
}

func TestChain_RateLimitKeysByXForwardedFor(t *testing.T) {
	// Two clients arriving through the same Fly edge (shared RemoteAddr)
	// must be keyed independently by X-Forwarded-For. Without XFF keying,
	// the second client would inherit the first's exhausted bucket.
	cfg := chainConfig()
	cfg.RateLimitRPM = 60
	cfg.RateLimitBurst = 1
	a := New(cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))

	send := func(xff string) int {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		req.Header.Set("X-Forwarded-For", xff)
		req.RemoteAddr = "172.16.0.1:443"
		return serveRequest(a, req).Code
	}

	// Client A exhausts its single-token burst.
	if send("10.0.0.10") != http.StatusOK {
		t.Fatalf("A first: want 200")
	}
	if send("10.0.0.10") != http.StatusTooManyRequests {
		t.Fatalf("A second: want 429")
	}
	// Client B sharing the RemoteAddr but distinct via XFF must still pass.
	if code := send("10.0.0.20"); code != http.StatusOK {
		t.Errorf("B status = %d, want 200 (keyed by XFF)", code)
	}
}
