package events

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

// DedupCandidate holds the fields used to compute a dedup hash for an event.
// The VenueKey should be produced by NormalizeVenueKey to ensure consistent
// hashing regardless of how the venue was originally represented (place ID,
// name string, etc.).
type DedupCandidate struct {
	Name      string
	VenueID   string // Normalized venue key — use NormalizeVenueKey() to populate
	StartDate string
}

// collapseSpaces matches two or more consecutive whitespace characters.
var collapseSpaces = regexp.MustCompile(`\s{2,}`)

// NormalizeVenueKey produces a canonical venue key for dedup hashing.
//
// Normalization rules:
//   - If a place ID is available (from a resolved place or source-provided @id),
//     use it as-is after trimming whitespace. IDs are already canonical identifiers.
//   - If only a venue name is available, normalize it: lowercase, trim leading/trailing
//     whitespace, and collapse internal runs of whitespace to a single space.
//   - For virtual-only events, use the virtual location URL (trimmed).
//   - Returns empty string if no venue information is available.
//
// This ensures that "  The  Rex  Jazz Bar " and "the rex jazz bar" produce the
// same venue key when no ID is available.
func NormalizeVenueKey(input EventInput) string {
	if input.Location != nil {
		if id := strings.TrimSpace(input.Location.ID); id != "" {
			return id
		}
		if name := strings.TrimSpace(input.Location.Name); name != "" {
			// Normalize: lowercase + collapse internal whitespace
			normalized := strings.ToLower(name)
			normalized = collapseSpaces.ReplaceAllString(normalized, " ")
			return normalized
		}
	}
	if input.VirtualLocation != nil {
		return strings.TrimSpace(input.VirtualLocation.URL)
	}
	return ""
}

// BuildDedupHash computes a deterministic SHA-256 hash from the candidate fields.
//
// Hash input format: "name|venue|startDate" where:
//   - name is lowercased and trimmed
//   - venue is the pre-normalized venue key (from NormalizeVenueKey); trimmed
//     and lowercased again for safety but callers should pre-normalize
//   - startDate is trimmed but NOT lowercased (RFC3339 uses uppercase T/Z)
//
// The hash is deterministic: same normalized inputs always produce the same hash.
func BuildDedupHash(candidate DedupCandidate) string {
	name := strings.ToLower(strings.TrimSpace(candidate.Name))
	name = collapseSpaces.ReplaceAllString(name, " ")
	venue := strings.ToLower(strings.TrimSpace(candidate.VenueID))
	venue = collapseSpaces.ReplaceAllString(venue, " ")
	start := strings.TrimSpace(candidate.StartDate)
	payload := strings.Join([]string{name, venue, start}, "|")
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}
