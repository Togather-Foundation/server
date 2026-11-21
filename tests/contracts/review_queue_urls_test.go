package contracts_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Togather-Foundation/server/tests/testhelpers"
	"github.com/stretchr/testify/require"
)

func TestReviewQueueListPartialURLs(t *testing.T) {
	env, token := setupAdminEnv(t)
	org := insertOrganization(t, env, "Community Arts Org")
	place := insertPlace(t, env, "Community Hall", "Toronto")

	const listingURL = "https://venue.example.org/events/community-arts"
	const legacyBaseURL = "https://organizer.example.org/calendar"
	var legacySourceID string
	err := env.Pool.QueryRow(env.Context, `
		INSERT INTO sources (name, source_type, base_url, license_url, license_type)
		VALUES ('Legacy calendar', 'scraper', $1, 'https://creativecommons.org/publicdomain/zero/1.0/', 'CC0')
		RETURNING id::text`, legacyBaseURL).Scan(&legacySourceID)
	require.NoError(t, err)

	testCases := []struct {
		name     string
		eventURL string
		sourceID string
	}{
		{name: "Community Arts", eventURL: listingURL},
		{name: "Maker Fair", sourceID: legacySourceID},
	}
	var eventULIDs []string
	for i, tc := range testCases {
		start := testhelpers.FutureTime(i + 1)
		eventULID := insertEventWithOccurrence(t, env, tc.name, org.ID, place.ID, "arts", "pending_review", nil, start)
		eventULIDs = append(eventULIDs, eventULID)

		var eventID string
		err := env.Pool.QueryRow(env.Context, `
			UPDATE events SET public_url = NULLIF($1, '') WHERE ulid = $2 RETURNING id`, tc.eventURL, eventULID).Scan(&eventID)
		require.NoError(t, err)
		insertReviewQueueEntry(t, env, eventID, `{}`, `{}`, `[]`, start)
		_, err = env.Pool.Exec(env.Context, `UPDATE event_review_queue SET source_id = NULLIF($1, '') WHERE event_id = $2`, tc.sourceID, eventID)
		require.NoError(t, err)
	}

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/admin/review-queue", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var payload struct {
		Items []map[string]any `json:"items"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	require.Len(t, payload.Items, 2)
	itemsByEvent := make(map[string]map[string]any)
	for _, item := range payload.Items {
		eventID, ok := item["eventId"].(string)
		require.True(t, ok)
		itemsByEvent[eventID] = item
	}

	listingOnly := itemsByEvent[eventULIDs[0]]
	require.Equal(t, listingURL, listingOnly["eventUrl"])
	require.NotContains(t, listingOnly, "sourceUrl")
	sourceOnly := itemsByEvent[eventULIDs[1]]
	require.Equal(t, legacyBaseURL, sourceOnly["sourceUrl"])
	require.NotContains(t, sourceOnly, "eventUrl")
}
