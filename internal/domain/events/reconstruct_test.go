package events

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReconstructPayloadFromEvent(t *testing.T) {
	t.Parallel()

	t.Run("nil event returns empty JSON", func(t *testing.T) {
		t.Parallel()

		data, err := reconstructPayloadFromEvent(nil)
		require.Error(t, err)
		assert.Equal(t, []byte("{}"), data)
	})

	t.Run("fully populated event contains all fields", func(t *testing.T) {
		t.Parallel()

		free := true
		start := time.Date(2026, 3, 15, 19, 0, 0, 0, time.UTC)
		end := time.Date(2026, 3, 15, 22, 0, 0, 0, time.UTC)
		door := time.Date(2026, 3, 15, 18, 30, 0, 0, time.UTC)
		priceMin := 10.0
		priceMax := 25.0
		virtualOcc := "https://zoom.example.com/occ"

		venueID := "venue-uuid-001"
		venueULID := "01VENUEULIDX"
		organizerID := "org-uuid-001"

		event := &Event{
			ID:                  "event-uuid-001",
			ULID:                "01ABCDE",
			Name:                "Jazz Night at the Rex",
			Description:         "A wonderful jazz evening",
			ImageURL:            "https://example.com/image.jpg",
			PublicURL:           "https://example.com/event",
			VirtualURL:          "https://example.com/virtual",
			Keywords:            []string{"jazz", "music"},
			InLanguage:          []string{"en"},
			IsAccessibleForFree: &free,
			AttendanceMode:      "mixed",
			EventStatus:         "scheduled",
			EventDomain:         "music",
			DedupHash:           "abc123hash",
			LifecycleState:      "published",
			PrimaryVenueID:      &venueID,
			PrimaryVenueULID:    &venueULID,
			OrganizerID:         &organizerID,
			Occurrences: []Occurrence{
				{
					StartTime:     start,
					EndTime:       &end,
					Timezone:      "America/Toronto",
					DoorTime:      &door,
					VirtualURL:    &virtualOcc,
					TicketURL:     "https://tickets.example.com",
					PriceMin:      &priceMin,
					PriceMax:      &priceMax,
					PriceCurrency: "CAD",
					Availability:  "in_stock",
				},
			},
		}

		data, err := reconstructPayloadFromEvent(event)
		require.NoError(t, err)

		var result map[string]any
		require.NoError(t, json.Unmarshal(data, &result))

		assert.Equal(t, true, result["_reconstructed"])
		assert.Equal(t, "Jazz Night at the Rex", result["name"])
		assert.Equal(t, "A wonderful jazz evening", result["description"])
		assert.Equal(t, "https://example.com/image.jpg", result["image"])
		assert.Equal(t, "https://example.com/event", result["url"])
		assert.Equal(t, "https://example.com/virtual", result["virtual_url"])
		assert.Equal(t, []any{"jazz", "music"}, result["keywords"])
		assert.Equal(t, []any{"en"}, result["in_language"])
		assert.Equal(t, true, result["is_accessible_for_free"])
		assert.Equal(t, "mixed", result["attendance_mode"])
		assert.Equal(t, "scheduled", result["event_status"])
		assert.Equal(t, "music", result["event_domain"])
		assert.Equal(t, "01ABCDE", result["ulid"])
		assert.Equal(t, "published", result["lifecycle_state"])
		assert.Equal(t, "abc123hash", result["dedup_hash"])
		assert.Equal(t, "venue-uuid-001", result["primary_venue_id"])
		assert.Equal(t, "01VENUEULIDX", result["primary_venue_ulid"])
		assert.Equal(t, "org-uuid-001", result["organizer_id"])

		occs, ok := result["occurrences"].([]any)
		require.True(t, ok, "occurrences should be a slice")
		require.Len(t, occs, 1)

		occ := occs[0].(map[string]any)
		assert.Equal(t, start.Format(time.RFC3339), occ["start_date"])
		assert.Equal(t, end.Format(time.RFC3339), occ["end_date"])
		assert.Equal(t, "America/Toronto", occ["timezone"])
		assert.Equal(t, door.Format(time.RFC3339), occ["door_time"])
		assert.Equal(t, "https://zoom.example.com/occ", occ["virtual_url"])
		assert.Equal(t, "https://tickets.example.com", occ["ticket_url"])
		assert.InDelta(t, 10.0, occ["price_min"], 0.001)
		assert.InDelta(t, 25.0, occ["price_max"], 0.001)
		assert.Equal(t, "CAD", occ["price_currency"])
		assert.Equal(t, "in_stock", occ["availability"])
	})

	t.Run("minimal event omits optional fields", func(t *testing.T) {
		t.Parallel()

		event := &Event{
			ID:             "event-uuid-002",
			ULID:           "01MINIMAL",
			Name:           "Minimal Event",
			LifecycleState: "published",
		}

		data, err := reconstructPayloadFromEvent(event)
		require.NoError(t, err)

		var result map[string]any
		require.NoError(t, json.Unmarshal(data, &result))

		assert.Equal(t, true, result["_reconstructed"])
		assert.Equal(t, "Minimal Event", result["name"])
		assert.Equal(t, "01MINIMAL", result["ulid"])
		assert.Equal(t, "published", result["lifecycle_state"])

		// Optional fields must be absent
		assert.NotContains(t, result, "description")
		assert.NotContains(t, result, "image")
		assert.NotContains(t, result, "url")
		assert.NotContains(t, result, "virtual_url")
		assert.NotContains(t, result, "keywords")
		assert.NotContains(t, result, "in_language")
		assert.NotContains(t, result, "is_accessible_for_free")
		assert.NotContains(t, result, "attendance_mode")
		assert.NotContains(t, result, "event_status")
		assert.NotContains(t, result, "event_domain")
		assert.NotContains(t, result, "dedup_hash")
		assert.NotContains(t, result, "occurrences")
		assert.NotContains(t, result, "primary_venue_id")
		assert.NotContains(t, result, "primary_venue_ulid")
		assert.NotContains(t, result, "organizer_id")
	})

	t.Run("multiple occurrences all included", func(t *testing.T) {
		t.Parallel()

		start1 := time.Date(2026, 4, 1, 19, 0, 0, 0, time.UTC)
		start2 := time.Date(2026, 4, 8, 19, 0, 0, 0, time.UTC)

		event := &Event{
			ID:             "event-uuid-003",
			ULID:           "01MULTI",
			Name:           "Weekly Jazz",
			LifecycleState: "published",
			Occurrences: []Occurrence{
				{StartTime: start1},
				{StartTime: start2},
			},
		}

		data, err := reconstructPayloadFromEvent(event)
		require.NoError(t, err)

		var result map[string]any
		require.NoError(t, json.Unmarshal(data, &result))

		occs, ok := result["occurrences"].([]any)
		require.True(t, ok)
		assert.Len(t, occs, 2)

		occ0 := occs[0].(map[string]any)
		occ1 := occs[1].(map[string]any)
		assert.Equal(t, start1.Format(time.RFC3339), occ0["start_date"])
		assert.Equal(t, start2.Format(time.RFC3339), occ1["start_date"])

		// No optional fields on sparse occurrences
		assert.NotContains(t, occ0, "end_date")
		assert.NotContains(t, occ0, "timezone")
		assert.NotContains(t, occ0, "door_time")
		assert.NotContains(t, occ0, "virtual_url")
		assert.NotContains(t, occ0, "ticket_url")
		assert.NotContains(t, occ0, "price_min")
		assert.NotContains(t, occ0, "price_max")
		assert.NotContains(t, occ0, "price_currency")
		assert.NotContains(t, occ0, "availability")
	})

	t.Run("nil pointer fields cause no panic and are omitted", func(t *testing.T) {
		t.Parallel()

		event := &Event{
			ID:                  "event-uuid-004",
			ULID:                "01NILPTR",
			Name:                "Nil Pointer Event",
			LifecycleState:      "pending_review",
			IsAccessibleForFree: nil, // nil pointer — must be omitted
			Occurrences: []Occurrence{
				{
					StartTime:  time.Now(),
					EndTime:    nil, // nil pointer
					DoorTime:   nil, // nil pointer
					VirtualURL: nil, // nil pointer
					PriceMin:   nil, // nil pointer
					PriceMax:   nil, // nil pointer
				},
			},
		}

		// Must not panic
		data, err := reconstructPayloadFromEvent(event)
		require.NoError(t, err)

		var result map[string]any
		require.NoError(t, json.Unmarshal(data, &result))

		assert.NotContains(t, result, "is_accessible_for_free")

		occs, ok := result["occurrences"].([]any)
		require.True(t, ok)
		require.Len(t, occs, 1)
		occ := occs[0].(map[string]any)
		assert.NotContains(t, occ, "end_date")
		assert.NotContains(t, occ, "door_time")
		assert.NotContains(t, occ, "virtual_url")
		assert.NotContains(t, occ, "price_min")
		assert.NotContains(t, occ, "price_max")
	})

	t.Run("occurrence with empty virtual_url string is omitted", func(t *testing.T) {
		t.Parallel()

		emptyStr := ""
		event := &Event{
			ID:             "event-uuid-005",
			ULID:           "01EMPTYVURL",
			Name:           "Empty VirtualURL Occ",
			LifecycleState: "published",
			Occurrences: []Occurrence{
				{
					StartTime:  time.Now(),
					VirtualURL: &emptyStr, // non-nil pointer to empty string
				},
			},
		}

		data, err := reconstructPayloadFromEvent(event)
		require.NoError(t, err)

		var result map[string]any
		require.NoError(t, json.Unmarshal(data, &result))

		occs := result["occurrences"].([]any)
		occ := occs[0].(map[string]any)
		assert.NotContains(t, occ, "virtual_url")
	})
}

