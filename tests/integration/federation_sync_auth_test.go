package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/tests/testhelpers"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"
)

// TestFederationSyncAuth verifies authentication and authorization for the
// federation sync endpoint, including API-key-to-node binding.
func TestFederationSyncAuth(t *testing.T) {
	env := setupTestEnv(t)

	// Node A: the caller. Node B exists so a mismatch is a real ID mismatch,
	// not merely an unknown domain.
	nodeDomain := "partner.example.org"
	nodeID := insertFederationNode(t, env, nodeDomain, "Partner Node", "https://partner.example.org", "active", 8)
	otherDomain := "other-partner.example.org"
	_ = insertFederationNode(t, env, otherDomain, "Other Partner Node", "https://other-partner.example.org", "active", 7)

	apiKey := testhelpers.InsertAPIKeyWithFederationNode(t, env.Pool, env.Context, "federation-partner-key", nodeID)

	// Sample event payload for sync
	syncPayload := map[string]any{
		"@context":  "https://togather.foundation/contexts/sel/v0.1.jsonld",
		"@type":     "Event",
		"@id":       "https://partner.example.org/events/01HYX3KQW7ERTV9XNBM2P8QJZF",
		"name":      "Partner Event",
		"startDate": testhelpers.FutureTime(1).Format(time.RFC3339),
		"location": map[string]any{
			"@type":           "Place",
			"name":            "Partner Venue",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
	}

	t.Run("unauthorized without API key", func(t *testing.T) {
		resp := postFederationSync(t, env, "", syncPayload)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "should reject request without API key")
	})

	t.Run("unauthorized with invalid API key", func(t *testing.T) {
		resp := postFederationSync(t, env, "invalid-key", syncPayload)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "should reject request with invalid API key")
	})

	t.Run("authorized with bound API key", func(t *testing.T) {
		resp := postFederationSync(t, env, apiKey, syncPayload)
		require.True(t, resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK,
			"should accept request with a key bound to the payload origin node (got %d)", resp.StatusCode)
	})

	t.Run("unbound API key is rejected", func(t *testing.T) {
		unboundKey := insertAPIKey(t, env, "federation-unbound-key")
		payload := cloneSyncPayload(syncPayload, "https://partner.example.org/events/UNBOUND_KEY_EVENT")

		resp := postFederationSync(t, env, unboundKey, payload)
		requireProblem(t, resp, http.StatusForbidden, "https://sel.events/problems/forbidden", "Federation node not authorized")

		require.False(t, checkFederatedEventExists(t, env, payload["@id"].(string)),
			"unbound key must not create an event")
	})

	t.Run("key bound to node A cannot post a node B event", func(t *testing.T) {
		payload := cloneSyncPayload(syncPayload, "https://other-partner.example.org/events/MISMATCH_EVENT")

		resp := postFederationSync(t, env, apiKey, payload)
		requireProblem(t, resp, http.StatusForbidden, "https://sel.events/problems/forbidden", "Federation node not authorized")

		require.False(t, checkFederatedEventExists(t, env, payload["@id"].(string)),
			"mismatched origin must not create an event")
	})

	t.Run("key bound to node A cannot upsert a node B place", func(t *testing.T) {
		const peerPlaceURI = "https://other-partner.example.org/places/FOREIGN_PLACE"
		const peerPlaceName = "Peer Venue"
		insertFederatedPlace(t, env, peerPlaceURI, peerPlaceName)

		payload := cloneSyncPayload(syncPayload, "https://partner.example.org/events/FOREIGN_PLACE_EVENT")
		payload["location"] = map[string]any{
			"@type":           "Place",
			"@id":             peerPlaceURI,
			"name":            "Overwritten By Node A",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		}

		resp := postFederationSync(t, env, apiKey, payload)
		requireProblem(t, resp, http.StatusForbidden, "https://sel.events/problems/forbidden", "Federation node not authorized")

		require.False(t, checkFederatedEventExists(t, env, payload["@id"].(string)),
			"foreign nested place must not create an event")
		require.True(t, checkFederatedPlaceExists(t, env, peerPlaceURI),
			"peer place row must still exist")
		require.Equal(t, peerPlaceName, getFederatedPlaceName(t, env, peerPlaceURI),
			"peer place row must not be overwritten")
	})

	t.Run("key bound to node A cannot upsert a node B organizer", func(t *testing.T) {
		const peerOrgURI = "https://other-partner.example.org/organizations/FOREIGN_ORG"
		const peerOrgName = "Peer Org"
		insertFederatedOrganization(t, env, peerOrgURI, peerOrgName)

		payload := cloneSyncPayload(syncPayload, "https://partner.example.org/events/FOREIGN_ORG_EVENT")
		payload["organizer"] = map[string]any{
			"@type": "Organization",
			"@id":   peerOrgURI,
			"name":  "Overwritten By Node A",
		}

		resp := postFederationSync(t, env, apiKey, payload)
		requireProblem(t, resp, http.StatusForbidden, "https://sel.events/problems/forbidden", "Federation node not authorized")

		require.False(t, checkFederatedEventExists(t, env, payload["@id"].(string)),
			"foreign nested organizer must not create an event")
		require.True(t, checkFederatedOrganizationExists(t, env, peerOrgURI),
			"peer organization row must still exist")
		require.Equal(t, peerOrgName, getFederatedOrganizationName(t, env, peerOrgURI),
			"peer organization row must not be overwritten")
	})

	inactiveCases := []struct {
		name   string
		domain string
		opts   federationNodeFixture
	}{
		{
			name:   "blocked node",
			domain: "blocked.example.org",
			opts:   federationNodeFixture{Status: "blocked", TrustLevel: 5},
		},
		{
			name:   "paused node",
			domain: "paused.example.org",
			opts:   federationNodeFixture{Status: "paused", TrustLevel: 5},
		},
		{
			name:   "push only node",
			domain: "push-only.example.org",
			opts:   federationNodeFixture{Status: "active", SyncEnabled: true, SyncDirection: "push_only", TrustLevel: 5},
		},
		{
			name:   "sync disabled node",
			domain: "sync-disabled.example.org",
			opts:   federationNodeFixture{Status: "active", SyncEnabled: false, SyncDirection: "bidirectional", TrustLevel: 5},
		},
	}

	for _, tc := range inactiveCases {
		t.Run(tc.name+" is rejected", func(t *testing.T) {
			inactiveNodeID := insertFederationNodeWithOptions(t, env, tc.domain, tc.name, "https://"+tc.domain, tc.opts)
			inactiveKey := testhelpers.InsertAPIKeyWithFederationNode(t, env.Pool, env.Context, "federation-"+tc.domain+"-key", inactiveNodeID)
			payload := cloneSyncPayload(syncPayload, "https://"+tc.domain+"/events/INACTIVE_NODE_EVENT")

			resp := postFederationSync(t, env, inactiveKey, payload)
			requireProblem(t, resp, http.StatusForbidden, "https://sel.events/problems/forbidden", "Federation node not authorized")

			require.False(t, checkFederatedEventExists(t, env, payload["@id"].(string)),
				"inactive node must not create an event")
		})
	}

	t.Run("rate limiting for federation sync", func(t *testing.T) {
		// Make many requests in quick succession
		successCount := 0
		rateLimitedCount := 0

		for i := 0; i < 10; i++ {
			resp := postFederationSync(t, env, apiKey, syncPayload)
			if resp.StatusCode == http.StatusTooManyRequests {
				rateLimitedCount++
			} else if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				successCount++
			}
		}

		// Should have either succeeded or rate limited (not other errors)
		require.Equal(t, 10, successCount+rateLimitedCount,
			"all requests should either succeed or be rate limited")
	})
}

