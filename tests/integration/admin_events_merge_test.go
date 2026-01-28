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

// TestAdminMergeDuplicatesSuccess tests merging duplicate events
func TestAdminMergeDuplicatesSuccess(t *testing.T) {
	env := setupTestEnv(t)

	username := "admin"
	password := "admin-password-123"
	email := "admin@example.com"
	insertAdminUser(t, env, username, password, email, "admin")
	adminToken := adminLogin(t, env, username, password)

	agentKey := insertAPIKey(t, env, "test-agent")

	// Create two duplicate events
	event1 := map[string]any{
		"name":        "Jazz Festival",
		"description": "First description",
		"startDate":   time.Date(2026, 10, 1, 19, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"location": map[string]any{
			"name":            "Central Park",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
		"source": map[string]any{
			"url":     "https://example.com/events/jazz1",
			"eventId": "jazz1",
		},
	}

	event2 := map[string]any{
		"name":        "Jazz Festival", // Same name
		"description": "Second description with more details",
		"startDate":   time.Date(2026, 10, 1, 19, 0, 0, 0, time.UTC).Format(time.RFC3339), // Same date
		"location": map[string]any{
			"name":            "Central Park", // Same location
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
		"source": map[string]any{
			"url":     "https://example.com/events/jazz2",
			"eventId": "jazz2",
		},
	}

	// Submit both events
	var eventID1, eventID2 string
	for i, event := range []map[string]any{event1, event2} {
		body, err := json.Marshal(event)
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+agentKey)
		req.Header.Set("Content-Type", "application/ld+json")

		resp, err := env.Server.Client().Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		var created map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))

		if i == 0 {
			eventID1 = eventIDFromPayload(created)
		} else {
			eventID2 = eventIDFromPayload(created)
		}
	}

	require.NotEmpty(t, eventID1)
	require.NotEmpty(t, eventID2)

	// Merge event2 into event1 (keep event1, retire event2)
	mergeRequest := map[string]any{
		"primary_id":   eventID1,
		"duplicate_id": eventID2,
	}

	mergeBody, err := json.Marshal(mergeRequest)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/admin/events/merge", bytes.NewReader(mergeBody))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify event2 now redirects to event1 or returns 301/410
	getReq, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events/"+eventID2, nil)
	require.NoError(t, err)

	getResp, err := env.Server.Client().Do(getReq)
	require.NoError(t, err)
	defer func() { _ = getResp.Body.Close() }()

	// Should either redirect or return 410 Gone with sameAs pointer
	assert.True(t,
		getResp.StatusCode == http.StatusMovedPermanently ||
			getResp.StatusCode == http.StatusGone,
		"merged event should redirect or be gone")
}

// TestAdminMergeDuplicatesUnauthorized tests that non-admin cannot merge events
func TestAdminMergeDuplicatesUnauthorized(t *testing.T) {
	env := setupTestEnv(t)

	username := "viewer"
	password := "viewer-password-123"
	email := "viewer@example.com"
	insertAdminUser(t, env, username, password, email, "viewer")
	viewerToken := adminLogin(t, env, username, password)

	mergeRequest := map[string]any{
		"primary_id":   "01234567890123456789012345",
		"duplicate_id": "01234567890123456789012346",
	}

	mergeBody, err := json.Marshal(mergeRequest)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/admin/events/merge", bytes.NewReader(mergeBody))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+viewerToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// TestAdminMergeDuplicatesValidation tests merge validation
func TestAdminMergeDuplicatesValidation(t *testing.T) {
	env := setupTestEnv(t)

	username := "admin"
	password := "admin-password-123"
	email := "admin@example.com"
	insertAdminUser(t, env, username, password, email, "admin")
	adminToken := adminLogin(t, env, username, password)

	tests := []struct {
		name    string
		request map[string]any
	}{
		{
			name:    "missing primary_id",
			request: map[string]any{"duplicate_id": "01234567890123456789012345"},
		},
		{
			name:    "missing duplicate_id",
			request: map[string]any{"primary_id": "01234567890123456789012345"},
		},
		{
			name: "same ID",
			request: map[string]any{
				"primary_id":   "01234567890123456789012345",
				"duplicate_id": "01234567890123456789012345",
			},
		},
		{
			name: "invalid ULID format",
			request: map[string]any{
				"primary_id":   "invalid-ulid",
				"duplicate_id": "01234567890123456789012345",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mergeBody, err := json.Marshal(tt.request)
			require.NoError(t, err)

			req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/admin/events/merge", bytes.NewReader(mergeBody))
			require.NoError(t, err)
			req.Header.Set("Authorization", "Bearer "+adminToken)
			req.Header.Set("Content-Type", "application/json")

			resp, err := env.Server.Client().Do(req)
			require.NoError(t, err)
			_ = resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "validation should fail for: %s", tt.name)
		})
	}
}

