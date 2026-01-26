package integration

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlaceTombstoneAfterDelete(t *testing.T) {
	env := setupTestEnv(t)

	username := "admin"
	password := "admin-password-123"
	email := "admin@example.com"
	insertAdminUser(t, env, username, password, email, "admin")
	adminToken := adminLogin(t, env, username, password)

	place := insertPlace(t, env, "Centennial Park", "Toronto")
	require.NotEmpty(t, place.ULID)

	deleteReq, err := http.NewRequest(http.MethodDelete, env.Server.URL+"/api/v1/admin/places/"+place.ULID, nil)
	require.NoError(t, err)
	deleteReq.Header.Set("Authorization", "Bearer "+adminToken)

	deleteResp, err := env.Server.Client().Do(deleteReq)
	require.NoError(t, err)
	defer deleteResp.Body.Close()
	require.Equal(t, http.StatusNoContent, deleteResp.StatusCode)

	getReq, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/places/"+place.ULID, nil)
	require.NoError(t, err)
	getReq.Header.Set("Accept", "application/ld+json")

	getResp, err := env.Server.Client().Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()
	require.Equal(t, http.StatusGone, getResp.StatusCode)

	var tombstone map[string]any
	require.NoError(t, json.NewDecoder(getResp.Body).Decode(&tombstone))

	assert.Equal(t, true, tombstone["sel:tombstone"], "tombstone should be marked")
	assert.NotNil(t, tombstone["sel:deletedAt"], "tombstone should include deletedAt")
	assert.Equal(t, place.ULID, eventIDFromPayload(tombstone), "tombstone should preserve place ID")
}
