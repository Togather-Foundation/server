package events

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	paginationpkg "github.com/Togather-Foundation/server/internal/api/pagination"
	"github.com/Togather-Foundation/server/internal/config"
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

func TestParseFiltersWarnsOnUnknownParams(t *testing.T) {
	values := url.Values{}
	values.Set("city", "Toronto")
	values.Set("lat", "43.6656")
	values.Set("lng", "-79.4113")
	values.Set("radius_km", "2")

	_, _, warnings, err := ParseFilters(values, nil)

	require.NoError(t, err)
	require.Contains(t, strings.Join(warnings, " "), "lat")
	require.Contains(t, strings.Join(warnings, " "), "lng")
	require.Contains(t, strings.Join(warnings, " "), "radius_km")
}

func TestParseFiltersNoWarningsForKnownParams(t *testing.T) {
	validCursor := paginationpkg.EncodeEventCursor(time.Unix(1706886000, 0), "01HYX3KQW7ERTV9XNBM2P8QJZF")

	values := url.Values{}
	values.Set("startDate", "2024-01-01")
	values.Set("endDate", "2024-01-31")
	values.Set("venueId", "01HYX3KQW7ERTV9XNBM2P8QJZF")
	values.Set("organizerId", "01HYX3KQW7ERTV9XNBM2P8QJZF")
	values.Set("state", "published")
	values.Set("domain", "music")
	values.Set("city", "Toronto")
	values.Set("region", "ON")
	values.Set("q", "jazz")
	values.Set("search", "jazz")
	values.Set("keywords", "jazz,night")
	values.Set("limit", "20")
	values.Set("after", validCursor)
	values.Set("cursor", validCursor)
	values.Set("start_date", "2024-01-02")
	values.Set("end_date", "2024-01-03")
	values.Set("venue_id", "01HYX3KQW7ERTV9XNBM2P8QJZF")
	values.Set("organizer_id", "01HYX3KQW7ERTV9XNBM2P8QJZF")
	values.Set("lifecycle_state", "published")
	values.Set("event_domain", "music")

	_, _, warnings, err := ParseFilters(values, nil)

	require.NoError(t, err)
	require.Empty(t, warnings)
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
	// endDate advances to next-day midnight so the SQL bound covers the entire
	// endDate day (events up to 23:59:59 on 2024-01-02).
	require.Equal(t, time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC), *filters.EndDate)
}

func TestParseFilters_EndDateIncludesEntireDay(t *testing.T) {
	// endDate=2026-08-08 must include events starting any time on that day
	// (up to 23:59:59), so the stored bound is next-day midnight.
	values := url.Values{}
	values.Set("endDate", "2026-08-08")

	filters, _, _, err := ParseFilters(values, nil)

	require.NoError(t, err)
	require.Nil(t, filters.StartDate)
	require.NotNil(t, filters.EndDate)
	require.Equal(t, time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC), *filters.EndDate)
}

func TestParseFilters_EndDateSameDayAsStartDate(t *testing.T) {
	// startDate == endDate is a valid same-day range: no error, no collapse.
	// endDate still advances to next-day midnight so the whole day is included.
	values := url.Values{}
	values.Set("startDate", "2026-08-08")
	values.Set("endDate", "2026-08-08")

	filters, _, _, err := ParseFilters(values, nil)

	require.NoError(t, err)
	require.NotNil(t, filters.StartDate)
	require.NotNil(t, filters.EndDate)
	require.Equal(t, time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC), *filters.StartDate)
	require.Equal(t, time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC), *filters.EndDate)
}

func TestParseFilters_EndDateDayBeforeStartDate(t *testing.T) {
	// endDate one day before startDate is still invalid — the day advance must
	// not mask a genuine endDate < startDate ordering.
	values := url.Values{}
	values.Set("startDate", "2026-08-09")
	values.Set("endDate", "2026-08-08")

	_, _, _, err := ParseFilters(values, nil)

	assertFilterError(t, err, "endDate", "must be on or after startDate")
}

