package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReviewQueue_HighConfidenceAutoFix tests reversed dates with high confidence correction
// (early morning end, short duration) generates warning with timezone_likely code
func TestReviewQueue_HighConfidenceAutoFix(t *testing.T) {
	env := setupTestEnv(t)
	key := insertAPIKey(t, env, "agent-autocorrect")

	payload := map[string]any{
		"name":      "Late Night Jazz",
		"startDate": "2026-03-31T23:00:00Z", // 11 PM
		"endDate":   "2026-03-31T02:00:00Z", // 2 AM (should be next day!)
		"location": map[string]any{
			"name":            "Jazz Club",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
		"description": "Amazing live jazz",
		"image":       "https://example.com/image.jpg",
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/ld+json")
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	// Read response body for debugging
	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))

	if resp.StatusCode != http.StatusAccepted {
		t.Logf("Unexpected status %d, response: %+v", resp.StatusCode, result)
	}

	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	assert.Equal(t, "pending_review", result["lifecycle_state"])

	// Verify warnings exist
	warnings, ok := result["warnings"].([]any)
	require.True(t, ok, "warnings should be an array")
	require.NotEmpty(t, warnings, "warnings should not be empty")

	// Check warning structure
	warning := warnings[0].(map[string]any)
	assert.Equal(t, "endDate", warning["field"])
	assert.Equal(t, "reversed_dates_timezone_likely", warning["code"])
	assert.Contains(t, warning["message"], "auto-corrected")
}

// TestReviewQueue_LowConfidenceCorrection tests reversed dates with low confidence
// (non-early-morning end or unusual duration) generates needs_review code
func TestReviewQueue_LowConfidenceCorrection(t *testing.T) {
	env := setupTestEnv(t)
	key := insertAPIKey(t, env, "agent-lowconf")

	payload := map[string]any{
		"name":      "Afternoon Event",
		"startDate": "2026-03-31T18:00:00Z", // 6 PM
		"endDate":   "2026-03-31T14:00:00Z", // 2 PM (reversed but afternoon - low confidence)
		"location": map[string]any{
			"name":            "Convention Center",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
		"description": "Conference",
		"image":       "https://example.com/conf.jpg",
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/ld+json")
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("Expected 202 Accepted, got %d. Response: %+v", resp.StatusCode, result)
	}

	assert.Equal(t, "pending_review", result["lifecycle_state"])

	warnings := result["warnings"].([]any)
	require.NotEmpty(t, warnings)

	warning := warnings[0].(map[string]any)
	assert.Equal(t, "endDate", warning["field"])
	assert.Equal(t, "reversed_dates_corrected_needs_review", warning["code"])
}

// TestReviewQueue_OccurrenceReversedDates tests that occurrence-level reversed dates
// also generate warnings (consistent with top-level behavior)
// TODO: This test needs occurrence with venue_id or virtual_url to pass DB constraint
/* DISABLED - needs venue setup
func TestReviewQueue_OccurrenceReversedDates(t *testing.T) {
	env := setupTestEnv(t)
	key := insertAPIKey(t, env, "agent-occurrence")

	payload := map[string]any{
		"name":      "Multi-Day Workshop",
		"startDate": "2026-03-31T23:00:00Z", // Add top-level startDate
		"occurrences": []map[string]any{
			{
				"startDate": "2026-03-31T23:00:00Z", // 11 PM
				"endDate":   "2026-03-31T02:00:00Z", // 2 AM (reversed!)
			},
		},
		"location": map[string]any{
			"name":            "Workshop Space",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
		"description": "Intensive workshop",
		"image":       "https://example.com/workshop.jpg",
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/ld+json")
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("Expected 202 Accepted, got %d. Response: %+v", resp.StatusCode, result)
	}

	assert.Equal(t, "pending_review", result["lifecycle_state"])

	warnings := result["warnings"].([]any)
	require.NotEmpty(t, warnings)

	// Check that warning is for occurrence
	warning := warnings[0].(map[string]any)
	assert.Contains(t, warning["field"], "occurrences[0].endDate")
	assert.Equal(t, "reversed_dates_timezone_likely", warning["code"])
}
*/

// TestReviewQueue_FixedResubmit tests that resubmitting with fixed data auto-approves
func TestReviewQueue_FixedResubmit(t *testing.T) {
	env := setupTestEnv(t)
	key := insertAPIKey(t, env, "agent-fix")

	// First submission with issues
	payload1 := map[string]any{
		"name":      "Jazz Night",
		"startDate": "2026-03-31T23:00:00Z",
		"endDate":   "2026-03-31T02:00:00Z", // Reversed
		"location": map[string]any{
			"name":            "Jazz Club",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
		"source": map[string]any{
			"url":     "https://example.com/jazz",
			"eventId": "jazz-456",
		},
		"description": "Live jazz",
		"image":       "https://example.com/jazz.jpg",
	}

	body1, _ := json.Marshal(payload1)
	req1, _ := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body1))
	req1.Header.Set("Authorization", "Bearer "+key)
	req1.Header.Set("Content-Type", "application/ld+json")
	resp1, err := env.Server.Client().Do(req1)
	require.NoError(t, err)
	defer func() {
		if closeErr := resp1.Body.Close(); closeErr != nil {
			t.Logf("failed to close response body: %v", closeErr)
		}
	}()

	require.Equal(t, http.StatusAccepted, resp1.StatusCode)

	// Second submission with fixed dates
	payload2 := map[string]any{
		"name":      "Jazz Night",
		"startDate": "2026-03-31T23:00:00Z",
		"endDate":   "2026-04-01T02:00:00Z", // Fixed!
		"location": map[string]any{
			"name":            "Jazz Club",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
		"source": map[string]any{
			"url":     "https://example.com/jazz",
			"eventId": "jazz-456",
		},
		"description": "Live jazz",
		"image":       "https://example.com/jazz.jpg",
	}

	body2, _ := json.Marshal(payload2)
	req2, _ := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body2))
	req2.Header.Set("Authorization", "Bearer "+key)
	req2.Header.Set("Content-Type", "application/ld+json")
	resp2, err := env.Server.Client().Do(req2)
	require.NoError(t, err)
	defer func() { _ = resp2.Body.Close() }()

	// Should auto-approve and return 201 Created (no longer needs review)
	var result map[string]any
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&result))

	// Event should be published, not pending
	assert.NotEqual(t, "pending_review", result["lifecycle_state"])
}

