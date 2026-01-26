package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/domain/ids"
)

type AdminHandler struct {
	Service      *events.Service
	AdminService *events.AdminService
	Env          string
	BaseURL      string
}

func NewAdminHandler(service *events.Service, adminService *events.AdminService, env string, baseURL string) *AdminHandler {
	return &AdminHandler{
		Service:      service,
		AdminService: adminService,
		Env:          env,
		BaseURL:      baseURL,
	}
}

// ListPendingEvents handles GET /api/v1/admin/events/pending
// Returns events in draft state or flagged for review
func (h *AdminHandler) ListPendingEvents(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Service == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	// Parse filters from query parameters
	filters, pagination, err := events.ParseFilters(r.URL.Query())
	if err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", err, h.Env)
		return
	}

	// Override lifecycle state to only return draft events (pending review)
	filters.LifecycleState = "draft"

	// Fetch pending events
	result, err := h.Service.List(r.Context(), filters, pagination)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	// Build response with JSON-LD context
	contextValue := loadDefaultContext()
	items := make([]map[string]any, 0, len(result.Events))
	for _, event := range result.Events {
		item := map[string]any{
			"@context": contextValue,
			"@type":    "Event",
			"@id":      buildEventURI(h.BaseURL, event.ULID),
			"name":     event.Name,
		}
		// Include additional fields for admin review
		if event.Description != "" {
			item["description"] = event.Description
		}
		if event.Confidence != nil {
			item["confidence"] = *event.Confidence
		}
		if event.LifecycleState != "" {
			item["lifecycle_state"] = event.LifecycleState
		}
		items = append(items, item)
	}

	writeJSON(w, http.StatusOK, listResponse{Items: items, NextCursor: result.NextCursor}, contentTypeFromRequest(r))
}

