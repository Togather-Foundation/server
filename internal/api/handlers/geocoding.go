package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/geocoding"
)

// OSMAttribution is the attribution string required by OpenStreetMap usage policy
const OSMAttribution = "Data © OpenStreetMap contributors, ODbL 1.0"

// GeocodingHandler handles geocoding requests.
type GeocodingHandler struct {
	Service *geocoding.GeocodingService
	Env     string
}

// NewGeocodingHandler creates a new geocoding handler.
func NewGeocodingHandler(service *geocoding.GeocodingService, env string) *GeocodingHandler {
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