// TestReviewQueue_RejectedResubmit tests that resubmitting rejected event with same issues fails
func TestReviewQueue_RejectedResubmit(t *testing.T) {
	env := setupTestEnv(t)
	key := insertAPIKey(t, env, "agent-rejected")

	// Submit event with issues
	payload := map[string]any{
		"name":      "Bad Event",
		"startDate": "2026-03-31T23:00:00Z",
		"endDate":   "2026-03-31T02:00:00Z", // Reversed
		"location": map[string]any{
			"name":            "Venue",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
		"source": map[string]any{
			"url":     "https://example.com/bad",
			"eventId": "bad-123",
		},
		"description": "Event",
		"image":       "https://example.com/img.jpg",
	}

	body, _ := json.Marshal(payload)
	req1, _ := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
	req1.Header.Set("Authorization", "Bearer "+key)
	req1.Header.Set("Content-Type", "application/ld+json")
	resp1, err1 := env.Server.Client().Do(req1)
	require.NoError(t, err1)
	defer func() { _ = resp1.Body.Close() }()

	require.Equal(t, http.StatusAccepted, resp1.StatusCode)

	var created map[string]any
	require.NoError(t, json.NewDecoder(resp1.Body).Decode(&created))
	eventULID := eventIDFromPayload(created)

	// Get the review queue entry ID (need to look up UUID from ULID)
	var reviewID string
	err := env.Pool.QueryRow(env.Context, `
		SELECT erq.id FROM event_review_queue erq
		JOIN events e ON erq.event_id = e.id
		WHERE e.ulid = $1
	`, eventULID).Scan(&reviewID)
	require.NoError(t, err)

	// Admin rejects the event
	_, err = env.Pool.Exec(env.Context, `
		UPDATE event_review_queue
		SET status = 'rejected',
			reviewed_at = NOW(),
			reviewed_by = 'admin@test.com',
			rejection_reason = 'Bad data'
		WHERE id = $1
	`, reviewID)
	require.NoError(t, err)

	// Resubmit with same issues
	req2, _ := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
	req2.Header.Set("Authorization", "Bearer "+key)
	req2.Header.Set("Content-Type", "application/ld+json")
	resp2, err2 := env.Server.Client().Do(req2)
	require.NoError(t, err2)
	defer func() { _ = resp2.Body.Close() }()

	var errResp map[string]any
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&errResp))

	// Should receive error indicating rejection (could be 400 or 409 depending on dedup timing)
	if resp2.StatusCode != http.StatusBadRequest && resp2.StatusCode != http.StatusConflict {
		t.Fatalf("Expected 400 or 409, got %d. Response: %+v", resp2.StatusCode, errResp)
	}

	// Should mention rejection or review (exact wording depends on implementation)
	if errResp["detail"] != nil {
		detail := errResp["detail"].(string)
		assert.True(t,
			strings.Contains(detail, "rejected") || strings.Contains(detail, "review") || strings.Contains(detail, "duplicate"),
			"Expected error detail to mention rejection/review/duplicate, got: %s", detail)
	}
}

// TestReviewQueue_NormalEvent tests that events without issues are created normally
func TestReviewQueue_NormalEvent(t *testing.T) {
	env := setupTestEnv(t)
	key := insertAPIKey(t, env, "agent-normal")

	payload := map[string]any{
		"name":      "Normal Event",
		"startDate": "2026-03-31T19:00:00Z",
		"endDate":   "2026-03-31T22:00:00Z", // Correct order
		"location": map[string]any{
			"name":            "Venue",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
		"description": "Regular event",
		"image":       "https://example.com/event.jpg",
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/ld+json")
	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	// Should receive 201 Created (NOT 202 Accepted)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))

	// Should NOT have warnings or pending_review state
	assert.NotEqual(t, "pending_review", result["lifecycle_state"])
	assert.Nil(t, result["warnings"])
}
