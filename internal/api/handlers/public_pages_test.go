package handlers

import (
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/stretchr/testify/require"
)

// TestBuildEventPayloadAllDayRendering verifies the public /events/{id} payload
// (used for application/ld+json, text/turtle, and the embedded HTML JSON-LD)
// renders an all-day occurrence as date-only in the occurrence's own zone and
// exposes the allDay marker.
func TestBuildEventPayloadAllDayRendering(t *testing.T) {
	toronto, err := time.LoadLocation("America/Toronto")
	require.NoError(t, err)
	paris, err := time.LoadLocation("Europe/Paris")
	require.NoError(t, err)

	// Paris midnight on 2026-11-01 = 2026-10-31T23:00:00Z. Rendering in the node
	// zone would shift the date back to 2026-10-31.
	midnight := time.Date(2026, 11, 1, 0, 0, 0, 0, paris)

	payload := buildEventPayload(&events.Event{
		ULID: "01HX1234567890ABCDEFGHJKMN",
		Name: "All Day Exhibition",
		Occurrences: []events.Occurrence{
			{StartTime: midnight, IsAllDay: true, Timezone: "Europe/Paris"},
		},
	}, "https://example.org", toronto)

	require.Equal(t, "2026-11-01", payload["startDate"])
	require.Equal(t, true, payload["allDay"])
}

// TestBuildEventPayloadTimedRendering verifies a timed occurrence still renders
// a full RFC 3339 instant with no allDay marker.
func TestBuildEventPayloadTimedRendering(t *testing.T) {
	toronto, err := time.LoadLocation("America/Toronto")
	require.NoError(t, err)

	start := time.Date(2026, 11, 1, 19, 0, 0, 0, toronto)

	payload := buildEventPayload(&events.Event{
		ULID: "01HX1234567890ABCDEFGHJKMN",
		Name: "Evening Show",
		Occurrences: []events.Occurrence{
			{StartTime: start, Timezone: "America/Toronto"},
		},
	}, "https://example.org", toronto)

	require.Equal(t, "2026-11-01T19:00:00-05:00", payload["startDate"])
	_, hasAllDay := payload["allDay"]
	require.False(t, hasAllDay, "timed occurrence must not expose allDay")
}
