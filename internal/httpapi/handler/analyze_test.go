package handler

import (
	"bytes"
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

// fakeAnalyzeUsecase is a test double implementing the AnalyzeUsecase
// interface. Tests set result/err to shape the response and read gotInput
// after invocation to assert on what the handler forwarded.
type fakeAnalyzeUsecase struct {
	result   prism.Result
	err      error
	gotInput usecase.AnalyzeInput
	calls    int
}

func (f *fakeAnalyzeUsecase) Analyze(_ context.Context, in usecase.AnalyzeInput) (prism.Result, error) {
	f.calls++
	f.gotInput = in
	return f.result, f.err
}

func serve(h *AnalyzeHandler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/analyze", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.Handle(rec, req)
	return rec
}

func TestAnalyzeHandler_Success(t *testing.T) {
	uc := &fakeAnalyzeUsecase{
		result: prism.Result{
			PR: prism.PRInfo{
				Provider:   "github",
				Repository: "owner/repo",
				ID:         "123",
				Title:      "Example PR",
				Author:     "alice",
				URL:        "https://github.com/owner/repo/pull/123",
			},
			Analysis: prism.AnalysisResult{
				ChangeType: "feature",
				RiskLevel:  "low",
				Summary:    "Adds a small feature",
			},
			Files: []prism.ChangedFile{
				{Path: "a.go", Status: "modified", Additions: 10, Deletions: 2, Language: "go"},
			},
		},
	}
	h := NewAnalyzeHandler(uc)

	rec := serve(h, `{"pull_request_url":"https://github.com/owner/repo/pull/123"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if uc.calls != 1 {
		t.Errorf("usecase called %d times, want 1", uc.calls)
	}
	if uc.gotInput.PullRequestURL != "https://github.com/owner/repo/pull/123" {
		t.Errorf("usecase.gotInput.PullRequestURL = %q", uc.gotInput.PullRequestURL)
	}

	var body struct {
		Result struct {
			PullRequest struct {
				Provider   string `json:"provider"`
				Repository string `json:"repository"`
				ID         string `json:"id"`
				URL        string `json:"url"`
			} `json:"pull_request"`
			Analysis struct {
				ChangeType string `json:"change_type"`
				RiskLevel  string `json:"risk_level"`
				Summary    string `json:"summary"`
			} `json:"analysis"`
			ChangedFiles []struct {
				Path      string `json:"path"`
				Additions int    `json:"additions"`
				Deletions int    `json:"deletions"`
			} `json:"changed_files"`
		} `json:"result"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Result.PullRequest.Repository != "owner/repo" {
		t.Errorf("pull_request.repository = %q", body.Result.PullRequest.Repository)
	}
	if body.Result.Analysis.ChangeType != "feature" {
		t.Errorf("analysis.change_type = %q", body.Result.Analysis.ChangeType)
	}
	if len(body.Result.ChangedFiles) != 1 || body.Result.ChangedFiles[0].Path != "a.go" {
		t.Errorf("changed_files = %+v", body.Result.ChangedFiles)
	}
}

func TestAnalyzeHandler_InvalidJSON(t *testing.T) {
	uc := &fakeAnalyzeUsecase{}
	h := NewAnalyzeHandler(uc)

	rec := serve(h, `{not json`)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if uc.calls != 0 {
		t.Errorf("usecase called %d times, want 0", uc.calls)
	}
	assertErrorCode(t, rec, "invalid_input")
}

func TestAnalyzeHandler_MissingPullRequestURL(t *testing.T) {
	uc := &fakeAnalyzeUsecase{}
	h := NewAnalyzeHandler(uc)

	rec := serve(h, `{}`)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if uc.calls != 0 {
		t.Error("usecase must not be called when validation fails")
	}
	assertErrorCode(t, rec, "invalid_input")
	// Error message should name the missing field.
	if !strings.Contains(readErrorMessage(t, rec), "pull_request_url") {
		t.Error("error.message must mention pull_request_url")
	}
}

func TestAnalyzeHandler_InvalidPullRequestURL(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{"http scheme", "http://github.com/owner/repo/pull/1"},
		{"wrong host", "https://gitlab.com/owner/repo/pull/1"},
		{"issues not pull", "https://github.com/owner/repo/issues/1"},
		{"missing number", "https://github.com/owner/repo/pull"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			uc := &fakeAnalyzeUsecase{}
			h := NewAnalyzeHandler(uc)

			rec := serve(h, `{"pull_request_url":"`+tc.url+`"}`)

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

func TestAnalyzeHandler_BodyTooLarge(t *testing.T) {
	// Simulate the middleware's MaxBytesReader by wrapping Body directly
	// in the request. When the handler reads past the limit the decoder
	// returns *http.MaxBytesError, which must map to 413.
	uc := &fakeAnalyzeUsecase{}
	h := NewAnalyzeHandler(uc)

	payload := `{"pull_request_url":"https://github.com/owner/repo/pull/123"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/analyze", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	// Limit below payload size so Decode trips MaxBytesError.
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

func TestAnalyzeHandler_UsecaseErrorMapping(t *testing.T) {
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
		{"context deadline exceeded", context.DeadlineExceeded, "timeout", http.StatusGatewayTimeout},
		{"context canceled", context.Canceled, "timeout", http.StatusGatewayTimeout},
		{"unknown error", errors.New("boom"), "internal_error", http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			uc := &fakeAnalyzeUsecase{err: tc.err}
			h := NewAnalyzeHandler(uc)

			rec := serve(h, `{"pull_request_url":"https://github.com/owner/repo/pull/1"}`)

			if rec.Code != tc.wantHTTP {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantHTTP)
			}
			assertErrorCode(t, rec, tc.wantCode)
		})
	}
}

func TestAnalyzeRequest_Validate(t *testing.T) {
	// Unit test for the Validate() method in isolation from the handler.
	cases := []struct {
		name    string
		req     AnalyzeRequest
		wantErr bool
	}{
		{"valid", AnalyzeRequest{PullRequestURL: "https://github.com/owner/repo/pull/1"}, false},
		{"empty", AnalyzeRequest{PullRequestURL: ""}, true},
		{"whitespace only", AnalyzeRequest{PullRequestURL: "   "}, true},
		{"not a github url", AnalyzeRequest{PullRequestURL: "https://example.com/foo"}, true},
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

// assertErrorCode decodes the standard error envelope and checks error.code.
func assertErrorCode(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	// Decode from a buffered copy so other helpers on the same rec can read it too.
	if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body.Error.Code != want {
		t.Errorf("error.code = %q, want %q", body.Error.Code, want)
	}
	if body.Error.Message == "" {
		t.Error("error.message must be non-empty")
	}
}

// readErrorMessage decodes error.message from the recorder body.
func readErrorMessage(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	return body.Error.Message
}
