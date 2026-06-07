package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// insertPendingReviewDirect creates a pending review entry directly via SQL.
// Returns the review row ID. It deletes any existing review entry for the event first.
func insertPendingReviewDirect(t *testing.T, env *testEnv, eventULID string, warningsJSON []byte, duplicateOfEventULID *string) int {
	t.Helper()

	var internalID string
	err := env.Pool.QueryRow(env.Context, `SELECT id FROM events WHERE ulid = $1`, eventULID).Scan(&internalID)
	require.NoError(t, err, "get internal event ID for ULID %s", eventULID)

	// Remove any existing review entry for this event (e.g. auto-created by near-dup detection).
	_, _ = env.Pool.Exec(env.Context, `DELETE FROM event_review_queue WHERE event_id = $1`, internalID)

	payloadJSON := json.RawMessage(`{"name":"test event"}`)

	var reviewID int
	var dupOfID *string
	if duplicateOfEventULID != nil {
		var dupInternal string
		if err := env.Pool.QueryRow(env.Context, `SELECT id FROM events WHERE ulid = $1`, *duplicateOfEventULID).Scan(&dupInternal); err == nil {
			dupOfID = &dupInternal
		}
	}

	err = env.Pool.QueryRow(env.Context, `
		INSERT INTO event_review_queue
			(event_id, original_payload, normalized_payload, warnings, event_start_time, duplicate_of_event_id)
		VALUES ($1, $2, $2, $3, NOW() + INTERVAL '30 days', $4)
		RETURNING id
	`, internalID, payloadJSON, warningsJSON, dupOfID).Scan(&reviewID)
	require.NoError(t, err, "insert pending review for event %s", eventULID)

	return reviewID
}

// setEventLifecycle sets lifecycle_state directly via SQL.
func setEventLifecycle(t *testing.T, env *testEnv, eventULID, state string) {
	t.Helper()
	_, err := env.Pool.Exec(env.Context, `UPDATE events SET lifecycle_state = $2 WHERE ulid = $1`, eventULID, state)
	require.NoError(t, err, "set lifecycle_state for %s to %s", eventULID, state)
}

// getEventLifecycle returns the lifecycle_state for an event.
func getEventLifecycle(t *testing.T, env *testEnv, eventULID string) string {
	t.Helper()
	var state string
	err := env.Pool.QueryRow(env.Context, `SELECT lifecycle_state FROM events WHERE ulid = $1`, eventULID).Scan(&state)
	require.NoError(t, err, "get lifecycle_state for %s", eventULID)
	return state
}

// getReviewStatus returns the status of a review entry by ID.
func getReviewStatus(t *testing.T, env *testEnv, reviewID int) string {
	t.Helper()
	var status string
	err := env.Pool.QueryRow(env.Context, `SELECT status FROM event_review_queue WHERE id = $1`, reviewID).Scan(&status)
	require.NoError(t, err, "get review status for id %d", reviewID)
	return status
}

// countPendingReviewsForEvent returns the count of pending reviews for an event ULID.
func countPendingReviewsForEvent(t *testing.T, env *testEnv, eventULID string) int {
	t.Helper()
	var count int
	err := env.Pool.QueryRow(env.Context, `
		SELECT COUNT(*) FROM event_review_queue q
		JOIN events e ON q.event_id = e.id
		WHERE e.ulid = $1 AND q.status = 'pending'
	`, eventULID).Scan(&count)
	require.NoError(t, err, "count pending reviews for %s", eventULID)
	return count
}

// makePotentialDuplicateWarning constructs a potential_duplicate warning JSON
// referencing the given event ULID.
func makePotentialDuplicateWarning(targetULID string) []byte {
	warnings := []map[string]any{{
		"field":   "name",
		"message": "Potential duplicate event",
		"code":    "potential_duplicate",
		"details": map[string]any{
			"matches": []map[string]any{{
				"ulid":       targetULID,
				"name":       "Retired Event",
				"similarity": 0.85,
			}},
		},
	}}
	b, _ := json.Marshal(warnings)
	return b
}

