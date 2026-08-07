package tools

import (
	"encoding/json"

	"github.com/Togather-Foundation/server/internal/jsonld"
	"github.com/mark3labs/mcp-go/mcp"
)

// aliasParams maps camelCase REST-style parameter names to their snake_case
// MCP-style equivalents. MCP tools expose snake_case names; agents that carry
// REST parameter names across interfaces (startDate, endDate, q, after, ...)
// would otherwise silently get no filter. Accepting both conventions removes
// that trap.
var aliasParams = map[string]string{
	"startDate": "start_date",
	"endDate":   "end_date",
	"q":         "query",
	"after":     "cursor",
	"nearLat":   "near_lat",
	"nearLon":   "near_lon",
	"radiusKm":  "radius",
}

// normalizeArgs canonicalizes tool call arguments: for every known camelCase
// alias present without its snake_case counterpart, copy the value over so the
// existing snake_case-based parsing sees it. Prefer the explicit snake_case
// value when both are supplied.
func normalizeArgs(args map[string]any) {
	for camel, snake := range aliasParams {
		camelVal, camelOK := args[camel]
		snakeVal, snakeOK := args[snake]
		if !snakeOK && camelOK {
			args[snake] = camelVal
		} else if snakeOK && !camelOK {
			args[camel] = snakeVal
		}
	}
}

// unmarshalArgs decodes MCP tool call arguments into the target struct,
// normalizing REST-style camelCase aliases to their MCP snake_case names first.
func unmarshalArgs(request mcp.CallToolRequest, out any) error {
	if request.Params.Arguments == nil {
		return nil
	}
	args, ok := request.Params.Arguments.(map[string]any)
	if !ok {
		data, err := json.Marshal(request.Params.Arguments)
		if err != nil {
			return err
		}
		return json.Unmarshal(data, out)
	}
	normalizeArgs(args)
	data, err := json.Marshal(args)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

// defaultContext returns the default JSON-LD context for SEL entities.
// It attempts to load the full context document, falling back to a stable
// context URI if loading fails (SEL compliant).
func defaultContext() any {
	ctxDoc, err := jsonld.LoadDefaultContext()
	if err != nil {
		// Return stable context URI as fallback (SEL compliant)
		return "https://sel.togather.foundation/contexts/sel/v0.1.jsonld"
	}
	if ctx, ok := ctxDoc["@context"]; ok {
		return ctx
	}
	return "https://sel.togather.foundation/contexts/sel/v0.1.jsonld"
}

// decodeTombstonePayload decodes a tombstone payload from JSON bytes.
// Returns an empty map if the payload is empty.
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

// toolResultJSON converts a payload to an MCP tool result with JSON content.
// Returns a tool error result if the conversion fails.
func toolResultJSON(payload any) (*mcp.CallToolResult, error) {
	resultJSON, err := mcp.NewToolResultJSON(payload)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("failed to build response", err), nil
	}
	return resultJSON, nil
}
