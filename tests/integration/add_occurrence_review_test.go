package integration

// Integration tests for the add-occurrence review workflow.
//
// The near-dup detection pipeline (potential_duplicate /
// near_duplicate_of_new_event warnings) is disabled in integration tests
// because testConfig() sets a zero-value DedupConfig (NearDuplicateThreshold
// = 0.0).  Instead, we set up state directly via raw SQL to exercise the full
// HTTP handler → service → DB path without relying on ingest-time detection.

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

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// insertBareEvent inserts a minimal event row and returns its internal UUID
// and ULID.  lifecycle_state defaults to "published".
// A virtual_url is set to satisfy the event_location_required CHECK constraint.
func insertBareEvent(t *testing.T, env *testEnv, ulid, name, state string) (internalID string) {
	t.Helper()
	if state == "" {
		state = "published"
	}
	err := env.Pool.QueryRow(env.Context, `
		INSERT INTO events (ulid, name, lifecycle_state, virtual_url)
		VALUES ($1, $2, $3, 'https://test.example.com/event')
		RETURNING id
	`, ulid, name, state).Scan(&internalID)
	require.NoError(t, err, "insertBareEvent: %s", ulid)
	return internalID
}

// insertBareOccurrence inserts an occurrence row linked to the given internal
// event UUID.  startTime is required; endTime may be zero (stored as NULL).
// A virtual_url is set to satisfy the occurrence_location_required CHECK constraint.
func insertBareOccurrence(t *testing.T, env *testEnv, eventID string, startTime, endTime time.Time) {
	t.Helper()
	var endVal interface{}
	if !endTime.IsZero() {
		endVal = endTime
	}
	_, err := env.Pool.Exec(env.Context, `
		INSERT INTO event_occurrences (event_id, start_time, end_time, timezone, virtual_url)
		VALUES ($1, $2, $3, 'America/Toronto', 'https://test.example.com/event')
	`, eventID, startTime, endVal)
	require.NoError(t, err, "insertBareOccurrence for event %s", eventID)
}

// insertReviewEntry inserts a pending review queue row for the given event and
// returns the auto-assigned numeric id.  warnings must be a JSON array string.
// duplicateOfEventID may be "" (will be stored as NULL).
func insertReviewEntry(
	t *testing.T, env *testEnv,
	eventID string, warnings string,
	startTime time.Time,
	duplicateOfEventID string,
) int {
	t.Helper()
	var dupID interface{}
	if duplicateOfEventID != "" {
		dupID = duplicateOfEventID
	}
	var reviewID int
	err := env.Pool.QueryRow(env.Context, `
		INSERT INTO event_review_queue
			(event_id, original_payload, normalized_payload, warnings,
			 event_start_time, status, duplicate_of_event_id)
		VALUES ($1, '{}', '{}', $2::jsonb, $3, 'pending', $4)
		RETURNING id
	`, eventID, warnings, startTime, dupID).Scan(&reviewID)
	require.NoError(t, err, "insertReviewEntry for event %s", eventID)
	return reviewID
}

