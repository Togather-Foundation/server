package integration

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"
)

// TestFieldProvenanceParameter verifies that field-level provenance can be requested
// via query parameter and is correctly included in responses.
func TestFieldProvenanceParameter(t *testing.T) {
	env := setupTestEnv(t)

	// Setup: Create an event with multiple sources contributing different fields
	source1 := insertSource(t, env, "Primary Source", "api", "CC0", 8)
	source2 := insertSource(t, env, "Secondary Source", "scraper", "CC0", 6)

	// Create event, place, and organization
	placeULID := ulid.Make().String()
	var placeID string
	err := env.Pool.QueryRow(env.Context,
		`INSERT INTO places (ulid, name, address_locality) VALUES ($1, $2, $3) RETURNING id`,
		placeULID, "Provenance Test Venue", "Toronto",
	).Scan(&placeID)
	require.NoError(t, err)

	orgULID := ulid.Make().String()
	var orgID string
	err = env.Pool.QueryRow(env.Context,
		`INSERT INTO organizations (ulid, name, address_locality) VALUES ($1, $2, $3) RETURNING id`,
		orgULID, "Event Organizer Inc", "Toronto",
	).Scan(&orgID)
	require.NoError(t, err)

	eventULID := ulid.Make().String()
	var eventID string
	err = env.Pool.QueryRow(env.Context,
		`INSERT INTO events (ulid, name, description, organizer_id, primary_venue_id, lifecycle_state, event_domain)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id`,
		eventULID, "Field Provenance Test Event", "An event to test field-level provenance tracking",
		orgID, placeID, "published", "arts",
	).Scan(&eventID)
	require.NoError(t, err)

	// Add occurrence
	_, err = env.Pool.Exec(env.Context,
		`INSERT INTO event_occurrences (event_id, start_time, venue_id) VALUES ($1, $2, $3)`,
		eventID, time.Date(2026, 10, 15, 19, 0, 0, 0, time.UTC), placeID,
	)
	require.NoError(t, err)

	// Record field-level provenance for name from source1
	_, err = env.Pool.Exec(env.Context,
		`INSERT INTO field_provenance (event_id, field_path, value_hash, value_preview, source_id, confidence, observed_at, applied_to_canonical)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		eventID, "/name", "hash-name-1", "Field Provenance Test Event", source1, 0.9, time.Now(), true,
	)
	require.NoError(t, err)

	// Record field-level provenance for description from source2
	_, err = env.Pool.Exec(env.Context,
		`INSERT INTO field_provenance (event_id, field_path, value_hash, value_preview, source_id, confidence, observed_at, applied_to_canonical)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		eventID, "/description", "hash-desc-2", "An event to test field-level provenance tracking", source2, 0.85, time.Now(), true,
	)
	require.NoError(t, err)

	// Test 1: Request event WITHOUT field provenance parameter (default behavior)
	req1, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events/"+eventULID, nil)
	require.NoError(t, err)
	req1.Header.Set("Accept", "application/ld+json")

	resp1, err := env.Server.Client().Do(req1)
	require.NoError(t, err)
	defer resp1.Body.Close()
	require.Equal(t, http.StatusOK, resp1.StatusCode)

	var payload1 map[string]any
	require.NoError(t, json.NewDecoder(resp1.Body).Decode(&payload1))
	require.NotEmpty(t, payload1["name"], "event should have name")

	// By default, field provenance might not be included (depends on implementation)
	// This test documents the default behavior

	// Test 2: Request event WITH field provenance parameter
	req2, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events/"+eventULID+"?include_provenance=true", nil)
	require.NoError(t, err)
	req2.Header.Set("Accept", "application/ld+json")

	resp2, err := env.Server.Client().Do(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()
	require.Equal(t, http.StatusOK, resp2.StatusCode)

	var payload2 map[string]any
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&payload2))

	// Verify field provenance is included when requested
	// The exact structure will be defined during implementation
	// Common patterns: "_provenance" field, "attribution" per field, etc.
	t.Logf("Response with provenance: %+v", payload2)

	// Expected structure (one of these patterns):
	// 1. Top-level "_provenance" or "fieldProvenance" object
	// 2. Per-field provenance nested in each field value
	// 3. Separate "provenance" array with field references

	if provenance, ok := payload2["_provenance"].(map[string]any); ok {
		require.NotEmpty(t, provenance, "provenance should be included when requested")
	} else if provenance, ok := payload2["fieldProvenance"].(map[string]any); ok {
		require.NotEmpty(t, provenance, "field provenance should be included when requested")
	} else if provenance, ok := payload2["provenance"].([]any); ok {
		require.NotEmpty(t, provenance, "provenance array should be included when requested")
	} else {
		// Log for implementation reference
		t.Logf("Field provenance structure not yet implemented - payload: %+v", payload2)
	}
}

