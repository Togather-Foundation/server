package contracts_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAPIEndpointAvailable(t *testing.T) {
	env := setupTestEnv(t)

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/openapi.json", nil)
	require.NoError(t, err)

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var payload map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	require.Equal(t, "3.1.0", payload["openapi"])

	paths, ok := payload["paths"].(map[string]any)
	require.True(t, ok)
	// Server base URL is the node root, so paths carry the full /api/v1 prefix.
	require.Contains(t, paths, "/api/v1/events")
	require.Contains(t, paths, "/api/v1/events/{id}")
	require.NotContains(t, paths, "/events", "paths must be relative to the node root and include /api/v1")
}
