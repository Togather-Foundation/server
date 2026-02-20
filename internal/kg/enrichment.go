package kg

import (
	"context"

	"github.com/Togather-Foundation/server/internal/storage/postgres"
)

// EnrichmentStore defines the persistence methods used by EnrichmentWorker.
// Defined by the consumer (idiomatic Go); *postgres.Queries satisfies it.
type EnrichmentStore interface {
	UpsertEntityIdentifier(ctx context.Context, arg postgres.UpsertEntityIdentifierParams) (postgres.EntityIdentifier, error)
}

// compile-time assertion: *postgres.Queries must satisfy EnrichmentStore.
var _ EnrichmentStore = (*postgres.Queries)(nil)

// ExtractStringValue extracts a plain string from a JSON-LD interface{} value.
// Handles the following variants:
//   - plain string → returned as-is
//   - map[string]interface{} with "@value" key → the value string is returned
//   - nil or other types → empty string
func ExtractStringValue(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case map[string]interface{}:
		if s, ok := val["@value"].(string); ok {
			return s
		}
		return ""
	default:
		return ""
	}
}
