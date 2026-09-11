package identity

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestValidateAuthorityURI_FullMatch verifies the full-match semantics: a URI
// that merely contains the pattern must be rejected, and a fully-matching URI
// is accepted.
func TestValidateAuthorityURI_FullMatch(t *testing.T) {
	// An unanchored pattern: "kg.artsdata.ca" would match a substring of the URI.
	require.ErrorIs(t, validateAuthorityURI(`kg\.artsdata\.ca`, "https://kg.artsdata.ca/resource/K11-24"),
		ErrInvalidURI, "a URI containing a match must still be rejected without a full match")

	require.NoError(t, validateAuthorityURI(`https?://kg\.artsdata\.ca/resource/K\d+-\d+$`, "https://kg.artsdata.ca/resource/K11-24"))
}

// TestValidateAuthorityURI_BadPattern verifies an invalid pattern is an internal
// error, not a structural ErrInvalidURI.
func TestValidateAuthorityURI_BadPattern(t *testing.T) {
	err := validateAuthorityURI(`([invalid`, "https://kg.artsdata.ca/resource/K11-24")
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrInvalidURI)
}
