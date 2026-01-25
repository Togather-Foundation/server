package integration

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

type healthPayload struct {
	Status string `json:"status"`
}

func TestHealthz(t *testing.T) {
	env := setupTestEnv(t)

	resp, err := env.Server.Client().Get(env.Server.URL + "/healthz")
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var payload healthPayload
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	require.Equal(t, "ok", payload.Status)
}

func TestReadyz(t *testing.T) {
	env := setupTestEnv(t)

	resp, err := env.Server.Client().Get(env.Server.URL + "/readyz")
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var payload healthPayload
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	require.Equal(t, "ready", payload.Status)
}
