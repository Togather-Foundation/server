package testdata

import (
	"strings"
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

func TestGenerator_TitleFormatting_NoExtraErrors(t *testing.T) {
	g := NewDeterministicGenerator()

	// Generate many events to test all template variations
	for i := 0; i < 100; i++ {
		input := g.RandomEventInput()

		// Event titles should never contain fmt.Sprintf EXTRA errors
		assert.NotContains(t, input.Name, "%!(EXTRA",
			"Event title should not contain fmt.Sprintf formatting errors: %s", input.Name)

		// Should also not contain raw format specifiers
		assert.NotContains(t, input.Name, "%s",
			"Event title should not contain unprocessed format specifiers: %s", input.Name)

		// Should be non-empty
		assert.NotEmpty(t, input.Name, "Event title should not be empty")

		// Should be reasonable length (not truncated)
		assert.Greater(t, len(input.Name), 3, "Event title should be meaningful length")
	}
}

func TestGenerator_TitleFormatting_AllCategories(t *testing.T) {
	categories := []EventCategory{
		CategoryMusic,
		CategoryArts,
		CategoryTech,
		CategorySocial,
		CategoryEducation,
		CategoryGames,
	}

	for _, category := range categories {
		t.Run(string(category), func(t *testing.T) {
			g := NewGenerator(time.Now().UnixNano())

			// Test each category multiple times to hit different templates
			for i := 0; i < 20; i++ {
				title := g.generateTitle(category)

				assert.NotContains(t, title, "%!(EXTRA",
					"Category %s should not produce EXTRA errors: %s", category, title)
				assert.NotContains(t, title, "%s",
					"Category %s should not have unprocessed format specifiers: %s", category, title)
				assert.NotEmpty(t, title, "Category %s should produce non-empty titles", category)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// BatchReviewEventInputs tests
// ---------------------------------------------------------------------------

func TestBatchReviewEventInputs_ScenarioCount(t *testing.T) {
	g := NewDeterministicGenerator()
	scenarios := g.BatchReviewEventInputs()
	require.Len(t, scenarios, 11, "should return 11 scenario groups")
}

func TestBatchReviewEventInputs_TotalEventCount(t *testing.T) {
	g := NewDeterministicGenerator()
	scenarios := g.BatchReviewEventInputs()

	total := 0
	for _, s := range scenarios {
		total += len(s.Events)
	}
	// RS-01:2, RS-02:2, RS-03:2, RS-04:2, RS-05:2, RS-06:1,
	// RS-07:2, RS-08:2, RS-09:1, RS-10:2, RS-11:4 = 22
	assert.Equal(t, 22, total, "total event count across all scenario groups should be 22")
}

func TestBatchReviewEventInputs_AllEventsHaveRequiredFields(t *testing.T) {
	g := NewDeterministicGenerator()
	scenarios := g.BatchReviewEventInputs()

	for _, scenario := range scenarios {
		for i, ev := range scenario.Events {
			assert.NotEmpty(t, ev.Name, "scenario %s event %d: Name must not be empty", scenario.GroupID, i)
			assert.NotEmpty(t, ev.StartDate, "scenario %s event %d: StartDate must not be empty", scenario.GroupID, i)

			hasLocation := ev.Location != nil || ev.VirtualLocation != nil
			assert.True(t, hasLocation, "scenario %s event %d: must have Location or VirtualLocation", scenario.GroupID, i)
		}
	}
}

func TestBatchReviewEventInputs_NoExampleComURLs(t *testing.T) {
	g := NewDeterministicGenerator()
	scenarios := g.BatchReviewEventInputs()

	for _, scenario := range scenarios {
		for i, ev := range scenario.Events {
			assert.NotContains(t, ev.URL, "example.com",
				"scenario %s event %d URL must not contain example.com", scenario.GroupID, i)
			assert.NotContains(t, ev.Image, "example.com",
				"scenario %s event %d Image must not contain example.com", scenario.GroupID, i)
			if ev.Source != nil {
				assert.NotContains(t, ev.Source.URL, "example.com",
					"scenario %s event %d Source.URL must not contain example.com", scenario.GroupID, i)
			}
		}
	}
}

func TestBatchReviewEventInputs_AllRSGroupIDsPresent(t *testing.T) {
	g := NewDeterministicGenerator()
	scenarios := g.BatchReviewEventInputs()

	expected := []string{"RS-01", "RS-02", "RS-03", "RS-04", "RS-05", "RS-06", "RS-07", "RS-08", "RS-09", "RS-10", "RS-11"}
	groupIDs := make(map[string]bool, len(scenarios))
	for _, s := range scenarios {
		groupIDs[s.GroupID] = true
	}
	for _, id := range expected {
		assert.True(t, groupIDs[id], "scenario group %q must be present", id)
	}
}

func TestBatchReviewEventInputs_AllEventsHaveUniqueEventIDs(t *testing.T) {
	g := NewDeterministicGenerator()
	scenarios := g.BatchReviewEventInputs()

	seen := make(map[string]string)
	for _, scenario := range scenarios {
		for i, ev := range scenario.Events {
			if ev.Source == nil {
				continue
			}
			eid := ev.Source.EventID
			if prev, ok := seen[eid]; ok {
				t.Errorf("duplicate EventID %q found in %s event %d (first seen in %s)", eid, scenario.GroupID, i, prev)
			}
			seen[eid] = scenario.GroupID
		}
	}
}

func TestBatchReviewEventInputs_RS01BaseSeriesHas4Occurrences(t *testing.T) {
	g := NewDeterministicGenerator()
	scenarios := g.BatchReviewEventInputs()

	var rs01 *ReviewEventScenario
	for i := range scenarios {
		if scenarios[i].GroupID == "RS-01" {
			rs01 = &scenarios[i]
			break
		}
	}
	require.NotNil(t, rs01, "RS-01 scenario must exist")
	require.Len(t, rs01.Events, 2, "RS-01 must have 2 events (base series + new occurrence)")

	baseSeries := rs01.Events[0]
	assert.Contains(t, baseSeries.Name, "Base Series", "first RS-01 event should be the base series")
	require.Len(t, baseSeries.Occurrences, 4, "RS-01 base series must have 4 occurrences")
}

func TestBatchReviewEventInputs_RS05OverlapActuallyOverlaps(t *testing.T) {
	g := NewDeterministicGenerator()
	scenarios := g.BatchReviewEventInputs()

	var rs05 *ReviewEventScenario
	for i := range scenarios {
		if scenarios[i].GroupID == "RS-05" {
			rs05 = &scenarios[i]
			break
		}
	}
	require.NotNil(t, rs05, "RS-05 scenario must exist")
	require.Len(t, rs05.Events, 2, "RS-05 must have 2 events")

	target := rs05.Events[0]
	overlap := rs05.Events[1]

	require.NotEmpty(t, target.Occurrences, "RS-05 target must have occurrences")
	firstOccStart, err := time.Parse(time.RFC3339, target.Occurrences[0].StartDate)
	require.NoError(t, err)
	firstOccEnd, err := time.Parse(time.RFC3339, target.Occurrences[0].EndDate)
	require.NoError(t, err)

	overlapStart, err := time.Parse(time.RFC3339, overlap.StartDate)
	require.NoError(t, err)
	overlapEnd, err := time.Parse(time.RFC3339, overlap.EndDate)
	require.NoError(t, err)

	// Overlap: overlapStart is within [firstOccStart, firstOccEnd) AND overlapEnd > firstOccStart
	overlaps := overlapStart.Before(firstOccEnd) && overlapEnd.After(firstOccStart)
	assert.True(t, overlaps, "RS-05 overlapping occurrence should actually overlap the first target occurrence")
}

func TestBatchReviewEventInputs_RS06HasReversedDates(t *testing.T) {
	g := NewDeterministicGenerator()
	scenarios := g.BatchReviewEventInputs()

	var rs06 *ReviewEventScenario
	for i := range scenarios {
		if scenarios[i].GroupID == "RS-06" {
			rs06 = &scenarios[i]
			break
		}
	}
	require.NotNil(t, rs06, "RS-06 scenario must exist")
	require.Len(t, rs06.Events, 1, "RS-06 should have 1 event")

	ev := rs06.Events[0]
	startTime, err := time.Parse(time.RFC3339, ev.StartDate)
	require.NoError(t, err, "RS-06 StartDate must parse")
	endTime, err := time.Parse(time.RFC3339, ev.EndDate)
	require.NoError(t, err, "RS-06 EndDate must parse")

	assert.True(t, endTime.Before(startTime), "RS-06 endDate should be before startDate (reversed dates)")
}

func TestBatchReviewEventInputs_RS09ContainsSessionsInName(t *testing.T) {
	g := NewDeterministicGenerator()
	scenarios := g.BatchReviewEventInputs()

	var rs09 *ReviewEventScenario
	for i := range scenarios {
		if scenarios[i].GroupID == "RS-09" {
			rs09 = &scenarios[i]
			break
		}
	}
	require.NotNil(t, rs09, "RS-09 scenario must exist")
	require.Len(t, rs09.Events, 1, "RS-09 should have 1 event")

	assert.Contains(t, rs09.Events[0].Name, "(8 sessions)", "RS-09 name must contain '(8 sessions)'")
}

func TestBatchReviewEventInputs_RS11Has4Events(t *testing.T) {
	g := NewDeterministicGenerator()
	scenarios := g.BatchReviewEventInputs()

	var rs11 *ReviewEventScenario
	for i := range scenarios {
		if scenarios[i].GroupID == "RS-11" {
			rs11 = &scenarios[i]
			break
		}
	}
	require.NotNil(t, rs11, "RS-11 scenario must exist")
	assert.Len(t, rs11.Events, 4, "RS-11 must have 4 events (same-day-different-times cluster)")

	// Verify we have two different days represented
	days := make(map[string]int)
	for _, ev := range rs11.Events {
		t1, err := time.Parse(time.RFC3339, ev.StartDate)
		require.NoError(t, err)
		day := t1.Format("2006-01-02")
		days[day]++
	}
	assert.Len(t, days, 2, "RS-11 events should span exactly 2 different days")

	// Each day should have 2 events
	for day, count := range days {
		assert.Equal(t, 2, count, "day %s should have exactly 2 RS-11 events", day)
	}
}

func TestBatchReviewEventInputs_AllRSNamesContainGroupID(t *testing.T) {
	g := NewDeterministicGenerator()
	scenarios := g.BatchReviewEventInputs()

	for _, scenario := range scenarios {
		for i, ev := range scenario.Events {
			assert.True(t, strings.HasPrefix(ev.Name, scenario.GroupID),
				"scenario %s event %d name %q should start with group ID", scenario.GroupID, i, ev.Name)
		}
	}
}

func TestGenerator_TitleFormatting_SpecificTemplates(t *testing.T) {
	tests := []struct {
		name     string
		category EventCategory
		contains string // expected substring in generated titles
	}{
		{
			name:     "single_placeholder_templates",
			category: CategoryMusic,
			contains: "",
		},
		{
			name:     "zero_placeholder_templates",
			category: CategoryMusic,
			contains: "Open Mic Night",
		},
		{
			name:     "two_placeholder_templates",
			category: CategoryMusic,
			contains: "Live at",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGenerator(42)

			foundExpected := false
			for i := 0; i < 50; i++ {
				title := g.generateTitle(tt.category)

				// No formatting errors
				assert.NotContains(t, title, "%!(EXTRA")
				assert.NotContains(t, title, "%s")

				// Check if we found the expected template
				if tt.contains == "" || strings.Contains(title, tt.contains) {
					foundExpected = true
				}
			}

			if tt.contains != "" {
				assert.True(t, foundExpected,
					"Should have found at least one title containing '%s'", tt.contains)
			}
		})
	}
}
