package identity

import (
	"encoding/hex"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

// pairObs builds identifier observations from authority|uri pairs.
func pairObs(pairs ...[2]string) []IdentifierObservation {
	out := make([]IdentifierObservation, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, IdentifierObservation{Authority: p[0], URI: p[1]})
	}
	return out
}

// TestFingerprint_OrderIndependent verifies the fingerprint is symmetric in its
// arguments and depends only on the shared signals.
func TestFingerprint_OrderIndependent(t *testing.T) {
	shared := [2]string{"artsdata", "https://kg.artsdata.ca/resource/K11-24"}

	// a and b carry different single-sided identifiers; only `shared` overlaps.
	a := pairObs(shared, [2]string{"wikidata", "https://www.wikidata.org/entity/Q1"})
	b := pairObs(shared, [2]string{"musicbrainz", "https://musicbrainz.org/place/abc"})

	require.Equal(t, Fingerprint(a, b), Fingerprint(b, a))
	require.Equal(t, Fingerprint(pairObs(shared), pairObs(shared)), Fingerprint(a, b),
		"single-sided identifiers must not affect the fingerprint")
}

// TestFingerprint_SingleSidedIgnored verifies an identifier asserted by only one
// entity does not affect the fingerprint.
func TestFingerprint_SingleSidedIgnored(t *testing.T) {
	shared := [2]string{"artsdata", "https://kg.artsdata.ca/resource/K11-24"}

	base := Fingerprint(pairObs(shared), pairObs(shared))

	// b asserts an extra, unshared identifier.
	withExtra := Fingerprint(pairObs(shared), pairObs(shared, [2]string{"wikidata", "https://www.wikidata.org/entity/Q9"}))
	require.Equal(t, base, withExtra, "a single-sided identifier must not change the fingerprint")

	// a asserts the extra identifier instead of b.
	withExtraOnA := Fingerprint(pairObs(shared, [2]string{"wikidata", "https://www.wikidata.org/entity/Q9"}), pairObs(shared))
	require.Equal(t, base, withExtraOnA, "a single-sided identifier on the other side must not change the fingerprint")
}

// TestFingerprint_SharedSignalChange verifies a newly shared signal changes the
// fingerprint (the re-open condition).
func TestFingerprint_SharedSignalChange(t *testing.T) {
	shared := [2]string{"artsdata", "https://kg.artsdata.ca/resource/K11-24"}

	before := Fingerprint(pairObs(shared), pairObs(shared))

	bothGain := Fingerprint(
		pairObs(shared, [2]string{"wikidata", "https://www.wikidata.org/entity/Q1"}),
		pairObs(shared, [2]string{"wikidata", "https://www.wikidata.org/entity/Q1"}),
	)
	require.NotEqual(t, before, bothGain, "a newly shared signal must change the fingerprint")
}

// TestFingerprint_Deterministic verifies the output is a stable, hex-encoded
// SHA-256 digest.
func TestFingerprint_Deterministic(t *testing.T) {
	a := pairObs([2]string{"artsdata", "https://kg.artsdata.ca/resource/K11-24"})
	b := pairObs([2]string{"artsdata", "https://kg.artsdata.ca/resource/K11-24"})

	fp := Fingerprint(a, b)

	require.Regexp(t, regexp.MustCompile(`^[0-9a-f]{64}$`), fp, "fingerprint must be a 64-char hex digest")
	require.Equal(t, fp, Fingerprint(a, b), "fingerprint must be deterministic")

	_, err := hex.DecodeString(fp)
	require.NoError(t, err)
}

// TestFingerprint_NoSharedSignals verifies two entities with disjoint identifiers
// produce the digest of the empty string.
func TestFingerprint_NoSharedSignals(t *testing.T) {
	a := pairObs([2]string{"artsdata", "https://kg.artsdata.ca/resource/K11-24"})
	b := pairObs([2]string{"wikidata", "https://www.wikidata.org/entity/Q1"})

	fp := Fingerprint(a, b)
	require.Equal(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", fp,
		"no shared signals must equal SHA-256 of the empty string")
}
