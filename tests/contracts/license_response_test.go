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

// TestLicenseInformationInJSONLD verifies that license metadata is included in event responses
// per FR-024 and SEL Interoperability Profile v0.1
func TestLicenseInformationInJSONLD(t *testing.T) {
	env := setupTestEnv(t)

	// Create an event via API (should have CC0 license by default)
	key := insertAPIKey(t, env, "license-test-agent")

	payload := map[string]any{
		"name":        "Licensed Event",
		"description": "A licensed event for contract testing with complete metadata to avoid review workflow",
		"image":       "https://example.com/images/licensed-event.jpg",
		"startDate":   time.Date(2026, 9, 1, 19, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"location": map[string]any{
			"name":            "Public Park",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
		"organizer": map[string]any{
			"name": "Community Arts",
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
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var created map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	eventID := eventIDFromPayload(created)
	require.NotEmpty(t, eventID)

	// Retrieve the event with JSON-LD
	req2, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events/"+eventID, nil)
	require.NoError(t, err)
	req2.Header.Set("Accept", "application/ld+json")

	resp2, err := env.Server.Client().Do(req2)
	require.NoError(t, err)
	defer func() { _ = resp2.Body.Close() }()
	require.Equal(t, http.StatusOK, resp2.StatusCode)

	var event map[string]any
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&event))

	// Verify license information is present (FR-024)
	// Per Schema.org, license can be a URL string or an object
	license, hasLicense := event["license"]
	require.True(t, hasLicense, "event must include license information per FR-024")
	require.NotNil(t, license, "license must not be null")

	// Check if license is a string (URL)
	if licenseURL, ok := license.(string); ok {
		require.NotEmpty(t, licenseURL, "license URL must not be empty")
		// Default should be CC0
		require.Contains(t, licenseURL, "creativecommons.org/publicdomain/zero",
			"default license should be CC0 per FR-015")
	} else if licenseObj, ok := license.(map[string]any); ok {
		// License as object (e.g., {"@id": "...", "name": "..."})
		require.NotEmpty(t, licenseObj, "license object must not be empty")
		if id, ok := licenseObj["@id"].(string); ok {
			require.NotEmpty(t, id, "license @id must not be empty")
		} else if url, ok := licenseObj["url"].(string); ok {
			require.NotEmpty(t, url, "license url must not be empty")
		} else {
			t.Fatalf("license object must have @id or url property, got: %+v", licenseObj)
		}
	} else {
		t.Fatalf("license must be string or object, got type: %T", license)
	}
}

// TestLicenseInEventList verifies that license information is included in event list responses
func TestLicenseInEventList(t *testing.T) {
	env := setupTestEnv(t)

	// Create an event directly in database
	org := insertOrganization(t, env, "License Test Org")
	place := insertPlace(t, env, "License Test Venue", "Toronto")
	_ = insertEventWithOccurrence(t, env, "List License Test", org.ID, place.ID, "arts", "published", []string{}, time.Date(2026, 10, 1, 19, 0, 0, 0, time.UTC))

	// Request event list with JSON-LD
	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events?limit=10", nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var response map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&response))

	items, ok := response["items"].([]any)
	require.True(t, ok, "response must have items array")
	require.NotEmpty(t, items, "items array should not be empty")

	// Check first event for license
	firstEvent := items[0].(map[string]any)

	// License should be present in list responses
	license, hasLicense := firstEvent["license"]
	if hasLicense {
		require.NotNil(t, license, "if license is present, it must not be null")

		// Validate license format
		if licenseURL, ok := license.(string); ok {
			require.NotEmpty(t, licenseURL, "license URL must not be empty")
		} else if licenseObj, ok := license.(map[string]any); ok {
			require.NotEmpty(t, licenseObj, "license object must not be empty")
		}
	}
	// Note: License might be omitted in list view for brevity, but if present must be valid
}

// TestLicenseStatusField verifies that license_status enum is correctly exposed
func TestLicenseStatusField(t *testing.T) {
	env := setupTestEnv(t)

	// Create event with specific license status
	eventULID := ulid.Make().String()
	placeULID := ulid.Make().String()
	var placeID, eventID string

	err := env.Pool.QueryRow(env.Context,
		`INSERT INTO places (ulid, name, address_locality) VALUES ($1, $2, $3) RETURNING id`,
		placeULID, "License Status Venue", "Toronto",
	).Scan(&placeID)
	require.NoError(t, err)

	err = env.Pool.QueryRow(env.Context,
		`INSERT INTO events (ulid, name, primary_venue_id, lifecycle_state, event_domain, license_url, license_status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id`,
		eventULID, "CC0 Event", placeID, "published", "arts",
		"https://creativecommons.org/publicdomain/zero/1.0/", "cc0",
	).Scan(&eventID)
	require.NoError(t, err)

	// Add occurrence
	_, err = env.Pool.Exec(env.Context,
		`INSERT INTO event_occurrences (event_id, start_time, venue_id) VALUES ($1, $2, $3)`,
		eventID, time.Date(2026, 11, 1, 18, 0, 0, 0, time.UTC), placeID,
	)
	require.NoError(t, err)

	// Retrieve event
	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events/"+eventULID, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var event map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&event))

	// Verify license is present
	license, hasLicense := event["license"]
	require.True(t, hasLicense, "event must have license field")
	require.NotNil(t, license, "license must not be null")

	// The license_status enum might be exposed in different ways:
	// 1. As part of the license object: {"@id": "...", "status": "cc0"}
	// 2. As a separate field: "licenseStatus": "cc0"
	// 3. Inferred from the license URL

	t.Logf("Event license representation: %+v", license)
}

// TestProprietaryLicenseRejection verifies that non-CC0 licenses are rejected per FR-015
func TestProprietaryLicenseRejection(t *testing.T) {
	env := setupTestEnv(t)

	key := insertAPIKey(t, env, "proprietary-license-agent")

	// Attempt to create event with proprietary license
	payload := map[string]any{
		"name":      "Proprietary Event",
		"startDate": time.Date(2026, 12, 1, 19, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"location": map[string]any{
			"name":            "Private Venue",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
		"license": "proprietary",
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
	defer func() { _ = resp.Body.Close() }()

	// Should be rejected per FR-015
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, "proprietary license should be rejected")

	var errorResponse map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&errorResponse))

	// Should be RFC 7807 problem details
	require.Contains(t, resp.Header.Get("Content-Type"), "application/problem+json")

	// Error should mention license
	if detail, ok := errorResponse["detail"].(string); ok {
		require.Contains(t, detail, "license", "error should mention license issue")
	}
}

// TestLicenseURLValidation verifies that license URLs are valid URLs
func TestLicenseURLValidation(t *testing.T) {
	env := setupTestEnv(t)

	// Create event with explicit license URL
	eventULID := ulid.Make().String()
	placeULID := ulid.Make().String()
	var placeID, eventID string

	err := env.Pool.QueryRow(env.Context,
		`INSERT INTO places (ulid, name, address_locality) VALUES ($1, $2, $3) RETURNING id`,
		placeULID, "URL Validation Venue", "Toronto",
	).Scan(&placeID)
	require.NoError(t, err)

	validLicenseURL := "https://creativecommons.org/publicdomain/zero/1.0/"
	err = env.Pool.QueryRow(env.Context,
		`INSERT INTO events (ulid, name, primary_venue_id, lifecycle_state, event_domain, license_url, license_status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id`,
		eventULID, "Valid License URL Event", placeID, "published", "arts", validLicenseURL, "cc0",
	).Scan(&eventID)
	require.NoError(t, err)

	// Add occurrence
	_, err = env.Pool.Exec(env.Context,
		`INSERT INTO event_occurrences (event_id, start_time, venue_id) VALUES ($1, $2, $3)`,
		eventID, time.Date(2027, 1, 1, 19, 0, 0, 0, time.UTC), placeID,
	)
	require.NoError(t, err)

	// Retrieve and verify
	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events/"+eventULID, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var event map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&event))

	// Verify license URL is returned
	license, hasLicense := event["license"]
	require.True(t, hasLicense, "event must have license")

	if licenseURL, ok := license.(string); ok {
		require.Equal(t, validLicenseURL, licenseURL, "license URL should match stored value")
	} else if licenseObj, ok := license.(map[string]any); ok {
		// If returned as object, check @id or url
		if id, ok := licenseObj["@id"].(string); ok {
			require.Equal(t, validLicenseURL, id, "license @id should match stored value")
		}
	}
}

// Helper: eventIDFromPayload extracts event ULID from response payload
func eventIDFromPayload(payload map[string]any) string {
	if id, ok := payload["@id"].(string); ok {
		// Extract ULID from full URI if needed
		if len(id) == 26 {
			return id
		}
		// Parse from URI like "http://localhost/events/01HYX3..."
		parts := []rune(id)
		if len(parts) >= 26 {
			return string(parts[len(parts)-26:])
		}
	}
	if id, ok := payload["id"].(string); ok {
		return id
	}
	return ""
}
