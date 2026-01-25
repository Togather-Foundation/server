package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/domain/ids"
)

type EventsHandler struct {
	Service *events.Service
	Ingest  *events.IngestService
	Env     string
	BaseURL string
}

func NewEventsHandler(service *events.Service, ingest *events.IngestService, env string, baseURL string) *EventsHandler {
	return &EventsHandler{Service: service, Ingest: ingest, Env: env, BaseURL: baseURL}
}

type listResponse struct {
	Items      []map[string]any `json:"items"`
	NextCursor string           `json:"next_cursor"`
}

func (h *EventsHandler) List(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Service == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	filters, pagination, err := events.ParseFilters(r.URL.Query())
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
	items := make([]map[string]any, 0, len(result.Events))
	for _, event := range result.Events {
		items = append(items, map[string]any{
			"@context": contextValue,
			"@type":    "Event",
			"name":     event.Name,
		})
	}

	writeJSON(w, http.StatusOK, listResponse{Items: items, NextCursor: result.NextCursor}, contentTypeFromRequest(r))
}

func (h *EventsHandler) Create(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Ingest == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	var input events.EventInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", err, h.Env)
		return
	}

	var (
		result *events.IngestResult
		err    error
	)
	if key := idempotencyKey(r); key != "" {
		result, err = h.Ingest.IngestWithIdempotency(r.Context(), input, key)
	} else {
		result, err = h.Ingest.Ingest(r.Context(), input)
	}
	if err != nil {
		if errors.Is(err, events.ErrConflict) {
			problem.Write(w, r, http.StatusConflict, "https://sel.events/problems/conflict", "Conflict", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", err, h.Env)
		return
	}

	status := http.StatusCreated
	if result != nil && result.IsDuplicate {
		status = http.StatusConflict
	}

	location := eventLocationPayload(input)
	payload := map[string]any{
		"@context": loadDefaultContext(),
		"@type":    "Event",
		"@id":      eventURI(h.BaseURL, result),
		"name":     input.Name,
	}
	if location != nil {
		payload["location"] = location
	}

	writeJSON(w, status, payload, contentTypeFromRequest(r))
}

func (h *EventsHandler) Get(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Service == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	ulidValue := strings.TrimSpace(pathParam(r, "id"))
	if ulidValue == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", events.FilterError{Field: "id", Message: "missing"}, h.Env)
		return
	}
	if err := ids.ValidateULID(ulidValue); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", events.FilterError{Field: "id", Message: "invalid ULID"}, h.Env)
		return
	}

	item, err := h.Service.GetByULID(r.Context(), ulidValue)
	if err != nil {
		if errors.Is(err, events.ErrNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Not found", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	contextValue := loadDefaultContext()
	payload := map[string]any{
		"@context": contextValue,
		"@type":    "Event",
		"name":     item.Name,
	}
	writeJSON(w, http.StatusOK, payload, contentTypeFromRequest(r))
}

func writeJSON(w http.ResponseWriter, status int, payload any, contentType string) {
	if contentType == "" {
		contentType = "application/json"
	}
	if !strings.HasPrefix(contentType, "application/") {
		contentType = "application/json"
	}

	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func contentTypeFromRequest(r *http.Request) string {
	if r == nil {
		return "application/json"
	}
	accept := strings.TrimSpace(r.Header.Get("Accept"))
	if accept == "" || strings.HasPrefix(accept, "application/json") {
		return "application/json"
	}
	if strings.HasPrefix(accept, "application/ld+json") {
		return "application/ld+json"
	}
	return "application/json"
}

func pathParam(r *http.Request, key string) string {
	if r == nil {
		return ""
	}
	return r.PathValue(key)
}

func eventURI(baseURL string, result *events.IngestResult) string {
	if result == nil || result.Event == nil {
		return ""
	}
	uri, err := ids.BuildCanonicalURI(baseURL, "events", result.Event.ULID)
	if err != nil {
		return ""
	}
	return uri
}

func eventLocationPayload(input events.EventInput) map[string]any {
	if input.Location != nil {
		return map[string]any{
			"@type": "Place",
			"name":  input.Location.Name,
		}
	}
	if input.VirtualLocation != nil {
		return map[string]any{
			"@type": "VirtualLocation",
			"url":   input.VirtualLocation.URL,
			"name":  input.VirtualLocation.Name,
		}
	}
	return nil
}

func idempotencyKey(r *http.Request) string {
	if r == nil {
		return ""
	}
	return strings.TrimSpace(r.Header.Get("Idempotency-Key"))
}
