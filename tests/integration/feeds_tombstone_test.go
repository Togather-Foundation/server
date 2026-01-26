package integration

import (
	"encoding/json"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestFeedsTombstoneInChangeFeed verifies that deleted events appear in the change feed with tombstone data
func TestFeedsTombstoneInChangeFeed(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Tombstone Test Org")
	place := insertPlace(t, env, "Tombstone Test Venue", "Toronto")

	// Create an event
	eventULID := insertEventWithOccurrence(t, env, "Event to Delete", org.ID, place.ID, "music", "published", []string{"test"}, time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC))
	eventID := lookupEventIDByULID(t, env, eventULID)

	// Get initial change feed state
	initialResp := fetchChangeFeed(t, env, url.Values{"limit": {"100"}})
	initialCount := len(initialResp.Items)

	// Soft delete the event (mark as deleted)
	softDeleteEvent(t, env, eventID)

	// Record delete action in event_changes
	recordDeleteChange(t, env, eventID)

	t.Run("delete action appears in change feed", func(t *testing.T) {
		resp := fetchChangeFeed(t, env, url.Values{"limit": {"100"}})

		// Should have one more change event
		require.Greater(t, len(resp.Items), initialCount, "should have new change event after delete")

		// Find the delete event
		var foundDelete bool
		for _, item := range resp.Items {
			if item.EventID == eventID && item.Action == "delete" {
				foundDelete = true

				// Verify delete event properties
				require.Equal(t, "delete", item.Action)
				require.NotEmpty(t, item.ChangedAt, "should have changed_at timestamp")
				require.Greater(t, item.SequenceNumber, int64(0), "should have valid sequence number")

				break
			}
		}
		require.True(t, foundDelete, "should find delete action in change feed")
	})

	t.Run("filter for delete actions only", func(t *testing.T) {
		params := url.Values{}
		params.Set("action", "delete")

		resp := fetchChangeFeed(t, env, params)

		// All returned items should be delete actions
		for _, item := range resp.Items {
			require.Equal(t, "delete", item.Action)
		}

		// Should find our deleted event
		var foundOurDelete bool
		for _, item := range resp.Items {
			if item.EventID == eventID {
				foundOurDelete = true
				break
			}
		}
		require.True(t, foundOurDelete, "should find our deleted event when filtering by action=delete")
	})

	t.Run("tombstone snapshot includes deletion metadata", func(t *testing.T) {
		params := url.Values{}
		params.Set("include_snapshot", "true")
		params.Set("action", "delete")

		resp := fetchChangeFeed(t, env, params)

		// Find our deleted event
		var foundWithSnapshot bool
		for _, item := range resp.Items {
			if item.EventID == eventID && item.Snapshot != nil {
				foundWithSnapshot = true

				// Verify tombstone metadata in snapshot
				require.NotNil(t, item.Snapshot, "delete event should have snapshot")

				// Tombstone should indicate deleted status
				if deletedAt, ok := item.Snapshot["deletedAt"].(string); ok {
					parsedTime, err := time.Parse(time.RFC3339, deletedAt)
					require.NoError(t, err, "deletedAt should be valid RFC3339 timestamp")
					require.False(t, parsedTime.IsZero(), "deletedAt should not be zero time")
				}

				break
			}
		}
		require.True(t, foundWithSnapshot, "should find deleted event with snapshot")
	})
}

