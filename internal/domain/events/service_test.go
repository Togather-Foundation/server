package events

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	paginationpkg "github.com/Togather-Foundation/server/internal/api/pagination"
	"github.com/stretchr/testify/require"
)

func TestParseFiltersDefaults(t *testing.T) {
	filters, pagination, _, err := ParseFilters(url.Values{}, nil)

	require.NoError(t, err)
	require.Equal(t, 50, pagination.Limit)
	require.Empty(t, pagination.After)
	// With no date params, startDate defaults to today so past events are excluded.
	require.NotNil(t, filters.StartDate)
	require.Nil(t, filters.EndDate)
	require.Empty(t, filters.City)
	require.Empty(t, filters.Region)
	require.Empty(t, filters.VenueULID)
	require.Empty(t, filters.OrganizerULID)
	require.Empty(t, filters.LifecycleState)
	require.Empty(t, filters.Query)
	require.Empty(t, filters.Domain)
	require.Nil(t, filters.Keywords)
}

func TestParseFiltersTrimsFields(t *testing.T) {
	validCursor := paginationpkg.EncodeEventCursor(time.Unix(1706886000, 0), "01HYX3KQW7ERTV9XNBM2P8QJZF")

	values := url.Values{}
	values.Set("city", "  Portland  ")
	values.Set("region", "  OR ")
	values.Set("q", "  jazz night ")
	values.Set("after", "  "+validCursor+" ")

	filters, pagination, _, err := ParseFilters(values, nil)

	require.NoError(t, err)
	require.Equal(t, "Portland", filters.City)
	require.Equal(t, "OR", filters.Region)
	require.Equal(t, "jazz night", filters.Query)
	require.Equal(t, validCursor, pagination.After)
}

func TestParseFiltersDateValidation(t *testing.T) {
	values := url.Values{}
	values.Set("startDate", "2024-01-02")
	values.Set("endDate", "2024-01-01")

	_, _, _, err := ParseFilters(values, nil)

	assertFilterError(t, err, "endDate", "must be on or after startDate")
}

func TestParseFiltersDateFormat(t *testing.T) {
	values := url.Values{}
	values.Set("startDate", "01-02-2024")

	_, _, _, err := ParseFilters(values, nil)

	assertFilterError(t, err, "startDate", "must be ISO8601 date")
}

func TestParseFiltersDateSuccess(t *testing.T) {
	values := url.Values{}
	values.Set("startDate", "2024-01-01")
	values.Set("endDate", "2024-01-02")

	filters, _, _, err := ParseFilters(values, nil)

	require.NoError(t, err)
	require.NotNil(t, filters.StartDate)
	require.NotNil(t, filters.EndDate)
	require.Equal(t, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), *filters.StartDate)
	require.Equal(t, time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), *filters.EndDate)
}

func TestParseFiltersVenueULIDValidation(t *testing.T) {
	values := url.Values{}
	values.Set("venueId", "not-a-ulid")

	_, _, _, err := ParseFilters(values, nil)

	assertFilterError(t, err, "venueId", "invalid ULID")
}

func TestParseFiltersOrganizerULIDValidation(t *testing.T) {
	values := url.Values{}
	values.Set("organizerId", "not-a-ulid")

	_, _, _, err := ParseFilters(values, nil)

	assertFilterError(t, err, "organizerId", "invalid ULID")
}

func TestParseFiltersLifecycleStateValidation(t *testing.T) {
	values := url.Values{}
	values.Set("state", "PUBLISHED")

	filters, _, _, err := ParseFilters(values, nil)

	require.NoError(t, err)
	require.Equal(t, "published", filters.LifecycleState)

	values.Set("state", "unknown")

	_, _, _, err = ParseFilters(values, nil)

	assertFilterError(t, err, "state", "unsupported lifecycle state")
}

func TestParseFiltersDomainValidation(t *testing.T) {
	values := url.Values{}
	values.Set("domain", "Arts")

	filters, _, _, err := ParseFilters(values, nil)

	require.NoError(t, err)
	require.Equal(t, "arts", filters.Domain)

	values.Set("domain", "invalid")

	_, _, _, err = ParseFilters(values, nil)

	assertFilterError(t, err, "domain", "unsupported event domain")
}

func TestParseFiltersKeywords(t *testing.T) {
	values := url.Values{}
	values.Set("keywords", " jazz, , blues ,rock ")

	filters, _, _, err := ParseFilters(values, nil)

	require.NoError(t, err)
	require.Equal(t, []string{"jazz", "blues", "rock"}, filters.Keywords)
}

