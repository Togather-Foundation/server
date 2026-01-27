package handlers

import (
	"net/http"
	"time"
)

// WellKnownHandler handles .well-known endpoints for node discovery
type WellKnownHandler struct {
	BaseURL string
	Version string
	Updated time.Time
}

// NewWellKnownHandler creates a new well-known endpoint handler
func NewWellKnownHandler(baseURL string, version string, updated time.Time) *WellKnownHandler {
	return &WellKnownHandler{
		BaseURL: baseURL,
		Version: version,
		Updated: updated,
	}
}

// SELProfile implements GET /.well-known/sel-profile per Interoperability Profile §1.7
// Returns node metadata for federation discovery
func (h *WellKnownHandler) SELProfile(w http.ResponseWriter, r *http.Request) {
	if h == nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	payload := map[string]any{
		"profile": "https://sel.events/profiles/interop",
		"version": h.Version,
		"node":    h.BaseURL,
		"updated": h.Updated.Format("2006-01-02"),
	}

	writeJSON(w, http.StatusOK, payload, "application/json")
}
