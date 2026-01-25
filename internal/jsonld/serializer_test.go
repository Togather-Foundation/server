package jsonld

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewSerializerDefaults(t *testing.T) {
	serializer := NewSerializer(nil)

	require.NotNil(t, serializer)
	require.Equal(t, defaultLoader, serializer.loader)
}

func TestSerializerCompactSuccess(t *testing.T) {
	loader := newTestContextLoader(t, `{"@context":{"name":"https://schema.org/name"}}`)
	serializer := NewSerializer(loader)

	document := map[string]any{
		"@context": map[string]any{"name": "https://schema.org/name"},
		"name":     "Jazz Fest",
	}

	compacted, err := serializer.Compact(document, "v1")

	require.NoError(t, err)
	require.Equal(t, "Jazz Fest", compacted["name"])
}

func TestSerializerCompactMissingContext(t *testing.T) {
	loader := newTestContextLoader(t, `{}`)
	serializer := NewSerializer(loader)

	_, err := serializer.Compact(map[string]any{}, "v1")

	require.ErrorIs(t, err, ErrMissingContext)
}

func TestSerializerFrameAndCompactInvalidFrame(t *testing.T) {
	loader := newTestContextLoader(t, `{"@context":{"name":"https://schema.org/name"}}`)
	serializer := NewSerializer(loader)

	_, err := serializer.FrameAndCompact(map[string]any{}, nil, "v1")

	require.ErrorIs(t, err, ErrInvalidFrame)
}

func newTestContextLoader(t *testing.T, doc string) *ContextLoader {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "v1.jsonld")
	require.NoError(t, os.WriteFile(path, []byte(doc), 0o600))

	return NewContextLoader(dir)
}
