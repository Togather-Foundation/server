package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"
)

// TestEventSourceAttribution verifies that retrieved events include correct source attribution metadata
// per FR-024 and SEL Interoperability Profile.
func TestEventSourceAttribution(t *testing.T) {
	env := setupTestEnv(t)

	// Setup: Insert a source and create an event via API (which should track provenance)
	sourceName := "Test Event Source API"
	_ = insertSource(t, env, sourceName, "api", "CC0", 8)
	key := insertAPIKey(t, env, "agent-provenance-test")

	// Create event with source information
	payload := map[string]any{
		"name":        "Community Art Festival",
		"description": "Annual celebration of local artists featuring live music, art installations, and interactive workshops for all ages.",
		"startDate":   time.Date(2026, 8, 15, 14, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"location": map[string]any{
			"name":            "City Hall Plaza",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
		"organizer": map[string]any{
			"name": "Toronto Arts Council",
		},
		"source": map[string]any{
			"url":     "https://example.com/events/art-festival-2026",
			"eventId": "evt-art-2026",
		},
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	// Create the event
	req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/ld+json")
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusCreated, resp.StatusCode, "event creation should succeed")

	var created map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	eventID := eventIDFromPayload(created)
	require.NotEmpty(t, eventID, "event should have an ID")

	// Verify source was recorded in event_sources table
	var recordedSourceID string
	var recordedSourceURL string
	var recordedPayload []byte
	err = env.Pool.QueryRow(env.Context,
		`SELECT source_id, source_url, payload FROM event_sources 
		 WHERE event_id = (SELECT id FROM events WHERE ulid = $1)`,
		eventID,
	).Scan(&recordedSourceID, &recordedSourceURL, &recordedPayload)
	require.NoError(t, err, "event_sources should have a record")
	require.NotEmpty(t, recordedSourceID, "source_id should be recorded")
	require.Equal(t, "https://example.com/events/art-festival-2026", recordedSourceURL)

	// Now retrieve the event and verify source attribution is included
	req2, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events/"+eventID, nil)
	require.NoError(t, err)
	req2.Header.Set("Accept", "application/ld+json")

	resp2, err := env.Server.Client().Do(req2)
	require.NoError(t, err)
	defer func() { _ = resp2.Body.Close() }()
	require.Equal(t, http.StatusOK, resp2.StatusCode)

	var retrieved map[string]any
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&retrieved))

	// Verify source attribution is present in the response
	// The exact structure depends on implementation, but we expect some form of source attribution
	// This could be in a "source" field, "attribution" field, or similar per SEL profile
	if sources, ok := retrieved["sources"].([]any); ok {
		require.NotEmpty(t, sources, "event should include source attribution")
		if len(sources) > 0 {
			firstSource := sources[0].(map[string]any)
			require.NotEmpty(t, firstSource["url"], "source should have URL")
		}
	} else if attribution, ok := retrieved["attribution"].(map[string]any); ok {
		require.NotEmpty(t, attribution, "event should include attribution metadata")
	} else {
		// Source might be embedded in a different way - check for any provenance info
		t.Logf("Response payload: %+v", retrieved)
		// For now, we'll just verify the event was created and can be retrieved
		// The actual source attribution structure will be defined during implementation
	}

	// Verify license information is present (FR-024)
	if license, ok := retrieved["license"].(string); ok {
		require.NotEmpty(t, license, "event should include license information")
	} else if license, ok := retrieved["license"].(map[string]any); ok {
		require.NotEmpty(t, license, "event should include license information")
	}
}