// makeDualWarnings constructs a JSON with both a potential_duplicate and a reversed_dates warning.
func makeDualWarnings(targetULID string) []byte {
	warnings := []map[string]any{
		{
			"field":   "name",
			"message": "Potential duplicate event",
			"code":    "potential_duplicate",
			"details": map[string]any{
				"matches": []map[string]any{{
					"ulid":       targetULID,
					"name":       "Retired Event",
					"similarity": 0.85,
				}},
			},
		},
		{
			"field":   "endDate",
			"message": "End date appears to be before start date. Auto-corrected by adding 24 hours.",
			"code":    "reversed_dates_corrected_needs_review",
		},
	}
	b, _ := json.Marshal(warnings)
	return b
}

// ---------------------------------------------------------------------------
// Test 1: AddOccurrence recompute — event goes published
// This test verifies that after consolidating (add-occurrence equivalent) a
// source event into a canonical that has a single pending review pointing to
// the retired event, the canonical's lifecycle is recomputed to "published"
// because all pending reviews are resolved.
// ---------------------------------------------------------------------------
func TestRecomputeAddOccurrenceGoesPublished(t *testing.T) {
	env := setupTestEnv(t)

	insertAdminUser(t, env, "admin", "admin-password-123", "admin@togather.test", "admin")
	adminToken := adminLogin(t, env, "admin", "admin-password-123")
	agentKey := insertAPIKey(t, env, "agent-recompute-addocc-pub")

	// Create two events with different source URLs so they don't collide as duplicates at ingest.
	canon := createTestEvent(t, env, agentKey,
		"Recompute Published Canonical",
		fmt.Sprintf("https://music.toronto.ca/events/recomp-addocc-canon-%d", time.Now().UnixNano()))
	retireEv := createTestEvent(t, env, agentKey,
		"Recompute Published Retiree",
		fmt.Sprintf("https://music.toronto.ca/events/recomp-addocc-retired-%d", time.Now().UnixNano()+1))

	canonULID := eventIDFromPayload(canon)
	retireULID := eventIDFromPayload(retireEv)
	require.NotEmpty(t, canonULID)
	require.NotEmpty(t, retireULID)

	// Give the canonical a pending_review state with a single warning referencing the retired event.
	warningsJSON := makePotentialDuplicateWarning(retireULID)
	reviewID := insertPendingReviewDirect(t, env, canonULID, warningsJSON, &retireULID)
	setEventLifecycle(t, env, canonULID, "pending_review")

	// Promote the canonical, retire the duplicate via consolidation.
	status, body := postConsolidate(t, env, adminToken, map[string]any{
		"event_ulid": canonULID,
		"retire":     []string{retireULID},
	})

	require.Equal(t, http.StatusOK, status, "consolidate should succeed; body: %v", body)

	// The canonical should now be published.
	assert.Equal(t, "published", getEventLifecycle(t, env, canonULID),
		"canonical should become published after all review warnings are resolved")

	// The review entry should be dismissed.
	assert.Equal(t, "merged", getReviewStatus(t, env, reviewID),
		"canonical's review entry should be dismissed (merged)")

	// The retired event should be deleted.
	retStatus, _ := getEvent(t, env, retireULID)
	assert.Equal(t, http.StatusGone, retStatus, "retired event should return 410 Gone")
}

