package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/geocoding"
)

// OSMAttribution is the attribution string required by OpenStreetMap usage policy
const OSMAttribution = "Data © OpenStreetMap contributors, ODbL 1.0"

// GeocodingService defines the interface for geocoding operations.
type GeocodingService interface {
	Geocode(ctx context.Context, query string, countryCodes string) (*geocoding.GeocodeResult, error)
	ReverseGeocode(ctx context.Context, lat, lon float64) (*geocoding.ReverseGeocodeResult, error)
}

// GeocodingHandler handles geocoding requests.
type GeocodingHandler struct {
	Service GeocodingService
	Env     string
}

// NewGeocodingHandler creates a new geocoding handler.
func NewGeocodingHandler(service GeocodingService, env string) *GeocodingHandler {
	return &GeocodingHandler{
		Service: service,
		Env:     env,
	}
}

// Geocode handles GET /api/v1/geocode requests.
// Query params:
//   - q: address/place name to geocode (required)
//   - countrycodes: comma-separated ISO country codes (optional, default: ca)
func (h *GeocodingHandler) Geocode(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Service == nil {
		problem.Write(w, r, http.StatusInternalServerError,
			"https://sel.events/problems/server-error",
			"Server error",
			nil,
			h.Env)
		return
	}

	// Validate HTTP method
	if r.Method != http.MethodGet {
		problem.Write(w, r, http.StatusMethodNotAllowed,
			"https://sel.events/problems/method-not-allowed",
			"Method not allowed",
			nil,
			h.Env)
		return
	}

	// Get query parameter
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		problem.Write(w, r, http.StatusBadRequest,
			"https://sel.events/problems/validation-error",
			"Missing required parameter",
			errors.New("query parameter 'q' is required"),
			h.Env)
		return
	}

	// Get country codes parameter (optional, defaults to ca)
	countryCodes := strings.TrimSpace(r.URL.Query().Get("countrycodes"))
	if countryCodes == "" {
		countryCodes = "ca"
	}

	// Perform geocoding
	result, err := h.Service.Geocode(r.Context(), query, countryCodes)
	if err != nil {
		if errors.Is(err, geocoding.ErrNoResults) {
			problem.Write(w, r, http.StatusNotFound,
				"https://sel.events/problems/not-found",
				"No results found",
				err,
				h.Env,
				problem.WithDetail("No geocoding results found for the given query"))
			return
		}

		problem.Write(w, r, http.StatusUnprocessableEntity,
			"https://sel.events/problems/geocoding-failed",
			"Geocoding failed",
			err,
			h.Env)
		return
	}

	// Build response
	response := map[string]interface{}{
		"latitude":     result.Latitude,
		"longitude":    result.Longitude,
		"display_name": result.DisplayName,
		"source":       result.Source,
		"cached":       result.Cached,
		"attribution":  OSMAttribution,
	}

	writeJSON(w, http.StatusOK, response, "application/json")
}

// ReverseGeocode handles GET /api/v1/reverse-geocode requests.
// Query params:
//   - lat: latitude (required, must be between -90 and 90)
//   - lon: longitude (required, must be between -180 and 180)
func (h *GeocodingHandler) ReverseGeocode(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Service == nil {
		problem.Write(w, r, http.StatusInternalServerError,
			"https://sel.events/problems/server-error",
			"Server error",
			nil,
			h.Env)
		return
	}

	// Validate HTTP method
	if r.Method != http.MethodGet {
		problem.Write(w, r, http.StatusMethodNotAllowed,
			"https://sel.events/problems/method-not-allowed",
			"Method not allowed",
			nil,
			h.Env)
		return
	}

	// Get latitude parameter
	latStr := strings.TrimSpace(r.URL.Query().Get("lat"))
	if latStr == "" {
		problem.Write(w, r, http.StatusBadRequest,
			"https://sel.events/problems/validation-error",
			"Missing required parameter",
			errors.New("query parameter 'lat' is required"),
			h.Env)
		return
	}

	// Get longitude parameter
	lonStr := strings.TrimSpace(r.URL.Query().Get("lon"))
	if lonStr == "" {
		problem.Write(w, r, http.StatusBadRequest,
			"https://sel.events/problems/validation-error",
			"Missing required parameter",
			errors.New("query parameter 'lon' is required"),
			h.Env)
		return
	}

	// Parse latitude
	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		problem.Write(w, r, http.StatusBadRequest,
			"https://sel.events/problems/validation-error",
			"Invalid latitude",
			errors.New("latitude must be a valid number"),
			h.Env)
		return
	}

	// Parse longitude
	lon, err := strconv.ParseFloat(lonStr, 64)
	if err != nil {
		problem.Write(w, r, http.StatusBadRequest,
			"https://sel.events/problems/validation-error",
			"Invalid longitude",
			errors.New("longitude must be a valid number"),
			h.Env)
		return
	}

	// Validate coordinate ranges
	if lat < -90 || lat > 90 {
		problem.Write(w, r, http.StatusBadRequest,
			"https://sel.events/problems/validation-error",
			"Invalid latitude range",
			errors.New("latitude must be between -90 and 90"),
			h.Env)
		return
	}

	if lon < -180 || lon > 180 {
		problem.Write(w, r, http.StatusBadRequest,
			"https://sel.events/problems/validation-error",
			"Invalid longitude range",
			errors.New("longitude must be between -180 and 180"),
			h.Env)
		return
	}

	// Perform reverse geocoding
	result, err := h.Service.ReverseGeocode(r.Context(), lat, lon)
	if err != nil {
		if errors.Is(err, geocoding.ErrNoResults) {
			problem.Write(w, r, http.StatusNotFound,
				"https://sel.events/problems/not-found",
				"No results found",
				err,
				h.Env,
				problem.WithDetail("No reverse geocoding results found for the given coordinates"))
			return
		}

		problem.Write(w, r, http.StatusUnprocessableEntity,
			"https://sel.events/problems/geocoding-failed",
			"Reverse geocoding failed",
			err,
			h.Env)
		return
	}

	// Build response
	response := map[string]interface{}{
		"display_name": result.DisplayName,
		"address":      result.Address,
		"latitude":     result.Latitude,
		"longitude":    result.Longitude,
		"source":       result.Source,
		"cached":       result.Cached,
		"attribution":  OSMAttribution,
	}

	writeJSON(w, http.StatusOK, response, "application/json")
}
