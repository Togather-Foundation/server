package geocoding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

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