// ---------------------------------------------------------------------------
// Test 2: AddOccurrence recompute — event stays pending_review
// This test verifies that after consolidating a source event into a canonical
// that has a pending review pointing to the retired event PLUS another
// unrelated warning, the canonical stays pending_review because not all
// warnings are resolved.
// ---------------------------------------------------------------------------
func TestRecomputeAddOccurrenceStaysPending(t *testing.T) {
	env := setupTestEnv(t)

	insertAdminUser(t, env, "admin", "admin-password-123", "admin@togather.test", "admin")
	adminToken := adminLogin(t, env, "admin", "admin-password-123")
	agentKey := insertAPIKey(t, env, "agent-recompute-addocc-pend")

	canon := createTestEvent(t, env, agentKey,
		"Recompute Stays Pending Canonical",
		fmt.Sprintf("https://music.toronto.ca/events/recomp-addocc-stay-%d", time.Now().UnixNano()))
	retireEv := createTestEvent(t, env, agentKey,
		"Recompute Stays Pending Retiree",
		fmt.Sprintf("https://music.toronto.ca/events/recomp-addocc-stay-b-%d", time.Now().UnixNano()+1))

	canonULID := eventIDFromPayload(canon)
	retireULID := eventIDFromPayload(retireEv)
	require.NotEmpty(t, canonULID)
	require.NotEmpty(t, retireULID)

	// Insert a pending review with TWO warnings: one pointing to the retired event,
	// one unrelated (reversed_dates) that should survive consolidation.
	warningsJSON := makeDualWarnings(retireULID)
	reviewID := insertPendingReviewDirect(t, env, canonULID, warningsJSON, &retireULID)
	setEventLifecycle(t, env, canonULID, "pending_review")

	status, body := postConsolidate(t, env, adminToken, map[string]any{
		"event_ulid": canonULID,
		"retire":     []string{retireULID},
	})

	require.Equal(t, http.StatusOK, status, "consolidate should succeed; body: %v", body)

	// The canonical should STILL be pending_review — the reversed_dates warning remains.
	assert.Equal(t, "pending_review", getEventLifecycle(t, env, canonULID),
		"canonical should stay pending_review because unrelated warning persists")

	// The review entry should still be pending (NOT dismissed), but with reduced warnings.
	assert.Equal(t, "pending", getReviewStatus(t, env, reviewID),
		"canonical's review entry should remain pending (dup warning stripped, reversed_dates remains)")

	// The retired event should be deleted.
	retStatus, _ := getEvent(t, env, retireULID)
	assert.Equal(t, http.StatusGone, retStatus, "retired event should return 410 Gone")
}

// ---------------------------------------------------------------------------
// Test 3: Merge recompute — surviving event becomes published
// This test verifies that after merging (via consolidate) a duplicate into a
// primary event that has a single pending review pointing to the duplicate,
// the primary's lifecycle is recomputed to "published" because the sole review
// warning is resolved.
// ---------------------------------------------------------------------------
func TestRecomputeMergeGoesPublished(t *testing.T) {
	env := setupTestEnv(t)

	insertAdminUser(t, env, "admin", "admin-password-123", "admin@togather.test", "admin")
	adminToken := adminLogin(t, env, "admin", "admin-password-123")
	agentKey := insertAPIKey(t, env, "agent-recompute-merge-pub")

	primary := createTestEvent(t, env, agentKey,
		"Merge Published Primary",
		fmt.Sprintf("https://music.toronto.ca/events/recomp-merge-prim-%d", time.Now().UnixNano()))
	dupe := createTestEvent(t, env, agentKey,
		"Merge Published Duplicate",
		fmt.Sprintf("https://music.toronto.ca/events/recomp-merge-dupe-%d", time.Now().UnixNano()+1))

	primaryULID := eventIDFromPayload(primary)
	dupeULID := eventIDFromPayload(dupe)
	require.NotEmpty(t, primaryULID)
	require.NotEmpty(t, dupeULID)

	// Simulate the merge scenario: the primary has a single pending review referencing the duplicate.
	warningsJSON := makePotentialDuplicateWarning(dupeULID)
	reviewID := insertPendingReviewDirect(t, env, primaryULID, warningsJSON, &dupeULID)
	setEventLifecycle(t, env, primaryULID, "pending_review")

	status, body := postConsolidate(t, env, adminToken, map[string]any{
		"event_ulid": primaryULID,
		"retire":     []string{dupeULID},
	})

	require.Equal(t, http.StatusOK, status, "consolidate should succeed; body: %v", body)

	// The primary should become published.
	assert.Equal(t, "published", getEventLifecycle(t, env, primaryULID),
		"primary should become published after duplicate warning is resolved")

	// The review entry should be dismissed.
	assert.Equal(t, "merged", getReviewStatus(t, env, reviewID),
		"primary's review entry should be dismissed (merged)")

	// The duplicate should return 410.
	retStatus, _ := getEvent(t, env, dupeULID)
	assert.Equal(t, http.StatusGone, retStatus, "duplicate event should return 410 Gone")
}

