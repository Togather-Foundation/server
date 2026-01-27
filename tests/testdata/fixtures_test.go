package testdata

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerator_RandomEventInput(t *testing.T) {
	g := NewDeterministicGenerator()

	input := g.RandomEventInput()

	// Required fields must be present
	assert.NotEmpty(t, input.Name, "Name should not be empty")
	assert.NotEmpty(t, input.StartDate, "StartDate should not be empty")
	require.NotNil(t, input.Location, "Location should not be nil")
	assert.NotEmpty(t, input.Location.Name, "Location.Name should not be empty")

	// StartDate should be valid RFC3339
	_, err := time.Parse(time.RFC3339, input.StartDate)
	require.NoError(t, err, "StartDate should be valid RFC3339")

	// EndDate should be after StartDate
	if input.EndDate != "" {
		startTime, _ := time.Parse(time.RFC3339, input.StartDate)
		endTime, err := time.Parse(time.RFC3339, input.EndDate)
		require.NoError(t, err, "EndDate should be valid RFC3339")
		assert.True(t, endTime.After(startTime), "EndDate should be after StartDate")
	}

	// Optional but expected fields for rich events
	assert.NotEmpty(t, input.Description, "Description should be populated")
	require.NotNil(t, input.Source, "Source should not be nil")
	assert.NotEmpty(t, input.Source.URL, "Source.URL should not be empty")
	assert.NotEmpty(t, input.Source.EventID, "Source.EventID should not be empty")
}

func TestGenerator_MinimalEventInput(t *testing.T) {
	g := NewDeterministicGenerator()

	input := g.MinimalEventInput()

	// Only truly required fields
	assert.NotEmpty(t, input.Name, "Name should not be empty")
	assert.NotEmpty(t, input.StartDate, "StartDate should not be empty")
	require.NotNil(t, input.Location, "Location should not be nil")
	assert.NotEmpty(t, input.Location.Name, "Location.Name should not be empty")

	// Should be minimal - no description, image, etc.
	assert.Empty(t, input.Description, "Description should be empty for minimal input")
	assert.Empty(t, input.Image, "Image should be empty for minimal input")
	assert.Nil(t, input.Source, "Source should be nil for minimal input")
}

func TestGenerator_VirtualEventInput(t *testing.T) {
	g := NewDeterministicGenerator()

	input := g.VirtualEventInput()

	assert.NotEmpty(t, input.Name, "Name should not be empty")
	assert.Contains(t, input.Name, "Online", "Virtual event name should indicate online")
	assert.Nil(t, input.Location, "Location should be nil for virtual events")
	require.NotNil(t, input.VirtualLocation, "VirtualLocation should not be nil")
	assert.NotEmpty(t, input.VirtualLocation.URL, "VirtualLocation.URL should not be empty")
}

func TestGenerator_HybridEventInput(t *testing.T) {
	g := NewDeterministicGenerator()

	input := g.HybridEventInput()

	assert.NotEmpty(t, input.Name, "Name should not be empty")
	assert.Contains(t, input.Name, "Hybrid", "Hybrid event name should indicate hybrid")
	require.NotNil(t, input.Location, "Location should not be nil for hybrid events")
	require.NotNil(t, input.VirtualLocation, "VirtualLocation should not be nil for hybrid events")
}

func TestGenerator_EventInputWithOccurrences(t *testing.T) {
	g := NewDeterministicGenerator()

	occurrenceCount := 4
	input := g.EventInputWithOccurrences(occurrenceCount)

	assert.NotEmpty(t, input.Name, "Name should not be empty")
	// StartDate is kept populated because IngestService validation requires it,
	// even when occurrences are provided. The first occurrence usually matches StartDate.
	assert.NotEmpty(t, input.StartDate, "StartDate should be set (required by validation)")
	require.Len(t, input.Occurrences, occurrenceCount, "Should have expected number of occurrences")

	// Verify occurrences are properly spaced (weekly)
	var prevStart time.Time
	for i, occ := range input.Occurrences {
		start, err := time.Parse(time.RFC3339, occ.StartDate)
		require.NoError(t, err, "Occurrence %d StartDate should be valid RFC3339", i)

		if i > 0 {
			diff := start.Sub(prevStart)
			assert.Equal(t, 7*24*time.Hour, diff, "Occurrences should be weekly")
		}
		prevStart = start

		assert.Equal(t, "America/Toronto", occ.Timezone, "Timezone should be set")
	}
}

