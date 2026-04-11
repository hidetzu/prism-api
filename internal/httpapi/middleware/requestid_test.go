package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestID_SetsHeaderAndContext(t *testing.T) {
	var contextID string
	h := RequestID()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contextID = RequestIDFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	headerID := rec.Header().Get(RequestIDHeader)
	if headerID == "" {
		t.Fatal("X-Request-Id header was not set")
	}
	if contextID == "" {
		t.Fatal("request id was not stored in context")
	}
	if headerID != contextID {
		t.Errorf("header id %q != context id %q", headerID, contextID)
	}
	if len(headerID) != 26 {
		t.Errorf("request id length = %d, want 26", len(headerID))
	}
}

func TestRequestID_GeneratesUniqueIDs(t *testing.T) {
	seen := make(map[string]bool, 200)
	h := RequestID()(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))

	for i := 0; i < 200; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		id := rec.Header().Get(RequestIDHeader)
		if seen[id] {
			t.Fatalf("duplicate request id detected: %s", id)
		}
		seen[id] = true
	}
}

func TestRequestIDFrom_EmptyContext(t *testing.T) {
	if id := RequestIDFrom(context.Background()); id != "" {
		t.Errorf("RequestIDFrom(empty) = %q, want empty", id)
	}
}
