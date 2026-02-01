package events

import (
	"strings"
	"testing"

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
	input.EndDate = "2026-02-01T10:00:00Z"
	_, err := ValidateEventInput(input, "example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "endDate")
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
			name: "invalid occurrence - end before start",
			input: EventInput{
				Name:      "Test Event",
				StartDate: "2026-02-01T10:00:00Z",
				Occurrences: []OccurrenceInput{
					{
						StartDate: "2026-02-01T12:00:00Z",
						EndDate:   "2026-02-01T10:00:00Z",
					},
				},
				Location: &PlaceInput{Name: "Test Venue"},
			},
			wantErr: true,
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