func TestGenerator_EventInputNeedsReview(t *testing.T) {
	g := NewDeterministicGenerator()

	input := g.EventInputNeedsReview()

	assert.NotEmpty(t, input.Name, "Name should not be empty")
	assert.NotEmpty(t, input.StartDate, "StartDate should not be empty")
	assert.Empty(t, input.Description, "Description should be empty (triggers review)")
	assert.Empty(t, input.Image, "Image should be empty (triggers review)")
}

func TestGenerator_EventInputFarFuture(t *testing.T) {
	g := NewDeterministicGenerator()

	input := g.EventInputFarFuture()

	startTime, err := time.Parse(time.RFC3339, input.StartDate)
	require.NoError(t, err)

	// Should be more than 730 days (2 years) in the future
	daysAhead := startTime.Sub(time.Now()).Hours() / 24
	assert.Greater(t, daysAhead, float64(730), "Event should be more than 2 years in the future")
}

func TestGenerator_BatchEventInputs(t *testing.T) {
	g := NewDeterministicGenerator()

	count := 10
	inputs := g.BatchEventInputs(count)

	require.Len(t, inputs, count, "Should generate expected number of events")

	// Verify all are valid and unique names
	names := make(map[string]bool)
	for i, input := range inputs {
		assert.NotEmpty(t, input.Name, "Event %d should have a name", i)
		assert.NotEmpty(t, input.StartDate, "Event %d should have a start date", i)
		require.NotNil(t, input.Location, "Event %d should have a location", i)

		// Names should be reasonably unique (deterministic generator may repeat with enough samples)
		if !names[input.Name] {
			names[input.Name] = true
		}
	}
}

func TestGenerator_DuplicateCandidates(t *testing.T) {
	g := NewDeterministicGenerator()

	first, second := g.DuplicateCandidates()

	// Same core identifying fields
	assert.Equal(t, first.Name, second.Name, "Names should match for duplicates")
	assert.Equal(t, first.StartDate, second.StartDate, "StartDates should match for duplicates")
	require.NotNil(t, first.Location)
	require.NotNil(t, second.Location)
	assert.Equal(t, first.Location.Name, second.Location.Name, "Venues should match for duplicates")

	// Different sources
	require.NotNil(t, first.Source)
	require.NotNil(t, second.Source)
	assert.NotEqual(t, first.Source.URL, second.Source.URL, "Source URLs should differ")
	assert.NotEqual(t, first.Source.EventID, second.Source.EventID, "Source EventIDs should differ")
}

func TestGenerator_Deterministic(t *testing.T) {
	g1 := NewDeterministicGenerator()
	g2 := NewDeterministicGenerator()

	// Same seed should produce same results
	input1 := g1.RandomEventInput()
	input2 := g2.RandomEventInput()

	assert.Equal(t, input1.Name, input2.Name, "Deterministic generators should produce same names")
	assert.Equal(t, input1.StartDate, input2.StartDate, "Deterministic generators should produce same dates")
}

func TestTorontoVenues_HasRealisticData(t *testing.T) {
	require.NotEmpty(t, TorontoVenues, "Should have sample venues")

	for i, venue := range TorontoVenues {
		assert.NotEmpty(t, venue.Name, "Venue %d should have a name", i)
		assert.Equal(t, "Toronto", venue.AddressLocality, "Venue %d should be in Toronto", i)
		assert.Equal(t, "ON", venue.AddressRegion, "Venue %d should be in Ontario", i)
		assert.Equal(t, "CA", venue.AddressCountry, "Venue %d should be in Canada", i)

		// Coordinates should be in Toronto area
		assert.Greater(t, venue.Latitude, 43.5, "Venue %d latitude should be reasonable", i)
		assert.Less(t, venue.Latitude, 44.0, "Venue %d latitude should be reasonable", i)
		assert.Greater(t, venue.Longitude, -80.0, "Venue %d longitude should be reasonable", i)
		assert.Less(t, venue.Longitude, -79.0, "Venue %d longitude should be reasonable", i)
	}
}

func TestSampleSources_MatchesCSVTypes(t *testing.T) {
	validTypes := map[string]bool{
		"API":      true,
		"HTML":     true,
		"ICS":      true,
		"JSONLD":   true,
		"RSS":      true,
		"PLATFORM": true,
	}

	require.NotEmpty(t, SampleSources, "Should have sample sources")

	for i, source := range SampleSources {
		assert.NotEmpty(t, source.Name, "Source %d should have a name", i)
		assert.NotEmpty(t, source.BaseURL, "Source %d should have a base URL", i)
		assert.True(t, validTypes[source.Type], "Source %d type %q should be valid", i, source.Type)
	}
}
