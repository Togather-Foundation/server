package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestEvent creates an event via the agent API and returns the parsed
// response payload. It calls t.Fatalf on any HTTP error or unexpected status.
func createTestEvent(t *testing.T, env *testEnv, agentKey string, name, sourceURL string) map[string]any {
	t.Helper()

	startDate := time.Now().Add(365 * 24 * time.Hour).UTC().Format(time.RFC3339)

	payload := map[string]any{
		"name":      name,
		"startDate": startDate,
		"location": map[string]any{
			"name":            "Test Venue",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
		"source": map[string]any{
			"url":     sourceURL,
			"eventId": fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		},
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+agentKey)
	req.Header.Set("Content-Type", "application/ld+json")
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	var created map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
		t.Fatalf("createTestEvent: unexpected status %d for %q: %v", resp.StatusCode, name, created)
	}

	return created
}

// postConsolidate POSTs to the consolidate endpoint and returns status + decoded body.
func postConsolidate(t *testing.T, env *testEnv, adminToken string, payload any) (int, map[string]any) {
	t.Helper()

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/admin/events/consolidate", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))

	return resp.StatusCode, result
}

// getEvent makes a GET /api/v1/events/{ulid} and returns status + body.
func getEvent(t *testing.T, env *testEnv, ulid string) (int, map[string]any) {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events/"+ulid, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))

	return resp.StatusCode, result
}

// retiredList extracts the "retired" array from a consolidate response as []string.
func retiredList(t *testing.T, resp map[string]any) []string {
	t.Helper()
	raw, ok := resp["retired"]
	require.True(t, ok, "response missing 'retired' field")
	arr, ok := raw.([]any)
	require.True(t, ok, "'retired' field is not an array")
	out := make([]string, len(arr))
	for i, v := range arr {
		s, ok := v.(string)
		require.True(t, ok, "retired[%d] is not a string", i)
		out[i] = s
	}
	return out
}

// ---------------------------------------------------------------------------
// Test 1: Create path – new canonical + retire two existing events
// ---------------------------------------------------------------------------

func TestAdminConsolidateCreate_Success(t *testing.T) {
	env := setupTestEnv(t)

	insertAdminUser(t, env, "admin", "admin-password-123", "admin@togather.test", "admin")
	adminToken := adminLogin(t, env, "admin", "admin-password-123")
	agentKey := insertAPIKey(t, env, "agent-consolidate-create")

	ev1 := createTestEvent(t, env, agentKey, "Retire Me Alpha", "https://music.toronto.ca/events/consolidate-create-a")
	ev2 := createTestEvent(t, env, agentKey, "Retire Me Beta", "https://music.toronto.ca/events/consolidate-create-b")

	ulid1 := eventIDFromPayload(ev1)
	ulid2 := eventIDFromPayload(ev2)
	require.NotEmpty(t, ulid1, "event 1 ULID is empty")
	require.NotEmpty(t, ulid2, "event 2 ULID is empty")

	// Use a start date within MaxFutureDays (730 days) and provide a description
	// to ensure confidence stays >= 0.6 and no quality warnings are generated,
	// which avoids triggering the review queue (and the consolidateResolvePending path).
	canonicalStart := time.Now().Add(180 * 24 * time.Hour).UTC().Format(time.RFC3339)

	status, body := postConsolidate(t, env, adminToken, map[string]any{
		"event": map[string]any{
			"name":        "Canonical Consolidated Event",
			"startDate":   canonicalStart,
			"description": "A well-described canonical event created during consolidation testing.",
			"location": map[string]any{
				"name":            "Main Venue",
				"addressLocality": "Toronto",
				"addressRegion":   "ON",
			},
		},
		"retire": []string{ulid1, ulid2},
	})

	require.Equal(t, http.StatusOK, status, "expected 200 OK; body: %v", body)

	// Event field with @id and name
	eventField, ok := body["event"].(map[string]any)
	require.True(t, ok, "response 'event' field is missing or not an object")
	assert.NotEmpty(t, eventField["@id"], "event @id should be set")
	assert.Equal(t, "Canonical Consolidated Event", eventField["name"])

	// lifecycle_state
	assert.Equal(t, "published", body["lifecycle_state"])

	// retired list contains both ULIDs
	retired := retiredList(t, body)
	assert.ElementsMatch(t, []string{ulid1, ulid2}, retired, "retired list mismatch")

	// review_entries_dismissed present and is an array
	_, hasDismissed := body["review_entries_dismissed"]
	assert.True(t, hasDismissed, "response missing review_entries_dismissed")

	// canonicalULID from the returned event @id
	canonicalULID := eventIDFromPayload(eventField)
	require.NotEmpty(t, canonicalULID, "could not extract canonical ULID from event @id")

	// Retired events should return 410 Gone
	for _, retiredULID := range []string{ulid1, ulid2} {
		retStatus, _ := getEvent(t, env, retiredULID)
		assert.Equal(t, http.StatusGone, retStatus, "retired event %s should return 410 Gone", retiredULID)
	}

	// Canonical event should return 200 OK
	canonStatus, canonBody := getEvent(t, env, canonicalULID)
	assert.Equal(t, http.StatusOK, canonStatus, "canonical event should return 200 OK; body: %v", canonBody)
}