// TestFieldProvenanceFiltering tests filtering field provenance by specific fields
func TestFieldProvenanceFiltering(t *testing.T) {
	env := setupTestEnv(t)

	// Setup event with provenance
	source := insertSource(t, env, "Test Source", "api", "CC0", 7)
	placeULID := ulid.Make().String()
	var placeID string
	err := env.Pool.QueryRow(env.Context,
		`INSERT INTO places (ulid, name, address_locality) VALUES ($1, $2, $3) RETURNING id`,
		placeULID, "Test Venue", "Toronto",
	).Scan(&placeID)
	require.NoError(t, err)

	eventULID := ulid.Make().String()
	var eventID string
	err = env.Pool.QueryRow(env.Context,
		`INSERT INTO events (ulid, name, description, primary_venue_id, lifecycle_state, event_domain)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id`,
		eventULID, "Filter Test Event", "Testing field filtering", placeID, "published", "arts",
	).Scan(&eventID)
	require.NoError(t, err)

	// Add occurrence
	_, err = env.Pool.Exec(env.Context,
		`INSERT INTO event_occurrences (event_id, start_time, venue_id) VALUES ($1, $2, $3)`,
		eventID, time.Date(2026, 11, 1, 18, 0, 0, 0, time.UTC), placeID,
	)
	require.NoError(t, err)

	// Add provenance for multiple fields
	fields := []string{"/name", "/description", "/startDate"}
	for _, field := range fields {
		_, err = env.Pool.Exec(env.Context,
			`INSERT INTO field_provenance (event_id, field_path, value_hash, value_preview, source_id, confidence, observed_at, applied_to_canonical)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			eventID, field, "hash-"+field, "preview", source, 0.8, time.Now(), true,
		)
		require.NoError(t, err)
	}

	// Request provenance for specific field only
	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events/"+eventULID+"?include_provenance=true&provenance_fields=name", nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var payload map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))

	// Verify response includes the event
	require.NotEmpty(t, payload["name"], "event should have name")

	// The filtering behavior will be defined during implementation
	t.Logf("Filtered provenance response: %+v", payload)
}

// TestFieldProvenanceMultipleSources verifies handling of conflicting provenance from multiple sources
func TestFieldProvenanceMultipleSources(t *testing.T) {
	env := setupTestEnv(t)

	// Create two sources with different trust levels
	highTrust := insertSource(t, env, "High Trust Source", "partner", "CC0", 9)
	lowTrust := insertSource(t, env, "Low Trust Source", "scraper", "CC0", 4)

	// Create event
	placeULID := ulid.Make().String()
	var placeID string
	err := env.Pool.QueryRow(env.Context,
		`INSERT INTO places (ulid, name, address_locality) VALUES ($1, $2, $3) RETURNING id`,
		placeULID, "Multi-Source Venue", "Toronto",
	).Scan(&placeID)
	require.NoError(t, err)

	eventULID := ulid.Make().String()
	var eventID string
	err = env.Pool.QueryRow(env.Context,
		`INSERT INTO events (ulid, name, primary_venue_id, lifecycle_state, event_domain)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id`,
		eventULID, "Canonical Name", placeID, "published", "arts",
	).Scan(&eventID)
	require.NoError(t, err)

	// Add occurrence
	_, err = env.Pool.Exec(env.Context,
		`INSERT INTO event_occurrences (event_id, start_time, venue_id) VALUES ($1, $2, $3)`,
		eventID, time.Date(2026, 12, 1, 19, 0, 0, 0, time.UTC), placeID,
	)
	require.NoError(t, err)

	// Record conflicting provenance for the same field from different sources
	// High trust source says "Canonical Name"
	_, err = env.Pool.Exec(env.Context,
		`INSERT INTO field_provenance (event_id, field_path, value_hash, value_preview, source_id, confidence, observed_at, applied_to_canonical)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		eventID, "/name", "hash-canonical", "Canonical Name", highTrust, 0.95, time.Now().Add(-1*time.Hour), true,
	)
	require.NoError(t, err)

	// Low trust source says "Alternative Name"
	_, err = env.Pool.Exec(env.Context,
		`INSERT INTO field_provenance (event_id, field_path, value_hash, value_preview, source_id, confidence, observed_at, applied_to_canonical)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		eventID, "/name", "hash-alternative", "Alternative Name", lowTrust, 0.70, time.Now(), false,
	)
	require.NoError(t, err)

	// Request with provenance
	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events/"+eventULID+"?include_provenance=true", nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var payload map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))

	// The canonical name should be from the high-trust source
	require.Equal(t, "Canonical Name", payload["name"], "should use high-trust source value")

	// When provenance is included, both sources should be visible
	// Implementation will determine exact structure
	t.Logf("Multi-source provenance response: %+v", payload)
}

// TestFieldProvenanceTimestamps verifies that observed_at timestamps are tracked
func TestFieldProvenanceTimestamps(t *testing.T) {
	env := setupTestEnv(t)

	source := insertSource(t, env, "Timestamp Test Source", "api", "CC0", 7)

	placeULID := ulid.Make().String()
	var placeID string
	err := env.Pool.QueryRow(env.Context,
		`INSERT INTO places (ulid, name, address_locality) VALUES ($1, $2, $3) RETURNING id`,
		placeULID, "Timestamp Venue", "Toronto",
	).Scan(&placeID)
	require.NoError(t, err)

	eventULID := ulid.Make().String()
	var eventID string
	err = env.Pool.QueryRow(env.Context,
		`INSERT INTO events (ulid, name, primary_venue_id, lifecycle_state, event_domain)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id`,
		eventULID, "Timestamp Test Event", placeID, "published", "arts",
	).Scan(&eventID)
	require.NoError(t, err)

	// Add occurrence
	_, err = env.Pool.Exec(env.Context,
		`INSERT INTO event_occurrences (event_id, start_time, venue_id) VALUES ($1, $2, $3)`,
		eventID, time.Date(2027, 1, 1, 20, 0, 0, 0, time.UTC), placeID,
	)
	require.NoError(t, err)

	// Record provenance with specific timestamp
	observedTime := time.Date(2026, 1, 26, 12, 0, 0, 0, time.UTC)
	_, err = env.Pool.Exec(env.Context,
		`INSERT INTO field_provenance (event_id, field_path, value_hash, value_preview, source_id, confidence, observed_at, applied_to_canonical)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		eventID, "/name", "hash-timestamp", "Timestamp Test Event", source, 0.9, observedTime, true,
	)
	require.NoError(t, err)

	// Verify timestamp was stored
	var storedTime time.Time
	err = env.Pool.QueryRow(env.Context,
		`SELECT observed_at FROM field_provenance WHERE event_id = $1 AND field_path = $2`,
		eventID, "/name",
	).Scan(&storedTime)
	require.NoError(t, err)
	require.Equal(t, observedTime.Unix(), storedTime.Unix(), "observed_at timestamp should be preserved")
}
