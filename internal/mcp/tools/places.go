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

	"github.com/Togather-Foundation/server/internal/domain/ids"
	"github.com/Togather-Foundation/server/internal/domain/places"
	"github.com/mark3labs/mcp-go/mcp"
)

// PlaceTools provides MCP tools for querying and managing places.
type PlaceTools struct {
	placesService *places.Service
	baseURL       string
}

// NewPlaceTools creates a new PlaceTools instance.
func NewPlaceTools(placesService *places.Service, baseURL string) *PlaceTools {
	return &PlaceTools{
		placesService: placesService,
		baseURL:       strings.TrimSpace(baseURL),
	}
}

// PlacesTool returns the MCP tool definition for listing or getting places.
// If id parameter is provided, returns a single place. Otherwise, returns a list of places.
func (t *PlaceTools) PlacesTool() mcp.Tool {
	return mcp.Tool{
		Name:        "places",
		Description: "List places with optional filters, or get a specific place by ULID. If 'id' is provided, returns a single JSON-LD formatted place. Otherwise, returns a JSON array of places matching the filter criteria. Supports proximity search via near_lat, near_lon, and radius parameters.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"id": map[string]interface{}{
					"type":        "string",
					"description": "Optional ULID of a specific place to retrieve. If provided, other parameters are ignored.",
				},
				"query": map[string]interface{}{
					"type":        "string",
					"description": "Search query to filter places by name or description",
				},
				"near_lat": map[string]interface{}{
					"type":        "number",
					"description": "Latitude for proximity search (requires near_lon, must be between -90 and 90)",
				},
				"near_lon": map[string]interface{}{
					"type":        "number",
					"description": "Longitude for proximity search (requires near_lat, must be between -180 and 180)",
				},
				"radius": map[string]interface{}{
					"type":        "number",
					"description": "Search radius in kilometers (default: 10, max: 100)",
					"default":     10,
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of places to return (default: 50, max: 200)",
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

// PlacesHandler handles the places tool call.
// If id is provided, delegates to get place logic. Otherwise, delegates to list places logic.
func (t *PlaceTools) PlacesHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if t == nil || t.placesService == nil {
		return mcp.NewToolResultError("places service not configured"), nil
	}

	args := struct {
		ID      string   `json:"id"`
		Query   string   `json:"query"`
		NearLat *float64 `json:"near_lat"`
		NearLon *float64 `json:"near_lon"`
		Radius  *float64 `json:"radius"`
		Limit   int      `json:"limit"`
		Cursor  string   `json:"cursor"`
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

	// If id is provided, get single place
	if strings.TrimSpace(args.ID) != "" {
		return t.getPlaceByID(ctx, strings.TrimSpace(args.ID))
	}

	// Otherwise, list places with filters
	return t.listPlaces(ctx, args.Query, args.NearLat, args.NearLon, args.Radius, args.Limit, args.Cursor)
}

// getPlaceByID retrieves a single place by ULID.
// Returns tombstone data if the place is deleted.
func (t *PlaceTools) getPlaceByID(ctx context.Context, id string) (*mcp.CallToolResult, error) {
	if err := ids.ValidateULID(id); err != nil {
		return mcp.NewToolResultErrorFromErr("invalid ULID format", err), nil
	}

	place, err := t.placesService.GetByULID(ctx, id)
	if err != nil {
		if errors.Is(err, places.ErrNotFound) {
			tombstone, tombErr := t.placesService.GetTombstoneByULID(ctx, id)
			if tombErr != nil && !errors.Is(tombErr, places.ErrNotFound) {
				// Log tombstone fetch error for diagnostics (don't fail the request)
				fmt.Fprintf(os.Stderr, "MCP: failed to fetch tombstone for place %s: %v\n", id, tombErr)
			}
			if tombErr == nil && tombstone != nil {
				payload, payloadErr := decodeTombstonePayload(tombstone.Payload)
				if payloadErr != nil {
					return mcp.NewToolResultErrorFromErr("failed to decode tombstone payload", payloadErr), nil
				}
				return toolResultJSON(payload)
			}
			return mcp.NewToolResultErrorf("place not found: %s", id), nil
		}
		return mcp.NewToolResultErrorFromErr("failed to get place", err), nil
	}

	// Defensive nil check (should not happen, but be safe)
	if place == nil {
		return mcp.NewToolResultErrorf("place not found: %s", id), nil
	}

	if strings.EqualFold(place.Lifecycle, "deleted") {
		tombstone, tombErr := t.placesService.GetTombstoneByULID(ctx, id)
		if tombErr != nil && !errors.Is(tombErr, places.ErrNotFound) {
			// Log tombstone fetch error for diagnostics (don't fail the request)
			fmt.Fprintf(os.Stderr, "MCP: failed to fetch tombstone for deleted place %s: %v\n", id, tombErr)
		}
		if tombErr == nil && tombstone != nil {
			payload, payloadErr := decodeTombstonePayload(tombstone.Payload)
			if payloadErr != nil {
				return mcp.NewToolResultErrorFromErr("failed to decode tombstone payload", payloadErr), nil
			}
			return toolResultJSON(payload)
		}

		payload := map[string]any{
			"@context":      defaultContext(),
			"@type":         "Place",
			"sel:tombstone": true,
			"sel:deletedAt": time.Now().Format(time.RFC3339),
		}
		if uri := buildPlaceURI(t.baseURL, place.ULID); uri != "" {
			payload["@id"] = uri
		}
		return toolResultJSON(payload)
	}

	payload := buildPlacePayload(place, t.baseURL)
	return toolResultJSON(payload)
}

// listPlaces retrieves a list of places with optional filters and pagination.
// Supports filtering by query text and proximity search (near_lat, near_lon, radius).
func (t *PlaceTools) listPlaces(ctx context.Context, query string, nearLat *float64, nearLon *float64, radius *float64, limit int, cursor string) (*mcp.CallToolResult, error) {

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
	if nearLat != nil {
		values.Set("near_lat", strconv.FormatFloat(*nearLat, 'f', -1, 64))
	}
	if nearLon != nil {
		values.Set("near_lon", strconv.FormatFloat(*nearLon, 'f', -1, 64))
	}
	if radius != nil {
		values.Set("radius", strconv.FormatFloat(*radius, 'f', -1, 64))
	}
	values.Set("limit", strconv.Itoa(limit))
	if strings.TrimSpace(cursor) != "" {
		values.Set("after", strings.TrimSpace(cursor))
	}

	filters, pagination, err := places.ParseFilters(values)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("invalid filters", err), nil
	}

	result, err := t.placesService.List(ctx, filters, pagination)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("failed to list places", err), nil
	}

	items := make([]map[string]any, 0, len(result.Places))
	for _, place := range result.Places {
		item := buildPlaceListItem(place, t.baseURL)
		if place.DistanceKm != nil {
			item["sel:distanceKm"] = *place.DistanceKm
		}
		items = append(items, item)
	}

	response := map[string]any{
		"items":       items,
		"next_cursor": result.NextCursor,
	}

	return toolResultJSON(response)
}

