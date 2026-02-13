package geocoding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/geocoding/nominatim"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGeocode_MockNominatim tests geocoding with a mock Nominatim server
func TestGeocode_MockNominatim(t *testing.T) {
	// Create mock Nominatim server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		if query == "Toronto City Hall" {
			response := []nominatim.SearchResult{
				{
					PlaceID:     1234,
					Lat:         "43.6532",
					Lon:         "-79.3832",
					DisplayName: "Toronto City Hall, 100 Queen Street West, Toronto, Ontario, Canada",
					Type:        "building",
					Class:       "amenity",
					OSMID:       5678,
					OSMType:     "way",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}
		// Return empty array for unknown queries
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	defer mockServer.Close()

	// Skip if no test database available
	dbURL := getTestDatabaseURL(t)
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping database test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	require.NoError(t, err)
	defer pool.Close()

	// Create Nominatim client pointing to mock server
	client := nominatim.NewClient(mockServer.URL, "test@example.com")

	// Create cache repository
	cache := postgres.NewGeocodingCacheRepository(pool)

	// Create geocoding service
	logger := zerolog.Nop()
	service := NewGeocodingService(client, cache, logger)

	// Test geocoding a known place
	result, err := service.Geocode(ctx, "Toronto City Hall", "ca")
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 43.6532, result.Latitude)
	assert.Equal(t, -79.3832, result.Longitude)
	assert.Equal(t, "Toronto City Hall, 100 Queen Street West, Toronto, Ontario, Canada", result.DisplayName)
	assert.Equal(t, "nominatim", result.Source)
	assert.False(t, result.Cached)

	// Test that second request hits cache
	result2, err := service.Geocode(ctx, "Toronto City Hall", "ca")
	require.NoError(t, err)
	assert.NotNil(t, result2)
	assert.Equal(t, 43.6532, result2.Latitude)
	assert.Equal(t, -79.3832, result2.Longitude)
	assert.Equal(t, "cache", result2.Source)
	assert.True(t, result2.Cached)
}

// TestGeocode_NoResults tests geocoding when Nominatim returns no results
func TestGeocode_NoResults(t *testing.T) {
	// Create mock Nominatim server that always returns empty results
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	defer mockServer.Close()

	// Skip if no test database available
	dbURL := getTestDatabaseURL(t)
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping database test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	require.NoError(t, err)
	defer pool.Close()

	// Create Nominatim client pointing to mock server
	client := nominatim.NewClient(mockServer.URL, "test@example.com")

	// Create cache repository
	cache := postgres.NewGeocodingCacheRepository(pool)

	// Create geocoding service
	logger := zerolog.Nop()
	service := NewGeocodingService(client, cache, logger)

	// Test geocoding a place that returns no results
	result, err := service.Geocode(ctx, "Nonexistent Place", "ca")
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrNoResults)
}

// TestGeocode_CacheExpiry tests that cache respects TTL
func TestGeocode_CacheExpiry(t *testing.T) {
	t.Skip("Cache expiry test requires database cleanup and is slow - run manually if needed")

	// This test would require:
	// 1. Setting a very short cache TTL (not currently configurable per-call)
	// 2. Waiting for expiry
	// 3. Verifying cache miss
	//
	// It's better tested manually or in a dedicated integration test suite
}

// getTestDatabaseURL returns the test database URL from environment
func getTestDatabaseURL(t *testing.T) string {
	t.Helper()
	return getEnv("TEST_DATABASE_URL", "")
}

