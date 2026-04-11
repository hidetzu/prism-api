package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/hidetzu/prism-api/internal/httpapi/middleware"
	"github.com/hidetzu/prism-api/internal/httpapi/response"
	"github.com/hidetzu/prism-api/internal/usecase"
	"github.com/hidetzu/prism-api/internal/validation"
)

// PromptRequest is the JSON body accepted by POST /v1/prompt. Mode and
// Language are optional; when omitted (or empty), the request is forwarded
// to pkg/prism which applies its own defaults ("light" and "en").
type PromptRequest struct {
	PullRequestURL string `json:"pull_request_url"`
	Mode           string `json:"mode,omitempty"`
	Language       string `json:"language,omitempty"`
}

// Validate enforces that pull_request_url is present and a well-formed
// GitHub pull request URL. Mode and Language are not validated here —
// pkg/prism either accepts them or rejects the call as ErrInvalidInput,
// which flows back through the usecase layer and becomes a 400 response.
func (r PromptRequest) Validate() error {
	if err := validation.Required("pull_request_url", r.PullRequestURL); err != nil {
		return err
	}
	return validation.GitHubPullRequestURL(r.PullRequestURL)
}

// PromptUsecase is the handler-side view of the prompt usecase. Defined
// here (not in internal/usecase) so the handler package follows the
// "accept interfaces, return structs" idiom per
// docs/development_rules.md §10.
type PromptUsecase interface {
	Prompt(ctx context.Context, in usecase.PromptInput) (string, error)
}

// PromptHandler serves POST /v1/prompt.
type PromptHandler struct {
	uc PromptUsecase
}

// NewPromptHandler constructs a PromptHandler with the provided usecase.
func NewPromptHandler(uc PromptUsecase) *PromptHandler {
	return &PromptHandler{uc: uc}
}

// Handle serves a single POST /v1/prompt request.
func (h *PromptHandler) Handle(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.RequestIDFrom(r.Context())

	var req PromptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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

	prompt, err := h.uc.Prompt(r.Context(), usecase.PromptInput{
		PullRequestURL: req.PullRequestURL,
		Mode:           req.Mode,
		Language:       req.Language,
	})
	if err != nil {
		writeUsecaseError(w, requestID, err)
		return
	}

	// Phase 2 instruction §8: envelope is {"prompt": "..."}.
	_ = response.WriteJSON(w, http.StatusOK, map[string]any{"prompt": prompt})
}
