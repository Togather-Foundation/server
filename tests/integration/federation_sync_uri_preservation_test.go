package integration

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestFederationSyncURIPreservation verifies that original URIs are preserved for federated events
func TestFederationSyncURIPreservation(t *testing.T) {
	env := setupTestEnv(t)

	// Create federation node and API key
	nodeDomain := "preserve.example.org"
	nodeID := insertFederationNode(t, env, nodeDomain, "Preservation Node", "https://preserve.example.org", "active", 8)
	apiKey := insertAPIKey(t, env, "federation-preserve-key")

	originalEventURI := "https://preserve.example.org/events/01HYX3KQW7ERTV9XNBM2P8QJZF"

	eventPayload := map[string]any{
		"@context":  "https://togather.foundation/contexts/sel/v0.1.jsonld",
		"@type":     "Event",
		"@id":       originalEventURI,
		"name":      "URI Preservation Test Event",
		"startDate": time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"location": map[string]any{
			"@type":           "Place",
			"name":            "Test Venue",
			"addressLocality": "Toronto",
		},
	}

	t.Run("federation_uri is stored correctly", func(t *testing.T) {
		resp := postFederationSync(t, env, apiKey, eventPayload)
		require.Equal(t, http.StatusCreated, resp.StatusCode, "event creation should succeed")

		// Verify federation_uri is preserved
		storedURI := getFederationURI(t, env, originalEventURI)
		require.Equal(t, originalEventURI, storedURI, "federation_uri should match original @id")
	})

	t.Run("origin_node_id is set correctly", func(t *testing.T) {
		originNodeID := getOriginNodeID(t, env, originalEventURI)
		require.Equal(t, nodeID, originNodeID, "origin_node_id should point to source federation node")
	})

	t.Run("local ULID is generated", func(t *testing.T) {
		localULID := getLocalULID(t, env, originalEventURI)
		require.NotEmpty(t, localULID, "local ULID should be generated")
		require.NotEqual(t, originalEventURI, localULID, "local ULID should differ from federation URI")

		// Verify ULID format (26 characters, uppercase alphanumeric)
		require.Len(t, localULID, 26, "ULID should be 26 characters")
		require.Regexp(t, "^[0-9A-Z]{26}$", localULID, "ULID should be uppercase alphanumeric")
	})

	t.Run("event retrievable by local ULID", func(t *testing.T) {
		localULID := getLocalULID(t, env, originalEventURI)

		// GET /api/v1/events/{localULID} should return the event
		resp := getEventByULID(t, env, localULID)
		require.Equal(t, http.StatusOK, resp.StatusCode, "event should be retrievable by local ULID")
	})

	t.Run("event JSON-LD includes both local and federation URIs", func(t *testing.T) {
		localULID := getLocalULID(t, env, originalEventURI)

		resp := getEventByULID(t, env, localULID)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var eventData map[string]any
		require.NoError(t, resp.Body.Close())
		resp = getEventByULID(t, env, localULID)
		defer resp.Body.Close()
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&eventData))

		// Local @id should be present
		localID, ok := eventData["@id"].(string)
		require.True(t, ok, "response should include @id")
		require.Contains(t, localID, localULID, "local @id should contain ULID")

		// Federation URI should be in sameAs
		sameAs, ok := eventData["sameAs"]
		require.True(t, ok, "response should include sameAs field for federation URI")

		// sameAs can be string or array
		switch v := sameAs.(type) {
		case string:
			require.Equal(t, originalEventURI, v, "sameAs should contain original federation URI")
		case []any:
			found := false
			for _, uri := range v {
				if uriStr, ok := uri.(string); ok && uriStr == originalEventURI {
					found = true
					break
				}
			}
			require.True(t, found, "sameAs array should contain original federation URI")
		default:
			require.Fail(t, "sameAs should be string or array")
		}
	})

	t.Run("never re-mint URIs for foreign events", func(t *testing.T) {
		// Update the event
		updatedPayload := map[string]any{
			"@context":    "https://togather.foundation/contexts/sel/v0.1.jsonld",
			"@type":       "Event",
			"@id":         originalEventURI,
			"name":        "URI Preservation Test Event - Updated",
			"description": "This event has been updated",
			"startDate":   time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC).Format(time.RFC3339),
		}

		resp := postFederationSync(t, env, apiKey, updatedPayload)
		require.True(t, resp.StatusCode >= 200 && resp.StatusCode < 300,
			"update should succeed (got %d)", resp.StatusCode)

		// federation_uri should remain unchanged
		storedURI := getFederationURI(t, env, originalEventURI)
		require.Equal(t, originalEventURI, storedURI, "federation_uri must not change on update")

		// origin_node_id should remain unchanged
		originNodeID := getOriginNodeID(t, env, originalEventURI)
		require.Equal(t, nodeID, originNodeID, "origin_node_id must not change on update")
	})

	t.Run("places and organizations also preserve URIs", func(t *testing.T) {
		placePayload := map[string]any{
			"@context":  "https://togather.foundation/contexts/sel/v0.1.jsonld",
			"@type":     "Event",
			"@id":       "https://preserve.example.org/events/EVENT_WITH_FEDERATED_PLACE",
			"name":      "Event with Federated Place",
			"startDate": time.Date(2026, 7, 15, 19, 0, 0, 0, time.UTC).Format(time.RFC3339),
			"location": map[string]any{
				"@type":           "Place",
				"@id":             "https://preserve.example.org/places/FEDERATED_PLACE_123",
				"name":            "Federated Venue",
				"addressLocality": "Toronto",
				"addressRegion":   "ON",
			},
			"organizer": map[string]any{
				"@type": "Organization",
				"@id":   "https://preserve.example.org/organizations/FEDERATED_ORG_456",
				"name":  "Federated Organization",
			},
		}

		resp := postFederationSync(t, env, apiKey, placePayload)
		require.True(t, resp.StatusCode >= 200 && resp.StatusCode < 300,
			"event with federated entities should succeed (got %d)", resp.StatusCode)

		// Verify place URI is preserved
		placeExists := checkFederatedPlaceExists(t, env, "https://preserve.example.org/places/FEDERATED_PLACE_123")
		require.True(t, placeExists, "federated place should exist with original URI")

		// Verify organization URI is preserved
		orgExists := checkFederatedOrganizationExists(t, env, "https://preserve.example.org/organizations/FEDERATED_ORG_456")
		require.True(t, orgExists, "federated organization should exist with original URI")
	})

	t.Run("local events do not have federation_uri", func(t *testing.T) {
		// Create a local event (not through federation sync)
		org := insertOrganization(t, env, "Local Org")
		place := insertPlace(t, env, "Local Place", "Toronto")
		localEventULID := insertEventWithOccurrence(t, env, "Local Event", org.ID, place.ID, "music", "published", []string{"local"}, time.Date(2026, 7, 20, 19, 0, 0, 0, time.UTC))

		// Verify no federation_uri
		var federationURI *string
		err := env.Pool.QueryRow(env.Context,
			`SELECT federation_uri FROM events WHERE ulid = $1`,
			localEventULID,
		).Scan(&federationURI)
		require.NoError(t, err)
		require.Nil(t, federationURI, "local events should not have federation_uri")

		// Verify no origin_node_id
		var originNodeID *string
		err = env.Pool.QueryRow(env.Context,
			`SELECT origin_node_id FROM events WHERE ulid = $1`,
			localEventULID,
		).Scan(&originNodeID)
		require.NoError(t, err)
		require.Nil(t, originNodeID, "local events should not have origin_node_id")
	})
}

