package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/domain/ids"
	"github.com/Togather-Foundation/server/internal/domain/places"
	"github.com/Togather-Foundation/server/internal/jsonld/schema"
)

type PlacesHandler struct {
	Service *places.Service
	Env     string
	BaseURL string
}

func NewPlacesHandler(service *places.Service, env string, baseURL string) *PlacesHandler {
	return &PlacesHandler{Service: service, Env: env, BaseURL: baseURL}
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

	contextValue := loadDefaultContext()
	items := make([]*schema.Place, 0, len(result.Places))
	for _, place := range result.Places {
		item := schema.NewPlace(place.Name)
		item.Context = contextValue
		item.ID = schema.BuildPlaceURI(h.BaseURL, place.ULID)
		item.Address = schema.NewPostalAddress(place.StreetAddress, place.City, place.Region, place.PostalCode, place.Country)
		if place.Latitude != nil && place.Longitude != nil {
			item.Geo = schema.NewGeoCoordinates(*place.Latitude, *place.Longitude)
		}
		if place.DistanceKm != nil {
			item.DistanceKm = place.DistanceKm
		}
		items = append(items, item)
	}

	writeJSON(w, http.StatusOK, listResponse{Items: items, NextCursor: result.NextCursor}, contentTypeFromRequest(r))
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

	place := schema.NewPlace(item.Name)
	place.Context = loadDefaultContext()
	place.ID = schema.BuildPlaceURI(h.BaseURL, item.ULID)
	place.Description = item.Description
	place.Address = schema.NewPostalAddress(item.StreetAddress, item.City, item.Region, item.PostalCode, item.Country)
	if item.Latitude != nil && item.Longitude != nil {
		place.Geo = schema.NewGeoCoordinates(*item.Latitude, *item.Longitude)
	}
	place.Telephone = item.Telephone
	place.Email = item.Email
	place.URL = item.URL
	if item.MaximumAttendeeCapacity != nil {
		place.MaximumAttendeeCapacity = *item.MaximumAttendeeCapacity
	}

	writeJSON(w, http.StatusOK, place, contentTypeFromRequest(r))
}
