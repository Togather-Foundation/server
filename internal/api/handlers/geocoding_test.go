package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Togather-Foundation/server/internal/geocoding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockGeocodingService is a mock implementation of the geocoding service for testing
type mockGeocodingService struct {
	geocodeFunc        func(ctx context.Context, query string, countryCodes string) (*geocoding.GeocodeResult, error)
	reverseGeocodeFunc func(ctx context.Context, lat, lon float64) (*geocoding.ReverseGeocodeResult, error)
}

func (m *mockGeocodingService) Geocode(ctx context.Context, query string, countryCodes string) (*geocoding.GeocodeResult, error) {
	if m.geocodeFunc != nil {
		return m.geocodeFunc(ctx, query, countryCodes)
	}
	return nil, nil
}

func (m *mockGeocodingService) ReverseGeocode(ctx context.Context, lat, lon float64) (*geocoding.ReverseGeocodeResult, error) {
	if m.reverseGeocodeFunc != nil {
		return m.reverseGeocodeFunc(ctx, lat, lon)
	}
	return nil, nil
}

// TestReverseGeocode_Success tests successful reverse geocoding via HTTP
func TestReverseGeocode_Success(t *testing.T) {
	mockService := &mockGeocodingService{
		reverseGeocodeFunc: func(ctx context.Context, lat, lon float64) (*geocoding.ReverseGeocodeResult, error) {
			return &geocoding.ReverseGeocodeResult{
				DisplayName: "Toronto City Hall, 100 Queen Street West, Toronto, Ontario, M5H 2N2, Canada",
				Address: geocoding.AddressComponents{
					Road:     "Queen Street West",
					Suburb:   "Downtown Toronto",
					City:     "Toronto",
					State:    "Ontario",
					Postcode: "M5H 2N2",
					Country:  "Canada",
				},
				Latitude:  43.6532,
				Longitude: -79.3832,
				Source:    "nominatim",
				Cached:    false,
			}, nil
		},
	}

	handler := NewGeocodingHandler(mockService, "test")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reverse-geocode?lat=43.6532&lon=-79.3832", nil)
	w := httptest.NewRecorder()

	handler.ReverseGeocode(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.Equal(t, "Toronto City Hall, 100 Queen Street West, Toronto, Ontario, M5H 2N2, Canada", result["display_name"])
	assert.Equal(t, 43.6532, result["latitude"])
	assert.Equal(t, -79.3832, result["longitude"])
	assert.Equal(t, "nominatim", result["source"])
	assert.Equal(t, false, result["cached"])
	assert.Equal(t, OSMAttribution, result["attribution"])

	// Check address components
	address, ok := result["address"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "Queen Street West", address["road"])
	assert.Equal(t, "Downtown Toronto", address["suburb"])
	assert.Equal(t, "Toronto", address["city"])
	assert.Equal(t, "Ontario", address["state"])
	assert.Equal(t, "M5H 2N2", address["postcode"])
	assert.Equal(t, "Canada", address["country"])
}

// TestReverseGeocode_MissingLatitude tests reverse geocoding with missing latitude
func TestReverseGeocode_MissingLatitude(t *testing.T) {
	mockService := &mockGeocodingService{}
	handler := NewGeocodingHandler(mockService, "test")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reverse-geocode?lon=-79.3832", nil)
	w := httptest.NewRecorder()

	handler.ReverseGeocode(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var result map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, "Missing required parameter", result["title"])
}

// TestReverseGeocode_MissingLongitude tests reverse geocoding with missing longitude
func TestReverseGeocode_MissingLongitude(t *testing.T) {
	mockService := &mockGeocodingService{}
	handler := NewGeocodingHandler(mockService, "test")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reverse-geocode?lat=43.6532", nil)
	w := httptest.NewRecorder()

	handler.ReverseGeocode(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var result map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, "Missing required parameter", result["title"])
}

// TestReverseGeocode_InvalidLatitude tests reverse geocoding with invalid latitude
func TestReverseGeocode_InvalidLatitude(t *testing.T) {
	mockService := &mockGeocodingService{}
	handler := NewGeocodingHandler(mockService, "test")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reverse-geocode?lat=invalid&lon=-79.3832", nil)
	w := httptest.NewRecorder()

	handler.ReverseGeocode(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var result map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, "Invalid latitude", result["title"])
}

// TestReverseGeocode_InvalidLongitude tests reverse geocoding with invalid longitude
func TestReverseGeocode_InvalidLongitude(t *testing.T) {
	mockService := &mockGeocodingService{}
	handler := NewGeocodingHandler(mockService, "test")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reverse-geocode?lat=43.6532&lon=invalid", nil)
	w := httptest.NewRecorder()

	handler.ReverseGeocode(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var result map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, "Invalid longitude", result["title"])
}

// TestReverseGeocode_LatitudeOutOfRange tests reverse geocoding with out-of-range latitude
func TestReverseGeocode_LatitudeOutOfRange(t *testing.T) {
	mockService := &mockGeocodingService{}
	handler := NewGeocodingHandler(mockService, "test")

	// Test latitude > 90
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reverse-geocode?lat=91.0&lon=-79.3832", nil)
	w := httptest.NewRecorder()

	handler.ReverseGeocode(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var result map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, "Invalid latitude range", result["title"])

	// Test latitude < -90
	req = httptest.NewRequest(http.MethodGet, "/api/v1/reverse-geocode?lat=-91.0&lon=-79.3832", nil)
	w = httptest.NewRecorder()

	handler.ReverseGeocode(w, req)

	resp = w.Result()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, "Invalid latitude range", result["title"])
}

