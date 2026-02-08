package events

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "adds https to www prefix",
			input:    "www.example.com",
			expected: "https://www.example.com",
		},
		{
			name:     "adds https to bare domain",
			input:    "example.com",
			expected: "https://example.com",
		},
		{
			name:     "adds https to subdomain",
			input:    "blog.example.com",
			expected: "https://blog.example.com",
		},
		{
			name:     "preserves existing https",
			input:    "https://example.com",
			expected: "https://example.com",
		},
		{
			name:     "preserves existing http",
			input:    "http://example.com",
			expected: "http://example.com",
		},
		{
			name:     "preserves mailto URLs",
			input:    "mailto:test@example.com",
			expected: "mailto:test@example.com",
		},
		{
			name:     "handles empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "trims whitespace and adds https to www",
			input:    "  www.example.com  ",
			expected: "https://www.example.com",
		},
		{
			name:     "trims whitespace and adds https to bare domain",
			input:    "  example.com  ",
			expected: "https://example.com",
		},
		{
			name:     "trims whitespace from complete URL",
			input:    "  https://example.com  ",
			expected: "https://example.com",
		},
		{
			name:     "handles short domain like bit.ly",
			input:    "bit.ly/abc123",
			expected: "https://bit.ly/abc123",
		},
		{
			name:     "handles domain with path",
			input:    "example.com/path/to/page",
			expected: "https://example.com/path/to/page",
		},
		{
			name:     "handles domain with query params",
			input:    "example.com?param=value",
			expected: "https://example.com?param=value",
		},
		{
			name:     "does not add protocol to relative paths",
			input:    "/path/to/resource",
			expected: "/path/to/resource",
		},
		{
			name:     "does not add protocol to anchor links",
			input:    "#section",
			expected: "#section",
		},
		{
			name:     "does not modify strings with spaces (invalid URLs)",
			input:    "not a url",
			expected: "not a url",
		},
		{
			name:     "handles Instagram URLs from Toronto data",
			input:    "www.instagram.com/username",
			expected: "https://www.instagram.com/username",
		},
		{
			name:     "handles Facebook URLs from Toronto data",
			input:    "www.facebook.com/events/123456",
			expected: "https://www.facebook.com/events/123456",
		},
		{
			name:     "handles bare domain social media",
			input:    "facebook.com/page",
			expected: "https://facebook.com/page",
		},
		{
			name:     "handles Eventbrite URLs",
			input:    "www.eventbrite.ca/e/event-name-123456",
			expected: "https://www.eventbrite.ca/e/event-name-123456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeURL(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

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
	tests := []struct {
		name          string
		input         EventInput
		wantCorrected bool
		description   string
	}{
		{
			name: "corrects midnight-spanning event with early morning end at 2 AM",
			input: EventInput{
				Name:      "Late Night Event",
				StartDate: "2025-03-31T23:00:00Z", // 11 PM
				EndDate:   "2025-03-31T02:00:00Z", // 2 AM (early morning)
			},
			wantCorrected: true,
			description:   "Early morning end (2 AM) + corrected duration (3h) → auto-fix",
		},
		{
			name: "corrects event spanning midnight ending at 4 AM",
			input: EventInput{
				Name:      "Overnight Event",
				StartDate: "2025-04-01T22:00:00Z", // 10 PM
				EndDate:   "2025-04-01T04:00:00Z", // 4 AM (early morning boundary)
			},
			wantCorrected: true,
			description:   "Early morning end (4 AM) + corrected duration (6h) → auto-fix",
		},
		{
			name: "does NOT correct event ending at 6 AM (beyond early morning threshold)",
			input: EventInput{
				Name:      "Event ending at 6 AM",
				StartDate: "2025-04-01T22:00:00Z",
				EndDate:   "2025-04-01T06:00:00Z", // 6 AM (hour=6, not 0-4)
			},
			wantCorrected: false,
			description:   "End hour (6) exceeds early morning threshold (0-4)",
		},
		{
			name: "does NOT correct event ending at noon (not early morning)",
			input: EventInput{
				Name:      "Event ending at noon",
				StartDate: "2025-04-01T22:00:00Z",
				EndDate:   "2025-04-01T12:00:00Z", // noon (hour=12, not 0-4)
			},
			wantCorrected: false,
			description:   "End hour (12) is not early morning (0-4)",
		},
		{
			name: "does NOT correct event with long corrected duration",
			input: EventInput{
				Name:      "Event with long duration",
				StartDate: "2025-04-02T03:00:00Z", // 3 AM
				EndDate:   "2025-04-02T02:00:00Z", // 2 AM (early morning) but would be 23h duration
			},
			wantCorrected: false,
			description:   "Corrected duration (23h) exceeds 7h threshold",
		},
		{
			name: "does not correct valid dates",
			input: EventInput{
				Name:      "Normal Event",
				StartDate: "2025-04-01T10:00:00Z",
				EndDate:   "2025-04-01T12:00:00Z",
			},
			wantCorrected: false,
			description:   "Dates are already in correct order",
		},
		{
			name: "does not correct when endDate is missing",
			input: EventInput{
				Name:      "No End Time",
				StartDate: "2025-04-01T10:00:00Z",
			},
			wantCorrected: false,
			description:   "No endDate to correct",
		},
		{
			name: "does not correct invalid date formats",
			input: EventInput{
				Name:      "Invalid Dates",
				StartDate: "not-a-date",
				EndDate:   "2025-04-01T10:00:00Z",
			},
			wantCorrected: false,
			description:   "Invalid startDate should be left for validation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := correctEndDateTimezoneError(tt.input)

			if tt.wantCorrected {
				// Verify that endDate was changed
				require.NotEqual(t, tt.input.EndDate, result.EndDate, tt.description)

				// For early morning cases, verify 24h was added
				if tt.input.EndDate != "" {
					inputTime, _ := time.Parse(time.RFC3339, tt.input.EndDate)
					resultTime, _ := time.Parse(time.RFC3339, result.EndDate)
					diff := resultTime.Sub(inputTime)
					require.Equal(t, 24*time.Hour, diff, "Should add exactly 24 hours")
				}
			} else {
				// Verify that endDate was NOT changed
				require.Equal(t, tt.input.EndDate, result.EndDate, tt.description)
			}
		})
	}
}

