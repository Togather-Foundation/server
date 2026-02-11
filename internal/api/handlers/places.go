package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/domain/ids"
	"github.com/Togather-Foundation/server/internal/domain/places"
)

type PlacesHandler struct {
	Service *places.Service
	Env     string
	BaseURL string
}

func NewPlacesHandler(service *places.Service, env string, baseURL string) *PlacesHandler {
	return &PlacesHandler{Service: service, Env: env, BaseURL: baseURL}
}

type placeListResponse struct {
	Items      any    `json:"items"` // Accepts any slice type for JSON encoding
	NextCursor string `json:"next_cursor"`
}

func (h *PlacesHandler) List(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Service == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	filters, pagination, err := places.ParseFilters(r.URL.Query())
	if err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", err, h.Env)
		return
	}

	result, err := h.Service.List(r.Context(), filters, pagination)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	items := make([]map[string]any, 0, len(result.Places))
	for _, place := range result.Places {
		item := BuildBaseListItem("Place", place.Name, place.ULID, "places", h.BaseURL)

		// Add address (required per Interop Profile §3.1 - must have address OR geo)
		if place.StreetAddress != "" || place.City != "" || place.Region != "" || place.PostalCode != "" || place.Country != "" {
			address := map[string]any{
				"@type": "PostalAddress",
			}
			if place.StreetAddress != "" {
				address["streetAddress"] = place.StreetAddress
			}
			if place.City != "" {
				address["addressLocality"] = place.City
			}
			if place.Region != "" {
				address["addressRegion"] = place.Region
			}
			if place.PostalCode != "" {
				address["postalCode"] = place.PostalCode
			}
			if place.Country != "" {
				address["addressCountry"] = place.Country
			}
			item["address"] = address
		}

		// Add geo coordinates
		if place.Latitude != nil && place.Longitude != nil {
			item["geo"] = map[string]any{
				"@type":     "GeoCoordinates",
				"latitude":  *place.Latitude,
				"longitude": *place.Longitude,
			}
		}

		items = append(items, item)
	}

	writeJSON(w, http.StatusOK, placeListResponse{Items: items, NextCursor: result.NextCursor}, contentTypeFromRequest(r))
}

func (h *PlacesHandler) Get(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Service == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	ulidValue, ok := ValidateAndExtractULID(w, r, "id", h.Env)
	if !ok {
		return
	}

	item, err := h.Service.GetByULID(r.Context(), ulidValue)
	if err != nil {
		if errors.Is(err, places.ErrNotFound) {
			// Check if there's a tombstone for this place
			tombstone, tombErr := h.Service.GetTombstoneByULID(r.Context(), ulidValue)
			if tombErr == nil && tombstone != nil {
				WriteTombstoneResponse(w, r, tombstone.Payload, tombstone.DeletedAt, "Place")
				return
			}
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Not found", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	if strings.EqualFold(item.Lifecycle, "deleted") {
		tombstone, tombErr := h.Service.GetTombstoneByULID(r.Context(), ulidValue)
		if tombErr == nil && tombstone != nil {
			WriteTombstoneResponse(w, r, tombstone.Payload, tombstone.DeletedAt, "Place")
			return
		}

		payload := map[string]any{
			"@context":      loadDefaultContext(),
			"@type":         "Place",
			"sel:tombstone": true,
			"sel:deletedAt": time.Now().Format(time.RFC3339),
		}
		if uri, err := ids.BuildCanonicalURI(h.BaseURL, "places", ulidValue); err == nil {
			payload["@id"] = uri
		}
		writeJSON(w, http.StatusGone, payload, contentTypeFromRequest(r))
		return
	}

	contextValue := loadDefaultContext()
	payload := map[string]any{
		"@context": contextValue,
		"@type":    "Place",
		"name":     item.Name,
	}

	// Add @id (required per Interop Profile §3.1)
	if uri, err := ids.BuildCanonicalURI(h.BaseURL, "places", item.ULID); err == nil {
		payload["@id"] = uri
	}

	// Add address (required per Interop Profile §3.1 - must have address OR geo)
	if item.StreetAddress != "" || item.City != "" || item.Region != "" || item.PostalCode != "" || item.Country != "" {
		address := map[string]any{
			"@type": "PostalAddress",
		}
		if item.StreetAddress != "" {
			address["streetAddress"] = item.StreetAddress
		}
		if item.City != "" {
			address["addressLocality"] = item.City
		}
		if item.Region != "" {
			address["addressRegion"] = item.Region
		}
		if item.PostalCode != "" {
			address["postalCode"] = item.PostalCode
		}
		if item.Country != "" {
			address["addressCountry"] = item.Country
		}
		payload["address"] = address
	}

	// Add geo coordinates
	if item.Latitude != nil && item.Longitude != nil {
		payload["geo"] = map[string]any{
			"@type":     "GeoCoordinates",
			"latitude":  *item.Latitude,
			"longitude": *item.Longitude,
		}
	}

	// Add optional fields
	if item.Telephone != "" {
		payload["telephone"] = item.Telephone
	}
	if item.Email != "" {
		payload["email"] = item.Email
	}
	if item.URL != "" {
		payload["url"] = item.URL
	}
	if item.MaximumAttendeeCapacity != nil {
		payload["maximumAttendeeCapacity"] = *item.MaximumAttendeeCapacity
	}

	writeJSON(w, http.StatusOK, payload, contentTypeFromRequest(r))
}
