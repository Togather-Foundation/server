package scraper

import (
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time interface compliance checks.
var _ Extractor = (*RestExtractor)(nil)
var _ Extractor = (*GraphQLExtractor)(nil)

func TestNewExtractor_REST(t *testing.T) {
	t.Parallel()
	source := SourceConfig{Name: "rest-src", REST: &RestConfig{Endpoint: "http://example.com"}}
	ext, err := NewExtractor(source, zerolog.Nop())
	require.NoError(t, err)
	assert.IsType(t, &RestExtractor{}, ext)
}

func TestNewExtractor_GraphQL(t *testing.T) {
	t.Parallel()
	source := SourceConfig{Name: "gql-src", GraphQL: &GraphQLConfig{Endpoint: "http://example.com"}}
	ext, err := NewExtractor(source, zerolog.Nop())
	require.NoError(t, err)
	assert.IsType(t, &GraphQLExtractor{}, ext)
}

func TestNewExtractor_REST_Precedence(t *testing.T) {
	t.Parallel()
	source := SourceConfig{
		Name:    "both-src",
		REST:    &RestConfig{Endpoint: "http://example.com"},
		GraphQL: &GraphQLConfig{Endpoint: "http://example.com"},
	}
	ext, err := NewExtractor(source, zerolog.Nop())
	require.NoError(t, err)
	assert.IsType(t, &RestExtractor{}, ext, "REST must take precedence when both configs present")
}

func TestNewExtractor_NoConfig(t *testing.T) {
	t.Parallel()
	source := SourceConfig{Name: "empty-src"}
	_, err := NewExtractor(source, zerolog.Nop())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNoExtractorConfig)
	assert.Contains(t, err.Error(), "empty-src")
}
