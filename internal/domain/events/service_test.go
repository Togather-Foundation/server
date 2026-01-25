package events

import (
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
	values.Set("after", "  cursor ")

	filters, pagination, err := ParseFilters(values)

	require.NoError(t, err)
	require.Equal(t, "Portland", filters.City)
	require.Equal(t, "OR", filters.Region)
	require.Equal(t, "jazz night", filters.Query)
	require.Equal(t, "cursor", pagination.After)
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
