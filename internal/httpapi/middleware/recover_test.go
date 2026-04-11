package middleware

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRecover_CatchesPanicAndReturns500(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	h := Chain(
		http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			panic("boom")
		}),
		RequestID(),
		Recover(logger),
	)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
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
	if body.Error.Code != "internal_error" {
		t.Errorf("error.code = %q, want internal_error", body.Error.Code)
	}
	if body.Error.RequestID == "" {
		t.Error("error.request_id must be populated by the recover middleware")
	}
}

func TestRecover_PassesThroughOnSuccess(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	h := Recover(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
}
