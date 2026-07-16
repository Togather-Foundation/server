package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"
)

func TestICSFeed(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Toronto Arts Org")
	place := insertPlace(t, env, "Centennial Park", "Toronto")
	_ = insertEventWithOccurrence(t, env, "Jazz in the Park", org.ID, place.ID, "music", "published", []string{"jazz"}, time.Now().AddDate(0, 0, 1))

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events.ics", nil)
	require.NoError(t, err)

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "text/calendar; charset=utf-8", resp.Header.Get("Content-Type"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	ics := string(body)

	require.Contains(t, ics, "BEGIN:VCALENDAR", "ICS feed should contain VCALENDAR")
	require.Contains(t, ics, "END:VCALENDAR", "ICS feed should close VCALENDAR")
	require.Contains(t, ics, "BEGIN:VEVENT", "ICS feed should contain VEVENT")
	require.Contains(t, ics, "END:VEVENT", "ICS feed should close VEVENT")
	require.Contains(t, ics, "Jazz in the Park", "ICS feed should contain event name")
}

func TestICSFeedDTSTAMP(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Toronto Arts Org")
	place := insertPlace(t, env, "Centennial Park", "Toronto")
	_ = insertEventWithOccurrence(t, env, "Jazz in the Park", org.ID, place.ID, "music", "published", []string{"jazz"}, time.Now().AddDate(0, 0, 1))

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events.ics", nil)
	require.NoError(t, err)

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	ics := string(body)

	require.Contains(t, ics, "DTSTAMP:", "ICS feed should contain DTSTAMP in VEVENT")
}

func TestICSFeedPaginationLink(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Toronto Arts Org")
	place := insertPlace(t, env, "Centennial Park", "Toronto")

	for i := 0; i < 5; i++ {
		_ = insertEventWithOccurrence(t, env, "Jazz in the Park", org.ID, place.ID, "music", "published", []string{"jazz"}, time.Now().AddDate(0, 0, 1+i))
	}

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events.ics?limit=1", nil)
	require.NoError(t, err)

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusOK, resp.StatusCode)

	link := resp.Header.Get("Link")
	require.NotEmpty(t, link, "Link header should be present for paginated results")
	require.Contains(t, link, `rel="next"`, "Link header should have rel=next")
	require.Contains(t, link, "/api/v1/events.ics", "Link header should point to ICS feed URL")
}

func TestICSSingleEvent(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Toronto Arts Org")
	place := insertPlace(t, env, "Centennial Park", "Toronto")
	eventName := "Jazz in the Park"
	eventULID := insertEventWithOccurrence(t, env, eventName, org.ID, place.ID, "music", "published", []string{"jazz"}, time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC))

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events/"+eventULID+"/ics", nil)
	require.NoError(t, err)

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "text/calendar; charset=utf-8", resp.Header.Get("Content-Type"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	ics := string(body)

	require.Contains(t, ics, "BEGIN:VCALENDAR", "ICS should contain VCALENDAR")
	require.Contains(t, ics, "END:VCALENDAR", "ICS should close VCALENDAR")
	require.Contains(t, ics, "BEGIN:VEVENT", "ICS should contain VEVENT")
	require.Contains(t, ics, "END:VEVENT", "ICS should close VEVENT")
	require.Contains(t, ics, "Jazz in the Park", "ICS should contain event name")
	require.Contains(t, ics, "DTSTAMP:", "ICS single event should contain DTSTAMP")
}

func TestICSSingleEventNotFound(t *testing.T) {
	env := setupTestEnv(t)

	// Use a valid ULID that doesn't exist in the database
	nonExistentULID := ulid.Make().String()
	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events/"+nonExistentULID+"/ics", nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	var payload map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	require.Equal(t, "https://sel.events/problems/not-found", payload["type"])
}

