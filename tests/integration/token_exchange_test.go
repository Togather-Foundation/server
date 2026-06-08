package integration

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/auth"
	"github.com/stretchr/testify/require"
)

func TestTokenExchangeIntegration(t *testing.T) {
	env := setupTestEnv(t)

	rawKey := insertAdminAPIKey(t, env, "integration-test-admin-key")

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
	require.NotEmpty(t, claims.Subject)

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

func insertAdminAPIKey(t *testing.T, env *testEnv, name string) string {
	t.Helper()

	rawBytes := make([]byte, 16)
	_, err := rand.Read(rawBytes)
	require.NoError(t, err)
	key := hex.EncodeToString(rawBytes)
	prefix := key[:8]
	hash := auth.HashAPIKeySHA256(key)

	_, err = env.Pool.Exec(env.Context,
		`INSERT INTO api_keys (prefix, key_hash, hash_version, name, role) VALUES ($1, $2, $3, $4, $5)`,
		prefix, hash, auth.HashVersionSHA256, name, string(auth.RoleAdmin),
	)
	require.NoError(t, err, "failed to insert admin API key")

	return key
}
