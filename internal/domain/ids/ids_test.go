package ids

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const testULID = "01HYX3KQW7ERTV9XNBM2P8QJZF"

func TestNewULIDReturnsValid(t *testing.T) {
	value, err := NewULID()

	require.NoError(t, err)
	require.NoError(t, ValidateULID(value))
}

func TestIsULIDAndValidateULID(t *testing.T) {
	require.True(t, IsULID(testULID))
	require.True(t, IsULID(" "+testULID+" "))
	require.NoError(t, ValidateULID(testULID))

	require.False(t, IsULID("not-a-ulid"))
	require.ErrorIs(t, ValidateULID("not-a-ulid"), ErrInvalidULID)
}

func TestBuildCanonicalURI(t *testing.T) {
	uri, err := BuildCanonicalURI("example.org", "events", testULID)

	require.NoError(t, err)
	require.Equal(t, "https://example.org/events/"+testULID, uri)

	_, err = BuildCanonicalURI("", "events", testULID)

	require.ErrorIs(t, err, ErrInvalidNodeDomain)

	_, err = BuildCanonicalURI("example.org", "", testULID)

	require.ErrorIs(t, err, ErrInvalidEntityPath)

	_, err = BuildCanonicalURI("example.org", "events", "bad")

	require.ErrorIs(t, err, ErrInvalidULID)
}

func TestParseEntityURICanonical(t *testing.T) {
	uri := "https://example.org/events/" + testULID

	parsed, err := ParseEntityURI("example.org", "events", uri, "")

	require.NoError(t, err)
	require.Equal(t, uri, parsed.URI)
	require.Equal(t, testULID, parsed.ULID)
	require.Equal(t, RoleCanonical, parsed.Role)
	require.Equal(t, "example.org", parsed.Host)
	require.Equal(t, "/events/"+testULID, parsed.Path)
}

func TestParseEntityURIForeignRequiresOrigin(t *testing.T) {
	uri := "https://other.org/events/" + testULID

	_, err := ParseEntityURI("example.org", "events", uri, "")

	require.ErrorIs(t, err, ErrMissingOriginNodeID)
}

func TestParseEntityURIForeignWithOrigin(t *testing.T) {
	uri := "https://other.org/events/" + testULID

	parsed, err := ParseEntityURI("example.org", "events", uri, "origin")

	require.NoError(t, err)
	require.Equal(t, RoleForeign, parsed.Role)
	require.Equal(t, testULID, parsed.ULID)
}

func TestParseEntityURIMismatchedPath(t *testing.T) {
	uri := "https://example.org/places/" + testULID

	_, err := ParseEntityURI("example.org", "events", uri, "")

	require.ErrorIs(t, err, ErrMismatchedEntityPath)
}

func TestParseEntityURIInvalidURI(t *testing.T) {
	_, err := ParseEntityURI("example.org", "events", "not-a-uri", "")

	require.ErrorIs(t, err, ErrInvalidURI)
}

func TestNormalizeSameAs(t *testing.T) {
	uri, err := NormalizeSameAs("example.org", "events", "https://other.org/events/"+testULID)

	require.NoError(t, err)
	require.Equal(t, "https://other.org/events/"+testULID, uri)

	uri, err = NormalizeSameAs("example.org", "events", "/events/"+testULID)

	require.NoError(t, err)
	require.Equal(t, "https://example.org/events/"+testULID, uri)

	uri, err = NormalizeSameAs("example.org", "events", testULID)

	require.NoError(t, err)
	require.Equal(t, "https://example.org/events/"+testULID, uri)

	uri, err = NormalizeSameAs("example.org", "events", "events/"+testULID)

	require.NoError(t, err)
	require.Equal(t, "https://example.org/events/"+testULID, uri)

	_, err = NormalizeSameAs("example.org", "events", "")

	require.ErrorIs(t, err, ErrUnsupportedSameAs)

	_, err = NormalizeSameAs("example.org", "events", "not-valid")

	require.ErrorIs(t, err, ErrUnsupportedSameAs)
}
