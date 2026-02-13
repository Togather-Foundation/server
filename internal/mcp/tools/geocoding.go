package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/Togather-Foundation/server/internal/geocoding"
	"github.com/mark3labs/mcp-go/mcp"
)

// GeocodingTools provides MCP tools for geocoding addresses and place names.
type GeocodingTools struct {
	service *geocoding.GeocodingService
}

// NewGeocodingTools creates a new GeocodingTools instance.
func NewGeocodingTools(service *geocoding.GeocodingService) *GeocodingTools {
	return &GeocodingTools{
		service: service,
	}
}

// GeocodeAddressTool returns the MCP tool definition for geocoding addresses.
func (t *GeocodingTools) GeocodeAddressTool() mcp.Tool {
	return mcp.Tool{
		Name:        "geocode_address",
		Description: "Geocode an address or place name to geographic coordinates (latitude/longitude). Uses OpenStreetMap Nominatim with caching. Returns coordinates and a human-readable display name.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"address": map[string]interface{}{
					"type":        "string",
					"description": "Address or place name to geocode (e.g., 'Toronto City Hall' or '100 Queen St W, Toronto, ON')",
				},
				"country_codes": map[string]interface{}{
					"type":        "string",
					"description": "Comma-separated ISO 3166-1 alpha-2 country codes to limit results (e.g., 'ca,us'). Default: 'ca'",
					"default":     "ca",
				},
			},
			Required: []string{"address"},
		},
	}
}

// GeocodeAddressHandler handles the geocode_address tool call.
func (t *GeocodingTools) GeocodeAddressHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if t == nil || t.service == nil {
		return mcp.NewToolResultError("geocoding service not configured"), nil
	}

	args := struct {
		Address      string `json:"address"`
		CountryCodes string `json:"country_codes"`
	}{
		CountryCodes: "ca", // Default
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

	// Validate address
	address := strings.TrimSpace(args.Address)
	if address == "" {
		return mcp.NewToolResultError("address parameter is required"), nil
	}

	// Geocode the address
	result, err := t.service.Geocode(ctx, address, args.CountryCodes)
	if err != nil {
		if errors.Is(err, geocoding.ErrNoResults) {
			return mcp.NewToolResultErrorf("no results found for address: %s", address), nil
		}
		return mcp.NewToolResultErrorFromErr("geocoding failed", err), nil
	}

	// Build response
	response := map[string]interface{}{
		"latitude":     result.Latitude,
		"longitude":    result.Longitude,
		"display_name": result.DisplayName,
		"source":       result.Source,
		"cached":       result.Cached,
	}

	return toolResultJSON(response)
}

// ReverseGeocodeTool returns the MCP tool definition for reverse geocoding.
func (t *GeocodingTools) ReverseGeocodeTool() mcp.Tool {
	return mcp.Tool{
		Name:        "reverse_geocode",
		Description: "Reverse geocode geographic coordinates to a human-readable address. Converts latitude/longitude to an address with structured components (road, city, state, etc.). Uses OpenStreetMap Nominatim with caching.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"latitude": map[string]interface{}{
					"type":        "number",
					"description": "Latitude coordinate (must be between -90 and 90)",
				},
				"longitude": map[string]interface{}{
					"type":        "number",
					"description": "Longitude coordinate (must be between -180 and 180)",
				},
			},
			Required: []string{"latitude", "longitude"},
		},
	}
}

// ReverseGeocodeHandler handles the reverse_geocode tool call.
func (t *GeocodingTools) ReverseGeocodeHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if t == nil || t.service == nil {
		return mcp.NewToolResultError("geocoding service not configured"), nil
	}

	args := struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
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

	// Validate coordinates
	if args.Latitude < -90 || args.Latitude > 90 {
		return mcp.NewToolResultError("latitude must be between -90 and 90"), nil
	}
	if args.Longitude < -180 || args.Longitude > 180 {
		return mcp.NewToolResultError("longitude must be between -180 and 180"), nil
	}

	// Reverse geocode the coordinates
	result, err := t.service.ReverseGeocode(ctx, args.Latitude, args.Longitude)
	if err != nil {
		if errors.Is(err, geocoding.ErrNoResults) {
			return mcp.NewToolResultErrorf("no results found for coordinates: lat=%f, lon=%f", args.Latitude, args.Longitude), nil
		}
		return mcp.NewToolResultErrorFromErr("reverse geocoding failed", err), nil
	}

	// Build response
	response := map[string]interface{}{
		"display_name": result.DisplayName,
		"address":      result.Address,
		"latitude":     result.Latitude,
		"longitude":    result.Longitude,
	}

	return toolResultJSON(response)
}