// TestFederationSyncValidation verifies payload validation for federation sync
func TestFederationSyncValidation(t *testing.T) {
	env := setupTestEnv(t)

	// Create federation node and a key bound to it
	nodeID := insertFederationNode(t, env, "validator.example.org", "Validator Node", "https://validator.example.org", "active", 7)
	apiKey := testhelpers.InsertAPIKeyWithFederationNode(t, env.Pool, env.Context, "federation-validator-key", nodeID)

	t.Run("reject missing required fields", func(t *testing.T) {
		invalidPayloads := []map[string]any{
			// Missing name
			{
				"@type":     "Event",
				"@id":       "https://validator.example.org/events/01HYX3KQW7ERTV9XNBM2P8QJZF",
				"startDate": time.Now().Format(time.RFC3339),
			},
			// Missing startDate
			{
				"@type": "Event",
				"@id":   "https://validator.example.org/events/01HYX3KQW7ERTV9XNBM2P8QJZF",
				"name":  "Event Without Date",
			},
			// Missing @id
			{
				"@type":     "Event",
				"name":      "Event Without ID",
				"startDate": time.Now().Format(time.RFC3339),
			},
			// Missing @type
			{
				"@id":       "https://validator.example.org/events/01HYX3KQW7ERTV9XNBM2P8QJZF",
				"name":      "Event Without Type",
				"startDate": time.Now().Format(time.RFC3339),
			},
		}

		for i, payload := range invalidPayloads {
			t.Run("invalid_payload_"+string(rune(i)), func(t *testing.T) {
				resp := postFederationSync(t, env, apiKey, payload)
				require.Equal(t, http.StatusBadRequest, resp.StatusCode,
					"should reject payload missing required fields")
			})
		}
	})

	t.Run("reject invalid date formats", func(t *testing.T) {
		payload := map[string]any{
			"@type":     "Event",
			"@id":       "https://validator.example.org/events/01HYX3KQW7ERTV9XNBM2P8QJZF",
			"name":      "Invalid Date Event",
			"startDate": "not-a-date",
		}

		resp := postFederationSync(t, env, apiKey, payload)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode,
			"should reject invalid date format")
	})

	t.Run("reject malformed JSON-LD", func(t *testing.T) {
		invalidJSON := []byte(`{"@type": "Event", "name": "Missing closing brace"`)

		req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/federation/sync", bytes.NewReader(invalidJSON))
		require.NoError(t, err, "failed to create HTTP request for malformed JSON test")
		req.Header.Set("Content-Type", "application/ld+json")
		req.Header.Set("Authorization", "Bearer "+apiKey)

		resp, err := env.Server.Client().Do(req)
		require.NoError(t, err, "failed to execute HTTP request for malformed JSON test")
		defer func() { _ = resp.Body.Close() }()

		require.Equal(t, http.StatusBadRequest, resp.StatusCode,
			"should reject malformed JSON")
	})

	t.Run("accept valid event payload", func(t *testing.T) {
		validPayload := map[string]any{
			"@context":  "https://togather.foundation/contexts/sel/v0.1.jsonld",
			"@type":     "Event",
			"@id":       "https://validator.example.org/events/01HYX3KQW7ERTV9XNBM2P8QJZF",
			"name":      "Valid Event",
			"startDate": testhelpers.FutureTime(1).Format(time.RFC3339),
			"location": map[string]any{
				"@type":           "Place",
				"name":            "Valid Venue",
				"addressLocality": "Toronto",
			},
		}

		resp := postFederationSync(t, env, apiKey, validPayload)
		require.True(t, resp.StatusCode >= 200 && resp.StatusCode < 300,
			"should accept valid payload (got %d)", resp.StatusCode)
	})

	t.Run("validate trust level constraints", func(t *testing.T) {
		// Create a low-trust node and bind a key to it. Trust level is not an
		// inbound-sync gate; it influences downstream handling only.
		lowTrustNodeID := insertFederationNode(t, env, "untrusted.example.org", "Untrusted Node", "https://untrusted.example.org", "active", 2)
		lowTrustKey := testhelpers.InsertAPIKeyWithFederationNode(t, env.Pool, env.Context, "federation-untrusted-key", lowTrustNodeID)

		payload := map[string]any{
			"@context":  "https://togather.foundation/contexts/sel/v0.1.jsonld",
			"@type":     "Event",
			"@id":       "https://untrusted.example.org/events/01HYX3KQW7ERTV9XNBM2P8QJZF",
			"name":      "Low Trust Event",
			"startDate": testhelpers.FutureTime(1).Format(time.RFC3339),
		}

		resp := postFederationSync(t, env, lowTrustKey, payload)
		// Low trust nodes might have events flagged for review but still accepted
		require.True(t, resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusAccepted,
			"should accept but flag events from low-trust nodes (got %d)", resp.StatusCode)
	})
}

