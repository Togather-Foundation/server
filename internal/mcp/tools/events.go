package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/domain/ids"
	"github.com/mark3labs/mcp-go/mcp"
)

// EventTools provides MCP tools for querying and managing events.
type EventTools struct {
	eventsService *events.Service
	ingestService *events.IngestService
	baseURL       string
}

// NewEventTools creates a new EventTools instance.
func NewEventTools(eventsService *events.Service, ingestService *events.IngestService, baseURL string) *EventTools {
	return &EventTools{
		eventsService: eventsService,
		ingestService: ingestService,
		baseURL:       strings.TrimSpace(baseURL),
	}
}

// EventsTool returns the MCP tool definition for listing or getting events.
// If id parameter is provided, returns a single event. Otherwise, returns a list of events.
func (t *EventTools) EventsTool() mcp.Tool {
	return mcp.Tool{
		Name:        "events",
		Description: "List events with optional filters, or get a specific event by ULID. If 'id' is provided, returns a single JSON-LD formatted event. Otherwise, returns a JSON array of events matching the filter criteria.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"id": map[string]interface{}{
					"type":        "string",
					"description": "Optional ULID of a specific event to retrieve. If provided, other parameters are ignored.",
				},
				"query": map[string]interface{}{
					"type":        "string",
					"description": "Search query to filter events by name or description",
				},
				"start_date": map[string]interface{}{
					"type":        "string",
					"description": "Filter events starting on or after this date (ISO8601 format: YYYY-MM-DD)",
				},
				"end_date": map[string]interface{}{
					"type":        "string",
					"description": "Filter events starting on or before this date (ISO8601 format: YYYY-MM-DD)",
				},
				"location": map[string]interface{}{
					"type":        "string",
					"description": "Filter by location (city or region name)",
				},
				"city": map[string]interface{}{
					"type":        "string",
					"description": "Filter by city name",
				},
				"region": map[string]interface{}{
					"type":        "string",
					"description": "Filter by region name",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of events to return (default: 50, max: 200)",
					"default":     50,
				},
				"cursor": map[string]interface{}{
					"type":        "string",
					"description": "Pagination cursor from a previous response",
				},
			},
		},
	}
}

// EventsHandler handles the events tool call.
// If id is provided, delegates to get event logic. Otherwise, delegates to list events logic.
func (t *EventTools) EventsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if t == nil || t.eventsService == nil {
		return mcp.NewToolResultError("events service not configured"), nil
	}

	// Parse arguments - check for id first
	args := struct {
		ID        string `json:"id"`
		Query     string `json:"query"`
		StartDate string `json:"start_date"`
		EndDate   string `json:"end_date"`
		Location  string `json:"location"`
		City      string `json:"city"`
		Region    string `json:"region"`
		Limit     int    `json:"limit"`
		Cursor    string `json:"cursor"`
	}{
		Limit: 50,
	}

	if request.Params.Arguments != nil {
		data, err := json.Marshal(request.Params.Arguments)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid arguments", err), nil
		}
		if err := json.Unmarshal(data, &args); err != nil {
			return mcp.NewToolResultErrorFromErr("invalid arguments", err), nil
		}
	}

	// If id is provided, get single event
	if strings.TrimSpace(args.ID) != "" {
		return t.getEventByID(ctx, strings.TrimSpace(args.ID))
	}

	// Otherwise, list events with filters
	return t.listEvents(ctx, args.Query, args.StartDate, args.EndDate, args.Location, args.City, args.Region, args.Limit, args.Cursor)
}