func TestParseFiltersLimitValidation(t *testing.T) {
	values := url.Values{}
	values.Set("limit", "abc")

	_, _, _, err := ParseFilters(values, nil)

	assertFilterError(t, err, "limit", "must be a number")

	values.Set("limit", "0")

	_, _, _, err = ParseFilters(values, nil)

	assertFilterError(t, err, "limit", "must be between 1 and 200")
}

func TestParseFiltersLimitSuccess(t *testing.T) {
	values := url.Values{}
	values.Set("limit", "200")

	_, pagination, _, err := ParseFilters(values, nil)

	require.NoError(t, err)
	require.Equal(t, 200, pagination.Limit)
}

func TestParseFiltersAfterCursorValidation(t *testing.T) {
	t.Run("valid cursor", func(t *testing.T) {
		validCursor := paginationpkg.EncodeEventCursor(time.Unix(1706886000, 0), "01HYX3KQW7ERTV9XNBM2P8QJZF")
		values := url.Values{}
		values.Set("after", validCursor)

		_, pagination, _, err := ParseFilters(values, nil)

		require.NoError(t, err)
		require.Equal(t, validCursor, pagination.After)
	})

	t.Run("empty cursor is valid", func(t *testing.T) {
		values := url.Values{}
		values.Set("after", "")

		_, pagination, _, err := ParseFilters(values, nil)

		require.NoError(t, err)
		require.Empty(t, pagination.After)
	})

	t.Run("whitespace-only cursor is treated as empty", func(t *testing.T) {
		values := url.Values{}
		values.Set("after", "   ")

		_, pagination, _, err := ParseFilters(values, nil)

		require.NoError(t, err)
		require.Empty(t, pagination.After)
	})

	t.Run("invalid cursor - RFC3339 timestamp", func(t *testing.T) {
		values := url.Values{}
		values.Set("after", "2026-01-01T00:00:00Z")

		_, _, _, err := ParseFilters(values, nil)

		assertFilterError(t, err, "after", "must be a valid cursor")
	})

	t.Run("invalid cursor - arbitrary string", func(t *testing.T) {
		values := url.Values{}
		values.Set("after", "not-a-valid-cursor")

		_, _, _, err := ParseFilters(values, nil)

		assertFilterError(t, err, "after", "must be a valid cursor")
	})

	t.Run("invalid cursor - raw ULID", func(t *testing.T) {
		values := url.Values{}
		values.Set("after", "01HYX3KQW7ERTV9XNBM2P8QJZF")

		_, _, _, err := ParseFilters(values, nil)

		assertFilterError(t, err, "after", "must be a valid cursor")
	})

	t.Run("invalid cursor - too short", func(t *testing.T) {
		values := url.Values{}
		values.Set("after", "123")

		_, _, _, err := ParseFilters(values, nil)

		assertFilterError(t, err, "after", "must be a valid cursor")
	})
}

func assertFilterError(t *testing.T, err error, field string, message string) {
	t.Helper()

	require.Error(t, err)

	var filterErr FilterError
	if errors.As(err, &filterErr) {
		require.Equal(t, field, filterErr.Field)
		require.Equal(t, message, filterErr.Message)
		return
	}

	require.Failf(t, "unexpected error type", "err=%T %v", err, err)
}

// Tests for Service methods

func TestNewService(t *testing.T) {
	repo := NewMockRepository()
	svc := NewService(repo)

	require.NotNil(t, svc)
	require.Equal(t, repo, svc.repo)
}

func TestService_List(t *testing.T) {
	ctx := context.Background()
	repo := NewMockRepository()
	svc := NewService(repo)

	filters := Filters{City: "Vancouver"}
	pagination := Pagination{Limit: 10}

	result, err := svc.List(ctx, filters, pagination)
	require.NoError(t, err)
	require.Empty(t, result.Events)
	require.Empty(t, result.NextCursor)
}

func TestService_GetByULID(t *testing.T) {
	ctx := context.Background()
	repo := NewMockRepository()
	svc := NewService(repo)

	// Test non-existent event returns ErrNotFound
	_, err := svc.GetByULID(ctx, "01ARZ3NDEKTSV4RRFFQ69G5FAV")
	require.ErrorIs(t, err, ErrNotFound)

	// Add an event and retrieve it
	testEvent := &Event{
		ULID: "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		Name: "Test Event",
	}
	repo.AddExistingEvent("test-source", "test-event-1", testEvent)

	event, err := svc.GetByULID(ctx, testEvent.ULID)
	require.NoError(t, err)
	require.NotNil(t, event)
	require.Equal(t, testEvent.ULID, event.ULID)
	require.Equal(t, testEvent.Name, event.Name)
}

