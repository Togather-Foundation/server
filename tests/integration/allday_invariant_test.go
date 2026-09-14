package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The all-day invariant: a stored occurrence row with is_all_day = true must be
// anchored to local midnight in its own zone, and a genuinely timed occurrence
// must keep its instant and carry no marker. Ingest has enforced this since the
// all-day/date-only work; these tests cover the two admin writers that did not
// (the review fix/approve path and the per-occurrence PUT).
//
// Regression: t_675b7d3c F2 — approving a review entry rewrote EVERY occurrence
// row of the event to occurrences[0]'s start/end plus the event-level all-day
// flag, collapsing a multi-date series and re-stamping timed rows as date-only.

const (
	alldayTestAdminUser     = "allday-admin"
	alldayTestAdminPassword = "allday-admin-password-123"
)

// createEventAsAgent posts an event payload with an agent API key and returns
// the created ULID.
func createEventAsAgent(t *testing.T, env *testEnv, agentKey string, payload map[string]any) string {
	t.Helper()

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+agentKey)
	req.Header.Set("Content-Type", "application/ld+json")
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		var failure map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&failure)
		require.Failf(t, "unexpected status", "create event: status=%d response=%v", resp.StatusCode, failure)
	}

	var created map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	ulid := eventIDFromPayload(created)
	require.NotEmpty(t, ulid, "create response must include an event @id")
	return ulid
}

// getEventAsAgent fetches an event and returns the decoded JSON-LD document.
func getEventAsAgent(t *testing.T, env *testEnv, ulid string) map[string]any {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events/"+ulid, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	return got
}

// subEvents extracts the subEvent array from an event document.
func subEvents(t *testing.T, event map[string]any) []map[string]any {
	t.Helper()

	raw, ok := event["subEvent"].([]any)
	require.True(t, ok, "subEvent must be present, got %T", event["subEvent"])

	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		sub, ok := item.(map[string]any)
		require.True(t, ok, "subEvent entries must be objects")
		out = append(out, sub)
	}
	return out
}

// occurrenceUUIDFromSubEvent pulls the occurrence UUID out of a subEvent @id
// (…/api/v1/admin/events/<ulid>/occurrences/<uuid>) — the identifier the admin
// occurrence endpoints take.
func occurrenceUUIDFromSubEvent(t *testing.T, sub map[string]any) string {
	t.Helper()

	id, ok := sub["@id"].(string)
	require.True(t, ok, "subEvent entry must carry an @id, got %v", sub["@id"])
	parts := strings.Split(id, "/")
	require.NotEmpty(t, parts)
	return parts[len(parts)-1]
}

// insertPendingReviewForEvent inserts a pending review queue row for an event
// and returns the review id.
func insertPendingReviewForEvent(t *testing.T, env *testEnv, eventULID string, allDay bool, startTime time.Time) int {
	t.Helper()

	var internalID string
	require.NoError(t, env.Pool.QueryRow(env.Context,
		`SELECT id FROM events WHERE ulid = $1`, eventULID).Scan(&internalID),
		"look up internal id for event %s", eventULID)

	var reviewID int
	require.NoError(t, env.Pool.QueryRow(env.Context, `
		INSERT INTO event_review_queue
			(event_id, original_payload, normalized_payload, warnings, event_start_time, event_all_day, status)
		VALUES ($1, '{}'::jsonb, '{}'::jsonb, '[]'::jsonb, $2, $3, 'pending')
		RETURNING id
	`, internalID, startTime, allDay).Scan(&reviewID),
		"insert pending review for event %s", eventULID)

	return reviewID
}

// postReviewFix posts date corrections to the review fix endpoint as an admin.
func postReviewFix(t *testing.T, env *testEnv, adminToken string, reviewID int, startDate, endDate *time.Time) *http.Response {
	t.Helper()

	corrections := map[string]any{}
	if startDate != nil {
		corrections["startDate"] = startDate.Format(time.RFC3339)
	}
	if endDate != nil {
		corrections["endDate"] = endDate.Format(time.RFC3339)
	}
	body, err := json.Marshal(map[string]any{"corrections": corrections, "notes": "integration test"})
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/v1/admin/review-queue/%d/fix", env.Server.URL, reviewID), bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	return resp
}

