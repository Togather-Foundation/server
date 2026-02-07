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

	items := make([]map[string]any, 0, len(result.Events))
	for _, event := range result.Events {
		item := BuildBaseListItem("Event", event.Name, event.ULID, "events", h.BaseURL)

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

		// Check if event was previously rejected
		var rejErr events.ErrPreviouslyRejected
		if errors.As(err, &rejErr) {
			problem.Write(w, r, http.StatusBadRequest,
				"https://sel.events/problems/previously-rejected",
				"Previously Rejected",
				fmt.Errorf("This event was reviewed on %s and rejected: %s",
					rejErr.ReviewedAt.Format(time.RFC3339), rejErr.Reason),
				h.Env)
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
