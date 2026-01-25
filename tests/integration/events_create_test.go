package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCreateEventHappyPath(t *testing.T) {
	env := setupTestEnv(t)

	key := insertAPIKey(t, env, "agent-happy")
	payload := map[string]any{
		"name":      "Neighborhood Jazz Night",
		"startDate": time.Date(2026, 9, 12, 19, 0, 0, 0, time.FixedZone("EDT", -4*60*60)).Format(time.RFC3339),
		"location": map[string]any{
			"name":            "Centennial Park",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
		"organizer": map[string]any{
			"name": "Toronto Arts Org",
		},
		"source": map[string]any{
			"url":     "https://example.com/events/jazz-night",
			"eventId": "evt-123",
		},
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/ld+json")
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	if resp.StatusCode != http.StatusCreated {
		var failure map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&failure)
		require.Failf(t, "unexpected status", "status=%d response=%v", resp.StatusCode, failure)
	}

	var created map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	require.Equal(t, payload["name"], eventNameFromPayload(created))

	location, err := createdEventLocation(created)
	require.NoError(t, err)
	require.Equal(t, "Centennial Park", eventNameFromPayload(location))
}

func TestCreateEventMissingRequiredFields(t *testing.T) {
	env := setupTestEnv(t)

	key := insertAPIKey(t, env, "agent-missing-required")
	payload := map[string]any{
		"description": "Missing required fields",
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/ld+json")
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.True(t, strings.HasPrefix(resp.Header.Get("Content-Type"), "application/problem+json"))
}

func TestCreateEventInvalidDateFormat(t *testing.T) {
	env := setupTestEnv(t)

	key := insertAPIKey(t, env, "agent-invalid-date")
	payload := map[string]any{
		"name":      "Bad date",
		"startDate": "2026-13-40",
		"location": map[string]any{
			"name":            "Centennial Park",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/ld+json")
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.True(t, strings.HasPrefix(resp.Header.Get("Content-Type"), "application/problem+json"))
}

func TestCreateEventMissingLocationAndVirtual(t *testing.T) {
	env := setupTestEnv(t)

	key := insertAPIKey(t, env, "agent-missing-location")
	payload := map[string]any{
		"name":      "No location",
		"startDate": time.Date(2026, 9, 12, 19, 0, 0, 0, time.UTC).Format(time.RFC3339),
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/ld+json")
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.True(t, strings.HasPrefix(resp.Header.Get("Content-Type"), "application/problem+json"))
}

func TestCreateEventMissingAuthHeader(t *testing.T) {
	env := setupTestEnv(t)

	payload := map[string]any{
		"name":      "Unauthorized event",
		"startDate": time.Date(2026, 9, 12, 19, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"location": map[string]any{
			"name":            "Centennial Park",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/ld+json")
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	require.True(t, strings.HasPrefix(resp.Header.Get("Content-Type"), "application/problem+json"))
}

func TestCreateEventInvalidAPIKey(t *testing.T) {
	env := setupTestEnv(t)

	payload := map[string]any{
		"name":      "Invalid key event",
		"startDate": time.Date(2026, 9, 12, 19, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"location": map[string]any{
			"name":            "Centennial Park",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer invalid-key")
	req.Header.Set("Content-Type", "application/ld+json")
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	require.True(t, strings.HasPrefix(resp.Header.Get("Content-Type"), "application/problem+json"))
}

func TestCreateEventIdempotencyReturnsSameEvent(t *testing.T) {
	env := setupTestEnv(t)

	key := insertAPIKey(t, env, "agent-idempotent")
	payload := map[string]any{
		"name":      "Idempotent event",
		"startDate": time.Date(2026, 9, 12, 19, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"location": map[string]any{
			"name":            "Centennial Park",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
		"source": map[string]any{
			"url":     "https://example.com/events/idem",
			"eventId": "idem-1",
		},
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	firstReq, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
	require.NoError(t, err)
	firstReq.Header.Set("Authorization", "Bearer "+key)
	firstReq.Header.Set("Content-Type", "application/ld+json")
	firstReq.Header.Set("Accept", "application/ld+json")
	firstReq.Header.Set("Idempotency-Key", "idem-key-1")

	firstResp, err := env.Server.Client().Do(firstReq)
	require.NoError(t, err)
	t.Cleanup(func() { _ = firstResp.Body.Close() })
	if firstResp.StatusCode == http.StatusConflict {
		var failure map[string]any
		_ = json.NewDecoder(firstResp.Body).Decode(&failure)
		require.Failf(t, "unexpected conflict", "status=%d response=%v", firstResp.StatusCode, failure)
	}
	if firstResp.StatusCode != http.StatusCreated {
		var failure map[string]any
		_ = json.NewDecoder(firstResp.Body).Decode(&failure)
		require.Failf(t, "unexpected status", "status=%d response=%v", firstResp.StatusCode, failure)
	}

	var firstPayload map[string]any
	require.NoError(t, json.NewDecoder(firstResp.Body).Decode(&firstPayload))
	firstID := eventIDFromPayload(firstPayload)
	require.NotEmpty(t, firstID)

	secondReq, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
	require.NoError(t, err)
	secondReq.Header.Set("Authorization", "Bearer "+key)
	secondReq.Header.Set("Content-Type", "application/ld+json")
	secondReq.Header.Set("Accept", "application/ld+json")
	secondReq.Header.Set("Idempotency-Key", "idem-key-1")

	secondResp, err := env.Server.Client().Do(secondReq)
	require.NoError(t, err)
	t.Cleanup(func() { _ = secondResp.Body.Close() })
	require.Equal(t, http.StatusConflict, secondResp.StatusCode)

	var secondPayload map[string]any
	require.NoError(t, json.NewDecoder(secondResp.Body).Decode(&secondPayload))
	secondID := eventIDFromPayload(secondPayload)
	require.Equal(t, firstID, secondID)
}