// addOccurrenceRequest POSTs to /api/v1/admin/review-queue/{id}/add-occurrence
// and returns the response.
func addOccurrenceRequest(t *testing.T, env *testEnv, token string, reviewID int, targetEventULID string) *http.Response {
	t.Helper()
	body := map[string]string{}
	if targetEventULID != "" {
		body["target_event_ulid"] = targetEventULID
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err)

	url := fmt.Sprintf("%s/api/v1/admin/review-queue/%d/add-occurrence", env.Server.URL, reviewID)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// adminSetup creates an admin user and returns a JWT token.
func adminSetup(t *testing.T, env *testEnv) string {
	t.Helper()
	insertAdminUser(t, env, "admin", "admin-password-123", "admin@example.com", "admin")
	return adminLogin(t, env, "admin", "admin-password-123")
}

// ---------------------------------------------------------------------------
// Forward path (potential_duplicate) — happy path
// ---------------------------------------------------------------------------

// TestAddOccurrenceReview_ForwardPath_TargetPublished verifies that when a
// pending review has a potential_duplicate warning and the target event has no
// other issues, the add-occurrence action:
//   - returns 200 with the updated review entry
//   - closes the review row (status = "merged")
//   - adds the new occurrence to the target event
//   - soft-deletes the source event (lifecycle_state = "deleted")
func TestAddOccurrenceReview_ForwardPath_TargetPublished(t *testing.T) {
	env := setupTestEnv(t)
	token := adminSetup(t, env)

	start := time.Date(2026, 11, 1, 19, 0, 0, 0, time.UTC)
	end := time.Date(2026, 11, 1, 22, 0, 0, 0, time.UTC)

	// Target: existing series event with one published occurrence (different time).
	targetULID := "01KKY7HMRFHPPXSQAYY9WR68TM"
	targetStart := time.Date(2026, 10, 1, 19, 0, 0, 0, time.UTC)
	targetEnd := time.Date(2026, 10, 1, 22, 0, 0, 0, time.UTC)
	targetID := insertBareEvent(t, env, targetULID, "Series Event", "published")
	insertBareOccurrence(t, env, targetID, targetStart, targetEnd)

	// Source: pending event with one occurrence — the new occurrence to absorb.
	sourceULID := "01KKY7HMRFHPPXSQAYYA371MKD"
	sourceID := insertBareEvent(t, env, sourceULID, "Series Event (dup)", "pending_review")
	insertBareOccurrence(t, env, sourceID, start, end)

	// Review queue: pending entry with potential_duplicate warning, pointing to source.
	warnings := `[{"code":"potential_duplicate","field":"","message":"potential duplicate"}]`
	reviewID := insertReviewEntry(t, env, sourceID, warnings, start, "")

	// Call the endpoint.
	resp := addOccurrenceRequest(t, env, token, reviewID, targetULID)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	assert.Equal(t, "merged", result["status"])
	assert.Equal(t, targetULID, result["targetEventUlid"])

	// The source event must be soft-deleted.
	var srcState string
	err := env.Pool.QueryRow(env.Context, `SELECT lifecycle_state FROM events WHERE ulid = $1`, sourceULID).Scan(&srcState)
	require.NoError(t, err)
	assert.Equal(t, "deleted", srcState)

	// The target event must now have two occurrences.
	var occCount int
	err = env.Pool.QueryRow(env.Context, `
		SELECT COUNT(*) FROM event_occurrences eo
		JOIN events e ON eo.event_id = e.id
		WHERE e.ulid = $1
	`, targetULID).Scan(&occCount)
	require.NoError(t, err)
	assert.Equal(t, 2, occCount, "target should have two occurrences after add-occurrence")

	// The review row must be closed.
	var reviewStatus string
	err = env.Pool.QueryRow(env.Context, `SELECT status FROM event_review_queue WHERE id = $1`, reviewID).Scan(&reviewStatus)
	require.NoError(t, err)
	assert.Equal(t, "merged", reviewStatus)
}

// ---------------------------------------------------------------------------
// Forward path — companion review cleanup
// ---------------------------------------------------------------------------

// TestAddOccurrenceReview_ForwardPath_CompanionReviewDismissed verifies that
// when a companion review exists on the target event (the near-dup reverse
// direction), it is also dismissed when the forward-path add-occurrence runs.
func TestAddOccurrenceReview_ForwardPath_CompanionReviewDismissed(t *testing.T) {
	env := setupTestEnv(t)
	token := adminSetup(t, env)

	start := time.Date(2026, 11, 2, 19, 0, 0, 0, time.UTC)
	end := time.Date(2026, 11, 2, 22, 0, 0, 0, time.UTC)

	// Target: existing series event.
	targetULID := "01KKY7HMRFHPPXSQAYYBTX5MT2"
	targetStart := time.Date(2026, 10, 2, 19, 0, 0, 0, time.UTC)
	targetEnd := time.Date(2026, 10, 2, 22, 0, 0, 0, time.UTC)
	targetID := insertBareEvent(t, env, targetULID, "Series Event B", "published")
	insertBareOccurrence(t, env, targetID, targetStart, targetEnd)

	// Source: new event with single occurrence.
	sourceULID := "01KKY7HMRFHPPXSQAYYFKBDVGG"
	sourceID := insertBareEvent(t, env, sourceULID, "Series Event B (new)", "pending_review")
	insertBareOccurrence(t, env, sourceID, start, end)

	// Source review — forward path.
	sourceWarnings := `[{"code":"potential_duplicate","field":"","message":"dup"}]`
	sourceReviewID := insertReviewEntry(t, env, sourceID, sourceWarnings, start, "")

	// Companion review on the TARGET event, cross-linking back to the source.
	// This simulates the near-dup ingest path that creates a companion warning.
	companionWarnings := `[{"code":"near_duplicate_of_new_event","field":"","message":"near dup of new"}]`
	companionReviewID := insertReviewEntry(t, env, targetID, companionWarnings, targetStart, sourceID)

	// Run add-occurrence (forward path).
	resp := addOccurrenceRequest(t, env, token, sourceReviewID, targetULID)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Companion review must have been dismissed (any non-pending status).
	var compStatus string
	err := env.Pool.QueryRow(env.Context, `SELECT status FROM event_review_queue WHERE id = $1`, companionReviewID).Scan(&compStatus)
	require.NoError(t, err)
	assert.NotEqual(t, "pending", compStatus, "companion review should be dismissed after add-occurrence")
}

// ---------------------------------------------------------------------------
// Near-dup path (near_duplicate_of_new_event) — happy path
// ---------------------------------------------------------------------------

// TestAddOccurrenceReview_NearDupPath verifies the inverted absorption path:
// the source is the newly-ingested event (stored in duplicate_of_event_id),
// and the target is the review's own event (existing series).
func TestAddOccurrenceReview_NearDupPath(t *testing.T) {
	env := setupTestEnv(t)
	token := adminSetup(t, env)

	// Target: existing series event (review sits on this one).
	targetULID := "01KKY7HMRFHPPXSQAYYFKCBEPH"
	targetStart := time.Date(2026, 10, 3, 19, 0, 0, 0, time.UTC)
	targetEnd := time.Date(2026, 10, 3, 22, 0, 0, 0, time.UTC)
	targetID := insertBareEvent(t, env, targetULID, "Series Event C", "published")
	insertBareOccurrence(t, env, targetID, targetStart, targetEnd)

	// Source: newly-ingested event (the new occurrence candidate).
	sourceULID := "01KKY7HMRFHPPXSQAYYFTHZ5G2"
	sourceStart := time.Date(2026, 11, 3, 19, 0, 0, 0, time.UTC)
	sourceEnd := time.Date(2026, 11, 3, 22, 0, 0, 0, time.UTC)
	sourceID := insertBareEvent(t, env, sourceULID, "Series Event C (new)", "pending_review")
	insertBareOccurrence(t, env, sourceID, sourceStart, sourceEnd)

	// Near-dup review sits on the TARGET event; duplicate_of_event_id points to source.
	warnings := `[{"code":"near_duplicate_of_new_event","field":"","message":"near dup"}]`
	reviewID := insertReviewEntry(t, env, targetID, warnings, targetStart, sourceID)

	// Call without target_event_ulid (near-dup path ignores it).
	resp := addOccurrenceRequest(t, env, token, reviewID, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	assert.Equal(t, "merged", result["status"])
	// targetEventUlid in the response must be the existing series (target).
	assert.Equal(t, targetULID, result["targetEventUlid"])

	// Source event (newly-ingested) must be soft-deleted.
	var srcState string
	err := env.Pool.QueryRow(env.Context, `SELECT lifecycle_state FROM events WHERE ulid = $1`, sourceULID).Scan(&srcState)
	require.NoError(t, err)
	assert.Equal(t, "deleted", srcState)

	// Target event must now have two occurrences.
	var occCount int
	err = env.Pool.QueryRow(env.Context, `
		SELECT COUNT(*) FROM event_occurrences eo
		JOIN events e ON eo.event_id = e.id
		WHERE e.ulid = $1
	`, targetULID).Scan(&occCount)
	require.NoError(t, err)
	assert.Equal(t, 2, occCount)
}

// ---------------------------------------------------------------------------
// Rejection: zero-occurrence source
// ---------------------------------------------------------------------------

// TestAddOccurrenceReview_ZeroOccurrenceSource verifies that when the source
// event has no occurrences, the endpoint returns 422 with problem type
// "zero-occurrence-source".
func TestAddOccurrenceReview_ZeroOccurrenceSource(t *testing.T) {
	env := setupTestEnv(t)
	token := adminSetup(t, env)

	start := time.Date(2026, 11, 4, 19, 0, 0, 0, time.UTC)

	// Target: existing event with an occurrence.
	targetULID := "01KKY7HMRFHPPXSQAYYG802FN2"
	targetStart := time.Date(2026, 10, 4, 19, 0, 0, 0, time.UTC)
	targetEnd := time.Date(2026, 10, 4, 22, 0, 0, 0, time.UTC)
	targetID := insertBareEvent(t, env, targetULID, "Series Event D", "published")
	insertBareOccurrence(t, env, targetID, targetStart, targetEnd)

	// Source: event with ZERO occurrences.
	sourceULID := "01KKY7HMRFHPPXSQAYYGNFEGRK"
	sourceID := insertBareEvent(t, env, sourceULID, "Series Event D (no-occ)", "pending_review")
	// intentionally no occurrence inserted

	warnings := `[{"code":"potential_duplicate","field":"","message":"dup"}]`
	reviewID := insertReviewEntry(t, env, sourceID, warnings, start, "")

	resp := addOccurrenceRequest(t, env, token, reviewID, targetULID)
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)

	var errResp map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&errResp))
	assert.Contains(t, errResp["type"], "zero-occurrence-source")
}

