package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/jobs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

// AdminGeocodingHandler handles admin geocoding operations.
type AdminGeocodingHandler struct {
	Pool        *pgxpool.Pool
	RiverClient *river.Client[pgx.Tx]
	Logger      *slog.Logger
	Env         string
}

// NewAdminGeocodingHandler creates a new admin geocoding handler.
func NewAdminGeocodingHandler(pool *pgxpool.Pool, riverClient *river.Client[pgx.Tx], logger *slog.Logger, env string) *AdminGeocodingHandler {
	return &AdminGeocodingHandler{
		Pool:        pool,
		RiverClient: riverClient,
		Logger:      logger,
		Env:         env,
	}
}

// BackfillResponse represents the response from the geocoding backfill endpoint.
type BackfillResponse struct {
	PlacesEnqueued    int    `json:"places_enqueued"`
	EventsEnqueued    int    `json:"events_enqueued"`
	EstimatedDuration string `json:"estimated_duration"`
}

// Backfill handles POST /api/v1/admin/geocoding/backfill
// Enqueues geocoding jobs for all places and events with address data but missing coordinates.
func (h *AdminGeocodingHandler) Backfill(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := h.Logger
	if logger == nil {
		logger = slog.Default()
	}

	logger.Info("starting geocoding backfill",
		"remote_addr", r.RemoteAddr,
	)

	// Query places with address but no coordinates
	const placesQuery = `
		SELECT ulid
		FROM places
		WHERE deleted_at IS NULL
		AND latitude IS NULL
		AND longitude IS NULL
		AND (
			street_address IS NOT NULL
			OR address_locality IS NOT NULL
			OR postal_code IS NOT NULL
		)
	`

	placesRows, err := h.Pool.Query(ctx, placesQuery)
	if err != nil {
		logger.Error("failed to query places for backfill",
			"error", err,
		)
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to query places", err, h.Env)
		return
	}
	defer placesRows.Close()

	var placeULIDs []string
	for placesRows.Next() {
		var ulid string
		if err := placesRows.Scan(&ulid); err != nil {
			logger.Warn("failed to scan place ULID", "error", err)
			continue
		}
		placeULIDs = append(placeULIDs, ulid)
	}

	if err := placesRows.Err(); err != nil {
		logger.Error("error iterating places rows", "error", err)
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to process places", err, h.Env)
		return
	}

	// Query events with primary venue that has no coordinates
	const eventsQuery = `
		SELECT DISTINCT e.ulid
		FROM events e
		INNER JOIN places p ON e.primary_venue_id = p.id
		WHERE e.deleted_at IS NULL
		AND p.deleted_at IS NULL
		AND p.latitude IS NULL
		AND p.longitude IS NULL
		AND (
			p.street_address IS NOT NULL
			OR p.address_locality IS NOT NULL
			OR p.postal_code IS NOT NULL
		)
	`

	eventsRows, err := h.Pool.Query(ctx, eventsQuery)
	if err != nil {
		logger.Error("failed to query events for backfill",
			"error", err,
		)
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to query events", err, h.Env)
		return
	}
	defer eventsRows.Close()

	var eventULIDs []string
	for eventsRows.Next() {
		var ulid string
		if err := eventsRows.Scan(&ulid); err != nil {
			logger.Warn("failed to scan event ULID", "error", err)
			continue
		}
		eventULIDs = append(eventULIDs, ulid)
	}

	if err := eventsRows.Err(); err != nil {
		logger.Error("error iterating events rows", "error", err)
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to process events", err, h.Env)
		return
	}

	// Enqueue geocoding jobs for places
	placesEnqueued := 0
	for _, ulid := range placeULIDs {
		_, err := h.RiverClient.Insert(ctx, jobs.GeocodePlaceArgs{PlaceID: ulid}, &river.InsertOpts{
			Queue:       "geocoding",
			MaxAttempts: jobs.GeocodingMaxAttempts,
		})
		if err != nil {
			logger.Warn("failed to enqueue place geocoding job",
				"place_ulid", ulid,
				"error", err,
			)
			continue
		}
		placesEnqueued++
	}

	// Enqueue geocoding jobs for events
	eventsEnqueued := 0
	for _, ulid := range eventULIDs {
		_, err := h.RiverClient.Insert(ctx, jobs.GeocodeEventArgs{EventID: ulid}, &river.InsertOpts{
			Queue:       "geocoding",
			MaxAttempts: jobs.GeocodingMaxAttempts,
		})
		if err != nil {
			logger.Warn("failed to enqueue event geocoding job",
				"event_ulid", ulid,
				"error", err,
			)
			continue
		}
		eventsEnqueued++
	}

	// Calculate estimated duration (1 req/sec + 1s safety sleep = ~2s per job)
	totalJobs := placesEnqueued + eventsEnqueued
	estimatedSeconds := totalJobs * 2
	estimatedDuration := formatDuration(estimatedSeconds)

	logger.Info("geocoding backfill completed",
		"places_enqueued", placesEnqueued,
		"events_enqueued", eventsEnqueued,
		"total_jobs", totalJobs,
		"estimated_duration", estimatedDuration,
	)

	response := BackfillResponse{
		PlacesEnqueued:    placesEnqueued,
		EventsEnqueued:    eventsEnqueued,
		EstimatedDuration: estimatedDuration,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.Error("failed to encode backfill response", "error", err)
	}
}

// formatDuration formats a duration in seconds into a human-readable string.
func formatDuration(seconds int) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	minutes := seconds / 60
	if minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	}
	hours := minutes / 60
	remainingMinutes := minutes % 60
	if remainingMinutes == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh%dm", hours, remainingMinutes)
}