// ---------------------------------------------------------------------------
// Test 2: Promote path – promote existing event as canonical, retire another
// ---------------------------------------------------------------------------

func TestAdminConsolidatePromote_Success(t *testing.T) {
	env := setupTestEnv(t)

	insertAdminUser(t, env, "admin", "admin-password-123", "admin@togather.test", "admin")
	adminToken := adminLogin(t, env, "admin", "admin-password-123")
	agentKey := insertAPIKey(t, env, "agent-consolidate-promote")

	ev1 := createTestEvent(t, env, agentKey, "Promote Me As Canonical", "https://events.sel.foundation/test-promote-a")
	ev2 := createTestEvent(t, env, agentKey, "Retire Me On Promote", "https://events.sel.foundation/test-promote-b")

	ulid1 := eventIDFromPayload(ev1)
	ulid2 := eventIDFromPayload(ev2)
	require.NotEmpty(t, ulid1)
	require.NotEmpty(t, ulid2)

	status, body := postConsolidate(t, env, adminToken, map[string]any{
		"event_ulid": ulid1,
		"retire":     []string{ulid2},
	})

	require.Equal(t, http.StatusOK, status, "expected 200 OK; body: %v", body)

	// event @id should point to ulid1 (the promoted canonical)
	eventField, ok := body["event"].(map[string]any)
	require.True(t, ok, "response 'event' field is missing or not an object")
	canonicalULID := eventIDFromPayload(eventField)
	assert.Equal(t, ulid1, canonicalULID, "canonical @id should be the promoted event ULID")

	// retired list should contain ulid2
	retired := retiredList(t, body)
	assert.Contains(t, retired, ulid2, "retired list should include second event ULID")
	assert.NotContains(t, retired, ulid1, "canonical ULID should not appear in retired list")

	// Retired event should return 410 Gone
	retStatus, _ := getEvent(t, env, ulid2)
	assert.Equal(t, http.StatusGone, retStatus, "retired event should return 410 Gone")

	// Promoted canonical event should still return 200 OK
	canonStatus, canonBody := getEvent(t, env, ulid1)
	assert.Equal(t, http.StatusOK, canonStatus, "promoted canonical event should return 200 OK; body: %v", canonBody)
}

// ---------------------------------------------------------------------------
// Test 3: review_entries_dismissed field is present (structure validation)
// ---------------------------------------------------------------------------

