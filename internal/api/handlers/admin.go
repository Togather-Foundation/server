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
	"github.com/Togather-Foundation/server/internal/domain/organizations"
	"github.com/Togather-Foundation/server/internal/domain/places"
	"github.com/Togather-Foundation/server/internal/jsonld/schema"
	"github.com/Togather-Foundation/server/internal/sanitize"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/Togather-Foundation/server/internal/validation"
)

type AdminHandler struct {
	Service       *events.Service
	AdminService  *events.AdminService
	Places        *places.Service
	Organizations *organizations.Service
	AuditLogger   *audit.Logger
	Queries       postgres.Querier
	Env           string
	BaseURL       string
}

func NewAdminHandler(service *events.Service, adminService *events.AdminService, placeService *places.Service, orgService *organizations.Service, auditLogger *audit.Logger, queries postgres.Querier, env string, baseURL string) *AdminHandler {
	return &AdminHandler{
		Service:       service,
		AdminService:  adminService,
		Places:        placeService,
		Organizations: orgService,
		AuditLogger:   auditLogger,
		Queries:       queries,
		Env:           env,
		BaseURL:       baseURL,
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

	// Override lifecycle state to only return events pending review
	filters.LifecycleState = "pending_review"

	// Fetch pending events
	result, err := h.Service.List(r.Context(), filters, pagination)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	// adminEventItem extends EventSummary with admin-specific fields.
	type adminEventItem struct {
		schema.EventSummary
		Description    string   `json:"description,omitempty"`
		Confidence     *float64 `json:"confidence,omitempty"`
		LifecycleState string   `json:"lifecycle_state,omitempty"`
	}

	// Build response with JSON-LD context
	items := make([]adminEventItem, 0, len(result.Events))
	for _, event := range result.Events {
		item := adminEventItem{
			EventSummary: schema.EventSummary{
				Type: "Event",
				ID:   buildEventURI(h.BaseURL, event.ULID),
				Name: event.Name,
			},
			Description:    event.Description,
			Confidence:     event.Confidence,
			LifecycleState: event.LifecycleState,
		}
		items = append(items, item)
	}

	writeJSON(w, http.StatusOK, listResponse{Items: items, NextCursor: result.NextCursor}, contentTypeFromRequest(r))
}

// ListEvents handles GET /api/v1/admin/events
// Paginated list of all events for admin review
func (h *AdminHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
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

	// adminEventListItem extends EventSummary with admin-specific fields.
	type adminEventListItem struct {
		schema.EventSummary
		Description    string   `json:"description,omitempty"`
		Confidence     *float64 `json:"confidence,omitempty"`
		LifecycleState string   `json:"lifecycle_state,omitempty"`
		StartDateAdmin string   `json:"start_date,omitempty"`
	}

	items := make([]adminEventListItem, 0, len(result.Events))
	for _, event := range result.Events {
		item := adminEventListItem{
			EventSummary: schema.EventSummary{
				Type: "Event",
				ID:   buildEventURI(h.BaseURL, event.ULID),
				Name: event.Name,
			},
			Description:    event.Description,
			Confidence:     event.Confidence,
			LifecycleState: event.LifecycleState,
		}
		// Add start date from first occurrence
		if len(event.Occurrences) > 0 {
			item.StartDateAdmin = event.Occurrences[0].StartTime.Format(time.RFC3339)
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
	ulidValue, ok := ValidateAndExtractULID(w, r, "id", h.Env)
	if !ok {
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
		// Check for URL validation errors
		var urlErr validation.URLValidationError
		if errors.As(err, &urlErr) {
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

	type adminEventResponse struct {
		schema.Event
		LifecycleState string `json:"lifecycle_state,omitempty"`
	}

	event := schema.NewEvent(updated.Name)
	event.Context = loadDefaultContext()
	event.ID = buildEventURI(h.BaseURL, updated.ULID)
	event.Description = updated.Description

	resp := adminEventResponse{
		Event:          *event,
		LifecycleState: updated.LifecycleState,
	}

	writeJSON(w, http.StatusOK, resp, contentTypeFromRequest(r))
}

// PublishEvent handles POST /api/v1/admin/events/{id}/publish
// Changes lifecycle_state from draft to published
func (h *AdminHandler) PublishEvent(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.AdminService == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	// Extract and validate event ID
	ulidValue, ok := ValidateAndExtractULID(w, r, "id", h.Env)
	if !ok {
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

	type adminPublishResponse struct {
		schema.Event
		LifecycleState string `json:"lifecycle_state,omitempty"`
	}

	event := schema.NewEvent(published.Name)
	event.Context = loadDefaultContext()
	event.ID = buildEventURI(h.BaseURL, published.ULID)

	resp := adminPublishResponse{
		Event:          *event,
		LifecycleState: published.LifecycleState,
	}

	writeJSON(w, http.StatusOK, resp, contentTypeFromRequest(r))
}

// UnpublishEvent handles POST /api/v1/admin/events/{id}/unpublish
// Changes lifecycle_state from published back to draft
func (h *AdminHandler) UnpublishEvent(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.AdminService == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	// Extract and validate event ID
	ulidValue, ok := ValidateAndExtractULID(w, r, "id", h.Env)
	if !ok {
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

	type adminUnpublishResponse struct {
		schema.Event
		LifecycleState string `json:"lifecycle_state,omitempty"`
	}

	event := schema.NewEvent(unpublished.Name)
	event.Context = loadDefaultContext()
	event.ID = buildEventURI(h.BaseURL, unpublished.ULID)

	resp := adminUnpublishResponse{
		Event:          *event,
		LifecycleState: unpublished.LifecycleState,
	}

	writeJSON(w, http.StatusOK, resp, contentTypeFromRequest(r))
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
	if err := ids.ValidateULID(req.PrimaryID); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid primary_id", events.FilterError{Field: "primary_id", Message: "invalid ULID"}, h.Env)
		return
	}
	if err := ids.ValidateULID(req.DuplicateID); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid duplicate_id", events.FilterError{Field: "duplicate_id", Message: "invalid ULID"}, h.Env)
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

// ListDuplicates handles GET /api/v1/admin/duplicates
// Returns pairs of potentially duplicate events for admin review
func (h *AdminHandler) ListDuplicates(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Service == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	// TODO: Implement duplicate detection algorithm
	// For now, return empty list to fix the JSON parsing error
	// Future implementation should:
	// 1. Query events with same dedup_hash
	// 2. Use fuzzy matching for similar names/dates/locations
	// 3. Return confidence scores for each pair

	writeJSON(w, http.StatusOK, listResponse{Items: []map[string]any{}, NextCursor: ""}, contentTypeFromRequest(r))
}

// DeleteEvent handles DELETE /api/v1/admin/events/{id}
// Soft-deletes an event and generates a tombstone
func (h *AdminHandler) DeleteEvent(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.AdminService == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	// Extract and validate event ID
	ulidValue, ok := ValidateAndExtractULID(w, r, "id", h.Env)
	if !ok {
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

// DeletePlace handles DELETE /api/v1/admin/places/{id}
// Soft-deletes a place and generates a tombstone
func (h *AdminHandler) DeletePlace(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Places == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	ulidValue, ok := ValidateAndExtractULID(w, r, "id", h.Env)
	if !ok {
		return
	}

	var req struct {
		Reason string `json:"reason,omitempty"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	place, err := h.Places.GetByULID(r.Context(), ulidValue)
	if err != nil {
		if errors.Is(err, places.ErrNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Place not found", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	if err := h.Places.SoftDelete(r.Context(), ulidValue, req.Reason); err != nil {
		if errors.Is(err, places.ErrNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Place not found", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	payload, err := buildPlaceTombstonePayload(ulidValue, place.Name, req.Reason, h.BaseURL)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	params := places.TombstoneCreateParams{
		PlaceID:   place.ID,
		PlaceURI:  buildPlaceURI(h.BaseURL, ulidValue),
		DeletedAt: time.Now(),
		Reason:    req.Reason,
		Payload:   payload,
	}

	if err := h.Places.CreateTombstone(r.Context(), params); err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func buildPlaceTombstonePayload(ulid, name, reason, baseURL string) ([]byte, error) {
	placeURI := buildPlaceURI(baseURL, ulid)
	if placeURI == "" {
		placeURI = "https://togather.foundation/places/" + strings.ToUpper(ulid)
	}

	payload := map[string]any{
		"@context":           "https://schema.org",
		"@type":              "Place",
		"@id":                placeURI,
		"name":               name,
		"sel:tombstone":      true,
		"sel:deletedAt":      time.Now().Format(time.RFC3339),
		"sel:deletionReason": reason,
	}

	return json.Marshal(payload)
}

func buildPlaceURI(baseURL, ulid string) string {
	if baseURL == "" || ulid == "" {
		return ""
	}
	uri, err := ids.BuildCanonicalURI(baseURL, "places", ulid)
	if err != nil {
		return ""
	}
	return uri
}

// DeleteOrganization handles DELETE /api/v1/admin/organizations/{id}
// Soft-deletes an organization and generates a tombstone
func (h *AdminHandler) DeleteOrganization(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Organizations == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	ulidValue, ok := ValidateAndExtractULID(w, r, "id", h.Env)
	if !ok {
		return
	}

	var req struct {
		Reason string `json:"reason,omitempty"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	org, err := h.Organizations.GetByULID(r.Context(), ulidValue)
	if err != nil {
		if errors.Is(err, organizations.ErrNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Organization not found", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	if err := h.Organizations.SoftDelete(r.Context(), ulidValue, req.Reason); err != nil {
		if errors.Is(err, organizations.ErrNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Organization not found", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	payload, err := buildOrganizationTombstonePayload(ulidValue, org.Name, req.Reason, h.BaseURL)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	params := organizations.TombstoneCreateParams{
		OrgID:     org.ID,
		OrgURI:    buildOrganizationURI(h.BaseURL, ulidValue),
		DeletedAt: time.Now(),
		Reason:    req.Reason,
		Payload:   payload,
	}

	if err := h.Organizations.CreateTombstone(r.Context(), params); err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func buildOrganizationTombstonePayload(ulid, name, reason, baseURL string) ([]byte, error) {
	orgURI := buildOrganizationURI(baseURL, ulid)
	if orgURI == "" {
		orgURI = "https://togather.foundation/organizations/" + strings.ToUpper(ulid)
	}

	payload := map[string]any{
		"@context":           "https://schema.org",
		"@type":              "Organization",
		"@id":                orgURI,
		"name":               name,
		"sel:tombstone":      true,
		"sel:deletedAt":      time.Now().Format(time.RFC3339),
		"sel:deletionReason": reason,
	}

	return json.Marshal(payload)
}

func buildOrganizationURI(baseURL, ulid string) string {
	if baseURL == "" || ulid == "" {
		return ""
	}
	uri, err := ids.BuildCanonicalURI(baseURL, "organizations", ulid)
	if err != nil {
		return ""
	}
	return uri
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

// GetStats handles GET /api/v1/admin/stats
// Returns event counts by lifecycle state for dashboard performance
func (h *AdminHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Queries == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	// Get count of pending (draft) events
	pendingCount, err := h.Queries.CountEventsByLifecycleState(r.Context(), "draft")
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	// Get count of published events
	publishedCount, err := h.Queries.CountEventsByLifecycleState(r.Context(), "published")
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	// Get total count of all events (excluding deleted)
	totalCount, err := h.Queries.CountAllEvents(r.Context())
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	// Build response payload
	stats := map[string]int64{
		"pending_count":   pendingCount,
		"published_count": publishedCount,
		"total_count":     totalCount,
	}

	writeJSON(w, http.StatusOK, stats, contentTypeFromRequest(r))
}

// FindSimilarPlaces handles GET /api/v1/admin/places/{id}/similar
// Returns similar places based on name and location matching
func (h *AdminHandler) FindSimilarPlaces(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.AdminService == nil || h.Places == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	// Extract and validate place ULID
	ulidValue, ok := ValidateAndExtractULID(w, r, "id", h.Env)
	if !ok {
		return
	}

	// Look up the place to get its details
	place, err := h.Places.GetByULID(r.Context(), ulidValue)
	if err != nil {
		if errors.Is(err, places.ErrNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Place not found", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	// Call AdminService to find similar places
	candidates, err := h.AdminService.FindSimilarPlaces(r.Context(), place.Name, place.City, place.Region, 0.3)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	// Filter out the place itself from results
	filtered := make([]map[string]any, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.ULID != ulidValue {
			filtered = append(filtered, map[string]any{
				"ulid":       candidate.ULID,
				"name":       candidate.Name,
				"similarity": candidate.Similarity,
			})
		}
	}

	writeJSON(w, http.StatusOK, filtered, contentTypeFromRequest(r))
}

// FindSimilarOrganizations handles GET /api/v1/admin/organizations/{id}/similar
// Returns similar organizations based on name and location matching
func (h *AdminHandler) FindSimilarOrganizations(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.AdminService == nil || h.Organizations == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	// Extract and validate organization ULID
	ulidValue, ok := ValidateAndExtractULID(w, r, "id", h.Env)
	if !ok {
		return
	}

	// Look up the organization to get its details
	org, err := h.Organizations.GetByULID(r.Context(), ulidValue)
	if err != nil {
		if errors.Is(err, organizations.ErrNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Organization not found", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	// Use AddressLocality and AddressRegion fields from Organization struct
	locality := org.AddressLocality
	region := org.AddressRegion

	// Call AdminService to find similar organizations
	candidates, err := h.AdminService.FindSimilarOrganizations(r.Context(), org.Name, locality, region, 0.3)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	// Filter out the organization itself from results
	filtered := make([]map[string]any, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.ULID != ulidValue {
			filtered = append(filtered, map[string]any{
				"ulid":       candidate.ULID,
				"name":       candidate.Name,
				"similarity": candidate.Similarity,
			})
		}
	}

	writeJSON(w, http.StatusOK, filtered, contentTypeFromRequest(r))
}

// MergePlaces handles POST /api/v1/admin/places/merge
// Merges a duplicate place into a primary place
func (h *AdminHandler) MergePlaces(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.AdminService == nil || h.Places == nil {
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
	if err := ids.ValidateULID(req.PrimaryID); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid primary_id", err, h.Env)
		return
	}
	if err := ids.ValidateULID(req.DuplicateID); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid duplicate_id", err, h.Env)
		return
	}
	if req.PrimaryID == req.DuplicateID {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Cannot merge place with itself", nil, h.Env)
		return
	}

	// Look up both places to get their internal UUIDs
	primaryPlace, err := h.Places.GetByULID(r.Context(), req.PrimaryID)
	if err != nil {
		if errors.Is(err, places.ErrNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Primary place not found", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	duplicatePlace, err := h.Places.GetByULID(r.Context(), req.DuplicateID)
	if err != nil {
		if errors.Is(err, places.ErrNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Duplicate place not found", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	// Call AdminService to merge places (using internal UUIDs)
	result, err := h.AdminService.MergePlaces(r.Context(), duplicatePlace.ID, primaryPlace.ID)
	if err != nil {
		// Log failure
		if h.AuditLogger != nil {
			h.AuditLogger.LogFromRequest(r, "admin.place.merge", "place", req.PrimaryID, "failure", map[string]string{
				"error":        err.Error(),
				"duplicate_id": req.DuplicateID,
			})
		}

		if errors.Is(err, places.ErrNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Place not found", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	// Log success
	if h.AuditLogger != nil {
		h.AuditLogger.LogFromRequest(r, "admin.place.merge", "place", req.PrimaryID, "success", map[string]string{
			"duplicate_id": req.DuplicateID,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":         "merged",
		"canonical_id":   result.CanonicalID,
		"already_merged": result.AlreadyMerged,
	}, contentTypeFromRequest(r))
}

// MergeOrganizations handles POST /api/v1/admin/organizations/merge
// Merges a duplicate organization into a primary organization
func (h *AdminHandler) MergeOrganizations(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.AdminService == nil || h.Organizations == nil {
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
	if err := ids.ValidateULID(req.PrimaryID); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid primary_id", err, h.Env)
		return
	}
	if err := ids.ValidateULID(req.DuplicateID); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid duplicate_id", err, h.Env)
		return
	}
	if req.PrimaryID == req.DuplicateID {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Cannot merge organization with itself", nil, h.Env)
		return
	}

	// Look up both organizations to get their internal UUIDs
	primaryOrg, err := h.Organizations.GetByULID(r.Context(), req.PrimaryID)
	if err != nil {
		if errors.Is(err, organizations.ErrNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Primary organization not found", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	duplicateOrg, err := h.Organizations.GetByULID(r.Context(), req.DuplicateID)
	if err != nil {
		if errors.Is(err, organizations.ErrNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Duplicate organization not found", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	// Call AdminService to merge organizations (using internal UUIDs)
	result, err := h.AdminService.MergeOrganizations(r.Context(), duplicateOrg.ID, primaryOrg.ID)
	if err != nil {
		// Log failure
		if h.AuditLogger != nil {
			h.AuditLogger.LogFromRequest(r, "admin.organization.merge", "organization", req.PrimaryID, "failure", map[string]string{
				"error":        err.Error(),
				"duplicate_id": req.DuplicateID,
			})
		}

		if errors.Is(err, organizations.ErrNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Organization not found", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	// Log success
	if h.AuditLogger != nil {
		h.AuditLogger.LogFromRequest(r, "admin.organization.merge", "organization", req.PrimaryID, "success", map[string]string{
			"duplicate_id": req.DuplicateID,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":         "merged",
		"canonical_id":   result.CanonicalID,
		"already_merged": result.AlreadyMerged,
	}, contentTypeFromRequest(r))
}

// UpdatePlace handles PUT /api/v1/admin/places/{id}
// Allows admin to update place fields
func (h *AdminHandler) UpdatePlace(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Places == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	ulidValue, ok := ValidateAndExtractULID(w, r, "id", h.Env)
	if !ok {
		return
	}

	var updates map[string]any
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", err, h.Env)
		return
	}

	params := mapToPlaceUpdateParams(updates)

	updated, err := h.Places.Update(r.Context(), ulidValue, params)
	if err != nil {
		if h.AuditLogger != nil {
			h.AuditLogger.LogFromRequest(r, "admin.place.update", "place", ulidValue, "failure", map[string]string{
				"error": err.Error(),
			})
		}

		if errors.Is(err, places.ErrNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Place not found", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	if h.AuditLogger != nil {
		h.AuditLogger.LogFromRequest(r, "admin.place.update", "place", ulidValue, "success", nil)
	}

	resp := map[string]any{
		"@id":   buildPlaceURI(h.BaseURL, updated.ULID),
		"@type": "Place",
		"name":  updated.Name,
	}
	if updated.Description != "" {
		resp["description"] = updated.Description
	}
	if updated.URL != "" {
		resp["url"] = updated.URL
	}
	if updated.Telephone != "" {
		resp["telephone"] = updated.Telephone
	}
	if updated.Email != "" {
		resp["email"] = updated.Email
	}
	address := map[string]string{"@type": "PostalAddress"}
	if updated.StreetAddress != "" {
		address["streetAddress"] = updated.StreetAddress
	}
	if updated.City != "" {
		address["addressLocality"] = updated.City
	}
	if updated.Region != "" {
		address["addressRegion"] = updated.Region
	}
	if updated.PostalCode != "" {
		address["postalCode"] = updated.PostalCode
	}
	if updated.Country != "" {
		address["addressCountry"] = updated.Country
	}
	if len(address) > 1 {
		resp["address"] = address
	}

	writeJSON(w, http.StatusOK, resp, contentTypeFromRequest(r))
}

// mapToPlaceUpdateParams converts a map[string]any to UpdatePlaceParams.
// All text fields are sanitized to prevent XSS attacks.
func mapToPlaceUpdateParams(updates map[string]any) places.UpdatePlaceParams {
	params := places.UpdatePlaceParams{}

	if name, ok := updates["name"].(string); ok {
		sanitized := sanitize.Text(name)
		params.Name = &sanitized
	}
	if description, ok := updates["description"].(string); ok {
		sanitized := sanitize.HTML(description)
		params.Description = &sanitized
	}
	if street, ok := updates["street_address"].(string); ok {
		sanitized := sanitize.Text(street)
		params.StreetAddress = &sanitized
	}
	if city, ok := updates["city"].(string); ok {
		sanitized := sanitize.Text(city)
		params.City = &sanitized
	}
	if region, ok := updates["region"].(string); ok {
		sanitized := sanitize.Text(region)
		params.Region = &sanitized
	}
	if postalCode, ok := updates["postal_code"].(string); ok {
		sanitized := sanitize.Text(postalCode)
		params.PostalCode = &sanitized
	}
	if country, ok := updates["country"].(string); ok {
		sanitized := sanitize.Text(country)
		params.Country = &sanitized
	}
	if telephone, ok := updates["telephone"].(string); ok {
		sanitized := sanitize.Text(telephone)
		params.Telephone = &sanitized
	}
	if email, ok := updates["email"].(string); ok {
		sanitized := sanitize.Text(email)
		params.Email = &sanitized
	}
	if url, ok := updates["url"].(string); ok {
		sanitized := sanitize.Text(url)
		params.URL = &sanitized
	}

	return params
}

// UpdateOrganization handles PUT /api/v1/admin/organizations/{id}
// Allows admin to update organization fields
func (h *AdminHandler) UpdateOrganization(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Organizations == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	ulidValue, ok := ValidateAndExtractULID(w, r, "id", h.Env)
	if !ok {
		return
	}

	var updates map[string]any
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", err, h.Env)
		return
	}

	params := mapToOrgUpdateParams(updates)

	updated, err := h.Organizations.Update(r.Context(), ulidValue, params)
	if err != nil {
		if h.AuditLogger != nil {
			h.AuditLogger.LogFromRequest(r, "admin.organization.update", "organization", ulidValue, "failure", map[string]string{
				"error": err.Error(),
			})
		}

		if errors.Is(err, organizations.ErrNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Organization not found", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	if h.AuditLogger != nil {
		h.AuditLogger.LogFromRequest(r, "admin.organization.update", "organization", ulidValue, "success", nil)
	}

	resp := map[string]any{
		"@id":   buildOrganizationURI(h.BaseURL, updated.ULID),
		"@type": "Organization",
		"name":  updated.Name,
	}
	if updated.Description != "" {
		resp["description"] = updated.Description
	}
	if updated.URL != "" {
		resp["url"] = updated.URL
	}
	if updated.Telephone != "" {
		resp["telephone"] = updated.Telephone
	}
	if updated.Email != "" {
		resp["email"] = updated.Email
	}
	address := map[string]string{"@type": "PostalAddress"}
	if updated.StreetAddress != "" {
		address["streetAddress"] = updated.StreetAddress
	}
	if updated.AddressLocality != "" {
		address["addressLocality"] = updated.AddressLocality
	}
	if updated.AddressRegion != "" {
		address["addressRegion"] = updated.AddressRegion
	}
	if updated.PostalCode != "" {
		address["postalCode"] = updated.PostalCode
	}
	if updated.AddressCountry != "" {
		address["addressCountry"] = updated.AddressCountry
	}
	if len(address) > 1 {
		resp["address"] = address
	}

	writeJSON(w, http.StatusOK, resp, contentTypeFromRequest(r))
}

// mapToOrgUpdateParams converts a map[string]any to UpdateOrganizationParams.
// All text fields are sanitized to prevent XSS attacks.
func mapToOrgUpdateParams(updates map[string]any) organizations.UpdateOrganizationParams {
	params := organizations.UpdateOrganizationParams{}

	if name, ok := updates["name"].(string); ok {
		sanitized := sanitize.Text(name)
		params.Name = &sanitized
	}
	if description, ok := updates["description"].(string); ok {
		sanitized := sanitize.HTML(description)
		params.Description = &sanitized
	}
	if street, ok := updates["street_address"].(string); ok {
		sanitized := sanitize.Text(street)
		params.StreetAddress = &sanitized
	}
	if locality, ok := updates["address_locality"].(string); ok {
		sanitized := sanitize.Text(locality)
		params.AddressLocality = &sanitized
	}
	if region, ok := updates["address_region"].(string); ok {
		sanitized := sanitize.Text(region)
		params.AddressRegion = &sanitized
	}
	if postalCode, ok := updates["postal_code"].(string); ok {
		sanitized := sanitize.Text(postalCode)
		params.PostalCode = &sanitized
	}
	if country, ok := updates["address_country"].(string); ok {
		sanitized := sanitize.Text(country)
		params.AddressCountry = &sanitized
	}
	if telephone, ok := updates["telephone"].(string); ok {
		sanitized := sanitize.Text(telephone)
		params.Telephone = &sanitized
	}
	if email, ok := updates["email"].(string); ok {
		sanitized := sanitize.Text(email)
		params.Email = &sanitized
	}
	if url, ok := updates["url"].(string); ok {
		sanitized := sanitize.Text(url)
		params.URL = &sanitized
	}

	return params
}