// putOccurrence patches an occurrence as an admin.
func putOccurrence(t *testing.T, env *testEnv, adminToken, eventULID, occurrenceUUID string, patch map[string]any) *http.Response {
	t.Helper()

	body, err := json.Marshal(patch)
	require.NoError(t, err)

	url := fmt.Sprintf("%s/api/v1/admin/events/%s/occurrences/%s", env.Server.URL, eventULID, occurrenceUUID)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	return resp
}

// assertAllDayRowsAreAnchored fails if any stored row claims is_all_day while
// holding a non-midnight local start time. local_start_time is a generated
// column (start_time AT TIME ZONE timezone), so this is the invariant expressed
// exactly as the schema sees it.
func assertAllDayRowsAreAnchored(t *testing.T, env *testEnv) {
	t.Helper()

	var offenders int
	require.NoError(t, env.Pool.QueryRow(env.Context, `
		SELECT COUNT(*) FROM event_occurrences
		WHERE is_all_day AND local_start_time <> TIME '00:00:00'
	`).Scan(&offenders))
	require.Zero(t, offenders, "rows with is_all_day = true must be anchored to local midnight")
}

// TestAdminFixReviewKeepsMultiOccurrenceSeries is the card's repro: a
// three-occurrence series in the review queue, corrected via the fix endpoint,
// must come back with three DISTINCT dates and the timed rows must stay timed.
func TestAdminFixReviewKeepsMultiOccurrenceSeries(t *testing.T) {
	env := setupTestEnv(t)

	insertAdminUser(t, env, alldayTestAdminUser, alldayTestAdminPassword, "allday-admin@example.com", "admin")
	adminToken := adminLogin(t, env, alldayTestAdminUser, alldayTestAdminPassword)
	agentKey := insertAPIKey(t, env, "allday-agent-series")

	// A listing with three genuinely timed occurrences. The event-level startDate
	// is timed here so the ingest validation leaves every occurrence timed (the
	// inferred event-level all-day over-reach was fixed in t_2fffdfe8, which is
	// now merged). The state the fix must survive is set up explicitly below:
	// event-level marker true, every occurrence row timed.
	ulid := createEventAsAgent(t, env, agentKey, map[string]any{
		"name":        "Three Night Run",
		"description": "A three-date series, every date timed at 19:00.",
		"startDate":   "2026-11-01T19:00:00-05:00",
		"occurrences": []map[string]any{
			{"startDate": "2026-11-01T19:00:00-05:00", "endDate": "2026-11-01T21:00:00-05:00"},
			{"startDate": "2026-11-02T19:00:00-05:00", "endDate": "2026-11-02T21:00:00-05:00"},
			{"startDate": "2026-11-03T19:00:00-05:00", "endDate": "2026-11-03T21:00:00-05:00"},
		},
		"location": map[string]any{
			"name":            "The Rex",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
	})

	before := subEvents(t, getEventAsAgent(t, env, ulid))
	require.Len(t, before, 3)
	require.Equal(t, "2026-11-01T19:00:00-05:00", before[0]["startDate"])
	require.Equal(t, "2026-11-02T19:00:00-05:00", before[1]["startDate"])
	require.Equal(t, "2026-11-03T19:00:00-05:00", before[2]["startDate"])

	// Put the event in the review queue with the event-level all-day flag set,
	// which is what the previous code smeared across all three rows.
	reviewStart := time.Date(2026, 11, 1, 19, 0, 0, 0, time.FixedZone("EST", -5*3600))
	reviewID := insertPendingReviewForEvent(t, env, ulid, true, reviewStart)

	correctedStart := time.Date(2026, 11, 1, 20, 0, 0, 0, time.FixedZone("EST", -5*3600))
	correctedEnd := time.Date(2026, 11, 1, 22, 0, 0, 0, time.FixedZone("EST", -5*3600))
	resp := postReviewFix(t, env, adminToken, reviewID, &correctedStart, &correctedEnd)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		var failure map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&failure)
		require.Failf(t, "unexpected status", "fix review: status=%d response=%v", resp.StatusCode, failure)
	}

	after := subEvents(t, getEventAsAgent(t, env, ulid))
	require.Len(t, after, 3, "every occurrence row must survive the fix")

	// The corrected row (occurrences[0], which the event's own dates mirror)
	// moves; the two sibling rows do not.
	require.Equal(t, "2026-11-01T20:00:00-05:00", after[0]["startDate"])
	require.Equal(t, "2026-11-02T19:00:00-05:00", after[1]["startDate"],
		"sibling occurrence must not be rewritten to occurrences[0]'s dates")
	require.Equal(t, "2026-11-03T19:00:00-05:00", after[2]["startDate"],
		"sibling occurrence must not be rewritten to occurrences[0]'s dates")

	for i, sub := range after {
		require.NotContains(t, sub, "allDay",
			"subEvent[%d] is genuinely timed and must not carry the event-level all-day marker", i)
	}
	// Distinct dates — the collapse turned all three into one instant.
	require.NotEqual(t, after[0]["startDate"], after[1]["startDate"])
	require.NotEqual(t, after[1]["startDate"], after[2]["startDate"])

	// The event-level dates mirror occurrences[0].
	require.Equal(t, "2026-11-01T20:00:00-05:00", eventLevelStartDate(t, env, ulid))

	assertAllDayRowsAreAnchored(t, env)
}

