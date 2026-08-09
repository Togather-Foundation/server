package tools

import (
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// Tool annotation expectations per the MCP 2025-11-25 spec. Every tool must
// carry a title; read-only tools advertise readOnlyHint; destructive tools
// advertise destructiveHint so clients can gate confirmation prompts.
type annotationExpectation struct {
	title       string
	readOnly    *bool
	destructive *bool
}

func boolPtr(b bool) *bool {
	return mcp.ToBoolPtr(b)
}

func TestToolAnnotations(t *testing.T) {
	tests := []struct {
		name string
		tool mcp.Tool
		want annotationExpectation
	}{
		{
			name: "events is read-only",
			tool: (NewEventTools(nil, nil, "")).EventsTool(),
			want: annotationExpectation{title: "List or get events", readOnly: boolPtr(true), destructive: boolPtr(false)},
		},
		{
			name: "places is read-only",
			tool: (NewPlaceTools(nil, "")).PlacesTool(),
			want: annotationExpectation{title: "List or get places", readOnly: boolPtr(true), destructive: boolPtr(false)},
		},
		{
			name: "organizations is read-only",
			tool: (NewOrganizationTools(nil, "")).OrganizationsTool(),
			want: annotationExpectation{title: "List or get organizations", readOnly: boolPtr(true), destructive: boolPtr(false)},
		},
		{
			name: "search is read-only",
			tool: (NewSearchTools(nil, nil, nil, "")).SearchTool(),
			want: annotationExpectation{title: "Search events, places, and organizations", readOnly: boolPtr(true), destructive: boolPtr(false)},
		},
		{
			name: "geocode_address is read-only",
			tool: (NewGeocodingTools(nil)).GeocodeAddressTool(),
			want: annotationExpectation{title: "Geocode an address to coordinates", readOnly: boolPtr(true), destructive: boolPtr(false)},
		},
		{
			name: "reverse_geocode is read-only",
			tool: (NewGeocodingTools(nil)).ReverseGeocodeTool(),
			want: annotationExpectation{title: "Reverse geocode coordinates to an address", readOnly: boolPtr(true), destructive: boolPtr(false)},
		},
		{
			name: "add_event is a non-destructive write",
			tool: (NewEventTools(nil, nil, "")).AddEventTool(),
			want: annotationExpectation{title: "Create a new event", readOnly: boolPtr(false), destructive: boolPtr(false)},
		},
		{
			name: "api_keys is read-only",
			tool: (NewDeveloperTools(nil, "")).APIKeysTool(),
			want: annotationExpectation{title: "List API keys or get usage statistics", readOnly: boolPtr(true), destructive: boolPtr(false)},
		},
		{
			name: "manage_api_key is destructive",
			tool: (NewDeveloperTools(nil, "")).ManageAPIKeyTool(),
			want: annotationExpectation{title: "Create or revoke API keys", readOnly: boolPtr(false), destructive: boolPtr(true)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := tt.tool.Annotations
			if a.Title != tt.want.title {
				t.Errorf("title = %q, want %q", a.Title, tt.want.title)
			}
			if got := derefBool(a.ReadOnlyHint); got != derefBool(tt.want.readOnly) {
				t.Errorf("readOnlyHint = %v, want %v", derefBool(a.ReadOnlyHint), derefBool(tt.want.readOnly))
			}
			if got := derefBool(a.DestructiveHint); got != derefBool(tt.want.destructive) {
				t.Errorf("destructiveHint = %v, want %v", derefBool(a.DestructiveHint), derefBool(tt.want.destructive))
			}
		})
	}
}

func derefBool(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}