// getEventByID retrieves a single event by ULID.
// Returns tombstone data if the event is deleted.
func (t *EventTools) getEventByID(ctx context.Context, id string) (*mcp.CallToolResult, error) {
	// Validate ULID format
	if err := ids.ValidateULID(id); err != nil {
		return mcp.NewToolResultErrorFromErr("invalid ULID format", err), nil
	}

	// Get event by ULID
	event, err := t.eventsService.GetByULID(ctx, id)
	if err != nil {
		if errors.Is(err, events.ErrNotFound) {
			tombstone, tombErr := t.eventsService.GetTombstoneByEventULID(ctx, id)
			if tombErr != nil && !errors.Is(tombErr, events.ErrNotFound) {
				// Log tombstone fetch error for diagnostics (don't fail the request)
				fmt.Fprintf(os.Stderr, "MCP: failed to fetch tombstone for event %s: %v\n", id, tombErr)
			}
			if tombErr == nil && tombstone != nil {
				payload, payloadErr := decodeTombstonePayload(tombstone.Payload)
				if payloadErr != nil {
					return mcp.NewToolResultErrorFromErr("failed to decode tombstone payload", payloadErr), nil
				}
				resultJSON, resultErr := mcp.NewToolResultJSON(payload)
				if resultErr != nil {
					return mcp.NewToolResultErrorFromErr("failed to build response", resultErr), nil
				}
				return resultJSON, nil
			}
			return mcp.NewToolResultErrorf("event not found: %s", id), nil
		}
		return mcp.NewToolResultErrorFromErr("failed to get event", err), nil
	}

	// Add nil check before accessing event fields
	if event == nil {
		return mcp.NewToolResultErrorf("event not found: %s", id), nil
	}

	if strings.EqualFold(event.LifecycleState, "deleted") {
		tombstone, tombErr := t.eventsService.GetTombstoneByEventULID(ctx, id)
		if tombErr != nil && !errors.Is(tombErr, events.ErrNotFound) {
			// Log tombstone fetch error for diagnostics (don't fail the request)
			fmt.Fprintf(os.Stderr, "MCP: failed to fetch tombstone for deleted event %s: %v\n", id, tombErr)
		}
		if tombErr == nil && tombstone != nil {
			payload, payloadErr := decodeTombstonePayload(tombstone.Payload)
			if payloadErr != nil {
				return mcp.NewToolResultErrorFromErr("failed to decode tombstone payload", payloadErr), nil
			}
			resultJSON, resultErr := mcp.NewToolResultJSON(payload)
			if resultErr != nil {
				return mcp.NewToolResultErrorFromErr("failed to build response", resultErr), nil
			}
			return resultJSON, nil
		}

		payload := map[string]any{
			"@context":      defaultContext(),
			"@type":         "Event",
			"sel:tombstone": true,
			"sel:deletedAt": time.Now().Format(time.RFC3339),
		}
		if uri := buildEventURI(t.baseURL, event.ULID); uri != "" {
			payload["@id"] = uri
		}
		resultJSON, resultErr := mcp.NewToolResultJSON(payload)
		if resultErr != nil {
			return mcp.NewToolResultErrorFromErr("failed to build response", resultErr), nil
		}
		return resultJSON, nil
	}

	payload := buildEventPayload(event, t.baseURL)
	resultJSON, err := mcp.NewToolResultJSON(payload)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("failed to build response", err), nil
	}

	return resultJSON, nil
}

// listEvents retrieves a list of events with optional filters and pagination.
// Supports filtering by query text, date range, and location parameters.
func (t *EventTools) listEvents(ctx context.Context, query, startDate, endDate, location, city, region string, limit int, cursor string) (*mcp.CallToolResult, error) {
	const maxListLimit = 200

	// Enforce limit caps
	if limit <= 0 {
		limit = 50
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}

	values := url.Values{}
	if strings.TrimSpace(query) != "" {
		values.Set("q", strings.TrimSpace(query))
	}
	if strings.TrimSpace(startDate) != "" {
		values.Set("startDate", strings.TrimSpace(startDate))
	}
	if strings.TrimSpace(endDate) != "" {
		values.Set("endDate", strings.TrimSpace(endDate))
	}
	cityValue := strings.TrimSpace(city)
	regionValue := strings.TrimSpace(region)
	if cityValue == "" && regionValue == "" {
		cityValue = strings.TrimSpace(location)
	}
	if cityValue != "" {
		values.Set("city", cityValue)
	}
	if regionValue != "" {
		values.Set("region", regionValue)
	}
	values.Set("limit", strconv.Itoa(limit))
	if strings.TrimSpace(cursor) != "" {
		values.Set("after", strings.TrimSpace(cursor))
	}

	filters, pagination, err := events.ParseFilters(values)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("invalid filters", err), nil
	}

	result, err := t.eventsService.List(ctx, filters, pagination)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("failed to list events", err), nil
	}

	items := make([]map[string]any, 0, len(result.Events))
	for _, event := range result.Events {
		items = append(items, buildListItem(event, t.baseURL))
	}

	response := map[string]any{
		"items":       items,
		"next_cursor": result.NextCursor,
	}

	resultJSON, err := mcp.NewToolResultJSON(response)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("failed to build response", err), nil
	}

	return resultJSON, nil
}

// AddEventTool returns the MCP tool definition for creating an event.
func (t *EventTools) AddEventTool() mcp.Tool {
	return mcp.Tool{
		Name:        "add_event",
		Description: "Create a new event. Accepts a JSON-LD event object and returns the created event with its assigned ID.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"event": map[string]interface{}{
					"type":        "object",
					"description": "JSON-LD event object with event details",
				},
			},
			Required: []string{"event"},
		},
	}
}

