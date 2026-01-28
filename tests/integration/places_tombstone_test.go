package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
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
	defer func() { _ = deleteResp.Body.Close() }()
	require.Equal(t, http.StatusNoContent, deleteResp.StatusCode)

	getReq, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/places/"+place.ULID, nil)
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
	assert.Equal(t, place.ULID, eventIDFromPayload(tombstone), "tombstone should preserve place ID")
}

// TestPlaceTombstoneHTMLFormat verifies that HTML tombstones return 410 Gone
func TestPlaceTombstoneHTMLFormat(t *testing.T) {
	env := setupTestEnv(t)

	username := "admin"
	password := "admin-password-123"
	email := "admin@example.com"
	insertAdminUser(t, env, username, password, email, "admin")
	adminToken := adminLogin(t, env, username, password)

	place := insertPlace(t, env, "Test Place HTML", "Toronto")
	require.NotEmpty(t, place.ULID)

	// Delete the place
	deleteReq, err := http.NewRequest(http.MethodDelete, env.Server.URL+"/api/v1/admin/places/"+place.ULID, nil)
	require.NoError(t, err)
	deleteReq.Header.Set("Authorization", "Bearer "+adminToken)

	deleteResp, err := env.Server.Client().Do(deleteReq)
	require.NoError(t, err)
	defer func() { _ = deleteResp.Body.Close() }()
	require.Equal(t, http.StatusNoContent, deleteResp.StatusCode)

	// Request HTML format via public pages endpoint
	getReq, err := http.NewRequest(http.MethodGet, env.Server.URL+"/places/"+place.ULID, nil)
	require.NoError(t, err)
	getReq.Header.Set("Accept", "text/html")

	getResp, err := env.Server.Client().Do(getReq)
	require.NoError(t, err)
	defer func() { _ = getResp.Body.Close() }()

	// Verify 410 Gone status
	require.Equal(t, http.StatusGone, getResp.StatusCode, "HTML tombstone should return 410 Gone")
	require.Contains(t, getResp.Header.Get("Content-Type"), "text/html")

	// Verify HTML contains tombstone markers
	body, err := io.ReadAll(getResp.Body)
	require.NoError(t, err)
	htmlContent := string(body)
	require.Contains(t, htmlContent, "<!DOCTYPE html>")
}

// TestPlaceTombstoneTurtleFormat verifies that Turtle tombstones return 410 Gone
func TestPlaceTombstoneTurtleFormat(t *testing.T) {
	env := setupTestEnv(t)

	username := "admin"
	password := "admin-password-123"
	email := "admin@example.com"
	insertAdminUser(t, env, username, password, email, "admin")
	adminToken := adminLogin(t, env, username, password)

	place := insertPlace(t, env, "Test Place Turtle", "Toronto")
	require.NotEmpty(t, place.ULID)

	// Delete the place
	deleteReq, err := http.NewRequest(http.MethodDelete, env.Server.URL+"/api/v1/admin/places/"+place.ULID, nil)
	require.NoError(t, err)
	deleteReq.Header.Set("Authorization", "Bearer "+adminToken)

	deleteResp, err := env.Server.Client().Do(deleteReq)
	require.NoError(t, err)
	defer func() { _ = deleteResp.Body.Close() }()
	require.Equal(t, http.StatusNoContent, deleteResp.StatusCode)

	// Request Turtle format via public pages endpoint
	getReq, err := http.NewRequest(http.MethodGet, env.Server.URL+"/places/"+place.ULID, nil)
	require.NoError(t, err)
	getReq.Header.Set("Accept", "text/turtle")

	getResp, err := env.Server.Client().Do(getReq)
	require.NoError(t, err)
	defer func() { _ = getResp.Body.Close() }()

	// Verify 410 Gone status
	require.Equal(t, http.StatusGone, getResp.StatusCode, "Turtle tombstone should return 410 Gone")
	require.Contains(t, getResp.Header.Get("Content-Type"), "text/turtle")

	// Verify Turtle contains RDF syntax
	body, err := io.ReadAll(getResp.Body)
	require.NoError(t, err)
	turtleContent := strings.TrimSpace(string(body))
	require.Contains(t, turtleContent, "@prefix")
}
