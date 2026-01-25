package jsonld

import (
	"testing"

	"github.com/piprate/json-gold/ld"
	"github.com/stretchr/testify/require"
)

func TestNewFrameOptionsDefaults(t *testing.T) {
	opts := NewFrameOptions("")

	require.Equal(t, ld.Embed("@always"), opts.Embed)
	require.True(t, opts.OmitGraph)
}

func TestFrameInvalidFrame(t *testing.T) {
	_, err := Frame(map[string]any{}, nil, nil)

	require.ErrorIs(t, err, ErrInvalidFrame)
}

func TestFrameUsesProvidedOptions(t *testing.T) {
	opts := ld.NewJsonLdOptions("")
	opts.OmitGraph = false

	document := map[string]any{"@context": map[string]any{}, "@id": "urn:example"}
	frame := map[string]any{"@context": map[string]any{}, "@id": "urn:example"}

	result, err := Frame(document, frame, opts)

	require.NoError(t, err)
	require.False(t, opts.OmitGraph)
	require.NotNil(t, result)
}

func TestEntityFrameMissingContext(t *testing.T) {
	_, err := EntityFrame(map[string]any{}, "Event")

	require.ErrorIs(t, err, ErrMissingContext)
}

func TestEntityFrameSuccess(t *testing.T) {
	ctxDoc := map[string]any{"@context": map[string]any{"name": "https://schema.org/name"}}

	frame, err := EntityFrame(ctxDoc, "Event")

	require.NoError(t, err)
	require.Equal(t, "Event", frame["@type"])
	require.Contains(t, frame, "@context")
}

func TestEntityListFrameUsesEntityFrame(t *testing.T) {
	ctxDoc := map[string]any{"@context": map[string]any{"name": "https://schema.org/name"}}

	frame, err := EntityListFrame(ctxDoc, "Organization")

	require.NoError(t, err)
	require.Equal(t, "Organization", frame["@type"])
}
