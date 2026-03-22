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
			name:     "strips query params from bare domain",
			input:    "example.com?param=value",
			expected: "https://example.com",
		},
		{
			name:     "strips query params from full URL",
			input:    "https://example.com?param=value",
			expected: "https://example.com",
		},
		{
			name:     "strips UTM tracking parameters",
			input:    "https://example.com/event?utm_campaign=spring&utm_source=facebook&fbclid=abc123",
			expected: "https://example.com/event",
		},
		{
			name:     "strips fragment from URL",
			input:    "https://example.com/page#section",
			expected: "https://example.com/page",
		},
		{
			name:     "strips both query and fragment",
			input:    "https://example.com/page?foo=bar#section",
			expected: "https://example.com/page",
		},
		{
			name:     "handles complex tracking params",
			input:    "www.eventbrite.ca/e/event-123?utm_source=google&gclid=xyz&ref=share",
			expected: "https://www.eventbrite.ca/e/event-123",
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
		{
			name:     "does not modify @mentions (let validation handle them)",
			input:    "@username",
			expected: "@username",
		},
		{
			name:     "does not modify @handle social shorthand",
			input:    "@instagram_user",
			expected: "@instagram_user",
		},
		{
			name:     "handles Twitter/X profile URLs",
			input:    "twitter.com/username",
			expected: "https://twitter.com/username",
		},
		{
			name:     "handles LinkedIn URLs",
			input:    "www.linkedin.com/company/example",
			expected: "https://www.linkedin.com/company/example",
		},
		{
			name:     "handles YouTube URLs",
			input:    "www.youtube.com/@channel",
			expected: "https://www.youtube.com/@channel",
		},
		{
			name:     "preserves path when stripping query",
			input:    "https://example.com/events/123?source=homepage",
			expected: "https://example.com/events/123",
		},
		{
			name:     "handles domain-only URL",
			input:    "https://example.com",
			expected: "https://example.com",
		},
		{
			name:     "handles domain with trailing slash",
			input:    "https://example.com/",
			expected: "https://example.com/",
		},
		{
			name:     "strips fragment with special chars",
			input:    "https://example.com/page#top-section",
			expected: "https://example.com/page",
		},
		{
			name:     "handles empty query string",
			input:    "https://example.com/page?",
			expected: "https://example.com/page",
		},
		{
			name:     "preserves port number",
			input:    "https://example.com:8080/page?foo=bar",
			expected: "https://example.com:8080/page",
		},
		{
			name:     "preserves mailto URLs unchanged (no stripping)",
			input:    "mailto:test@example.com?subject=Hello",
			expected: "mailto:test@example.com?subject=Hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeURL(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCleanText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "plain text unchanged",
			input: "Jazz Night",
			want:  "Jazz Night",
		},
		{
			name:  "HTML entities in name (WordPress numeric)",
			input: "Music &#038; Truffles KIDS &#8211; David Jalbert, piano",
			want:  "Music & Truffles KIDS – David Jalbert, piano",
		},
		{
			name:  "HTML tags in description (entity-encoded)",
			input: "&lt;p&gt;The Show: David will share stories.&lt;/p&gt;\n",
			want:  "The Show: David will share stories.",
		},
		{
			name:  "literal HTML tags with entities inside",
			input: "<p>A &amp; B</p>\n<br/>More text",
			want:  "A & B More text",
		},
		{
			name:  "collapses internal whitespace",
			input: "Hello\n\nWorld\t  test",
			want:  "Hello World test",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "named entity (no tags)",
			input: "Caf&eacute; &amp; Bar",
			want:  "Café & Bar",
		},
		{
			name:  "trims surrounding whitespace",
			input: "  trimmed  ",
			want:  "trimmed",
		},
		{
			name:  "WordPress backslash-apostrophe in description",
			input: `See Toronto\'s wildlife through the eyes of local photographers.\n`,
			want:  "See Toronto's wildlife through the eyes of local photographers.",
		},
		{
			name:  "entity-encoded HTML with backslash-apostrophe (Tribe Events listing page)",
			input: `&lt;p&gt;See Toronto\'s wildlife.&lt;/p&gt;\n`,
			want:  "See Toronto's wildlife.",
		},
		{
			name:  "double backslash collapses to single",
			input: `path\\to\\file`,
			want:  `path\to\file`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, cleanText(tc.input))
		})
	}
}