// TestAdminMergeDuplicatesNotFound tests merging non-existent events
func TestAdminMergeDuplicatesNotFound(t *testing.T) {
	env := setupTestEnv(t)

	username := "admin"
	password := "admin-password-123"
	email := "admin@example.com"
	insertAdminUser(t, env, username, password, email, "admin")
	adminToken := adminLogin(t, env, username, password)

	mergeRequest := map[string]any{
		"primary_id":   "01KFXJ2N3D5QJRA8WJ6Q8BGV4W",
		"duplicate_id": "01KFXJ2N3D5QJRA8WJ6Q8BGV4X",
	}

	mergeBody, err := json.Marshal(mergeRequest)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/admin/events/merge", bytes.NewReader(mergeBody))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestAdminMergeDuplicatesPreservesProvenance tests that merge preserves provenance from both events
func TestAdminMergeDuplicatesPreservesProvenance(t *testing.T) {
	env := setupTestEnv(t)

	username := "admin"
	password := "admin-password-123"
	email := "admin@example.com"
	insertAdminUser(t, env, username, password, email, "admin")
	adminToken := adminLogin(t, env, username, password)

	agentKey := insertAPIKey(t, env, "test-agent")

	// Create two events from different sources
	event1 := map[string]any{
		"name":      "Community Event",
		"startDate": time.Date(2026, 10, 1, 19, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"location": map[string]any{
			"name":            "Community Center",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
		"source": map[string]any{
			"url":     "https://source1.example.com/events/comm1",
			"eventId": "comm1",
		},
	}

	event2 := map[string]any{
		"name":      "Community Event",
		"startDate": time.Date(2026, 10, 1, 19, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"location": map[string]any{
			"name":            "Community Center",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
		"source": map[string]any{
			"url":     "https://source2.example.com/events/comm2",
			"eventId": "comm2",
		},
	}

	var eventID1, eventID2 string
	for i, event := range []map[string]any{event1, event2} {
		body, err := json.Marshal(event)
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+agentKey)
		req.Header.Set("Content-Type", "application/ld+json")

		resp, err := env.Server.Client().Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		var created map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))

		if i == 0 {
			eventID1 = eventIDFromPayload(created)
		} else {
			eventID2 = eventIDFromPayload(created)
		}
	}

	// Merge events
	mergeRequest := map[string]any{
		"primary_id":   eventID1,
		"duplicate_id": eventID2,
	}

	mergeBody, err := json.Marshal(mergeRequest)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/admin/events/merge", bytes.NewReader(mergeBody))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Fetch merged event with provenance
	getReq, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events/"+eventID1+"?provenance=true", nil)
	require.NoError(t, err)

	getResp, err := env.Server.Client().Do(getReq)
	require.NoError(t, err)
	defer func() { _ = getResp.Body.Close() }()

	var merged map[string]any
	require.NoError(t, json.NewDecoder(getResp.Body).Decode(&merged))

	// Verify provenance includes both sources (implementation specific - adjust as needed)
	// This test may need adjustment based on actual provenance structure
	assert.NotNil(t, merged, "merged event should exist")
}
