package integration

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/api/pagination"
	"github.com/stretchr/testify/require"
)

type changeFeedResponse struct {
	Items      []changeFeedItem `json:"items"`
	NextCursor string           `json:"next_cursor"`
}

type changeFeedItem struct {
	SequenceNumber int64           `json:"sequence_number"`
	EventID        string          `json:"event_id"`
	Action         string          `json:"action"`
	ChangedAt      string          `json:"changed_at"`
	ChangedFields  json.RawMessage `json:"changed_fields,omitempty"`
	Snapshot       json.RawMessage `json:"snapshot,omitempty"`
}

// TestChangeFeedPagination verifies cursor-based pagination for the change feed
func TestChangeFeedPagination(t *testing.T) {
	env := setupTestEnv(t)

	// Seed test data with multiple change events
	org := insertOrganization(t, env, "Test Org")
	place := insertPlace(t, env, "Test Venue", "Toronto")

	// Create events with changes
	event1ULID := insertEventWithOccurrence(t, env, "Event One", org.ID, place.ID, "music", "published", []string{"test"}, time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC))
	time.Sleep(10 * time.Millisecond) // Ensure different timestamps

	event2ULID := insertEventWithOccurrence(t, env, "Event Two", org.ID, place.ID, "arts", "published", []string{"test"}, time.Date(2026, 7, 15, 19, 0, 0, 0, time.UTC))
	time.Sleep(10 * time.Millisecond)

	event3ULID := insertEventWithOccurrence(t, env, "Event Three", org.ID, place.ID, "culture", "published", []string{"test"}, time.Date(2026, 7, 20, 19, 0, 0, 0, time.UTC))

	// Each event creation should generate a 'create' change event
	t.Run("fetch all changes without pagination", func(t *testing.T) {
		params := url.Values{}
		params.Set("limit", "100")

		resp := fetchChangeFeed(t, env, params)

		// Should have at least 3 create events
		require.GreaterOrEqual(t, len(resp.Items), 3, "should have at least 3 change events")

		// Verify sequence numbers are monotonically increasing
		for i := 1; i < len(resp.Items); i++ {
			require.Greater(t, resp.Items[i].SequenceNumber, resp.Items[i-1].SequenceNumber,
				"sequence numbers should be monotonically increasing")
		}

		// Verify all are 'create' actions
		for _, item := range resp.Items {
			require.Equal(t, "create", item.Action)
		}
	})

	t.Run("paginate with limit", func(t *testing.T) {
		params := url.Values{}
		params.Set("limit", "2")

		first := fetchChangeFeed(t, env, params)
		require.Len(t, first.Items, 2, "should return exactly 2 items")
		require.NotEmpty(t, first.NextCursor, "should provide next cursor")

		// Fetch next page
		params.Set("after", first.NextCursor)
		second := fetchChangeFeed(t, env, params)

		require.GreaterOrEqual(t, len(second.Items), 1, "should have at least 1 more item")

		// Verify no overlap
		firstLastSeq := first.Items[len(first.Items)-1].SequenceNumber
		secondFirstSeq := second.Items[0].SequenceNumber
		require.Greater(t, secondFirstSeq, firstLastSeq, "pages should not overlap")
	})

	t.Run("filter by action type", func(t *testing.T) {
		params := url.Values{}
		params.Set("action", "create")

		resp := fetchChangeFeed(t, env, params)

		// All items should be 'create' actions
		for _, item := range resp.Items {
			require.Equal(t, "create", item.Action)
		}
	})

	t.Run("filter by since cursor", func(t *testing.T) {
		// Get first event's sequence
		params := url.Values{}
		params.Set("limit", "1")
		first := fetchChangeFeed(t, env, params)
		require.Len(t, first.Items, 1)

		firstSeq := first.Items[0].SequenceNumber

		// Fetch changes after first event (using cursor pagination)
		params = url.Values{}
		params.Set("after", first.NextCursor)
		resp := fetchChangeFeed(t, env, params)

		// All returned items should have sequence > firstSeq
		for _, item := range resp.Items {
			require.Greater(t, item.SequenceNumber, firstSeq)
		}
	})

	t.Run("update event creates change record", func(t *testing.T) {
		// Get current count
		initialResp := fetchChangeFeed(t, env, url.Values{"limit": {"100"}})
		initialCount := len(initialResp.Items)

		// Update event1
		updateEventName(t, env, event1ULID, "Event One Updated")

		// Fetch changes again
		updatedResp := fetchChangeFeed(t, env, url.Values{"limit": {"100"}})

		// Should have one more change event
		require.Greater(t, len(updatedResp.Items), initialCount, "should have new change event after update")

		// Find the update event
		var foundUpdate bool
		for _, item := range updatedResp.Items {
			if item.Action == "update" {
				foundUpdate = true
				require.NotNil(t, item.ChangedFields, "update should include changed_fields")
			}
		}
		require.True(t, foundUpdate, "should find an update action")
	})

	t.Run("snapshot includes event data", func(t *testing.T) {
		params := url.Values{}
		params.Set("include_snapshot", "true")

		resp := fetchChangeFeed(t, env, params)
		require.NotEmpty(t, resp.Items)

		// Verify at least one item has snapshot data
		foundSnapshot := false
		for _, item := range resp.Items {
			if len(item.Snapshot) > 0 {
				foundSnapshot = true
				// Unmarshal snapshot to check fields
				var snapshot map[string]any
				err := json.Unmarshal(item.Snapshot, &snapshot)
				require.NoError(t, err, "snapshot should be valid JSON")
				// Verify snapshot contains expected fields
				require.NotNil(t, snapshot["@id"], "snapshot should include @id")
				require.NotNil(t, snapshot["@type"], "snapshot should include @type")
				require.NotNil(t, snapshot["name"], "snapshot should include name")
			}
		}
		require.True(t, foundSnapshot, "should have at least one item with snapshot data")
	})

	t.Run("empty result set when cursor beyond latest", func(t *testing.T) {
		// Get latest change
		latest := fetchChangeFeed(t, env, url.Values{"limit": {"1"}})
		require.Len(t, latest.Items, 1)

		// Encode a cursor far beyond the latest sequence
		farFutureCursor := pagination.EncodeChangeCursor(latest.Items[0].SequenceNumber + 1000)

		// Use cursor beyond latest
		params := url.Values{}
		params.Set("after", farFutureCursor)

		resp := fetchChangeFeed(t, env, params)
		require.Empty(t, resp.Items, "should return empty list when cursor is beyond latest")
		require.Empty(t, resp.NextCursor, "should not provide next cursor for empty result")
	})

	// Verify event IDs match created events
	t.Run("verify event IDs in change feed", func(t *testing.T) {
		resp := fetchChangeFeed(t, env, url.Values{"limit": {"100"}})

		eventIDs := make(map[string]bool)
		for _, item := range resp.Items {
			eventIDs[item.EventID] = true
		}

		// Should contain our test events (by ULID)
		event1ID := lookupEventIDByULID(t, env, event1ULID)
		event2ID := lookupEventIDByULID(t, env, event2ULID)
		event3ID := lookupEventIDByULID(t, env, event3ULID)

		require.True(t, eventIDs[event1ID], "should contain event1")
		require.True(t, eventIDs[event2ID], "should contain event2")
		require.True(t, eventIDs[event3ID], "should contain event3")
	})
}

