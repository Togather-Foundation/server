package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/geocoding"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

// GeocodeEventArgs defines the job arguments for geocoding an event.
type GeocodeEventArgs struct {
	EventID string `json:"event_id"` // ULID of the event to geocode
}

func (GeocodeEventArgs) Kind() string { return JobKindGeocodeEvent }

// EventGeocodingService defines the minimal interface needed for event geocoding jobs.
// This follows the idiomatic Go principle: consumers define interfaces.
type EventGeocodingService interface {
	Geocode(ctx context.Context, query string, countryCodes string) (*geocoding.GeocodeResult, error)
	DefaultCountry() string
}

// GeocodeEventWorker geocodes events that have location/address data but missing coordinates.
// It queries the event and its primary venue, builds an address from available fields,
// calls the geocoding service, and updates the venue with resolved latitude/longitude.
//
// The worker respects rate limits by:
// 1. Running in a dedicated "geocoding" queue with MaxWorkers: 1
// 2. Adding a 1-second sleep after each geocoding attempt as a safety net
//
// Retry behavior:
// - Max 3 attempts with exponential backoff (1min, 5min, 30min)
// - Failures never block event creation (enrichment only)
type GeocodeEventWorker struct {
	river.WorkerDefaults[GeocodeEventArgs]
	Pool             *pgxpool.Pool
	GeocodingService EventGeocodingService
	Logger           *slog.Logger
}

func (GeocodeEventWorker) Kind() string { return JobKindGeocodeEvent }

func (w GeocodeEventWorker) Work(ctx context.Context, job *river.Job[GeocodeEventArgs]) error {
	if w.Pool == nil {
		return fmt.Errorf("database pool not configured")
	}
	if w.GeocodingService == nil {
		return fmt.Errorf("geocoding service not configured")
	}

	logger := w.Logger
	if logger == nil {
		logger = slog.Default()
	}

	eventID := job.Args.EventID
	if eventID == "" {
		return fmt.Errorf("event_id is required")
	}

	logger.Info("starting event geocoding job",
		"event_id", eventID,
		"attempt", job.Attempt,
	)

	// Query event and its primary venue
	const getEventQuery = `
		SELECT 
			e.ulid,
			e.name,
			p.ulid as venue_ulid,
			p.name as venue_name,
			p.street_address,
			p.address_locality,
			p.address_region,
			p.postal_code,
			p.address_country,
			p.latitude,
			p.longitude
		FROM events e
		LEFT JOIN places p ON e.primary_venue_id = p.id
		WHERE e.ulid = $1 AND e.deleted_at IS NULL
	`

	var (
		eventULID     string
		eventName     string
		venueULID     *string
		venueName     *string
		streetAddress *string
		city          *string
		region        *string
		postalCode    *string
		country       *string
		latitude      *float64
		longitude     *float64
	)

	err := w.Pool.QueryRow(ctx, getEventQuery, eventID).Scan(
		&eventULID, &eventName, &venueULID, &venueName,
		&streetAddress, &city, &region, &postalCode, &country,
		&latitude, &longitude,
	)
	if err != nil {
		logger.Warn("failed to query event",
			"event_id", eventID,
			"error", err,
		)
		return fmt.Errorf("query event %s: %w", eventID, err)
	}

	// If event has no primary venue, skip geocoding
	if venueULID == nil {
		logger.Info("event has no primary venue, skipping geocoding",
			"event_id", eventID,
		)
		return nil
	}

	// Check if venue already has coordinates
	if latitude != nil && longitude != nil {
		logger.Info("event venue already has coordinates, skipping geocoding",
			"event_id", eventID,
			"venue_ulid", *venueULID,
			"lat", *latitude,
			"lon", *longitude,
		)
		return nil
	}

	// Build address query from available fields
	addressParts := make([]string, 0, 6)
	// Include venue name when no street address is available, as the venue name
	// is often the best geocoding signal (e.g., "Art Gallery of Ontario, Toronto, ON")
	if (streetAddress == nil || *streetAddress == "") && venueName != nil && *venueName != "" {
		addressParts = append(addressParts, *venueName)
	}
	if streetAddress != nil && *streetAddress != "" {
		addressParts = append(addressParts, *streetAddress)
	}
	if city != nil && *city != "" {
		addressParts = append(addressParts, *city)
	}
	if region != nil && *region != "" {
		addressParts = append(addressParts, *region)
	}
	if postalCode != nil && *postalCode != "" {
		addressParts = append(addressParts, *postalCode)
	}
	if country != nil && *country != "" {
		addressParts = append(addressParts, *country)
	}

	if len(addressParts) == 0 {
		logger.Warn("event venue has no address fields to geocode",
			"event_id", eventID,
			"venue_ulid", *venueULID,
			"venue_name", venueName,
		)
		return fmt.Errorf("event %s venue has no address fields", eventID)
	}

	query := strings.Join(addressParts, ", ")

	// Determine country codes for geocoding (default from service config)
	countryCodes := w.GeocodingService.DefaultCountry()
	if country != nil && *country != "" {
		// Map country names to ISO codes (simplified, could be extended)
		switch strings.ToLower(*country) {
		case "canada", "ca":
			countryCodes = "ca"
		case "united states", "usa", "us":
			countryCodes = "us"
		case "united kingdom", "uk", "gb":
			countryCodes = "gb"
		default:
			// Use as-is if already looks like an ISO code
			if len(*country) == 2 {
				countryCodes = strings.ToLower(*country)
			}
		}
	}

	logger.Info("geocoding event venue",
		"event_id", eventID,
		"venue_ulid", *venueULID,
		"query", query,
		"country_codes", countryCodes,
	)

	// Call geocoding service
	result, err := w.GeocodingService.Geocode(ctx, query, countryCodes)

	// Add 1-second sleep after API call as rate limit safety net
	time.Sleep(1 * time.Second)

	if err != nil {
		logger.Warn("event venue geocoding failed",
			"event_id", eventID,
			"venue_ulid", *venueULID,
			"query", query,
			"attempt", job.Attempt,
			"error", err,
		)
		return fmt.Errorf("geocode event %s venue: %w", eventID, err)
	}

	logger.Info("event venue geocoding successful",
		"event_id", eventID,
		"venue_ulid", *venueULID,
		"lat", result.Latitude,
		"lon", result.Longitude,
		"display_name", result.DisplayName,
		"source", result.Source,
	)

	// Update venue (place) with resolved coordinates
	// Note: geo_point is a GENERATED ALWAYS column computed from latitude/longitude,
	// so we only need to set the coordinate values.
	const updatePlaceQuery = `
		UPDATE places
		SET latitude = $1, longitude = $2
		WHERE ulid = $3
	`

	_, err = w.Pool.Exec(ctx, updatePlaceQuery, result.Latitude, result.Longitude, *venueULID)
	if err != nil {
		logger.Error("failed to update event venue with coordinates",
			"event_id", eventID,
			"venue_ulid", *venueULID,
			"lat", result.Latitude,
			"lon", result.Longitude,
			"error", err,
		)
		return fmt.Errorf("update event %s venue: %w", eventID, err)
	}

	logger.Info("event geocoding job completed",
		"event_id", eventID,
		"venue_ulid", *venueULID,
		"lat", result.Latitude,
		"lon", result.Longitude,
	)

	return nil
}
