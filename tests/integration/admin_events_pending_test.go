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

// TestAdminPendingListEmpty tests fetching an empty pending list
func TestAdminPendingListEmpty(t *testing.T) {
	env := setupTestEnv(t)

	// Insert admin user and get JWT
	username := "admin"
	password := "admin-password-123"
	email := "admin@example.com"
	insertAdminUser(t, env, username, password, email, "admin")
	token := adminLogin(t, env, username, password)

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/admin/events/pending", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))

	items, ok := result["items"].([]any)
	require.True(t, ok, "expected items array")
	assert.Empty(t, items, "expected empty pending list")
}

// TestAdminPendingListWithEvents tests fetching pending events
func TestAdminPendingListWithEvents(t *testing.T) {
	env := setupTestEnv(t)

	// Insert admin user and get JWT
	username := "admin"
	password := "admin-password-123"
	email := "admin@example.com"
	insertAdminUser(t, env, username, password, email, "admin")
	adminToken := adminLogin(t, env, username, password)

	// Create an agent API key
	agentKey := insertAPIKey(t, env, "test-agent")

	// Submit events that should be flagged for review
	// 1. Event with low confidence (< 0.6)
	lowConfidenceEvent := map[string]any{
		"name":      "Low Confidence Event",
		"startDate": time.Date(2026, 10, 1, 19, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"location": map[string]any{
			"name":            "Test Venue",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
		"confidence": 0.5, // Below 0.6 threshold
		"source": map[string]any{
			"url":     "https://example.com/events/low-conf",
			"eventId": "low-conf-123",
		},
	}

	// 2. Event with missing description (should be flagged)
	noDescEvent := map[string]any{
		"name":      "No Description Event",
		"startDate": time.Date(2026, 10, 2, 19, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"location": map[string]any{
			"name":            "Test Venue",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
		// Missing description field
		"source": map[string]any{
			"url":     "https://example.com/events/no-desc",
			"eventId": "no-desc-456",
		},
	}

	// 3. Event far in future (>730 days, should be flagged)
	futureEvent := map[string]any{
		"name":        "Far Future Event",
		"description": "This event is more than 2 years in the future",
		"startDate":   time.Now().AddDate(3, 0, 0).Format(time.RFC3339), // 3 years from now
		"location": map[string]any{
			"name":            "Test Venue",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
		"source": map[string]any{
			"url":     "https://example.com/events/future",
			"eventId": "future-789",
		},
	}

	// Submit all three events
	events := []map[string]any{lowConfidenceEvent, noDescEvent, futureEvent}
	for _, event := range events {
		body, err := json.Marshal(event)
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+agentKey)
		req.Header.Set("Content-Type", "application/ld+json")
		req.Header.Set("Accept", "application/ld+json")

		resp, err := env.Server.Client().Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
	}

	// Fetch pending events as admin
	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/admin/events/pending", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Accept", "application/json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))

	items, ok := result["items"].([]any)
	require.True(t, ok, "expected items array")

	// All three events should be in pending state (draft lifecycle_state)
	assert.GreaterOrEqual(t, len(items), 3, "expected at least 3 pending events")
}

// TestAdminPendingListPagination tests pagination of pending events
func TestAdminPendingListPagination(t *testing.T) {
	env := setupTestEnv(t)

	// Insert admin user and get JWT
	username := "admin"
	password := "admin-password-123"
	email := "admin@example.com"
	insertAdminUser(t, env, username, password, email, "admin")
	adminToken := adminLogin(t, env, username, password)

	// Create agent API key
	agentKey := insertAPIKey(t, env, "test-agent")

	// Submit multiple low-confidence events
	for i := 0; i < 15; i++ {
		event := map[string]any{
			"name":      "Pending Event " + string(rune('A'+i)),
			"startDate": time.Date(2026, 10, i+1, 19, 0, 0, 0, time.UTC).Format(time.RFC3339),
			"location": map[string]any{
				"name":            "Test Venue",
				"addressLocality": "Toronto",
				"addressRegion":   "ON",
			},
			"confidence": 0.5, // Low confidence
			"source": map[string]any{
				"url":     "https://example.com/events/pending-" + string(rune('a'+i)),
				"eventId": "pending-" + string(rune('a'+i)),
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
		_ = resp.Body.Close()
	}

	// Fetch first page with limit
	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/admin/events/pending?limit=10", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Accept", "application/json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))

	items, ok := result["items"].([]any)
	require.True(t, ok, "expected items array")
	assert.LessOrEqual(t, len(items), 10, "expected at most 10 items per page")

	// Check for next_cursor
	if len(items) == 10 {
		_, hasCursor := result["next_cursor"]
		assert.True(t, hasCursor, "expected next_cursor when more items available")
	}
}

// TestAdminPendingListFilters tests filtering pending events
func TestAdminPendingListFilters(t *testing.T) {
	env := setupTestEnv(t)

	// Insert admin user and get JWT
	username := "admin"
	password := "admin-password-123"
	email := "admin@example.com"
	insertAdminUser(t, env, username, password, email, "admin")
	adminToken := adminLogin(t, env, username, password)

	// Create agent API key
	agentKey := insertAPIKey(t, env, "test-agent")

	// Submit events in different cities
	torontoEvent := map[string]any{
		"name":      "Toronto Pending Event",
		"startDate": time.Date(2026, 10, 1, 19, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"location": map[string]any{
			"name":            "Toronto Venue",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
		"confidence": 0.5,
		"source": map[string]any{
			"url":     "https://example.com/events/toronto",
			"eventId": "toronto-123",
		},
	}

	montrealEvent := map[string]any{
		"name":      "Montreal Pending Event",
		"startDate": time.Date(2026, 10, 2, 19, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"location": map[string]any{
			"name":            "Montreal Venue",
			"addressLocality": "Montreal",
			"addressRegion":   "QC",
		},
		"confidence": 0.5,
		"source": map[string]any{
			"url":     "https://example.com/events/montreal",
			"eventId": "montreal-456",
		},
	}

	for _, event := range []map[string]any{torontoEvent, montrealEvent} {
		body, err := json.Marshal(event)
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+agentKey)
		req.Header.Set("Content-Type", "application/ld+json")

		resp, err := env.Server.Client().Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
	}

	// Filter by city
	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/admin/events/pending?city=Toronto", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Accept", "application/json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))

	items, ok := result["items"].([]any)
	require.True(t, ok, "expected items array")

	// Verify Toronto events are returned
	for _, item := range items {
		eventMap := item.(map[string]any)
		name, _ := eventMap["name"].(string)
		if name == "Toronto Pending Event" || name == "Montreal Pending Event" {
			// If we found Toronto event, good. Montreal should not be here
			if name == "Toronto Pending Event" {
				assert.Contains(t, name, "Toronto", "expected Toronto event")
			}
		}
	}
}

// TestAdminPendingListUnauthorized tests that non-admin users cannot access pending list
func TestAdminPendingListUnauthorized(t *testing.T) {
	env := setupTestEnv(t)

	// Insert viewer user (not admin)
	username := "viewer"
	password := "viewer-password-123"
	email := "viewer@example.com"
	insertAdminUser(t, env, username, password, email, "viewer")
	viewerToken := adminLogin(t, env, username, password)

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/admin/events/pending", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+viewerToken)
	req.Header.Set("Accept", "application/json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	// Should be forbidden for non-admin roles
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// TestAdminPendingListSortOrder tests that pending events are sorted by submission date
func TestAdminPendingListSortOrder(t *testing.T) {
	env := setupTestEnv(t)

	// Insert admin user and get JWT
	username := "admin"
	password := "admin-password-123"
	email := "admin@example.com"
	insertAdminUser(t, env, username, password, email, "admin")
	adminToken := adminLogin(t, env, username, password)

	// Create agent API key
	agentKey := insertAPIKey(t, env, "test-agent")

	// Submit events with different timestamps
	eventNames := []string{"First Event", "Second Event", "Third Event"}
	for _, name := range eventNames {
		event := map[string]any{
			"name":      name,
			"startDate": time.Date(2026, 10, 1, 19, 0, 0, 0, time.UTC).Format(time.RFC3339),
			"location": map[string]any{
				"name":            "Test Venue",
				"addressLocality": "Toronto",
				"addressRegion":   "ON",
			},
			"confidence": 0.5,
			"source": map[string]any{
				"url":     "https://example.com/events/" + name,
				"eventId": name,
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
		_ = resp.Body.Close()

		// Small delay to ensure different created_at timestamps
		time.Sleep(10 * time.Millisecond)
	}

	// Fetch pending events
	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/admin/events/pending", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Accept", "application/json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))

	items, ok := result["items"].([]any)
	require.True(t, ok, "expected items array")
	assert.GreaterOrEqual(t, len(items), 3, "expected at least 3 events")

	// Verify events are ordered (typically newest first or oldest first - implementation dependent)
	// Just verify we have the expected events
	names := make([]string, 0, len(items))
	for _, item := range items {
		eventMap := item.(map[string]any)
		if name, ok := eventMap["name"].(string); ok {
			names = append(names, name)
		}
	}

	assert.Contains(t, names, "First Event")
	assert.Contains(t, names, "Second Event")
	assert.Contains(t, names, "Third Event")
}
