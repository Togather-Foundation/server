package integration

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrganizationTombstoneAfterDelete(t *testing.T) {
	env := setupTestEnv(t)

	username := "admin"
	password := "admin-password-123"
	email := "admin@example.com"
	insertAdminUser(t, env, username, password, email, "admin")
	adminToken := adminLogin(t, env, username, password)

	org := insertOrganization(t, env, "Toronto Arts Org")
	require.NotEmpty(t, org.ULID)

	deleteReq, err := http.NewRequest(http.MethodDelete, env.Server.URL+"/api/v1/admin/organizations/"+org.ULID, nil)
	require.NoError(t, err)
	deleteReq.Header.Set("Authorization", "Bearer "+adminToken)

	deleteResp, err := env.Server.Client().Do(deleteReq)
	require.NoError(t, err)
	defer func() { _ = deleteResp.Body.Close() }()
	require.Equal(t, http.StatusNoContent, deleteResp.StatusCode)

	getReq, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/organizations/"+org.ULID, nil)
	require.NoError(t, err)
	getReq.Header.Set("Accept", "application/ld+json")

	getResp, err := env.Server.Client().Do(getReq)
	require.NoError(t, err)
	defer func() { _ = getResp.Body.Close() }()
	require.Equal(t, http.StatusGone, getResp.StatusCode)

	var tombstone map[string]any
	require.NoError(t, json.NewDecoder(getResp.Body).Decode(&tombstone))

	assert.Equal(t, true, tombstone["sel:tombstone"], "tombstone should be marked")
	assert.NotNil(t, tombstone["sel:deletedAt"], "tombstone should include deletedAt")
	assert.Equal(t, org.ULID, eventIDFromPayload(tombstone), "tombstone should preserve organization ID")
}
