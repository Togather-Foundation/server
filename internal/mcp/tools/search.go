package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/domain/organizations"
	"github.com/Togather-Foundation/server/internal/domain/places"
	"github.com/mark3labs/mcp-go/mcp"
)

const (
	defaultSearchLimit = 20
	maxSearchLimit     = 200
)

// SearchTools provides MCP tools for cross-entity search.
type SearchTools struct {
	eventsService *events.Service
	placesService *places.Service
	orgService    *organizations.Service
	baseURL       string
}

// NewSearchTools creates a new SearchTools instance.
func NewSearchTools(eventsService *events.Service, placesService *places.Service, orgService *organizations.Service, baseURL string) *SearchTools {
	return &SearchTools{
		eventsService: eventsService,
		placesService: placesService,
		orgService:    orgService,
		baseURL:       strings.TrimSpace(baseURL),
	}
}

// SearchTool returns the MCP tool definition for cross-entity search.
func (t *SearchTools) SearchTool() mcp.Tool {
	return mcp.Tool{
		Name:        "search",
		Description: "Search events, places, and organizations with a single query. Returns a merged JSON array with type tags.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "Search query to match names and descriptions",
				},
				"types": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Optional list of entity types to search (event, place, organization)",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum total number of results to return (default: 20, max: 200)",
					"default":     defaultSearchLimit,
				},
			},
			Required: []string{"query"},
		},
	}
}

// SearchHandler handles the search tool call.
func (t *SearchTools) SearchHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if t == nil {
		return mcp.NewToolResultError("search tools not configured"), nil
	}

	args := struct {
		Query string   `json:"query"`
		Types []string `json:"types"`
		Limit int      `json:"limit"`
	}{
		Limit: defaultSearchLimit,
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

	query := strings.TrimSpace(args.Query)
	if query == "" {
		return mcp.NewToolResultError("query parameter is required"), nil
	}

	limit := normalizeSearchLimit(args.Limit)
	selectedTypes, err := normalizeSearchTypes(args.Types)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("invalid types", err), nil
	}

	typeOrder := []string{"event", "place", "organization"}
	activeTypes := make([]string, 0, len(typeOrder))
	for _, item := range typeOrder {
		if selectedTypes[item] {
			activeTypes = append(activeTypes, item)
		}
	}
	if len(activeTypes) == 0 {
		return mcp.NewToolResultError("no valid types provided"), nil
	}

	perTypeLimits := distributeLimits(limit, len(activeTypes))

	items := make([]map[string]any, 0, limit)
	for index, entityType := range activeTypes {
		if perTypeLimits[index] == 0 {
			continue
		}

		switch entityType {
		case "event":
			if t.eventsService == nil {
				return mcp.NewToolResultError("events service not configured"), nil
			}
			results, err := t.searchEvents(ctx, query, perTypeLimits[index])
			if err != nil {
				return mcp.NewToolResultErrorFromErr("failed to search events", err), nil
			}
			items = append(items, results...)
		case "place":
			if t.placesService == nil {
				return mcp.NewToolResultError("places service not configured"), nil
			}
			results, err := t.searchPlaces(ctx, query, perTypeLimits[index])
			if err != nil {
				return mcp.NewToolResultErrorFromErr("failed to search places", err), nil
			}
			items = append(items, results...)
		case "organization":
			if t.orgService == nil {
				return mcp.NewToolResultError("organizations service not configured"), nil
			}
			results, err := t.searchOrganizations(ctx, query, perTypeLimits[index])
			if err != nil {
				return mcp.NewToolResultErrorFromErr("failed to search organizations", err), nil
			}
			items = append(items, results...)
		}
	}

	if len(items) > limit {
		items = items[:limit]
	}

	response := map[string]any{
		"items": items,
		"count": len(items),
	}

	return toolResultJSON(response)
}

func (t *SearchTools) searchEvents(ctx context.Context, query string, limit int) ([]map[string]any, error) {
	values := url.Values{}
	values.Set("q", query)
	values.Set("limit", strconv.Itoa(limit))
	filters, pagination, err := events.ParseFilters(values)
	if err != nil {
		return nil, err
	}
	result, err := t.eventsService.List(ctx, filters, pagination)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(result.Events))
	for _, event := range result.Events {
		item := buildListItem(event, t.baseURL, t.placesService, t.orgService)
		item["type"] = "event"
		items = append(items, item)
	}
	return items, nil
}

func (t *SearchTools) searchPlaces(ctx context.Context, query string, limit int) ([]map[string]any, error) {
	values := url.Values{}
	values.Set("q", query)
	values.Set("limit", strconv.Itoa(limit))
	filters, pagination, err := places.ParseFilters(values)
	if err != nil {
		return nil, err
	}
	result, err := t.placesService.List(ctx, filters, pagination)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(result.Places))
	for _, place := range result.Places {
		item := buildPlaceListItem(place, t.baseURL)
		item["type"] = "place"
		items = append(items, item)
	}
	return items, nil
}

func (t *SearchTools) searchOrganizations(ctx context.Context, query string, limit int) ([]map[string]any, error) {
	values := url.Values{}
	values.Set("q", query)
	values.Set("limit", strconv.Itoa(limit))
	filters, pagination, err := organizations.ParseFilters(values)
	if err != nil {
		return nil, err
	}
	result, err := t.orgService.List(ctx, filters, pagination)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(result.Organizations))
	for _, org := range result.Organizations {
		item := buildOrganizationListItem(org, t.baseURL)
		item["type"] = "organization"
		items = append(items, item)
	}
	return items, nil
}

func normalizeSearchTypes(types []string) (map[string]bool, error) {
	if len(types) == 0 {
		return map[string]bool{
			"event":        true,
			"place":        true,
			"organization": true,
		}, nil
	}

	selected := map[string]bool{}
	for _, raw := range types {
		value := strings.ToLower(strings.TrimSpace(raw))
		if value == "" {
			continue
		}
		switch value {
		case "event", "events":
			selected["event"] = true
		case "place", "places":
			selected["place"] = true
		case "organization", "organizations", "org", "orgs":
			selected["organization"] = true
		default:
			return nil, fmt.Errorf("unsupported type: %s", value)
		}
	}

	return selected, nil
}

func normalizeSearchLimit(limit int) int {
	if limit <= 0 {
		return defaultSearchLimit
	}
	if limit > maxSearchLimit {
		return maxSearchLimit
	}
	return limit
}

func distributeLimits(total int, buckets int) []int {
	if buckets <= 0 {
		return nil
	}
	limits := make([]int, buckets)
	base := total / buckets
	extra := total % buckets
	for i := 0; i < buckets; i++ {
		limits[i] = base
		if i < extra {
			limits[i]++
		}
	}
	return limits
}