// TestFeedsTombstonePayload verifies tombstone table entries
func TestFeedsTombstonePayload(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Tombstone Payload Org")
	place := insertPlace(t, env, "Tombstone Payload Venue", "Toronto")

	// Create an event
	eventULID := insertEventWithOccurrence(t, env, "Event for Tombstone Test", org.ID, place.ID, "arts", "published", []string{"tombstone"}, time.Date(2026, 7, 15, 19, 0, 0, 0, time.UTC))
	eventID := lookupEventIDByULID(t, env, eventULID)

	// Construct event URI
	eventURI := "http://localhost/events/" + eventULID

	// Delete event and create tombstone
	softDeleteEvent(t, env, eventID)
	tombstoneID := createEventTombstone(t, env, eventID, eventURI, "Testing tombstone functionality")

	t.Run("tombstone entry exists in database", func(t *testing.T) {
		var exists bool
		err := env.Pool.QueryRow(env.Context,
			`SELECT EXISTS(SELECT 1 FROM event_tombstones WHERE id = $1)`,
			tombstoneID,
		).Scan(&exists)
		require.NoError(t, err)
		require.True(t, exists, "tombstone should exist in event_tombstones table")
	})

	t.Run("tombstone contains event metadata", func(t *testing.T) {
		var storedEventID, storedURI, deletionReason string
		var deletedAt time.Time
		var payload []byte

		err := env.Pool.QueryRow(env.Context,
			`SELECT event_id, event_uri, deleted_at, deletion_reason, payload
			 FROM event_tombstones WHERE id = $1`,
			tombstoneID,
		).Scan(&storedEventID, &storedURI, &deletedAt, &deletionReason, &payload)
		require.NoError(t, err)

		require.Equal(t, eventID, storedEventID, "tombstone should reference correct event_id")
		require.Equal(t, eventURI, storedURI, "tombstone should have correct event_uri")
		require.NotZero(t, deletedAt, "tombstone should have deleted_at timestamp")
		require.Equal(t, "Testing tombstone functionality", deletionReason)
		require.NotEmpty(t, payload, "tombstone should have JSON payload")

		// Verify payload is valid JSON
		var payloadData map[string]any
		require.NoError(t, json.Unmarshal(payload, &payloadData))
		require.NotEmpty(t, payloadData, "tombstone payload should be non-empty JSON")
	})

	t.Run("tombstone includes superseded_by_uri if provided", func(t *testing.T) {
		// Create another event
		newEventULID := insertEventWithOccurrence(t, env, "Replacement Event", org.ID, place.ID, "arts", "published", []string{"replacement"}, time.Date(2026, 7, 20, 19, 0, 0, 0, time.UTC))
		newEventID := lookupEventIDByULID(t, env, newEventULID)
		newEventURI := "http://localhost/events/" + newEventULID

		// Delete original event and create tombstone with superseded_by
		softDeleteEvent(t, env, newEventID)
		tombstoneWithSupersededID := createEventTombstoneWithSupersededBy(t, env, newEventID, newEventURI, "Replaced by new event", eventURI)

		var supersededByURI *string
		err := env.Pool.QueryRow(env.Context,
			`SELECT superseded_by_uri FROM event_tombstones WHERE id = $1`,
			tombstoneWithSupersededID,
		).Scan(&supersededByURI)
		require.NoError(t, err)
		require.NotNil(t, supersededByURI, "superseded_by_uri should be set")
		require.Equal(t, eventURI, *supersededByURI, "superseded_by_uri should match provided value")
	})
}

// TestFeedsTombstoneOrdering verifies tombstones appear in correct sequence
func TestFeedsTombstoneOrdering(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Ordering Test Org")
	place := insertPlace(t, env, "Ordering Test Venue", "Toronto")

	// Create multiple events
	event1ULID := insertEventWithOccurrence(t, env, "Event 1", org.ID, place.ID, "music", "published", []string{"test"}, time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC))
	time.Sleep(10 * time.Millisecond)

	event2ULID := insertEventWithOccurrence(t, env, "Event 2", org.ID, place.ID, "arts", "published", []string{"test"}, time.Date(2026, 7, 15, 19, 0, 0, 0, time.UTC))
	time.Sleep(10 * time.Millisecond)

	event3ULID := insertEventWithOccurrence(t, env, "Event 3", org.ID, place.ID, "culture", "published", []string{"test"}, time.Date(2026, 7, 20, 19, 0, 0, 0, time.UTC))

	event1ID := lookupEventIDByULID(t, env, event1ULID)
	event2ID := lookupEventIDByULID(t, env, event2ULID)
	event3ID := lookupEventIDByULID(t, env, event3ULID)

	// Delete in specific order: 2, 1, 3
	softDeleteEvent(t, env, event2ID)
	recordDeleteChange(t, env, event2ID)
	time.Sleep(10 * time.Millisecond)

	softDeleteEvent(t, env, event1ID)
	recordDeleteChange(t, env, event1ID)
	time.Sleep(10 * time.Millisecond)

	softDeleteEvent(t, env, event3ID)
	recordDeleteChange(t, env, event3ID)

	t.Run("deletions appear in deletion order", func(t *testing.T) {
		params := url.Values{}
		params.Set("action", "delete")

		resp := fetchChangeFeed(t, env, params)
		require.GreaterOrEqual(t, len(resp.Items), 3, "should have at least 3 delete events")

		// Find our delete events and verify order
		var foundSequence []string
		for _, item := range resp.Items {
			if item.EventID == event1ID || item.EventID == event2ID || item.EventID == event3ID {
				foundSequence = append(foundSequence, item.EventID)
			}
		}

		require.Len(t, foundSequence, 3, "should find all 3 delete events")
		require.Equal(t, event2ID, foundSequence[0], "event2 should be deleted first")
		require.Equal(t, event1ID, foundSequence[1], "event1 should be deleted second")
		require.Equal(t, event3ID, foundSequence[2], "event3 should be deleted third")
	})

	t.Run("tombstones have monotonic sequence numbers", func(t *testing.T) {
		params := url.Values{}
		params.Set("action", "delete")

		resp := fetchChangeFeed(t, env, params)

		var deleteSequences []int64
		for _, item := range resp.Items {
			if item.EventID == event1ID || item.EventID == event2ID || item.EventID == event3ID {
				deleteSequences = append(deleteSequences, item.SequenceNumber)
			}
		}

		require.Len(t, deleteSequences, 3)

		// Verify monotonic increase
		require.Less(t, deleteSequences[0], deleteSequences[1], "sequence numbers should increase")
		require.Less(t, deleteSequences[1], deleteSequences[2], "sequence numbers should increase")
	})
}

