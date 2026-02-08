package contracts_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// setupAdminEnv creates a test environment with an admin user and returns the JWT token
func setupAdminEnv(t *testing.T) (*testEnv, string) {
	t.Helper()
	env := setupTestEnv(t)
	insertAdminUser(t, env, "admin", "password", "admin@test.com", "admin")
	token := adminLogin(t, env, "admin", "password")
	return env, token
}

// TestReviewQueueListResponseStructure verifies the list endpoint returns valid JSON envelope
func TestReviewQueueListResponseStructure(t *testing.T) {
	env, token := setupAdminEnv(t)

	// Create test data
	org := insertOrganization(t, env, "Toronto Arts Org")
	place := insertPlace(t, env, "Centennial Park", "Toronto")
	eventULID1 := insertEventWithOccurrence(t, env, "Jazz in the Park", org.ID, place.ID, "music", "pending_review", []string{"jazz"}, time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC))
	eventULID2 := insertEventWithOccurrence(t, env, "Summer Arts Expo", org.ID, place.ID, "arts", "pending_review", []string{"summer"}, time.Date(2026, 7, 11, 19, 0, 0, 0, time.UTC))

	// Get event IDs for review queue entries
	var eventID1, eventID2 string
	err := env.Pool.QueryRow(env.Context, `SELECT id FROM events WHERE ulid = $1`, eventULID1).Scan(&eventID1)
	require.NoError(t, err)
	err = env.Pool.QueryRow(env.Context, `SELECT id FROM events WHERE ulid = $1`, eventULID2).Scan(&eventID2)
	require.NoError(t, err)

	// Insert review queue entries
	insertReviewQueueEntry(t, env, eventID1, `{"name":"Jazz in the Park"}`, `{"name":"Jazz in the Park"}`, `[]`, time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC))
	insertReviewQueueEntry(t, env, eventID2, `{"name":"Summer Arts Expo"}`, `{"name":"Summer Arts Expo"}`, `[]`, time.Date(2026, 7, 11, 19, 0, 0, 0, time.UTC))

	// Test list endpoint
	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/admin/api/review-queue", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	// Verify status code
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.True(t, strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json"))

	// Verify response structure
	var payload map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))

	// Verify required fields in JSON envelope
	require.Contains(t, payload, "items")
	require.Contains(t, payload, "next_cursor")

	// Verify items is an array
	items, ok := payload["items"].([]any)
	require.True(t, ok, "items should be an array")
	require.GreaterOrEqual(t, len(items), 2, "should have at least 2 items")

	// Verify next_cursor is a string (even if empty)
	_, ok = payload["next_cursor"].(string)
	require.True(t, ok, "next_cursor should be a string")
}

// TestReviewQueueListItemFields verifies each item has required fields with correct types
func TestReviewQueueListItemFields(t *testing.T) {
	env, token := setupAdminEnv(t)

	org := insertOrganization(t, env, "Toronto Arts Org")
	place := insertPlace(t, env, "Centennial Park", "Toronto")
	eventULID := insertEventWithOccurrence(t, env, "Jazz in the Park", org.ID, place.ID, "music", "pending_review", []string{"jazz"}, time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC))

	var eventID string
	err := env.Pool.QueryRow(env.Context, `SELECT id FROM events WHERE ulid = $1`, eventULID).Scan(&eventID)
	require.NoError(t, err)

	insertReviewQueueEntry(t, env, eventID, `{"name":"Jazz in the Park"}`, `{"name":"Jazz in the Park"}`, `[{"severity":"warning","message":"Test warning"}]`, time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC))

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/admin/api/review-queue", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var payload map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))

	items := payload["items"].([]any)
	require.NotEmpty(t, items)

	// Verify first item structure
	item := items[0].(map[string]any)

	// Required fields
	require.Contains(t, item, "id")
	require.Contains(t, item, "eventId")
	require.Contains(t, item, "status")
	require.Contains(t, item, "warnings")
	require.Contains(t, item, "createdAt")

	// Verify field types
	_, ok := item["id"].(float64) // JSON numbers are float64
	require.True(t, ok, "id should be a number")

	eventIDField, ok := item["eventId"].(string)
	require.True(t, ok, "eventId should be a string")
	require.NotEmpty(t, eventIDField)

	status, ok := item["status"].(string)
	require.True(t, ok, "status should be a string")
	require.Equal(t, "pending", status)

	warnings, ok := item["warnings"].([]any)
	require.True(t, ok, "warnings should be an array")
	require.NotEmpty(t, warnings)

	_, ok = item["createdAt"].(string)
	require.True(t, ok, "createdAt should be a string (ISO 8601)")
}

// Helper function to insert review queue entries
func insertReviewQueueEntry(t *testing.T, env *testEnv, eventID string, originalPayload string, normalizedPayload string, warnings string, startTime time.Time) string {
	t.Helper()

	var id int
	err := env.Pool.QueryRow(env.Context,
		`INSERT INTO event_review_queue (event_id, original_payload, normalized_payload, warnings, event_start_time, status)
		 VALUES ($1, $2, $3, $4, $5, 'pending')
		 RETURNING id`,
		eventID, originalPayload, normalizedPayload, warnings, startTime,
	).Scan(&id)
	require.NoError(t, err)

	return strconv.Itoa(id)
}
