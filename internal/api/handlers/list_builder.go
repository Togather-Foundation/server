package handlers

import (
	"github.com/Togather-Foundation/server/internal/domain/ids"
)

// BuildBaseListItem creates a base JSON-LD list item with @context, @type, name, and @id.
// Additional fields can be added by the caller.
//
// Parameters:
//   - entityType: JSON-LD @type (e.g., "Event", "Place", "Organization")
//   - name: Human-readable name of the entity
//   - ulid: ULID identifier for the entity
//   - entityPath: Path segment for canonical URI (e.g., "events", "places", "organizations")
//   - baseURL: Base URL for building canonical URIs
//
// Returns a map ready for JSON serialization with @context, @type, name, and @id fields.
func BuildBaseListItem(entityType, name, ulid, entityPath, baseURL string) map[string]any {
	item := map[string]any{
		"@context": loadDefaultContext(),
		"@type":    entityType,
		"name":     name,
	}

	// Add @id (required per Interop Profile §3.1)
	if uri, err := ids.BuildCanonicalURI(baseURL, entityPath, ulid); err == nil {
		item["@id"] = uri
	}

	return item
}
