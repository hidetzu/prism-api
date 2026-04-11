package usecase

import (
	"context"

	"github.com/hidetzu/prism/pkg/prism"
)

// PromptInput carries the HTTP-layer-agnostic inputs that a prompt handler
// extracts from its validated request. Mode and Language are optional;
// empty values are forwarded as-is to pkg/prism, which applies its own
// defaults ("light" mode, "en" language). We intentionally do not lock
// those defaults into prism-api code so that future pkg/prism updates
// take effect without a prism-api change.
type PromptInput struct {
	PullRequestURL string
	GitHubToken    string
	Mode           string
	Language       string
}

// Prompter is the thin adapter between HTTP handlers and pkg/prism.Prompt.
// It stores the pkg/prism entry point as a function field so tests can
// inject a fake via field assignment without wrapping the stdlib function
// in an extra interface. See Analyzer for the same rationale.
type Prompter struct {
	prompt func(ctx context.Context, opts prism.AnalyzeOptions) (string, error)
}

// NewPrompter constructs a Prompter that delegates to the real
// pkg/prism.Prompt. Production code should always use this constructor.
func NewPrompter() *Prompter {
	return &Prompter{prompt: prism.Prompt}
}

// Prompt invokes pkg/prism.Prompt with the GitHub provider fixed, patches
// excluded, and the caller-supplied Mode and Language forwarded without
// modification. Phase 2 is GitHub only (Phase 2 instruction §5), so
// Provider is always "github". Empty Mode / Language are intentional: the
// upstream library fills in its own defaults.
//
// pkg/prism's sentinel errors (ErrInvalidInput, ErrUnsupportedProvider,
// ErrAuthRequired, ErrUpstreamFailure) flow through unchanged so the HTTP
// layer can map them to the correct response.Code via errors.Is.
func (p *Prompter) Prompt(ctx context.Context, in PromptInput) (string, error) {
	return p.prompt(ctx, prism.AnalyzeOptions{
		Provider:       "github",
		PRURL:          in.PullRequestURL,
		GitHubToken:    in.GitHubToken,
		IncludePatches: false,
		Mode:           in.Mode,
		Language:       in.Language,
	})
}
