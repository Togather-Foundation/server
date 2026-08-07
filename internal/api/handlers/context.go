package handlers

import "github.com/Togather-Foundation/server/internal/jsonld"

// fallbackContextURI is the stable SEL context URI returned when the embedded
// context document cannot be loaded or lacks an "@context" key. It mirrors the
// MCP tools' defaultContext() fallback so API responses never emit a JSON-LD
// document with no context at all (critical for ?context=document mode, where
// a single top-level @context is the only one in the response).
const fallbackContextURI = "https://sel.togather.foundation/contexts/sel/v0.1.jsonld"

func loadDefaultContext() any {
	ctxDoc, err := jsonld.LoadDefaultContext()
	if err != nil {
		return fallbackContextURI
	}
	if ctx, ok := ctxDoc["@context"]; ok {
		return ctx
	}
	return fallbackContextURI
}
