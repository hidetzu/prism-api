// Package validation provides shared input validation helpers used by HTTP
// handlers. Per docs/development_rules.md §6, validation runs inside handlers
// rather than as middleware; helpers here are request-type-agnostic building
// blocks that request types call from their Validate() methods.
package validation

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Error is a structured validation failure. Handlers map it to
// response.CodeInvalidInput, writing Message into the error body. Use
// errors.As to extract it:
//
//	var verr *validation.Error
//	if errors.As(err, &verr) { ... }
type Error struct {
	Field   string
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// Required returns a validation Error if value is empty or contains only
// whitespace. A whitespace-only string is treated as missing because it
// almost always reflects a client-side bug rather than intent.
func Required(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return &Error{Field: field, Message: "is required"}
	}
	return nil
}

// GitHubPullRequestURL returns a validation Error if raw is not a pull
// request URL in the form https://github.com/<owner>/<repo>/pull/<number>.
// The check is strict:
//
//   - scheme must be https
//   - host must be exactly github.com (no www, no enterprise subdomain)
//   - path must be /<owner>/<repo>/pull/<number>
//   - <number> must be a positive integer
//
// Query strings and trailing slashes are tolerated.
func GitHubPullRequestURL(raw string) error {
	const field = "pull_request_url"

	u, err := url.Parse(raw)
	if err != nil {
		return &Error{Field: field, Message: "is not a valid URL"}
	}
	if u.Scheme != "https" {
		return &Error{Field: field, Message: "must use https scheme"}
	}
	if u.Host != "github.com" {
		return &Error{Field: field, Message: "must be a github.com URL"}
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 4 || parts[0] == "" || parts[1] == "" || parts[2] != "pull" {
		return &Error{Field: field, Message: "must be a pull request URL of the form /<owner>/<repo>/pull/<number>"}
	}
	n, err := strconv.Atoi(parts[3])
	if err != nil || n <= 0 {
		return &Error{Field: field, Message: "pull request number must be a positive integer"}
	}
	return nil
}