// ---------------------------------------------------------------------------
// Rejection: multi-occurrence source
// ---------------------------------------------------------------------------

// TestAddOccurrenceReview_MultiOccurrenceSource verifies that when the source
// event has more than one occurrence, the endpoint returns 422 with problem
// type "ambiguous-occurrence-source".
func TestAddOccurrenceReview_MultiOccurrenceSource(t *testing.T) {
	env := setupTestEnv(t)
	token := adminSetup(t, env)

	// Target: existing event.
	targetULID := "01KKY7HMRFHPPXSQAYYHX9BFG9"
	targetStart := time.Date(2026, 10, 5, 19, 0, 0, 0, time.UTC)
	targetEnd := time.Date(2026, 10, 5, 22, 0, 0, 0, time.UTC)
	targetID := insertBareEvent(t, env, targetULID, "Series Event E", "published")
	insertBareOccurrence(t, env, targetID, targetStart, targetEnd)

	// Source: event with TWO occurrences — ambiguous.
	sourceULID := "01KKY7HMRFHPPXSQAYYJ1GRR9J"
	sourceStart1 := time.Date(2026, 11, 5, 19, 0, 0, 0, time.UTC)
	sourceEnd1 := time.Date(2026, 11, 5, 22, 0, 0, 0, time.UTC)
	sourceStart2 := time.Date(2026, 12, 5, 19, 0, 0, 0, time.UTC)
	sourceEnd2 := time.Date(2026, 12, 5, 22, 0, 0, 0, time.UTC)
	sourceID := insertBareEvent(t, env, sourceULID, "Series Event E (multi)", "pending_review")
	insertBareOccurrence(t, env, sourceID, sourceStart1, sourceEnd1)
	insertBareOccurrence(t, env, sourceID, sourceStart2, sourceEnd2)

	warnings := `[{"code":"potential_duplicate","field":"","message":"dup"}]`
	reviewID := insertReviewEntry(t, env, sourceID, warnings, sourceStart1, "")

	resp := addOccurrenceRequest(t, env, token, reviewID, targetULID)
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)

	var errResp map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&errResp))
	assert.Contains(t, errResp["type"], "ambiguous-occurrence-source")
}