func TestAdminConsolidateDismissesReviews(t *testing.T) {
	env := setupTestEnv(t)

	insertAdminUser(t, env, "admin", "admin-password-123", "admin@togather.test", "admin")
	adminToken := adminLogin(t, env, "admin", "admin-password-123")
	agentKey := insertAPIKey(t, env, "agent-consolidate-review")

	ev1 := createTestEvent(t, env, agentKey, "Review Dismiss Target", "https://music.toronto.ca/events/review-dismiss-a")
	ev2 := createTestEvent(t, env, agentKey, "Review Dismiss Canonical", "https://music.toronto.ca/events/review-dismiss-b")

	ulid1 := eventIDFromPayload(ev1)
	ulid2 := eventIDFromPayload(ev2)
	require.NotEmpty(t, ulid1)
	require.NotEmpty(t, ulid2)

	status, body := postConsolidate(t, env, adminToken, map[string]any{
		"event_ulid": ulid2,
		"retire":     []string{ulid1},
	})

	require.Equal(t, http.StatusOK, status, "expected 200 OK; body: %v", body)

	// review_entries_dismissed must be present as an array (may be empty for normal events)
	dismissed, ok := body["review_entries_dismissed"]
	require.True(t, ok, "response missing 'review_entries_dismissed' field")
	_, isArr := dismissed.([]any)
	assert.True(t, isArr, "review_entries_dismissed should be an array, got: %T", dismissed)
}

// ---------------------------------------------------------------------------
// Test 4: Unauthorized access
// ---------------------------------------------------------------------------

func TestAdminConsolidate_Unauthorized(t *testing.T) {
	env := setupTestEnv(t)

	agentKey := insertAPIKey(t, env, "agent-consolidate-unauth")

	payload := map[string]any{
		"event_ulid": "01HTEST0000000000000000001",
		"retire":     []string{"01HTEST0000000000000000002"},
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	// --- No auth header → 401 ---
	req1, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/admin/events/consolidate", bytes.NewReader(body))
	require.NoError(t, err)
	req1.Header.Set("Content-Type", "application/json")

	resp1, err := env.Server.Client().Do(req1)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp1.Body.Close() })
	assert.Equal(t, http.StatusUnauthorized, resp1.StatusCode, "expected 401 with no auth")

	// --- Agent API key (not a JWT admin token) → 401 ---
	body2, err := json.Marshal(payload)
	require.NoError(t, err)
	req2, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/admin/events/consolidate", bytes.NewReader(body2))
	require.NoError(t, err)
	req2.Header.Set("Authorization", "Bearer "+agentKey)
	req2.Header.Set("Content-Type", "application/json")

	resp2, err := env.Server.Client().Do(req2)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp2.Body.Close() })
	// Agent API keys are not accepted on admin JWT-guarded endpoints
	assert.True(t,
		resp2.StatusCode == http.StatusUnauthorized || resp2.StatusCode == http.StatusForbidden,
		"expected 401 or 403 for agent key on admin endpoint, got %d", resp2.StatusCode)
}

// ---------------------------------------------------------------------------
// Test 5: Validation errors
// ---------------------------------------------------------------------------

func TestAdminConsolidate_ValidationErrors(t *testing.T) {
	env := setupTestEnv(t)

	insertAdminUser(t, env, "admin", "admin-password-123", "admin@togather.test", "admin")
	adminToken := adminLogin(t, env, "admin", "admin-password-123")

	type testCase struct {
		name    string
		payload map[string]any
	}

	cases := []testCase{
		{
			name: "both_event_and_event_ulid",
			payload: map[string]any{
				"event":      map[string]any{"name": "My Event"},
				"event_ulid": "01HTEST0000000000000000001",
				"retire":     []string{"01HTEST0000000000000000002"},
			},
		},
		{
			name: "neither_event_nor_event_ulid",
			payload: map[string]any{
				"retire": []string{"01HTEST0000000000000000002"},
			},
		},
		{
			name: "retire_is_empty",
			payload: map[string]any{
				"event_ulid": "01HTEST0000000000000000001",
				"retire":     []string{},
			},
		},
		{
			name: "canonical_ulid_also_in_retire",
			payload: map[string]any{
				"event_ulid": "01HTEST0000000000000000001",
				"retire":     []string{"01HTEST0000000000000000001"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := postConsolidate(t, env, adminToken, tc.payload)
			assert.Equal(t, http.StatusBadRequest, status,
				"case %q: expected 400 Bad Request; body: %v", tc.name, body)
		})
	}
}