// Helper functions

// getFederationURI retrieves the federation_uri for an event
func getFederationURI(t *testing.T, env *testEnv, federationURI string) string {
	t.Helper()

	var storedURI string
	err := env.Pool.QueryRow(env.Context,
		`SELECT federation_uri FROM events WHERE federation_uri = $1`,
		federationURI,
	).Scan(&storedURI)
	require.NoError(t, err)

	return storedURI
}

// getOriginNodeID retrieves the origin_node_id for an event
func getOriginNodeID(t *testing.T, env *testEnv, federationURI string) string {
	t.Helper()

	var nodeID string
	err := env.Pool.QueryRow(env.Context,
		`SELECT origin_node_id FROM events WHERE federation_uri = $1`,
		federationURI,
	).Scan(&nodeID)
	require.NoError(t, err)

	return nodeID
}

// getLocalULID retrieves the local ULID for a federated event
func getLocalULID(t *testing.T, env *testEnv, federationURI string) string {
	t.Helper()

	var ulid string
	err := env.Pool.QueryRow(env.Context,
		`SELECT ulid FROM events WHERE federation_uri = $1`,
		federationURI,
	).Scan(&ulid)
	require.NoError(t, err)

	return ulid
}

// getEventByULID makes a GET request to retrieve an event by ULID
func getEventByULID(t *testing.T, env *testEnv, ulid string) *http.Response {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events/"+ulid, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	return resp
}

// checkFederatedPlaceExists checks if a place with federation_uri exists
func checkFederatedPlaceExists(t *testing.T, env *testEnv, federationURI string) bool {
	t.Helper()

	var exists bool
	err := env.Pool.QueryRow(env.Context,
		`SELECT EXISTS(SELECT 1 FROM places WHERE federation_uri = $1)`,
		federationURI,
	).Scan(&exists)
	require.NoError(t, err)

	return exists
}

// checkFederatedOrganizationExists checks if an organization with federation_uri exists
func checkFederatedOrganizationExists(t *testing.T, env *testEnv, federationURI string) bool {
	t.Helper()

	var exists bool
	err := env.Pool.QueryRow(env.Context,
		`SELECT EXISTS(SELECT 1 FROM organizations WHERE federation_uri = $1)`,
		federationURI,
	).Scan(&exists)
	require.NoError(t, err)

	return exists
}
