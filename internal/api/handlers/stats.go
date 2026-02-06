package handlers

import (
	"net/http"
	"os"
	"time"

	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
)

// StatsHandler provides server statistics for monitoring and landing page widgets
type StatsHandler struct {
	queries   postgres.Querier
	version   string
	gitCommit string
	startTime time.Time
	env       string
}

// NewStatsHandler creates a new stats handler
func NewStatsHandler(queries postgres.Querier, version, gitCommit string, startTime time.Time, env string) *StatsHandler {
	return &StatsHandler{
		queries:   queries,
		version:   version,
		gitCommit: gitCommit,
		startTime: startTime,
		env:       env,
	}
}

// StatsResponse represents the server statistics
type StatsResponse struct {
	// Server information
	Status    string `json:"status"`
	Version   string `json:"version"`
	GitCommit string `json:"git_commit"`
	Uptime    int64  `json:"uptime_seconds"`
	Slot      string `json:"slot,omitempty"`

	// Event statistics
	TotalEvents     int64 `json:"total_events"`
	PublishedEvents int64 `json:"published_events"`
	PendingEvents   int64 `json:"pending_events"`

	// Timestamp
	Timestamp string `json:"timestamp"`
}

// GetStats handles GET /api/v1/stats
// Returns server statistics including event counts, health status, and uptime
// This endpoint is public (no authentication required) for use by landing pages and monitoring tools
func (h *StatsHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.queries == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.env)
		return
	}

	ctx := r.Context()

	// Get event counts
	totalCount, err := h.queries.CountAllEvents(ctx)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to retrieve statistics", err, h.env)
		return
	}

	publishedCount, err := h.queries.CountEventsByLifecycleState(ctx, "published")
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to retrieve statistics", err, h.env)
		return
	}

	pendingCount, err := h.queries.CountEventsByLifecycleState(ctx, "draft")
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to retrieve statistics", err, h.env)
		return
	}

	// Get deployment slot identifier (for blue-green deployments)
	slot := os.Getenv("DEPLOYMENT_SLOT")
	if slot == "" {
		slot = os.Getenv("SLOT")
	}

	// Calculate uptime
	uptime := int64(time.Since(h.startTime).Seconds())

	// Build response
	stats := StatsResponse{
		Status:          "healthy",
		Version:         h.version,
		GitCommit:       h.gitCommit,
		Uptime:          uptime,
		Slot:            slot,
		TotalEvents:     totalCount,
		PublishedEvents: publishedCount,
		PendingEvents:   pendingCount,
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
	}

	// Set cache headers for reasonable caching (5 minutes)
	w.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(w, http.StatusOK, stats, contentTypeFromRequest(r))
}
