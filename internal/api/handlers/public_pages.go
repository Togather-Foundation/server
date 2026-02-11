package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/api/render"
	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/domain/organizations"
	"github.com/Togather-Foundation/server/internal/domain/places"
	"github.com/Togather-Foundation/server/internal/jsonld"
	"github.com/Togather-Foundation/server/internal/jsonld/schema"
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
	ulidValue, ok := ValidateAndExtractULID(w, r, "id", h.Env)
	if !ok {
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
	ulidValue, ok := ValidateAndExtractULID(w, r, "id", h.Env)
	if !ok {
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
	ulidValue, ok := ValidateAndExtractULID(w, r, "id", h.Env)
	if !ok {
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
//
// Current implementation uses simple string matching to determine content type preference.
// Supported formats (in priority order):
//  1. text/html - Human-readable HTML with embedded JSON-LD
//  2. text/turtle - RDF Turtle serialization
//  3. application/ld+json, application/json, */* - JSON-LD (default)
//
// Limitations:
//   - Does not parse quality values (q parameters) from Accept header
//   - Uses first-match logic rather than RFC 7231 content negotiation
//   - Example: "Accept: text/html;q=0.8, application/ld+json;q=0.9" will serve HTML
//     (incorrectly prefers HTML over higher-quality JSON-LD)
//
// This simplified approach is sufficient for most SEL clients. For stricter RFC 7231
// compliance, consider using a proper Accept header parser (e.g., github.com/elnormous/contenttype).
func (h *PublicPagesHandler) serveWithContentNegotiation(w http.ResponseWriter, r *http.Request, payload map[string]any) {
	accept := r.Header.Get("Accept")

	// Determine preferred format using first-match logic (does not parse q values)
	// Priority: HTML > Turtle > JSON-LD (default)
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
		typeVal := extractType(payload)
		err = fmt.Errorf("failed to serialize %s to Turtle: %w", typeVal, err)
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

func buildEventPayload(event *events.Event, baseURL string) map[string]any {
	ev := schema.NewEvent(event.Name)
	ev.Context = loadDefaultContext()
	ev.ID = buildEventURI(baseURL, event.ULID)
	ev.Description = event.Description

	// Extract startDate from first occurrence if available
	if len(event.Occurrences) > 0 && !event.Occurrences[0].StartTime.IsZero() {
		ev.StartDate = event.Occurrences[0].StartTime.Format(time.RFC3339)
	}

	// Add location if venue ID is available
	if event.PrimaryVenueID != nil && *event.PrimaryVenueID != "" {
		ev.Location = map[string]any{
			"@type": "Place",
			"@id":   schema.BuildPlaceURI(baseURL, *event.PrimaryVenueID),
		}
	}

	// Add organizer if organizer ID is available
	if event.OrganizerID != nil && *event.OrganizerID != "" {
		ev.Organizer = map[string]any{
			"@type": "Organization",
			"@id":   schema.BuildOrganizationURI(baseURL, *event.OrganizerID),
		}
	}

	return structToMap(ev)
}

func buildPlacePayload(place *places.Place, baseURL string) map[string]any {
	p := schema.NewPlace(place.Name)
	p.Context = loadDefaultContext()
	p.ID = schema.BuildPlaceURI(baseURL, place.ULID)
	p.Address = schema.NewPostalAddress(place.StreetAddress, place.City, place.Region, place.PostalCode, place.Country)
	if place.Latitude != nil && place.Longitude != nil {
		p.Geo = schema.NewGeoCoordinates(*place.Latitude, *place.Longitude)
	}

	return structToMap(p)
}

func buildOrganizationPayload(org *organizations.Organization, baseURL string) map[string]any {
	o := schema.NewOrganization(org.Name)
	o.Context = loadDefaultContext()
	o.ID = schema.BuildOrganizationURI(baseURL, org.ULID)
	o.Description = org.Description
	o.URL = org.URL

	return structToMap(o)
}

// structToMap converts a typed struct to map[string]any via JSON round-trip.
// This is needed because the render package expects map[string]any for template data.
func structToMap(v any) map[string]any {
	data, err := json.Marshal(v)
	if err != nil {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return map[string]any{}
	}
	return m
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