// ---------------------------------------------------------------------------
// Test 4: Merge recompute — surviving stays pending
// This test verifies that after merging (via consolidate) a duplicate into a
// primary event that has TWO pending reviews — one about the duplicate and one
// unrelated — the primary stays pending_review because the unrelated warning
// survives.
// ---------------------------------------------------------------------------
func TestRecomputeMergeStaysPending(t *testing.T) {
	env := setupTestEnv(t)

	insertAdminUser(t, env, "admin", "admin-password-123", "admin@togather.test", "admin")
	adminToken := adminLogin(t, env, "admin", "admin-password-123")
	agentKey := insertAPIKey(t, env, "agent-recompute-merge-pend")

	primary := createTestEvent(t, env, agentKey,
		"Merge Stays Pending Primary",
		fmt.Sprintf("https://music.toronto.ca/events/recomp-merge-stay-%d", time.Now().UnixNano()))
	dupe := createTestEvent(t, env, agentKey,
		"Merge Stays Pending Duplicate",
		fmt.Sprintf("https://music.toronto.ca/events/recomp-merge-stay-b-%d", time.Now().UnixNano()+1))

	primaryULID := eventIDFromPayload(primary)
	dupeULID := eventIDFromPayload(dupe)
	require.NotEmpty(t, primaryULID)
	require.NotEmpty(t, dupeULID)

	// Two warnings: one about the duplicate, one unrelated.
	warningsJSON := makeDualWarnings(dupeULID)
	reviewID := insertPendingReviewDirect(t, env, primaryULID, warningsJSON, &dupeULID)
	setEventLifecycle(t, env, primaryULID, "pending_review")

	status, body := postConsolidate(t, env, adminToken, map[string]any{
		"event_ulid": primaryULID,
		"retire":     []string{dupeULID},
	})

	require.Equal(t, http.StatusOK, status, "consolidate should succeed; body: %v", body)

	// The primary should stay pending_review.
	assert.Equal(t, "pending_review", getEventLifecycle(t, env, primaryULID),
		"primary should stay pending_review because unrelated warning persists")

	// The review entry should still be pending.
	assert.Equal(t, "pending", getReviewStatus(t, env, reviewID),
		"primary's review entry should remain pending")

	// The duplicate should return 410.
	retStatus, _ := getEvent(t, env, dupeULID)
	assert.Equal(t, http.StatusGone, retStatus, "duplicate event should return 410 Gone")
}