// AddEventHandler handles the add_event tool call.
func (t *EventTools) AddEventHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if t == nil || t.ingestService == nil {
		return mcp.NewToolResultError("ingest service not configured"), nil
	}

	args := struct {
		Event json.RawMessage `json:"event"`
	}{}

	if request.Params.Arguments != nil {
		data, err := json.Marshal(request.Params.Arguments)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid arguments", err), nil
		}
		if err := json.Unmarshal(data, &args); err != nil {
			return mcp.NewToolResultErrorFromErr("invalid arguments", err), nil
		}
	}

	if len(args.Event) == 0 {
		return mcp.NewToolResultError("event parameter is required"), nil
	}

	var raw map[string]any
	if err := json.Unmarshal(args.Event, &raw); err != nil {
		return mcp.NewToolResultErrorFromErr("invalid event payload", err), nil
	}

	payload, err := json.Marshal(raw)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("invalid event payload", err), nil
	}

	var input events.EventInput
	if err := json.Unmarshal(payload, &input); err != nil {
		return mcp.NewToolResultErrorFromErr("failed to decode event payload", err), nil
	}

	input = addMCPProvenance(input, t.baseURL)

	result, err := t.ingestService.Ingest(ctx, input)
	if err != nil {
		if errors.Is(err, events.ErrConflict) {
			return mcp.NewToolResultErrorFromErr("event conflict", err), nil
		}
		return mcp.NewToolResultErrorFromErr("failed to ingest event", err), nil
	}

	response := map[string]any{
		"id":           result.Event.ULID,
		"event":        result.Event,
		"is_duplicate": result.IsDuplicate,
		"needs_review": result.NeedsReview,
	}
	if uri := buildEventURI(t.baseURL, result.Event.ULID); uri != "" {
		response["@id"] = uri
	}

	resultJSON, err := mcp.NewToolResultJSON(response)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("failed to build response", err), nil
	}

	return resultJSON, nil
}

func buildListItem(event events.Event, baseURL string) map[string]any {
	item := map[string]any{
		"@context": defaultContext(),
		"@type":    "Event",
		"name":     event.Name,
	}
	if uri := buildEventURI(baseURL, event.ULID); uri != "" {
		item["@id"] = uri
	}

	if len(event.Occurrences) > 0 {
		item["startDate"] = event.Occurrences[0].StartTime.Format(time.RFC3339)
	}

	location := buildEventLocation(event, baseURL)
	if location != nil {
		item["location"] = location
	}

	return item
}

func buildEventPayload(event *events.Event, baseURL string) map[string]any {
	if event == nil {
		return map[string]any{}
	}

	payload := map[string]any{
		"@context": defaultContext(),
		"@type":    "Event",
		"@id":      buildEventURI(baseURL, event.ULID),
		"name":     event.Name,
	}

	if len(event.Occurrences) > 0 {
		payload["startDate"] = event.Occurrences[0].StartTime.Format(time.RFC3339)
	}

	location := buildEventLocation(*event, baseURL)
	if location != nil {
		payload["location"] = location
	}

	if event.LicenseURL != "" {
		payload["license"] = event.LicenseURL
	} else {
		payload["license"] = "https://creativecommons.org/publicdomain/zero/1.0/"
	}

	if event.FederationURI != nil && *event.FederationURI != "" {
		payload["sameAs"] = *event.FederationURI
	}

	return payload
}

func buildEventLocation(event events.Event, baseURL string) any {
	if len(event.Occurrences) > 0 && event.Occurrences[0].VenueID != nil {
		if placeURI, err := ids.BuildCanonicalURI(baseURL, "places", *event.Occurrences[0].VenueID); err == nil {
			return placeURI
		}
	}
	if event.PrimaryVenueID != nil {
		if placeURI, err := ids.BuildCanonicalURI(baseURL, "places", *event.PrimaryVenueID); err == nil {
			return placeURI
		}
	}
	if len(event.Occurrences) > 0 && event.Occurrences[0].VirtualURL != nil && *event.Occurrences[0].VirtualURL != "" {
		return map[string]any{
			"@type": "VirtualLocation",
			"url":   *event.Occurrences[0].VirtualURL,
		}
	}
	if event.VirtualURL != "" {
		return map[string]any{
			"@type": "VirtualLocation",
			"url":   event.VirtualURL,
		}
	}
	return nil
}

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

func addMCPProvenance(input events.EventInput, baseURL string) events.EventInput {
	if input.Source == nil {
		input.Source = &events.SourceInput{}
	}
	if strings.TrimSpace(input.Source.Name) == "" {
		input.Source.Name = "mcp-agent"
	}
	if strings.TrimSpace(input.Source.URL) == "" {
		baseURL = strings.TrimSpace(baseURL)
		if baseURL == "" {
			baseURL = "https://mcp-agent"
		}
		input.Source.URL = baseURL
	}
	if strings.TrimSpace(input.Source.EventID) == "" {
		if ulidValue, err := ids.NewULID(); err == nil {
			input.Source.EventID = ulidValue
		}
	}
	return input
}