func TestNormalizeEventInputHTMLCleaning(t *testing.T) {
	input := EventInput{
		Name:        "Music &#038; Truffles KIDS &#8211; David Jalbert, piano",
		Description: "&lt;p&gt;The Show: David will share stories.&lt;/p&gt;\n",
		StartDate:   "2026-02-22T15:00:00Z",
	}
	got := NormalizeEventInput(input)
	assert.Equal(t, "Music & Truffles KIDS – David Jalbert, piano", got.Name)
	assert.Equal(t, "The Show: David will share stories.", got.Description)
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

func TestIsMultiSessionEvent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		input     EventInput
		wantMulti bool
		wantMsg   string // substring match (empty means no check)
	}{
		// Duration-based
		{
			name:      "normal 2h event",
			input:     EventInput{StartDate: "2026-02-10T19:00:00Z", EndDate: "2026-02-10T21:00:00Z", Name: "Concert"},
			wantMulti: false,
			wantMsg:   "",
		},
		{
			name:      "1 week event plus 1 second",
			input:     EventInput{StartDate: "2026-02-10T10:00:00Z", EndDate: "2026-02-17T10:00:01Z", Name: "Art Show"},
			wantMulti: true,
			wantMsg:   "spans",
		},
		{
			name:      "5 week course by duration",
			input:     EventInput{StartDate: "2026-02-10T10:00:00Z", EndDate: "2026-03-17T10:00:00Z", Name: "Keep Fit"},
			wantMulti: true,
			wantMsg:   "spans",
		},
		{
			name:      "no end date",
			input:     EventInput{StartDate: "2026-02-10T10:00:00Z", Name: "Open Mic"},
			wantMulti: false,
			wantMsg:   "",
		},
		{
			name:      "exactly 1 week is not flagged",
			input:     EventInput{StartDate: "2026-02-10T10:00:00Z", EndDate: "2026-02-17T10:00:00Z", Name: "Festival"},
			wantMulti: false,
			wantMsg:   "",
		},
		// Title-based
		{
			name:      "6 sessions in title",
			input:     EventInput{StartDate: "2026-02-10T10:00:00Z", EndDate: "2026-02-10T12:00:00Z", Name: "Keep Fit in Winter (6 sessions)"},
			wantMulti: true,
			wantMsg:   "pattern",
		},
		{
			name:      "1 session in title",
			input:     EventInput{StartDate: "2026-02-10T10:00:00Z", EndDate: "2026-02-10T12:00:00Z", Name: "Intro Yoga (1 session)"},
			wantMulti: true,
			wantMsg:   "pattern",
		},
		{
			name:      "4 weeks in title",
			input:     EventInput{StartDate: "2026-02-10T10:00:00Z", Name: "Drawing Course (4 weeks)"},
			wantMulti: true,
			wantMsg:   "pattern",
		},
		{
			name:      "workshop series",
			input:     EventInput{StartDate: "2026-02-10T10:00:00Z", Name: "Pottery Workshop Series"},
			wantMulti: true,
			wantMsg:   "pattern",
		},
		{
			name:      "weekly in title",
			input:     EventInput{StartDate: "2026-02-10T10:00:00Z", Name: "Weekly Meditation Circle"},
			wantMulti: true,
			wantMsg:   "pattern",
		},
		{
			name:      "course in title",
			input:     EventInput{StartDate: "2026-02-10T10:00:00Z", Name: "Beginner Painting Course"},
			wantMulti: true,
			wantMsg:   "pattern",
		},
		{
			name:      "8 classes in title",
			input:     EventInput{StartDate: "2026-02-10T10:00:00Z", Name: "Yoga (8 Classes)"},
			wantMulti: true,
			wantMsg:   "pattern",
		},
		{
			name:      "3 workshops in title",
			input:     EventInput{StartDate: "2026-02-10T10:00:00Z", Name: "Pottery (3 Workshops)"},
			wantMulti: true,
			wantMsg:   "pattern",
		},
		// False positives to avoid
		{
			name:      "racecourse not flagged",
			input:     EventInput{StartDate: "2026-02-10T10:00:00Z", Name: "Day at the Racecourse"},
			wantMulti: false,
			wantMsg:   "",
		},
		{
			name:      "discourse not flagged",
			input:     EventInput{StartDate: "2026-02-10T10:00:00Z", Name: "Public Discourse"},
			wantMulti: false,
			wantMsg:   "",
		},
		{
			name:      "normal title no match",
			input:     EventInput{StartDate: "2026-02-10T10:00:00Z", Name: "Jazz Night at the Rex"},
			wantMulti: false,
			wantMsg:   "",
		},
		// Case insensitivity
		{
			name:      "WEEKLY uppercase",
			input:     EventInput{StartDate: "2026-02-10T10:00:00Z", Name: "WEEKLY Jazz Sessions"},
			wantMulti: true,
			wantMsg:   "pattern",
		},
		{
			name:      "COURSE uppercase",
			input:     EventInput{StartDate: "2026-02-10T10:00:00Z", Name: "BEGINNER COURSE"},
			wantMulti: true,
			wantMsg:   "pattern",
		},
		// Custom threshold: zero means default (168h)
		{
			name: "zero threshold uses default 168h - short event not flagged",
			input: EventInput{
				StartDate:                     "2026-02-10T10:00:00Z",
				EndDate:                       "2026-02-17T10:00:00Z", // exactly 168h
				Name:                          "Festival",
				MultiSessionDurationThreshold: 0,
			},
			wantMulti: false,
			wantMsg:   "",
		},
		// Custom threshold: 10-day event with default (168h) threshold IS flagged
		{
			name: "10-day event with default threshold flagged",
			input: EventInput{
				StartDate: "2026-02-10T10:00:00Z",
				EndDate:   "2026-02-20T10:00:00Z", // 10 days = 240h > 168h
				Name:      "Art Show",
			},
			wantMulti: true,
			wantMsg:   "spans",
		},
		// Custom threshold: 10-day event with 720h (30 days) threshold NOT flagged
		{
			name: "10-day event with 720h custom threshold not flagged",
			input: EventInput{
				StartDate:                     "2026-02-10T10:00:00Z",
				EndDate:                       "2026-02-20T10:00:00Z", // 10 days = 240h < 720h
				Name:                          "Festival",
				MultiSessionDurationThreshold: 720 * time.Hour,
			},
			wantMulti: false,
			wantMsg:   "",
		},
		// Custom threshold: 35-day event with 720h (30 days) threshold IS flagged
		{
			name: "35-day event with 720h custom threshold flagged",
			input: EventInput{
				StartDate:                     "2026-02-10T10:00:00Z",
				EndDate:                       "2026-03-17T10:00:00Z", // 35 days = 840h > 720h
				Name:                          "Long Festival",
				MultiSessionDurationThreshold: 720 * time.Hour,
			},
			wantMulti: true,
			wantMsg:   "spans",
		},
		// Custom threshold: exactly at threshold boundary (not flagged)
		{
			name: "event exactly at custom threshold not flagged",
			input: EventInput{
				StartDate:                     "2026-02-10T10:00:00Z",
				EndDate:                       "2026-03-12T10:00:00Z", // exactly 720h (30 days)
				Name:                          "Festival",
				MultiSessionDurationThreshold: 720 * time.Hour,
			},
			wantMulti: false,
			wantMsg:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotMulti, gotMsg := IsMultiSessionEvent(tt.input)
			assert.Equal(t, tt.wantMulti, gotMulti, "IsMultiSessionEvent flag mismatch")
			if tt.wantMsg != "" {
				assert.Contains(t, gotMsg, tt.wantMsg, "reason should contain %q", tt.wantMsg)
			}
			if !tt.wantMulti {
				assert.Empty(t, gotMsg, "reason should be empty when not multi-session")
			}
		})
	}
}

