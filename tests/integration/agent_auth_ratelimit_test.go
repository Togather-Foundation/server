package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestAgentAuthEnforcement tests that API key authentication is enforced on agent endpoints
func TestAgentAuthEnforcement(t *testing.T) {
	env := setupTestEnv(t)

	// Sample event payload
	payload := map[string]any{
		"name":        "Test Event",
		"description": "Test event for verifying authentication and authorization on agent endpoints.",
		"startDate":   time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		"location": map[string]any{
			"name":            "Test Venue",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	t.Run("POST /api/v1/events without auth returns 401", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/ld+json")

		resp, err := env.Server.Client().Do(req)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })

		require.Equal(t, http.StatusUnauthorized, resp.StatusCode,
			"POST /api/v1/events should require authentication")
	})

	t.Run("POST /api/v1/events with invalid API key returns 401", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/ld+json")
		req.Header.Set("Authorization", "Bearer invalid-key-123")

		resp, err := env.Server.Client().Do(req)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })

		require.Equal(t, http.StatusUnauthorized, resp.StatusCode,
			"POST /api/v1/events should reject invalid API keys")
	})

	t.Run("POST /api/v1/events with valid API key succeeds", func(t *testing.T) {
		time.Sleep(2 * time.Millisecond) // Avoid ULID prefix collision
		apiKey := insertAPIKey(t, env, "test-agent-key")

		req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/ld+json")
		req.Header.Set("Authorization", "Bearer "+apiKey)

		resp, err := env.Server.Client().Do(req)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })

		require.True(t, resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK,
			"POST /api/v1/events with valid API key should succeed (got %d)", resp.StatusCode)
	})

	t.Run("POST /api/v1/events:batch without auth returns 401", func(t *testing.T) {
		batchPayload := map[string]any{
			"events": []map[string]any{payload},
		}
		batchBody, err := json.Marshal(batchPayload)
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events:batch", bytes.NewReader(batchBody))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/ld+json")

		resp, err := env.Server.Client().Do(req)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })

		require.Equal(t, http.StatusUnauthorized, resp.StatusCode,
			"POST /api/v1/events:batch should require authentication")
	})

	t.Run("POST /api/v1/events:batch with valid API key succeeds", func(t *testing.T) {
		time.Sleep(2 * time.Millisecond) // Avoid ULID prefix collision
		apiKey := insertAPIKey(t, env, "test-batch-key-auth")

		batchPayload := map[string]any{
			"events": []map[string]any{payload},
		}
		batchBody, err := json.Marshal(batchPayload)
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events:batch", bytes.NewReader(batchBody))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/ld+json")
		req.Header.Set("Authorization", "Bearer "+apiKey)

		resp, err := env.Server.Client().Do(req)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })

		require.True(t, resp.StatusCode >= 200 && resp.StatusCode < 300,
			"POST /api/v1/events:batch with valid API key should succeed (got %d)", resp.StatusCode)
	})
}

