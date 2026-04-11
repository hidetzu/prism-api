package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/hidetzu/prism-api/internal/httpapi/response"
)

func TestConcurrencyLimit_AllowsWithinCapacity(t *testing.T) {
	h := ConcurrencyLimit(2)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Serial requests within capacity.
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if rec.Code != http.StatusOK {
			t.Errorf("request %d: status = %d, want 200", i, rec.Code)
		}
	}
}

func TestConcurrencyLimit_RejectsBeyondCapacityReturns503(t *testing.T) {
	// Coordinate one in-flight request to hold the only slot while a
	// second request is issued from the test goroutine.
	release := make(chan struct{})
	inHandler := make(chan struct{}, 1)

	h := ConcurrencyLimit(1)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		inHandler <- struct{}{}
		<-release
		w.WriteHeader(http.StatusOK)
	}))

	// Fire the first request in the background; it will acquire the slot
	// and then block on <-release.
	firstDone := make(chan int, 1)
	go func() {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		firstDone <- rec.Code
	}()

	// Wait until the first request is actually inside the handler.
	<-inHandler

	// Second request is immediately rejected.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("rejected status = %d, want 503", rec.Code)
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
	if body.Error.Code != string(response.CodeServiceUnavailable) {
		t.Errorf("error.code = %q, want %q", body.Error.Code, response.CodeServiceUnavailable)
	}
	if body.Error.Message == "" {
		t.Error("error.message must be non-empty")
	}

	// Let the first request finish.
	close(release)
	if code := <-firstDone; code != http.StatusOK {
		t.Errorf("first status = %d, want 200", code)
	}
}

func TestConcurrencyLimit_RecoversAfterRelease(t *testing.T) {
	// Verify that the slot is actually returned after the handler finishes:
	// a request fired after the blocking one is released should succeed.
	release := make(chan struct{})
	var callCount int
	var mu sync.Mutex

	h := ConcurrencyLimit(1)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		callCount++
		first := callCount == 1
		mu.Unlock()
		if first {
			<-release
		}
		w.WriteHeader(http.StatusOK)
	}))

	// Fire the first request in background.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if rec.Code != http.StatusOK {
			t.Errorf("first status = %d, want 200", rec.Code)
		}
	}()

	close(release)
	wg.Wait()

	// After the goroutine exits, the middleware's defer has released the
	// slot. A follow-up request should therefore acquire it without issue.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("after release status = %d, want 200", rec.Code)
	}
}

func TestConcurrencyLimit_MultipleSlots(t *testing.T) {
	// With capacity 2, two concurrent requests should both pass while a
	// third is rejected.
	release := make(chan struct{})
	inHandler := make(chan struct{}, 2)

	h := ConcurrencyLimit(2)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		inHandler <- struct{}{}
		<-release
		w.WriteHeader(http.StatusOK)
	}))

	// Fire two requests in background.
	results := make(chan int, 2)
	for i := 0; i < 2; i++ {
		go func() {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
			results <- rec.Code
		}()
	}

	// Wait until both are inside the handler.
	<-inHandler
	<-inHandler

	// Third request is rejected.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("third status = %d, want 503", rec.Code)
	}

	// Release both.
	close(release)
	for i := 0; i < 2; i++ {
		if code := <-results; code != http.StatusOK {
			t.Errorf("background %d: status = %d, want 200", i, code)
		}
	}
}

func TestConcurrencyLimit_DisabledWhenMaxZero(t *testing.T) {
	// max=0 → no-op middleware; many concurrent requests all pass through.
	release := make(chan struct{})
	var started sync.WaitGroup
	started.Add(5)

	h := ConcurrencyLimit(0)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		started.Done()
		<-release
		w.WriteHeader(http.StatusOK)
	}))

	results := make(chan int, 5)
	for i := 0; i < 5; i++ {
		go func() {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
			results <- rec.Code
		}()
	}

	started.Wait() // all 5 must reach the handler
	close(release)

	for i := 0; i < 5; i++ {
		if code := <-results; code != http.StatusOK {
			t.Errorf("request %d status = %d, want 200", i, code)
		}
	}
}

func TestConcurrencyLimit_NegativeMaxDisabled(t *testing.T) {
	called := false
	h := ConcurrencyLimit(-1)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if !called {
		t.Error("handler should run when max is negative")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}