// eventLevelStartDate re-reads the event and returns its event-level startDate.
func eventLevelStartDate(t *testing.T, env *testEnv, ulid string) string {
	t.Helper()
	doc := getEventAsAgent(t, env, ulid)
	value, ok := doc["startDate"].(string)
	require.True(t, ok, "event must expose a startDate")
	return value
}

// TestAdminPutOccurrenceAllDayAnchorsToMidnight pins the per-occurrence PUT: a
// timed start_time sent with all_day:true must be stored as local midnight, so
// the stored instant and the date-only rendering agree.
func TestAdminPutOccurrenceAllDayAnchorsToMidnight(t *testing.T) {
	env := setupTestEnv(t)

	insertAdminUser(t, env, alldayTestAdminUser, alldayTestAdminPassword, "allday-admin@example.com", "admin")
	adminToken := adminLogin(t, env, alldayTestAdminUser, alldayTestAdminPassword)
	agentKey := insertAPIKey(t, env, "allday-agent-put")

	ulid := createEventAsAgent(t, env, agentKey, map[string]any{
		"name":        "Single Date Event",
		"description": "One timed occurrence to be marked all-day.",
		"startDate":   "2026-11-05T19:00:00-05:00",
		"occurrences": []map[string]any{
			{"startDate": "2026-11-05T19:00:00-05:00", "endDate": "2026-11-05T21:00:00-05:00"},
		},
		"location": map[string]any{
			"name":            "Hart House",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
	})

	subs := subEvents(t, getEventAsAgent(t, env, ulid))
	require.Len(t, subs, 1)
	occurrenceUUID := occurrenceUUIDFromSubEvent(t, subs[0])
	require.NotEmpty(t, occurrenceUUID)

	// all_day:true with a timed instant: the marker must not be stored next to
	// 19:30 — the row is anchored to local midnight instead.
	resp := putOccurrence(t, env, adminToken, ulid, occurrenceUUID, map[string]any{
		"all_day":    true,
		"start_time": "2026-11-05T19:30:00-05:00",
		"end_time":   "2026-11-05T23:45:00-05:00",
	})
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode, "PUT occurrence with all_day:true")

	var updated map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&updated))
	require.Equal(t, true, updated["all_day"])
	require.Equal(t, "2026-11-05T00:00:00-05:00", updated["start_time"],
		"stored instant must be local midnight in the occurrence zone")
	require.Equal(t, "2026-11-05T00:00:00-05:00", updated["end_time"],
		"end_time must be anchored with the marker")

	// The database row agrees with the response, and the date-only rendering
	// agrees with the stored instant.
	var localStart, localEnd string
	var isAllDay bool
	require.NoError(t, env.Pool.QueryRow(env.Context, `
		SELECT to_char(local_start_time, 'HH24:MI:SS'),
		       to_char((end_time AT TIME ZONE timezone)::time, 'HH24:MI:SS'),
		       is_all_day
		  FROM event_occurrences
		 WHERE id = $1::uuid
	`, occurrenceUUID).Scan(&localStart, &localEnd, &isAllDay))
	require.True(t, isAllDay, "is_all_day must be stored")
	require.Equal(t, "00:00:00", localStart)
	require.Equal(t, "00:00:00", localEnd)

	rendered := subEvents(t, getEventAsAgent(t, env, ulid))
	require.Len(t, rendered, 1)
	require.Equal(t, "2026-11-05", rendered[0]["startDate"], "date-only rendering must follow the anchored instant")
	require.Equal(t, true, rendered[0]["allDay"])

	assertAllDayRowsAreAnchored(t, env)

	// Retiming the row off local midnight without an explicit all_day clears the
	// stale marker: a row may not claim to be date-only while holding 19:30.
	// Both ends move together (an end before the new start is rejected by the
	// database's valid_end_time check).
	retime := putOccurrence(t, env, adminToken, ulid, occurrenceUUID, map[string]any{
		"start_time": "2026-11-06T19:30:00-05:00",
		"end_time":   "2026-11-06T21:00:00-05:00",
	})
	defer func() { _ = retime.Body.Close() }()
	if retime.StatusCode != http.StatusOK {
		var failure map[string]any
		_ = json.NewDecoder(retime.Body).Decode(&failure)
		require.Failf(t, "unexpected status", "PUT retime: status=%d response=%v", retime.StatusCode, failure)
	}

	var retimed map[string]any
	require.NoError(t, json.NewDecoder(retime.Body).Decode(&retimed))
	require.Equal(t, "2026-11-06T19:30:00-05:00", retimed["start_time"], "timed instant must be preserved verbatim")
	require.NotContains(t, retimed, "all_day", "a retimed row must not keep the date-only marker")

	renderedAfterRetime := subEvents(t, getEventAsAgent(t, env, ulid))
	require.Equal(t, "2026-11-06T19:30:00-05:00", renderedAfterRetime[0]["startDate"])
	require.NotContains(t, renderedAfterRetime[0], "allDay")

	assertAllDayRowsAreAnchored(t, env)
}

