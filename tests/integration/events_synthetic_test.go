package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/api"
	"github.com/Togather-Foundation/server/tests/testhelpers"
	"github.com/stretchr/testify/require"
)

func syntheticEventFixture(start time.Time) map[string]any {
	return map[string]any{
		"name":        "Provenance Workshop",
		"description": "An afternoon workshop exploring how local catalogues record and share the sources of public event information.",
		"startDate":   start.Format(time.RFC3339),
		"location":    map[string]string{"name": "Provenance Hall", "addressLocality": "Toronto", "addressRegion": "ON"},
		"image":       "https://organizer.example.org/image.jpg",
		"source":      map[string]string{"url": "https://organizer.example.org/workshops", "eventId": "workshop"},
	}
}

func postSyntheticEventFixture(t *testing.T, server *httptest.Server, key string, payload map[string]any, wantStatus int) map[string]any {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/events", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/ld+json")
	req.Header.Set("Accept", "application/ld+json")
	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	require.Equal(t, wantStatus, resp.StatusCode, "%+v", result)
	return result
}

func TestSyntheticEventIdentityAndPublicReads(t *testing.T) {
	for _, tc := range []struct {
		name  string
		order []bool
	}{
		{name: "ordinary first with marker omitted", order: []bool{false, true}},
		{name: "synthetic first with explicit false comparator", order: []bool{true, false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := setupTestEnv(t)
			key := insertAPIKey(t, env, "synthetic-provenance")
			start := testhelpers.FutureTime(5)
			payloads := map[bool]map[string]any{false: syntheticEventFixture(start), true: syntheticEventFixture(start)}
			payloads[true]["sel:synthetic"] = true
			if tc.order[0] {
				payloads[false]["sel:synthetic"] = false
			}

			created := make(map[bool]string)
			for _, synthetic := range tc.order {
				created[synthetic] = createEventAsAgent(t, env, key, payloads[synthetic])
			}
			require.NotEqual(t, created[false], created[true])
			for _, synthetic := range tc.order {
				payload := payloads[synthetic]
				feedReplay := postSyntheticEventFixture(t, env.Server, key, payload, http.StatusConflict)
				require.Equal(t, created[synthetic], eventIDFromPayload(feedReplay))
				delete(payload, "source")
				hashReplay := postSyntheticEventFixture(t, env.Server, key, payload, http.StatusConflict)
				require.Equal(t, created[synthetic], eventIDFromPayload(hashReplay))
			}

			require.NotContains(t, getEventAsAgent(t, env, created[false]), "sel:synthetic")
			require.Equal(t, true, getEventAsAgent(t, env, created[true])["sel:synthetic"])
			query := url.Values{"q": {"Provenance Workshop"}}
			public := fetchEventsList(t, env, query)
			assertSyntheticListMarkers(t, public.Items, created[false], created[true])

			insertAdminUser(t, env, "admin", "password", "admin@test.com", "admin")
			token := adminLogin(t, env, "admin", "password")
			req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/admin/events?"+query.Encode(), nil)
			require.NoError(t, err)
			req.Header.Set("Authorization", "Bearer "+token)
			resp, err := env.Server.Client().Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()
			require.Equal(t, http.StatusOK, resp.StatusCode)
			var admin eventListResponse
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&admin))
			assertSyntheticListMarkers(t, admin.Items, created[false], created[true])
		})
	}
}

func assertSyntheticListMarkers(t *testing.T, items []map[string]any, ordinaryID, syntheticID string) {
	t.Helper()
	require.Len(t, items, 2)
	byID := make(map[string]map[string]any)
	for _, item := range items {
		byID[eventIDFromPayload(item)] = item
	}
	require.Contains(t, byID, ordinaryID)
	require.NotContains(t, byID[ordinaryID], "sel:synthetic")
	require.Equal(t, true, byID[syntheticID]["sel:synthetic"])
}

