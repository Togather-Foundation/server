package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/domain/ids"
)

// Series-candidate query-parameter bounds. The detector defaults live in the
// events domain package; these are the accepted request ranges.
const (
	seriesCandidatesMinDays       = 1
	seriesCandidatesMaxDays       = 365
	seriesCandidatesMinLookback   = 0
	seriesCandidatesMaxLookback   = 90
	seriesCandidatesMinEvents     = 2
	seriesCandidatesMaxEvents     = 50
	seriesCandidatesMinLimit      = 1
	seriesCandidatesMaxLimit      = 200
	seriesCandidatesMinOffset     = 0
	seriesCandidatesMaxOffset     = 10000
	seriesCandidatesDefaultLook   = events.DefaultSeriesCandidateLookbackDays
	seriesCandidatesDefaultMinEv  = events.DefaultSeriesCandidateMinEvents
	seriesCandidatesDefaultLimit  = events.DefaultSeriesCandidateLimit
	seriesCandidatesDefaultOffset = 0
)

type seriesCandidateVenueResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type seriesCandidateMemberResponse struct {
	ULID            string `json:"ulid"`
	Name            string `json:"name"`
	StartDate       string `json:"start_date"`
	OccurrenceCount int    `json:"occurrence_count"`
	LifecycleState  string `json:"lifecycle_state"`
}

type seriesCandidateResponse struct {
	SuggestedCanonicalULID   string                          `json:"suggested_canonical_ulid"`
	Name                     string                          `json:"name"`
	Venue                    seriesCandidateVenueResponse    `json:"venue"`
	DayOfWeek                int                             `json:"day_of_week"`
	LocalStartTime           string                          `json:"local_start_time"`
	EventCount               int                             `json:"event_count"`
	OccurrenceCount          int                             `json:"occurrence_count"`
	FirstDate                string                          `json:"first_date"`
	LastDate                 string                          `json:"last_date"`
	Members                  []seriesCandidateMemberResponse `json:"members"`
	DistinctDescriptionCount int                             `json:"distinct_description_count"`
	ContentVaries            bool                            `json:"content_varies"`
}

type seriesCandidatesResponse struct {
	Items         []seriesCandidateResponse `json:"items"`
	TotalClusters int                       `json:"total_clusters"`
	Truncated     bool                      `json:"truncated"`
}

// GetSeriesCandidates handles GET /api/v1/admin/events/series-candidates.
// It is a read-only discovery surface: it finds conservative weekly-series
// clusters so an agent can consolidate N same-programme events into one event
// with N occurrences. It never writes events, occurrences, or review rows.
func (h *AdminHandler) GetSeriesCandidates(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.AdminService == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	query := r.URL.Query()

	days, err := parseSeriesCandidateInt(query, "days", events.DefaultSeriesCandidateDays, seriesCandidatesMinDays, seriesCandidatesMaxDays)
	if err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", err, h.Env)
		return
	}
	lookbackDays, err := parseSeriesCandidateInt(query, "lookback_days", seriesCandidatesDefaultLook, seriesCandidatesMinLookback, seriesCandidatesMaxLookback)
	if err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", err, h.Env)
		return
	}
	minEvents, err := parseSeriesCandidateInt(query, "min_events", seriesCandidatesDefaultMinEv, seriesCandidatesMinEvents, seriesCandidatesMaxEvents)
	if err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", err, h.Env)
		return
	}
	limit, err := parseSeriesCandidateInt(query, "limit", seriesCandidatesDefaultLimit, seriesCandidatesMinLimit, seriesCandidatesMaxLimit)
	if err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", err, h.Env)
		return
	}
	offset, err := parseSeriesCandidateInt(query, "offset", seriesCandidatesDefaultOffset, seriesCandidatesMinOffset, seriesCandidatesMaxOffset)
	if err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", err, h.Env)
		return
	}

	venueID := strings.TrimSpace(query.Get("venue_id"))
	if venueID != "" {
		if err := ids.ValidateULID(venueID); err != nil {
			problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", FilterError{Field: "venue_id", Message: "invalid ULID"}, h.Env)
			return
		}
		// ULIDs are stored uppercase; accept lowercase input but normalize it
		// before querying so it does not silently match zero venues.
		venueID = strings.ToUpper(venueID)
	}

	source := strings.TrimSpace(query.Get("source"))

	result, err := h.AdminService.FindSeriesCandidates(r.Context(), events.SeriesCandidatesParams{
		Now:          time.Now(),
		Days:         days,
		LookbackDays: lookbackDays,
		MinEvents:    minEvents,
		VenueULID:    venueID,
		Source:       source,
		Offset:       offset,
		Limit:        limit,
	})
	if err != nil {
		if errors.Is(err, events.ErrSourceNotFound) || errors.Is(err, events.ErrSourceAmbiguous) {
			// Surface the resolver's message (accepted forms, ULID hint) instead
			// of replacing it with a generic one.
			problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", err, h.Env,
				problem.WithDetail(err.Error()))
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", fmt.Errorf("find series candidates: %w", err), h.Env)
		return
	}

	items := make([]seriesCandidateResponse, 0, len(result.Items))
	for _, cluster := range result.Items {
		members := make([]seriesCandidateMemberResponse, 0, len(cluster.Members))
		for _, member := range cluster.Members {
			members = append(members, seriesCandidateMemberResponse{
				ULID:            member.ULID,
				Name:            member.Name,
				StartDate:       member.StartDate,
				OccurrenceCount: member.OccurrenceCount,
				LifecycleState:  member.LifecycleState,
			})
		}
		items = append(items, seriesCandidateResponse{
			SuggestedCanonicalULID:   cluster.SuggestedCanonicalULID,
			Name:                     cluster.Name,
			Venue:                    seriesCandidateVenueResponse{ID: cluster.VenueID, Name: cluster.VenueName},
			DayOfWeek:                cluster.DayOfWeek,
			LocalStartTime:           cluster.LocalStartTime,
			EventCount:               cluster.EventCount,
			OccurrenceCount:          cluster.OccurrenceCount,
			FirstDate:                cluster.FirstDate,
			LastDate:                 cluster.LastDate,
			Members:                  members,
			DistinctDescriptionCount: cluster.DistinctDescriptionCount,
			ContentVaries:            cluster.ContentVaries,
		})
	}

	writeJSON(w, http.StatusOK, seriesCandidatesResponse{
		Items:         items,
		TotalClusters: result.TotalClusters,
		Truncated:     result.Truncated,
	}, "application/json")
}

func parseSeriesCandidateInt(query url.Values, name string, defaultValue, minValue, maxValue int) (int, error) {
	raw := strings.TrimSpace(query.Get(name))
	if raw == "" {
		return defaultValue, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < minValue || value > maxValue {
		return 0, FilterError{
			Field:   name,
			Message: fmt.Sprintf("must be an integer between %d and %d", minValue, maxValue),
		}
	}
	return value, nil
}