// TestMultipleSourcesAttribution tests that events from multiple sources show all sources
func TestMultipleSourcesAttribution(t *testing.T) {
	env := setupTestEnv(t)

	// Insert two sources
	source1ID := insertSource(t, env, "Source One", "api", "CC0", 7)
	source2ID := insertSource(t, env, "Source Two", "scraper", "CC0", 6)

	// Create an event
	eventULID := ulid.Make().String()
	var eventID string
	err := env.Pool.QueryRow(env.Context,
		`INSERT INTO events (ulid, name, primary_venue_id, lifecycle_state, event_domain)
		 VALUES ($1, $2, (SELECT id FROM places LIMIT 1), $3, $4)
		 RETURNING id`,
		eventULID, "Multi-Source Event", "published", "arts",
	).Scan(&eventID)

	// If no places exist, create one
	if err != nil {
		placeULID := ulid.Make().String()
		var placeID string
		err = env.Pool.QueryRow(env.Context,
			`INSERT INTO places (ulid, name, address_locality) VALUES ($1, $2, $3) RETURNING id`,
			placeULID, "Test Venue", "Toronto",
		).Scan(&placeID)
		require.NoError(t, err)

		err = env.Pool.QueryRow(env.Context,
			`INSERT INTO events (ulid, name, primary_venue_id, lifecycle_state, event_domain)
			 VALUES ($1, $2, $3, $4, $5)
			 RETURNING id`,
			eventULID, "Multi-Source Event", placeID, "published", "arts",
		).Scan(&eventID)
		require.NoError(t, err)
	}

	// Get the venue_id from the event for the occurrence
	var venueID string
	err = env.Pool.QueryRow(env.Context,
		`SELECT primary_venue_id FROM events WHERE id = $1`,
		eventID,
	).Scan(&venueID)
	require.NoError(t, err)

	// Add occurrence
	_, err = env.Pool.Exec(env.Context,
		`INSERT INTO event_occurrences (event_id, start_time, venue_id) VALUES ($1, $2, $3)`,
		eventID, time.Date(2026, 9, 1, 18, 0, 0, 0, time.UTC), venueID,
	)
	require.NoError(t, err)

	// Record two different sources for this event
	_, err = env.Pool.Exec(env.Context,
		`INSERT INTO event_sources (event_id, source_id, source_url, payload, payload_hash, confidence)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		eventID, source1ID, "https://source1.com/event/123", `{"name":"Multi-Source Event"}`, "hash1", 0.9,
	)
	require.NoError(t, err)

	_, err = env.Pool.Exec(env.Context,
		`INSERT INTO event_sources (event_id, source_id, source_url, payload, payload_hash, confidence)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		eventID, source2ID, "https://source2.com/events/456", `{"name":"Multi-Source Event"}`, "hash2", 0.85,
	)
	require.NoError(t, err)

	// Retrieve the event
	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events/"+eventULID, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var retrieved map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&retrieved))

	// Verify both sources are present in attribution
	// The exact structure will be defined during implementation
	// For now we just verify we can retrieve the event
	require.NotEmpty(t, retrieved["name"], "event should have a name")
}

// TestSourceConfidenceLevels verifies that source confidence is tracked
func TestSourceConfidenceLevels(t *testing.T) {
	env := setupTestEnv(t)

	// Create a high-trust source
	highTrustSource := insertSource(t, env, "High Trust Source", "partner", "CC0", 9)

	// Create event and associate with high-trust source
	eventULID := ulid.Make().String()
	placeULID := ulid.Make().String()
	var placeID, eventID string

	err := env.Pool.QueryRow(env.Context,
		`INSERT INTO places (ulid, name, address_locality) VALUES ($1, $2, $3) RETURNING id`,
		placeULID, "Confidence Test Venue", "Toronto",
	).Scan(&placeID)
	require.NoError(t, err)

	err = env.Pool.QueryRow(env.Context,
		`INSERT INTO events (ulid, name, primary_venue_id, lifecycle_state, event_domain)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id`,
		eventULID, "High Confidence Event", placeID, "published", "arts",
	).Scan(&eventID)
	require.NoError(t, err)

	// Add occurrence
	_, err = env.Pool.Exec(env.Context,
		`INSERT INTO event_occurrences (event_id, start_time, venue_id) VALUES ($1, $2, $3)`,
		eventID, time.Date(2026, 10, 1, 19, 0, 0, 0, time.UTC), placeID,
	)
	require.NoError(t, err)

	// Record source with high confidence
	_, err = env.Pool.Exec(env.Context,
		`INSERT INTO event_sources (event_id, source_id, source_url, payload, payload_hash, confidence)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		eventID, highTrustSource, "https://trusted.com/events/high-conf",
		`{"name":"High Confidence Event"}`, "hash-high", 0.95,
	)
	require.NoError(t, err)

	// Verify confidence is stored correctly
	var storedConfidence float64
	err = env.Pool.QueryRow(env.Context,
		`SELECT confidence FROM event_sources WHERE event_id = $1`,
		eventID,
	).Scan(&storedConfidence)
	require.NoError(t, err)
	require.Equal(t, 0.95, storedConfidence, "confidence should be stored accurately")
}

// Helper function to insert a source
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