// ---------------------------------------------------------------------------
// Rejection: unsupported review (no duplicate warning)
// ---------------------------------------------------------------------------

// TestAddOccurrenceReview_UnsupportedReview verifies that a review without
// any potential_duplicate or near_duplicate_of_new_event warning returns 422.
func TestAddOccurrenceReview_UnsupportedReview(t *testing.T) {
	env := setupTestEnv(t)
	token := adminSetup(t, env)

	targetULID := "01KKY7HMRFHPPXSQAYYJFHCGR5"
	targetStart := time.Date(2026, 10, 6, 19, 0, 0, 0, time.UTC)
	targetEnd := time.Date(2026, 10, 6, 22, 0, 0, 0, time.UTC)
	targetID := insertBareEvent(t, env, targetULID, "Series Event F", "published")
	insertBareOccurrence(t, env, targetID, targetStart, targetEnd)

	sourceULID := "01KKY7HMRFHPPXSQAYYN9HEZ6B"
	sourceStart := time.Date(2026, 11, 6, 19, 0, 0, 0, time.UTC)
	sourceEnd := time.Date(2026, 11, 6, 22, 0, 0, 0, time.UTC)
	sourceID := insertBareEvent(t, env, sourceULID, "Source F", "pending_review")
	insertBareOccurrence(t, env, sourceID, sourceStart, sourceEnd)

	// Only a reversed-dates warning — NOT a duplicate warning.
	warnings := `[{"code":"reversed_dates_timezone_likely","field":"endDate","message":"reversed"}]`
	reviewID := insertReviewEntry(t, env, sourceID, warnings, sourceStart, "")

	resp := addOccurrenceRequest(t, env, token, reviewID, targetULID)
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)

	var errResp map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&errResp))
	assert.Contains(t, errResp["type"], "unsupported-review-for-occurrence")
}

