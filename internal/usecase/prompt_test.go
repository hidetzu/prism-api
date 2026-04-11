package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/hidetzu/prism/pkg/prism"
)

// fakePrompter returns a Prompter whose underlying prism.Prompt call is
// replaced with fn. Tests use this to observe the AnalyzeOptions handed
// into pkg/prism and to return canned results or errors.
func fakePrompter(fn func(ctx context.Context, opts prism.AnalyzeOptions) (string, error)) *Prompter {
	return &Prompter{prompt: fn}
}

func TestPrompter_Prompt_PassesCorrectOptions(t *testing.T) {
	var got prism.AnalyzeOptions
	p := fakePrompter(func(_ context.Context, opts prism.AnalyzeOptions) (string, error) {
		got = opts
		return "generated prompt", nil
	})

	out, err := p.Prompt(context.Background(), PromptInput{
		PullRequestURL: "https://github.com/owner/repo/pull/42",
		GitHubToken:    "ghp_example",
		Mode:           "detailed",
		Language:       "ja",
	})
	if err != nil {
		t.Fatalf("Prompt() err = %v", err)
	}

	if got.Provider != "github" {
		t.Errorf("Provider = %q, want github", got.Provider)
	}
	if got.PRURL != "https://github.com/owner/repo/pull/42" {
		t.Errorf("PRURL = %q", got.PRURL)
	}
	if got.GitHubToken != "ghp_example" {
		t.Errorf("GitHubToken = %q, want ghp_example", got.GitHubToken)
	}
	if got.IncludePatches {
		t.Error("IncludePatches must be false")
	}
	if got.Mode != "detailed" {
		t.Errorf("Mode = %q, want detailed", got.Mode)
	}
	if got.Language != "ja" {
		t.Errorf("Language = %q, want ja", got.Language)
	}
	if out != "generated prompt" {
		t.Errorf("prompt = %q", out)
	}
}

func TestPrompter_Prompt_EmptyModeAndLanguageForwarded(t *testing.T) {
	// Empty Mode and Language must be forwarded as empty strings so
	// pkg/prism applies its own defaults. prism-api does not duplicate
	// those defaults in its own code.
	var got prism.AnalyzeOptions
	p := fakePrompter(func(_ context.Context, opts prism.AnalyzeOptions) (string, error) {
		got = opts
		return "", nil
	})

	if _, err := p.Prompt(context.Background(), PromptInput{
		PullRequestURL: "https://github.com/owner/repo/pull/1",
	}); err != nil {
		t.Fatalf("Prompt() err = %v", err)
	}
	if got.Mode != "" {
		t.Errorf("Mode = %q, want empty", got.Mode)
	}
	if got.Language != "" {
		t.Errorf("Language = %q, want empty", got.Language)
	}
}

func TestPrompter_Prompt_EmptyTokenIsPassedThrough(t *testing.T) {
	var gotToken string
	p := fakePrompter(func(_ context.Context, opts prism.AnalyzeOptions) (string, error) {
		gotToken = opts.GitHubToken
		return "", nil
	})

	if _, err := p.Prompt(context.Background(), PromptInput{
		PullRequestURL: "https://github.com/owner/repo/pull/1",
	}); err != nil {
		t.Fatalf("Prompt() err = %v", err)
	}
	if gotToken != "" {
		t.Errorf("GitHubToken = %q, want empty string", gotToken)
	}
}

func TestPrompter_Prompt_ErrorPassthrough(t *testing.T) {
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
			p := fakePrompter(func(_ context.Context, _ prism.AnalyzeOptions) (string, error) {
				return "", tc.err
			})
			_, err := p.Prompt(context.Background(), PromptInput{
				PullRequestURL: "https://github.com/owner/repo/pull/1",
			})
			if !errors.Is(err, tc.err) {
				t.Errorf("errors.Is(err, %v) = false, err = %v", tc.err, err)
			}
		})
	}
}

func TestPrompter_Prompt_ContextPropagated(t *testing.T) {
	type ctxKey struct{}
	want := "marker"
	ctx := context.WithValue(context.Background(), ctxKey{}, want)

	var observed any
	p := fakePrompter(func(ctx context.Context, _ prism.AnalyzeOptions) (string, error) {
		observed = ctx.Value(ctxKey{})
		return "", nil
	})

	if _, err := p.Prompt(ctx, PromptInput{
		PullRequestURL: "https://github.com/owner/repo/pull/1",
	}); err != nil {
		t.Fatalf("Prompt() err = %v", err)
	}
	if observed != want {
		t.Errorf("context value in downstream call = %v, want %q", observed, want)
	}
}

func TestNewPrompter_UsesRealPrism(t *testing.T) {
	p := NewPrompter()
	if p == nil {
		t.Fatal("NewPrompter() = nil")
	}
	if p.prompt == nil {
		t.Error("prompt function field must be populated by NewPrompter")
	}
}
