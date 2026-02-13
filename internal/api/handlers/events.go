package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/domain/ids"
	"github.com/Togather-Foundation/server/internal/domain/provenance"
	"github.com/Togather-Foundation/server/internal/jobs"
	"github.com/Togather-Foundation/server/internal/jsonld/schema"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

type EventsHandler struct {
	Service           *events.Service
	Ingest            *events.IngestService
	ProvenanceService *provenance.Service
	RiverClient       *river.Client[pgx.Tx]
	Queries           *postgres.Queries
	Env               string
	BaseURL           string
}

func NewEventsHandler(service *events.Service, ingest *events.IngestService, provenanceService *provenance.Service, riverClient *river.Client[pgx.Tx], queries *postgres.Queries, env string, baseURL string) *EventsHandler {
	return &EventsHandler{
		Service:           service,
		Ingest:            ingest,
		ProvenanceService: provenanceService,
		RiverClient:       riverClient,
		Queries:           queries,
		Env:               env,
		BaseURL:           baseURL,
	}
}

type listResponse struct {
	Items      any    `json:"items"` // Accepts any slice type for JSON encoding
	NextCursor string `json:"next_cursor"`
	Total      int64  `json:"total,omitempty"` // Optional: total count for filtered results
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
	items := make([]*schema.EventSummary, 0, len(result.Events))
	for _, event := range result.Events {
		item := schema.NewEventSummary(event.Name)
		item.Context = contextValue
		item.ID = schema.BuildEventURI(h.BaseURL, event.ULID)

		// Add startDate (required per Interop Profile §3.1)
		if len(event.Occurrences) > 0 {
			item.StartDate = event.Occurrences[0].StartTime.Format(time.RFC3339)
			if event.Occurrences[0].EndTime != nil {
				item.EndDate = event.Occurrences[0].EndTime.Format(time.RFC3339)
			}
		}

		// Add location (required per Interop Profile §3.1)
		item.Location = buildEventLocation(h.BaseURL, &event)

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

		// Check if event was previously rejected
		var rejErr events.ErrPreviouslyRejected
		if errors.As(err, &rejErr) {
			problem.Write(w, r, http.StatusBadRequest,
				"https://sel.events/problems/previously-rejected",
				"Previously Rejected",
				fmt.Errorf("this event was reviewed on %s and rejected: %s",
					rejErr.ReviewedAt.Format(time.RFC3339), rejErr.Reason),
				h.Env)
			return
		}

		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", err, h.Env)
		return
	}

	// Determine HTTP status code
	status := http.StatusCreated
	if result != nil && result.IsDuplicate {
		status = http.StatusConflict
	} else if result != nil && result.NeedsReview {
		// Event queued for admin review - return 202 Accepted
		status = http.StatusAccepted
	}

	location := createEventLocation(input)
	event := schema.NewEvent(input.Name)
	event.Context = loadDefaultContext()
	event.ID = eventURI(h.BaseURL, result)
	event.Location = location

	// Include lifecycle_state and warnings for events needing review
	if result != nil && result.NeedsReview {
		type createEventResponse struct {
			schema.Event
			LifecycleState string           `json:"lifecycle_state,omitempty"`
			Warnings       []map[string]any `json:"warnings,omitempty"`
		}
		resp := createEventResponse{
			Event:          *event,
			LifecycleState: "pending_review",
		}
		if len(result.Warnings) > 0 {
			warnings := make([]map[string]any, len(result.Warnings))
			for i, w := range result.Warnings {
				warnings[i] = map[string]any{
					"field":   w.Field,
					"code":    w.Code,
					"message": w.Message,
				}
			}
			resp.Warnings = warnings
		}
		writeJSON(w, status, resp, contentTypeFromRequest(r))
		return
	}

	writeJSON(w, status, event, contentTypeFromRequest(r))
}

