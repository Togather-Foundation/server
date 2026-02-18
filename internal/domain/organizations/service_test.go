package organizations

import (
	"errors"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseFiltersDefaults(t *testing.T) {
	filters, pagination, err := ParseFilters(url.Values{})

	require.NoError(t, err)
	require.Equal(t, 50, pagination.Limit)
	require.Empty(t, pagination.After)
	require.Empty(t, filters.Query)
}

func TestParseFiltersTrimsFields(t *testing.T) {
	values := url.Values{}
	values.Set("q", "  local org ")
	values.Set("after", "  01HQZX3Y4K6F7G8H9J0K1M2N3P ")

	filters, pagination, err := ParseFilters(values)

	require.NoError(t, err)
	require.Equal(t, "local org", filters.Query)
	require.Equal(t, "01HQZX3Y4K6F7G8H9J0K1M2N3P", pagination.After)
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

		assertFilterError(t, err, "after", "must be a valid ULID (e.g., 01HQZX3Y4K6F7G8H9J0K1M2N3P)")
	})

	t.Run("invalid cursor - arbitrary string", func(t *testing.T) {
		values := url.Values{}
		values.Set("after", "not-a-valid-ulid")

		_, _, err := ParseFilters(values)

		assertFilterError(t, err, "after", "must be a valid ULID (e.g., 01HQZX3Y4K6F7G8H9J0K1M2N3P)")
	})

	t.Run("invalid cursor - too short", func(t *testing.T) {
		values := url.Values{}
		values.Set("after", "123")

		_, _, err := ParseFilters(values)

		assertFilterError(t, err, "after", "must be a valid ULID (e.g., 01HQZX3Y4K6F7G8H9J0K1M2N3P)")
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
