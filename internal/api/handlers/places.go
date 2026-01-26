package handlers

import (
	"encoding/json"
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
	Items      []map[string]any `json:"items"`
	NextCursor string           `json:"next_cursor"`
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
	items := make([]map[string]any, 0, len(result.Places))
	for _, place := range result.Places {
		items = append(items, map[string]any{
			"@context": contextValue,
			"@type":    "Place",
			"name":     place.Name,
		})
	}

	writeJSON(w, http.StatusOK, placeListResponse{Items: items, NextCursor: result.NextCursor}, contentTypeFromRequest(r))
}

func (h *PlacesHandler) Get(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Service == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	ulidValue := strings.TrimSpace(pathParam(r, "id"))
	if ulidValue == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", places.FilterError{Field: "id", Message: "missing"}, h.Env)
		return
	}
	if err := places.ValidateULID(ulidValue); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", places.FilterError{Field: "id", Message: "invalid ULID"}, h.Env)
		return
	}

	item, err := h.Service.GetByULID(r.Context(), ulidValue)
	if err != nil {
		if errors.Is(err, places.ErrNotFound) {
			// Check if there's a tombstone for this place
			tombstone, tombErr := h.Service.GetTombstoneByULID(r.Context(), ulidValue)
			if tombErr == nil && tombstone != nil {
				var payload map[string]any
				if err := json.Unmarshal(tombstone.Payload, &payload); err != nil {
					payload = map[string]any{
						"@context":      loadDefaultContext(),
						"@type":         "Place",
						"sel:tombstone": true,
						"sel:deletedAt": tombstone.DeletedAt.Format(time.RFC3339),
					}
				}
				writeJSON(w, http.StatusGone, payload, contentTypeFromRequest(r))
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
			var payload map[string]any
			if err := json.Unmarshal(tombstone.Payload, &payload); err != nil {
				payload = map[string]any{
					"@context":      loadDefaultContext(),
					"@type":         "Place",
					"sel:tombstone": true,
					"sel:deletedAt": tombstone.DeletedAt.Format(time.RFC3339),
				}
			}
			writeJSON(w, http.StatusGone, payload, contentTypeFromRequest(r))
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
	writeJSON(w, http.StatusOK, payload, contentTypeFromRequest(r))
}