func TestICSSingleEventGone(t *testing.T) {
	env := setupTestEnv(t)

	username := "admin"
	password := "admin-password-123"
	email := "admin@example.com"
	insertAdminUser(t, env, username, password, email, "admin")
	adminToken := adminLogin(t, env, username, password)

	agentKey := insertAPIKey(t, env, "test-agent")

	event := map[string]any{
		"name":        "Event to Delete",
		"description": "This event will be deleted",
		"startDate":   time.Date(2026, 11, 1, 19, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"location": map[string]any{
			"name":            "Test Venue",
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
		},
		"source": map[string]any{
			"url":     "https://example.com/events/delete-test",
			"eventId": "delete-test-123",
		},
	}

	body, err := json.Marshal(event)
	require.NoError(t, err)

	createReq, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
	require.NoError(t, err)
	createReq.Header.Set("Authorization", "Bearer "+agentKey)
	createReq.Header.Set("Content-Type", "application/ld+json")

	createResp, err := env.Server.Client().Do(createReq)
	require.NoError(t, err)
	defer func() { _ = createResp.Body.Close() }()

	var created map[string]any
	require.NoError(t, json.NewDecoder(createResp.Body).Decode(&created))
	eventID := eventIDFromPayload(created)
	require.NotEmpty(t, eventID)

	deleteReq, err := http.NewRequest(http.MethodDelete, env.Server.URL+"/api/v1/admin/events/"+eventID, nil)
	require.NoError(t, err)
	deleteReq.Header.Set("Authorization", "Bearer "+adminToken)

	deleteResp, err := env.Server.Client().Do(deleteReq)
	require.NoError(t, err)
	defer func() { _ = deleteResp.Body.Close() }()
	require.Equal(t, http.StatusNoContent, deleteResp.StatusCode)

	icsReq, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events/"+eventID+"/ics", nil)
	require.NoError(t, err)
	icsReq.Header.Set("Accept", "application/ld+json")

	icsResp, err := env.Server.Client().Do(icsReq)
	require.NoError(t, err)
	defer func() { _ = icsResp.Body.Close() }()

	require.Equal(t, http.StatusGone, icsResp.StatusCode)

	var tombstone map[string]any
	require.NoError(t, json.NewDecoder(icsResp.Body).Decode(&tombstone))
	require.Equal(t, true, tombstone["sel:tombstone"])
}

func TestEventsListLinkHeader(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Toronto Arts Org")
	place := insertPlace(t, env, "Centennial Park", "Toronto")
	_ = insertEventWithOccurrence(t, env, "Jazz in the Park", org.ID, place.ID, "music", "published", []string{"jazz"}, time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC))

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events", nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusOK, resp.StatusCode)

	link := resp.Header.Get("Link")
	require.NotEmpty(t, link, "Link header should be present for event list")
	require.Contains(t, link, `rel="alternate"`, "Link header should have rel=alternate")
	require.Contains(t, link, "/api/v1/events.ics", "Link header should point to ICS feed URL")
}

func TestEventsGetLinkHeader(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Toronto Arts Org")
	place := insertPlace(t, env, "Centennial Park", "Toronto")
	eventName := "Jazz in the Park"
	eventULID := insertEventWithOccurrence(t, env, eventName, org.ID, place.ID, "music", "published", []string{"jazz"}, time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC))

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events/"+eventULID, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusOK, resp.StatusCode)

	link := resp.Header.Get("Link")
	require.NotEmpty(t, link, "Link header should be present for single event")
	require.Contains(t, link, `rel="alternate"`, "Link header should have rel=alternate")
	require.Contains(t, link, "/api/v1/events/"+eventULID+"/ics", "Link header should point to single event ICS URL")
}

func TestICSFeedEmpty(t *testing.T) {
	env := setupTestEnv(t)

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events.ics", nil)
	require.NoError(t, err)

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "text/calendar; charset=utf-8", resp.Header.Get("Content-Type"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	ics := string(body)

	require.Contains(t, ics, "BEGIN:VCALENDAR", "Empty ICS feed should still contain VCALENDAR")
	require.Contains(t, ics, "END:VCALENDAR", "Empty ICS feed should close VCALENDAR")
	require.NotContains(t, ics, "BEGIN:VEVENT", "Empty ICS feed should not contain VEVENT")
}

func TestICSSingleEventContentDisposition(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Toronto Arts Org")
	place := insertPlace(t, env, "Centennial Park", "Toronto")
	eventULID := insertEventWithOccurrence(t, env, "Jazz in the Park", org.ID, place.ID, "music", "published", []string{"jazz"}, time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC))

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events/"+eventULID+"/ics", nil)
	require.NoError(t, err)

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusOK, resp.StatusCode)

	disposition := resp.Header.Get("Content-Disposition")
	require.Contains(t, disposition, "attachment", "Content-Disposition should be attachment")
	require.Contains(t, disposition, ".ics", "Content-Disposition should suggest ICS filename")
}
