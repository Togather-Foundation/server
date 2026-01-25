package events

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

type DedupCandidate struct {
	Name      string
	VenueID   string
	StartDate string
}

func BuildDedupHash(candidate DedupCandidate) string {
	name := strings.ToLower(strings.TrimSpace(candidate.Name))
	venue := strings.ToLower(strings.TrimSpace(candidate.VenueID))
	start := strings.TrimSpace(candidate.StartDate)
	payload := strings.Join([]string{name, venue, start}, "|")
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}