func TestSeriesCompanionQuery(t *testing.T) {
	t.Run("struct fields populated correctly", func(t *testing.T) {
		startTime := time.Date(2026, 3, 15, 19, 0, 0, 0, time.UTC)
		q := SeriesCompanionQuery{
			NormalizedName: "Pottery Workshop",
			VenueID:        "venue-uuid-123",
			StartTime:      startTime,
			ExcludeULID:    "exclude-ulid-456",
		}
		assert.Equal(t, "Pottery Workshop", q.NormalizedName)
		assert.Equal(t, "venue-uuid-123", q.VenueID)
		assert.Equal(t, startTime, q.StartTime)
		assert.Equal(t, "exclude-ulid-456", q.ExcludeULID)
	})
}

func TestCrossWeekDateOffset(t *testing.T) {
	candidateTime := func(daysFromNew, hour, min int) time.Time {
		return time.Date(2026, 3, 15+daysFromNew, hour, min, 0, 0, time.UTC)
	}
	newEventTime := time.Date(2026, 3, 15, 19, 0, 0, 0, time.UTC)

	isWithinDateWindow := func(candidate, newEvent time.Time) bool {
		offset := candidate.Sub(newEvent).Hours() / 24
		return offset >= 7 && offset <= 21
	}

	isWithinTimeWindow := func(candidate, newEvent time.Time) bool {
		candTOD := candidate.Hour()*3600 + candidate.Minute()*60 + candidate.Second()
		newTOD := newEvent.Hour()*3600 + newEvent.Minute()*60 + newEvent.Second()
		diff := candTOD - newTOD
		if diff < 0 {
			diff = -diff
		}
		if diff > 43200 {
			diff = 86400 - diff
		}
		return diff < 1800
	}

	tests := []struct {
		name      string
		candidate time.Time
		wantDate  bool
		wantTime  bool
	}{
		{
			name:      "8 days apart same time - both match",
			candidate: candidateTime(8, 19, 0),
			wantDate:  true,
			wantTime:  true,
		},
		{
			name:      "14 days apart same time - both match",
			candidate: candidateTime(14, 19, 0),
			wantDate:  true,
			wantTime:  true,
		},
		{
			name:      "21 days apart same time - both match",
			candidate: candidateTime(21, 19, 0),
			wantDate:  true,
			wantTime:  true,
		},
		{
			name:      "6 days apart same time - date outside window",
			candidate: candidateTime(6, 19, 0),
			wantDate:  false,
			wantTime:  true,
		},
		{
			name:      "22 days apart same time - date outside window",
			candidate: candidateTime(22, 19, 0),
			wantDate:  false,
			wantTime:  true,
		},
		{
			name:      "14 days apart 25 min later - time within window",
			candidate: candidateTime(14, 19, 25),
			wantDate:  true,
			wantTime:  true,
		},
		{
			name:      "14 days apart 29 min later - time within window",
			candidate: candidateTime(14, 19, 29),
			wantDate:  true,
			wantTime:  true,
		},
		{
			name:      "14 days apart 31 min later - time outside window",
			candidate: candidateTime(14, 19, 31),
			wantDate:  true,
			wantTime:  false,
		},
		{
			name:      "14 days apart 15 min earlier - time within window",
			candidate: candidateTime(14, 18, 45),
			wantDate:  true,
			wantTime:  true,
		},
		{
			name:      "14 days apart 45 min earlier - time outside window",
			candidate: candidateTime(14, 18, 15),
			wantDate:  true,
			wantTime:  false,
		},
		{
			name:      "10 days apart 1 hour later - time outside window",
			candidate: candidateTime(10, 20, 0),
			wantDate:  true,
			wantTime:  false,
		},
		{
			name:      "7 days apart same time - both match (boundary)",
			candidate: candidateTime(7, 19, 0),
			wantDate:  true,
			wantTime:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDate := isWithinDateWindow(tt.candidate, newEventTime)
			gotTime := isWithinTimeWindow(tt.candidate, newEventTime)
			assert.Equal(t, tt.wantDate, gotDate, "date window mismatch")
			assert.Equal(t, tt.wantTime, gotTime, "time window mismatch")
		})
	}
}

