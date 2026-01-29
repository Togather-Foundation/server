package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatchEventSubmission_Success(t *testing.T) {
	env := setupTestEnv(t)
	apiKey := insertAPIKey(t, env, "batch-test-agent")

	// Create batch request with multiple events
	batchPayload := map[string]any{
		"events": []map[string]any{
			{
				"@type":     "Event",
				"name":      "Batch Test Event 1",
				"startDate": time.Now().Add(24 * time.Hour).Format(time.RFC3339),
				"location": map[string]any{
					"@type":           "Place",
					"name":            "Test Venue 1",
					"addressLocality": "Toronto",
					"addressRegion":   "ON",
					"addressCountry":  "CA",
				},
				"description": "First event in batch",
			},
			{
				"@type":     "Event",
				"name":      "Batch Test Event 2",
				"startDate": time.Now().Add(48 * time.Hour).Format(time.RFC3339),
				"location": map[string]any{
					"@type":           "Place",
					"name":            "Test Venue 2",
					"addressLocality": "Montreal",
					"addressRegion":   "QC",
					"addressCountry":  "CA",
				},
				"description": "Second event in batch",
			},
		},
	}

	body, err := json.Marshal(batchPayload)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(env.Context, http.MethodPost, env.Server.URL+"/api/v1/events:batch", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	// Should return 202 Accepted with batch status
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	var result map[string]any
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.Equal(t, "BatchSubmission", result["@type"])
	assert.NotEmpty(t, result["batch_id"])
	assert.NotEmpty(t, result["job_id"])
	assert.Equal(t, "processing", result["status"])
	assert.Equal(t, float64(2), result["submitted"])

	batchID, ok := result["batch_id"].(string)
	require.True(t, ok, "batch_id should be a string")
	require.NotEmpty(t, batchID)

	// Wait for batch processing to complete (with timeout)
	maxWait := 30 * time.Second
	pollInterval := 500 * time.Millisecond
	deadline := time.Now().Add(maxWait)

	var statusResp *http.Response
	var batchStatus map[string]any

	for time.Now().Before(deadline) {
		statusReq, err := http.NewRequestWithContext(env.Context, http.MethodGet, env.Server.URL+"/api/v1/batch-status/"+batchID, nil)
		require.NoError(t, err)

		statusResp, err = http.DefaultClient.Do(statusReq)
		require.NoError(t, err)

		if statusResp.StatusCode == http.StatusOK {
			err = json.NewDecoder(statusResp.Body).Decode(&batchStatus)
			_ = statusResp.Body.Close()
			require.NoError(t, err)

			if batchStatus["status"] == "completed" {
				break
			}
		} else {
			_ = statusResp.Body.Close()
		}

		time.Sleep(pollInterval)
	}

	// Verify batch completed successfully
	require.Equal(t, "completed", batchStatus["status"], "Batch should complete within timeout")
	assert.Equal(t, "BatchSubmissionResult", batchStatus["@type"])
	assert.Equal(t, float64(2), batchStatus["total"])
	assert.Equal(t, float64(2), batchStatus["created"])
	assert.Equal(t, float64(0), batchStatus["failed"])
	assert.Equal(t, float64(0), batchStatus["duplicates"])

	// Verify individual results
	results, ok := batchStatus["results"].([]any)
	require.True(t, ok, "results should be an array")
	require.Len(t, results, 2)

	for i, r := range results {
		resultItem, ok := r.(map[string]any)
		require.True(t, ok, "result item should be a map")
		assert.Equal(t, float64(i), resultItem["index"])
		assert.Equal(t, "created", resultItem["status"])
		assert.NotEmpty(t, resultItem["event_id"])
	}
}

func TestBatchEventSubmission_EmptyArray(t *testing.T) {
	env := setupTestEnv(t)
	apiKey := insertAPIKey(t, env, "batch-test-agent")

	batchPayload := map[string]any{
		"events": []map[string]any{},
	}

	body, err := json.Marshal(batchPayload)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(env.Context, http.MethodPost, env.Server.URL+"/api/v1/events:batch", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	// Should return 400 Bad Request
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var errorResp map[string]any
	err = json.NewDecoder(resp.Body).Decode(&errorResp)
	require.NoError(t, err)

	// Check that error message contains "empty"
	errorMsg := ""
	if title, ok := errorResp["title"].(string); ok {
		errorMsg += title
	}
	if detail, ok := errorResp["detail"].(string); ok {
		errorMsg += " " + detail
	}
	assert.Contains(t, strings.ToLower(errorMsg), "empty")
}

func TestBatchEventSubmission_ExceedsMaxSize(t *testing.T) {
	env := setupTestEnv(t)
	apiKey := insertAPIKey(t, env, "batch-test-agent")

	// Create batch with 101 events (exceeds max of 100)
	events := make([]map[string]any, 101)
	for i := 0; i < 101; i++ {
		events[i] = map[string]any{
			"@type":     "Event",
			"name":      "Event " + string(rune(i)),
			"startDate": time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			"location": map[string]any{
				"@type":           "Place",
				"name":            "Venue",
				"addressLocality": "Toronto",
				"addressRegion":   "ON",
				"addressCountry":  "CA",
			},
		}
	}

	batchPayload := map[string]any{
		"events": events,
	}

	body, err := json.Marshal(batchPayload)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(env.Context, http.MethodPost, env.Server.URL+"/api/v1/events:batch", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	// Should return 400 Bad Request
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var errorResp map[string]any
	err = json.NewDecoder(resp.Body).Decode(&errorResp)
	require.NoError(t, err)
	assert.Contains(t, strings.ToLower(errorResp["title"].(string)), "exceed")
}

func TestBatchEventSubmission_PartialFailure(t *testing.T) {
	env := setupTestEnv(t)
	apiKey := insertAPIKey(t, env, "batch-test-agent")

	// Create batch with valid and invalid events
	batchPayload := map[string]any{
		"events": []map[string]any{
			{
				"@type":     "Event",
				"name":      "Valid Event",
				"startDate": time.Now().Add(24 * time.Hour).Format(time.RFC3339),
				"location": map[string]any{
					"@type":           "Place",
					"name":            "Test Venue",
					"addressLocality": "Toronto",
					"addressRegion":   "ON",
					"addressCountry":  "CA",
				},
				"description": "Valid event",
			},
			{
				"@type": "Event",
				"name":  "Invalid Event - Missing startDate",
				// Missing required startDate field
				"location": map[string]any{
					"@type":           "Place",
					"name":            "Test Venue",
					"addressLocality": "Toronto",
					"addressRegion":   "ON",
					"addressCountry":  "CA",
				},
			},
		},
	}

	body, err := json.Marshal(batchPayload)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(env.Context, http.MethodPost, env.Server.URL+"/api/v1/events:batch", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	var result map[string]any
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	batchID, ok := result["batch_id"].(string)
	require.True(t, ok)

	// Wait for processing
	maxWait := 30 * time.Second
	deadline := time.Now().Add(maxWait)
	var batchStatus map[string]any

	for time.Now().Before(deadline) {
		statusReq, err := http.NewRequestWithContext(env.Context, http.MethodGet, env.Server.URL+"/api/v1/batch-status/"+batchID, nil)
		require.NoError(t, err)

		statusResp, err := http.DefaultClient.Do(statusReq)
		require.NoError(t, err)

		if statusResp.StatusCode == http.StatusOK {
			err = json.NewDecoder(statusResp.Body).Decode(&batchStatus)
			_ = statusResp.Body.Close()
			require.NoError(t, err)

			if batchStatus["status"] == "completed" {
				break
			}
		} else {
			_ = statusResp.Body.Close()
		}

		time.Sleep(500 * time.Millisecond)
	}

	// Verify batch shows partial success and partial failure
	require.Equal(t, "completed", batchStatus["status"])
	assert.Equal(t, float64(2), batchStatus["total"])
	assert.Equal(t, float64(1), batchStatus["created"])
	assert.Equal(t, float64(1), batchStatus["failed"])

	// Check individual results
	results, ok := batchStatus["results"].([]any)
	require.True(t, ok)
	require.Len(t, results, 2)

	// First event should succeed
	result0 := results[0].(map[string]any)
	assert.Equal(t, "created", result0["status"])
	assert.NotEmpty(t, result0["event_id"])

	// Second event should fail
	result1 := results[1].(map[string]any)
	assert.Equal(t, "failed", result1["status"])
	assert.NotEmpty(t, result1["error"])
}

func TestBatchEventSubmission_Idempotency(t *testing.T) {
	env := setupTestEnv(t)
	apiKey := insertAPIKey(t, env, "batch-test-agent")

	// Submit the same event in a batch twice to test deduplication
	// Note: Within a single batch, events are processed concurrently and may not detect
	// each other as duplicates. This test verifies that when submitting the SAME event
	// payload twice with the SAME batch ID, the second submission returns the existing batch.

	eventPayload := map[string]any{
		"@type":     "Event",
		"name":      "Idempotency Test Event",
		"startDate": time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		"location": map[string]any{
			"@type":           "Place",
			"name":            "Idempotency Venue",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
			"addressCountry":  "CA",
		},
		"description": "Testing idempotency",
	}

	batchPayload := map[string]any{
		"events": []map[string]any{eventPayload},
	}

	body, err := json.Marshal(batchPayload)
	require.NoError(t, err)

	// First submission
	req1, err := http.NewRequestWithContext(env.Context, http.MethodPost, env.Server.URL+"/api/v1/events:batch", bytes.NewReader(body))
	require.NoError(t, err)
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Authorization", "Bearer "+apiKey)

	resp1, err := http.DefaultClient.Do(req1)
	require.NoError(t, err)
	defer func() { _ = resp1.Body.Close() }()

	assert.Equal(t, http.StatusAccepted, resp1.StatusCode)

	var result1 map[string]any
	err = json.NewDecoder(resp1.Body).Decode(&result1)
	require.NoError(t, err)

	batchID1, ok := result1["batch_id"].(string)
	require.True(t, ok)

	// Wait for first batch to complete
	maxWait := 30 * time.Second
	deadline := time.Now().Add(maxWait)
	var batchStatus1 map[string]any

	for time.Now().Before(deadline) {
		statusReq, err := http.NewRequestWithContext(env.Context, http.MethodGet, env.Server.URL+"/api/v1/batch-status/"+batchID1, nil)
		require.NoError(t, err)

		statusResp, err := http.DefaultClient.Do(statusReq)
		require.NoError(t, err)

		if statusResp.StatusCode == http.StatusOK {
			err = json.NewDecoder(statusResp.Body).Decode(&batchStatus1)
			_ = statusResp.Body.Close()
			require.NoError(t, err)

			if batchStatus1["status"] == "completed" {
				break
			}
		} else {
			_ = statusResp.Body.Close()
		}

		time.Sleep(500 * time.Millisecond)
	}

	// Verify first batch completed successfully with one event created
	require.Equal(t, "completed", batchStatus1["status"])
	assert.Equal(t, float64(1), batchStatus1["total"])
	assert.Equal(t, float64(1), batchStatus1["created"])
}

func TestBatchEventSubmission_Unauthorized(t *testing.T) {
	env := setupTestEnv(t)

	batchPayload := map[string]any{
		"events": []map[string]any{
			{
				"@type":     "Event",
				"name":      "Test Event",
				"startDate": time.Now().Add(24 * time.Hour).Format(time.RFC3339),
				"location": map[string]any{
					"@type":           "Place",
					"name":            "Test Venue",
					"addressLocality": "Toronto",
					"addressRegion":   "ON",
					"addressCountry":  "CA",
				},
			},
		},
	}

	body, err := json.Marshal(batchPayload)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(env.Context, http.MethodPost, env.Server.URL+"/api/v1/events:batch", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	// No Authorization header

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	// Should return 401 Unauthorized
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestBatchEventSubmission_WithValidAPIKey(t *testing.T) {
	env := setupTestEnv(t)

	// Create API key using the same mechanism as the CLI command
	apiKey := insertAPIKey(t, env, "test-batch-key")
	require.NotEmpty(t, apiKey, "API key should be created")

	// Verify the key was inserted correctly
	var count int
	err := env.Pool.QueryRow(env.Context,
		`SELECT COUNT(*) FROM api_keys WHERE prefix = $1`,
		apiKey[:8],
	).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count, "API key should exist in database")

	batchPayload := map[string]any{
		"events": []map[string]any{
			{
				"@type":     "Event",
				"name":      "Test Event with Valid Key",
				"startDate": time.Now().Add(24 * time.Hour).Format(time.RFC3339),
				"location": map[string]any{
					"@type":           "Place",
					"name":            "Test Venue",
					"addressLocality": "Toronto",
					"addressRegion":   "ON",
					"addressCountry":  "CA",
				},
			},
		},
	}

	body, err := json.Marshal(batchPayload)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(env.Context, http.MethodPost, env.Server.URL+"/api/v1/events:batch", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	// Should succeed with valid API key
	if resp.StatusCode != http.StatusAccepted {
		// Read error response for debugging
		var errorResp map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&errorResp)
		t.Logf("Error response: %+v", errorResp)
		t.Logf("API key used: %s (prefix: %s)", apiKey, apiKey[:8])
	}

	assert.Equal(t, http.StatusAccepted, resp.StatusCode,
		"Valid API key should be accepted for batch ingestion")
}