func (h *EventsHandler) Get(w http.ResponseWriter, r *http.Request) {
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
		if errors.Is(err, events.ErrNotFound) {
			// Check if there's a tombstone for this event
			tombstone, tombErr := h.Service.GetTombstoneByEventULID(r.Context(), ulidValue)
			if tombErr == nil && tombstone != nil {
				WriteTombstoneResponse(w, r, tombstone.Payload, tombstone.DeletedAt, "Event")
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
			WriteTombstoneResponse(w, r, tombstone.Payload, tombstone.DeletedAt, "Event")
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

	// eventDetailResponse wraps schema.Event with provenance fields that don't
	// belong in the schema.org type.
	type eventDetailResponse struct {
		schema.Event
		Sources    any `json:"sources,omitempty"`
		Provenance any `json:"_provenance,omitempty"`
	}

	contextValue := loadDefaultContext()
	event := schema.NewEvent(item.Name)
	event.Context = contextValue
	event.ID = schema.BuildEventURI(h.BaseURL, item.ULID)
	event.Description = item.Description
	event.Image = item.ImageURL
	event.URL = item.PublicURL
	event.Keywords = item.Keywords
	event.InLanguage = item.InLanguage
	event.IsAccessibleForFree = item.IsAccessibleForFree
	event.EventStatus = item.EventStatus
	event.EventAttendanceMode = item.AttendanceMode

	// Add startDate/endDate/doorTime from first occurrence (required per Interop Profile §3.1)
	if len(item.Occurrences) > 0 {
		occ := item.Occurrences[0]
		event.StartDate = occ.StartTime.Format(time.RFC3339)
		if occ.EndTime != nil {
			event.EndDate = occ.EndTime.Format(time.RFC3339)
		}
		if occ.DoorTime != nil {
			event.DoorTime = occ.DoorTime.Format(time.RFC3339)
		}

		// Build offers from first occurrence
		if occ.TicketURL != "" || occ.PriceMin != nil {
			offer := schema.NewOffer()
			offer.URL = occ.TicketURL
			offer.Price = occ.PriceMin
			offer.PriceCurrency = occ.PriceCurrency
			offer.Availability = occ.Availability
			event.Offers = []schema.Offer{*offer}
		}
	}

	// Add location (required per Interop Profile §3.1)
	event.Location = buildEventLocation(h.BaseURL, item)

	// Add organizer as URI reference if present
	if item.OrganizerID != nil && *item.OrganizerID != "" {
		event.Organizer = schema.BuildOrganizationURI(h.BaseURL, *item.OrganizerID)
	}

	// Add license information per FR-024
	if item.LicenseURL != "" {
		event.License = item.LicenseURL
	} else {
		// Default to CC0 if not specified
		event.License = "https://creativecommons.org/publicdomain/zero/1.0/"
	}

	// Add federation URI as sameAs if present (federated events)
	if item.FederationURI != nil && *item.FederationURI != "" {
		event.SameAs = []string{*item.FederationURI}
	}

	resp := eventDetailResponse{Event: *event}

	// Check if provenance is requested (FR-029, US5)
	includeProvenance := strings.EqualFold(r.URL.Query().Get("include_provenance"), "true")
	provenanceFields := parseProvenanceFields(r.URL.Query().Get("provenance_fields"))

	if includeProvenance && h.ProvenanceService != nil {
		// Get source attribution
		sources, err := h.ProvenanceService.GetEventSourceAttribution(r.Context(), item.ID)
		if err == nil && len(sources) > 0 {
			resp.Sources = buildSourcesPayload(sources)
		}

		// Get field-level provenance if requested
		var fieldProvenance []provenance.FieldProvenanceInfo
		if len(provenanceFields) > 0 {
			fieldProvenance, err = h.ProvenanceService.GetFieldProvenance(r.Context(), item.ID, provenanceFields)
		} else {
			fieldProvenance, err = h.ProvenanceService.GetFieldProvenance(r.Context(), item.ID, nil)
		}

		if err == nil && len(fieldProvenance) > 0 {
			resp.Provenance = buildFieldProvenancePayload(fieldProvenance)
		}
	}

	writeJSON(w, http.StatusOK, resp, contentTypeFromRequest(r))
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
	// Note: We ignore encoding errors here because HTTP headers have already been sent
	// (WriteHeader was called above), so we cannot return an error response.
	// Encoding errors are extremely rare in practice for well-formed data structures.
	// If this becomes a concern, callers should validate payloads before calling writeJSON.
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
	return schema.BuildEventURI(baseURL, result.Event.ULID)
}

// buildEventLocation determines the location value for an event.
// Returns a Place URI string, a VirtualLocation struct, or nil.
func buildEventLocation(baseURL string, event *events.Event) any {
	// Prefer occurrence-level venue
	if len(event.Occurrences) > 0 && event.Occurrences[0].VenueID != nil {
		if uri := schema.BuildPlaceURI(baseURL, *event.Occurrences[0].VenueID); uri != "" {
			return uri
		}
	}
	// Fall back to primary venue
	if event.PrimaryVenueID != nil {
		if uri := schema.BuildPlaceURI(baseURL, *event.PrimaryVenueID); uri != "" {
			return uri
		}
	}
	// Occurrence-level virtual URL
	if len(event.Occurrences) > 0 && event.Occurrences[0].VirtualURL != nil && *event.Occurrences[0].VirtualURL != "" {
		return schema.NewVirtualLocation(*event.Occurrences[0].VirtualURL)
	}
	// Event-level virtual URL
	if event.VirtualURL != "" {
		return schema.NewVirtualLocation(event.VirtualURL)
	}
	return nil
}

// createEventLocation builds a location value for event creation responses.
func createEventLocation(input events.EventInput) any {
	if input.Location != nil {
		p := schema.NewPlace(input.Location.Name)
		return p
	}
	if input.VirtualLocation != nil {
		vl := schema.NewVirtualLocation(input.VirtualLocation.URL)
		vl.Name = input.VirtualLocation.Name
		return vl
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

// CreateBatch handles batch event submission via POST /api/v1/events:batch
func (h *EventsHandler) CreateBatch(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.RiverClient == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	// Parse batch request
	var input struct {
		Events []events.EventInput `json:"events"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", err, h.Env)
		return
	}

	// Validate batch size
	if len(input.Events) == 0 {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Events array cannot be empty", nil, h.Env)
		return
	}
	if len(input.Events) > 100 {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Batch size exceeds maximum of 100 events", nil, h.Env)
		return
	}

	// Generate batch ID
	batchID, err := ids.NewULID()
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to generate batch ID", err, h.Env)
		return
	}

	// Enqueue batch ingestion job
	jobArgs := jobs.BatchIngestionArgs{
		BatchID: batchID,
		Events:  input.Events,
	}

	insertOpts := jobs.InsertOptsForKind(jobs.JobKindBatchIngestion)
	jobResult, err := h.RiverClient.Insert(r.Context(), jobArgs, &insertOpts)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to enqueue batch job", err, h.Env)
		return
	}

	// Return batch status URI and job information
	statusURI, err := ids.BuildCanonicalURI(h.BaseURL, "batch-status", batchID)
	if err != nil {
		statusURI = h.BaseURL + "/api/v1/batch-status/" + batchID
	}

	response := map[string]any{
		"@context":   loadDefaultContext(),
		"@type":      "BatchSubmission",
		"batch_id":   batchID,
		"job_id":     jobResult.Job.ID,
		"status":     "processing",
		"status_url": statusURI,
		"submitted":  len(input.Events),
	}

	writeJSON(w, http.StatusAccepted, response, contentTypeFromRequest(r))
}

// GetBatchStatus retrieves the status of a batch ingestion job
func (h *EventsHandler) GetBatchStatus(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.RiverClient == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	batchID := pathParam(r, "id")
	if batchID == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Batch ID is required", nil, h.Env)
		return
	}

	// Query batch results from database using SQLc
	batchResult, err := h.Queries.GetBatchIngestionResult(r.Context(), batchID)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			// Batch not yet completed or doesn't exist
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Batch not found or still processing", nil, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to retrieve batch status", err, h.Env)
		return
	}

	var results []map[string]any
	if err := json.Unmarshal(batchResult.Results, &results); err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to parse batch results", err, h.Env)
		return
	}

	// Compute summary statistics
	created := 0
	failed := 0
	duplicates := 0
	for _, result := range results {
		if status, ok := result["status"].(string); ok {
			switch status {
			case "created":
				created++
			case "failed":
				failed++
			case "duplicate":
				duplicates++
			}
		}
	}

	response := map[string]any{
		"@context":     loadDefaultContext(),
		"@type":        "BatchSubmissionResult",
		"batch_id":     batchID,
		"status":       "completed",
		"completed_at": batchResult.CompletedAt.Time.Format(time.RFC3339),
		"total":        len(results),
		"created":      created,
		"failed":       failed,
		"duplicates":   duplicates,
		"results":      results,
	}

	writeJSON(w, http.StatusOK, response, contentTypeFromRequest(r))
}
