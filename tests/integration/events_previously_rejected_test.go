package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestCreateEventPreviouslyRejected tests that resubmitting a previously rejected event
// returns HTTP 400 with proper RFC 7807 problem details.
//
// NOTE: This test will pass once srv-xmq (review queue implementation) is complete.
// Until then, this test serves as documentation of the expected behavior.
//
// Scenario:
// 1. Submit event with data quality issues (e.g., reversed dates)
// 2. Admin reviews and rejects the event
// 3. Source resubmits the SAME event with SAME issues
// 4. Should receive HTTP 400 with previously-rejected error
func TestCreateEventPreviouslyRejected(t *testing.T) {
	t.Skip("Skipping until srv-xmq (review queue implementation) is complete")

	env := setupTestEnv(t)
	key := insertAPIKey(t, env, "agent-rejected")

	// Event with reversed dates (common data quality issue)
	payload := map[string]any{
		"name":      "Late Night Jazz",
		"startDate": time.Date(2026, 3, 31, 23, 0, 0, 0, time.UTC).Format(time.RFC3339), // 11 PM
		"endDate":   time.Date(2026, 3, 31, 2, 0, 0, 0, time.UTC).Format(time.RFC3339),  // 2 AM (should be next day!)
		"location": map[string]any{
			"name":            "Jazz Club",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
		"source": map[string]any{
			"url":     "https://example.com/events/jazz",
			"eventId": "jazz-123",
		},
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	// First submission - should be accepted for review
	req1, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
	require.NoError(t, err)
	req1.Header.Set("Authorization", "Bearer "+key)
	req1.Header.Set("Content-Type", "application/ld+json")
	req1.Header.Set("Accept", "application/ld+json")

	resp1, err := env.Server.Client().Do(req1)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp1.Body.Close() })

	// Should receive 202 Accepted with warnings
	require.Equal(t, http.StatusAccepted, resp1.StatusCode)

	var created map[string]any
	require.NoError(t, json.NewDecoder(resp1.Body).Decode(&created))
	eventID := eventIDFromPayload(created)
	require.NotEmpty(t, eventID)

	// Admin rejects the event (this would be done via admin API)
	// For test purposes, we'll directly insert a rejection record
	reviewedAt := time.Now().UTC()
	reviewedBy := "admin@togather.ca"
	rejectionReason := "Cannot determine correct time. Please fix the data before resubmitting."

	// TODO: Once srv-xmq is complete, use proper admin API to reject:
	// rejectEvent(t, env, eventID, rejectionReason, reviewedBy)

	// Direct DB insertion for test (temporary until admin API exists)
	_, err = env.Pool.Exec(env.Context, `
		UPDATE event_review_queue 
		SET status = 'rejected', 
		    reviewed_at = $1, 
		    reviewed_by = $2,
		    rejection_reason = $3
		WHERE event_id = $4
	`, reviewedAt, reviewedBy, rejectionReason, eventID)
	require.NoError(t, err)

	// Second submission - same event, same issues
	req2, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
	require.NoError(t, err)
	req2.Header.Set("Authorization", "Bearer "+key)
	req2.Header.Set("Content-Type", "application/ld+json")
	req2.Header.Set("Accept", "application/ld+json")

	resp2, err := env.Server.Client().Do(req2)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp2.Body.Close() })

	// Should receive 400 Bad Request
	require.Equal(t, http.StatusBadRequest, resp2.StatusCode)
	require.True(t, strings.HasPrefix(resp2.Header.Get("Content-Type"), "application/problem+json"))

	// Parse problem details
	var problem map[string]any
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&problem))

	// Verify RFC 7807 problem details structure
	require.Equal(t, "https://sel.events/problems/previously-rejected", problem["type"])
	require.Equal(t, "Previously Rejected", problem["title"])
	require.Equal(t, float64(400), problem["status"])

	// Verify detail message includes review information
	detail, ok := problem["detail"].(string)
	require.True(t, ok)
	require.Contains(t, detail, "reviewed")
	require.Contains(t, detail, "rejected")
	require.Contains(t, detail, rejectionReason)
	require.Contains(t, detail, reviewedAt.Format(time.RFC3339))
}

// TestCreateEventRejectedThenFixed tests that after rejection, fixing the issues
// allows the event to be successfully created.
func TestCreateEventRejectedThenFixed(t *testing.T) {
	t.Skip("Skipping until srv-xmq (review queue implementation) is complete")

	env := setupTestEnv(t)
	key := insertAPIKey(t, env, "agent-fixed")

	// Submit, get rejected (same as previous test)
	// Original event would have reversed dates:
	// startDate: 2026-03-31T23:00:00Z
	// endDate:   2026-03-31T02:00:00Z
	// ... rejection logic ...

	// Now submit with FIXED dates
	fixedPayload := map[string]any{
		"name":      "Late Night Show",
		"startDate": time.Date(2026, 3, 31, 23, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"endDate":   time.Date(2026, 4, 1, 2, 0, 0, 0, time.UTC).Format(time.RFC3339), // Next day!
		"location": map[string]any{
			"name":            "Comedy Club",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
		"source": map[string]any{
			"url":     "https://example.com/events/comedy",
			"eventId": "comedy-456",
		},
	}

	body, err := json.Marshal(fixedPayload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/ld+json")
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	// Should succeed with 201 Created (no warnings now)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var created map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	require.Equal(t, "Late Night Show", eventNameFromPayload(created))
}
