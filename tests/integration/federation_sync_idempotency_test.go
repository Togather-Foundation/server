package integration

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestFederationSyncIdempotency verifies idempotent behavior for federation sync
func TestFederationSyncIdempotency(t *testing.T) {
	env := setupTestEnv(t)

	// Create federation node and API key
	_ = insertFederationNode(t, env, "idempotent.example.org", "Idempotent Node", "https://idempotent.example.org", "active", 8)
	apiKey := insertAPIKey(t, env, "federation-idempotent-key")

	eventPayload := map[string]any{
		"@context":  "https://togather.foundation/contexts/sel/v0.1.jsonld",
		"@type":     "Event",
		"@id":       "https://idempotent.example.org/events/01HYX3KQW7ERTV9XNBM2P8QJZF",
		"name":      "Idempotent Test Event",
		"startDate": time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"location": map[string]any{
			"@type":           "Place",
			"name":            "Test Venue",
			"addressLocality": "Toronto",
		},
	}

	t.Run("first submission creates event", func(t *testing.T) {
		resp := postFederationSync(t, env, apiKey, eventPayload)
		require.Equal(t, http.StatusCreated, resp.StatusCode, "first submission should create event")

		// Verify event was created
		eventExists := checkFederatedEventExists(t, env, "https://idempotent.example.org/events/01HYX3KQW7ERTV9XNBM2P8QJZF")
		require.True(t, eventExists, "event should exist after creation")
	})

	t.Run("duplicate submission is idempotent", func(t *testing.T) {
		// Submit the same event again
		resp := postFederationSync(t, env, apiKey, eventPayload)
		require.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated,
			"duplicate submission should be accepted (got %d)", resp.StatusCode)

		// Event should still exist (not duplicated)
		eventCount := countFederatedEvents(t, env, "https://idempotent.example.org/events/01HYX3KQW7ERTV9XNBM2P8QJZF")
		require.Equal(t, 1, eventCount, "should not create duplicate events")
	})

	t.Run("updated event modifies existing", func(t *testing.T) {
		// Modify the event payload
		updatedPayload := make(map[string]any)
		for k, v := range eventPayload {
			updatedPayload[k] = v
		}
		updatedPayload["name"] = "Idempotent Test Event - Updated"
		updatedPayload["description"] = "This event has been updated"

		resp := postFederationSync(t, env, apiKey, updatedPayload)
		require.True(t, resp.StatusCode >= 200 && resp.StatusCode < 300,
			"updated event should be accepted (got %d)", resp.StatusCode)

		// Verify update was applied
		eventName := getFederatedEventName(t, env, "https://idempotent.example.org/events/01HYX3KQW7ERTV9XNBM2P8QJZF")
		require.Equal(t, "Idempotent Test Event - Updated", eventName, "event name should be updated")

		// Still only one event
		eventCount := countFederatedEvents(t, env, "https://idempotent.example.org/events/01HYX3KQW7ERTV9XNBM2P8QJZF")
		require.Equal(t, 1, eventCount, "should still have only one event after update")
	})

	t.Run("concurrent duplicate submissions handled", func(t *testing.T) {
		// Create a new event for concurrent test
		concurrentPayload := map[string]any{
			"@context":  "https://togather.foundation/contexts/sel/v0.1.jsonld",
			"@type":     "Event",
			"@id":       "https://idempotent.example.org/events/CONCURRENT_EVENT_123",
			"name":      "Concurrent Test Event",
			"startDate": time.Date(2026, 7, 15, 19, 0, 0, 0, time.UTC).Format(time.RFC3339),
		}

		// Submit concurrently (simulated with rapid sequential submissions)
		responses := make([]*http.Response, 3)
		for i := 0; i < 3; i++ {
			responses[i] = postFederationSync(t, env, apiKey, concurrentPayload)
		}

		// All should succeed (one create, others idempotent)
		for i, resp := range responses {
			require.True(t, resp.StatusCode >= 200 && resp.StatusCode < 300,
				"concurrent submission %d should succeed (got %d)", i, resp.StatusCode)
		}

		// Should have exactly one event
		eventCount := countFederatedEvents(t, env, "https://idempotent.example.org/events/CONCURRENT_EVENT_123")
		require.Equal(t, 1, eventCount, "concurrent submissions should not create duplicates")
	})

	t.Run("idempotency respects origin URI", func(t *testing.T) {
		// Same ULID but different origin should create separate events
		ulid := "01HYX3KQW7ERTV9XNBM2P8QJZF"

		// Create another federation node
		_ = insertFederationNode(t, env, "other.example.org", "Other Node", "https://other.example.org", "active", 7)
		otherAPIKey := insertAPIKey(t, env, "federation-other-key")

		otherPayload := map[string]any{
			"@context":  "https://togather.foundation/contexts/sel/v0.1.jsonld",
			"@type":     "Event",
			"@id":       "https://other.example.org/events/" + ulid, // Same ULID, different domain
			"name":      "Event from Other Node",
			"startDate": time.Date(2026, 7, 20, 19, 0, 0, 0, time.UTC).Format(time.RFC3339),
		}

		resp := postFederationSync(t, env, otherAPIKey, otherPayload)
		require.Equal(t, http.StatusCreated, resp.StatusCode,
			"same ULID from different origin should create new event")

		// Verify both events exist
		event1Exists := checkFederatedEventExists(t, env, "https://idempotent.example.org/events/"+ulid)
		event2Exists := checkFederatedEventExists(t, env, "https://other.example.org/events/"+ulid)
		require.True(t, event1Exists, "original event should exist")
		require.True(t, event2Exists, "event from different origin should exist")
	})
}

// Helper functions

// checkFederatedEventExists checks if an event with a specific federation_uri exists
func checkFederatedEventExists(t *testing.T, env *testEnv, federationURI string) bool {
	t.Helper()

	var exists bool
	err := env.Pool.QueryRow(env.Context,
		`SELECT EXISTS(SELECT 1 FROM events WHERE federation_uri = $1)`,
		federationURI,
	).Scan(&exists)
	require.NoError(t, err)

	return exists
}

// countFederatedEvents counts events with a specific federation_uri
func countFederatedEvents(t *testing.T, env *testEnv, federationURI string) int {
	t.Helper()

	var count int
	err := env.Pool.QueryRow(env.Context,
		`SELECT COUNT(*) FROM events WHERE federation_uri = $1`,
		federationURI,
	).Scan(&count)
	require.NoError(t, err)

	return count
}

// getFederatedEventName retrieves the name of a federated event
func getFederatedEventName(t *testing.T, env *testEnv, federationURI string) string {
	t.Helper()

	var name string
	err := env.Pool.QueryRow(env.Context,
		`SELECT name FROM events WHERE federation_uri = $1`,
		federationURI,
	).Scan(&name)
	require.NoError(t, err)

	return name
}
