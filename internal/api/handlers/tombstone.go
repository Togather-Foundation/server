package handlers

import (
	"encoding/json"
	"net/http"
	"time"
)

// WriteTombstoneResponse writes a 410 Gone response with tombstone data.
// If the tombstone payload can be unmarshaled, it uses that; otherwise,
// it creates a minimal tombstone response with the provided entity type.
//
// Parameters:
//   - w: HTTP response writer
//   - r: HTTP request
//   - payload: JSON payload from the tombstone record
//   - deletedAt: Timestamp when the entity was deleted
//   - entityType: JSON-LD @type (e.g., "Event", "Place", "Organization")
func WriteTombstoneResponse(w http.ResponseWriter, r *http.Request, payload []byte, deletedAt time.Time, entityType string) {
	var parsedPayload map[string]any
	if err := json.Unmarshal(payload, &parsedPayload); err != nil {
		// Fallback to minimal tombstone if payload parsing fails
		parsedPayload = map[string]any{
			"@context":      loadDefaultContext(),
			"@type":         entityType,
			"sel:tombstone": true,
			"sel:deletedAt": deletedAt.Format(time.RFC3339),
		}
	}
	writeJSON(w, http.StatusGone, parsedPayload, contentTypeFromRequest(r))
}
