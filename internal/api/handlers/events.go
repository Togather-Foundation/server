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
	"github.com/Togather-Foundation/server/internal/domain/provenance"
)

type EventsHandler struct {
	Service           *events.Service
	Ingest            *events.IngestService
	ProvenanceService *provenance.Service
	Env               string
	BaseURL           string
}

func NewEventsHandler(service *events.Service, ingest *events.IngestService, provenanceService *provenance.Service, env string, baseURL string) *EventsHandler {
	return &EventsHandler{
		Service:           service,
		Ingest:            ingest,
		ProvenanceService: provenanceService,
		Env:               env,
		BaseURL:           baseURL,
	}
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
		item := map[string]any{
			"@context": contextValue,
			"@type":    "Event",
			"name":     event.Name,
		}

		// Add @id (required per Interop Profile §3.1)
		if uri, err := ids.BuildCanonicalURI(h.BaseURL, "events", event.ULID); err == nil {
			item["@id"] = uri
		}

		// Add startDate (required per Interop Profile §3.1)
		if len(event.Occurrences) > 0 {
			item["startDate"] = event.Occurrences[0].StartTime.Format(time.RFC3339)
		}

		// Add location (required per Interop Profile §3.1)
		// Use URI reference for Place if venue is available
		if len(event.Occurrences) > 0 && event.Occurrences[0].VenueID != nil {
			if placeURI, err := ids.BuildCanonicalURI(h.BaseURL, "places", *event.Occurrences[0].VenueID); err == nil {
				item["location"] = placeURI
			}
		} else if event.PrimaryVenueID != nil {
			if placeURI, err := ids.BuildCanonicalURI(h.BaseURL, "places", *event.PrimaryVenueID); err == nil {
				item["location"] = placeURI
			}
		} else if len(event.Occurrences) > 0 && event.Occurrences[0].VirtualURL != nil && *event.Occurrences[0].VirtualURL != "" {
			// Virtual event
			item["location"] = map[string]any{
				"@type": "VirtualLocation",
				"url":   *event.Occurrences[0].VirtualURL,
			}
		} else if event.VirtualURL != "" {
			item["location"] = map[string]any{
				"@type": "VirtualLocation",
				"url":   event.VirtualURL,
			}
		}

		items = append(items, item)
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
			// Check if there's a tombstone for this event
			tombstone, tombErr := h.Service.GetTombstoneByEventULID(r.Context(), ulidValue)
			if tombErr == nil && tombstone != nil {
				// Return 410 Gone with tombstone payload
				var payload map[string]any
				if err := json.Unmarshal(tombstone.Payload, &payload); err != nil {
					// Fallback to minimal tombstone if payload parsing fails
					payload = map[string]any{
						"@context":      loadDefaultContext(),
						"@type":         "Event",
						"sel:tombstone": true,
						"sel:deletedAt": tombstone.DeletedAt.Format(time.RFC3339),
					}
				}
				writeJSON(w, http.StatusGone, payload, contentTypeFromRequest(r))
				return
			}
			// No tombstone found, return 404
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Not found", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	if strings.EqualFold(item.LifecycleState, "deleted") {
		tombstone, tombErr := h.Service.GetTombstoneByEventULID(r.Context(), ulidValue)
		if tombErr == nil && tombstone != nil {
			var payload map[string]any
			if err := json.Unmarshal(tombstone.Payload, &payload); err != nil {
				payload = map[string]any{
					"@context":      loadDefaultContext(),
					"@type":         "Event",
					"sel:tombstone": true,
					"sel:deletedAt": tombstone.DeletedAt.Format(time.RFC3339),
				}
			}
			writeJSON(w, http.StatusGone, payload, contentTypeFromRequest(r))
			return
		}

		payload := map[string]any{
			"@context":      loadDefaultContext(),
			"@type":         "Event",
			"sel:tombstone": true,
			"sel:deletedAt": time.Now().Format(time.RFC3339),
		}
		if uri, err := ids.BuildCanonicalURI(h.BaseURL, "events", ulidValue); err == nil {
			payload["@id"] = uri
		}
		writeJSON(w, http.StatusGone, payload, contentTypeFromRequest(r))
		return
	}

	contextValue := loadDefaultContext()
	payload := map[string]any{
		"@context": contextValue,
		"@type":    "Event",
		"@id":      buildEventURI(h.BaseURL, item.ULID),
		"name":     item.Name,
	}

	// Add startDate (required per Interop Profile §3.1)
	if len(item.Occurrences) > 0 {
		payload["startDate"] = item.Occurrences[0].StartTime.Format(time.RFC3339)
	}

	// Add location (required per Interop Profile §3.1)
	if len(item.Occurrences) > 0 && item.Occurrences[0].VenueID != nil {
		if placeURI, err := ids.BuildCanonicalURI(h.BaseURL, "places", *item.Occurrences[0].VenueID); err == nil {
			payload["location"] = placeURI
		}
	} else if item.PrimaryVenueID != nil {
		if placeURI, err := ids.BuildCanonicalURI(h.BaseURL, "places", *item.PrimaryVenueID); err == nil {
			payload["location"] = placeURI
		}
	} else if len(item.Occurrences) > 0 && item.Occurrences[0].VirtualURL != nil && *item.Occurrences[0].VirtualURL != "" {
		payload["location"] = map[string]any{
			"@type": "VirtualLocation",
			"url":   *item.Occurrences[0].VirtualURL,
		}
	} else if item.VirtualURL != "" {
		payload["location"] = map[string]any{
			"@type": "VirtualLocation",
			"url":   item.VirtualURL,
		}
	}

	// Add license information per FR-024
	if item.LicenseURL != "" {
		payload["license"] = item.LicenseURL
	} else {
		// Default to CC0 if not specified
		payload["license"] = "https://creativecommons.org/publicdomain/zero/1.0/"
	}

	// Add federation URI as sameAs if present (federated events)
	if item.FederationURI != nil && *item.FederationURI != "" {
		payload["sameAs"] = *item.FederationURI
	}

	// Check if provenance is requested (FR-029, US5)
	includeProvenance := strings.EqualFold(r.URL.Query().Get("include_provenance"), "true")
	provenanceFields := parseProvenanceFields(r.URL.Query().Get("provenance_fields"))

	if includeProvenance && h.ProvenanceService != nil {
		// Get source attribution
		sources, err := h.ProvenanceService.GetEventSourceAttribution(r.Context(), item.ID)
		if err == nil && len(sources) > 0 {
			payload["sources"] = buildSourcesPayload(sources)
		}

		// Get field-level provenance if requested
		var fieldProvenance []provenance.FieldProvenanceInfo
		if len(provenanceFields) > 0 {
			fieldProvenance, err = h.ProvenanceService.GetFieldProvenance(r.Context(), item.ID, provenanceFields)
		} else {
			fieldProvenance, err = h.ProvenanceService.GetFieldProvenance(r.Context(), item.ID, nil)
		}

		if err == nil && len(fieldProvenance) > 0 {
			payload["_provenance"] = buildFieldProvenancePayload(fieldProvenance)
		}
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

// parseProvenanceFields parses comma-separated field paths from query parameter
// Validates field paths per JSON Pointer spec (RFC 6901)
func parseProvenanceFields(fieldsParam string) []string {
	if fieldsParam == "" {
		return nil
	}
	fields := strings.Split(fieldsParam, ",")
	result := make([]string, 0, len(fields))
	for _, f := range fields {
		field := strings.TrimSpace(f)
		if field == "" {
			continue
		}

		// Ensure field path starts with "/"
		if !strings.HasPrefix(field, "/") {
			field = "/" + field
		}

		// Validate field path format per JSON Pointer (RFC 6901)
		if !isValidFieldPath(field) {
			continue // Skip invalid paths
		}

		result = append(result, field)
	}
	return result
}

// isValidFieldPath validates field path per JSON Pointer spec (RFC 6901)
// and prevents common security issues
func isValidFieldPath(path string) bool {
	if path == "" || path == "/" {
		return false
	}

	// Must start with /
	if !strings.HasPrefix(path, "/") {
		return false
	}

	// Prevent directory traversal
	if strings.Contains(path, "..") {
		return false
	}

	// Limit path depth to prevent abuse
	const maxDepth = 5
	segments := strings.Split(path[1:], "/") // Skip leading /
	if len(segments) > maxDepth {
		return false
	}

	// Validate each segment
	for _, segment := range segments {
		if segment == "" {
			return false // No empty segments
		}
		// Allow alphanumeric, underscore, hyphen, and tilde (JSON Pointer escape char)
		for _, ch := range segment {
			if !isValidFieldChar(ch) {
				return false
			}
		}
	}

	return true
}

// isValidFieldChar checks if character is valid in field path
func isValidFieldChar(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') ||
		(ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') ||
		ch == '_' || ch == '-' || ch == '~'
}

// buildSourcesPayload constructs sources attribution array for JSON-LD response (FR-024)
func buildSourcesPayload(sources []provenance.EventSourceAttribution) []map[string]any {
	result := make([]map[string]any, 0, len(sources))
	for _, src := range sources {
		source := map[string]any{
			"@type":   "DataFeed",
			"name":    src.SourceName,
			"url":     src.SourceURL,
			"license": src.LicenseURL,
			"provider": map[string]any{
				"@type": "Organization",
				"name":  src.SourceName,
			},
		}

		// Add optional fields
		if src.Confidence != nil {
			source["sel:confidence"] = *src.Confidence
		}
		if src.SourceEventID != nil {
			source["sel:sourceEventId"] = *src.SourceEventID
		}

		// Add timestamps (FR-029 - dual timestamp tracking)
		source["sel:retrievedAt"] = src.RetrievedAt.Format(time.RFC3339)

		result = append(result, source)
	}
	return result
}

// buildFieldProvenancePayload constructs field-level provenance map for JSON-LD response
func buildFieldProvenancePayload(provenanceInfo []provenance.FieldProvenanceInfo) map[string]any {
	// Group provenance by field path
	grouped := provenance.GroupProvenanceByField(provenanceInfo)

	result := make(map[string]any)
	for fieldPath, entries := range grouped {
		// Remove leading "/" for JSON key (e.g., "/name" becomes "name")
		key := strings.TrimPrefix(fieldPath, "/")

		fieldInfo := make([]map[string]any, 0, len(entries))
		for _, entry := range entries {
			info := map[string]any{
				"source": map[string]any{
					"name":       entry.SourceName,
					"type":       entry.SourceType,
					"trustLevel": entry.TrustLevel,
					"license":    entry.LicenseURL,
				},
				"confidence": entry.Confidence,
				"observedAt": entry.ObservedAt.Format(time.RFC3339),
			}

			if entry.ValuePreview != nil {
				info["valuePreview"] = *entry.ValuePreview
			}

			fieldInfo = append(fieldInfo, info)
		}

		// If only one entry, unwrap the array
		if len(fieldInfo) == 1 {
			result[key] = fieldInfo[0]
		} else {
			result[key] = fieldInfo
		}
	}

	return result
}
