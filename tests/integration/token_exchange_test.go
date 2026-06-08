package integration

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/auth"
	"github.com/Togather-Foundation/server/tests/testhelpers"
	"github.com/stretchr/testify/require"
)

func TestTokenExchangeIntegration(t *testing.T) {
	env := setupTestEnv(t)

	rawKey := testhelpers.InsertAPIKeyWithRole(t, env.Pool, env.Context, "integration-test-admin-key", "admin")

	var keyID string
	err := env.Pool.QueryRow(env.Context, `SELECT id FROM api_keys WHERE prefix = $1`, rawKey[:8]).Scan(&keyID)
	require.NoError(t, err, "failed to query API key ID")

	req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/auth/token", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	req.Header.Set("Accept", "application/json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var tokenResp struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&tokenResp))
	require.NotEmpty(t, tokenResp.Token)
	require.NotEmpty(t, tokenResp.ExpiresAt)

	adminKey, err := auth.DeriveAdminJWTKey([]byte(env.Config.Auth.JWTSecret))
	require.NoError(t, err, "failed to derive admin JWT key")
	jwtMgr := auth.NewJWTManagerFromKey(adminKey, time.Hour, "sel.events")
	claims, err := jwtMgr.Validate(tokenResp.Token)
	require.NoError(t, err, "JWT validation failed")
	require.Equal(t, "admin", claims.Role)
	require.Equal(t, keyID, claims.Subject, "JWT subject must match the API key ID")

	expiresAt, err := time.Parse(time.RFC3339, tokenResp.ExpiresAt)
	require.NoError(t, err, "expires_at parsing failed")
	require.False(t, expiresAt.IsZero())

	adminReq, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/admin/stats", nil)
	require.NoError(t, err)
	adminReq.Header.Set("Authorization", "Bearer "+tokenResp.Token)
	adminReq.Header.Set("Accept", "application/json")

	adminResp, err := env.Server.Client().Do(adminReq)
	require.NoError(t, err)
	defer func() { _ = adminResp.Body.Close() }()

	require.Equal(t, http.StatusOK, adminResp.StatusCode)
}

func TestTokenExchangeAgentKeyForbidden(t *testing.T) {
	env := setupTestEnv(t)

	rawKey := insertAPIKey(t, env, "integration-test-agent-key")

	req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/auth/token", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	req.Header.Set("Accept", "application/json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusForbidden, resp.StatusCode)
}
