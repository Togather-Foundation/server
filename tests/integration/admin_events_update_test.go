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

// TestAdminUpdateEventSuccess tests successfully updating an event's fields
func TestAdminUpdateEventSuccess(t *testing.T) {
	env := setupTestEnv(t)

	// Insert admin user
	username := "admin"
	password := "admin-password-123"
	email := "admin@example.com"
	insertAdminUser(t, env, username, password, email, "admin")
	adminToken := adminLogin(t, env, username, password)

	// Create agent and submit an event
	agentKey := insertAPIKey(t, env, "test-agent")

	event := map[string]any{
		"name":        "Original Event Name",
		"description": "Original description",
		"startDate":   time.Date(2026, 10, 1, 19, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"location": map[string]any{
			"name":            "Original Venue",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
		"source": map[string]any{
			"url":     "https://example.com/events/original",
			"eventId": "original-123",
		},
	}

	// Submit event
	body, err := json.Marshal(event)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+agentKey)
	req.Header.Set("Content-Type", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var created map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	eventID := eventIDFromPayload(created)
	require.NotEmpty(t, eventID)

	// Update event as admin
	updates := map[string]any{
		"name":        "Updated Event Name",
		"description": "Updated description",
	}

	updateBody, err := json.Marshal(updates)
	require.NoError(t, err)

	updateReq, err := http.NewRequest(http.MethodPut, env.Server.URL+"/api/v1/admin/events/"+eventID, bytes.NewReader(updateBody))
	require.NoError(t, err)
	updateReq.Header.Set("Authorization", "Bearer "+adminToken)
	updateReq.Header.Set("Content-Type", "application/json")

	updateResp, err := env.Server.Client().Do(updateReq)
	require.NoError(t, err)
	defer updateResp.Body.Close()

	assert.Equal(t, http.StatusOK, updateResp.StatusCode)
}

// TestAdminUpdateEventPublishDraft tests publishing a draft event
func TestAdminUpdateEventPublishDraft(t *testing.T) {
	env := setupTestEnv(t)

	username := "admin"
	password := "admin-password-123"
	email := "admin@example.com"
	insertAdminUser(t, env, username, password, email, "admin")
	adminToken := adminLogin(t, env, username, password)

	agentKey := insertAPIKey(t, env, "test-agent")

	event := map[string]any{
		"name":       "Draft Event",
		"startDate":  time.Date(2026, 10, 1, 19, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"confidence": 0.5, // Low confidence - will be draft
		"location": map[string]any{
			"name":            "Test Venue",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
		"source": map[string]any{
			"url":     "https://example.com/events/draft",
			"eventId": "draft-123",
		},
	}

	body, err := json.Marshal(event)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+agentKey)
	req.Header.Set("Content-Type", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var created map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	eventID := eventIDFromPayload(created)

	// Publish event
	updates := map[string]any{
		"lifecycle_state": "published",
	}

	updateBody, err := json.Marshal(updates)
	require.NoError(t, err)

	updateReq, err := http.NewRequest(http.MethodPut, env.Server.URL+"/api/v1/admin/events/"+eventID, bytes.NewReader(updateBody))
	require.NoError(t, err)
	updateReq.Header.Set("Authorization", "Bearer "+adminToken)
	updateReq.Header.Set("Content-Type", "application/json")

	updateResp, err := env.Server.Client().Do(updateReq)
	require.NoError(t, err)
	defer updateResp.Body.Close()

	assert.Equal(t, http.StatusOK, updateResp.StatusCode)
}

// TestAdminUpdateEventUnauthorized tests that non-admin cannot update events
func TestAdminUpdateEventUnauthorized(t *testing.T) {
	env := setupTestEnv(t)

	username := "viewer"
	password := "viewer-password-123"
	email := "viewer@example.com"
	insertAdminUser(t, env, username, password, email, "viewer")
	viewerToken := adminLogin(t, env, username, password)

	updates := map[string]any{
		"name": "Unauthorized Update",
	}

	updateBody, err := json.Marshal(updates)
	require.NoError(t, err)

	updateReq, err := http.NewRequest(http.MethodPut, env.Server.URL+"/api/v1/admin/events/01234567890123456789012345", bytes.NewReader(updateBody))
	require.NoError(t, err)
	updateReq.Header.Set("Authorization", "Bearer "+viewerToken)
	updateReq.Header.Set("Content-Type", "application/json")

	updateResp, err := env.Server.Client().Do(updateReq)
	require.NoError(t, err)
	defer updateResp.Body.Close()

	require.Equal(t, http.StatusForbidden, updateResp.StatusCode)
}

// TestAdminUpdateEventNotFound tests updating non-existent event
func TestAdminUpdateEventNotFound(t *testing.T) {
	env := setupTestEnv(t)

	username := "admin"
	password := "admin-password-123"
	email := "admin@example.com"
	insertAdminUser(t, env, username, password, email, "admin")
	adminToken := adminLogin(t, env, username, password)

	updates := map[string]any{
		"name": "Updated Name",
	}

	updateBody, err := json.Marshal(updates)
	require.NoError(t, err)

	updateReq, err := http.NewRequest(http.MethodPut, env.Server.URL+"/api/v1/admin/events/01FAKE000000000000000000", bytes.NewReader(updateBody))
	require.NoError(t, err)
	updateReq.Header.Set("Authorization", "Bearer "+adminToken)
	updateReq.Header.Set("Content-Type", "application/json")

	updateResp, err := env.Server.Client().Do(updateReq)
	require.NoError(t, err)
	defer updateResp.Body.Close()

	assert.Equal(t, http.StatusNotFound, updateResp.StatusCode)
}

// TestAdminUpdateEventValidation tests validation of update fields
func TestAdminUpdateEventValidation(t *testing.T) {
	env := setupTestEnv(t)

	username := "admin"
	password := "admin-password-123"
	email := "admin@example.com"
	insertAdminUser(t, env, username, password, email, "admin")
	adminToken := adminLogin(t, env, username, password)

	tests := []struct {
		name    string
		updates map[string]any
	}{
		{
			name:    "empty name",
			updates: map[string]any{"name": ""},
		},
		{
			name:    "name too long",
			updates: map[string]any{"name": string(make([]byte, 501))}, // > 500 chars
		},
		{
			name:    "invalid lifecycle_state",
			updates: map[string]any{"lifecycle_state": "invalid_state"},
		},
		{
			name:    "invalid date format",
			updates: map[string]any{"startDate": "not-a-date"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updateBody, err := json.Marshal(tt.updates)
			require.NoError(t, err)

			updateReq, err := http.NewRequest(http.MethodPut, env.Server.URL+"/api/v1/admin/events/01234567890123456789012345", bytes.NewReader(updateBody))
			require.NoError(t, err)
			updateReq.Header.Set("Authorization", "Bearer "+adminToken)
			updateReq.Header.Set("Content-Type", "application/json")

			updateResp, err := env.Server.Client().Do(updateReq)
			require.NoError(t, err)
			updateResp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, updateResp.StatusCode, "validation should fail for: %s", tt.name)
		})
	}
}