// TestAdminPutOccurrenceEndOnlyRetimeClearsMarker pins the end-only retime rule:
// moving only end_time off local midnight on an all-day row clears the marker and
// preserves the requested instant (it is not silently snapped back to midnight).
func TestAdminPutOccurrenceEndOnlyRetimeClearsMarker(t *testing.T) {
	env := setupTestEnv(t)

	insertAdminUser(t, env, alldayTestAdminUser, alldayTestAdminPassword, "allday-admin@example.com", "admin")
	adminToken := adminLogin(t, env, alldayTestAdminUser, alldayTestAdminPassword)
	agentKey := insertAPIKey(t, env, "allday-agent-end-only")

	ulid := createEventAsAgent(t, env, agentKey, map[string]any{
		"name":        "End-Only Retime Event",
		"description": "An all-day occurrence whose end is retimed off midnight.",
		"startDate":   "2026-11-05T19:00:00-05:00",
		"occurrences": []map[string]any{
			{"startDate": "2026-11-05T19:00:00-05:00", "endDate": "2026-11-05T21:00:00-05:00"},
		},
		"location": map[string]any{
			"name":            "Hart House",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
	})

	subs := subEvents(t, getEventAsAgent(t, env, ulid))
	require.Len(t, subs, 1)
	occurrenceUUID := occurrenceUUIDFromSubEvent(t, subs[0])

	// Make the row all-day (anchored to local midnight).
	resp := putOccurrence(t, env, adminToken, ulid, occurrenceUUID, map[string]any{
		"all_day":    true,
		"start_time": "2026-11-05T19:00:00-05:00",
		"end_time":   "2026-11-05T23:00:00-05:00",
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	// Retime only the end off midnight.
	retime := putOccurrence(t, env, adminToken, ulid, occurrenceUUID, map[string]any{
		"end_time": "2026-11-05T21:00:00-05:00",
	})
	defer func() { _ = retime.Body.Close() }()
	require.Equal(t, http.StatusOK, retime.StatusCode)

	var updated map[string]any
	require.NoError(t, json.NewDecoder(retime.Body).Decode(&updated))
	require.Equal(t, "2026-11-05T21:00:00-05:00", updated["end_time"],
		"a timed end must be preserved verbatim, not snapped to midnight")
	require.NotContains(t, updated, "all_day", "the all-day marker must be cleared by an end-only retime")

	_, _, endDate, endClock, isAllDay := occurrenceLocalWindow(t, env, occurrenceUUID)
	require.False(t, isAllDay, "is_all_day must be false after an end-only retime")
	require.Equal(t, "2026-11-05", endDate)
	require.Equal(t, "21:00:00", endClock)

	assertAllDayRowsAreAnchored(t, env)
}

// TestAdminFixReviewSingleAllDayEventRoundTrip keeps the single-occurrence
// all-day path honest end to end: the marker survives the review path AND the
// stored instant is local midnight.
func TestAdminFixReviewSingleAllDayEventRoundTrip(t *testing.T) {
	env := setupTestEnv(t)

	insertAdminUser(t, env, alldayTestAdminUser, alldayTestAdminPassword, "allday-admin@example.com", "admin")
	adminToken := adminLogin(t, env, alldayTestAdminUser, alldayTestAdminPassword)
	agentKey := insertAPIKey(t, env, "allday-agent-single")

	ulid := createEventAsAgent(t, env, agentKey, map[string]any{
		"name":        "One Day Festival",
		"description": "A date-only single-occurrence event.",
		"startDate":   "2026-11-07",
		"occurrences": []map[string]any{
			{"startDate": "2026-11-07"},
		},
		"location": map[string]any{
			"name":            "Trinity Bellwoods",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
	})

	subs := subEvents(t, getEventAsAgent(t, env, ulid))
	require.Len(t, subs, 1)
	require.Equal(t, "2026-11-07", subs[0]["startDate"])
	require.Equal(t, true, subs[0]["allDay"])

	reviewStart := time.Date(2026, 11, 7, 0, 0, 0, 0, time.FixedZone("EST", -5*3600))
	reviewID := insertPendingReviewForEvent(t, env, ulid, true, reviewStart)

	// The admin corrects the date with a timed value; because the event is
	// date-only the correction is re-anchored to midnight.
	correctedStart := time.Date(2026, 11, 8, 14, 0, 0, 0, time.FixedZone("EST", -5*3600))
	resp := postReviewFix(t, env, adminToken, reviewID, &correctedStart, nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		var failure map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&failure)
		require.Failf(t, "unexpected status", "fix review: status=%d response=%v", resp.StatusCode, failure)
	}

	rendered := subEvents(t, getEventAsAgent(t, env, ulid))
	require.Len(t, rendered, 1)
	require.Equal(t, "2026-11-08", rendered[0]["startDate"], "corrected date must be date-only")
	require.Equal(t, true, rendered[0]["allDay"], "marker must survive the review path")
	require.Equal(t, "2026-11-08", eventLevelStartDate(t, env, ulid))

	assertAllDayRowsAreAnchored(t, env)
}

// postOccurrence creates a new occurrence on an event as an admin.
func postOccurrence(t *testing.T, env *testEnv, adminToken, eventULID string, body map[string]any) *http.Response {
	t.Helper()

	payload, err := json.Marshal(body)
	require.NoError(t, err)

	url := fmt.Sprintf("%s/api/v1/admin/events/%s/occurrences", env.Server.URL, eventULID)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	return resp
}

// occurrenceLocalWindow reads the occurrence's local civil window straight from
// the database: local_date/local_start_time are generated columns (start_time AT
// TIME ZONE timezone), and the end is read the same way. This is the invariant
// expressed exactly as the schema sees it.
func occurrenceLocalWindow(t *testing.T, env *testEnv, occurrenceUUID string) (startDate, startClock, endDate, endClock string, isAllDay bool) {
	t.Helper()

	require.NoError(t, env.Pool.QueryRow(env.Context, `
		SELECT to_char(local_date, 'YYYY-MM-DD'),
		       to_char(local_start_time, 'HH24:MI:SS'),
		       to_char((end_time AT TIME ZONE timezone)::date, 'YYYY-MM-DD'),
		       to_char((end_time AT TIME ZONE timezone)::time, 'HH24:MI:SS'),
		       is_all_day
		  FROM event_occurrences
		 WHERE id = $1::uuid
	`, occurrenceUUID).Scan(&startDate, &startClock, &endDate, &endClock, &isAllDay))
	return startDate, startClock, endDate, endClock, isAllDay
}

// newParisAllDayOccurrence creates a single-occurrence event and re-anchors it
// as a correctly-anchored all-day row in Europe/Paris (a positive-offset zone)
// on the given civil date. It returns the event ULID and the occurrence UUID.
// The name and date must be unique per caller to avoid the ingest near-duplicate
// detector flagging sibling test events.
func newParisAllDayOccurrence(t *testing.T, env *testEnv, adminToken, agentKey, name, date string) (string, string) {
	t.Helper()

	ulid := createEventAsAgent(t, env, agentKey, map[string]any{
		"name":        name,
		"description": "A single all-day occurrence anchored in Europe/Paris.",
		"startDate":   date + "T19:00:00-05:00",
		"occurrences": []map[string]any{
			{"startDate": date + "T19:00:00-05:00", "endDate": date + "T21:00:00-05:00"},
		},
		"location": map[string]any{
			"name":            "Le Marais " + name,
			"addressLocality": "Paris",
			"addressRegion":   "IDF",
		},
	})

	subs := subEvents(t, getEventAsAgent(t, env, ulid))
	require.Len(t, subs, 1)
	occurrenceUUID := occurrenceUUIDFromSubEvent(t, subs[0])

	resp := putOccurrence(t, env, adminToken, ulid, occurrenceUUID, map[string]any{
		"all_day":    true,
		"timezone":   "Europe/Paris",
		"start_time": date + "T19:00:00+01:00",
		"end_time":   date + "T23:00:00+01:00",
	})
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode, "anchor Paris all-day row")

	var updated map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&updated))
	require.Equal(t, date+"T00:00:00+01:00", updated["start_time"])
	require.Equal(t, date+"T00:00:00+01:00", updated["end_time"])
	return ulid, occurrenceUUID
}