// Helper functions

// federationNodeFixture describes the inbound-relevant state of a test node.
type federationNodeFixture struct {
	Status        string
	SyncEnabled   bool
	SyncDirection string
	TrustLevel    int
}

// insertFederationNode creates an active, inbound-enabled federation node and
// returns its ID.
func insertFederationNode(t *testing.T, env *testEnv, domain, name, baseURL, status string, trustLevel int) string {
	t.Helper()
	return insertFederationNodeWithOptions(t, env, domain, name, baseURL, federationNodeFixture{
		Status:        status,
		SyncEnabled:   true,
		SyncDirection: "bidirectional",
		TrustLevel:    trustLevel,
	})
}

// insertFederationNodeWithOptions creates a federation node with explicit sync
// settings and returns its ID.
func insertFederationNodeWithOptions(t *testing.T, env *testEnv, domain, name, baseURL string, opts federationNodeFixture) string {
	t.Helper()

	var nodeID string
	err := env.Pool.QueryRow(env.Context,
		`INSERT INTO federation_nodes (node_domain, node_name, base_url, federation_status, trust_level, sync_enabled, sync_direction)
		 VALUES ($1, $2, $3, $4, $5, $6, COALESCE(NULLIF($7, ''), 'bidirectional'))
		 RETURNING id`,
		domain, name, baseURL, opts.Status, opts.TrustLevel, opts.SyncEnabled, opts.SyncDirection,
	).Scan(&nodeID)
	require.NoError(t, err, "should successfully insert federation node")

	return nodeID
}

