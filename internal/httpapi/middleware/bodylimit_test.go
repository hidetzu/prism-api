package middleware

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hidetzu/prism-api/internal/httpapi/response"
)

func TestBodyLimit_BelowLimitPassesThrough(t *testing.T) {
	payload := "ok"
	var got string
	h := BodyLimit(100)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		got = string(b)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if got != payload {
		t.Errorf("handler read %q, want %q", got, payload)
	}
}

func TestBodyLimit_AtLimitPasses(t *testing.T) {
	payload := strings.Repeat("a", 10)
	h := BodyLimit(10)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read at-limit body: %v", err)
		}
		if len(b) != 10 {
			t.Errorf("body length = %d, want 10", len(b))
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestBodyLimit_ContentLengthExceedsReturns413(t *testing.T) {
	var handlerCalled bool
	h := BodyLimit(10)(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		handlerCalled = true
	}))

	payload := strings.Repeat("a", 100)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if handlerCalled {
		t.Error("handler should not have been called when body exceeds limit")
	}
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", rec.Code)
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
	if body.Error.Code != string(response.CodePayloadTooLarge) {
		t.Errorf("error.code = %q, want %q", body.Error.Code, response.CodePayloadTooLarge)
	}
	if body.Error.Message == "" {
		t.Error("error.message must be non-empty")
	}
}

func TestBodyLimit_UnknownLengthEnforcedAtReadTime(t *testing.T) {
	// Simulate chunked transfer by zeroing ContentLength so the early
	// Content-Length branch is bypassed. The handler's read must fail
	// once the actual body crosses the limit.
	payload := strings.Repeat("a", 100)
	var readErr error
	h := BodyLimit(10)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, readErr = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload))
	req.ContentLength = -1 // unknown length, typical for chunked
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if readErr == nil {
		t.Fatal("handler read should have failed once beyond limit")
	}
	var mbe *http.MaxBytesError
	if !errors.As(readErr, &mbe) {
		t.Errorf("read err = %T (%v), want *http.MaxBytesError", readErr, readErr)
	}
}

func TestBodyLimit_DisabledWhenZero(t *testing.T) {
	var called bool
	h := BodyLimit(0)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))

	payload := strings.Repeat("a", 10000)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !called {
		t.Error("handler should be called when limit is 0")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestBodyLimit_NegativeLimitDisabled(t *testing.T) {
	var called bool
	h := BodyLimit(-1)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("anything"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !called {
		t.Error("handler should run when limit is negative")
	}
}