func TestNearDuplicateWarnings(t *testing.T) {
	t.Parallel()

	t.Run("generates warning with near_duplicate_of_new_event code", func(t *testing.T) {
		t.Parallel()

		event := &Event{
			ID:             "existing-uuid",
			ULID:           "01EXISTING",
			Name:           "Jazz at the Rex",
			LifecycleState: "published",
		}
		newULID := "01NEWULID"

		data, err := nearDuplicateWarnings(event, newULID, nearDupNewEventData{Name: "New Jazz Event"})
		require.NoError(t, err)

		var warnings []ValidationWarning
		require.NoError(t, json.Unmarshal(data, &warnings))

		require.Len(t, warnings, 1)
		assert.Equal(t, "near_duplicate", warnings[0].Field)
		assert.Equal(t, "near_duplicate_of_new_event", warnings[0].Code)
		assert.Contains(t, warnings[0].Message, newULID)
		assert.Contains(t, warnings[0].Message, "Jazz at the Rex")
	})

	t.Run("nil existing event uses generic message", func(t *testing.T) {
		t.Parallel()

		newULID := "01NEWULID2"

		data, err := nearDuplicateWarnings(nil, newULID, nearDupNewEventData{Name: "New Jazz Event"})
		require.NoError(t, err)

		var warnings []ValidationWarning
		require.NoError(t, json.Unmarshal(data, &warnings))

		require.Len(t, warnings, 1)
		assert.Equal(t, "near_duplicate", warnings[0].Field)
		assert.Equal(t, "near_duplicate_of_new_event", warnings[0].Code)
		assert.Contains(t, warnings[0].Message, newULID)
	})

	t.Run("existing event with empty name uses generic message", func(t *testing.T) {
		t.Parallel()

		event := &Event{
			ID:   "existing-uuid-empty-name",
			ULID: "01NONAME",
		}
		newULID := "01NEWULID3"

		data, err := nearDuplicateWarnings(event, newULID, nearDupNewEventData{Name: "New Jazz Event"})
		require.NoError(t, err)

		var warnings []ValidationWarning
		require.NoError(t, json.Unmarshal(data, &warnings))

		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0].Message, newULID)
		// Generic message when name is empty
		assert.Contains(t, warnings[0].Message, "This existing event")
	})
}
