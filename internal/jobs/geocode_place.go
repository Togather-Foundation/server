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

// GeocodePlaceArgs defines the job arguments for geocoding a place.
type GeocodePlaceArgs struct {
	PlaceID string `json:"place_id"` // ULID of the place to geocode
}

func (GeocodePlaceArgs) Kind() string { return JobKindGeocodePlace }

// GeocodePlaceWorker geocodes places that have address data but missing coordinates.
// It queries the place, builds an address from available fields, calls the geocoding
// service, and updates the place with resolved latitude/longitude.
//
// The worker respects rate limits by:
// 1. Running in a dedicated "geocoding" queue with MaxWorkers: 1
// 2. Adding a 1-second sleep after each geocoding attempt as a safety net
//
// Retry behavior:
// - Max 3 attempts with exponential backoff (1min, 5min, 30min)
// - Failures never block place creation (enrichment only)
type GeocodePlaceWorker struct {
	river.WorkerDefaults[GeocodePlaceArgs]
	Pool             *pgxpool.Pool
	GeocodingService *geocoding.GeocodingService
	Logger           *slog.Logger
}

func (GeocodePlaceWorker) Kind() string { return JobKindGeocodePlace }

func (w GeocodePlaceWorker) Work(ctx context.Context, job *river.Job[GeocodePlaceArgs]) error {
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

	placeID := job.Args.PlaceID
	if placeID == "" {
		return fmt.Errorf("place_id is required")
	}

	logger.Info("starting place geocoding job",
		"place_id", placeID,
		"attempt", job.Attempt,
	)

	// Query place by ULID
	const getPlaceQuery = `
		SELECT ulid, name, street_address, city, region, postal_code, country, latitude, longitude
		FROM places
		WHERE ulid = $1 AND deleted_at IS NULL
	`

	var (
		ulid          string
		name          string
		streetAddress *string
		city          *string
		region        *string
		postalCode    *string
		country       *string
		latitude      *float64
		longitude     *float64
	)

	err := w.Pool.QueryRow(ctx, getPlaceQuery, placeID).Scan(
		&ulid, &name, &streetAddress, &city, &region, &postalCode, &country, &latitude, &longitude,
	)
	if err != nil {
		logger.Warn("failed to query place",
			"place_id", placeID,
			"error", err,
		)
		return fmt.Errorf("query place %s: %w", placeID, err)
	}

	// Check if place already has coordinates
	if latitude != nil && longitude != nil {
		logger.Info("place already has coordinates, skipping geocoding",
			"place_id", placeID,
			"lat", *latitude,
			"lon", *longitude,
		)
		return nil
	}

	// Build address query from available fields
	addressParts := make([]string, 0, 5)
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
		logger.Warn("place has no address fields to geocode",
			"place_id", placeID,
			"name", name,
		)
		return fmt.Errorf("place %s has no address fields", placeID)
	}

	query := strings.Join(addressParts, ", ")

	// Determine country codes for geocoding (default to "ca" if not specified)
	countryCodes := "ca"
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

	logger.Info("geocoding place",
		"place_id", placeID,
		"query", query,
		"country_codes", countryCodes,
	)

	// Call geocoding service
	result, err := w.GeocodingService.Geocode(ctx, query, countryCodes)

	// Add 1-second sleep after API call as rate limit safety net
	time.Sleep(1 * time.Second)

	if err != nil {
		logger.Warn("place geocoding failed",
			"place_id", placeID,
			"query", query,
			"attempt", job.Attempt,
			"error", err,
		)
		return fmt.Errorf("geocode place %s: %w", placeID, err)
	}

	logger.Info("place geocoding successful",
		"place_id", placeID,
		"lat", result.Latitude,
		"lon", result.Longitude,
		"display_name", result.DisplayName,
		"source", result.Source,
	)

	// Update place with resolved coordinates
	const updatePlaceQuery = `
		UPDATE places
		SET latitude = $1, longitude = $2, geo_point = ST_SetSRID(ST_MakePoint($2, $1), 4326)
		WHERE ulid = $3
	`

	_, err = w.Pool.Exec(ctx, updatePlaceQuery, result.Latitude, result.Longitude, placeID)
	if err != nil {
		logger.Error("failed to update place with coordinates",
			"place_id", placeID,
			"lat", result.Latitude,
			"lon", result.Longitude,
			"error", err,
		)
		return fmt.Errorf("update place %s: %w", placeID, err)
	}

	logger.Info("place geocoding job completed",
		"place_id", placeID,
		"lat", result.Latitude,
		"lon", result.Longitude,
	)

	return nil
}
