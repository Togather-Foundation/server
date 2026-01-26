package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventTombstoneAfterDelete(t *testing.T) {
	env := setupTestEnv(t)

	username := "admin"
	password := "admin-password-123"
	email := "admin@example.com"
	insertAdminUser(t, env, username, password, email, "admin")
	adminToken := adminLogin(t, env, username, password)

	agentKey := insertAPIKey(t, env, "test-agent")

	event := map[string]any{
		"name":        "Event Tombstone Test",
		"description": "This event will be deleted",
		"startDate":   time.Date(2026, 11, 1, 19, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"location": map[string]any{
			"name":            "Test Venue",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
		"source": map[string]any{
			"url":     "https://example.com/events/tombstone-test",
			"eventId": "tombstone-test-123",
		},
	}

	body, err := json.Marshal(event)
	require.NoError(t, err)

	createReq, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
	require.NoError(t, err)
	createReq.Header.Set("Authorization", "Bearer "+agentKey)
	createReq.Header.Set("Content-Type", "application/ld+json")

	createResp, err := env.Server.Client().Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	var created map[string]any
	require.NoError(t, json.NewDecoder(createResp.Body).Decode(&created))
	eventID := eventIDFromPayload(created)
	require.NotEmpty(t, eventID)

	deleteReq, err := http.NewRequest(http.MethodDelete, env.Server.URL+"/api/v1/admin/events/"+eventID, nil)
	require.NoError(t, err)
	deleteReq.Header.Set("Authorization", "Bearer "+adminToken)

	deleteResp, err := env.Server.Client().Do(deleteReq)
	require.NoError(t, err)
	defer deleteResp.Body.Close()
	require.Equal(t, http.StatusNoContent, deleteResp.StatusCode)

	getReq, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events/"+eventID, nil)
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
	assert.Equal(t, eventID, eventIDFromPayload(tombstone), "tombstone should preserve event ID")
}
