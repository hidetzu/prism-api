package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hidetzu/prism/pkg/prism"

	"github.com/hidetzu/prism-api/internal/config"
	"github.com/hidetzu/prism-api/internal/usecase"
)

// stubAnalyzeUsecase implements handler.AnalyzeUsecase by returning canned
// values. Tests wire it through newWithHandlers so chain integration can
// exercise the real middleware stack without reaching pkg/prism.
//
// When block is non-nil the stub parks on receive, simulating a long-running
// handler so concurrency_limit and timeout tests can orchestrate scenarios.
// When entered is non-nil the stub signals arrival before parking.
type stubAnalyzeUsecase struct {
	result  prism.Result
	err     error
	block   chan struct{}
	entered chan struct{}
}

func (s *stubAnalyzeUsecase) Analyze(ctx context.Context, _ usecase.AnalyzeInput) (prism.Result, error) {
	if s.entered != nil {
		s.entered <- struct{}{}
	}
	if s.block != nil {
		select {
		case <-s.block:
		case <-ctx.Done():
			return prism.Result{}, ctx.Err()
		}
	}
	return s.result, s.err
}

// stubPromptUsecase implements handler.PromptUsecase by returning canned
// values. Mirrors stubAnalyzeUsecase.
type stubPromptUsecase struct {
	prompt string
	err    error
}

func (s *stubPromptUsecase) Prompt(_ context.Context, _ usecase.PromptInput) (string, error) {
	return s.prompt, s.err
}

// silentLogger returns a logger whose output goes to io.Discard, for tests
// that do not care about log capture.
func silentLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

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
	// Non-Fly fallback: two clients arriving through a reverse proxy
	// (no Fly-Client-IP) must be keyed independently by X-Forwarded-For.
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

func TestChain_RateLimitResistantToXFFSpoofing(t *testing.T) {
	// SECURITY: On Fly.io the edge sets Fly-Client-IP with the real
	// client address. An attacker who rotates X-Forwarded-For must NOT
	// get a fresh rate-limit bucket per request if Fly-Client-IP is the
	// same. The rate limiter keys by Fly-Client-IP when present, making
	// XFF spoofing ineffective.
	cfg := chainConfig()
	cfg.RateLimitRPM = 60
	cfg.RateLimitBurst = 1
	a := New(cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))

	send := func(flyClientIP, xff string) int {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		req.Header.Set("Fly-Client-IP", flyClientIP)
		req.Header.Set("X-Forwarded-For", xff)
		req.RemoteAddr = "10.0.0.1:1234"
		return serveRequest(a, req).Code
	}

	// Same real IP (Fly-Client-IP), rotated spoofed XFF.
	if send("198.51.100.1", "fake-ip-1") != http.StatusOK {
		t.Fatalf("first request: want 200")
	}
	// If rate limiting used XFF, "fake-ip-2" would be a fresh bucket → 200.
	// Since Fly-Client-IP is preferred, the same real IP is rate-limited → 429.
	if code := send("198.51.100.1", "fake-ip-2"); code != http.StatusTooManyRequests {
		t.Errorf("second request with spoofed XFF: status = %d, want 429 (Fly-Client-IP must be used)", code)
	}
}

func TestChain_AnalyzeEndpointSuccess(t *testing.T) {
	stubA := &stubAnalyzeUsecase{
		result: prism.Result{
			PR: prism.PRInfo{
				Provider:   "github",
				Repository: "owner/repo",
				ID:         "1",
				Title:      "Example",
				URL:        "https://github.com/owner/repo/pull/1",
			},
			Analysis: prism.AnalysisResult{
				ChangeType: "feature",
				RiskLevel:  "low",
			},
		},
	}
	a := newWithHandlers(chainConfig(), silentLogger(), stubA, &stubPromptUsecase{})

	body := `{"pull_request_url":"https://github.com/owner/repo/pull/1"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/analyze", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := serveRequest(a, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Header().Get("X-Request-Id") == "" {
		t.Error("X-Request-Id must be set on success")
	}

	var got struct {
		Result struct {
			PullRequest struct {
				Repository string `json:"repository"`
				ID         string `json:"id"`
			} `json:"pull_request"`
			Analysis struct {
				ChangeType string `json:"change_type"`
			} `json:"analysis"`
		} `json:"result"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Result.PullRequest.Repository != "owner/repo" {
		t.Errorf("pull_request.repository = %q", got.Result.PullRequest.Repository)
	}
	if got.Result.Analysis.ChangeType != "feature" {
		t.Errorf("analysis.change_type = %q", got.Result.Analysis.ChangeType)
	}
}