// UpdateEvent handles PUT /api/v1/admin/events/{id}
// Allows admin to update event fields
func (h *AdminHandler) UpdateEvent(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.AdminService == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	// Extract and validate event ID
	ulidValue := strings.TrimSpace(pathParam(r, "id"))
	if ulidValue == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", events.FilterError{Field: "id", Message: "missing"}, h.Env)
		return
	}
	if err := ids.ValidateULID(ulidValue); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", events.FilterError{Field: "id", Message: "invalid ULID"}, h.Env)
		return
	}

	// Parse update request body
	var updates map[string]any
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", err, h.Env)
		return
	}

	// Convert map to UpdateEventParams
	params := mapToUpdateParams(updates)

	// Call admin service to update event
	updated, err := h.AdminService.UpdateEvent(r.Context(), ulidValue, params)
	if err != nil {
		if errors.Is(err, events.ErrNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Event not found", err, h.Env)
			return
		}
		if errors.Is(err, events.ErrInvalidUpdateParams) {
			problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid update parameters", err, h.Env)
			return
		}
		// Check for FilterError
		var filterErr events.FilterError
		if errors.As(err, &filterErr) {
			problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	contextValue := loadDefaultContext()
	payload := map[string]any{
		"@context": contextValue,
		"@type":    "Event",
		"@id":      buildEventURI(h.BaseURL, updated.ULID),
		"name":     updated.Name,
	}
	if updated.Description != "" {
		payload["description"] = updated.Description
	}
	if updated.LifecycleState != "" {
		payload["lifecycle_state"] = updated.LifecycleState
	}

	writeJSON(w, http.StatusOK, payload, contentTypeFromRequest(r))
}

// PublishEvent handles POST /api/v1/admin/events/{id}/publish
// Changes lifecycle_state from draft to published
func (h *AdminHandler) PublishEvent(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.AdminService == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	// Extract and validate event ID
	ulidValue := strings.TrimSpace(pathParam(r, "id"))
	if ulidValue == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", events.FilterError{Field: "id", Message: "missing"}, h.Env)
		return
	}
	if err := ids.ValidateULID(ulidValue); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", events.FilterError{Field: "id", Message: "invalid ULID"}, h.Env)
		return
	}

	// Call admin service to publish event
	published, err := h.AdminService.PublishEvent(r.Context(), ulidValue)
	if err != nil {
		if errors.Is(err, events.ErrNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Event not found", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	contextValue := loadDefaultContext()
	payload := map[string]any{
		"@context":        contextValue,
		"@type":           "Event",
		"@id":             buildEventURI(h.BaseURL, published.ULID),
		"name":            published.Name,
		"lifecycle_state": published.LifecycleState,
	}

	writeJSON(w, http.StatusOK, payload, contentTypeFromRequest(r))
}

// UnpublishEvent handles POST /api/v1/admin/events/{id}/unpublish
// Changes lifecycle_state from published back to draft
func (h *AdminHandler) UnpublishEvent(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.AdminService == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	// Extract and validate event ID
	ulidValue := strings.TrimSpace(pathParam(r, "id"))
	if ulidValue == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", events.FilterError{Field: "id", Message: "missing"}, h.Env)
		return
	}
	if err := ids.ValidateULID(ulidValue); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", events.FilterError{Field: "id", Message: "invalid ULID"}, h.Env)
		return
	}

	// Call admin service to unpublish event
	unpublished, err := h.AdminService.UnpublishEvent(r.Context(), ulidValue)
	if err != nil {
		if errors.Is(err, events.ErrNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Event not found", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	contextValue := loadDefaultContext()
	payload := map[string]any{
		"@context":        contextValue,
		"@type":           "Event",
		"@id":             buildEventURI(h.BaseURL, unpublished.ULID),
		"name":            unpublished.Name,
		"lifecycle_state": unpublished.LifecycleState,
	}

	writeJSON(w, http.StatusOK, payload, contentTypeFromRequest(r))
}

// MergeEvents handles POST /api/v1/admin/events/merge
// Merges a duplicate event into a primary event
func (h *AdminHandler) MergeEvents(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.AdminService == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	// Parse merge request
	var req struct {
		PrimaryID   string `json:"primary_id"`
		DuplicateID string `json:"duplicate_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", err, h.Env)
		return
	}

	// Validate ULIDs
	if req.PrimaryID == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Missing primary_id", nil, h.Env)
		return
	}
	if req.DuplicateID == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Missing duplicate_id", nil, h.Env)
		return
	}

	params := events.MergeEventsParams{
		PrimaryULID:   req.PrimaryID,
		DuplicateULID: req.DuplicateID,
	}

	// Call admin service to merge events
	err := h.AdminService.MergeEvents(r.Context(), params)
	if err != nil {
		if errors.Is(err, events.ErrNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Event not found", err, h.Env)
			return
		}
		if errors.Is(err, events.ErrCannotMergeSameEvent) {
			problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Cannot merge event with itself", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "merged"}, contentTypeFromRequest(r))
}

// validateUpdateFields validates the fields that can be updated
func validateUpdateFields(updates map[string]any) error {
	// Validate name if present
	if name, ok := updates["name"].(string); ok {
		if name == "" {
			return events.FilterError{Field: "name", Message: "cannot be empty"}
		}
		if len(name) > 500 {
			return events.FilterError{Field: "name", Message: "exceeds maximum length of 500 characters"}
		}
	}

	// Validate lifecycle_state if present
	if state, ok := updates["lifecycle_state"].(string); ok {
		state = strings.ToLower(strings.TrimSpace(state))
		validStates := map[string]bool{
			"draft":       true,
			"published":   true,
			"postponed":   true,
			"rescheduled": true,
			"sold_out":    true,
			"cancelled":   true,
			"completed":   true,
		}
		if !validStates[state] {
			return events.FilterError{Field: "lifecycle_state", Message: "invalid state"}
		}
	}

	// Validate startDate if present
	if startDate, ok := updates["startDate"].(string); ok {
		if startDate != "" {
			// Try parsing as RFC3339 to validate format
			_, err := time.Parse(time.RFC3339, strings.TrimSpace(startDate))
			if err != nil {
				// Also try date-only format
				_, err2 := time.Parse("2006-01-02", strings.TrimSpace(startDate))
				if err2 != nil {
					return events.FilterError{Field: "startDate", Message: "invalid date format"}
				}
			}
		}
	}

	return nil
}

// mapToUpdateParams converts a map[string]any to UpdateEventParams
func mapToUpdateParams(updates map[string]any) events.UpdateEventParams {
	params := events.UpdateEventParams{}

	if name, ok := updates["name"].(string); ok {
		params.Name = &name
	}
	if description, ok := updates["description"].(string); ok {
		params.Description = &description
	}
	if lifecycleState, ok := updates["lifecycle_state"].(string); ok {
		params.LifecycleState = &lifecycleState
	}
	if imageURL, ok := updates["image_url"].(string); ok {
		params.ImageURL = &imageURL
	}
	if publicURL, ok := updates["public_url"].(string); ok {
		params.PublicURL = &publicURL
	}
	if eventDomain, ok := updates["event_domain"].(string); ok {
		params.EventDomain = &eventDomain
	}
	if keywords, ok := updates["keywords"].([]any); ok {
		strKeywords := make([]string, 0, len(keywords))
		for _, k := range keywords {
			if s, ok := k.(string); ok {
				strKeywords = append(strKeywords, s)
			}
		}
		if len(strKeywords) > 0 {
			params.Keywords = strKeywords
		}
	}

	return params
}

// buildEventURI constructs the canonical URI for an event
func buildEventURI(baseURL, ulid string) string {
	if baseURL == "" || ulid == "" {
		return ""
	}
	uri, err := ids.BuildCanonicalURI(baseURL, "events", ulid)
	if err != nil {
		return ""
	}
	return uri
}
