package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/api/render"
	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/domain/ids"
	"github.com/Togather-Foundation/server/internal/domain/organizations"
	"github.com/Togather-Foundation/server/internal/domain/places"
	"github.com/Togather-Foundation/server/internal/jsonld"
)

// PublicPagesHandler handles dereferenceable URIs with content negotiation.
// It serves Events, Places, and Organizations at /events/{id}, /places/{id}, /organizations/{id}
// with support for text/html, application/ld+json, and text/turtle formats.
type PublicPagesHandler struct {
	EventsService        *events.Service
	PlacesService        *places.Service
	OrganizationsService *organizations.Service
	Env                  string
	BaseURL              string
}

// NewPublicPagesHandler creates a new PublicPagesHandler.
func NewPublicPagesHandler(
	eventsService *events.Service,
	placesService *places.Service,
	organizationsService *organizations.Service,
	env string,
	baseURL string,
) *PublicPagesHandler {
	return &PublicPagesHandler{
		EventsService:        eventsService,
		PlacesService:        placesService,
		OrganizationsService: organizationsService,
		Env:                  env,
		BaseURL:              baseURL,
	}
}

// GetEvent handles GET /events/{id} with content negotiation.
func (h *PublicPagesHandler) GetEvent(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.EventsService == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	// Extract and validate ULID
	ulidValue := strings.TrimSpace(pathParam(r, "id"))
	if ulidValue == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", events.FilterError{Field: "id", Message: "missing"}, h.Env)
		return
	}
	if err := ids.ValidateULID(ulidValue); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", events.FilterError{Field: "id", Message: "invalid ULID"}, h.Env)
		return
	}

	// Fetch event data
	item, err := h.EventsService.GetByULID(r.Context(), ulidValue)
	if err != nil {
		if errors.Is(err, events.ErrNotFound) {
			// Check for tombstone
			tombstone, tombErr := h.EventsService.GetTombstoneByEventULID(r.Context(), ulidValue)
			if tombErr == nil && tombstone != nil {
				h.serveEventTombstone(w, r, tombstone)
				return
			}
			// No tombstone found, return 404
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Not found", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	// Check if event is soft-deleted
	if item.LifecycleState == "deleted" {
		tombstone, tombErr := h.EventsService.GetTombstoneByEventULID(r.Context(), ulidValue)
		if tombErr == nil && tombstone != nil {
			h.serveEventTombstone(w, r, tombstone)
			return
		}
		// No tombstone found for deleted event, return 410 with minimal info
		problem.Write(w, r, http.StatusGone, "https://sel.events/problems/gone", "Resource deleted", nil, h.Env)
		return
	}

	// Build JSON-LD payload
	payload := buildEventPayload(item, h.BaseURL)

	// Perform content negotiation
	h.serveWithContentNegotiation(w, r, payload)
}

// GetPlace handles GET /places/{id} with content negotiation.
func (h *PublicPagesHandler) GetPlace(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.PlacesService == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	// Extract and validate ULID
	ulidValue := strings.TrimSpace(pathParam(r, "id"))
	if ulidValue == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", places.FilterError{Field: "id", Message: "missing"}, h.Env)
		return
	}
	if err := ids.ValidateULID(ulidValue); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", places.FilterError{Field: "id", Message: "invalid ULID"}, h.Env)
		return
	}

	// Fetch place data
	item, err := h.PlacesService.GetByULID(r.Context(), ulidValue)
	if err != nil {
		if errors.Is(err, places.ErrNotFound) {
			// Check for tombstone
			tombstone, tombErr := h.PlacesService.GetTombstoneByULID(r.Context(), ulidValue)
			if tombErr == nil && tombstone != nil {
				h.servePlaceTombstone(w, r, tombstone)
				return
			}
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Not found", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	// Check if place is soft-deleted
	if item.Lifecycle == "deleted" {
		tombstone, tombErr := h.PlacesService.GetTombstoneByULID(r.Context(), ulidValue)
		if tombErr == nil && tombstone != nil {
			h.servePlaceTombstone(w, r, tombstone)
			return
		}
		// No tombstone found for deleted place, return 410 with minimal info
		problem.Write(w, r, http.StatusGone, "https://sel.events/problems/gone", "Resource deleted", nil, h.Env)
		return
	}

	// Build JSON-LD payload
	payload := buildPlacePayload(item, h.BaseURL)

	// Perform content negotiation
	h.serveWithContentNegotiation(w, r, payload)
}

// GetOrganization handles GET /organizations/{id} with content negotiation.
func (h *PublicPagesHandler) GetOrganization(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.OrganizationsService == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	// Extract and validate ULID
	ulidValue := strings.TrimSpace(pathParam(r, "id"))
	if ulidValue == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", organizations.FilterError{Field: "id", Message: "missing"}, h.Env)
		return
	}
	if err := ids.ValidateULID(ulidValue); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", organizations.FilterError{Field: "id", Message: "invalid ULID"}, h.Env)
		return
	}

	// Fetch organization data
	item, err := h.OrganizationsService.GetByULID(r.Context(), ulidValue)
	if err != nil {
		if errors.Is(err, organizations.ErrNotFound) {
			// Check for tombstone
			tombstone, tombErr := h.OrganizationsService.GetTombstoneByULID(r.Context(), ulidValue)
			if tombErr == nil && tombstone != nil {
				h.serveOrganizationTombstone(w, r, tombstone)
				return
			}
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Not found", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	// Check if organization is soft-deleted
	if item.Lifecycle == "deleted" {
		tombstone, tombErr := h.OrganizationsService.GetTombstoneByULID(r.Context(), ulidValue)
		if tombErr == nil && tombstone != nil {
			h.serveOrganizationTombstone(w, r, tombstone)
			return
		}
		// No tombstone found for deleted organization, return 410 with minimal info
		problem.Write(w, r, http.StatusGone, "https://sel.events/problems/gone", "Resource deleted", nil, h.Env)
		return
	}

	// Build JSON-LD payload
	payload := buildOrganizationPayload(item, h.BaseURL)

	// Perform content negotiation
	h.serveWithContentNegotiation(w, r, payload)
}

// serveWithContentNegotiation performs content negotiation based on Accept header.
func (h *PublicPagesHandler) serveWithContentNegotiation(w http.ResponseWriter, r *http.Request, payload map[string]any) {
	accept := r.Header.Get("Accept")

	// Determine preferred format
	switch {
	case strings.Contains(accept, "text/html"):
		// Serve HTML with embedded JSON-LD
		h.serveHTML(w, r, payload)
	case strings.Contains(accept, "text/turtle"):
		// Serve Turtle RDF
		h.serveTurtle(w, r, payload)
	case strings.Contains(accept, "application/ld+json"), strings.Contains(accept, "application/json"), accept == "*/*", accept == "":
		// Serve JSON-LD (default)
		h.serveJSONLD(w, payload)
	default:
		// Default to JSON-LD
		h.serveJSONLD(w, payload)
	}
}

// serveHTML renders HTML with embedded JSON-LD.
func (h *PublicPagesHandler) serveHTML(w http.ResponseWriter, r *http.Request, payload map[string]any) {
	h.serveHTMLWithStatus(w, r, payload, http.StatusOK)
}

// serveHTMLWithStatus renders HTML with embedded JSON-LD and a specific status code.
func (h *PublicPagesHandler) serveHTMLWithStatus(w http.ResponseWriter, r *http.Request, payload map[string]any, statusCode int) {
	var html string
	var err error

	// Determine type and render accordingly
	typeVal := extractType(payload)
	switch typeVal {
	case "Event":
		html, err = render.RenderEventHTML(payload)
	case "Place":
		html, err = render.RenderPlaceHTML(payload)
	case "Organization":
		html, err = render.RenderOrganizationHTML(payload)
	default:
		// Fallback to event rendering
		html, err = render.RenderEventHTML(payload)
	}

	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(statusCode)
	_, _ = w.Write([]byte(html))
}

// serveTurtle renders Turtle RDF.
func (h *PublicPagesHandler) serveTurtle(w http.ResponseWriter, r *http.Request, payload map[string]any) {
	h.serveTurtleWithStatus(w, r, payload, http.StatusOK)
}

// serveTurtleWithStatus renders Turtle RDF with a specific status code.
func (h *PublicPagesHandler) serveTurtleWithStatus(w http.ResponseWriter, r *http.Request, payload map[string]any, statusCode int) {
	turtle, err := jsonld.SerializeToTurtle(payload)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	w.Header().Set("Content-Type", "text/turtle; charset=utf-8")
	w.WriteHeader(statusCode)
	_, _ = w.Write([]byte(turtle))
}

// serveJSONLD renders JSON-LD.
func (h *PublicPagesHandler) serveJSONLD(w http.ResponseWriter, payload map[string]any) {
	w.Header().Set("Content-Type", "application/ld+json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(payload)
}

// serveTombstoneHelper is a generic helper for serving tombstones with content negotiation.
// Reduces duplication across serveEventTombstone, servePlaceTombstone, serveOrganizationTombstone.
func (h *PublicPagesHandler) serveTombstoneHelper(w http.ResponseWriter, r *http.Request, deletedAt time.Time, payloadBytes json.RawMessage, entityType string) {
	var payload map[string]any
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		payload = map[string]any{
			"@context":      loadDefaultContext(),
			"@type":         entityType,
			"sel:tombstone": true,
			"sel:deletedAt": deletedAt.Format(time.RFC3339),
		}
	}

	// Ensure tombstone fields are present
	if _, ok := payload["sel:tombstone"]; !ok {
		payload["sel:tombstone"] = true
	}
	if _, ok := payload["sel:deletedAt"]; !ok {
		payload["sel:deletedAt"] = deletedAt.Format(time.RFC3339)
	}

	// Perform content negotiation for tombstone
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "text/html") {
		h.serveHTMLWithStatus(w, r, payload, http.StatusGone)
	} else if strings.Contains(accept, "text/turtle") {
		h.serveTurtleWithStatus(w, r, payload, http.StatusGone)
	} else {
		w.Header().Set("Content-Type", "application/ld+json; charset=utf-8")
		w.WriteHeader(http.StatusGone)
		_ = json.NewEncoder(w).Encode(payload)
	}
}