func TestFilterError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      FilterError
		expected string
	}{
		{
			name:     "with field",
			err:      FilterError{Field: "startDate", Message: "must be ISO8601 date"},
			expected: "invalid startDate: must be ISO8601 date",
		},
		{
			name:     "without field",
			err:      FilterError{Message: "something went wrong"},
			expected: "something went wrong",
		},
		{
			name:     "empty field",
			err:      FilterError{Field: "", Message: "error message"},
			expected: "error message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Error()
			require.Equal(t, tt.expected, result)
		})
	}
}

// ─── srv-h7j38: default startDate=today ──────────────────────────────────────

func TestParseFilters_DefaultStartDateToday(t *testing.T) {
	loc, err := time.LoadLocation("America/Toronto")
	require.NoError(t, err)

	// Capture now before and after the call; compute today in both snapshots.
	// If the call does not straddle midnight both will agree. If it does, we
	// accept either value so the test stays green during the one-second window
	// where a real midnight crossing could occur.
	before := time.Now().In(loc)
	filters, _, _, err := ParseFilters(url.Values{}, loc)
	after := time.Now().In(loc)

	require.NoError(t, err)
	require.NotNil(t, filters.StartDate, "startDate should default to today when no date params provided")
	require.Nil(t, filters.EndDate)

	todayBefore := time.Date(before.Year(), before.Month(), before.Day(), 0, 0, 0, 0, loc)
	todayAfter := time.Date(after.Year(), after.Month(), after.Day(), 0, 0, 0, 0, loc)
	require.True(t,
		filters.StartDate.Equal(todayBefore) || filters.StartDate.Equal(todayAfter),
		"startDate %v should be today at midnight in %s (before=%v, after=%v)",
		*filters.StartDate, loc, todayBefore, todayAfter,
	)
}

func TestParseFilters_ExplicitStartDateNoDefault(t *testing.T) {
	values := url.Values{}
	values.Set("startDate", "2026-06-01")

	filters, _, _, err := ParseFilters(values, nil)

	require.NoError(t, err)
	require.NotNil(t, filters.StartDate)
	require.Equal(t, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), *filters.StartDate)
	// No end date was provided, so EndDate is nil.
	require.Nil(t, filters.EndDate)
}

func TestParseFilters_ExplicitEndDateOnlyNoDefault(t *testing.T) {
	// Caller provided only endDate (requesting historical range) — startDate must NOT be defaulted.
	values := url.Values{}
	values.Set("endDate", "2026-12-31")

	filters, _, _, err := ParseFilters(values, nil)

	require.NoError(t, err)
	require.Nil(t, filters.StartDate, "startDate must NOT be defaulted when endDate is explicitly provided")
	require.NotNil(t, filters.EndDate)
}

// ─── srv-gvmef: snake_case alias warnings ────────────────────────────────────

func TestParseFilters_SnakeCaseStartDate(t *testing.T) {
	values := url.Values{}
	values.Set("start_date", "2026-06-01")

	filters, _, warnings, err := ParseFilters(values, nil)

	require.NoError(t, err)
	require.NotNil(t, filters.StartDate)
	require.Equal(t, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), *filters.StartDate)
	require.Len(t, warnings, 1)
	require.Contains(t, warnings[0], "start_date")
	require.Contains(t, warnings[0], "startDate")
}

func TestParseFilters_SnakeCaseEndDate(t *testing.T) {
	values := url.Values{}
	values.Set("start_date", "2026-06-01")
	values.Set("end_date", "2026-06-30")

	filters, _, warnings, err := ParseFilters(values, nil)

	require.NoError(t, err)
	require.NotNil(t, filters.StartDate)
	require.NotNil(t, filters.EndDate)
	require.Len(t, warnings, 2)
}

func TestParseFilters_SnakeCaseVenueId(t *testing.T) {
	values := url.Values{}
	values.Set("start_date", "2026-06-01") // prevent default-today from interfering
	values.Set("venue_id", "01ARZ3NDEKTSV4RRFFQ69G5FAV")

	filters, _, warnings, err := ParseFilters(values, nil)

	require.NoError(t, err)
	require.Equal(t, "01ARZ3NDEKTSV4RRFFQ69G5FAV", filters.VenueULID)
	require.True(t, len(warnings) >= 1)
	var found bool
	for _, w := range warnings {
		if strings.Contains(w, "venue_id") {
			found = true
		}
	}
	require.True(t, found, "expected warning about venue_id alias")
}

