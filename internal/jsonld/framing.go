package jsonld

import (
	"errors"

	"github.com/piprate/json-gold/ld"
)

var (
	ErrInvalidFrame   = errors.New("invalid JSON-LD frame")
	ErrMissingContext = errors.New("context document missing @context")
)

// NewFrameOptions returns default JSON-LD framing options.
func NewFrameOptions(base string) *ld.JsonLdOptions {
	opts := ld.NewJsonLdOptions(base)
	opts.Embed = "@always"
	opts.OmitGraph = true
	return opts
}

// Frame applies a JSON-LD frame to a document.
func Frame(document any, frame map[string]any, opts *ld.JsonLdOptions) (map[string]any, error) {
	if frame == nil {
		return nil, ErrInvalidFrame
	}
	if opts == nil {
		opts = NewFrameOptions("")
	}

	processor := ld.NewJsonLdProcessor()
	result, err := processor.Frame(document, frame, opts)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// EntityFrame builds a frame for a single entity type (Event, Place, Organization).
func EntityFrame(contextDoc map[string]any, entityType string) (map[string]any, error) {
	ctx, err := extractContext(contextDoc)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"@context": ctx,
		"@type":    entityType,
	}, nil
}

// EntityListFrame builds a frame for list responses of a given type.
func EntityListFrame(contextDoc map[string]any, entityType string) (map[string]any, error) {
	return EntityFrame(contextDoc, entityType)
}

func extractContext(contextDoc map[string]any) (any, error) {
	ctx, ok := contextDoc["@context"]
	if !ok {
		return nil, ErrMissingContext
	}
	return ctx, nil
}