// serveEventTombstone serves a 410 Gone response for event tombstones.
func (h *PublicPagesHandler) serveEventTombstone(w http.ResponseWriter, r *http.Request, tombstone *events.Tombstone) {
	h.serveTombstoneHelper(w, r, tombstone.DeletedAt, tombstone.Payload, "Event")
}

// servePlaceTombstone serves a 410 Gone response for place tombstones.
func (h *PublicPagesHandler) servePlaceTombstone(w http.ResponseWriter, r *http.Request, tombstone *places.Tombstone) {
	h.serveTombstoneHelper(w, r, tombstone.DeletedAt, tombstone.Payload, "Place")
}

// serveOrganizationTombstone serves a 410 Gone response for organization tombstones.
func (h *PublicPagesHandler) serveOrganizationTombstone(w http.ResponseWriter, r *http.Request, tombstone *organizations.Tombstone) {
	h.serveTombstoneHelper(w, r, tombstone.DeletedAt, tombstone.Payload, "Organization")
}

// Helper functions to build payloads

// buildBasePayload creates a JSON-LD payload with common @context, @type, and @id fields.
// Additional entity-specific fields can be added to the returned map.
func buildBasePayload(entityType string, uri string) map[string]any {
	return map[string]any{
		"@context": loadDefaultContext(),
		"@type":    entityType,
		"@id":      uri,
	}
}

