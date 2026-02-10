package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestEscapeILIKEPattern(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "normal text",
			input:    "Toronto",
			expected: "Toronto",
		},
		{
			name:     "percent sign",
			input:    "100% effort",
			expected: `100\% effort`,
		},
		{
			name:     "underscore",
			input:    "test_pattern",
			expected: `test\_pattern`,
		},
		{
			name:     "backslash",
			input:    `test\path`,
			expected: `test\\path`,
		},
		{
			name:     "SQL injection attempt",
			input:    `%'; DROP TABLE events; --`,
			expected: `\%'; DROP TABLE events; --`,
		},
		{
			name:     "multiple wildcards",
			input:    `%_test_%_`,
			expected: `\%\_test\_\%\_`,
		},
		{
			name:     "mixed escape characters",
			input:    `\%_test`,
			expected: `\\\%\_test`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeILIKEPattern(tt.input)
			if got != tt.expected {
				t.Errorf("escapeILIKEPattern(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestEventRepository_ILIKEInjectionPrevention(t *testing.T) {
	ctx := context.Background()
	container, dbURL := setupPostgres(t, ctx)
	defer func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}()

	pool, err := pgxpool.New(ctx, dbURL)
	require.NoError(t, err)
	defer pool.Close()

	repo := &EventRepository{pool: pool}

	// Setup test data
	org := insertOrganization(t, ctx, pool, "Test Org")
	place := insertPlace(t, ctx, pool, "Test Venue", "Toronto", "ON")
	startTime := time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC)

	// Create events with special characters that could be used for injection
	testCases := []struct {
		name        string
		eventName   string
		searchQuery string
		shouldMatch bool
		description string
	}{
		{
			name:        "percent wildcard should be escaped",
			eventName:   "Event with %percent% signs",
			searchQuery: "%percent%",
			shouldMatch: true,
			description: "Event with literal % signs should match when searching for literal %",
		},
		{
			name:        "underscore wildcard should be escaped",
			eventName:   "Event with _underscore_",
			searchQuery: "_underscore_",
			shouldMatch: true,
			description: "Event with literal _ should match when searching for literal _",
		},
		{
			name:        "backslash should be safely escaped",
			eventName:   `Event with \backslash\`,
			searchQuery: `\backslash\`,
			shouldMatch: true,
			description: "Event with backslash should match when searching for literal backslash",
		},
		{
			name:        "combined special chars should be escaped",
			eventName:   `Event with %_\ combo`,
			searchQuery: `%_\`,
			shouldMatch: true,
			description: "Event with multiple special chars should match when searching for those chars",
		},
	}

	// Insert all test events
	eventULIDs := make(map[string]string)
	for _, tc := range testCases {
		ulid := insertEvent(t, ctx, pool, tc.eventName, tc.description, org, place, "arts", "published", nil, startTime)
		eventULIDs[tc.eventName] = ulid
	}

	// Test each scenario
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			filters := events.Filters{Query: tc.searchQuery}
			result, err := repo.List(ctx, filters, events.Pagination{Limit: 100})
			require.NoError(t, err)

			// Check if the specific event was found
			found := false
			expectedULID := eventULIDs[tc.eventName]
			for _, evt := range result.Events {
				if evt.ULID == expectedULID {
					found = true
					break
				}
			}

			if tc.shouldMatch {
				require.True(t, found, "Expected to find event %q when searching for %q", tc.eventName, tc.searchQuery)
			} else {
				require.False(t, found, "Expected NOT to find event %q when searching for %q", tc.eventName, tc.searchQuery)
			}
		})
	}

	// Additional test: Ensure special chars are treated as literals, not wildcards
	t.Run("percent sign in search does not wildcard match", func(t *testing.T) {
		// Create two events: one with literal "% " and one with any character + space
		ulid1 := insertEvent(t, ctx, pool, "100% Complete", "Has literal percent", org, place, "arts", "published", nil, startTime.Add(1*time.Hour))
		_ = insertEvent(t, ctx, pool, "100X Complete", "Has X not percent", org, place, "arts", "published", nil, startTime.Add(2*time.Hour))

		// Search for "100% " - should only match the first event, not wildcard match "100X"
		filters := events.Filters{Query: "100%"}
		result, err := repo.List(ctx, filters, events.Pagination{Limit: 100})
		require.NoError(t, err)

		// Should only find the event with literal "%"
		require.Equal(t, 1, len(result.Events), "Searching for 100%% should only find '100%% Complete', not '100X Complete'")
		require.Equal(t, ulid1, result.Events[0].ULID)
	})

	// Additional test: Ensure underscore doesn't wildcard match single chars
	t.Run("underscore in search does not wildcard match", func(t *testing.T) {
		// Create two events: one with literal "_" and one with a different character
		ulid1 := insertEvent(t, ctx, pool, "test_event", "Has underscore", org, place, "arts", "published", nil, startTime.Add(3*time.Hour))
		_ = insertEvent(t, ctx, pool, "testXevent", "Has X not underscore", org, place, "arts", "published", nil, startTime.Add(4*time.Hour))

		// Search for "test_event" - should only match the first event
		filters := events.Filters{Query: "test_"}
		result, err := repo.List(ctx, filters, events.Pagination{Limit: 100})
		require.NoError(t, err)

		// Should only find the event with literal "_"
		require.Equal(t, 1, len(result.Events), "Searching for test_ should only find 'test_event', not 'testXevent'")
		require.Equal(t, ulid1, result.Events[0].ULID)
	})
}
