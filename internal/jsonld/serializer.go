package jsonld

import (
	"errors"

	"github.com/piprate/json-gold/ld"
)

var ErrInvalidDocument = errors.New("invalid JSON-LD document")

// Serializer handles JSON-LD compaction and framing using versioned contexts.
type Serializer struct {
	loader *ContextLoader
}

// NewSerializer constructs a serializer with the provided context loader.
func NewSerializer(loader *ContextLoader) *Serializer {
	if loader == nil {
		loader = defaultLoader
	}
	return &Serializer{loader: loader}
}

// Compact compacts a JSON-LD document using the specified context version.
func (s *Serializer) Compact(document any, version string) (map[string]any, error) {
	ctxDoc, err := s.loader.Load(version)
	if err != nil {
		return nil, err
	}
	ctx, err := extractContext(ctxDoc)
	if err != nil {
		return nil, err
	}

	processor := ld.NewJsonLdProcessor()
	opts := ld.NewJsonLdOptions("")
	opts.CompactArrays = true

	result, err := processor.Compact(document, ctx, opts)
	if err != nil {
		return nil, err
	}

	compacted, ok := result.(map[string]any)
	if !ok {
		return nil, ErrInvalidDocument
	}
	return compacted, nil
}

// Frame applies a JSON-LD frame to a document with optional framing options.
func (s *Serializer) Frame(document any, frame map[string]any, opts *ld.JsonLdOptions) (map[string]any, error) {
	return Frame(document, frame, opts)
}

// FrameAndCompact frames a document and then compacts the result with the specified context version.
func (s *Serializer) FrameAndCompact(document any, frame map[string]any, version string) (map[string]any, error) {
	framed, err := Frame(document, frame, nil)
	if err != nil {
		return nil, err
	}
	return s.Compact(framed, version)
}
