package places

import (
	"errors"
	"net/url"
	"testing"

	"github.com/Togather-Foundation/server/internal/domain/ids"
	"github.com/stretchr/testify/require"
)

func TestParseFiltersDefaults(t *testing.T) {
	filters, pagination, err := ParseFilters(url.Values{})

	require.NoError(t, err)
	require.Equal(t, 50, pagination.Limit)
	require.Empty(t, pagination.After)
	require.Empty(t, filters.City)
	require.Empty(t, filters.Query)
}

func TestParseFiltersTrimsFields(t *testing.T) {
	values := url.Values{}
	values.Set("city", "  Austin ")
	values.Set("q", "  live music ")
	values.Set("after", "  cursor ")

	filters, pagination, err := ParseFilters(values)

	require.NoError(t, err)
	require.Equal(t, "Austin", filters.City)
	require.Equal(t, "live music", filters.Query)
	require.Equal(t, "cursor", pagination.After)
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

func TestValidateULID(t *testing.T) {
	valid, err := ids.NewULID()
	require.NoError(t, err)

	require.NoError(t, ValidateULID(valid))
	require.ErrorIs(t, ValidateULID("not-a-ulid"), ids.ErrInvalidULID)
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
