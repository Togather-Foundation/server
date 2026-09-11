package identity

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// Fingerprint computes the signal-scoped evidence fingerprint for an unordered
// pair of entities. Only identifiers asserted by BOTH entities (the shared
// "authority|uri" signals) contribute; identifiers held by a single entity are
// ignored, so an unrelated new identifier on one side does not change the
// fingerprint and cannot re-open a suppressed pair.
//
// The fingerprint is the hex-encoded SHA-256 of the sorted, newline-joined
// "authority|uri" values of the shared signals. It is order-independent:
// Fingerprint(a, b) == Fingerprint(b, a). The value is stored keyed by the
// canonical (id_a < id_b) pair in identity_not_duplicates and recomputed on
// read to detect a material change in evidence.
func Fingerprint(a, b []IdentifierObservation) string {
	aSignals := make(map[string]struct{}, len(a))
	for _, o := range a {
		if o.Authority == "" || o.URI == "" {
			continue
		}
		aSignals[o.Authority+"|"+o.URI] = struct{}{}
	}

	shared := make([]string, 0, len(b))
	seen := make(map[string]struct{}, len(b))
	for _, o := range b {
		if o.Authority == "" || o.URI == "" {
			continue
		}
		sig := o.Authority + "|" + o.URI
		if _, ok := aSignals[sig]; !ok {
			continue
		}
		if _, dup := seen[sig]; dup {
			continue
		}
		seen[sig] = struct{}{}
		shared = append(shared, sig)
	}

	sort.Strings(shared)
	sum := sha256.Sum256([]byte(strings.Join(shared, "\n")))
	return hex.EncodeToString(sum[:])
}
