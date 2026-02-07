package events

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeStringSlice(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		lower    bool
		expected []string
	}{
		{
			name:     "trims whitespace",
			input:    []string{"  hello  ", "  world  "},
			lower:    false,
			expected: []string{"hello", "world"},
		},
		{
			name:     "removes empty strings",
			input:    []string{"hello", "", "world", "  "},
			lower:    false,
			expected: []string{"hello", "world"},
		},
		{
			name:     "removes duplicates",
			input:    []string{"hello", "hello", "world"},
			lower:    false,
			expected: []string{"hello", "world"},
		},
		{
			name:     "lowercase conversion",
			input:    []string{"HELLO", "World"},
			lower:    true,
			expected: []string{"hello", "world"},
		},
		{
			name:     "empty input",
			input:    []string{},
			lower:    false,
			expected: []string{},
		},
		{
			name:     "nil input",
			input:    nil,
			lower:    false,
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeStringSlice(tt.input, tt.lower)
			if len(tt.expected) == 0 {
				assert.Nil(t, result)
			} else {
				assert.ElementsMatch(t, tt.expected, result)
			}
		})
	}
}

func TestNormalizePlaceInput(t *testing.T) {
	input := PlaceInput{
		Name:            "  Test Venue  ",
		StreetAddress:   "  123 Main St  ",
		AddressLocality: "  Toronto  ",
		AddressRegion:   "  ON  ",
		PostalCode:      "  M5V 3A8  ",
		AddressCountry:  "  CA  ",
	}

	result := normalizePlaceInput(input)
	assert.Equal(t, "Test Venue", result.Name)
	assert.Equal(t, "123 Main St", result.StreetAddress)
	assert.Equal(t, "Toronto", result.AddressLocality)
	assert.Equal(t, "ON", result.AddressRegion)
	assert.Equal(t, "M5V 3A8", result.PostalCode)
	assert.Equal(t, "CA", result.AddressCountry)
}

func TestNormalizeOrganizationInput(t *testing.T) {
	input := OrganizationInput{
		Name: "  Test Org  ",
		URL:  "  https://example.com  ",
	}

	result := normalizeOrganizationInput(input)
	assert.Equal(t, "Test Org", result.Name)
	assert.Equal(t, "https://example.com", result.URL)
}

func TestNormalizeVirtualLocationInput(t *testing.T) {
	input := VirtualLocationInput{
		Type: "  VirtualLocation  ",
		URL:  "  https://zoom.us/j/123  ",
		Name: "  Zoom Meeting  ",
	}

	result := normalizeVirtualLocationInput(input)
	assert.Equal(t, "VirtualLocation", result.Type)
	assert.Equal(t, "https://zoom.us/j/123", result.URL)
	assert.Equal(t, "Zoom Meeting", result.Name)
}

func TestNormalizeOfferInput(t *testing.T) {
	input := OfferInput{
		URL:           "  https://tickets.example.com  ",
		Price:         "  25.00  ",
		PriceCurrency: "  CAD  ",
	}

	result := normalizeOfferInput(input)
	assert.Equal(t, "https://tickets.example.com", result.URL)
	assert.Equal(t, "25.00", result.Price)
	assert.Equal(t, "CAD", result.PriceCurrency)
}

func TestNormalizeSourceInput(t *testing.T) {
	input := SourceInput{
		URL:     "  https://source.example.com  ",
		EventID: "  event123  ",
		Name:    "  Event Source  ",
		License: "  CC0-1.0  ",
	}

	result := normalizeSourceInput(input)
	assert.Equal(t, "https://source.example.com", result.URL)
	assert.Equal(t, "event123", result.EventID)
	// Note: Name and License are not normalized by this function
	assert.Equal(t, "  Event Source  ", result.Name)
	assert.Equal(t, "  CC0-1.0  ", result.License)
}

