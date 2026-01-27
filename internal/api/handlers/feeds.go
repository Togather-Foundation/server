package handlers

import (
	"net/http"
	"strconv"

	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/domain/federation"
)

// FeedsHandler handles change feed endpoints.
type FeedsHandler struct {
	Service *federation.ChangeFeedService
	Env     string
	BaseURL string
}

// NewFeedsHandler creates a new feeds handler.
func NewFeedsHandler(service *federation.ChangeFeedService, env string, baseURL string) *FeedsHandler {
	return &FeedsHandler{
		Service: service,
		Env:     env,
		BaseURL: baseURL,
	}
}

// ListChanges handles GET /api/v1/feeds/changes
func (h *FeedsHandler) ListChanges(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Service == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	// Parse query parameters
	query := r.URL.Query()

	// Support both 'since' (per Interop Profile) and 'after' (legacy) for cursor parameter
	// 'since' parameter can be either a cursor or timestamp - cursor takes precedence
	cursor := query.Get("since") // Prefer 'since' per Interop Profile §4.3
	if cursor == "" {
		cursor = query.Get("after") // Fallback to 'after' for backward compatibility
	}

	// Build service params
	params := federation.ChangeFeedParams{
		After:           cursor,
		Action:          query.Get("action"),
		IncludeSnapshot: query.Get("include_snapshot") == "true",
	}

	// Parse limit
	if limitStr := query.Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit < 1 || limit > 1000 {
			problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid limit parameter", nil, h.Env)
			return
		}
		params.Limit = limit
	} else {
		params.Limit = federation.DefaultChangeFeedLimit
	}

	// Call service
	result, err := h.Service.GetChanges(r.Context(), params)
	if err != nil {
		switch err {
		case federation.ErrInvalidCursor:
			problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid cursor", err, h.Env)
			return
		case federation.ErrInvalidLimit:
			problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid limit", err, h.Env)
			return
		case federation.ErrInvalidAction:
			problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid action", err, h.Env)
			return
		default:
			problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
			return
		}
	}

	// Format response per Interop Profile §4.3
	response := map[string]any{
		"cursor":      result.Cursor,
		"changes":     result.Changes,
		"next_cursor": result.NextCursor,
	}

	writeJSON(w, http.StatusOK, response, contentTypeFromRequest(r))
}
