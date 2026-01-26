package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/audit"
	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/domain/ids"
	"github.com/Togather-Foundation/server/internal/sanitize"
)

type AdminHandler struct {
	Service      *events.Service
	AdminService *events.AdminService
	AuditLogger  *audit.Logger
	Env          string
	BaseURL      string
}

func NewAdminHandler(service *events.Service, adminService *events.AdminService, auditLogger *audit.Logger, env string, baseURL string) *AdminHandler {
	return &AdminHandler{
		Service:      service,
		AdminService: adminService,
		AuditLogger:  auditLogger,
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
		// Log failure
		if h.AuditLogger != nil {
			h.AuditLogger.LogFromRequest(r, "admin.event.update", "event", ulidValue, "failure", map[string]string{
				"error": err.Error(),
			})
		}

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

	// Log success
	if h.AuditLogger != nil {
		h.AuditLogger.LogFromRequest(r, "admin.event.update", "event", ulidValue, "success", nil)
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
		// Log failure
		if h.AuditLogger != nil {
			h.AuditLogger.LogFromRequest(r, "admin.event.publish", "event", ulidValue, "failure", map[string]string{
				"error": err.Error(),
			})
		}

		if errors.Is(err, events.ErrNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Event not found", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	// Log success
	if h.AuditLogger != nil {
		h.AuditLogger.LogFromRequest(r, "admin.event.publish", "event", ulidValue, "success", nil)
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
		// Log failure
		if h.AuditLogger != nil {
			h.AuditLogger.LogFromRequest(r, "admin.event.unpublish", "event", ulidValue, "failure", map[string]string{
				"error": err.Error(),
			})
		}

		if errors.Is(err, events.ErrNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Event not found", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	// Log success
	if h.AuditLogger != nil {
		h.AuditLogger.LogFromRequest(r, "admin.event.unpublish", "event", ulidValue, "success", nil)
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
		// Log failure
		if h.AuditLogger != nil {
			h.AuditLogger.LogFromRequest(r, "admin.event.merge", "event", req.PrimaryID, "failure", map[string]string{
				"error":        err.Error(),
				"duplicate_id": req.DuplicateID,
			})
		}

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

	// Log success
	if h.AuditLogger != nil {
		h.AuditLogger.LogFromRequest(r, "admin.event.merge", "event", req.PrimaryID, "success", map[string]string{
			"duplicate_id": req.DuplicateID,
		})
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "merged"}, contentTypeFromRequest(r))
}

// DeleteEvent handles DELETE /api/v1/admin/events/{id}
// Soft-deletes an event and generates a tombstone
func (h *AdminHandler) DeleteEvent(w http.ResponseWriter, r *http.Request) {
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

	// Parse deletion reason (optional)
	var req struct {
		Reason string `json:"reason,omitempty"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	// Call admin service to delete event
	err := h.AdminService.DeleteEvent(r.Context(), ulidValue, req.Reason)
	if err != nil {
		// Log failure
		if h.AuditLogger != nil {
			details := map[string]string{
				"error": err.Error(),
			}
			if req.Reason != "" {
				details["reason"] = req.Reason
			}
			h.AuditLogger.LogFromRequest(r, "admin.event.delete", "event", ulidValue, "failure", details)
		}

		if errors.Is(err, events.ErrNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Event not found", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	// Log success
	if h.AuditLogger != nil {
		details := map[string]string{}
		if req.Reason != "" {
			details["reason"] = req.Reason
		}
		h.AuditLogger.LogFromRequest(r, "admin.event.delete", "event", ulidValue, "success", details)
	}

	w.WriteHeader(http.StatusNoContent)
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
// All text fields are sanitized to prevent XSS attacks
func mapToUpdateParams(updates map[string]any) events.UpdateEventParams {
	params := events.UpdateEventParams{}

	if name, ok := updates["name"].(string); ok {
		sanitized := sanitize.Text(name) // Remove all HTML from names
		params.Name = &sanitized
	}
	if description, ok := updates["description"].(string); ok {
		sanitized := sanitize.HTML(description) // Allow safe HTML formatting in descriptions
		params.Description = &sanitized
	}
	if lifecycleState, ok := updates["lifecycle_state"].(string); ok {
		sanitized := sanitize.Text(lifecycleState) // Plain text only
		params.LifecycleState = &sanitized
	}
	if imageURL, ok := updates["image_url"].(string); ok {
		sanitized := sanitize.Text(imageURL) // Plain text URLs
		params.ImageURL = &sanitized
	}
	if publicURL, ok := updates["public_url"].(string); ok {
		sanitized := sanitize.Text(publicURL) // Plain text URLs
		params.PublicURL = &sanitized
	}
	if eventDomain, ok := updates["event_domain"].(string); ok {
		sanitized := sanitize.Text(eventDomain) // Plain text domain
		params.EventDomain = &sanitized
	}
	if keywords, ok := updates["keywords"].([]any); ok {
		strKeywords := make([]string, 0, len(keywords))
		for _, k := range keywords {
			if s, ok := k.(string); ok {
				strKeywords = append(strKeywords, s)
			}
		}
		if len(strKeywords) > 0 {
			params.Keywords = sanitize.TextSlice(strKeywords) // Sanitize all keywords
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
