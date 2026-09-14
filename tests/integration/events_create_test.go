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
		"name":        "Neighborhood Jazz Night",
		"description": "An evening of smooth jazz featuring local Toronto musicians in the heart of Centennial Park.",
		"startDate":   time.Date(2026, 9, 12, 19, 0, 0, 0, time.FixedZone("EDT", -4*60*60)).Format(time.RFC3339),
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

func TestCreateAllDayEventRoundTrip(t *testing.T) {
	env := setupTestEnv(t)

	key := insertAPIKey(t, env, "agent-all-day")
	payload := map[string]any{
		"name":        "All Day Exhibition",
		"description": "A full-day gallery exhibition open from morning to close.",
		"startDate":   "2026-11-01",
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
	if resp.StatusCode != http.StatusCreated {
		var failure map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&failure)
		require.Failf(t, "unexpected status", "status=%d response=%v", resp.StatusCode, failure)
	}

	var created map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))

	idStr, _ := created["@id"].(string)
	require.NotEmpty(t, idStr, "create response must include @id")
	parts := strings.Split(idStr, "/")
	ulid := parts[len(parts)-1]
	require.NotEmpty(t, ulid)

	getReq, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events/"+ulid, nil)
	require.NoError(t, err)
	getReq.Header.Set("Accept", "application/ld+json")
	getResp, err := env.Server.Client().Do(getReq)
	require.NoError(t, err)
	t.Cleanup(func() { _ = getResp.Body.Close() })
	require.Equal(t, http.StatusOK, getResp.StatusCode)

	var got map[string]any
	require.NoError(t, json.NewDecoder(getResp.Body).Decode(&got))
	require.Equal(t, "2026-11-01", got["startDate"])
	require.Equal(t, true, got["allDay"])
}

// TestCreateAllDayEventWithTimeRoundTrip verifies an explicit allDay:true flag
// with an RFC 3339 startDate (that includes a time/offset) is not silently
// dropped on ingest and reads back as a date-only value.
func TestCreateAllDayEventWithTimeRoundTrip(t *testing.T) {
	env := setupTestEnv(t)

	key := insertAPIKey(t, env, "agent-all-day-time")
	payload := map[string]any{
		"name":        "All Day Exhibition (explicit)",
		"description": "A full-day gallery exhibition submitted with an explicit all-day flag.",
		"allDay":      true,
		"startDate":   "2026-11-01T00:00:00+01:00",
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
	if resp.StatusCode != http.StatusCreated {
		var failure map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&failure)
		require.Failf(t, "unexpected status", "status=%d response=%v", resp.StatusCode, failure)
	}

	var created map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))

	idStr, _ := created["@id"].(string)
	require.NotEmpty(t, idStr, "create response must include @id")
	parts := strings.Split(idStr, "/")
	ulid := parts[len(parts)-1]
	require.NotEmpty(t, ulid)

	getReq, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events/"+ulid, nil)
	require.NoError(t, err)
	getReq.Header.Set("Accept", "application/ld+json")
	getResp, err := env.Server.Client().Do(getReq)
	require.NoError(t, err)
	t.Cleanup(func() { _ = getResp.Body.Close() })
	require.Equal(t, http.StatusOK, getResp.StatusCode)

	var got map[string]any
	require.NoError(t, json.NewDecoder(getResp.Body).Decode(&got))
	require.Equal(t, "2026-11-01", got["startDate"])
	require.Equal(t, true, got["allDay"])
}

// TestCreateMixedAllDayAndTimedOccurrencesRoundTrip verifies a multi-occurrence
// event whose first occurrence is date-only but whose later occurrence is timed
// keeps the timed occurrence's clock time and does not mark it all-day.
// Regression for t_2fffdfe8.
func TestCreateMixedAllDayAndTimedOccurrencesRoundTrip(t *testing.T) {
	env := setupTestEnv(t)

	key := insertAPIKey(t, env, "agent-mixed-all-day")
	payload := map[string]any{
		"name":        "Mixed Listing Probe",
		"description": "A mixed schedule with one all-day date and one timed performance.",
		"startDate":   "2026-11-01",
		"occurrences": []any{
			map[string]any{"startDate": "2026-11-01"},
			map[string]any{"startDate": "2026-11-02T19:00:00-05:00"},
		},
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
	if resp.StatusCode != http.StatusCreated {
		var failure map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&failure)
		require.Failf(t, "unexpected status", "status=%d response=%v", resp.StatusCode, failure)
	}

	var created map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))

	idStr, _ := created["@id"].(string)
	require.NotEmpty(t, idStr, "create response must include @id")
	parts := strings.Split(idStr, "/")
	ulid := parts[len(parts)-1]
	require.NotEmpty(t, ulid)

	getReq, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events/"+ulid, nil)
	require.NoError(t, err)
	getReq.Header.Set("Accept", "application/ld+json")
	getResp, err := env.Server.Client().Do(getReq)
	require.NoError(t, err)
	t.Cleanup(func() { _ = getResp.Body.Close() })
	require.Equal(t, http.StatusOK, getResp.StatusCode)

	var got map[string]any
	require.NoError(t, json.NewDecoder(getResp.Body).Decode(&got))

	subEvents, ok := got["subEvent"].([]any)
	require.True(t, ok, "response must contain subEvent array")
	require.Len(t, subEvents, 2, "expected 2 subEvents")

	s0, ok := subEvents[0].(map[string]any)
	require.True(t, ok, "subEvent[0] must be an object")
	require.Equal(t, "2026-11-01", s0["startDate"], "subEvent[0] date-only startDate")
	require.Equal(t, true, s0["allDay"], "subEvent[0] must be all-day")

	s1, ok := subEvents[1].(map[string]any)
	require.True(t, ok, "subEvent[1] must be an object")
	require.Equal(t, "2026-11-02T19:00:00-05:00", s1["startDate"], "subEvent[1] must keep its clock time")
	require.NotEqual(t, true, s1["allDay"], "subEvent[1] must not be marked all-day")
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
		"name":        "Idempotent event",
		"description": "Test event for verifying idempotency key handling in the event creation API.",
		"startDate":   time.Date(2026, 9, 12, 19, 0, 0, 0, time.UTC).Format(time.RFC3339),
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
