package auth

import "context"

// contextKeyAgent is the context key under which an authenticated API key is
// stored. It lives here (not in the api/middleware package) so both the HTTP
// auth middleware and the MCP tool layer can read the same value without a
// layering violation (internal/mcp must not import internal/api).
type contextKeyAgent struct{}

// ContextWithAgentKey returns a copy of ctx carrying the given API key.
// The key is present only for authenticated requests; absent keys are not
// stored (callers must not store nil).
func ContextWithAgentKey(ctx context.Context, key *APIKey) context.Context {
	if key == nil {
		return ctx
	}
	return context.WithValue(ctx, contextKeyAgent{}, key)
}

// AgentKeyFromContext returns the authenticated API key stored in ctx, or nil
// if the request is anonymous.
func AgentKeyFromContext(ctx context.Context) *APIKey {
	if key, ok := ctx.Value(contextKeyAgent{}).(*APIKey); ok {
		return key
	}
	return nil
}