func buildPlaceListItem(place places.Place, baseURL string) map[string]any {
	item := map[string]any{
		"@context": defaultContext(),
		"@type":    "Place",
		"name":     place.Name,
	}
	if uri := buildPlaceURI(baseURL, place.ULID); uri != "" {
		item["@id"] = uri
	}

	address := buildPlaceAddress(place)
	if address != nil {
		item["address"] = address
	}
	geo := buildPlaceGeo(place)
	if geo != nil {
		item["geo"] = geo
	}

	return item
}

func buildPlacePayload(place *places.Place, baseURL string) map[string]any {
	if place == nil {
		return map[string]any{}
	}

	payload := map[string]any{
		"@context": defaultContext(),
		"@type":    "Place",
		"name":     place.Name,
	}
	if uri := buildPlaceURI(baseURL, place.ULID); uri != "" {
		payload["@id"] = uri
	}

	address := buildPlaceAddress(*place)
	if address != nil {
		payload["address"] = address
	}
	geo := buildPlaceGeo(*place)
	if geo != nil {
		payload["geo"] = geo
	}

	return payload
}

func buildPlaceAddress(place places.Place) map[string]any {
	if place.StreetAddress == "" && place.City == "" && place.Region == "" && place.PostalCode == "" && place.Country == "" {
		return nil
	}

	address := map[string]any{
		"@type": "PostalAddress",
	}
	if place.StreetAddress != "" {
		address["streetAddress"] = place.StreetAddress
	}
	if place.City != "" {
		address["addressLocality"] = place.City
	}
	if place.Region != "" {
		address["addressRegion"] = place.Region
	}
	if place.PostalCode != "" {
		address["postalCode"] = place.PostalCode
	}
	if place.Country != "" {
		address["addressCountry"] = place.Country
	}

	return address
}

func buildPlaceGeo(place places.Place) map[string]any {
	if place.Latitude == nil || place.Longitude == nil {
		return nil
	}

	return map[string]any{
		"@type":     "GeoCoordinates",
		"latitude":  *place.Latitude,
		"longitude": *place.Longitude,
	}
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