func TestParseFilters_EndDateAdvancesAcrossDST(t *testing.T) {
	loc, err := time.LoadLocation("America/Toronto")
	require.NoError(t, err)

	tests := []struct {
		name    string
		endDate string
		want    time.Time // next-day midnight in America/Toronto
	}{
		{
			name:    "spring forward",
			endDate: "2024-03-10",
			want:    time.Date(2024, 3, 11, 0, 0, 0, 0, loc), // EDT (UTC-4)
		},
		{
			name:    "fall back",
			endDate: "2024-11-03",
			want:    time.Date(2024, 11, 4, 0, 0, 0, 0, loc), // EST (UTC-5)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := url.Values{}
			values.Set("endDate", tt.endDate)

			filters, _, _, err := ParseFilters(values, loc)

			require.NoError(t, err)
			require.NotNil(t, filters.EndDate)
			require.True(t, filters.EndDate.Equal(tt.want),
				"endDate %v should equal %v (instant across DST)", *filters.EndDate, tt.want)
			_, gotOffset := filters.EndDate.Zone()
			_, wantOffset := tt.want.Zone()
			require.Equal(t, wantOffset, gotOffset,
				"endDate must be next-day midnight in the server timezone, not UTC")
		})
	}
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

func TestParseFiltersCursorAlias(t *testing.T) {
	validCursor := paginationpkg.EncodeEventCursor(time.Unix(1706886000, 0), "01HYX3KQW7ERTV9XNBM2P8QJZF")

	t.Run("cursor alias sets pagination.After and warns", func(t *testing.T) {
		values := url.Values{}
		values.Set("startDate", "2026-06-01")
		values.Set("cursor", validCursor)

		_, pagination, warnings, err := ParseFilters(values, nil)

		require.NoError(t, err)
		require.Equal(t, validCursor, pagination.After)
		joined := strings.Join(warnings, " ")
		require.Contains(t, joined, "cursor")
		require.Contains(t, joined, "after")
	})

	t.Run("canonical after wins over cursor with no alias warning", func(t *testing.T) {
		other := paginationpkg.EncodeEventCursor(time.Unix(1706886001, 0), "01HYX3KQW7ERTV9XNBM2P8QJZF")
		values := url.Values{}
		values.Set("startDate", "2026-06-01")
		values.Set("after", validCursor)
		values.Set("cursor", other)

		_, pagination, warnings, err := ParseFilters(values, nil)

		require.NoError(t, err)
		require.Equal(t, validCursor, pagination.After)
		for _, w := range warnings {
			require.NotContains(t, w, "cursor")
		}
	})

	t.Run("invalid cursor via alias errors", func(t *testing.T) {
		values := url.Values{}
		values.Set("startDate", "2026-06-01")
		values.Set("cursor", "not-a-valid-cursor")

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

	filters := Filters{}
	pagination := Pagination{Limit: 10}

	result, err := svc.List(ctx, filters, pagination)
	require.NoError(t, err)
	require.Empty(t, result.Events)
	require.Empty(t, result.NextCursor)
}

// TestService_ListCityBoundary validates that the city/region filters are
// interpreted against the node's configured geographic boundary, not against
// address_locality data. A requested city/region within the node's boundary
// returns all events (the node is single-scope); one outside returns nothing.
// Regression for #19: previously the SQL filtered on p.address_locality,
// silently dropping events whose venue had a null or mis-parsed locality.
func TestService_ListCityBoundary(t *testing.T) {
	ctx := context.Background()

	boundary := config.GeographicBoundaryConfig{
		Regions:    []string{"Ontario"},
		Localities: []string{"Toronto", "Mississauga", "North York"},
	}

	repo := NewMockRepository()
	repo.AddEvent(&Event{ULID: "01HX1234567890ABCDEFGHJKMN", Name: "In Scope"})
	svc := NewService(repo).WithGeographicBoundaryConfig(boundary)

	t.Run("city within node boundary returns all events", func(t *testing.T) {
		result, err := svc.List(ctx, Filters{City: "Toronto"}, Pagination{Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Events, 1)
	})

	t.Run("region within node boundary returns all events", func(t *testing.T) {
		result, err := svc.List(ctx, Filters{Region: "Ontario"}, Pagination{Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Events, 1)
	})

	t.Run("city outside node boundary returns empty", func(t *testing.T) {
		result, err := svc.List(ctx, Filters{City: "Ottawa"}, Pagination{Limit: 10})
		require.NoError(t, err)
		require.Empty(t, result.Events)
	})

	t.Run("no location input returns all events (node scope)", func(t *testing.T) {
		result, err := svc.List(ctx, Filters{}, Pagination{Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Events, 1)
	})
}

// TestService_ListCityBoundaryNoConfig verifies that a node with no geographic
// boundary configured treats city/region as a no-op (no filtering), so existing
// deployments without a boundary.yaml keep working.
func TestService_ListCityBoundaryNoConfig(t *testing.T) {
	ctx := context.Background()

	repo := NewMockRepository()
	repo.AddEvent(&Event{ULID: "01HX1234567890ABCDEFGHJKMN", Name: "In Scope"})
	svc := NewService(repo) // no WithGeographicBoundaryConfig

	result, err := svc.List(ctx, Filters{City: "Toronto"}, Pagination{Limit: 10})
	require.NoError(t, err)
	require.Len(t, result.Events, 1)
}

// TestService_ListCityBoundaryEdgeCases pins the mixed-dimension and partial-
// config semantics of the node-boundary check: an unconfigured dimension is not
// filtered; both filters set requires BOTH to pass; normalization handles case
// and surrounding whitespace.
func TestService_ListCityBoundaryEdgeCases(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name     string
		boundary config.GeographicBoundaryConfig
		filters  Filters
		wantLen  int // number of events expected (1 = in scope, 0 = filtered out)
	}{
		{
			name:     "localities-only boundary ignores region filter",
			boundary: config.GeographicBoundaryConfig{Localities: []string{"Toronto"}},
			filters:  Filters{Region: "Quebec"},
			wantLen:  1,
		},
		{
			name:     "regions-only boundary ignores city filter",
			boundary: config.GeographicBoundaryConfig{Regions: []string{"Ontario"}},
			filters:  Filters{City: "Montreal"},
			wantLen:  1,
		},
		{
			name:     "both filters within boundary",
			boundary: config.GeographicBoundaryConfig{Regions: []string{"Ontario"}, Localities: []string{"Toronto"}},
			filters:  Filters{City: "Toronto", Region: "Ontario"},
			wantLen:  1,
		},
		{
			name:     "city outside rejects even when region within",
			boundary: config.GeographicBoundaryConfig{Regions: []string{"Ontario"}, Localities: []string{"Toronto"}},
			filters:  Filters{City: "Ottawa", Region: "Ontario"},
			wantLen:  0,
		},
		{
			name:     "normalized city matches (case + whitespace)",
			boundary: config.GeographicBoundaryConfig{Localities: []string{"Toronto"}},
			filters:  Filters{City: "  TORONTO  "},
			wantLen:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewMockRepository()
			repo.AddEvent(&Event{ULID: "01HX1234567890ABCDEFGHJKMN", Name: "In Scope"})
			svc := NewService(repo).WithGeographicBoundaryConfig(tt.boundary)

			result, err := svc.List(ctx, tt.filters, Pagination{Limit: 10})
			require.NoError(t, err)
			require.Len(t, result.Events, tt.wantLen)
		})
	}
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

// ─── srv-of0g3: search alias ──────────────────────────────────────────────────

func TestParseFilters_SearchAlias(t *testing.T) {
	t.Run("search alias alone produces query and warning", func(t *testing.T) {
		values := url.Values{}
		values.Set("start_date", "2026-06-01")
		values.Set("search", "foo")

		filters, _, warnings, err := ParseFilters(values, nil)

		require.NoError(t, err)
		require.Equal(t, "foo", filters.Query)
		require.Len(t, warnings, 2) // one for start_date, one for search
		var found bool
		for _, w := range warnings {
			if strings.Contains(w, "search") && strings.Contains(w, "q") {
				found = true
			}
		}
		require.True(t, found, "expected warning about search alias")
	})

	t.Run("canonical q wins over search alias", func(t *testing.T) {
		values := url.Values{}
		values.Set("startDate", "2026-06-01")
		values.Set("q", "foo")
		values.Set("search", "bar")

		filters, _, warnings, err := ParseFilters(values, nil)

		require.NoError(t, err)
		require.Equal(t, "foo", filters.Query)
		for _, w := range warnings {
			require.NotContains(t, w, "search", "no search warning when canonical q is present")
		}
	})

	t.Run("canonical q alone produces no warning", func(t *testing.T) {
		values := url.Values{}
		values.Set("startDate", "2026-06-01")
		values.Set("q", "foo")

		filters, _, warnings, err := ParseFilters(values, nil)

		require.NoError(t, err)
		require.Equal(t, "foo", filters.Query)
		for _, w := range warnings {
			require.NotContains(t, w, "search")
			require.NotContains(t, w, "q")
		}
	})
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
