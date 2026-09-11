package contracts_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/auth"
	"github.com/stretchr/testify/require"
)

// identityRoute is a single identity admin endpoint under test.
type identityRoute struct {
	method string
	path   string
	body   map[string]any
}

// TestIdentityRoutesRequireAdminAuth verifies the five identity admin routes are
// behind the admin JWT gate: 401 without a token, 403 for a non-admin token, and
// a reachable handler (non-401/403) with an admin token.
func TestIdentityRoutesRequireAdminAuth(t *testing.T) {
	env, token := setupAdminEnv(t)

	// A non-admin JWT signed with the same derived admin key but role "agent".
	key, err := auth.DeriveAdminJWTKey([]byte(sharedConfig.Auth.JWTSecret))
	require.NoError(t, err)
	agentMgr := auth.NewJWTManagerFromKey(key, time.Hour, "sel.events")
	agentToken, err := agentMgr.Generate("agent-1", "agent")
	require.NoError(t, err)

	placeA := insertPlace(t, env, "Identity Place A", "Toronto")
	placeB := insertPlace(t, env, "Identity Place B", "Toronto")

	routes := []identityRoute{
		{method: http.MethodGet, path: "/api/v1/admin/identity/place/" + placeA.ULID},
		{method: http.MethodGet, path: "/api/v1/admin/identity/conflicts?type=place"},
		{method: http.MethodGet, path: "/api/v1/admin/identity/decisions"},
		{method: http.MethodPost, path: "/api/v1/admin/identity/link", body: map[string]any{
			"entity_type": "place", "entity_id": placeA.ULID,
			"authority": "artsdata", "uri": "https://kg.artsdata.ca/resource/K11-24",
		}},
		{method: http.MethodPost, path: "/api/v1/admin/identity/reject", body: map[string]any{
			"entity_type": "place", "entity_id": placeA.ULID, "counterpart_id": placeB.ULID,
			"reason": "distinct",
		}},
	}

	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			// No token → 401.
			resp := doIdentityRequest(t, env, route, "")
			require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "no token must 401")
			_ = resp.Body.Close()

			// Non-admin token → 403.
			resp = doIdentityRequest(t, env, route, agentToken)
			require.Equal(t, http.StatusForbidden, resp.StatusCode, "non-admin token must 403")
			_ = resp.Body.Close()

			// Admin token → reaches the handler (not 401/403).
			resp = doIdentityRequest(t, env, route, token)
			require.NotEqual(t, http.StatusUnauthorized, resp.StatusCode, "admin token must not 401")
			require.NotEqual(t, http.StatusForbidden, resp.StatusCode, "admin token must not 403")
			_ = resp.Body.Close()
		})
	}
}

func doIdentityRequest(t *testing.T, env *testEnv, route identityRoute, token string) *http.Response {
	t.Helper()
	var body bytes.Buffer
	if route.body != nil {
		require.NoError(t, json.NewEncoder(&body).Encode(route.body))
	}
	req, err := http.NewRequest(route.method, env.Server.URL+route.path, &body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	return resp
}
