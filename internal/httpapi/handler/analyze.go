package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/hidetzu/prism/pkg/prism"

	"github.com/hidetzu/prism-api/internal/httpapi/middleware"
	"github.com/hidetzu/prism-api/internal/httpapi/response"
	"github.com/hidetzu/prism-api/internal/usecase"
	"github.com/hidetzu/prism-api/internal/validation"
)

// AnalyzeRequest is the JSON body accepted by POST /v1/analyze.
type AnalyzeRequest struct {
	PullRequestURL string `json:"pull_request_url"`
}

// Validate enforces that pull_request_url is present and a well-formed
// GitHub pull request URL. Handlers call this before invoking the usecase.
func (r AnalyzeRequest) Validate() error {
	if err := validation.Required("pull_request_url", r.PullRequestURL); err != nil {
		return err
	}
	return validation.GitHubPullRequestURL(r.PullRequestURL)
}

// AnalyzeUsecase is the handler-side view of the usecase dependency. It is
// defined here (not in internal/usecase) so the handler package follows
// Go's "accept interfaces, return structs" idiom per
// docs/development_rules.md §10. The concrete implementation is
// usecase.Analyzer.
type AnalyzeUsecase interface {
	Analyze(ctx context.Context, in usecase.AnalyzeInput) (prism.Result, error)
}

// AnalyzeHandler serves POST /v1/analyze. It decodes the request, validates
// it, delegates to the usecase, and writes either the pkg/prism Result as
// {"result": ...} or a standard error body.
type AnalyzeHandler struct {
	uc AnalyzeUsecase
}

// NewAnalyzeHandler constructs an AnalyzeHandler with the provided usecase.
func NewAnalyzeHandler(uc AnalyzeUsecase) *AnalyzeHandler {
	return &AnalyzeHandler{uc: uc}
}

// Handle serves a single POST /v1/analyze request.
func (h *AnalyzeHandler) Handle(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.RequestIDFrom(r.Context())

	var req AnalyzeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// body_limit middleware wraps r.Body with http.MaxBytesReader; when
		// the read crosses the limit the decoder surfaces *http.MaxBytesError.
		// Translate to 413 so the rejection semantics match the early-path
		// rejection performed by body_limit itself.
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			response.WriteError(w, requestID, response.CodePayloadTooLarge,
				"request body exceeds the configured limit")
			return
		}
		response.WriteError(w, requestID, response.CodeInvalidInput,
			"request body must be a valid JSON object")
		return
	}

	if err := req.Validate(); err != nil {
		var verr *validation.Error
		if errors.As(err, &verr) {
			response.WriteError(w, requestID, response.CodeInvalidInput, verr.Error())
			return
		}
		response.WriteError(w, requestID, response.CodeInvalidInput, err.Error())
		return
	}

	result, err := h.uc.Analyze(r.Context(), usecase.AnalyzeInput{
		PullRequestURL: req.PullRequestURL,
	})
	if err != nil {
		writeAnalyzeUsecaseError(w, requestID, err)
		return
	}

	// pkg/prism.Result already has the JSON tags the response contract
	// wants (pull_request, analysis, changed_files), so we can serialize
	// it directly without a translation layer. Per Phase 2 instruction §7
	// the envelope is {"result": <Result>}.
	_ = response.WriteJSON(w, http.StatusOK, map[string]any{"result": result})
}

// writeAnalyzeUsecaseError maps pkg/prism sentinel errors to the canonical
// response.Code values. Unknown errors become CodeInternalError without
// leaking the underlying message to the client.
func writeAnalyzeUsecaseError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, prism.ErrInvalidInput):
		response.WriteError(w, requestID, response.CodeInvalidInput,
			"the pull request input could not be processed")
	case errors.Is(err, prism.ErrUnsupportedProvider):
		response.WriteError(w, requestID, response.CodeUnsupportedProvider,
			"the requested provider is not supported")
	case errors.Is(err, prism.ErrAuthRequired):
		response.WriteError(w, requestID, response.CodeAuthRequired,
			"authentication is required to access this repository")
	case errors.Is(err, prism.ErrUpstreamFailure):
		response.WriteError(w, requestID, response.CodeUpstreamFailure,
			"upstream service is temporarily unavailable")
	default:
		response.WriteError(w, requestID, response.CodeInternalError,
			"internal server error")
	}
}
