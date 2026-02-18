package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUsageTracking_RecordsAPIKeyUsage verifies that the UsageRecorder is properly
// wired into the router and records API key usage to the database (srv-8r58k).
func TestUsageTracking_RecordsAPIKeyUsage(t *testing.T) {
	env := setupTestEnv(t)

	// Create an API key
	apiKey := insertAPIKey(t, env, "usage-tracking-test")

	// Get the API key ID from the database (we need it to query usage)
	var apiKeyID string
	err := env.Pool.QueryRow(env.Context,
		`SELECT id FROM api_keys WHERE name = $1`, "usage-tracking-test",
	).Scan(&apiKeyID)
	require.NoError(t, err, "should find API key in database")

	// Make several authenticated requests that should succeed
	for i := 0; i < 3; i++ {
		payload := map[string]any{
			"name":        fmt.Sprintf("Usage Tracking Test Event %d", i),
			"description": "Test event for verifying API key usage tracking.",
			"startDate":   time.Now().Add(24 * time.Hour).Add(time.Duration(i) * time.Hour).Format(time.RFC3339),
			"location": map[string]any{
				"name":            "Test Venue",
				"addressLocality": "Toronto",
				"addressRegion":   "ON",
			},
		}
		body, err := json.Marshal(payload)
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/ld+json")
		req.Header.Set("Authorization", "Bearer "+apiKey)

		resp, err := env.Server.Client().Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode,
			"authenticated request %d should succeed", i)
	}

	// FLUSH: trigger the recorder to write buffered data to DB
	// Close does a final flush before shutting down the recorder
	require.NotNil(t, env.UsageRecorder, "usage recorder should be initialized")
	err = env.UsageRecorder.Close()
	require.NoError(t, err, "should be able to close usage recorder")

	// Now verify usage was recorded in the database
	var requestCount, errorCount int64
	err = env.Pool.QueryRow(env.Context,
		`SELECT COALESCE(SUM(request_count), 0), COALESCE(SUM(error_count), 0) 
		 FROM api_key_usage WHERE api_key_id = $1::uuid`, apiKeyID,
	).Scan(&requestCount, &errorCount)
	require.NoError(t, err, "should be able to query usage data")

	assert.Equal(t, int64(3), requestCount, "should have recorded 3 requests")
	assert.Equal(t, int64(0), errorCount, "successful requests should have 0 errors")
}

// TestUsageTracking_RecordsErrors verifies that error responses are tracked separately.
func TestUsageTracking_RecordsErrors(t *testing.T) {
	env := setupTestEnv(t)

	// Create an API key
	apiKey := insertAPIKey(t, env, "usage-error-test")

	var apiKeyID string
	err := env.Pool.QueryRow(env.Context,
		`SELECT id FROM api_keys WHERE name = $1`, "usage-error-test",
	).Scan(&apiKeyID)
	require.NoError(t, err)

	// Make a request with invalid payload (should get 400-level error)
	invalidPayload := []byte(`{"invalid": true}`) // Missing required fields
	req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(invalidPayload))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/ld+json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.GreaterOrEqual(t, resp.StatusCode, 400, "invalid payload should return error status")

	// Also make a successful request
	validPayload := map[string]any{
		"name":        "Usage Error Tracking Test",
		"description": "Test event for verifying error tracking in API key usage.",
		"startDate":   time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		"location": map[string]any{
			"name":            "Test Venue",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
	}
	body, err := json.Marshal(validPayload)
	require.NoError(t, err)

	req2, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
	require.NoError(t, err)
	req2.Header.Set("Content-Type", "application/ld+json")
	req2.Header.Set("Authorization", "Bearer "+apiKey)

	resp2, err := env.Server.Client().Do(req2)
	require.NoError(t, err)
	_ = resp2.Body.Close()
	require.Equal(t, http.StatusCreated, resp2.StatusCode, "valid request should succeed")

	// Flush usage data
	require.NotNil(t, env.UsageRecorder)
	err = env.UsageRecorder.Close()
	require.NoError(t, err)

	// Verify: should have 2 total requests, 1 error
	var requestCount, errorCount int64
	err = env.Pool.QueryRow(env.Context,
		`SELECT COALESCE(SUM(request_count), 0), COALESCE(SUM(error_count), 0) 
		 FROM api_key_usage WHERE api_key_id = $1::uuid`, apiKeyID,
	).Scan(&requestCount, &errorCount)
	require.NoError(t, err)

	assert.Equal(t, int64(2), requestCount, "should have recorded 2 total requests")
	assert.Equal(t, int64(1), errorCount, "should have recorded 1 error")
}

// TestUsageTracking_NoRecordingForPublicEndpoints verifies that public (unauthenticated)
// endpoints do not record usage.
func TestUsageTracking_NoRecordingForPublicEndpoints(t *testing.T) {
	env := setupTestEnv(t)

	// Make unauthenticated requests to public endpoints
	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events", nil)
	require.NoError(t, err)

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Flush
	require.NotNil(t, env.UsageRecorder)
	err = env.UsageRecorder.Close()
	require.NoError(t, err)

	// Verify no usage was recorded
	var count int64
	err = env.Pool.QueryRow(env.Context,
		`SELECT COUNT(*) FROM api_key_usage`,
	).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count, "public endpoints should not record usage")
}
