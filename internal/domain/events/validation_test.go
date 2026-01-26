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