// ---------------------------------------------------------------------------
// Rejection: review not found
// ---------------------------------------------------------------------------

// TestAddOccurrenceReview_NotFound verifies that a request for a non-existent
// review ID returns 404.
func TestAddOccurrenceReview_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	token := adminSetup(t, env)

	resp := addOccurrenceRequest(t, env, token, 999999, "01KKY7HMRFHPPXSQAYY9WR68TM")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// Auth guard
// ---------------------------------------------------------------------------

// TestAddOccurrenceReview_Unauthorized verifies that unauthenticated requests
// are rejected with 401.
func TestAddOccurrenceReview_Unauthorized(t *testing.T) {
	env := setupTestEnv(t)

	url := fmt.Sprintf("%s/api/v1/admin/review-queue/1/add-occurrence", env.Server.URL)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader([]byte(`{}`)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// TestAddOccurrenceReview_ForbiddenViewer verifies that a non-admin (viewer)
// user gets 403 when attempting the add-occurrence action.
func TestAddOccurrenceReview_ForbiddenViewer(t *testing.T) {
	env := setupTestEnv(t)
	insertAdminUser(t, env, "viewer", "viewer-password-123", "viewer@example.com", "viewer")
	viewerToken := adminLogin(t, env, "viewer", "viewer-password-123")

	resp := addOccurrenceRequest(t, env, viewerToken, 1, "01KKY7HMRFHPPXSQAYY9WR68TM")
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}
