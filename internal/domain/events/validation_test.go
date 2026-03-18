package events

import (
	"strings"
	"testing"

	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/domain/ids"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateULID(t *testing.T) {
	valid, err := ids.NewULID()
	require.NoError(t, err)

	require.NoError(t, ids.ValidateULID(valid))
	require.ErrorIs(t, ids.ValidateULID("not-a-ulid"), ids.ErrInvalidULID)
}

// validEventInput returns a minimal valid event input for testing
func validEventInput() EventInput {
	return EventInput{
		Name:      "Test Event",
		StartDate: "2026-02-01T10:00:00Z",
		Location: &PlaceInput{
			Name:            "Test Venue",
			AddressLocality: "Toronto",
		},
	}
}

func TestValidateEventInput_Valid(t *testing.T) {
	input := validEventInput()
	result, err := ValidateEventInput(input, "example.com")
	require.NoError(t, err)
	assert.Equal(t, "Test Event", result.Name)
}

func TestValidateEventInput_EmptyName(t *testing.T) {
	input := validEventInput()
	input.Name = ""
	_, err := ValidateEventInput(input, "example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name")
}

func TestValidateEventInput_NameTooLong(t *testing.T) {
	input := validEventInput()
	input.Name = strings.Repeat("a", 501)
	_, err := ValidateEventInput(input, "example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name")
}

