package organizations

import (
	"errors"
	"net/url"
	"testing"
	"time"

	paginationpkg "github.com/Togather-Foundation/server/internal/api/pagination"
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
	validCursor := paginationpkg.EncodeEventCursor(time.Unix(1706886000, 0), "01HYX3KQW7ERTV9XNBM2P8QJZF")

	values := url.Values{}
	values.Set("q", "  local org ")
	values.Set("after", "  "+validCursor+" ")

	filters, pagination, err := ParseFilters(values)

	require.NoError(t, err)
	require.Equal(t, "local org", filters.Query)
	require.Equal(t, validCursor, pagination.After)
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
	t.Run("valid cursor", func(t *testing.T) {
		validCursor := paginationpkg.EncodeEventCursor(time.Unix(1706886000, 0), "01HYX3KQW7ERTV9XNBM2P8QJZF")
		values := url.Values{}
		values.Set("after", validCursor)

		_, pagination, err := ParseFilters(values)

		require.NoError(t, err)
		require.Equal(t, validCursor, pagination.After)
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

		assertFilterError(t, err, "after", "must be a valid cursor")
	})

	t.Run("invalid cursor - arbitrary string", func(t *testing.T) {
		values := url.Values{}
		values.Set("after", "not-a-valid-cursor")

		_, _, err := ParseFilters(values)

		assertFilterError(t, err, "after", "must be a valid cursor")
	})

	t.Run("invalid cursor - raw ULID", func(t *testing.T) {
		values := url.Values{}
		values.Set("after", "01HYX3KQW7ERTV9XNBM2P8QJZF")

		_, _, err := ParseFilters(values)

		assertFilterError(t, err, "after", "must be a valid cursor")
	})

	t.Run("invalid cursor - too short", func(t *testing.T) {
		values := url.Values{}
		values.Set("after", "123")

		_, _, err := ParseFilters(values)

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