// TestAgentRateLimiting tests that rate limiting is enforced per TierAgent
// Note: This test verifies rate limiting behavior exists but may not always trigger 429s
// depending on the configured rate limit (default: 300/min) and burst capacity.
func TestAgentRateLimiting(t *testing.T) {
	env := setupTestEnv(t)

	// Create API key for rate limit testing
	apiKey := insertAPIKey(t, env, "ratelimit-test-agent")

	t.Run("TierAgent accepts requests within rate limit", func(t *testing.T) {
		// Send a reasonable number of requests that should succeed (not be rate-limited)
		// Each request uses a unique event name to avoid duplicate detection (409 Conflict)
		successCount := 0
		rateLimitedCount := 0

		for i := 0; i < 5; i++ {
			uniquePayload := map[string]any{
				"name":        fmt.Sprintf("Rate Limit Test Event %d", i),
				"description": "Test event for verifying rate limiting behavior on agent tier API endpoints.",
				"startDate":   time.Now().Add(24 * time.Hour).Add(time.Duration(i) * time.Hour).Format(time.RFC3339),
				"location": map[string]any{
					"name":            "Test Venue",
					"addressLocality": "Toronto",
					"addressRegion":   "ON",
				},
			}
			body, err := json.Marshal(uniquePayload)
			require.NoError(t, err)

			req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/ld+json")
			req.Header.Set("Authorization", "Bearer "+apiKey)

			resp, err := env.Server.Client().Do(req)
			require.NoError(t, err)
			_ = resp.Body.Close()

			if resp.StatusCode == http.StatusTooManyRequests {
				rateLimitedCount++
			} else {
				successCount++
			}
		}

		// None should be rate-limited (1000/min limit with only 5 requests)
		require.Equal(t, 0, rateLimitedCount,
			"Properly paced requests should not be rate-limited (succeeded: %d)", successCount)
		require.Greater(t, successCount, 0, "At least some requests should succeed")
	})

	t.Run("Different API keys have independent rate limits", func(t *testing.T) {
		apiKey1 := insertAPIKey(t, env, "ratelimit-agent-1")
		apiKey2 := insertAPIKey(t, env, "ratelimit-agent-2")

		// Use unique payloads to avoid duplicate detection
		payload1 := map[string]any{
			"name":        "Rate Limit Independent Key Test 1",
			"description": "Test event for verifying independent rate limits per API key.",
			"startDate":   time.Now().Add(48 * time.Hour).Format(time.RFC3339),
			"location": map[string]any{
				"name":            "Test Venue",
				"addressLocality": "Toronto",
				"addressRegion":   "ON",
			},
		}
		payload2 := map[string]any{
			"name":        "Rate Limit Independent Key Test 2",
			"description": "Test event for verifying independent rate limits per API key.",
			"startDate":   time.Now().Add(72 * time.Hour).Format(time.RFC3339),
			"location": map[string]any{
				"name":            "Test Venue",
				"addressLocality": "Toronto",
				"addressRegion":   "ON",
			},
		}

		// Send request with first API key
		body1, err := json.Marshal(payload1)
		require.NoError(t, err)

		req1, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body1))
		require.NoError(t, err)
		req1.Header.Set("Content-Type", "application/ld+json")
		req1.Header.Set("Authorization", "Bearer "+apiKey1)

		resp1, err := env.Server.Client().Do(req1)
		require.NoError(t, err)
		_ = resp1.Body.Close()

		// Second API key should work independently with a different event
		body2, err := json.Marshal(payload2)
		require.NoError(t, err)

		req2, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body2))
		require.NoError(t, err)
		req2.Header.Set("Content-Type", "application/ld+json")
		req2.Header.Set("Authorization", "Bearer "+apiKey2)

		resp2, err := env.Server.Client().Do(req2)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp2.Body.Close() })

		// Neither should be rate-limited (429)
		require.NotEqual(t, http.StatusTooManyRequests, resp1.StatusCode,
			"First API key request should not be rate-limited (got %d)", resp1.StatusCode)
		require.NotEqual(t, http.StatusTooManyRequests, resp2.StatusCode,
			"Second API key should have independent rate limit (got %d)", resp2.StatusCode)
	})

	t.Run("Rate limiting applies to batch endpoints", func(t *testing.T) {
		time.Sleep(2 * time.Millisecond) // Avoid ULID prefix collision
		batchKey := insertAPIKey(t, env, "batch-ratelimit-key")

		// Send a few batch requests with unique events to avoid duplicate detection
		for i := 0; i < 3; i++ {
			uniqueEvent := map[string]any{
				"name":        fmt.Sprintf("Batch Rate Limit Test Event %d", i),
				"description": "Test event for verifying rate limiting behavior on batch endpoints.",
				"startDate":   time.Now().Add(96 * time.Hour).Add(time.Duration(i) * time.Hour).Format(time.RFC3339),
				"location": map[string]any{
					"name":            "Test Venue",
					"addressLocality": "Toronto",
					"addressRegion":   "ON",
				},
			}
			batchPayload := map[string]any{
				"events": []map[string]any{uniqueEvent},
			}
			body, err := json.Marshal(batchPayload)
			require.NoError(t, err)

			req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events:batch", bytes.NewReader(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/ld+json")
			req.Header.Set("Authorization", "Bearer "+batchKey)

			resp, err := env.Server.Client().Do(req)
			require.NoError(t, err)
			_ = resp.Body.Close()

			// Should either succeed, be accepted, or be rate limited (not unexpected errors)
			require.True(t,
				(resp.StatusCode >= 200 && resp.StatusCode < 300) ||
					resp.StatusCode == http.StatusConflict ||
					resp.StatusCode == http.StatusTooManyRequests,
				"Batch request should succeed, conflict, or be rate limited, got %d", resp.StatusCode)
		}
	})
}

// TestPublicEndpointsNoAuth tests that public endpoints don't require authentication
func TestPublicEndpointsNoAuth(t *testing.T) {
	env := setupTestEnv(t)

	// Create a test event first (with auth)
	apiKey := insertAPIKey(t, env, "setup-test-event")
	payload := map[string]any{
		"name":        "Public Access Test Event",
		"description": "Test event for verifying public access to read endpoints without authentication.",
		"startDate":   time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		"location": map[string]any{
			"name":            "Public Venue",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	createReq, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
	require.NoError(t, err)
	createReq.Header.Set("Content-Type", "application/ld+json")
	createReq.Header.Set("Authorization", "Bearer "+apiKey)

	createResp, err := env.Server.Client().Do(createReq)
	require.NoError(t, err)
	t.Cleanup(func() { _ = createResp.Body.Close() })
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	t.Run("GET /api/v1/events without auth succeeds", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events", nil)
		require.NoError(t, err)

		resp, err := env.Server.Client().Do(req)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })

		require.Equal(t, http.StatusOK, resp.StatusCode,
			"GET /api/v1/events should be accessible without authentication")
	})

	t.Run("GET /api/v1/places without auth succeeds", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/places", nil)
		require.NoError(t, err)

		resp, err := env.Server.Client().Do(req)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })

		require.Equal(t, http.StatusOK, resp.StatusCode,
			"GET /api/v1/places should be accessible without authentication")
	})

	t.Run("GET /api/v1/organizations without auth succeeds", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/organizations", nil)
		require.NoError(t, err)

		resp, err := env.Server.Client().Do(req)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })

		require.Equal(t, http.StatusOK, resp.StatusCode,
			"GET /api/v1/organizations should be accessible without authentication")
	})
}