// TestAdminPostOccurrenceAllDayAnchorsToMidnight pins the POST create writer:
// all_day:true with a timed instant must store local midnight in the
// occurrence's zone, so the stored instant and the date-only rendering agree.
func TestAdminPostOccurrenceAllDayAnchorsToMidnight(t *testing.T) {
	env := setupTestEnv(t)

	insertAdminUser(t, env, alldayTestAdminUser, alldayTestAdminPassword, "allday-admin@example.com", "admin")
	adminToken := adminLogin(t, env, alldayTestAdminUser, alldayTestAdminPassword)
	agentKey := insertAPIKey(t, env, "allday-agent-post")

	ulid := createEventAsAgent(t, env, agentKey, map[string]any{
		"name":        "POST All-Day Event",
		"description": "A base event to POST an all-day occurrence onto.",
		"startDate":   "2026-11-05T19:00:00-05:00",
		"occurrences": []map[string]any{
			{"startDate": "2026-11-05T19:00:00-05:00", "endDate": "2026-11-05T21:00:00-05:00"},
		},
		"location": map[string]any{
			"name":            "The Rex",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
	})

	resp := postOccurrence(t, env, adminToken, ulid, map[string]any{
		"all_day":    true,
		"timezone":   "America/Toronto",
		"start_time": "2026-11-07T19:00:00-05:00",
		"end_time":   "2026-11-07T21:00:00-05:00",
	})
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusCreated, resp.StatusCode, "POST occurrence with all_day:true")

	var created map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	require.Equal(t, true, created["all_day"])
	require.Equal(t, "2026-11-07T00:00:00-05:00", created["start_time"],
		"POST all_day:true must store local midnight, not the timed instant")
	require.Equal(t, "2026-11-07T00:00:00-05:00", created["end_time"])

	occUUID, ok := created["id"].(string)
	require.True(t, ok, "POST response must include the occurrence id")

	startDate, startClock, _, _, isAllDay := occurrenceLocalWindow(t, env, occUUID)
	require.Equal(t, "2026-11-07", startDate, "stored row must be anchored to the requested civil date")
	require.Equal(t, "00:00:00", startClock, "stored row must be local midnight")
	require.True(t, isAllDay)

	assertAllDayRowsAreAnchored(t, env)
}