func buildEventPayload(event *events.Event, baseURL string) map[string]any {
	payload := buildBasePayload("Event", buildEventURI(baseURL, event.ULID))
	payload["name"] = event.Name

	// Extract startDate from first occurrence if available
	if len(event.Occurrences) > 0 && !event.Occurrences[0].StartTime.IsZero() {
		payload["startDate"] = event.Occurrences[0].StartTime.Format(time.RFC3339)
	}

	if event.Description != "" {
		payload["description"] = event.Description
	}

	// Add location if venue ID is available
	if event.PrimaryVenueID != nil && *event.PrimaryVenueID != "" {
		payload["location"] = map[string]any{
			"@type": "Place",
			"@id":   buildPlaceURI(baseURL, *event.PrimaryVenueID),
		}
	}

	// Add organizer if organizer ID is available
	if event.OrganizerID != nil && *event.OrganizerID != "" {
		payload["organizer"] = map[string]any{
			"@type": "Organization",
			"@id":   buildOrganizationURI(baseURL, *event.OrganizerID),
		}
	}

	return payload
}

func buildPlacePayload(place *places.Place, baseURL string) map[string]any {
	payload := buildBasePayload("Place", buildPlaceURI(baseURL, place.ULID))
	payload["name"] = place.Name

	// Build address from City, Region, Country fields
	if place.City != "" || place.Region != "" || place.Country != "" {
		address := map[string]any{
			"@type": "PostalAddress",
		}
		if place.City != "" {
			address["addressLocality"] = place.City
		}
		if place.Region != "" {
			address["addressRegion"] = place.Region
		}
		if place.Country != "" {
			address["addressCountry"] = place.Country
		}
		payload["address"] = address
	}

	return payload
}

func buildOrganizationPayload(org *organizations.Organization, baseURL string) map[string]any {
	payload := buildBasePayload("Organization", buildOrganizationURI(baseURL, org.ULID))
	payload["name"] = org.Name

	if org.Description != "" {
		payload["description"] = org.Description
	}
	if org.URL != "" {
		payload["url"] = org.URL
	}

	return payload
}

func extractType(payload map[string]any) string {
	if typeVal, ok := payload["@type"].(string); ok {
		return typeVal
	}
	if typeVal, ok := payload["type"].(string); ok {
		return typeVal
	}
	return ""
}
