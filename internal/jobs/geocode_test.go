package jobs

import (
	"context"
	"log/slog"
	"testing"

	"github.com/Togather-Foundation/server/internal/geocoding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockGeocodingService implements geocoding.GeocodingService for testing
type MockGeocodingService struct {
	GeocodeFunc func(ctx context.Context, query string, countryCodes string) (*geocoding.GeocodeResult, error)
}

func (m *MockGeocodingService) Geocode(ctx context.Context, query string, countryCodes string) (*geocoding.GeocodeResult, error) {
	if m.GeocodeFunc != nil {
		return m.GeocodeFunc(ctx, query, countryCodes)
	}
	return &geocoding.GeocodeResult{
		Latitude:    43.6532,
		Longitude:   -79.3832,
		DisplayName: "Toronto, ON, Canada",
		Source:      "mock",
		Cached:      false,
	}, nil
}

func TestGeocodePlaceWorker_MissingConfig(t *testing.T) {
	worker := GeocodePlaceWorker{}
	err := worker.Work(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database pool not configured")
}

func TestGeocodeEventWorker_MissingConfig(t *testing.T) {
	worker := GeocodeEventWorker{}
	err := worker.Work(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database pool not configured")
}

func TestGeocodePlaceWorker_Kind(t *testing.T) {
	worker := GeocodePlaceWorker{}
	assert.Equal(t, JobKindGeocodePlace, worker.Kind())

	args := GeocodePlaceArgs{PlaceID: "test"}
	assert.Equal(t, JobKindGeocodePlace, args.Kind())
}

func TestGeocodeEventWorker_Kind(t *testing.T) {
	worker := GeocodeEventWorker{}
	assert.Equal(t, JobKindGeocodeEvent, worker.Kind())

	args := GeocodeEventArgs{EventID: "test"}
	assert.Equal(t, JobKindGeocodeEvent, args.Kind())
}

// Test worker initialization
func TestNewWorkersWithPool_RegistersGeocodingWorkers(t *testing.T) {
	// This is more of an integration test that verifies workers are registered
	// We can't easily test the internal structure of river.Workers, but we can
	// verify that passing a geocoding service doesn't panic
	mockGeocodingService := &geocoding.GeocodingService{}
	logger := slog.Default()

	workers := NewWorkersWithPool(
		nil, // pool - nil is ok for this test
		nil, // ingestService
		nil, // eventsRepo
		mockGeocodingService,
		logger,
		"test",
	)

	assert.NotNil(t, workers)
}

// Test that nil geocoding service doesn't break worker registration
func TestNewWorkersWithPool_NilGeocodingService(t *testing.T) {
	logger := slog.Default()

	workers := NewWorkersWithPool(
		nil, // pool
		nil, // ingestService
		nil, // eventsRepo
		nil, // geocodingService - should not panic
		logger,
		"test",
	)

	assert.NotNil(t, workers)
}