// TestAdminAllDayInvariantPositiveOffsetZone pins the F1 fix in a
// positive-offset zone (Europe/Paris). A DB-read instant carries the server's
// local zone, not the occurrence's zone, so the civil date must come from the
// occurrence's own zone — otherwise an unrelated PUT or a single-sided
// correction would silently shift the date a day.
func TestAdminAllDayInvariantPositiveOffsetZone(t *testing.T) {
	env := setupTestEnv(t)

	insertAdminUser(t, env, alldayTestAdminUser, alldayTestAdminPassword, "allday-admin@example.com", "admin")
	adminToken := adminLogin(t, env, alldayTestAdminUser, alldayTestAdminPassword)
	agentKey := insertAPIKey(t, env, "allday-agent-paris")

	t.Run("unrelated PUT leaves a correctly-anchored Paris row unchanged", func(t *testing.T) {
		ulid, occurrenceUUID := newParisAllDayOccurrence(t, env, adminToken, agentKey, "Paris All-Day A", "2026-11-06")

		resp := putOccurrence(t, env, adminToken, ulid, occurrenceUUID, map[string]any{
			"ticket_url": "https://paris.example/tickets",
		})
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		startDate, startClock, endDate, endClock, isAllDay := occurrenceLocalWindow(t, env, occurrenceUUID)
		require.Equal(t, "2026-11-06", startDate, "an unrelated PUT must not shift the civil date")
		require.Equal(t, "00:00:00", startClock)
		require.Equal(t, "2026-11-06", endDate, "an unrelated PUT must not shift the end")
		require.Equal(t, "00:00:00", endClock)
		require.True(t, isAllDay)
	})

	t.Run("correcting only the start must not shift the end", func(t *testing.T) {
		ulid, occurrenceUUID := newParisAllDayOccurrence(t, env, adminToken, agentKey, "Paris All-Day B", "2026-11-07")

		resp := putOccurrence(t, env, adminToken, ulid, occurrenceUUID, map[string]any{
			"all_day":    true,
			"start_time": "2026-11-06T14:00:00+01:00",
		})
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		startDate, startClock, endDate, endClock, isAllDay := occurrenceLocalWindow(t, env, occurrenceUUID)
		require.Equal(t, "2026-11-06", startDate, "correcting only the start must move the start")
		require.Equal(t, "00:00:00", startClock)
		require.Equal(t, "2026-11-07", endDate, "correcting only the start must not shift the end")
		require.Equal(t, "00:00:00", endClock)
		require.True(t, isAllDay)
	})

	t.Run("correcting only the end must not shift the start", func(t *testing.T) {
		ulid, occurrenceUUID := newParisAllDayOccurrence(t, env, adminToken, agentKey, "Paris All-Day C", "2026-11-08")

		resp := putOccurrence(t, env, adminToken, ulid, occurrenceUUID, map[string]any{
			"all_day":  true,
			"end_time": "2026-11-09T14:00:00+01:00",
		})
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		startDate, startClock, endDate, endClock, isAllDay := occurrenceLocalWindow(t, env, occurrenceUUID)
		require.Equal(t, "2026-11-08", startDate, "correcting only the end must not shift the start")
		require.Equal(t, "00:00:00", startClock)
		require.Equal(t, "2026-11-09", endDate, "correcting only the end must move the end")
		require.Equal(t, "00:00:00", endClock)
		require.True(t, isAllDay)
	})

	t.Run("tz change re-anchors preserving the civil date", func(t *testing.T) {
		// January keeps every zone in standard time, so the host Go tzdata and
		// the Postgres tzdata agree on the offset (no DST-boundary ambiguity).
		ulid, occurrenceUUID := newParisAllDayOccurrence(t, env, adminToken, agentKey, "Paris All-Day D", "2026-01-10")

		resp := putOccurrence(t, env, adminToken, ulid, occurrenceUUID, map[string]any{
			"timezone": "America/Vancouver",
		})
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		startDate, startClock, _, _, isAllDay := occurrenceLocalWindow(t, env, occurrenceUUID)
		require.Equal(t, "2026-01-10", startDate, "the civil date must survive the zone change")
		require.Equal(t, "00:00:00", startClock)
		require.True(t, isAllDay)
	})
}