func TestSyntheticEventPendingReviewAndSeriesKeys(t *testing.T) {
	env := setupTestEnv(t)
	key := insertAPIKey(t, env, "synthetic-series")
	start := testhelpers.FutureTime(5)
	payload := syntheticEventFixture(start)
	payload["lifecycle_state"] = "review"
	payload["recurrence"] = map[string]any{
		"external_key": "workshops:weekly",
		"series_name":  "Provenance Workshop",
		"series_start": start.Format(time.RFC3339),
		"rrule":        "FREQ=WEEKLY;COUNT=2",
		"tzid":         "UTC",
	}

	ordinary := postSyntheticEventFixture(t, env.Server, key, payload, http.StatusAccepted)
	payload["sel:synthetic"] = true
	synthetic := postSyntheticEventFixture(t, env.Server, key, payload, http.StatusAccepted)
	ordinaryID, syntheticID := eventIDFromPayload(ordinary), eventIDFromPayload(synthetic)
	require.NotEmpty(t, ordinaryID)
	require.NotEmpty(t, syntheticID)
	require.NotEqual(t, ordinaryID, syntheticID)

	var seriesCount, reviewCount int
	err := env.Pool.QueryRow(env.Context, `
		SELECT count(DISTINCT s.id), count(DISTINCT r.id)
		  FROM events e
		  JOIN event_series s ON s.id = e.series_id
		  JOIN event_review_queue r ON r.event_id = e.id AND r.synthetic = e.synthetic
		 WHERE e.ulid IN ($1, $2) AND e.lifecycle_state = 'pending_review' AND r.status = 'pending'
		   AND CASE WHEN e.synthetic THEN s.synthetic_external_key ELSE s.external_key END = 'workshops:weekly'
		   AND r.source_external_id = 'workshop'`, ordinaryID, syntheticID).Scan(&seriesCount, &reviewCount)
	require.NoError(t, err)
	require.Equal(t, 2, seriesCount)
	require.Equal(t, 2, reviewCount)
}

func TestSyntheticEventRejectionIsolated(t *testing.T) {
	env := setupTestEnv(t)
	key := insertAPIKey(t, env, "synthetic-rejections")
	payload := syntheticEventFixture(testhelpers.FutureTimeAt(15, 18, 0))
	payload["endDate"] = testhelpers.FutureTimeAt(15, 14, 0).Format(time.RFC3339)
	rejected := postSyntheticEventFixture(t, env.Server, key, payload, http.StatusAccepted)
	rejectedID := eventIDFromPayload(rejected)
	require.NotEmpty(t, rejectedID)
	result, err := env.Pool.Exec(env.Context, `
		UPDATE event_review_queue SET status = 'rejected', reviewed_at = NOW(), rejection_reason = 'Bad dates'
		WHERE event_id = (SELECT id FROM events WHERE ulid = $1)`, rejectedID)
	require.NoError(t, err)
	require.EqualValues(t, 1, result.RowsAffected())

	// Reach the review lookup with the same source key after removing the direct attribution.
	_, err = env.Pool.Exec(env.Context, `DELETE FROM event_sources WHERE event_id = (SELECT id FROM events WHERE ulid = $1)`, rejectedID)
	require.NoError(t, err)
	payload["name"] = "Provenance Workshop Evening"
	problem := postSyntheticEventFixture(t, env.Server, key, payload, http.StatusBadRequest)
	require.Equal(t, "https://sel.events/problems/previously-rejected", problem["type"])

	payload["sel:synthetic"] = true
	queued := postSyntheticEventFixture(t, env.Server, key, payload, http.StatusAccepted)
	require.NotEmpty(t, eventIDFromPayload(queued))
	require.NotEqual(t, rejectedID, eventIDFromPayload(queued))
}

func TestSyntheticEventFuzzyReviewIsolation(t *testing.T) {
	for _, tc := range []struct {
		name      string
		otherDay  int
		otherName string
		warning   string
	}{
		{name: "near duplicate", otherDay: 5, otherName: "Provenance Workshop Session", warning: "potential_duplicate"},
		{name: "cross week companion", otherDay: 12, otherName: "Provenance Workshop", warning: "cross_week_series_companion"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := setupTestEnv(t)
			cfg := env.Config
			cfg.Dedup.NearDuplicateThreshold = 0.4
			server := httptest.NewServer(api.NewRouter(cfg, testhelpers.TestLogger(), env.Pool, "test", "test-commit", "test-date").Handler)
			t.Cleanup(server.Close)
			key := insertAPIKey(t, env, "synthetic-fuzzy")
			original := syntheticEventFixture(testhelpers.FutureTime(5))
			other := syntheticEventFixture(testhelpers.FutureTime(tc.otherDay))
			other["name"] = tc.otherName
			delete(original, "source")
			delete(other, "source")
			postSyntheticEventFixture(t, server, key, original, http.StatusCreated)

			other["sel:synthetic"] = true
			postSyntheticEventFixture(t, server, key, other, http.StatusCreated)
			var reviewCount int
			require.NoError(t, env.Pool.QueryRow(env.Context, `SELECT count(*) FROM event_review_queue`).Scan(&reviewCount))
			require.Zero(t, reviewCount)

			delete(other, "sel:synthetic")
			control := postSyntheticEventFixture(t, server, key, other, http.StatusAccepted)
			require.Contains(t, syntheticWarningCodes(t, control), tc.warning)
		})
	}
}

func syntheticWarningCodes(t *testing.T, response map[string]any) []string {
	t.Helper()
	warnings, ok := response["warnings"].([]any)
	require.True(t, ok)
	var codes []string
	for _, entry := range warnings {
		warning, ok := entry.(map[string]any)
		require.True(t, ok)
		code, ok := warning["code"].(string)
		require.True(t, ok)
		codes = append(codes, code)
	}
	return codes
}