func TestChain_PromptEndpointSuccess(t *testing.T) {
	stubP := &stubPromptUsecase{prompt: "Review this PR focusing on error handling."}
	a := newWithHandlers(chainConfig(), silentLogger(), &stubAnalyzeUsecase{}, stubP)

	body := `{"pull_request_url":"https://github.com/owner/repo/pull/1","mode":"detailed","language":"ja"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/prompt", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := serveRequest(a, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Header().Get("X-Request-Id") == "" {
		t.Error("X-Request-Id must be set on success")
	}

	var got struct {
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Prompt != "Review this PR focusing on error handling." {
		t.Errorf("prompt = %q", got.Prompt)
	}
}

func TestChain_ConcurrencyLimitRejectsBeyondCapacity(t *testing.T) {
	// Cap the server at one in-flight request so the second request is
	// rejected by concurrency_limit without relying on time.Sleep.
	cfg := chainConfig()
	cfg.MaxConcurrentRequests = 1

	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	stubA := &stubAnalyzeUsecase{
		block:   release,
		entered: entered,
	}
	a := newWithHandlers(cfg, silentLogger(), stubA, &stubPromptUsecase{})

	firstDone := make(chan int, 1)
	go func() {
		body := `{"pull_request_url":"https://github.com/owner/repo/pull/1"}`
		req := httptest.NewRequest(http.MethodPost, "/v1/analyze", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		firstDone <- serveRequest(a, req).Code
	}()

	// Wait until the first request is actually inside the stub — by this
	// point the concurrency_limit middleware has already acquired the only
	// slot on its behalf.
	<-entered

	body := `{"pull_request_url":"https://github.com/owner/repo/pull/2"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/analyze", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := serveRequest(a, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("second request status = %d, want 503", rec.Code)
	}
	var errBody struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&errBody); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if errBody.Error.Code != "service_unavailable" {
		t.Errorf("error.code = %q, want service_unavailable", errBody.Error.Code)
	}

	// Release the first request; it should complete normally.
	close(release)
	if code := <-firstDone; code != http.StatusOK {
		t.Errorf("first request status = %d, want 200", code)
	}
}

func TestChain_TimeoutMapsToGatewayTimeout(t *testing.T) {
	// Set a tight REQUEST_TIMEOUT and park the stub usecase on a channel
	// that is never closed. The Timeout middleware will cancel the
	// request context, the stub select picks up ctx.Done(), and the
	// handler must surface this as 504 via the shared usecase error
	// mapping — not as 500 internal_error.
	cfg := chainConfig()
	cfg.RequestTimeout = 50 * time.Millisecond

	never := make(chan struct{}) // intentionally never closed
	stubA := &stubAnalyzeUsecase{block: never}
	a := newWithHandlers(cfg, silentLogger(), stubA, &stubPromptUsecase{})

	body := `{"pull_request_url":"https://github.com/owner/repo/pull/1"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/analyze", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := serveRequest(a, req)

	if rec.Code != http.StatusGatewayTimeout {
		t.Errorf("status = %d, want 504", rec.Code)
	}
	if rec.Header().Get("X-Request-Id") == "" {
		t.Error("X-Request-Id must be set even on timeout")
	}

	var errBody struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&errBody); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if errBody.Error.Code != "timeout" {
		t.Errorf("error.code = %q, want timeout", errBody.Error.Code)
	}
	if errBody.Error.Message == "" {
		t.Error("error.message must be non-empty")
	}
}
