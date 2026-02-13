package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// GeocodingCacheRepository manages geocoding cache operations.
type GeocodingCacheRepository struct {
	pool *pgxpool.Pool
	tx   pgx.Tx
}

// NewGeocodingCacheRepository creates a new geocoding cache repository.
func NewGeocodingCacheRepository(pool *pgxpool.Pool) *GeocodingCacheRepository {
	return &GeocodingCacheRepository{pool: pool}
}

// CachedGeocode represents a cached forward geocoding result.
type CachedGeocode struct {
	ID              int64
	QueryNormalized string
	CountryCodes    string
	Latitude        float64
	Longitude       float64
	DisplayName     string
	PlaceType       string
	OSMID           *int64
	RawResponse     []byte // JSONB
	Source          string
	HitCount        int
	CreatedAt       time.Time
	ExpiresAt       *time.Time
}

// CachedReverse represents a cached reverse geocoding result.
type CachedReverse struct {
	ID              int64
	Latitude        float64
	Longitude       float64
	DisplayName     string
	AddressRoad     *string
	AddressSuburb   *string
	AddressCity     *string
	AddressState    *string
	AddressPostcode *string
	AddressCountry  *string
	OSMID           *int64
	RawResponse     []byte // JSONB
	HitCount        int
	CreatedAt       time.Time
	ExpiresAt       *time.Time
}

// GeocodingFailure represents a tracked geocoding failure.
type GeocodingFailure struct {
	ID              int64
	QueryNormalized string
	CountryCodes    string
	FailureReason   string
	AttemptCount    int
	RetryAfter      *time.Time
	CreatedAt       time.Time
	ExpiresAt       *time.Time
}

// NormalizeQuery normalizes a geocoding query for cache lookups.
// Converts to lowercase, trims whitespace, and collapses multiple spaces.
func NormalizeQuery(q string) string {
	q = strings.ToLower(strings.TrimSpace(q))
	// Collapse multiple spaces into one
	words := strings.Fields(q)
	return strings.Join(words, " ")
}

// GetCachedGeocode retrieves a cached forward geocoding result.
// Returns nil if not found or expired.
func (r *GeocodingCacheRepository) GetCachedGeocode(ctx context.Context, queryNormalized, countryCodes string) (*CachedGeocode, error) {
	queryer := r.queryer()

	const query = `
		SELECT id, query_normalized, country_codes, latitude, longitude, 
		       display_name, place_type, osm_id, raw_response, source, 
		       hit_count, created_at, expires_at
		FROM geocoding_cache
		WHERE query_normalized = $1 
		  AND country_codes = $2
		  AND (expires_at IS NULL OR expires_at > NOW())
		LIMIT 1
	`

	var cached CachedGeocode
	var expiresAt sql.NullTime

	err := queryer.QueryRow(ctx, query, queryNormalized, countryCodes).Scan(
		&cached.ID,
		&cached.QueryNormalized,
		&cached.CountryCodes,
		&cached.Latitude,
		&cached.Longitude,
		&cached.DisplayName,
		&cached.PlaceType,
		&cached.OSMID,
		&cached.RawResponse,
		&cached.Source,
		&cached.HitCount,
		&cached.CreatedAt,
		&expiresAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // Not found
		}
		return nil, fmt.Errorf("get cached geocode: %w", err)
	}

	if expiresAt.Valid {
		cached.ExpiresAt = &expiresAt.Time
	}

	return &cached, nil
}

// CacheGeocode stores a forward geocoding result in the cache.
// Uses upsert (ON CONFLICT) to update existing entries.
func (r *GeocodingCacheRepository) CacheGeocode(ctx context.Context, result CachedGeocode) error {
	queryer := r.queryer()

	const query = `
		INSERT INTO geocoding_cache (
			query_normalized, country_codes, latitude, longitude,
			display_name, place_type, osm_id, raw_response, source,
			hit_count, created_at, expires_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
		)
		ON CONFLICT (query_normalized, country_codes)
		DO UPDATE SET
			latitude = EXCLUDED.latitude,
			longitude = EXCLUDED.longitude,
			display_name = EXCLUDED.display_name,
			place_type = EXCLUDED.place_type,
			osm_id = EXCLUDED.osm_id,
			raw_response = EXCLUDED.raw_response,
			expires_at = EXCLUDED.expires_at
	`

	expiresAt := result.ExpiresAt
	if expiresAt == nil {
		defaultExpiry := time.Now().Add(30 * 24 * time.Hour) // 30 days
		expiresAt = &defaultExpiry
	}

	_, err := queryer.Exec(ctx, query,
		result.QueryNormalized,
		result.CountryCodes,
		result.Latitude,
		result.Longitude,
		result.DisplayName,
		result.PlaceType,
		result.OSMID,
		result.RawResponse,
		result.Source,
		result.HitCount,
		time.Now(),
		expiresAt,
	)

	if err != nil {
		return fmt.Errorf("cache geocode: %w", err)
	}

	return nil
}

