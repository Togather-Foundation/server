package mcp

import (
	"context"
	"testing"

	"github.com/Togather-Foundation/server/internal/auth"
)

// TestPublicTools asserts the read-only tools are in the public allowlist and
// the write/account tools are not. This is the map that gates anonymous MCP
// sessions — drift here changes who can call what without a key.
func TestPublicTools(t *testing.T) {
	for _, name := range []string{"events", "places", "organizations", "search", "geocode_address", "reverse_geocode"} {
		if !publicTools[name] {
			t.Errorf("tool %q should be public but is not in publicTools", name)
		}
	}
	for _, name := range []string{"add_event", "api_keys", "manage_api_key"} {
		if publicTools[name] {
			t.Errorf("tool %q must require a key but is in publicTools", name)
		}
	}
}

// TestToolRequiresKey exercises the enforcement function registered as the MCP
// tool-handler middleware: anonymous sessions may call public tools but not
// write tools; authenticated sessions may call both.
func TestToolRequiresKey(t *testing.T) {
	authedCtx := auth.ContextWithAgentKey(context.Background(), &auth.APIKey{ID: "test-key"})

	cases := []struct {
		name      string
		tool      string
		ctx       context.Context
		wantError bool
	}{
		{"anonymous can call public tool", "events", context.Background(), false},
		{"anonymous cannot call write tool", "add_event", context.Background(), true},
		{"anonymous cannot call account tool", "api_keys", context.Background(), true},
		{"authenticated can call public tool", "events", authedCtx, false},
		{"authenticated can call write tool", "add_event", authedCtx, false},
		{"authenticated can call account tool", "manage_api_key", authedCtx, false},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			err := toolRequiresKey(tt.tool, tt.ctx)
			if tt.wantError {
				if err == nil {
					t.Errorf("expected error for %q, got nil", tt.tool)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error for %q: %v", tt.tool, err)
			}
		})
	}
}
