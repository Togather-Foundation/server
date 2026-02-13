package tools

import (
	"encoding/json"

	"github.com/Togather-Foundation/server/internal/jsonld"
	"github.com/mark3labs/mcp-go/mcp"
)

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
