package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hidetzu/prism-api/internal/httpapi/response"
)

// okHandler is a no-op handler used as the inner handler in rate limit
// tests. It always writes 200.
func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

// newReq builds a request with the given remote addr. Tests use this to
// simulate requests from different clients.
func newReq(remoteAddr string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remoteAddr
	return req
}

// newReqXFF builds a request with a given X-Forwarded-For header plus a
// shared RemoteAddr to simulate traffic arriving through an edge proxy.
func newReqXFF(xff string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", xff)
	req.RemoteAddr = "10.0.0.1:1234"
	return req
}

func TestRateLimit_AllowsWithinBurst(t *testing.T) {
	h := RateLimit(60, 3)(okHandler())
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, newReq("1.2.3.4:1234"))
		if rec.Code != http.StatusOK {
			t.Errorf("request %d: status = %d, want 200", i, rec.Code)
		}
	}
}

func TestRateLimit_RejectsBeyondBurstReturns429(t *testing.T) {
	h := RateLimit(60, 2)(okHandler())

	// Exhaust the burst.
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, newReq("1.2.3.4:1234"))
		if rec.Code != http.StatusOK {
			t.Fatalf("initial request %d status = %d, want 200", i, rec.Code)
		}
	}

	// Third request from the same IP is rejected.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newReq("1.2.3.4:1234"))
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("third request status = %d, want 429", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}

	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != string(response.CodeRateLimited) {
		t.Errorf("error.code = %q, want %q", body.Error.Code, response.CodeRateLimited)
	}
	if body.Error.Message == "" {
		t.Error("error.message must be non-empty")
	}
}

func TestRateLimit_PerIPIsIndependent(t *testing.T) {
	h := RateLimit(60, 1)(okHandler())

	// IP A exhausts its single-token burst.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newReq("1.1.1.1:1234"))
	if rec.Code != http.StatusOK {
		t.Fatalf("A initial status = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, newReq("1.1.1.1:1234"))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("A second status = %d, want 429", rec.Code)
	}

	// IP B still has its full burst — should be unaffected by A.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, newReq("2.2.2.2:1234"))
	if rec.Code != http.StatusOK {
		t.Errorf("B status = %d, want 200 (independent limiter)", rec.Code)
	}
}

func TestRateLimit_KeysByXForwardedForNotRemoteAddr(t *testing.T) {
	// Simulates Fly.io's edge proxy: multiple clients share a single
	// RemoteAddr (the edge IP) but are distinguished by X-Forwarded-For.
	h := RateLimit(60, 1)(okHandler())

	// Client A (via XFF 10.0.0.10) exhausts its burst.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newReqXFF("10.0.0.10"))
	if rec.Code != http.StatusOK {
		t.Fatalf("A initial status = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, newReqXFF("10.0.0.10"))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("A second status = %d, want 429", rec.Code)
	}

	// Client B (via XFF 10.0.0.20, same RemoteAddr) must still pass.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, newReqXFF("10.0.0.20"))
	if rec.Code != http.StatusOK {
		t.Errorf("B status = %d, want 200 (keyed by XFF, not RemoteAddr)", rec.Code)
	}
}

func TestRateLimit_RecoversOverTime(t *testing.T) {
	// 600 rpm = 10 rps = 1 token / 100 ms. Burst 1.
	// After 150 ms sleep, >= 1 token has regenerated.
	h := RateLimit(600, 1)(okHandler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newReq("1.2.3.4:1234"))
	if rec.Code != http.StatusOK {
		t.Fatalf("initial status = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, newReq("1.2.3.4:1234"))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("rapid-follow status = %d, want 429", rec.Code)
	}

	time.Sleep(150 * time.Millisecond)

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, newReq("1.2.3.4:1234"))
	if rec.Code != http.StatusOK {
		t.Errorf("after recovery status = %d, want 200", rec.Code)
	}
}

func TestRateLimit_DisabledWhenRPMZero(t *testing.T) {
	called := 0
	h := RateLimit(0, 1)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called++
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 5; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, newReq("1.2.3.4:1234"))
		if rec.Code != http.StatusOK {
			t.Errorf("request %d status = %d, want 200", i, rec.Code)
		}
	}
	if called != 5 {
		t.Errorf("handler called %d times, want 5", called)
	}
}

func TestRateLimit_NegativeRPMDisabled(t *testing.T) {
	called := false
	h := RateLimit(-1, 1)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newReq("1.2.3.4:1234"))
	if !called {
		t.Error("handler should run when rpm is negative")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}
