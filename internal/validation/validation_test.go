package validation

import (
	"errors"
	"testing"
)

func TestRequired(t *testing.T) {
	cases := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"non-empty", "value", false},
		{"value with internal spaces", "a b", false},
		{"value with surrounding spaces", " value ", false},
		{"empty", "", true},
		{"spaces only", "   ", true},
		{"tab only", "\t", true},
		{"newline only", "\n", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Required("field", tc.value)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Required(%q) err = %v, wantErr = %v", tc.value, err, tc.wantErr)
			}
			if err == nil {
				return
			}
			var verr *Error
			if !errors.As(err, &verr) {
				t.Fatalf("err is not *Error: %T", err)
			}
			if verr.Field != "field" {
				t.Errorf("Field = %q, want %q", verr.Field, "field")
			}
			if verr.Message == "" {
				t.Errorf("Message must be non-empty")
			}
		})
	}
}

func TestGitHubPullRequestURL_Accepted(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{"canonical", "https://github.com/owner/repo/pull/123"},
		{"large number", "https://github.com/owner/repo/pull/999999"},
		{"trailing slash", "https://github.com/owner/repo/pull/123/"},
		{"query string tolerated", "https://github.com/owner/repo/pull/123?foo=bar"},
		{"fragment tolerated", "https://github.com/owner/repo/pull/123#discussion"},
		{"repo name with hyphen", "https://github.com/org-name/repo-name/pull/1"},
		{"repo name with dot", "https://github.com/owner/repo.js/pull/1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := GitHubPullRequestURL(tc.url); err != nil {
				t.Errorf("GitHubPullRequestURL(%q) err = %v, want nil", tc.url, err)
			}
		})
	}
}

func TestGitHubPullRequestURL_Rejected(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{"empty", ""},
		{"bare word", "notaurl"},
		{"http scheme", "http://github.com/owner/repo/pull/123"},
		{"ftp scheme", "ftp://github.com/owner/repo/pull/123"},
		{"no scheme", "github.com/owner/repo/pull/123"},
		{"wrong host", "https://gitlab.com/owner/repo/pull/123"},
		{"www subdomain", "https://www.github.com/owner/repo/pull/123"},
		{"enterprise subdomain", "https://github.corp.example/owner/repo/pull/123"},
		{"missing path", "https://github.com"},
		{"only owner", "https://github.com/owner"},
		{"no pull segment", "https://github.com/owner/repo"},
		{"issues instead of pull", "https://github.com/owner/repo/issues/123"},
		{"too many segments", "https://github.com/owner/repo/pull/123/files"},
		{"non-numeric pr number", "https://github.com/owner/repo/pull/abc"},
		{"zero pr number", "https://github.com/owner/repo/pull/0"},
		{"negative pr number", "https://github.com/owner/repo/pull/-5"},
		{"empty owner segment", "https://github.com//repo/pull/123"},
		{"empty repo segment", "https://github.com/owner//pull/123"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := GitHubPullRequestURL(tc.url)
			if err == nil {
				t.Fatalf("GitHubPullRequestURL(%q) err = nil, want error", tc.url)
			}
			var verr *Error
			if !errors.As(err, &verr) {
				t.Fatalf("err is not *Error: %T", err)
			}
			if verr.Field != "pull_request_url" {
				t.Errorf("Field = %q, want pull_request_url", verr.Field)
			}
			if verr.Message == "" {
				t.Errorf("Message must be non-empty")
			}
		})
	}
}

func TestError_Error(t *testing.T) {
	e := &Error{Field: "pull_request_url", Message: "is required"}
	const want = "pull_request_url: is required"
	if got := e.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