// TestReverseGeocode_LongitudeOutOfRange tests reverse geocoding with out-of-range longitude
func TestReverseGeocode_LongitudeOutOfRange(t *testing.T) {
	mockService := &mockGeocodingService{}
	handler := NewGeocodingHandler(mockService, "test")

	// Test longitude > 180
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reverse-geocode?lat=43.6532&lon=181.0", nil)
	w := httptest.NewRecorder()

	handler.ReverseGeocode(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var result map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, "Invalid longitude range", result["title"])

	// Test longitude < -180
	req = httptest.NewRequest(http.MethodGet, "/api/v1/reverse-geocode?lat=43.6532&lon=-181.0", nil)
	w = httptest.NewRecorder()

	handler.ReverseGeocode(w, req)

	resp = w.Result()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, "Invalid longitude range", result["title"])
}

// TestReverseGeocode_MethodNotAllowed tests reverse geocoding with wrong HTTP method
func TestReverseGeocode_MethodNotAllowed(t *testing.T) {
	mockService := &mockGeocodingService{}
	handler := NewGeocodingHandler(mockService, "test")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/reverse-geocode?lat=43.6532&lon=-79.3832", nil)
	w := httptest.NewRecorder()

	handler.ReverseGeocode(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

// TestReverseGeocode_ServiceError tests reverse geocoding when service returns error
func TestReverseGeocode_ServiceError(t *testing.T) {
	mockService := &mockGeocodingService{
		reverseGeocodeFunc: func(ctx context.Context, lat, lon float64) (*geocoding.ReverseGeocodeResult, error) {
			return nil, geocoding.ErrGeocodingFailed
		},
	}

	handler := NewGeocodingHandler(mockService, "test")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reverse-geocode?lat=43.6532&lon=-79.3832", nil)
	w := httptest.NewRecorder()

	handler.ReverseGeocode(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)

	var result map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, "Reverse geocoding failed", result["title"])
}

// TestReverseGeocode_CachedResult tests reverse geocoding with cached result
func TestReverseGeocode_CachedResult(t *testing.T) {
	mockService := &mockGeocodingService{
		reverseGeocodeFunc: func(ctx context.Context, lat, lon float64) (*geocoding.ReverseGeocodeResult, error) {
			return &geocoding.ReverseGeocodeResult{
				DisplayName: "Cached Location",
				Address: geocoding.AddressComponents{
					Road:    "Cached Road",
					City:    "Cached City",
					Country: "Cached Country",
				},
				Latitude:  43.6532,
				Longitude: -79.3832,
				Source:    "cache",
				Cached:    true,
			}, nil
		},
	}

	handler := NewGeocodingHandler(mockService, "test")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reverse-geocode?lat=43.6532&lon=-79.3832", nil)
	w := httptest.NewRecorder()

	handler.ReverseGeocode(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.Equal(t, "Cached Location", result["display_name"])
	assert.Equal(t, "cache", result["source"])
	assert.Equal(t, true, result["cached"])
}
