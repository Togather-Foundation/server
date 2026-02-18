package events

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseFiltersDefaults(t *testing.T) {
	filters, pagination, err := ParseFilters(url.Values{})

	require.NoError(t, err)
	require.Equal(t, 50, pagination.Limit)
	require.Empty(t, pagination.After)
	require.Nil(t, filters.StartDate)
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
	values := url.Values{}
	values.Set("city", "  Portland  ")
	values.Set("region", "  OR ")
	values.Set("q", "  jazz night ")
	values.Set("after", "  01HQZX3Y4K6F7G8H9J0K1M2N3P ")

	filters, pagination, err := ParseFilters(values)

	require.NoError(t, err)
	require.Equal(t, "Portland", filters.City)
	require.Equal(t, "OR", filters.Region)
	require.Equal(t, "jazz night", filters.Query)
	require.Equal(t, "01HQZX3Y4K6F7G8H9J0K1M2N3P", pagination.After)
}

func TestParseFiltersDateValidation(t *testing.T) {
	values := url.Values{}
	values.Set("startDate", "2024-01-02")
	values.Set("endDate", "2024-01-01")

	_, _, err := ParseFilters(values)

	assertFilterError(t, err, "endDate", "must be on or after startDate")
}

func TestParseFiltersDateFormat(t *testing.T) {
	values := url.Values{}
	values.Set("startDate", "01-02-2024")

	_, _, err := ParseFilters(values)

	assertFilterError(t, err, "startDate", "must be ISO8601 date")
}

func TestParseFiltersDateSuccess(t *testing.T) {
	values := url.Values{}
	values.Set("startDate", "2024-01-01")
	values.Set("endDate", "2024-01-02")

	filters, _, err := ParseFilters(values)

	require.NoError(t, err)
	require.NotNil(t, filters.StartDate)
	require.NotNil(t, filters.EndDate)
	require.Equal(t, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), *filters.StartDate)
	require.Equal(t, time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), *filters.EndDate)
}

func TestParseFiltersVenueULIDValidation(t *testing.T) {
	values := url.Values{}
	values.Set("venueId", "not-a-ulid")

	_, _, err := ParseFilters(values)

	assertFilterError(t, err, "venueId", "invalid ULID")
}

func TestParseFiltersOrganizerULIDValidation(t *testing.T) {
	values := url.Values{}
	values.Set("organizerId", "not-a-ulid")

	_, _, err := ParseFilters(values)

	assertFilterError(t, err, "organizerId", "invalid ULID")
}

func TestParseFiltersLifecycleStateValidation(t *testing.T) {
	values := url.Values{}
	values.Set("state", "PUBLISHED")

	filters, _, err := ParseFilters(values)

	require.NoError(t, err)
	require.Equal(t, "published", filters.LifecycleState)

	values.Set("state", "unknown")

	_, _, err = ParseFilters(values)

	assertFilterError(t, err, "state", "unsupported lifecycle state")
}

func TestParseFiltersDomainValidation(t *testing.T) {
	values := url.Values{}
	values.Set("domain", "Arts")

	filters, _, err := ParseFilters(values)

	require.NoError(t, err)
	require.Equal(t, "arts", filters.Domain)

	values.Set("domain", "invalid")

	_, _, err = ParseFilters(values)

	assertFilterError(t, err, "domain", "unsupported event domain")
}

func TestParseFiltersKeywords(t *testing.T) {
	values := url.Values{}
	values.Set("keywords", " jazz, , blues ,rock ")

	filters, _, err := ParseFilters(values)

	require.NoError(t, err)
	require.Equal(t, []string{"jazz", "blues", "rock"}, filters.Keywords)
}

func TestParseFiltersLimitValidation(t *testing.T) {
	values := url.Values{}
	values.Set("limit", "abc")

	_, _, err := ParseFilters(values)

	assertFilterError(t, err, "limit", "must be a number")

	values.Set("limit", "0")

	_, _, err = ParseFilters(values)

	assertFilterError(t, err, "limit", "must be between 1 and 200")
}

func TestParseFiltersLimitSuccess(t *testing.T) {
	values := url.Values{}
	values.Set("limit", "200")

	_, pagination, err := ParseFilters(values)

	require.NoError(t, err)
	require.Equal(t, 200, pagination.Limit)
}

func TestParseFiltersAfterCursorValidation(t *testing.T) {
	t.Run("valid ULID cursor", func(t *testing.T) {
		values := url.Values{}
		values.Set("after", "01HQZX3Y4K6F7G8H9J0K1M2N3P")

		_, pagination, err := ParseFilters(values)

		require.NoError(t, err)
		require.Equal(t, "01HQZX3Y4K6F7G8H9J0K1M2N3P", pagination.After)
	})

	t.Run("empty cursor is valid", func(t *testing.T) {
		values := url.Values{}
		values.Set("after", "")

		_, pagination, err := ParseFilters(values)

		require.NoError(t, err)
		require.Empty(t, pagination.After)
	})

	t.Run("whitespace-only cursor is treated as empty", func(t *testing.T) {
		values := url.Values{}
		values.Set("after", "   ")

		_, pagination, err := ParseFilters(values)

		require.NoError(t, err)
		require.Empty(t, pagination.After)
	})

	t.Run("invalid cursor - RFC3339 timestamp", func(t *testing.T) {
		values := url.Values{}
		values.Set("after", "2026-01-01T00:00:00Z")

		_, _, err := ParseFilters(values)

		assertFilterError(t, err, "after", "invalid ULID")
	})

	t.Run("invalid cursor - arbitrary string", func(t *testing.T) {
		values := url.Values{}
		values.Set("after", "not-a-valid-ulid")

		_, _, err := ParseFilters(values)

		assertFilterError(t, err, "after", "invalid ULID")
	})

	t.Run("invalid cursor - too short", func(t *testing.T) {
		values := url.Values{}
		values.Set("after", "123")

		_, _, err := ParseFilters(values)

		assertFilterError(t, err, "after", "invalid ULID")
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
