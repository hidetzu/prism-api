package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/hidetzu/prism/pkg/prism"
)

// fakeAnalyzer returns an Analyzer whose underlying prism.Analyze call is
// replaced with fn. Tests use this to observe the AnalyzeOptions handed
// into pkg/prism and to return canned results or errors.
func fakeAnalyzer(fn func(ctx context.Context, opts prism.AnalyzeOptions) (prism.Result, error)) *Analyzer {
	return &Analyzer{analyze: fn}
}

func TestAnalyzer_Analyze_PassesCorrectOptions(t *testing.T) {
	var got prism.AnalyzeOptions
	a := fakeAnalyzer(func(_ context.Context, opts prism.AnalyzeOptions) (prism.Result, error) {
		got = opts
		return prism.Result{
			PR: prism.PRInfo{
				Provider:   "github",
				Repository: "owner/repo",
				ID:         "123",
				URL:        "https://github.com/owner/repo/pull/123",
			},
		}, nil
	})

	result, err := a.Analyze(context.Background(), AnalyzeInput{
		PullRequestURL: "https://github.com/owner/repo/pull/123",
		GitHubToken:    "ghp_example",
	})
	if err != nil {
		t.Fatalf("Analyze() err = %v", err)
	}

	if got.Provider != "github" {
		t.Errorf("Provider = %q, want github", got.Provider)
	}
	if got.PRURL != "https://github.com/owner/repo/pull/123" {
		t.Errorf("PRURL = %q", got.PRURL)
	}
	if got.GitHubToken != "ghp_example" {
		t.Errorf("GitHubToken = %q, want ghp_example", got.GitHubToken)
	}
	if got.IncludePatches {
		t.Error("IncludePatches must remain false (pkg/prism default)")
	}
	if got.Mode != "" {
		t.Errorf("Mode = %q, want empty for Analyze", got.Mode)
	}
	if got.Language != "" {
		t.Errorf("Language = %q, want empty for Analyze", got.Language)
	}

	if result.PR.Repository != "owner/repo" {
		t.Errorf("result.PR.Repository = %q, want owner/repo", result.PR.Repository)
	}
}

func TestAnalyzer_Analyze_EmptyTokenIsPassedThrough(t *testing.T) {
	// Phase 2 supports unauthenticated public-repo calls; the adapter must
	// not substitute a default token for an empty input.
	var gotToken string
	a := fakeAnalyzer(func(_ context.Context, opts prism.AnalyzeOptions) (prism.Result, error) {
		gotToken = opts.GitHubToken
		return prism.Result{}, nil
	})

	if _, err := a.Analyze(context.Background(), AnalyzeInput{
		PullRequestURL: "https://github.com/owner/repo/pull/1",
	}); err != nil {
		t.Fatalf("Analyze() err = %v", err)
	}
	if gotToken != "" {
		t.Errorf("GitHubToken = %q, want empty string", gotToken)
	}
}

func TestAnalyzer_Analyze_ErrorPassthrough(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"invalid input", prism.ErrInvalidInput},
		{"unsupported provider", prism.ErrUnsupportedProvider},
		{"auth required", prism.ErrAuthRequired},
		{"upstream failure", prism.ErrUpstreamFailure},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := fakeAnalyzer(func(_ context.Context, _ prism.AnalyzeOptions) (prism.Result, error) {
				return prism.Result{}, tc.err
			})
			_, err := a.Analyze(context.Background(), AnalyzeInput{
				PullRequestURL: "https://github.com/owner/repo/pull/1",
			})
			if !errors.Is(err, tc.err) {
				t.Errorf("errors.Is(err, %v) = false, err = %v", tc.err, err)
			}
		})
	}
}

func TestAnalyzer_Analyze_ContextPropagated(t *testing.T) {
	// pkg/prism cancellation depends on the context reaching the call
	// site, so verify the adapter forwards ctx rather than dropping it.
	type ctxKey struct{}
	want := "marker"
	ctx := context.WithValue(context.Background(), ctxKey{}, want)

	var observed any
	a := fakeAnalyzer(func(ctx context.Context, _ prism.AnalyzeOptions) (prism.Result, error) {
		observed = ctx.Value(ctxKey{})
		return prism.Result{}, nil
	})

	if _, err := a.Analyze(ctx, AnalyzeInput{
		PullRequestURL: "https://github.com/owner/repo/pull/1",
	}); err != nil {
		t.Fatalf("Analyze() err = %v", err)
	}
	if observed != want {
		t.Errorf("context value in downstream call = %v, want %q", observed, want)
	}
}

func TestNewAnalyzer_UsesRealPrism(t *testing.T) {
	// Smoke check: the constructor must return a non-nil Analyzer whose
	// analyze field is populated. We do not actually invoke pkg/prism
	// here (that would reach the network).
	a := NewAnalyzer()
	if a == nil {
		t.Fatal("NewAnalyzer() = nil")
	}
	if a.analyze == nil {
		t.Error("analyze function field must be populated by NewAnalyzer")
	}
}