func TestCrossWeekCompanion(t *testing.T) {
	t.Run("struct fields", func(t *testing.T) {
		c := CrossWeekCompanion{
			ULID:      "01ARZ3NDEKTSV4RRFFQ69G5FAV",
			Name:      "Weekly Yoga",
			StartDate: "2026-03-08T19:00:00Z",
			StartTime: "19:00:00",
			VenueName: "Community Centre",
		}
		assert.Equal(t, "01ARZ3NDEKTSV4RRFFQ69G5FAV", c.ULID)
		assert.Equal(t, "Weekly Yoga", c.Name)
		assert.Equal(t, "2026-03-08T19:00:00Z", c.StartDate)
		assert.Equal(t, "19:00:00", c.StartTime)
		assert.Equal(t, "Community Centre", c.VenueName)
	})
}

func TestNormalizeRegion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Ontario", "ON"},
		{"ontario", "ON"},
		{"ONTARIO", "ON"},
		{"ON", "ON"},
		{"  on  ", "ON"},
		{"British Columbia", "BC"},
		{"Washington", "WA"},
		{"WA", "WA"},
		{"Québec", "QC"},
		{"Quebec", "QC"},
		{"", ""},
		{"SomeUnknownRegion", "SOMEUNKNOWNREGION"},
		{"Alberta", "AB"},
		{"New York", "NY"},
		{"California", "CA"},
		{"Newfoundland and Labrador", "NL"},
		{"Newfoundland", "NL"},
		{"Prince Edward Island", "PE"},
		{"Northwest Territories", "NT"},
		{"Yukon", "YT"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeRegion(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}