func TestCorrectEndDateTimezoneError_Integration(t *testing.T) {
	t.Run("corrects event with early morning end", func(t *testing.T) {
		// Event ending at 3 AM (early morning)
		input := EventInput{
			Name:      "Late Night Event",
			StartDate: "2025-06-08T23:00:00Z", // 11 PM
			EndDate:   "2025-06-08T03:00:00Z", // 3 AM (should be next day)
		}

		result := correctEndDateTimezoneError(input)

		// Should add 24 hours (corrected duration: 4h)
		expected := "2025-06-09T03:00:00Z"
		require.Equal(t, expected, result.EndDate)
	})

	t.Run("does NOT correct event ending at 5 PM", func(t *testing.T) {
		// Event ending at 5 PM - NOT early morning, won't auto-fix
		input := EventInput{
			Name:      "Daytime Event",
			StartDate: "2025-06-08T00:30:00Z",
			EndDate:   "2025-06-07T17:00:00Z", // 5 PM day before
		}

		result := correctEndDateTimezoneError(input)

		// Should NOT correct (end hour is 17, not 0-4)
		require.Equal(t, input.EndDate, result.EndDate)
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
		// Event with early morning end but very long corrected duration
		input := EventInput{
			Name:      "Suspicious Event",
			StartDate: "2025-06-08T10:00:00Z",
			EndDate:   "2025-06-06T02:00:00Z", // 2 AM, 2 days before (would be 40h duration)
		}

		result := correctEndDateTimezoneError(input)

		// Should not correct (corrected duration > 7h)
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
			StartDate: "2025-03-31T23:00:00Z", // 11 PM
			EndDate:   "2025-03-31T02:00:00Z", // 2 AM (changed from 6 AM to be in early morning range)
		}

		normalized := NormalizeEventInput(input)

		// Should trim name AND correct endDate
		require.Equal(t, "Monday Latin Nights", normalized.Name)
		require.Equal(t, "2025-04-01T02:00:00Z", normalized.EndDate) // Corrected to next day

		// NOTE: When passed to ValidateEventInputWithWarnings with the original input,
		// this will now generate a "reversed_dates_timezone_likely" warning
	})
}
