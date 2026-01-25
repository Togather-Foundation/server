package handlers

import "github.com/Togather-Foundation/server/internal/jsonld"

func loadDefaultContext() any {
	ctxDoc, err := jsonld.LoadDefaultContext()
	if err != nil {
		return nil
	}
	if ctx, ok := ctxDoc["@context"]; ok {
		return ctx
	}
	return nil
}
