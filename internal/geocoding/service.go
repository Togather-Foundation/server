package geocoding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/Togather-Foundation/server/internal/geocoding/nominatim"
	"github.com/Togather-Foundation/server/internal/metrics"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/rs/zerolog"
)

// GeocodingService orchestrates geocoding operations using cache and Nominatim API.
type GeocodingService struct {
	client *nominatim.Client
	cache  *postgres.GeocodingCacheRepository
	logger zerolog.Logger
}

// NewGeocodingService creates a new geocoding service.
func NewGeocodingService(
	client *nominatim.Client,
	cache *postgres.GeocodingCacheRepository,
	logger zerolog.Logger,
) *GeocodingService {
	return &GeocodingService{
		client: client,
		cache:  cache,
		logger: logger,
	}
}

// GeocodeResult represents the result of a geocoding operation.
type GeocodeResult struct {
	Latitude    float64
	Longitude   float64
	DisplayName string
	Source      string // "cache" or "nominatim"
	Cached      bool
}

// ErrGeocodingFailed is returned when geocoding fails after all retries.
var ErrGeocodingFailed = errors.New("geocoding failed")

// ErrNoResults is returned when Nominatim returns no results for a query.
var ErrNoResults = errors.New("no geocoding results found")

// Geocode performs forward geocoding (query -> coordinates).
// It checks the cache first, then calls Nominatim if needed, and caches the result.
func (s *GeocodingService) Geocode(ctx context.Context, query string, countryCodes string) (*GeocodeResult, error) {
	// Normalize query for cache lookup
	normalized := postgres.NormalizeQuery(query)
	if normalized == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}

	// Default country codes if not provided
	if countryCodes == "" {
		countryCodes = "ca"
	}

	// Check for recent failures to avoid hammering Nominatim
	failure, err := s.cache.GetRecentFailure(ctx, normalized, countryCodes)
	if err != nil {
		s.logger.Warn().Err(err).Str("query", query).Msg("failed to check geocoding failure cache")
	}
	if failure != nil {
		metrics.GeocodingRequestsTotal.WithLabelValues("forward", "failure_cache").Inc()
		return nil, fmt.Errorf("%w: %s (cached failure, try again later)", ErrGeocodingFailed, failure.FailureReason)
	}

	// Check cache for existing result
	cached, err := s.cache.GetCachedGeocode(ctx, normalized, countryCodes)
	if err != nil {
		s.logger.Warn().Err(err).Str("query", query).Msg("failed to check geocoding cache")
	}

	if cached != nil {
		// Cache hit
		metrics.GeocodingCacheHitsTotal.WithLabelValues("forward").Inc()
		metrics.GeocodingRequestsTotal.WithLabelValues("forward", "cache").Inc()

		// Increment hit count asynchronously (best effort)
		go func() {
			// Use background context to avoid cancellation
			bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := s.cache.IncrementHitCount(bgCtx, cached.ID, "geocoding_cache"); err != nil {
				s.logger.Warn().Err(err).Int64("id", cached.ID).Msg("failed to increment cache hit count")
			}
		}()

		s.logger.Debug().
			Str("query", query).
			Float64("lat", cached.Latitude).
			Float64("lon", cached.Longitude).
			Msg("geocoding cache hit")

		return &GeocodeResult{
			Latitude:    cached.Latitude,
			Longitude:   cached.Longitude,
			DisplayName: cached.DisplayName,
			Source:      "cache",
			Cached:      true,
		}, nil
	}

	// Cache miss - call Nominatim API
	metrics.GeocodingCacheMissesTotal.WithLabelValues("forward").Inc()

	s.logger.Debug().
		Str("query", query).
		Str("country_codes", countryCodes).
		Msg("geocoding cache miss, calling Nominatim")

	startTime := time.Now()
	results, err := s.client.Search(ctx, query, nominatim.SearchOptions{
		CountryCodes: countryCodes,
		Limit:        1,
	})
	latency := time.Since(startTime).Seconds()

	metrics.GeocodingNominatimLatency.WithLabelValues("search").Observe(latency)

	if err != nil {
		metrics.GeocodingNominatimRequestsTotal.WithLabelValues("search", "error").Inc()
		metrics.GeocodingFailuresTotal.WithLabelValues("forward", "error").Inc()

		s.logger.Error().
			Err(err).
			Str("query", query).
			Dur("latency", time.Since(startTime)).
			Msg("nominatim search failed")

		// Record failure in cache to avoid repeated attempts
		cacheErr := s.cache.RecordFailure(ctx, normalized, countryCodes, err.Error())
		if cacheErr != nil {
			s.logger.Warn().Err(cacheErr).Str("query", query).Msg("failed to cache geocoding failure")
		}

		return nil, fmt.Errorf("%w: %v", ErrGeocodingFailed, err)
	}

	if len(results) == 0 {
		metrics.GeocodingNominatimRequestsTotal.WithLabelValues("search", "success").Inc()
		metrics.GeocodingFailuresTotal.WithLabelValues("forward", "not_found").Inc()

		s.logger.Warn().
			Str("query", query).
			Str("country_codes", countryCodes).
			Msg("nominatim returned no results")

		// Record failure (no results) in cache
		cacheErr := s.cache.RecordFailure(ctx, normalized, countryCodes, "no results found")
		if cacheErr != nil {
			s.logger.Warn().Err(cacheErr).Str("query", query).Msg("failed to cache no-results failure")
		}

		return nil, fmt.Errorf("%w for query: %s", ErrNoResults, query)
	}

	metrics.GeocodingNominatimRequestsTotal.WithLabelValues("search", "success").Inc()
	metrics.GeocodingRequestsTotal.WithLabelValues("forward", "nominatim").Inc()

	result := results[0]

	// Parse lat/lon from string
	lat, err := strconv.ParseFloat(result.Lat, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid latitude in nominatim result: %w", err)
	}
	lon, err := strconv.ParseFloat(result.Lon, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid longitude in nominatim result: %w", err)
	}

	s.logger.Info().
		Str("query", query).
		Float64("lat", lat).
		Float64("lon", lon).
		Str("display_name", result.DisplayName).
		Dur("latency", time.Since(startTime)).
		Msg("geocoding successful")

	// Cache the result
	rawJSON, _ := json.Marshal(result)
	cacheEntry := postgres.CachedGeocode{
		QueryNormalized: normalized,
		CountryCodes:    countryCodes,
		Latitude:        lat,
		Longitude:       lon,
		DisplayName:     result.DisplayName,
		PlaceType:       result.Type,
		OSMID:           &result.OSMID,
		RawResponse:     rawJSON,
		Source:          "nominatim",
		HitCount:        0,
		CreatedAt:       time.Now(),
	}

	if err := s.cache.CacheGeocode(ctx, cacheEntry); err != nil {
		s.logger.Warn().Err(err).Str("query", query).Msg("failed to cache geocoding result")
	}

	return &GeocodeResult{
		Latitude:    lat,
		Longitude:   lon,
		DisplayName: result.DisplayName,
		Source:      "nominatim",
		Cached:      false,
	}, nil
}