func TestNormalizeOccurrences(t *testing.T) {
	occurrences := []OccurrenceInput{
		{
			StartDate:  "  2026-02-01T10:00:00Z  ",
			EndDate:    "  2026-02-01T12:00:00Z  ",
			Timezone:   "  America/Toronto  ",
			DoorTime:   "  2026-02-01T09:30:00Z  ",
			VenueID:    "  venue123  ",
			VirtualURL: "  https://zoom.us/j/123  ",
		},
	}

	result := normalizeOccurrences(occurrences)
	require.Len(t, result, 1)
	assert.Equal(t, "2026-02-01T10:00:00Z", result[0].StartDate)
	assert.Equal(t, "2026-02-01T12:00:00Z", result[0].EndDate)
	assert.Equal(t, "America/Toronto", result[0].Timezone)
	assert.Equal(t, "2026-02-01T09:30:00Z", result[0].DoorTime)
	assert.Equal(t, "venue123", result[0].VenueID)
	assert.Equal(t, "https://zoom.us/j/123", result[0].VirtualURL)
}

func TestCorrectEndDateTimezoneError(t *testing.T) {
	t.Run("corrects midnight-spanning event with timezone error", func(t *testing.T) {
		// Event should be 11 PM - 2 AM (3 hours), but endDate appears before startDate
		input := EventInput{
			Name:      "Monday Latin Nights",
			StartDate: "2025-03-31T23:00:00Z", // 11 PM
			EndDate:   "2025-03-31T06:00:00Z", // 6 AM (appears 17 hours before start!)
		}

		result := correctEndDateTimezoneError(input)

		// Should add 24 hours to endDate, making it 2025-04-01T06:00:00Z
		require.Equal(t, "2025-04-01T06:00:00Z", result.EndDate)
	})

	t.Run("corrects event spanning midnight", func(t *testing.T) {
		// Event: 11:30 PM - 5 PM next day, but dates got scrambled
		input := EventInput{
			Name:      "Weston Farmers Market",
			StartDate: "2025-06-08T00:30:00Z",
			EndDate:   "2025-06-07T17:00:00Z", // Day before!
		}

		result := correctEndDateTimezoneError(input)

		// Should add 24 hours
		expected := "2025-06-08T17:00:00Z"
		require.Equal(t, expected, result.EndDate)
	})

	t.Run("does not correct valid dates", func(t *testing.T) {
		input := EventInput{
			Name:      "Normal Event",
			StartDate: "2025-06-08T10:00:00Z",
			EndDate:   "2025-06-08T12:00:00Z",
		}

		result := correctEndDateTimezoneError(input)

		require.Equal(t, input.EndDate, result.EndDate, "Should not modify valid dates")
	})

	t.Run("does not correct when endDate is missing", func(t *testing.T) {
		input := EventInput{
			Name:      "Single Date Event",
			StartDate: "2025-06-08T10:00:00Z",
			EndDate:   "",
		}

		result := correctEndDateTimezoneError(input)

		require.Equal(t, "", result.EndDate)
	})

	t.Run("does not correct when adding 24h makes event too long", func(t *testing.T) {
		// If endDate is way before startDate, don't auto-correct
		// (might be genuinely bad data, not just timezone error)
		input := EventInput{
			Name:      "Suspicious Event",
			StartDate: "2025-06-08T10:00:00Z",
			EndDate:   "2025-06-06T10:00:00Z", // 2 days before
		}

		result := correctEndDateTimezoneError(input)

		// Adding 24h would make it 2025-06-07T10:00:00Z, still before start
		// So no correction should happen
		require.Equal(t, input.EndDate, result.EndDate, "Should not correct large gaps")
	})

	t.Run("does not correct invalid date formats", func(t *testing.T) {
		input := EventInput{
			Name:      "Bad Format",
			StartDate: "not-a-date",
			EndDate:   "also-not-a-date",
		}

		result := correctEndDateTimezoneError(input)

		require.Equal(t, input.EndDate, result.EndDate, "Should not panic or modify invalid dates")
	})

	t.Run("integrates with NormalizeEventInput", func(t *testing.T) {
		// Test that the correction is applied during normalization
		input := EventInput{
			Name:      "  Monday Latin Nights  ",
			StartDate: "2025-03-31T23:00:00Z",
			EndDate:   "2025-03-31T06:00:00Z", // Before startDate
		}

		normalized := NormalizeEventInput(input)

		// Should trim name AND correct endDate
		require.Equal(t, "Monday Latin Nights", normalized.Name)
		require.Equal(t, "2025-04-01T06:00:00Z", normalized.EndDate)
	})
}