// GetCachedReverse retrieves a cached reverse geocoding result.
// Uses ST_DWithin with 100m radius to find nearby cached results.
// Returns nil if not found or expired.
func (r *GeocodingCacheRepository) GetCachedReverse(ctx context.Context, lat, lon float64) (*CachedReverse, error) {
	queryer := r.queryer()

	const query = `
		SELECT id, latitude, longitude, display_name,
		       address_road, address_suburb, address_city, address_state,
		       address_postcode, address_country, osm_id, raw_response,
		       hit_count, created_at, expires_at
		FROM reverse_geocoding_cache
		WHERE ST_DWithin(
			geo_point::geography,
			ST_SetSRID(ST_MakePoint($2, $1), 4326)::geography,
			100  -- 100 meters
		)
		AND (expires_at IS NULL OR expires_at > NOW())
		ORDER BY ST_Distance(
			geo_point::geography,
			ST_SetSRID(ST_MakePoint($2, $1), 4326)::geography
		)
		LIMIT 1
	`

	var cached CachedReverse
	var expiresAt sql.NullTime

	err := queryer.QueryRow(ctx, query, lat, lon).Scan(
		&cached.ID,
		&cached.Latitude,
		&cached.Longitude,
		&cached.DisplayName,
		&cached.AddressRoad,
		&cached.AddressSuburb,
		&cached.AddressCity,
		&cached.AddressState,
		&cached.AddressPostcode,
		&cached.AddressCountry,
		&cached.OSMID,
		&cached.RawResponse,
		&cached.HitCount,
		&cached.CreatedAt,
		&expiresAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // Not found
		}
		return nil, fmt.Errorf("get cached reverse: %w", err)
	}

	if expiresAt.Valid {
		cached.ExpiresAt = &expiresAt.Time
	}

	return &cached, nil
}

// CacheReverse stores a reverse geocoding result in the cache.
func (r *GeocodingCacheRepository) CacheReverse(ctx context.Context, result CachedReverse) error {
	queryer := r.queryer()

	const query = `
		INSERT INTO reverse_geocoding_cache (
			latitude, longitude, geo_point, display_name,
			address_road, address_suburb, address_city, address_state,
			address_postcode, address_country, osm_id, raw_response,
			hit_count, created_at, expires_at
		) VALUES (
			$1, $2, ST_SetSRID(ST_MakePoint($3, $1), 4326), $4,
			$5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15
		)
	`

	expiresAt := result.ExpiresAt
	if expiresAt == nil {
		defaultExpiry := time.Now().Add(30 * 24 * time.Hour) // 30 days
		expiresAt = &defaultExpiry
	}

	_, err := queryer.Exec(ctx, query,
		result.Latitude,
		result.Longitude,
		result.Longitude, // lon for ST_MakePoint (x, y)
		result.DisplayName,
		result.AddressRoad,
		result.AddressSuburb,
		result.AddressCity,
		result.AddressState,
		result.AddressPostcode,
		result.AddressCountry,
		result.OSMID,
		result.RawResponse,
		result.HitCount,
		time.Now(),
		expiresAt,
	)

	if err != nil {
		return fmt.Errorf("cache reverse: %w", err)
	}

	return nil
}

// RecordFailure records a geocoding failure to avoid repeated failed lookups.
func (r *GeocodingCacheRepository) RecordFailure(ctx context.Context, queryNormalized, countryCodes, reason string) error {
	queryer := r.queryer()

	const query = `
		INSERT INTO geocoding_failures (
			query_normalized, country_codes, failure_reason,
			attempt_count, created_at, expires_at
		) VALUES (
			$1, $2, $3, 1, NOW(), NOW() + INTERVAL '7 days'
		)
		ON CONFLICT (query_normalized, country_codes)
		DO UPDATE SET
			failure_reason = EXCLUDED.failure_reason,
			attempt_count = geocoding_failures.attempt_count + 1,
			expires_at = NOW() + INTERVAL '7 days'
	`

	_, err := queryer.Exec(ctx, query, queryNormalized, countryCodes, reason)
	if err != nil {
		return fmt.Errorf("record failure: %w", err)
	}

	return nil
}

// GetRecentFailure checks if a query has recently failed.
// Returns nil if no recent failure found or failure has expired.
func (r *GeocodingCacheRepository) GetRecentFailure(ctx context.Context, queryNormalized, countryCodes string) (*GeocodingFailure, error) {
	queryer := r.queryer()

	const query = `
		SELECT id, query_normalized, country_codes, failure_reason,
		       attempt_count, retry_after, created_at, expires_at
		FROM geocoding_failures
		WHERE query_normalized = $1
		  AND country_codes = $2
		  AND (expires_at IS NULL OR expires_at > NOW())
		LIMIT 1
	`

	var failure GeocodingFailure
	var retryAfter, expiresAt sql.NullTime

	err := queryer.QueryRow(ctx, query, queryNormalized, countryCodes).Scan(
		&failure.ID,
		&failure.QueryNormalized,
		&failure.CountryCodes,
		&failure.FailureReason,
		&failure.AttemptCount,
		&retryAfter,
		&failure.CreatedAt,
		&expiresAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // No recent failure
		}
		return nil, fmt.Errorf("get recent failure: %w", err)
	}

	if retryAfter.Valid {
		failure.RetryAfter = &retryAfter.Time
	}
	if expiresAt.Valid {
		failure.ExpiresAt = &expiresAt.Time
	}

	return &failure, nil
}

// IncrementHitCount increments the hit_count for a cache entry.
func (r *GeocodingCacheRepository) IncrementHitCount(ctx context.Context, id int64, table string) error {
	queryer := r.queryer()

	// Validate table name to prevent SQL injection
	switch table {
	case "geocoding_cache", "reverse_geocoding_cache":
	default:
		return fmt.Errorf("invalid table name: %s", table)
	}

	query := fmt.Sprintf("UPDATE %s SET hit_count = hit_count + 1 WHERE id = $1", table)

	_, err := queryer.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("increment hit count: %w", err)
	}

	return nil
}

// queryer returns the active queryer (transaction or pool).
type geocodingCacheQueryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (r *GeocodingCacheRepository) queryer() geocodingCacheQueryer {
	if r.tx != nil {
		return r.tx
	}
	return r.pool
}
