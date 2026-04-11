package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hidetzu/prism/pkg/prism"

	"github.com/hidetzu/prism-api/internal/usecase"
)

// fakePromptUsecase is a test double implementing the PromptUsecase
// interface.
type fakePromptUsecase struct {
	prompt   string
	err      error
	gotInput usecase.PromptInput
	calls    int
}

func (f *fakePromptUsecase) Prompt(_ context.Context, in usecase.PromptInput) (string, error) {
	f.calls++
	f.gotInput = in
	return f.prompt, f.err
}

func servePrompt(h *PromptHandler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/prompt", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.Handle(rec, req)
	return rec
}

func TestPromptHandler_Success_FullRequest(t *testing.T) {
	uc := &fakePromptUsecase{prompt: "Review this PR focusing on error handling."}
	h := NewPromptHandler(uc)

	rec := servePrompt(h, `{
		"pull_request_url":"https://github.com/owner/repo/pull/123",
		"mode":"detailed",
		"language":"ja"
	}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if uc.calls != 1 {
		t.Errorf("usecase called %d times, want 1", uc.calls)
	}
	if uc.gotInput.PullRequestURL != "https://github.com/owner/repo/pull/123" {
		t.Errorf("gotInput.PullRequestURL = %q", uc.gotInput.PullRequestURL)
	}
	if uc.gotInput.Mode != "detailed" {
		t.Errorf("gotInput.Mode = %q, want detailed", uc.gotInput.Mode)
	}
	if uc.gotInput.Language != "ja" {
		t.Errorf("gotInput.Language = %q, want ja", uc.gotInput.Language)
	}

	var body struct {
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Prompt != "Review this PR focusing on error handling." {
		t.Errorf("prompt = %q", body.Prompt)
	}
}

func TestPromptHandler_Success_DefaultsForwardedAsEmpty(t *testing.T) {
	// When Mode and Language are omitted, they must reach the usecase as
	// empty strings so pkg/prism applies its own defaults. The handler
	// must not substitute "light"/"en" in its own code.
	uc := &fakePromptUsecase{prompt: "p"}
	h := NewPromptHandler(uc)

	rec := servePrompt(h, `{"pull_request_url":"https://github.com/owner/repo/pull/1"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if uc.gotInput.Mode != "" {
		t.Errorf("gotInput.Mode = %q, want empty (pkg/prism handles defaults)", uc.gotInput.Mode)
	}
	if uc.gotInput.Language != "" {
		t.Errorf("gotInput.Language = %q, want empty (pkg/prism handles defaults)", uc.gotInput.Language)
	}
	if uc.gotInput.GitHubToken != "" {
		t.Errorf("gotInput.GitHubToken = %q, want empty (Phase 2 has no token passthrough)", uc.gotInput.GitHubToken)
	}
}

func TestPromptHandler_InvalidJSON(t *testing.T) {
	uc := &fakePromptUsecase{}
	h := NewPromptHandler(uc)

	rec := servePrompt(h, `{not json`)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if uc.calls != 0 {
		t.Errorf("usecase called %d times, want 0", uc.calls)
	}
	assertErrorCode(t, rec, "invalid_input")
}

func TestPromptHandler_MissingPullRequestURL(t *testing.T) {
	uc := &fakePromptUsecase{}
	h := NewPromptHandler(uc)

	rec := servePrompt(h, `{"mode":"light"}`)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if uc.calls != 0 {
		t.Error("usecase must not be called when validation fails")
	}
	assertErrorCode(t, rec, "invalid_input")
	if !strings.Contains(readErrorMessage(t, rec), "pull_request_url") {
		t.Error("error.message must mention pull_request_url")
	}
}

func TestPromptHandler_InvalidPullRequestURL(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{"http scheme", "http://github.com/owner/repo/pull/1"},
		{"wrong host", "https://gitlab.com/owner/repo/pull/1"},
		{"tree not pull", "https://github.com/owner/repo/tree/main"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			uc := &fakePromptUsecase{}
			h := NewPromptHandler(uc)

			rec := servePrompt(h, `{"pull_request_url":"`+tc.url+`"}`)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", rec.Code)
			}
			if uc.calls != 0 {
				t.Error("usecase must not be called when URL validation fails")
			}
			assertErrorCode(t, rec, "invalid_input")
		})
	}
}

func TestPromptHandler_BodyTooLarge(t *testing.T) {
	uc := &fakePromptUsecase{}
	h := NewPromptHandler(uc)

	payload := `{"pull_request_url":"https://github.com/owner/repo/pull/1"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/prompt", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	req.Body = http.MaxBytesReader(rec, req.Body, 8)
	h.Handle(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", rec.Code)
	}
	assertErrorCode(t, rec, "payload_too_large")
	if uc.calls != 0 {
		t.Error("usecase must not be called when body exceeds limit")
	}
}

func TestPromptHandler_UsecaseErrorMapping(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode string
		wantHTTP int
	}{
		{"invalid input", prism.ErrInvalidInput, "invalid_input", http.StatusBadRequest},
		{"unsupported provider", prism.ErrUnsupportedProvider, "unsupported_provider", http.StatusBadRequest},
		{"auth required", prism.ErrAuthRequired, "auth_required", http.StatusUnauthorized},
		{"upstream failure", prism.ErrUpstreamFailure, "upstream_failure", http.StatusBadGateway},
		{"unknown error", errors.New("boom"), "internal_error", http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			uc := &fakePromptUsecase{err: tc.err}
			h := NewPromptHandler(uc)

			rec := servePrompt(h, `{"pull_request_url":"https://github.com/owner/repo/pull/1"}`)

			if rec.Code != tc.wantHTTP {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantHTTP)
			}
			assertErrorCode(t, rec, tc.wantCode)
		})
	}
}

func TestPromptRequest_Validate(t *testing.T) {
	cases := []struct {
		name    string
		req     PromptRequest
		wantErr bool
	}{
		{"valid minimal", PromptRequest{PullRequestURL: "https://github.com/owner/repo/pull/1"}, false},
		{"valid full", PromptRequest{
			PullRequestURL: "https://github.com/owner/repo/pull/1",
			Mode:           "detailed",
			Language:       "ja",
		}, false},
		{"empty url", PromptRequest{PullRequestURL: ""}, true},
		{"whitespace-only url", PromptRequest{PullRequestURL: "  "}, true},
		{"non-github url", PromptRequest{PullRequestURL: "https://example.com/foo"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() err = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}
