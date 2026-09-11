// Package tombstones builds the canonical URI and JSON-LD tombstone payload for
// soft-deleted places and organizations. It is the single home for these
// builders, shared by the admin HTTP handlers (delete flows) and the domain
// service (merge flows), so the tombstone shape cannot drift between the two.
package tombstones

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/ids"
)

// BuildPlaceURI constructs the canonical SEL URI for a place ULID.
// Returns an empty string when the base URL or ULID is empty/invalid.
func BuildPlaceURI(baseURL, ulid string) string {
	if baseURL == "" || ulid == "" {
		return ""
	}
	uri, err := ids.BuildCanonicalURI(baseURL, "places", ulid)
	if err != nil {
		return ""
	}
	return uri
}

// BuildOrganizationURI constructs the canonical SEL URI for an organization ULID.
// Returns an empty string when the base URL or ULID is empty/invalid.
func BuildOrganizationURI(baseURL, ulid string) string {
	if baseURL == "" || ulid == "" {
		return ""
	}
	uri, err := ids.BuildCanonicalURI(baseURL, "organizations", ulid)
	if err != nil {
		return ""
	}
	return uri
}

// BuildPlaceTombstonePayload marshals a JSON-LD tombstone for a soft-deleted place.
func BuildPlaceTombstonePayload(ulid, name, reason, baseURL string) ([]byte, error) {
	placeURI := BuildPlaceURI(baseURL, ulid)
	if placeURI == "" {
		placeURI = "https://togather.foundation/places/" + strings.ToUpper(ulid)
	}

	payload := map[string]any{
		"@context":           "https://schema.org",
		"@type":              "Place",
		"@id":                placeURI,
		"name":               name,
		"sel:tombstone":      true,
		"sel:deletedAt":      time.Now().Format(time.RFC3339),
		"sel:deletionReason": reason,
	}

	return json.Marshal(payload)
}

// BuildOrganizationTombstonePayload marshals a JSON-LD tombstone for a soft-deleted organization.
func BuildOrganizationTombstonePayload(ulid, name, reason, baseURL string) ([]byte, error) {
	orgURI := BuildOrganizationURI(baseURL, ulid)
	if orgURI == "" {
		orgURI = "https://togather.foundation/organizations/" + strings.ToUpper(ulid)
	}

	payload := map[string]any{
		"@context":           "https://schema.org",
		"@type":              "Organization",
		"@id":                orgURI,
		"name":               name,
		"sel:tombstone":      true,
		"sel:deletedAt":      time.Now().Format(time.RFC3339),
		"sel:deletionReason": reason,
	}

	return json.Marshal(payload)
}
