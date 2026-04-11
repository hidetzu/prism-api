// Package usecase holds the thin application-layer adapters between HTTP
// handlers and github.com/hidetzu/prism/pkg/prism. Handlers depend on
// small interfaces defined in the handler package (accept interfaces,
// return structs, per docs/development_rules.md §10); the concrete
// implementations live here.
package usecase

import (
	"context"

	"github.com/hidetzu/prism/pkg/prism"
)

// AnalyzeInput carries the HTTP-layer-agnostic inputs that a handler
// extracts from its validated AnalyzeRequest and hands to the usecase.
// Handler-specific concerns (JSON field names, HTTP headers, error
// response shape) stay in the handler package.
type AnalyzeInput struct {
	PullRequestURL string
	GitHubToken    string
}

// Analyzer is the thin adapter between HTTP handlers and pkg/prism.Analyze.
//
// The pkg/prism entry point is stored as a function field rather than an
// interface wrapper. That keeps the indirection minimal (no extra type to
// maintain and no wrapping layer at call time) and still lets tests inject
// a fake via field assignment.
type Analyzer struct {
	analyze func(ctx context.Context, opts prism.AnalyzeOptions) (prism.Result, error)
}

// NewAnalyzer constructs an Analyzer that delegates to the real
// pkg/prism.Analyze. Production code should always use this constructor.
func NewAnalyzer() *Analyzer {
	return &Analyzer{analyze: prism.Analyze}
}

// Analyze invokes pkg/prism.Analyze with the GitHub provider fixed and
// patches excluded. Phase 2 is GitHub only (Phase 2 instruction §5), so
// Provider is always "github"; the auto-detection path in pkg/prism is
// deliberately bypassed so the provider is explicit in logs and debug
// traces. IncludePatches stays at the pkg/prism default (false) to keep
// response bodies lightweight; a future API flag can opt in per request.
//
// The returned Result is passed through as-is. pkg/prism's sentinel errors
// (ErrInvalidInput, ErrUnsupportedProvider, ErrAuthRequired,
// ErrUpstreamFailure) also flow through unchanged so the HTTP layer can
// map them to the correct response.Code via errors.Is.
func (a *Analyzer) Analyze(ctx context.Context, in AnalyzeInput) (prism.Result, error) {
	return a.analyze(ctx, prism.AnalyzeOptions{
		Provider:       "github",
		PRURL:          in.PullRequestURL,
		GitHubToken:    in.GitHubToken,
		IncludePatches: false,
	})
}