// Helper functions

// softDeleteEvent marks an event as deleted
func softDeleteEvent(t *testing.T, env *testEnv, eventID string) {
	t.Helper()

	_, err := env.Pool.Exec(env.Context,
		`UPDATE events SET deleted_at = now(), lifecycle_state = 'deleted' WHERE id = $1`,
		eventID,
	)
	require.NoError(t, err, "should successfully soft delete event")
}

// recordDeleteChange inserts a delete action into event_changes
func recordDeleteChange(t *testing.T, env *testEnv, eventID string) {
	t.Helper()

	_, err := env.Pool.Exec(env.Context,
		`INSERT INTO event_changes (event_id, action, changed_at)
		 VALUES ($1, 'delete', now())`,
		eventID,
	)
	require.NoError(t, err, "should successfully record delete change")
}

// createEventTombstone creates a tombstone record
func createEventTombstone(t *testing.T, env *testEnv, eventID, eventURI, deletionReason string) string {
	t.Helper()

	// Fetch event data for tombstone payload
	var eventName string
	err := env.Pool.QueryRow(env.Context,
		`SELECT name FROM events WHERE id = $1`,
		eventID,
	).Scan(&eventName)
	require.NoError(t, err)

	// Create minimal tombstone payload
	payload := map[string]any{
		"@type":     "Event",
		"@id":       eventURI,
		"name":      eventName,
		"deletedAt": time.Now().UTC().Format(time.RFC3339),
	}
	payloadJSON, err := json.Marshal(payload)
	require.NoError(t, err)

	var tombstoneID string
	err = env.Pool.QueryRow(env.Context,
		`INSERT INTO event_tombstones (event_id, event_uri, deleted_at, deletion_reason, payload)
		 VALUES ($1, $2, now(), $3, $4)
		 RETURNING id`,
		eventID, eventURI, deletionReason, payloadJSON,
	).Scan(&tombstoneID)
	require.NoError(t, err, "should successfully create tombstone")

	return tombstoneID
}

// createEventTombstoneWithSupersededBy creates a tombstone with superseded_by_uri
func createEventTombstoneWithSupersededBy(t *testing.T, env *testEnv, eventID, eventURI, deletionReason, supersededByURI string) string {
	t.Helper()

	// Fetch event data for tombstone payload
	var eventName string
	err := env.Pool.QueryRow(env.Context,
		`SELECT name FROM events WHERE id = $1`,
		eventID,
	).Scan(&eventName)
	require.NoError(t, err)

	// Create tombstone payload with superseded_by reference
	payload := map[string]any{
		"@type":        "Event",
		"@id":          eventURI,
		"name":         eventName,
		"deletedAt":    time.Now().UTC().Format(time.RFC3339),
		"supersededBy": supersededByURI,
	}
	payloadJSON, err := json.Marshal(payload)
	require.NoError(t, err)

	var tombstoneID string
	err = env.Pool.QueryRow(env.Context,
		`INSERT INTO event_tombstones (event_id, event_uri, deleted_at, deletion_reason, superseded_by_uri, payload)
		 VALUES ($1, $2, now(), $3, $4, $5)
		 RETURNING id`,
		eventID, eventURI, deletionReason, supersededByURI, payloadJSON,
	).Scan(&tombstoneID)
	require.NoError(t, err, "should successfully create tombstone with superseded_by")

	return tombstoneID
}