// ---------------------------------------------------------------------------
// Test 5: Consolidate recompute — canonical published after retirement
// This test verifies that when a canonical event has a pending review tied to
// a retired event, consolidation retires that event, dismisses the review,
// and the canonical becomes published because 0 pending reviews remain.
// ---------------------------------------------------------------------------
func TestRecomputeConsolidateCanonicalPublished(t *testing.T) {
	env := setupTestEnv(t)

	insertAdminUser(t, env, "admin", "admin-password-123", "admin@togather.test", "admin")
	adminToken := adminLogin(t, env, "admin", "admin-password-123")
	agentKey := insertAPIKey(t, env, "agent-recompute-consol-pub")

	canon := createTestEvent(t, env, agentKey,
		"Consolidate Published Canonical",
		fmt.Sprintf("https://music.toronto.ca/events/recomp-consol-pub-%d", time.Now().UnixNano()))
	retired := createTestEvent(t, env, agentKey,
		"Consolidate Published Retired",
		fmt.Sprintf("https://music.toronto.ca/events/recomp-consol-pub-b-%d", time.Now().UnixNano()+1))

	canonULID := eventIDFromPayload(canon)
	retiredULID := eventIDFromPayload(retired)
	require.NotEmpty(t, canonULID)
	require.NotEmpty(t, retiredULID)

	// Canonical has 1 pending review tied to the event being retired.
	warningsJSON := makePotentialDuplicateWarning(retiredULID)
	reviewID := insertPendingReviewDirect(t, env, canonULID, warningsJSON, &retiredULID)
	setEventLifecycle(t, env, canonULID, "pending_review")

	assert.Equal(t, 1, countPendingReviewsForEvent(t, env, canonULID),
		"canonical should have exactly 1 pending review before consolidation")

	status, body := postConsolidate(t, env, adminToken, map[string]any{
		"event_ulid": canonULID,
		"retire":     []string{retiredULID},
	})

	require.Equal(t, http.StatusOK, status, "consolidate should succeed; body: %v", body)

	// Canonical should be published.
	assert.Equal(t, "published", getEventLifecycle(t, env, canonULID),
		"canonical should become published after all reviews dismissed")

	// The pending review should be dismissed.
	assert.Equal(t, "merged", getReviewStatus(t, env, reviewID),
		"canonical's review entry should be dismissed")

	// 0 pending reviews remain.
	assert.Equal(t, 0, countPendingReviewsForEvent(t, env, canonULID),
		"canonical should have 0 pending reviews after consolidation")

	// Retired event is gone.
	retStatus, _ := getEvent(t, env, retiredULID)
	assert.Equal(t, http.StatusGone, retStatus, "retired event should return 410 Gone")
}

// ---------------------------------------------------------------------------
// Test 6: Consolidate recompute — canonical stays pending
// This test verifies that when a canonical event has 2 pending reviews but
// consolidation only resolves 1 (the other is unrelated to any retired event),
// the canonical stays pending_review.
// ---------------------------------------------------------------------------
func TestRecomputeConsolidateCanonicalStaysPending(t *testing.T) {
	env := setupTestEnv(t)

	insertAdminUser(t, env, "admin", "admin-password-123", "admin@togather.test", "admin")
	adminToken := adminLogin(t, env, "admin", "admin-password-123")
	agentKey := insertAPIKey(t, env, "agent-recompute-consol-pend")

	canon := createTestEvent(t, env, agentKey,
		"Consolidate Stays Pending Canonical",
		fmt.Sprintf("https://music.toronto.ca/events/recomp-consol-stay-%d", time.Now().UnixNano()))
	retired := createTestEvent(t, env, agentKey,
		"Consolidate Stays Pending Retired",
		fmt.Sprintf("https://music.toronto.ca/events/recomp-consol-stay-b-%d", time.Now().UnixNano()+1))

	canonULID := eventIDFromPayload(canon)
	retiredULID := eventIDFromPayload(retired)
	require.NotEmpty(t, canonULID)
	require.NotEmpty(t, retiredULID)

	// Two warnings: 1 about the retired event, 1 unrelated.
	warningsJSON := makeDualWarnings(retiredULID)
	reviewID := insertPendingReviewDirect(t, env, canonULID, warningsJSON, &retiredULID)
	setEventLifecycle(t, env, canonULID, "pending_review")

	assert.Equal(t, 1, countPendingReviewsForEvent(t, env, canonULID),
		"canonical should have exactly 1 pending review before consolidation")

	status, body := postConsolidate(t, env, adminToken, map[string]any{
		"event_ulid": canonULID,
		"retire":     []string{retiredULID},
	})

	require.Equal(t, http.StatusOK, status, "consolidate should succeed; body: %v", body)

	// Canonical should stay pending_review.
	assert.Equal(t, "pending_review", getEventLifecycle(t, env, canonULID),
		"canonical should stay pending_review because unrelated warning remains")

	// The review entry should still be pending (not dismissed).
	assert.Equal(t, "pending", getReviewStatus(t, env, reviewID),
		"canonical's review entry should remain pending")

	// 1 pending review remains.
	assert.Equal(t, 1, countPendingReviewsForEvent(t, env, canonULID),
		"canonical should have 1 pending review after consolidation (the unrelated warning)")

	// Retired event is gone.
	retStatus, _ := getEvent(t, env, retiredULID)
	assert.Equal(t, http.StatusGone, retStatus, "retired event should return 410 Gone")
}

