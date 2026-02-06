package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
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

// ListPlacesTool returns the MCP tool definition for listing places.
func (t *PlaceTools) ListPlacesTool() mcp.Tool {
	return mcp.Tool{
		Name:        "list_places",
		Description: "List places with optional filters for query, proximity, and pagination. Returns a JSON array of places matching the criteria.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "Search query to filter places by name or description",
				},
				"near_lat": map[string]interface{}{
					"type":        "number",
					"description": "Latitude for proximity search",
				},
				"near_lon": map[string]interface{}{
					"type":        "number",
					"description": "Longitude for proximity search",
				},
				"radius": map[string]interface{}{
					"type":        "number",
					"description": "Search radius in meters (requires near_lat and near_lon)",
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

// ListPlacesHandler handles the list_places tool call.
func (t *PlaceTools) ListPlacesHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if t == nil || t.placesService == nil {
		return mcp.NewToolResultError("places service not configured"), nil
	}

	args := struct {
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

	values := url.Values{}
	if strings.TrimSpace(args.Query) != "" {
		values.Set("q", strings.TrimSpace(args.Query))
	}
	if args.NearLat != nil {
		values.Set("near_lat", strconv.FormatFloat(*args.NearLat, 'f', -1, 64))
	}
	if args.NearLon != nil {
		values.Set("near_lon", strconv.FormatFloat(*args.NearLon, 'f', -1, 64))
	}
	if args.Radius != nil {
		values.Set("radius", strconv.FormatFloat(*args.Radius, 'f', -1, 64))
	}
	if args.Limit > 0 {
		values.Set("limit", strconv.Itoa(args.Limit))
	}
	if strings.TrimSpace(args.Cursor) != "" {
		values.Set("after", strings.TrimSpace(args.Cursor))
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
		items = append(items, buildPlaceListItem(place, t.baseURL))
	}

	response := map[string]any{
		"items":       items,
		"next_cursor": result.NextCursor,
	}

	return toolResultJSON(response)
}

// GetPlaceTool returns the MCP tool definition for getting a single place by ID.
func (t *PlaceTools) GetPlaceTool() mcp.Tool {
	return mcp.Tool{
		Name:        "get_place",
		Description: "Get detailed information about a specific place by its ULID. Returns a JSON-LD formatted place object.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"id": map[string]interface{}{
					"type":        "string",
					"description": "The ULID of the place to retrieve",
				},
			},
			Required: []string{"id"},
		},
	}
}

// GetPlaceHandler handles the get_place tool call.
func (t *PlaceTools) GetPlaceHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if t == nil || t.placesService == nil {
		return mcp.NewToolResultError("places service not configured"), nil
	}

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

	if strings.TrimSpace(args.ID) == "" {
		return mcp.NewToolResultError("id parameter is required"), nil
	}

	if err := ids.ValidateULID(args.ID); err != nil {
		return mcp.NewToolResultErrorFromErr("invalid ULID format", err), nil
	}

	place, err := t.placesService.GetByULID(ctx, args.ID)
	if err != nil {
		if errors.Is(err, places.ErrNotFound) {
			if tombstone, tombErr := t.placesService.GetTombstoneByULID(ctx, args.ID); tombErr == nil && tombstone != nil {
				payload, payloadErr := decodeTombstonePayload(tombstone.Payload)
				if payloadErr != nil {
					return mcp.NewToolResultErrorFromErr("failed to decode tombstone payload", payloadErr), nil
				}
				return toolResultJSON(payload)
			}
			return mcp.NewToolResultErrorf("place not found: %s", args.ID), nil
		}
		return mcp.NewToolResultErrorFromErr("failed to get place", err), nil
	}

	if strings.EqualFold(place.Lifecycle, "deleted") {
		if tombstone, tombErr := t.placesService.GetTombstoneByULID(ctx, args.ID); tombErr == nil && tombstone != nil {
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

// CreatePlaceTool returns the MCP tool definition for creating a place.
func (t *PlaceTools) CreatePlaceTool() mcp.Tool {
	return mcp.Tool{
		Name:        "create_place",
		Description: "Create a new place. Accepts a JSON-LD place object and returns the created place with its assigned ID.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"place": map[string]interface{}{
					"type":        "object",
					"description": "JSON-LD place object with place details",
				},
			},
			Required: []string{"place"},
		},
	}
}

