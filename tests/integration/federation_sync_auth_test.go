package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestFederationSyncAuth verifies authentication and authorization for federation sync endpoint
func TestFederationSyncAuth(t *testing.T) {
	env := setupTestEnv(t)

	// Create a trusted federation node
	nodeDomain := "partner.example.org"
	_ = insertFederationNode(t, env, nodeDomain, "Partner Node", "https://partner.example.org", "active", 8)

	// Create API key for the federation node
	apiKey := insertAPIKey(t, env, "federation-partner-key")

	// Sample event payload for sync
	syncPayload := map[string]any{
		"@context":  "https://togather.foundation/contexts/sel/v0.1.jsonld",
		"@type":     "Event",
		"@id":       "https://partner.example.org/events/01HYX3KQW7ERTV9XNBM2P8QJZF",
		"name":      "Partner Event",
		"startDate": time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC).Format(time.RFC3339),
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

	t.Run("authorized with valid API key", func(t *testing.T) {
		resp := postFederationSync(t, env, apiKey, syncPayload)
		// Should accept (may return 201 Created or 200 OK)
		require.True(t, resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted,
			"should accept request with valid API key (got %d)", resp.StatusCode)
	})

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

	// Create federation node and API key
	_ = insertFederationNode(t, env, "validator.example.org", "Validator Node", "https://validator.example.org", "active", 7)
	apiKey := insertAPIKey(t, env, "federation-validator-key")

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
		defer resp.Body.Close()

		require.Equal(t, http.StatusBadRequest, resp.StatusCode,
			"should reject malformed JSON")
	})

	t.Run("accept valid event payload", func(t *testing.T) {
		validPayload := map[string]any{
			"@context":  "https://togather.foundation/contexts/sel/v0.1.jsonld",
			"@type":     "Event",
			"@id":       "https://validator.example.org/events/01HYX3KQW7ERTV9XNBM2P8QJZF",
			"name":      "Valid Event",
			"startDate": time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC).Format(time.RFC3339),
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
		// Create a low-trust node
		_ = insertFederationNode(t, env, "untrusted.example.org", "Untrusted Node", "https://untrusted.example.org", "active", 2)
		lowTrustKey := insertAPIKey(t, env, "federation-untrusted-key")

		payload := map[string]any{
			"@context":  "https://togather.foundation/contexts/sel/v0.1.jsonld",
			"@type":     "Event",
			"@id":       "https://untrusted.example.org/events/01HYX3KQW7ERTV9XNBM2P8QJZF",
			"name":      "Low Trust Event",
			"startDate": time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC).Format(time.RFC3339),
		}

		resp := postFederationSync(t, env, lowTrustKey, payload)
		// Low trust nodes might have events flagged for review but still accepted
		require.True(t, resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusAccepted,
			"should accept but flag events from low-trust nodes (got %d)", resp.StatusCode)
	})
}

// Helper functions

// insertFederationNode creates a federation node in the database
func insertFederationNode(t *testing.T, env *testEnv, domain, name, baseURL, status string, trustLevel int) string {
	t.Helper()

	var nodeID string
	err := env.Pool.QueryRow(env.Context,
		`INSERT INTO federation_nodes (node_domain, node_name, base_url, federation_status, trust_level, sync_enabled)
		 VALUES ($1, $2, $3, $4, $5, true)
		 RETURNING id`,
		domain, name, baseURL, status, trustLevel,
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