// ---------------------------------------------------------------------------
// Test 7: Dismissed review archival gap
// This test verifies that dismissed review queue entries are NOT cleaned up
// by CleanupArchivedReviews (which only covers approved/superseded/merged).
// It then confirms the fix by adding 'dismissed' to the cleanup query and
// verifying the entry is cleaned up.
// ---------------------------------------------------------------------------
func TestRecomputeDismissedReviewArchivalGap(t *testing.T) {
	env := setupTestEnv(t)

	// Create an event to anchor the review entry.
	agentKey := insertAPIKey(t, env, "agent-recompute-archival")
	ev := createTestEvent(t, env, agentKey,
		"Archival Gap Event",
		fmt.Sprintf("https://music.toronto.ca/events/recomp-archival-%d", time.Now().UnixNano()))
	eventULID := eventIDFromPayload(ev)
	require.NotEmpty(t, eventULID)

	// Insert a dismissed review entry with reviewed_at set to > 90 days ago.
	var internalID string
	err := env.Pool.QueryRow(env.Context, `SELECT id FROM events WHERE ulid = $1`, eventULID).Scan(&internalID)
	require.NoError(t, err)

	// Remove any existing review entry for this event (auto-created during ingest).
	_, _ = env.Pool.Exec(env.Context, `DELETE FROM event_review_queue WHERE event_id = $1`, internalID)

	payload := json.RawMessage(`{"name":"archival test"}`)
	warnings := json.RawMessage(`[]`)
	oldDate := time.Now().Add(-120 * 24 * time.Hour) // 120 days ago, well past the 90-day threshold

	var dismissedID int
	err = env.Pool.QueryRow(env.Context, `
		INSERT INTO event_review_queue
			(event_id, original_payload, normalized_payload, warnings, event_start_time,
			 status, reviewed_by, reviewed_at, created_at, updated_at)
		VALUES ($1, $2, $2, $3, NOW() + INTERVAL '30 days',
		        'dismissed', 'system', $4, $4, $4)
		RETURNING id
	`, internalID, payload, warnings, oldDate).Scan(&dismissedID)
	require.NoError(t, err)

	// Verify the dismissed entry exists.
	var count int
	err = env.Pool.QueryRow(env.Context, `SELECT COUNT(*) FROM event_review_queue WHERE id = $1`, dismissedID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "dismissed review entry should exist before cleanup")

	// Run the cleanup query for archived reviews (only approved/superseded/merged).
	// This simulates what CleanupArchivedReviews does.
	result, err := env.Pool.Exec(env.Context, `
		DELETE FROM event_review_queue
		 WHERE status IN ('approved', 'superseded', 'merged')
		   AND reviewed_at < NOW() - INTERVAL '90 days'
	`)
	require.NoError(t, err)

	// The dismissed entry should PERSIST — confirming the gap.
	err = env.Pool.QueryRow(env.Context, `SELECT COUNT(*) FROM event_review_queue WHERE id = $1`, dismissedID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count,
		"dismissed review entry should persist after cleanup (gap: dismissed not in cleanup query)")

	// Now FIX THE GAP: include 'dismissed' in the cleanup query.
	_, err = env.Pool.Exec(env.Context, `
		DELETE FROM event_review_queue
		 WHERE status IN ('approved', 'superseded', 'merged', 'dismissed')
		   AND reviewed_at < NOW() - INTERVAL '90 days'
	`)
	require.NoError(t, err)

	// The dismissed entry should NOW be cleaned up.
	err = env.Pool.QueryRow(env.Context, `SELECT COUNT(*) FROM event_review_queue WHERE id = $1`, dismissedID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count,
		"dismissed review entry should be cleaned up after fix (gap resolved)")

	// Verify that the cleanup affected rows.
	rowsAffected := result.RowsAffected()
	assert.Equal(t, int64(0), rowsAffected,
		"original query (without dismissed) should affect 0 dismissed rows")
}
