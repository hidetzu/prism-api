package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLogger_LogsRequestFields(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	h := Logger(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/example", nil)
	req.RemoteAddr = "203.0.113.7:54321"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var entry map[string]any
	if err := json.NewDecoder(&buf).Decode(&entry); err != nil {
		t.Fatalf("decode log entry: %v", err)
	}

	wantFields := []string{"method", "path", "status", "duration_ms", "remote_ip"}
	for _, f := range wantFields {
		if _, ok := entry[f]; !ok {
			t.Errorf("missing field %q in log entry: %v", f, entry)
		}
	}
	if entry["method"] != "POST" {
		t.Errorf("method = %v, want POST", entry["method"])
	}
	if entry["path"] != "/v1/example" {
		t.Errorf("path = %v, want /v1/example", entry["path"])
	}
	if entry["status"] != float64(201) {
		t.Errorf("status = %v, want 201", entry["status"])
	}
	if entry["remote_ip"] != "203.0.113.7" {
		t.Errorf("remote_ip = %v, want 203.0.113.7", entry["remote_ip"])
	}
}

func TestLogger_UsesXForwardedFor(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	h := Logger(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "198.51.100.5, 203.0.113.7")
	req.RemoteAddr = "10.0.0.1:9999"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var entry map[string]any
	if err := json.NewDecoder(&buf).Decode(&entry); err != nil {
		t.Fatalf("decode log entry: %v", err)
	}
	if entry["remote_ip"] != "198.51.100.5" {
		t.Errorf("remote_ip = %v, want 198.51.100.5", entry["remote_ip"])
	}
}
