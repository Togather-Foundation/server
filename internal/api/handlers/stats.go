package handlers

import (
	"net/http"
	"os"
	"time"

	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgtype"
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

	// Entity counts
	TotalEvents        int64 `json:"total_events"`
	TotalPlaces        int64 `json:"total_places"`
	TotalOrganizations int64 `json:"total_organizations"`
	TotalSources       int64 `json:"total_sources"`
	TotalUsers         int64 `json:"total_users"`

	// Event lifecycle breakdown
	PublishedEvents int64 `json:"published_events"`
	PendingEvents   int64 `json:"pending_events"`

	// Time-based event metrics
	EventsLast7Days  int64  `json:"events_created_last_7_days"`
	EventsLast30Days int64  `json:"events_created_last_30_days"`
	UpcomingEvents   int64  `json:"upcoming_events"`
	PastEvents       int64  `json:"past_events"`
	OldestEventDate  string `json:"oldest_event_date,omitempty"`
	NewestEventDate  string `json:"newest_event_date,omitempty"`

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

	// Get entity counts (exclude soft-deleted records)
	totalEvents, err := h.queries.CountAllEvents(ctx)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to retrieve statistics", err, h.env)
		return
	}

	totalPlaces, err := h.queries.CountAllPlaces(ctx)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to retrieve statistics", err, h.env)
		return
	}

	totalOrgs, err := h.queries.CountAllOrganizations(ctx)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to retrieve statistics", err, h.env)
		return
	}

	totalSources, err := h.queries.CountAllSources(ctx)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to retrieve statistics", err, h.env)
		return
	}

	totalUsers, err := h.queries.CountAllUsers(ctx)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to retrieve statistics", err, h.env)
		return
	}

	// Get event lifecycle breakdown
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

	// Get time-based event metrics
	sevenDaysAgo := pgtype.Timestamptz{Time: time.Now().AddDate(0, 0, -7), Valid: true}
	eventsLast7Days, err := h.queries.CountEventsCreatedSince(ctx, sevenDaysAgo)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to retrieve statistics", err, h.env)
		return
	}

	thirtyDaysAgo := pgtype.Timestamptz{Time: time.Now().AddDate(0, 0, -30), Valid: true}
	eventsLast30Days, err := h.queries.CountEventsCreatedSince(ctx, thirtyDaysAgo)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to retrieve statistics", err, h.env)
		return
	}

	upcomingEvents, err := h.queries.CountUpcomingEvents(ctx)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to retrieve statistics", err, h.env)
		return
	}

	pastEvents, err := h.queries.CountPastEvents(ctx)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to retrieve statistics", err, h.env)
		return
	}

	// Get event date range
	dateRange, err := h.queries.GetEventDateRange(ctx)
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

	// Format date range (handle null values)
	var oldestDate, newestDate string
	if dateRange.OldestEventDate != nil {
		if t, ok := dateRange.OldestEventDate.(time.Time); ok {
			oldestDate = t.Format(time.RFC3339)
		}
	}
	if dateRange.NewestEventDate != nil {
		if t, ok := dateRange.NewestEventDate.(time.Time); ok {
			newestDate = t.Format(time.RFC3339)
		}
	}

	// Build response
	stats := StatsResponse{
		Status:    "healthy",
		Version:   h.version,
		GitCommit: h.gitCommit,
		Uptime:    uptime,
		Slot:      slot,

		// Entity counts
		TotalEvents:        totalEvents,
		TotalPlaces:        totalPlaces,
		TotalOrganizations: totalOrgs,
		TotalSources:       totalSources,
		TotalUsers:         totalUsers,

		// Event lifecycle
		PublishedEvents: publishedCount,
		PendingEvents:   pendingCount,

		// Time-based metrics
		EventsLast7Days:  eventsLast7Days,
		EventsLast30Days: eventsLast30Days,
		UpcomingEvents:   upcomingEvents,
		PastEvents:       pastEvents,
		OldestEventDate:  oldestDate,
		NewestEventDate:  newestDate,

		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	// Set cache headers for reasonable caching (5 minutes)
	w.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(w, http.StatusOK, stats, contentTypeFromRequest(r))
}
