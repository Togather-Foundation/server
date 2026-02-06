package tools

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/domain/ids"
	"github.com/Togather-Foundation/server/internal/jsonld"
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

// ListEventsTool returns the MCP tool definition for listing events.
func (t *EventTools) ListEventsTool() mcp.Tool {
	return mcp.Tool{
		Name:        "list_events",
		Description: "List events with optional filters for query, date range, location, and pagination. Returns a JSON array of events matching the criteria.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
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

// ListEventsHandler handles the list_events tool call.
func (t *EventTools) ListEventsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if t == nil || t.eventsService == nil {
		return mcp.NewToolResultError("events service not configured"), nil
	}

	// Parse arguments
	args := struct {
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

	values := url.Values{}
	if strings.TrimSpace(args.Query) != "" {
		values.Set("q", strings.TrimSpace(args.Query))
	}
	if strings.TrimSpace(args.StartDate) != "" {
		values.Set("startDate", strings.TrimSpace(args.StartDate))
	}
	if strings.TrimSpace(args.EndDate) != "" {
		values.Set("endDate", strings.TrimSpace(args.EndDate))
	}
	city := strings.TrimSpace(args.City)
	region := strings.TrimSpace(args.Region)
	if city == "" && region == "" {
		city = strings.TrimSpace(args.Location)
	}
	if city != "" {
		values.Set("city", city)
	}
	if region != "" {
		values.Set("region", region)
	}
	if args.Limit > 0 {
		values.Set("limit", strconv.Itoa(args.Limit))
	}
	if strings.TrimSpace(args.Cursor) != "" {
		values.Set("after", strings.TrimSpace(args.Cursor))
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

// GetEventTool returns the MCP tool definition for getting a single event by ID.
func (t *EventTools) GetEventTool() mcp.Tool {
	return mcp.Tool{
		Name:        "get_event",
		Description: "Get detailed information about a specific event by its ULID. Returns a JSON-LD formatted event object.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"id": map[string]interface{}{
					"type":        "string",
					"description": "The ULID of the event to retrieve",
				},
			},
			Required: []string{"id"},
		},
	}
}

// GetEventHandler handles the get_event tool call.
func (t *EventTools) GetEventHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if t == nil || t.eventsService == nil {
		return mcp.NewToolResultError("events service not configured"), nil
	}

	// Parse arguments
	args := struct {
		ID string `json:"id"`
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

	if args.ID == "" {
		return mcp.NewToolResultError("id parameter is required"), nil
	}

	// Validate ULID format
	if err := ids.ValidateULID(args.ID); err != nil {
		return mcp.NewToolResultErrorFromErr("invalid ULID format", err), nil
	}

	// Get event by ULID
	event, err := t.eventsService.GetByULID(ctx, args.ID)
	if err != nil {
		if errors.Is(err, events.ErrNotFound) {
			if tombstone, tombErr := t.eventsService.GetTombstoneByEventULID(ctx, args.ID); tombErr == nil && tombstone != nil {
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
			return mcp.NewToolResultErrorf("event not found: %s", args.ID), nil
		}
		return mcp.NewToolResultErrorFromErr("failed to get event", err), nil
	}

	if strings.EqualFold(event.LifecycleState, "deleted") {
		if tombstone, tombErr := t.eventsService.GetTombstoneByEventULID(ctx, args.ID); tombErr == nil && tombstone != nil {
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

// CreateEventTool returns the MCP tool definition for creating an event.
func (t *EventTools) CreateEventTool() mcp.Tool {
	return mcp.Tool{
		Name:        "create_event",
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

// CreateEventHandler handles the create_event tool call.
func (t *EventTools) CreateEventHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

func defaultContext() any {
	ctxDoc, err := jsonld.LoadDefaultContext()
	if err != nil {
		return nil
	}
	if ctx, ok := ctxDoc["@context"]; ok {
		return ctx
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

func decodeTombstonePayload(payload []byte) (map[string]any, error) {
	if len(payload) == 0 {
		return map[string]any{}, nil
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, err
	}
	return decoded, nil
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