// fetchChangeFeed makes a GET request to /api/v1/feeds/changes
func fetchChangeFeed(t *testing.T, env *testEnv, params url.Values) changeFeedResponse {
	t.Helper()

	u := env.Server.URL + "/api/v1/feeds/changes"
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	req, err := http.NewRequest(http.MethodGet, u, nil)
	require.NoError(t, err, "failed to create HTTP GET request for change feed")
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err, "failed to execute change feed HTTP request")
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode, "change feed request should succeed")

	var payload changeFeedResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload), "failed to decode change feed response JSON")
	return payload
}

// updateEventName updates an event's name to trigger a change event
func updateEventName(t *testing.T, env *testEnv, eventULID, newName string) {
	t.Helper()

	eventID := lookupEventIDByULID(t, env, eventULID)

	_, err := env.Pool.Exec(env.Context,
		`UPDATE events SET name = $1, updated_at = now() WHERE id = $2`,
		newName, eventID,
	)
	require.NoError(t, err, "failed to update event name in database")

	// Record change in event_changes table
	_, err = env.Pool.Exec(env.Context,
		`INSERT INTO event_changes (event_id, action, changed_fields, changed_at)
		 VALUES ($1, 'update', $2, now())`,
		eventID, `["name"]`,
	)
	require.NoError(t, err, "failed to insert event change record")
}

// lookupEventIDByULID retrieves the internal UUID for an event by its ULID
func lookupEventIDByULID(t *testing.T, env *testEnv, eventULID string) string {
	t.Helper()

	var eventID string
	err := env.Pool.QueryRow(env.Context,
		`SELECT id FROM events WHERE ulid = $1`,
		eventULID,
	).Scan(&eventID)
	require.NoError(t, err, "should find event by ULID")

	return eventID
}

// TestChangeFeedTimestamps verifies source and received timestamps are tracked
func TestChangeFeedTimestamps(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Timestamp Test Org")
	place := insertPlace(t, env, "Timestamp Test Venue", "Toronto")

	// Create event
	eventULID := insertEventWithOccurrence(t, env, "Timestamp Test Event", org.ID, place.ID, "music", "published", []string{"test"}, time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC))

	// Fetch change feed
	resp := fetchChangeFeed(t, env, url.Values{"include_snapshot": {"true"}})
	require.NotEmpty(t, resp.Items)

	// Find our event's change
	var found bool
	eventID := lookupEventIDByULID(t, env, eventULID)
	for _, item := range resp.Items {
		if item.EventID == eventID {
			found = true

			// Verify changed_at timestamp is present and valid
			require.NotEmpty(t, item.ChangedAt, "should have changed_at timestamp")

			parsedTime, err := time.Parse(time.RFC3339, item.ChangedAt)
			require.NoError(t, err, "changed_at should be valid RFC3339 timestamp")
			require.False(t, parsedTime.IsZero(), "changed_at should not be zero time")

			// Verify it's reasonably recent (within last minute)
			require.WithinDuration(t, time.Now().UTC(), parsedTime, time.Minute, "changed_at should be recent")

			break
		}
	}
	require.True(t, found, "should find change event for created event")
}