func TestValidateEventInput_DescriptionTooLong(t *testing.T) {
	input := validEventInput()
	input.Description = strings.Repeat("a", 10001)
	_, err := ValidateEventInput(input, "example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "description")
}

func TestValidateEventInput_InvalidStartDate(t *testing.T) {
	input := validEventInput()
	input.StartDate = "not-a-date"
	_, err := ValidateEventInput(input, "example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "startDate")
}

func TestValidateEventInput_EndBeforeStart(t *testing.T) {
	input := validEventInput()
	input.StartDate = "2026-02-01T12:00:00Z"
	input.EndDate = "2026-02-01T10:00:00Z" // 2 hours before start
	// Changed behavior: this now returns a WARNING instead of an error
	result, err := ValidateEventInputWithWarnings(input, "example.com", nil, config.ValidationConfig{})
	require.NoError(t, err, "Reversed dates should not cause validation error")
	require.NotNil(t, result)
	require.NotEmpty(t, result.Warnings, "Should have warning for reversed dates")

	// Verify the warning details
	// End hour is 10 (not early morning 0-4), so it needs review
	found := false
	for _, w := range result.Warnings {
		if w.Field == "endDate" && w.Code == "reversed_dates_corrected_needs_review" {
			found = true
			break
		}
	}
	assert.True(t, found, "Should have reversed_dates_corrected_needs_review warning")
}

func TestValidateEventInput_NoLocation(t *testing.T) {
	input := validEventInput()
	input.Location = nil
	input.VirtualLocation = nil
	_, err := ValidateEventInput(input, "example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "location")
}

func TestValidateEventInput_VirtualLocation(t *testing.T) {
	input := validEventInput()
	input.Location = nil
	input.VirtualLocation = &VirtualLocationInput{
		URL: "https://zoom.us/j/123456",
	}
	_, err := ValidateEventInput(input, "example.com")
	require.NoError(t, err)
}

func TestValidateEventInput_InvalidURL(t *testing.T) {
	input := validEventInput()
	input.URL = "not-a-url"
	_, err := ValidateEventInput(input, "example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "url")
}

func TestValidateEventInput_InvalidImageURL(t *testing.T) {
	input := validEventInput()
	input.Image = "not-a-url"
	_, err := ValidateEventInput(input, "example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "image")
}

func TestValidateEventInput_NonCC0License(t *testing.T) {
	input := validEventInput()
	input.License = "https://creativecommons.org/licenses/by/4.0/"
	_, err := ValidateEventInput(input, "example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "license")
}

func TestValidateEventInput_CC0License(t *testing.T) {
	input := validEventInput()
	input.License = "https://creativecommons.org/publicdomain/zero/1.0/"
	_, err := ValidateEventInput(input, "example.com")
	require.NoError(t, err)
}

func TestIsCC0License(t *testing.T) {
	tests := []struct {
		name    string
		license string
		want    bool
	}{
		{"full CC0 URL", "https://creativecommons.org/publicdomain/zero/1.0/", true},
		{"CC0 short form", "CC0-1.0", true},
		{"CC0 mixed case", "cc0-1.0", true},
		{"CC-BY license", "https://creativecommons.org/licenses/by/4.0/", false},
		{"empty string", "", false},
		{"random string", "MIT", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isCC0License(tt.license))
		})
	}
}

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid HTTPS URL", "https://example.com/path", false},
		{"valid HTTP URL", "http://example.com/path", false},
		{"invalid scheme", "ftp://example.com/resource", true},
		{"invalid URL", "not-a-url", true},
		{"empty URL", "", true},
		{"relative URL", "/path/to/resource", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateURL(tt.url)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestParseRFC3339(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid RFC3339", "2026-02-01T10:00:00Z", false},
		{"valid with timezone", "2026-02-01T10:00:00-05:00", false},
		{"invalid date", "not-a-date", true},
		{"empty string", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseRFC3339("testField", tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidationError_Error(t *testing.T) {
	err1 := ValidationError{Field: "name", Message: "is required"}
	assert.Equal(t, "invalid name: is required", err1.Error())

	err2 := ValidationError{Field: "", Message: "general error"}
	assert.Equal(t, "general error", err2.Error())
}

func TestNormalizeEventInput(t *testing.T) {
	input := EventInput{
		Name:      "  Test Event  ",
		StartDate: "2026-02-01T10:00:00Z",
		Keywords:  []string{"  music  ", "  concert  "},
		Location: &PlaceInput{
			Name: "  Test Venue  ",
		},
	}

	normalized := NormalizeEventInput(input)
	assert.Equal(t, "Test Event", normalized.Name)
	assert.ElementsMatch(t, []string{"music", "concert"}, normalized.Keywords)
	assert.Equal(t, "Test Venue", normalized.Location.Name)
}

func TestValidateOrganizationInput(t *testing.T) {
	nodeDomain := "example.com"

	tests := []struct {
		name    string
		org     OrganizationInput
		wantErr bool
	}{
		{
			name: "valid with name only",
			org: OrganizationInput{
				Name: "Test Org",
			},
			wantErr: false,
		},
		{
			name: "valid with ID only",
			org: OrganizationInput{
				ID: "https://example.com/organizations/01ARZ3NDEKTSV4RRFFQ69G5FAV",
			},
			wantErr: false,
		},
		{
			name: "valid with ID and name",
			org: OrganizationInput{
				ID:   "https://example.com/organizations/01ARZ3NDEKTSV4RRFFQ69G5FAV",
				Name: "Test Org",
			},
			wantErr: false,
		},
		{
			name: "valid with URL",
			org: OrganizationInput{
				Name: "Test Org",
				URL:  "https://testorg.com",
			},
			wantErr: false,
		},
		{
			name:    "invalid - no ID or name",
			org:     OrganizationInput{},
			wantErr: true,
		},
		{
			name: "invalid - whitespace name only",
			org: OrganizationInput{
				Name: "   ",
			},
			wantErr: true,
		},
		{
			name: "invalid - bad canonical URI",
			org: OrganizationInput{
				ID: "not-a-valid-uri",
			},
			wantErr: true,
		},
		{
			name: "invalid - non-canonical URI (external)",
			org: OrganizationInput{
				ID: "https://external.com/organizations/01ARZ3NDEKTSV4RRFFQ69G5FAV",
			},
			wantErr: true,
		},
		{
			name: "invalid URL",
			org: OrganizationInput{
				Name: "Test Org",
				URL:  "not-a-url",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOrganizationInput(tt.org, nodeDomain)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateCanonicalURI(t *testing.T) {
	nodeDomain := "example.com"

	tests := []struct {
		name       string
		entityPath string
		value      string
		wantErr    bool
	}{
		{
			name:       "valid canonical URI",
			entityPath: "events",
			value:      "https://example.com/events/01ARZ3NDEKTSV4RRFFQ69G5FAV",
			wantErr:    false,
		},
		{
			name:       "invalid - external domain",
			entityPath: "events",
			value:      "https://external.com/events/01ARZ3NDEKTSV4RRFFQ69G5FAV",
			wantErr:    true,
		},
		{
			name:       "invalid - not a URI",
			entityPath: "events",
			value:      "not-a-uri",
			wantErr:    true,
		},
		{
			name:       "invalid - wrong entity path",
			entityPath: "events",
			value:      "https://example.com/places/01ARZ3NDEKTSV4RRFFQ69G5FAV",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCanonicalURI(nodeDomain, tt.entityPath, tt.value)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNormalizeSameAs(t *testing.T) {
	nodeDomain := "example.com"

	tests := []struct {
		name       string
		entityPath string
		values     []string
		wantErr    bool
		wantLen    int
	}{
		{
			name:       "valid external URIs",
			entityPath: "events",
			values: []string{
				"https://external.com/events/123",
				"https://another.com/e/456",
			},
			wantErr: false,
			wantLen: 2,
		},
		{
			name:       "empty list",
			entityPath: "events",
			values:     []string{},
			wantErr:    false,
			wantLen:    0,
		},
		{
			name:       "invalid URI in list",
			entityPath: "events",
			values: []string{
				"https://valid.com/events/123",
				"not-a-uri",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := normalizeSameAs(nodeDomain, tt.entityPath, tt.values)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, result, tt.wantLen)
			}
		})
	}
}

func TestValidateEventInput_WithOccurrences(t *testing.T) {
	nodeDomain := "example.com"

	tests := []struct {
		name    string
		input   EventInput
		wantErr bool
	}{
		{
			name: "valid occurrence",
			input: EventInput{
				Name:      "Test Event",
				StartDate: "2026-02-01T10:00:00Z",
				Occurrences: []OccurrenceInput{
					{
						StartDate: "2026-02-01T10:00:00Z",
						EndDate:   "2026-02-01T12:00:00Z",
					},
				},
				Location: &PlaceInput{Name: "Test Venue"},
			},
			wantErr: false,
		},
		{
			name: "occurrence with door time",
			input: EventInput{
				Name:      "Test Event",
				StartDate: "2026-02-01T10:00:00Z",
				Occurrences: []OccurrenceInput{
					{
						StartDate: "2026-02-01T10:00:00Z",
						DoorTime:  "2026-02-01T09:30:00Z",
					},
				},
				Location: &PlaceInput{Name: "Test Venue"},
			},
			wantErr: false,
		},
		{
			name: "occurrence with timezone",
			input: EventInput{
				Name:      "Test Event",
				StartDate: "2026-02-01T10:00:00Z",
				Occurrences: []OccurrenceInput{
					{
						StartDate: "2026-02-01T10:00:00Z",
						Timezone:  "America/New_York",
					},
				},
				Location: &PlaceInput{Name: "Test Venue"},
			},
			wantErr: false,
		},
		{
			name: "occurrence with virtual URL",
			input: EventInput{
				Name:      "Test Event",
				StartDate: "2026-02-01T10:00:00Z",
				Occurrences: []OccurrenceInput{
					{
						StartDate:  "2026-02-01T10:00:00Z",
						VirtualURL: "https://zoom.us/j/123456",
					},
				},
				Location: &PlaceInput{Name: "Test Venue"},
			},
			wantErr: false,
		},
		{
			name: "occurrence with reversed dates - generates warning not error",
			input: EventInput{
				Name:      "Test Event",
				StartDate: "2026-02-01T10:00:00Z",
				Occurrences: []OccurrenceInput{
					{
						StartDate: "2026-02-01T12:00:00Z",
						EndDate:   "2026-02-01T10:00:00Z", // Reversed - 2 hour gap at 10 AM
					},
				},
				Location: &PlaceInput{Name: "Test Venue"},
			},
			wantErr: false, // Should generate warning, not error
		},
		{
			name: "invalid occurrence - bad timezone",
			input: EventInput{
				Name:      "Test Event",
				StartDate: "2026-02-01T10:00:00Z",
				Occurrences: []OccurrenceInput{
					{
						StartDate: "2026-02-01T10:00:00Z",
						Timezone:  "Invalid/Timezone",
					},
				},
				Location: &PlaceInput{Name: "Test Venue"},
			},
			wantErr: true,
		},
		{
			name: "invalid occurrence - bad virtual URL",
			input: EventInput{
				Name:      "Test Event",
				StartDate: "2026-02-01T10:00:00Z",
				Occurrences: []OccurrenceInput{
					{
						StartDate:  "2026-02-01T10:00:00Z",
						VirtualURL: "not-a-url",
					},
				},
				Location: &PlaceInput{Name: "Test Venue"},
			},
			wantErr: true,
		},
		{
			// Regression: occurrence with no venueId/virtualUrl is valid when parent has location.
			name: "occurrence without venueId or virtualUrl inherits parent location - valid",
			input: EventInput{
				Name:      "Weekly Yoga",
				StartDate: "2026-06-01T10:00:00Z",
				Location:  &PlaceInput{Name: "Studio 9"},
				Occurrences: []OccurrenceInput{
					{StartDate: "2026-06-01T10:00:00Z", EndDate: "2026-06-01T11:30:00Z", Timezone: "America/Toronto"},
					{StartDate: "2026-06-08T10:00:00Z", EndDate: "2026-06-08T11:30:00Z", Timezone: "America/Toronto"},
				},
			},
			wantErr: false,
		},
		{
			// Regression: occurrence with no venueId/virtualUrl and parent only has virtualLocation - valid.
			name: "occurrence without venueId inherits parent virtualLocation - valid",
			input: EventInput{
				Name:            "Online Series",
				StartDate:       "2026-06-01T10:00:00Z",
				VirtualLocation: &VirtualLocationInput{URL: "https://zoom.us/j/abc123"},
				Occurrences: []OccurrenceInput{
					{StartDate: "2026-06-01T10:00:00Z", EndDate: "2026-06-01T11:00:00Z"},
					{StartDate: "2026-06-08T10:00:00Z", EndDate: "2026-06-08T11:00:00Z"},
				},
			},
			wantErr: false,
		},
		{
			// Regression: occurrence with no venueId/virtualUrl AND parent has no location - must error.
			// This is the contract enforced by occurrence_location_required DB constraint.
			name: "occurrence without venueId and parent has no location - invalid",
			input: EventInput{
				Name:      "Nowhere Event",
				StartDate: "2026-06-01T10:00:00Z",
				// No Location, no VirtualLocation
				Occurrences: []OccurrenceInput{
					{StartDate: "2026-06-01T10:00:00Z", EndDate: "2026-06-01T11:00:00Z"},
				},
			},
			wantErr: true,
		},
		{
			// New contract: a parent location with only a canonical @id is accepted at
			// validation time. The ingest layer resolves it via GetPlaceByULID; if the
			// place doesn't exist ingest returns an error. Validation is optimistic.
			name: "occurrence without venueId and parent location has @id only (empty name) - valid at validation",
			input: EventInput{
				Name:      "Canonical Venue Event",
				StartDate: "2026-06-01T10:00:00Z",
				Location: &PlaceInput{
					ID: "https://example.com/places/01ARZ3NDEKTSV4RRFFQ69G5FAV",
				},
				Occurrences: []OccurrenceInput{
					{StartDate: "2026-06-01T10:00:00Z", EndDate: "2026-06-01T11:00:00Z"},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateEventInput(tt.input, nodeDomain)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateEventInput_SingleOccurrenceLocation covers the single-occurrence path
// (no Occurrences array, just StartDate) where the parent location must be resolvable at
// ingest time. A Location with only a canonical @id (empty Name) passes validatePlaceInput
// but cannot be resolved by the ingest layer (UpsertPlace requires a Name), so
// PrimaryVenueID would be nil and the occurrence would hit the occurrence_location_required
// DB constraint. The fix added an early-rejection check in ValidateEventInputWithWarnings.
func TestValidateEventInput_SingleOccurrenceLocation(t *testing.T) {
	nodeDomain := "example.com"

	tests := []struct {
		name            string
		input           EventInput
		wantErr         bool
		wantErrContains string
	}{
		{
			// Baseline: single-occurrence event with a named location — must pass.
			name: "single_occurrence_named_location_valid",
			input: EventInput{
				Name:      "Yoga at The Studio",
				StartDate: "2026-06-01T10:00:00Z",
				EndDate:   "2026-06-01T11:30:00Z",
				License:   "CC0-1.0",
				Location:  &PlaceInput{Name: "The Studio", AddressLocality: "Toronto"},
			},
			wantErr: false,
		},
		{
			// Baseline: single-occurrence event with a virtualLocation — must pass.
			name: "single_occurrence_virtual_location_valid",
			input: EventInput{
				Name:            "Online Yoga",
				StartDate:       "2026-06-01T10:00:00Z",
				EndDate:         "2026-06-01T11:30:00Z",
				License:         "CC0-1.0",
				VirtualLocation: &VirtualLocationInput{URL: "https://zoom.us/j/abc123"},
			},
			wantErr: false,
		},
		{
			// New contract: single-occurrence event where Location has only a canonical @id
			// (empty Name) is accepted at validation. The ingest layer resolves it via
			// GetPlaceByULID — if the place doesn't exist, ingest returns a clear error.
			// Validation is now optimistic; rejection happens at ingest time, not here.
			name: "single_occurrence_canonical_id_only_location_accepted_at_validation",
			input: EventInput{
				Name:      "Event at Canonical Venue",
				StartDate: "2026-06-01T10:00:00Z",
				EndDate:   "2026-06-01T11:30:00Z",
				License:   "CC0-1.0",
				Location: &PlaceInput{
					ID: "https://example.com/places/01ARZ3NDEKTSV4RRFFQ69G5FAV",
					// Name intentionally omitted — @id only
				},
			},
			wantErr: false,
		},
		{
			// Regression: single-occurrence event with no location at all — must also be
			// rejected. (This was already covered by the location-required check, but
			// keep it here to document the single-occurrence-specific error path.)
			name: "single_occurrence_no_location_rejected",
			input: EventInput{
				Name:      "Nowhere Event",
				StartDate: "2026-06-01T10:00:00Z",
				EndDate:   "2026-06-01T11:30:00Z",
				License:   "CC0-1.0",
				// No Location, no VirtualLocation
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.ValidationConfig{AllowTestDomains: true}
			_, err := ValidateEventInputWithWarnings(tc.input, nodeDomain, nil, cfg)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ValidateEventInputWithWarnings() = nil; want an error")
				}
				if tc.wantErrContains != "" && !strings.Contains(err.Error(), tc.wantErrContains) {
					t.Errorf("error = %q; want it to contain %q", err.Error(), tc.wantErrContains)
				}
			} else {
				if err != nil {
					t.Fatalf("ValidateEventInputWithWarnings() error = %v; want nil", err)
				}
			}
		})
	}
}

func TestValidatePlaceInput(t *testing.T) {
	nodeDomain := "example.com"

	tests := []struct {
		name    string
		place   PlaceInput
		wantErr bool
	}{
		{
			name: "valid place with name",
			place: PlaceInput{
				Name: "Test Venue",
			},
			wantErr: false,
		},
		{
			name: "valid place with ID",
			place: PlaceInput{
				ID: "https://example.com/places/01ARZ3NDEKTSV4RRFFQ69G5FAV",
			},
			wantErr: false,
		},
		{
			name:    "invalid - no name or ID",
			place:   PlaceInput{},
			wantErr: true,
		},
		{
			name: "invalid - whitespace name",
			place: PlaceInput{
				Name: "   ",
			},
			wantErr: true,
		},
		{
			name: "invalid - bad canonical URI",
			place: PlaceInput{
				ID: "https://external.com/places/01ARZ3NDEKTSV4RRFFQ69G5FAV",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePlaceInput(tt.place, nodeDomain)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateEventInput_TestDomainBlocklist tests the example.com / images.example.com blocklist.
func TestValidateEventInput_TestDomainBlocklist(t *testing.T) {
	base := func() EventInput {
		return EventInput{
			Name:      "Test Event",
			StartDate: "2026-06-01T10:00:00Z",
			Location:  &PlaceInput{Name: "Venue"},
		}
	}

	tests := []struct {
		name             string
		url              string
		image            string
		allowTestDomains bool
		wantErrContains  string // non-empty means expect a hard error containing this string
	}{
		{
			name:            "example.com URL is blocked",
			url:             "https://example.com/events/1234",
			wantErrContains: "reserved test domain",
		},
		{
			name:            "images.example.com image is blocked",
			image:           "https://images.example.com/events/5.jpg",
			wantErrContains: "reserved test domain",
		},
		{
			name:  "real URL passes",
			url:   "https://realvenue.ca/event/abc",
			image: "https://cdn.realvenue.ca/img/abc.jpg",
		},
		{
			name:             "AllowTestDomains suppresses example.com URL error",
			url:              "https://example.com/events/99",
			allowTestDomains: true,
		},
		{
			name:             "AllowTestDomains suppresses images.example.com image error",
			image:            "https://images.example.com/events/3.jpg",
			allowTestDomains: true,
		},
		{
			name:            "subdomain of example.com is blocked",
			url:             "https://sub.example.com/event/1",
			wantErrContains: "reserved test domain",
		},
		{
			name:             "AllowTestDomains suppresses sub.example.com URL error",
			url:              "https://sub.example.com/event/1",
			allowTestDomains: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := base()
			input.URL = tc.url
			input.Image = tc.image

			cfg := config.ValidationConfig{AllowTestDomains: tc.allowTestDomains}
			result, err := ValidateEventInputWithWarnings(input, "node.togather.ca", nil, cfg)

			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrContains)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}
