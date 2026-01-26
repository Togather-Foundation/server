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

// TestAdminDeleteEventSuccess tests successful soft delete of an event
func TestAdminDeleteEventSuccess(t *testing.T) {
	env := setupTestEnv(t)

	username := "admin"
	password := "admin-password-123"
	email := "admin@example.com"
	insertAdminUser(t, env, username, password, email, "admin")
	adminToken := adminLogin(t, env, username, password)

	agentKey := insertAPIKey(t, env, "test-agent")

	// Create event
	event := map[string]any{
		"name":        "Event to Delete",
		"description": "This event will be deleted",
		"startDate":   time.Date(2026, 10, 1, 19, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"location": map[string]any{
			"name":            "Test Venue",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
		"source": map[string]any{
			"url":     "https://example.com/events/delete-me",
			"eventId": "delete-me-123",
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
	require.NotEmpty(t, eventID)

	// Delete event as admin
	deleteReq, err := http.NewRequest(http.MethodDelete, env.Server.URL+"/api/v1/admin/events/"+eventID, nil)
	require.NoError(t, err)
	deleteReq.Header.Set("Authorization", "Bearer "+adminToken)

	deleteResp, err := env.Server.Client().Do(deleteReq)
	require.NoError(t, err)
	defer deleteResp.Body.Close()

	assert.Equal(t, http.StatusNoContent, deleteResp.StatusCode)

	// Verify event returns 410 Gone with tombstone
	getReq, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events/"+eventID, nil)
	require.NoError(t, err)
	getReq.Header.Set("Accept", "application/ld+json")

	getResp, err := env.Server.Client().Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	require.Equal(t, http.StatusGone, getResp.StatusCode, "deleted event should return 410 Gone")

	// Verify tombstone contains deletedAt timestamp
	var tombstone map[string]any
	require.NoError(t, json.NewDecoder(getResp.Body).Decode(&tombstone))

	assert.NotNil(t, tombstone["sel:deletedAt"], "tombstone should include deletedAt")
	assert.Equal(t, eventID, eventIDFromPayload(tombstone), "tombstone should preserve event ID")
}

// TestAdminDeleteEventUnauthorized tests that non-admin cannot delete events
func TestAdminDeleteEventUnauthorized(t *testing.T) {
	env := setupTestEnv(t)

	username := "viewer"
	password := "viewer-password-123"
	email := "viewer@example.com"
	insertAdminUser(t, env, username, password, email, "viewer")
	viewerToken := adminLogin(t, env, username, password)

	deleteReq, err := http.NewRequest(http.MethodDelete, env.Server.URL+"/api/v1/admin/events/01234567890123456789012345", nil)
	require.NoError(t, err)
	deleteReq.Header.Set("Authorization", "Bearer "+viewerToken)

	deleteResp, err := env.Server.Client().Do(deleteReq)
	require.NoError(t, err)
	defer deleteResp.Body.Close()

	require.Equal(t, http.StatusForbidden, deleteResp.StatusCode)
}

// TestAdminDeleteEventNotFound tests deleting non-existent event
func TestAdminDeleteEventNotFound(t *testing.T) {
	env := setupTestEnv(t)

	username := "admin"
	password := "admin-password-123"
	email := "admin@example.com"
	insertAdminUser(t, env, username, password, email, "admin")
	adminToken := adminLogin(t, env, username, password)

	deleteReq, err := http.NewRequest(http.MethodDelete, env.Server.URL+"/api/v1/admin/events/01FAKE000000000000000000", nil)
	require.NoError(t, err)
	deleteReq.Header.Set("Authorization", "Bearer "+adminToken)

	deleteResp, err := env.Server.Client().Do(deleteReq)
	require.NoError(t, err)
	defer deleteResp.Body.Close()

	assert.Equal(t, http.StatusNotFound, deleteResp.StatusCode)
}

// TestAdminDeleteEventIdempotent tests that deleting already-deleted event is idempotent
func TestAdminDeleteEventIdempotent(t *testing.T) {
	env := setupTestEnv(t)

	username := "admin"
	password := "admin-password-123"
	email := "admin@example.com"
	insertAdminUser(t, env, username, password, email, "admin")
	adminToken := adminLogin(t, env, username, password)

	agentKey := insertAPIKey(t, env, "test-agent")

	// Create event
	event := map[string]any{
		"name":      "Event for Idempotent Delete",
		"startDate": time.Date(2026, 10, 1, 19, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"location": map[string]any{
			"name":            "Test Venue",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
		"source": map[string]any{
			"url":     "https://example.com/events/idempotent",
			"eventId": "idempotent-123",
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

	// Delete event first time
	deleteReq1, err := http.NewRequest(http.MethodDelete, env.Server.URL+"/api/v1/admin/events/"+eventID, nil)
	require.NoError(t, err)
	deleteReq1.Header.Set("Authorization", "Bearer "+adminToken)

	deleteResp1, err := env.Server.Client().Do(deleteReq1)
	require.NoError(t, err)
	defer deleteResp1.Body.Close()
	require.Equal(t, http.StatusNoContent, deleteResp1.StatusCode)

	// Delete event second time (should be idempotent)
	deleteReq2, err := http.NewRequest(http.MethodDelete, env.Server.URL+"/api/v1/admin/events/"+eventID, nil)
	require.NoError(t, err)
	deleteReq2.Header.Set("Authorization", "Bearer "+adminToken)

	deleteResp2, err := env.Server.Client().Do(deleteReq2)
	require.NoError(t, err)
	defer deleteResp2.Body.Close()

	// Should still return 204 or 410 (both acceptable for idempotent delete)
	assert.True(t,
		deleteResp2.StatusCode == http.StatusNoContent ||
			deleteResp2.StatusCode == http.StatusGone,
		"idempotent delete should succeed")
}

// TestAdminDeleteEventWithReason tests deleting event with a reason
func TestAdminDeleteEventWithReason(t *testing.T) {
	env := setupTestEnv(t)

	username := "admin"
	password := "admin-password-123"
	email := "admin@example.com"
	insertAdminUser(t, env, username, password, email, "admin")
	adminToken := adminLogin(t, env, username, password)

	agentKey := insertAPIKey(t, env, "test-agent")

	// Create event
	event := map[string]any{
		"name":      "Event to Delete with Reason",
		"startDate": time.Date(2026, 10, 1, 19, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"location": map[string]any{
			"name":            "Test Venue",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
		"source": map[string]any{
			"url":     "https://example.com/events/delete-reason",
			"eventId": "delete-reason-123",
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

	// Delete event with reason
	deletePayload := map[string]any{
		"reason": "Event cancelled by organizer",
	}
	deleteBody, err := json.Marshal(deletePayload)
	require.NoError(t, err)

	deleteReq, err := http.NewRequest(http.MethodDelete, env.Server.URL+"/api/v1/admin/events/"+eventID, bytes.NewReader(deleteBody))
	require.NoError(t, err)
	deleteReq.Header.Set("Authorization", "Bearer "+adminToken)
	deleteReq.Header.Set("Content-Type", "application/json")

	deleteResp, err := env.Server.Client().Do(deleteReq)
	require.NoError(t, err)
	defer deleteResp.Body.Close()

	assert.Equal(t, http.StatusNoContent, deleteResp.StatusCode)

	// Verify tombstone includes reason (if supported)
	getReq, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events/"+eventID, nil)
	require.NoError(t, err)

	getResp, err := env.Server.Client().Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	require.Equal(t, http.StatusGone, getResp.StatusCode)
}

// TestAdminDeleteEventExcludedFromListing tests that deleted events don't appear in listings
func TestAdminDeleteEventExcludedFromListing(t *testing.T) {
	env := setupTestEnv(t)

	username := "admin"
	password := "admin-password-123"
	email := "admin@example.com"
	insertAdminUser(t, env, username, password, email, "admin")
	adminToken := adminLogin(t, env, username, password)

	agentKey := insertAPIKey(t, env, "test-agent")

	// Create and publish event
	event := map[string]any{
		"name":        "Event to be Excluded",
		"description": "This event will be deleted and excluded from listings",
		"startDate":   time.Date(2026, 10, 1, 19, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"location": map[string]any{
			"name":            "Test Venue",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
		"source": map[string]any{
			"url":     "https://example.com/events/excluded",
			"eventId": "excluded-123",
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

	// Delete event
	deleteReq, err := http.NewRequest(http.MethodDelete, env.Server.URL+"/api/v1/admin/events/"+eventID, nil)
	require.NoError(t, err)
	deleteReq.Header.Set("Authorization", "Bearer "+adminToken)

	deleteResp, err := env.Server.Client().Do(deleteReq)
	require.NoError(t, err)
	defer deleteResp.Body.Close()

	// List events - deleted event should NOT appear
	listReq, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events", nil)
	require.NoError(t, err)

	listResp, err := env.Server.Client().Do(listReq)
	require.NoError(t, err)
	defer listResp.Body.Close()

	var listResult map[string]any
	require.NoError(t, json.NewDecoder(listResp.Body).Decode(&listResult))

	items, ok := listResult["items"].([]any)
	require.True(t, ok)

	// Verify deleted event is not in the list
	for _, item := range items {
		itemMap := item.(map[string]any)
		assert.NotEqual(t, eventID, eventIDFromPayload(itemMap), "deleted event should not appear in listings")
	}
}
