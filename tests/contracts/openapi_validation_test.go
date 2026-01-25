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
	require.Contains(t, paths, "/events")
	require.Contains(t, paths, "/events/{id}")
}