// postFederationSync makes a POST request to the federation sync endpoint
func postFederationSync(t *testing.T, env *testEnv, apiKey string, payload map[string]any) *http.Response {
	t.Helper()

	body, err := json.Marshal(payload)
	require.NoError(t, err, "failed to marshal federation sync payload to JSON")

	req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/federation/sync", bytes.NewReader(body))
	require.NoError(t, err, "failed to create HTTP POST request for federation sync")
	req.Header.Set("Content-Type", "application/ld+json")
	req.Header.Set("Accept", "application/ld+json")

	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err, "failed to execute federation sync HTTP request")
	t.Cleanup(func() { _ = resp.Body.Close() })

	return resp
}

// cloneSyncPayload copies a payload and replaces its @id (and name) so each
// subtest targets a distinct event URI.
func cloneSyncPayload(base map[string]any, eventURI string) map[string]any {
	clone := make(map[string]any, len(base))
	for k, v := range base {
		clone[k] = v
	}
	clone["@id"] = eventURI
	return clone
}

// insertFederatedPlace pre-inserts a peer-owned place row so tests can prove a
// rejected sync does not overwrite it.
func insertFederatedPlace(t *testing.T, env *testEnv, federationURI, name string) {
	t.Helper()

	_, err := env.Pool.Exec(env.Context,
		`INSERT INTO places (ulid, name, address_locality, address_region, federation_uri)
		 VALUES ($1, $2, $3, $4, $5)`,
		ulid.Make().String(), name, "Toronto", "ON", federationURI,
	)
	require.NoError(t, err, "failed to insert peer place")
}

// getFederatedPlaceName retrieves a place name by federation_uri.
func getFederatedPlaceName(t *testing.T, env *testEnv, federationURI string) string {
	t.Helper()

	var name string
	err := env.Pool.QueryRow(env.Context,
		`SELECT name FROM places WHERE federation_uri = $1`,
		federationURI,
	).Scan(&name)
	require.NoError(t, err)
	return name
}

// insertFederatedOrganization pre-inserts a peer-owned organization row so
// tests can prove a rejected sync does not overwrite it.
func insertFederatedOrganization(t *testing.T, env *testEnv, federationURI, name string) {
	t.Helper()

	_, err := env.Pool.Exec(env.Context,
		`INSERT INTO organizations (ulid, name, address_locality, federation_uri)
		 VALUES ($1, $2, $3, $4)`,
		ulid.Make().String(), name, "Toronto", federationURI,
	)
	require.NoError(t, err, "failed to insert peer organization")
}

// getFederatedOrganizationName retrieves an organization name by federation_uri.
func getFederatedOrganizationName(t *testing.T, env *testEnv, federationURI string) string {
	t.Helper()

	var name string
	err := env.Pool.QueryRow(env.Context,
		`SELECT name FROM organizations WHERE federation_uri = $1`,
		federationURI,
	).Scan(&name)
	require.NoError(t, err)
	return name
}

// requireProblem asserts an RFC 7807 problem+json response.
func requireProblem(t *testing.T, resp *http.Response, status int, wantType, wantTitle string) {
	t.Helper()

	require.Equal(t, status, resp.StatusCode)
	require.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Equal(t, wantType, body["type"])
	require.Equal(t, wantTitle, body["title"])
	require.EqualValues(t, status, body["status"])
}

// TestFederationBoundAPIKeyCreateEchoesStoredNode verifies that creating a
// federation-bound key returns the canonical UUID the database stored, not the
// (possibly non-canonical) request string.
func TestFederationBoundAPIKeyCreateEchoesStoredNode(t *testing.T) {
	env := setupTestEnv(t)

	nodeID := insertFederationNode(t, env, "echo.example.org", "Echo Node", "https://echo.example.org", "active", 5)
	insertAdminUser(t, env, "admin", "admin-password-123", "admin@example.com", "admin")
	token := adminLogin(t, env, "admin", "admin-password-123")

	requestBody := map[string]any{
		"name":               "echo-peer-key",
		"federation_node_id": strings.ToUpper(nodeID), // request in uppercase
	}
	body, err := json.Marshal(requestBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/admin/api-keys", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var created map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	require.Equal(t, nodeID, created["federation_node_id"],
		"response must echo the canonical stored node UUID")

	var storedNodeID string
	err = env.Pool.QueryRow(env.Context,
		`SELECT federation_node_id FROM api_keys WHERE name = $1`,
		"echo-peer-key",
	).Scan(&storedNodeID)
	require.NoError(t, err)
	require.Equal(t, nodeID, storedNodeID, "key must be bound in the database")
}