// CreatePlaceHandler handles the create_place tool call.
func (t *PlaceTools) CreatePlaceHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if t == nil || t.placesService == nil {
		return mcp.NewToolResultError("places service not configured"), nil
	}

	args := struct {
		Place json.RawMessage `json:"place"`
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

	if len(args.Place) == 0 {
		return mcp.NewToolResultError("place parameter is required"), nil
	}

	var raw map[string]any
	if err := json.Unmarshal(args.Place, &raw); err != nil {
		return mcp.NewToolResultErrorFromErr("invalid place payload", err), nil
	}

	params, err := parseCreatePlaceParams(raw, t.baseURL)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("invalid place payload", err), nil
	}

	place, err := t.placesService.Create(ctx, params)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("failed to create place", err), nil
	}

	response := map[string]any{
		"id":    place.ULID,
		"place": buildPlacePayload(place, t.baseURL),
	}
	if uri := buildPlaceURI(t.baseURL, place.ULID); uri != "" {
		response["@id"] = uri
	}

	return toolResultJSON(response)
}

func toolResultJSON(payload any) (*mcp.CallToolResult, error) {
	resultJSON, err := mcp.NewToolResultJSON(payload)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("failed to build response", err), nil
	}
	return resultJSON, nil
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

func parseCreatePlaceParams(raw map[string]any, baseURL string) (places.CreateParams, error) {
	var params places.CreateParams

	name := strings.TrimSpace(getString(raw["name"]))
	if name == "" {
		return params, fmt.Errorf("name is required")
	}
	params.Name = name
	params.Description = strings.TrimSpace(getString(raw["description"]))

	address := map[string]any{}
	if addrValue, ok := raw["address"]; ok {
		if addrMap, ok := addrValue.(map[string]any); ok {
			address = addrMap
		}
	}

	street := strings.TrimSpace(getString(address["streetAddress"]))
	locality := strings.TrimSpace(getString(address["addressLocality"]))
	region := strings.TrimSpace(getString(address["addressRegion"]))
	postal := strings.TrimSpace(getString(address["postalCode"]))
	country := strings.TrimSpace(getString(address["addressCountry"]))

	if street == "" {
		street = strings.TrimSpace(getString(raw["streetAddress"]))
	}
	if locality == "" {
		locality = strings.TrimSpace(getString(raw["addressLocality"]))
	}
	if region == "" {
		region = strings.TrimSpace(getString(raw["addressRegion"]))
	}
	if postal == "" {
		postal = strings.TrimSpace(getString(raw["postalCode"]))
	}
	if country == "" {
		country = strings.TrimSpace(getString(raw["addressCountry"]))
	}

	params.StreetAddress = street
	params.AddressLocality = locality
	params.AddressRegion = region
	params.PostalCode = postal
	params.AddressCountry = country

	var latValue *float64
	var lonValue *float64
	if geoValue, ok := raw["geo"]; ok {
		if geoMap, ok := geoValue.(map[string]any); ok {
			latValue = parseFloat(geoMap["latitude"])
			lonValue = parseFloat(geoMap["longitude"])
		}
	}
	if latValue == nil {
		latValue = parseFloat(raw["latitude"])
	}
	if lonValue == nil {
		lonValue = parseFloat(raw["longitude"])
	}
	if latValue != nil || lonValue != nil {
		if latValue == nil || lonValue == nil {
			return params, fmt.Errorf("geo requires both latitude and longitude")
		}
		if *latValue < -90 || *latValue > 90 {
			return params, fmt.Errorf("latitude must be between -90 and 90")
		}
		if *lonValue < -180 || *lonValue > 180 {
			return params, fmt.Errorf("longitude must be between -180 and 180")
		}
		params.Latitude = latValue
		params.Longitude = lonValue
	}

	if params.StreetAddress == "" && params.AddressLocality == "" && params.AddressRegion == "" && params.PostalCode == "" && params.AddressCountry == "" && params.Latitude == nil {
		return params, fmt.Errorf("address or geo is required")
	}

	if idValue := strings.TrimSpace(getString(raw["@id"])); idValue != "" {
		if strings.TrimSpace(baseURL) == "" {
			params.FederationURI = &idValue
		} else {
			if parsed, err := ids.ParseEntityURI(baseURL, "places", idValue, "federation"); err == nil {
				if parsed.Role == ids.RoleCanonical {
					params.ULID = parsed.ULID
				} else {
					params.FederationURI = &idValue
				}
			} else {
				params.FederationURI = &idValue
			}
		}
	}

	return params, nil
}

func getString(value any) string {
	if value == nil {
		return ""
	}
	if str, ok := value.(string); ok {
		return str
	}
	return ""
}

func parseFloat(value any) *float64 {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case float64:
		return &v
	case float32:
		converted := float64(v)
		return &converted
	case int:
		converted := float64(v)
		return &converted
	case int32:
		converted := float64(v)
		return &converted
	case int64:
		converted := float64(v)
		return &converted
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return nil
		}
		parsed, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return nil
		}
		return &parsed
	default:
		return nil
	}
}
