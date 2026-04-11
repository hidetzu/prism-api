// Package handler contains HTTP handlers for prism-api endpoints.
package handler

import (
	"net/http"
	"runtime"

	"github.com/hidetzu/prism-api/internal/httpapi/response"
)

// apiVersion is the current prism-api release marker. Bump at tag time.
const apiVersion = "0.2.0"

// HealthHandler serves liveness, readiness, and version endpoints.
type HealthHandler struct {
	apiVersion string
	goVersion  string
}

// NewHealthHandler constructs a HealthHandler with versions captured at
// startup.
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{
		apiVersion: apiVersion,
		goVersion:  runtime.Version(),
	}
}

// Live serves the liveness probe. A 200 response indicates the process is
// running and able to serve requests.
func (h *HealthHandler) Live(w http.ResponseWriter, _ *http.Request) {
	_ = response.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Ready serves the readiness probe. In Phase 1 there are no downstream
// dependencies to probe, so readiness is equivalent to liveness.
func (h *HealthHandler) Ready(w http.ResponseWriter, _ *http.Request) {
	_ = response.WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// Version returns the API and Go runtime versions.
func (h *HealthHandler) Version(w http.ResponseWriter, _ *http.Request) {
	_ = response.WriteJSON(w, http.StatusOK, map[string]string{
		"api_version": h.apiVersion,
		"go_version":  h.goVersion,
	})
}
