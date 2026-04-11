package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteJSON_WritesStatusAndBody(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := WriteJSON(rec, http.StatusCreated, map[string]string{"status": "ok"}); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %q, want ok", body["status"])
	}
}

func TestWriteError_StatusBodyAndHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, "01HXXX", CodeInvalidInput, "bad input")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}

	var body errorBody
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != CodeInvalidInput {
		t.Errorf("error.code = %q, want invalid_input", body.Error.Code)
	}
	if body.Error.Message != "bad input" {
		t.Errorf("error.message = %q", body.Error.Message)
	}
	if body.Error.RequestID != "01HXXX" {
		t.Errorf("error.request_id = %q", body.Error.RequestID)
	}
}

func TestCode_HTTPStatus(t *testing.T) {
	cases := []struct {
		code Code
		want int
	}{
		{CodeInvalidInput, http.StatusBadRequest},
		{CodeAuthRequired, http.StatusUnauthorized},
		{CodePayloadTooLarge, http.StatusRequestEntityTooLarge},
		{CodeRateLimited, http.StatusTooManyRequests},
		{CodeServiceUnavailable, http.StatusServiceUnavailable},
		{CodeTimeout, http.StatusGatewayTimeout},
		{CodeUpstreamFailure, http.StatusBadGateway},
		{CodeInternalError, http.StatusInternalServerError},
		{Code("unknown"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		if got := c.code.HTTPStatus(); got != c.want {
			t.Errorf("Code(%q).HTTPStatus() = %d, want %d", c.code, got, c.want)
		}
	}
}
