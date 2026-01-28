package contracts_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"
)

// TestDualTimestampTracking verifies that both source-provided timestamps and server-received
// timestamps are tracked and exposed correctly per FR-029
func TestDualTimestampTracking(t *testing.T) {
	env := setupTestEnv(t)

	// Create source and event via API
	key := insertAPIKey(t, env, "timestamp-test-agent")

	// Source-provided timestamp (when the event was published by the source)
	sourcePublishedTime := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

	payload := map[string]any{
		"name":      "Timestamp Test Event",
		"startDate": time.Date(2026, 9, 1, 19, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"location": map[string]any{
			"name":            "Timestamp Venue",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
		"organizer": map[string]any{
			"name": "Timestamp Org",
		},
		"source": map[string]any{
			"url":         "https://example.com/events/timestamp-test",
			"eventId":     "evt-ts-123",
			"publishedAt": sourcePublishedTime.Format(time.RFC3339),
		},
	}

	beforeSubmission := time.Now()

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/ld+json")
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	afterSubmission := time.Now()

	var created map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	eventID := eventIDFromPayload(created)
	require.NotEmpty(t, eventID)

	// Verify dual timestamps in database
	var retrievedAt time.Time
	var payloadData []byte
	err = env.Pool.QueryRow(env.Context,
		`SELECT retrieved_at, payload FROM event_sources 
		 WHERE event_id = (SELECT id FROM events WHERE ulid = $1)`,
		eventID,
	).Scan(&retrievedAt, &payloadData)
	require.NoError(t, err, "event_sources should have record with timestamps")

	// Server-received timestamp (retrieved_at) should be within our measurement window
	require.True(t, retrievedAt.After(beforeSubmission) || retrievedAt.Equal(beforeSubmission),
		"retrieved_at should be after or equal to submission start")
	require.True(t, retrievedAt.Before(afterSubmission) || retrievedAt.Equal(afterSubmission),
		"retrieved_at should be before or equal to submission end")

	// Source-provided timestamp should be preserved in payload
	var payloadParsed map[string]any
	require.NoError(t, json.Unmarshal(payloadData, &payloadParsed))

	if sourceInfo, ok := payloadParsed["source"].(map[string]any); ok {
		if publishedAt, ok := sourceInfo["publishedAt"].(string); ok {
			parsedTime, err := time.Parse(time.RFC3339, publishedAt)
			require.NoError(t, err, "source publishedAt should be valid timestamp")
			require.Equal(t, sourcePublishedTime.Unix(), parsedTime.Unix(),
				"source-provided timestamp should be preserved exactly")
		}
	}

	t.Logf("Source timestamp: %v, Server timestamp: %v", sourcePublishedTime, retrievedAt)
}

// TestFieldProvenanceTimestamps verifies that field_provenance table tracks observed_at correctly
func TestFieldProvenanceTimestamps(t *testing.T) {
	env := setupTestEnv(t)

	// Create source and event
	sourceID := insertSource(t, env, "Field Timestamp Source", "api", "CC0", 7)
	placeULID := ulid.Make().String()
	var placeID string
	err := env.Pool.QueryRow(env.Context,
		`INSERT INTO places (ulid, name, address_locality) VALUES ($1, $2, $3) RETURNING id`,
		placeULID, "Field TS Venue", "Toronto",
	).Scan(&placeID)
	require.NoError(t, err)

	eventULID := ulid.Make().String()
	var eventID string
	err = env.Pool.QueryRow(env.Context,
		`INSERT INTO events (ulid, name, primary_venue_id, lifecycle_state, event_domain)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id`,
		eventULID, "Field Timestamp Event", placeID, "published", "arts",
	).Scan(&eventID)
	require.NoError(t, err)

	// Add occurrence
	_, err = env.Pool.Exec(env.Context,
		`INSERT INTO event_occurrences (event_id, start_time, venue_id) VALUES ($1, $2, $3)`,
		eventID, time.Date(2026, 10, 1, 19, 0, 0, 0, time.UTC), placeID,
	)
	require.NoError(t, err)

	// Record field provenance with specific observed_at time
	sourceObservedTime := time.Date(2026, 1, 26, 14, 30, 0, 0, time.UTC)
	beforeInsert := time.Now()

	_, err = env.Pool.Exec(env.Context,
		`INSERT INTO field_provenance (event_id, field_path, value_hash, value_preview, source_id, confidence, observed_at, applied_to_canonical)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		eventID, "/name", "hash-field-ts", "Field Timestamp Event", sourceID, 0.9, sourceObservedTime, true,
	)
	require.NoError(t, err)

	afterInsert := time.Now()

	// Verify the observed_at timestamp is preserved exactly as provided
	var observedAt time.Time
	err = env.Pool.QueryRow(env.Context,
		`SELECT observed_at FROM field_provenance WHERE event_id = $1 AND field_path = $2`,
		eventID, "/name",
	).Scan(&observedAt)
	require.NoError(t, err)

	// observed_at should match exactly what we provided
	require.Equal(t, sourceObservedTime.Unix(), observedAt.Unix(),
		"observed_at should preserve source-provided timestamp exactly")

	// Note: The record also has an implicit created_at from the database
	// but observed_at represents when the SOURCE observed the data
	t.Logf("Source observed time: %v, DB insert time range: %v to %v",
		sourceObservedTime, beforeInsert, afterInsert)
}

// TestEventSourceRetrievedAtDefault verifies that retrieved_at defaults to now() if not provided
func TestEventSourceRetrievedAtDefault(t *testing.T) {
	env := setupTestEnv(t)

	// Create source and event
	sourceID := insertSource(t, env, "Retrieved At Source", "scraper", "CC0", 6)
	placeULID := ulid.Make().String()
	var placeID string
	err := env.Pool.QueryRow(env.Context,
		`INSERT INTO places (ulid, name, address_locality) VALUES ($1, $2, $3) RETURNING id`,
		placeULID, "Retrieved At Venue", "Toronto",
	).Scan(&placeID)
	require.NoError(t, err)

	eventULID := ulid.Make().String()
	var eventID string
	err = env.Pool.QueryRow(env.Context,
		`INSERT INTO events (ulid, name, primary_venue_id, lifecycle_state, event_domain)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id`,
		eventULID, "Retrieved At Event", placeID, "published", "arts",
	).Scan(&eventID)
	require.NoError(t, err)

	// Add occurrence
	_, err = env.Pool.Exec(env.Context,
		`INSERT INTO event_occurrences (event_id, start_time, venue_id) VALUES ($1, $2, $3)`,
		eventID, time.Date(2026, 11, 1, 19, 0, 0, 0, time.UTC), placeID,
	)
	require.NoError(t, err)

	// Insert event_source without explicit retrieved_at (should default to now())
	beforeInsert := time.Now()

	_, err = env.Pool.Exec(env.Context,
		`INSERT INTO event_sources (event_id, source_id, source_url, payload, payload_hash)
		 VALUES ($1, $2, $3, $4, $5)`,
		eventID, sourceID, "https://scraper.com/event/123", `{"name":"Retrieved At Event"}`, "hash-default",
	)
	require.NoError(t, err)

	afterInsert := time.Now()

	// Verify retrieved_at was set to current time
	var retrievedAt time.Time
	err = env.Pool.QueryRow(env.Context,
		`SELECT retrieved_at FROM event_sources WHERE event_id = $1`,
		eventID,
	).Scan(&retrievedAt)
	require.NoError(t, err)

	// Should be between our before/after markers
	require.True(t, retrievedAt.After(beforeInsert) || retrievedAt.Equal(beforeInsert),
		"retrieved_at should default to current time (after insert start)")
	require.True(t, retrievedAt.Before(afterInsert) || retrievedAt.Equal(afterInsert),
		"retrieved_at should default to current time (before insert end)")
}

// TestTimestampPrecision verifies that timestamps preserve microsecond precision
func TestTimestampPrecision(t *testing.T) {
	env := setupTestEnv(t)

	// Create source and event
	sourceID := insertSource(t, env, "Precision Source", "api", "CC0", 8)
	placeULID := ulid.Make().String()
	var placeID string
	err := env.Pool.QueryRow(env.Context,
		`INSERT INTO places (ulid, name, address_locality) VALUES ($1, $2, $3) RETURNING id`,
		placeULID, "Precision Venue", "Toronto",
	).Scan(&placeID)
	require.NoError(t, err)

	eventULID := ulid.Make().String()
	var eventID string
	err = env.Pool.QueryRow(env.Context,
		`INSERT INTO events (ulid, name, primary_venue_id, lifecycle_state, event_domain)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id`,
		eventULID, "Precision Event", placeID, "published", "arts",
	).Scan(&eventID)
	require.NoError(t, err)

	// Add occurrence
	_, err = env.Pool.Exec(env.Context,
		`INSERT INTO event_occurrences (event_id, start_time, venue_id) VALUES ($1, $2, $3)`,
		eventID, time.Date(2026, 12, 1, 19, 0, 0, 0, time.UTC), placeID,
	)
	require.NoError(t, err)

	// Create a precise timestamp with microseconds
	preciseTime := time.Date(2026, 1, 26, 15, 45, 30, 123456789, time.UTC)

	_, err = env.Pool.Exec(env.Context,
		`INSERT INTO event_sources (event_id, source_id, source_url, payload, payload_hash, retrieved_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		eventID, sourceID, "https://precise.com/event", `{"name":"Precision Event"}`, "hash-precise", preciseTime,
	)
	require.NoError(t, err)

	// Retrieve and verify precision
	var retrievedAt time.Time
	err = env.Pool.QueryRow(env.Context,
		`SELECT retrieved_at FROM event_sources WHERE event_id = $1`,
		eventID,
	).Scan(&retrievedAt)
	require.NoError(t, err)

	// PostgreSQL TIMESTAMPTZ has microsecond precision
	// Compare timestamps (postgres may truncate nanoseconds to microseconds)
	expectedMicros := preciseTime.UnixMicro()
	actualMicros := retrievedAt.UnixMicro()

	require.Equal(t, expectedMicros, actualMicros,
		"timestamp should preserve microsecond precision")
}

// TestTimestampComparison verifies that source timestamp vs server timestamp can be compared
func TestTimestampComparison(t *testing.T) {
	env := setupTestEnv(t)

	// Scenario: Source published event 1 hour ago, but we just retrieved it now
	sourcePublishTime := time.Now().Add(-1 * time.Hour)
	serverRetrievalTime := time.Now()

	// Create source
	sourceID := insertSource(t, env, "Comparison Source", "api", "CC0", 7)

	placeULID := ulid.Make().String()
	var placeID string
	err := env.Pool.QueryRow(env.Context,
		`INSERT INTO places (ulid, name, address_locality) VALUES ($1, $2, $3) RETURNING id`,
		placeULID, "Comparison Venue", "Toronto",
	).Scan(&placeID)
	require.NoError(t, err)

	eventULID := ulid.Make().String()
	var eventID string
	err = env.Pool.QueryRow(env.Context,
		`INSERT INTO events (ulid, name, primary_venue_id, lifecycle_state, event_domain)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id`,
		eventULID, "Comparison Event", placeID, "published", "arts",
	).Scan(&eventID)
	require.NoError(t, err)

	// Add occurrence
	_, err = env.Pool.Exec(env.Context,
		`INSERT INTO event_occurrences (event_id, start_time, venue_id) VALUES ($1, $2, $3)`,
		eventID, time.Date(2027, 1, 1, 19, 0, 0, 0, time.UTC), placeID,
	)
	require.NoError(t, err)

	// Store both timestamps
	payloadWithSourceTime := map[string]any{
		"name":        "Comparison Event",
		"publishedAt": sourcePublishTime.Format(time.RFC3339),
	}
	payloadJSON, err := json.Marshal(payloadWithSourceTime)
	require.NoError(t, err)

	_, err = env.Pool.Exec(env.Context,
		`INSERT INTO event_sources (event_id, source_id, source_url, payload, payload_hash, retrieved_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		eventID, sourceID, "https://compare.com/event", payloadJSON, "hash-compare", serverRetrievalTime,
	)
	require.NoError(t, err)

	// Query and verify we can extract both timestamps
	var retrievedAt time.Time
	var payload []byte
	err = env.Pool.QueryRow(env.Context,
		`SELECT retrieved_at, payload FROM event_sources WHERE event_id = $1`,
		eventID,
	).Scan(&retrievedAt, &payload)
	require.NoError(t, err)

	// Parse source timestamp from payload
	var payloadData map[string]any
	require.NoError(t, json.Unmarshal(payload, &payloadData))

	sourceTimeStr, ok := payloadData["publishedAt"].(string)
	require.True(t, ok, "payload should contain publishedAt")

	sourceTime, err := time.Parse(time.RFC3339, sourceTimeStr)
	require.NoError(t, err)

	// Verify the time difference (retrieval should be after publication)
	require.True(t, retrievedAt.After(sourceTime),
		"server retrieval time should be after source publish time")

	timeDiff := retrievedAt.Sub(sourceTime)
	require.True(t, timeDiff > 50*time.Minute && timeDiff < 70*time.Minute,
		"time difference should be approximately 1 hour")

	t.Logf("Source published: %v, Server retrieved: %v, Difference: %v",
		sourceTime, retrievedAt, timeDiff)
}

// Helper function to insert a test source
func insertSource(t *testing.T, env *testEnv, name string, sourceType string, licenseType string, trustLevel int) string {
	t.Helper()

	var sourceID string
	err := env.Pool.QueryRow(env.Context,
		`INSERT INTO sources (name, source_type, license_url, license_type, trust_level, is_active)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id`,
		name, sourceType, "https://creativecommons.org/publicdomain/zero/1.0/", licenseType, trustLevel, true,
	).Scan(&sourceID)
	require.NoError(t, err, "failed to insert source")

	return sourceID
}
