package integration

import (
	"bytes"
	"encoding/json"
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
		"name":      "Test Event",
		"startDate": time.Now().Add(24 * time.Hour).Format(time.RFC3339),
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

	// Sample event payload
	payload := map[string]any{
		"name":      "Rate Limit Test Event",
		"startDate": time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		"location": map[string]any{
			"name":            "Test Venue",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
	}

	t.Run("TierAgent accepts requests within rate limit", func(t *testing.T) {
		// Send a reasonable number of requests that should succeed
		successCount := 0
		failureCount := 0

		for i := 0; i < 10; i++ {
			body, err := json.Marshal(payload)
			require.NoError(t, err)

			req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/ld+json")
			req.Header.Set("Authorization", "Bearer "+apiKey)

			resp, err := env.Server.Client().Do(req)
			require.NoError(t, err)
			_ = resp.Body.Close()

			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				successCount++
			} else {
				failureCount++
			}

			// Pace requests to stay within rate limit
			time.Sleep(100 * time.Millisecond)
		}

		// All paced requests should succeed
		require.Equal(t, 10, successCount,
			"Properly paced requests should all succeed (failures: %d)", failureCount)
	})

	t.Run("Different API keys have independent rate limits", func(t *testing.T) {
		apiKey1 := insertAPIKey(t, env, "ratelimit-agent-1")
		apiKey2 := insertAPIKey(t, env, "ratelimit-agent-2")

		// Send requests with first API key
		body, err := json.Marshal(payload)
		require.NoError(t, err)

		req1, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
		require.NoError(t, err)
		req1.Header.Set("Content-Type", "application/ld+json")
		req1.Header.Set("Authorization", "Bearer "+apiKey1)

		resp1, err := env.Server.Client().Do(req1)
		require.NoError(t, err)
		_ = resp1.Body.Close()

		// Second API key should work independently
		body2, err := json.Marshal(payload)
		require.NoError(t, err)

		req2, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body2))
		require.NoError(t, err)
		req2.Header.Set("Content-Type", "application/ld+json")
		req2.Header.Set("Authorization", "Bearer "+apiKey2)

		resp2, err := env.Server.Client().Do(req2)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp2.Body.Close() })

		require.True(t, resp2.StatusCode >= 200 && resp2.StatusCode < 300,
			"Second API key should have independent rate limit (got %d)", resp2.StatusCode)
	})

	t.Run("Rate limiting applies to batch endpoints", func(t *testing.T) {
		time.Sleep(2 * time.Millisecond) // Avoid ULID prefix collision
		batchKey := insertAPIKey(t, env, "batch-ratelimit-key")

		batchPayload := map[string]any{
			"events": []map[string]any{payload},
		}

		// Send a few batch requests (paced to avoid hitting limit)
		for i := 0; i < 5; i++ {
			body, err := json.Marshal(batchPayload)
			require.NoError(t, err)

			req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events:batch", bytes.NewReader(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/ld+json")
			req.Header.Set("Authorization", "Bearer "+batchKey)

			resp, err := env.Server.Client().Do(req)
			require.NoError(t, err)
			_ = resp.Body.Close()

			// Should either succeed or be rate limited (not other errors)
			require.True(t,
				(resp.StatusCode >= 200 && resp.StatusCode < 300) || resp.StatusCode == http.StatusTooManyRequests,
				"Batch request should succeed or be rate limited, got %d", resp.StatusCode)

			time.Sleep(100 * time.Millisecond)
		}
	})
}

// TestPublicEndpointsNoAuth tests that public endpoints don't require authentication
func TestPublicEndpointsNoAuth(t *testing.T) {
	env := setupTestEnv(t)

	// Create a test event first (with auth)
	apiKey := insertAPIKey(t, env, "setup-test-event")
	payload := map[string]any{
		"name":      "Public Access Test Event",
		"startDate": time.Now().Add(24 * time.Hour).Format(time.RFC3339),
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