func TestParseFilters_SnakeCaseOrganizerId(t *testing.T) {
	values := url.Values{}
	values.Set("start_date", "2026-06-01")
	values.Set("organizer_id", "01ARZ3NDEKTSV4RRFFQ69G5FAV")

	filters, _, warnings, err := ParseFilters(values, nil)

	require.NoError(t, err)
	require.Equal(t, "01ARZ3NDEKTSV4RRFFQ69G5FAV", filters.OrganizerULID)
	require.True(t, len(warnings) >= 1)
	var found bool
	for _, w := range warnings {
		if strings.Contains(w, "organizer_id") {
			found = true
		}
	}
	require.True(t, found, "expected warning about organizer_id alias")
}

func TestParseFilters_SnakeCaseLifecycleState(t *testing.T) {
	values := url.Values{}
	values.Set("start_date", "2026-06-01")
	values.Set("lifecycle_state", "published")

	filters, _, warnings, err := ParseFilters(values, nil)

	require.NoError(t, err)
	require.Equal(t, "published", filters.LifecycleState)
	require.True(t, len(warnings) >= 1)
	var found bool
	for _, w := range warnings {
		if strings.Contains(w, "lifecycle_state") {
			found = true
		}
	}
	require.True(t, found, "expected warning about lifecycle_state alias")
}

func TestParseFilters_SnakeCaseEventDomain(t *testing.T) {
	values := url.Values{}
	values.Set("start_date", "2026-06-01")
	values.Set("event_domain", "arts")

	filters, _, warnings, err := ParseFilters(values, nil)

	require.NoError(t, err)
	require.Equal(t, "arts", filters.Domain)
	require.True(t, len(warnings) >= 1)
	var found bool
	for _, w := range warnings {
		if strings.Contains(w, "event_domain") {
			found = true
		}
	}
	require.True(t, found, "expected warning about event_domain alias")
}

func TestParseFilters_CanonicalWinsOverAlias(t *testing.T) {
	// When both canonical and alias are present, canonical wins with no warning.
	values := url.Values{}
	values.Set("startDate", "2026-06-01")
	values.Set("start_date", "2026-01-01")

	filters, _, warnings, err := ParseFilters(values, nil)

	require.NoError(t, err)
	require.NotNil(t, filters.StartDate)
	require.Equal(t, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), *filters.StartDate, "canonical startDate should win")
	for _, w := range warnings {
		require.NotContains(t, w, "start_date", "no warning when canonical param is present")
	}
}

func TestParseFilters_MultipleAliasesMultipleWarnings(t *testing.T) {
	values := url.Values{}
	values.Set("start_date", "2026-06-01")
	values.Set("end_date", "2026-06-30")

	_, _, warnings, err := ParseFilters(values, nil)

	require.NoError(t, err)
	require.Len(t, warnings, 2)
}

// ─── srv-1uvo0: nil-loc guard coverage ───────────────────────────────────────

// TestParseFilters_NilLocEqualsUTC asserts that passing loc=nil produces the
// same StartDate as passing loc=time.UTC explicitly, confirming the nil guard
// at service.go:61-63 falls back to UTC correctly.
func TestParseFilters_NilLocEqualsUTC(t *testing.T) {
	values := url.Values{}
	values.Set("startDate", "2026-06-01")

	filtersNil, _, _, err := ParseFilters(values, nil)
	require.NoError(t, err)

	filtersUTC, _, _, err := ParseFilters(values, time.UTC)
	require.NoError(t, err)

	require.NotNil(t, filtersNil.StartDate)
	require.NotNil(t, filtersUTC.StartDate)
	require.Equal(t, *filtersUTC.StartDate, *filtersNil.StartDate,
		"nil loc should behave identically to time.UTC")
}

// TestParseFilters_NilLocAliasWarning asserts that using a snake_case alias
// (start_date) together with loc=nil still produces a warning AND returns the
// correct parsed date — the nil guard must not suppress alias detection.
func TestParseFilters_NilLocAliasWarning(t *testing.T) {
	values := url.Values{}
	values.Set("start_date", "2026-06-01")

	filters, _, warnings, err := ParseFilters(values, nil)

	require.NoError(t, err)
	require.NotNil(t, filters.StartDate)
	require.Equal(t, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), *filters.StartDate,
		"date should be parsed correctly even when loc=nil")
	require.Len(t, warnings, 1, "alias warning must still be emitted when loc=nil")
	require.Contains(t, warnings[0], "start_date")
	require.Contains(t, warnings[0], "startDate")
}