// getEnv gets an environment variable or returns a default value
func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// TestReverseGeocode_MockNominatim tests reverse geocoding with a mock Nominatim server
func TestReverseGeocode_MockNominatim(t *testing.T) {
	// Create mock Nominatim server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lat := r.URL.Query().Get("lat")
		lon := r.URL.Query().Get("lon")

		// Toronto City Hall coordinates
		if lat == "43.653200" && lon == "-79.383200" {
			response := nominatim.ReverseResult{
				PlaceID:     1234,
				Lat:         "43.6532",
				Lon:         "-79.3832",
				DisplayName: "Toronto City Hall, 100 Queen Street West, Downtown Toronto, Toronto, Ontario, M5H 2N2, Canada",
				Type:        "building",
				Class:       "amenity",
				OSMID:       5678,
				OSMType:     "way",
				Address: nominatim.Address{
					Road:     "Queen Street West",
					Suburb:   "Downtown Toronto",
					City:     "Toronto",
					State:    "Ontario",
					Postcode: "M5H 2N2",
					Country:  "Canada",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}

		// Return error for unknown coordinates
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"Unable to geocode"}`))
	}))
	defer mockServer.Close()

	// Skip if no test database available
	dbURL := getTestDatabaseURL(t)
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping database test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	require.NoError(t, err)
	defer pool.Close()

	// Clean up any existing cache entries for this test
	_, _ = pool.Exec(ctx, "DELETE FROM reverse_geocoding_cache WHERE latitude BETWEEN 43.6 AND 43.7 AND longitude BETWEEN -79.4 AND -79.3")

	// Create Nominatim client pointing to mock server
	client := nominatim.NewClient(mockServer.URL, "test@example.com")

	// Create cache repository
	cache := postgres.NewGeocodingCacheRepository(pool)

	// Create geocoding service
	logger := zerolog.Nop()
	service := NewGeocodingService(client, cache, logger)

	// Test reverse geocoding
	result, err := service.ReverseGeocode(ctx, 43.6532, -79.3832)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "Toronto City Hall, 100 Queen Street West, Downtown Toronto, Toronto, Ontario, M5H 2N2, Canada", result.DisplayName)
	assert.Equal(t, "Queen Street West", result.Address.Road)
	assert.Equal(t, "Downtown Toronto", result.Address.Suburb)
	assert.Equal(t, "Toronto", result.Address.City)
	assert.Equal(t, "Ontario", result.Address.State)
	assert.Equal(t, "M5H 2N2", result.Address.Postcode)
	assert.Equal(t, "Canada", result.Address.Country)
	assert.Equal(t, 43.6532, result.Latitude)
	assert.Equal(t, -79.3832, result.Longitude)
	assert.Equal(t, "nominatim", result.Source)
	assert.False(t, result.Cached)

	// Allow cache write to complete (background operation)
	time.Sleep(100 * time.Millisecond)

	// Test that second request hits cache (exact same coordinates)
	result2, err := service.ReverseGeocode(ctx, 43.6532, -79.3832)
	require.NoError(t, err)
	assert.NotNil(t, result2)
	assert.Equal(t, "Toronto City Hall, 100 Queen Street West, Downtown Toronto, Toronto, Ontario, M5H 2N2, Canada", result2.DisplayName)
	assert.Equal(t, "cache", result2.Source)
	assert.True(t, result2.Cached)
}

// TestReverseGeocode_CacheBucketRadius tests that nearby coordinates share cache entries
func TestReverseGeocode_CacheBucketRadius(t *testing.T) {
	// Create mock Nominatim server
	requestCount := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		response := nominatim.ReverseResult{
			PlaceID:     1234,
			Lat:         r.URL.Query().Get("lat"),
			Lon:         r.URL.Query().Get("lon"),
			DisplayName: "Test Location",
			Type:        "building",
			Class:       "amenity",
			OSMID:       5678,
			OSMType:     "way",
			Address: nominatim.Address{
				Road:    "Test Street",
				City:    "Test City",
				Country: "Test Country",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	// Skip if no test database available
	dbURL := getTestDatabaseURL(t)
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping database test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	require.NoError(t, err)
	defer pool.Close()

	// Clean up any existing cache entries for this test
	_, _ = pool.Exec(ctx, "DELETE FROM reverse_geocoding_cache WHERE latitude BETWEEN 43.6 AND 43.7 AND longitude BETWEEN -79.4 AND -79.3")

	// Create Nominatim client pointing to mock server
	client := nominatim.NewClient(mockServer.URL, "test@example.com")

	// Create cache repository
	cache := postgres.NewGeocodingCacheRepository(pool)

	// Create geocoding service
	logger := zerolog.Nop()
	service := NewGeocodingService(client, cache, logger)

	// First request - cache miss
	result1, err := service.ReverseGeocode(ctx, 43.6532, -79.3832)
	require.NoError(t, err)
	assert.NotNil(t, result1)
	assert.Equal(t, "nominatim", result1.Source)
	assert.False(t, result1.Cached)
	assert.Equal(t, 1, requestCount) // Should have made 1 API call

	// Allow cache write to complete
	time.Sleep(100 * time.Millisecond)

	// Second request ~50m away (within 100m cache radius) - should hit cache
	// Approximate offset: 0.0005 degrees ≈ 50m at this latitude
	result2, err := service.ReverseGeocode(ctx, 43.6537, -79.3827)
	require.NoError(t, err)
	assert.NotNil(t, result2)
	assert.Equal(t, "cache", result2.Source)
	assert.True(t, result2.Cached)
	assert.Equal(t, 1, requestCount) // Should still be 1 API call (cache hit)

	// Third request >100m away - should miss cache
	// Approximate offset: 0.0015 degrees ≈ 150m at this latitude
	result3, err := service.ReverseGeocode(ctx, 43.6547, -79.3817)
	require.NoError(t, err)
	assert.NotNil(t, result3)
	assert.Equal(t, "nominatim", result3.Source)
	assert.False(t, result3.Cached)
	assert.Equal(t, 2, requestCount) // Should have made 2 API calls
}

// TestReverseGeocode_InvalidCoordinates tests reverse geocoding with invalid coordinates
func TestReverseGeocode_InvalidCoordinates(t *testing.T) {
	// Skip if no test database available
	dbURL := getTestDatabaseURL(t)
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping database test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	require.NoError(t, err)
	defer pool.Close()

	// Create mock client (won't be called)
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer mockServer.Close()
	client := nominatim.NewClient(mockServer.URL, "test@example.com")

	// Create cache repository
	cache := postgres.NewGeocodingCacheRepository(pool)

	// Create geocoding service
	logger := zerolog.Nop()
	service := NewGeocodingService(client, cache, logger)

	// Test invalid latitude (> 90)
	result, err := service.ReverseGeocode(ctx, 91.0, -79.3832)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "invalid latitude")

	// Test invalid latitude (< -90)
	result, err = service.ReverseGeocode(ctx, -91.0, -79.3832)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "invalid latitude")

	// Test invalid longitude (> 180)
	result, err = service.ReverseGeocode(ctx, 43.6532, 181.0)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "invalid longitude")

	// Test invalid longitude (< -180)
	result, err = service.ReverseGeocode(ctx, 43.6532, -181.0)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "invalid longitude")
}

// TestReverseGeocode_NominatimFailure tests reverse geocoding when Nominatim fails
func TestReverseGeocode_NominatimFailure(t *testing.T) {
	// Create mock Nominatim server that always fails
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"Internal server error"}`))
	}))
	defer mockServer.Close()

	// Skip if no test database available
	dbURL := getTestDatabaseURL(t)
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping database test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	require.NoError(t, err)
	defer pool.Close()

	// Clean up any existing cache entries for this test (use different coordinates)
	_, _ = pool.Exec(ctx, "DELETE FROM reverse_geocoding_cache WHERE latitude BETWEEN 43.9 AND 44.0 AND longitude BETWEEN -79.5 AND -79.4")

	// Create Nominatim client pointing to mock server
	client := nominatim.NewClient(mockServer.URL, "test@example.com")

	// Create cache repository
	cache := postgres.NewGeocodingCacheRepository(pool)

	// Create geocoding service
	logger := zerolog.Nop()
	service := NewGeocodingService(client, cache, logger)

	// Test reverse geocoding with failing Nominatim (using coordinates not in cache)
	result, err := service.ReverseGeocode(ctx, 43.95, -79.45)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrGeocodingFailed)
}
