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

func TestParseFiltersProximityValid(t *testing.T) {
	values := url.Values{}
	values.Set("near_lat", "43.6532")
	values.Set("near_lon", "-79.3832")
	values.Set("radius", "5")

	filters, _, err := ParseFilters(values)

	require.NoError(t, err)
	require.NotNil(t, filters.Latitude)
	require.NotNil(t, filters.Longitude)
	require.NotNil(t, filters.RadiusKm)
	require.Equal(t, 43.6532, *filters.Latitude)
	require.Equal(t, -79.3832, *filters.Longitude)
	require.Equal(t, 5.0, *filters.RadiusKm)
}

func TestParseFiltersProximityDefaultRadius(t *testing.T) {
	values := url.Values{}
	values.Set("near_lat", "43.6532")
	values.Set("near_lon", "-79.3832")

	filters, _, err := ParseFilters(values)

	require.NoError(t, err)
	require.NotNil(t, filters.RadiusKm)
	require.Equal(t, 10.0, *filters.RadiusKm) // Default is 10km
}

func TestParseFiltersProximityLatOutOfRange(t *testing.T) {
	values := url.Values{}
	values.Set("near_lat", "91")
	values.Set("near_lon", "-79.3832")

	_, _, err := ParseFilters(values)

	assertFilterError(t, err, "near_lat", "must be between -90 and 90")

	values.Set("near_lat", "-91")
	_, _, err = ParseFilters(values)

	assertFilterError(t, err, "near_lat", "must be between -90 and 90")
}

func TestParseFiltersProximityLonOutOfRange(t *testing.T) {
	values := url.Values{}
	values.Set("near_lat", "43.6532")
	values.Set("near_lon", "181")

	_, _, err := ParseFilters(values)

	assertFilterError(t, err, "near_lon", "must be between -180 and 180")

	values.Set("near_lon", "-181")
	_, _, err = ParseFilters(values)

	assertFilterError(t, err, "near_lon", "must be between -180 and 180")
}

func TestParseFiltersProximityRadiusTooLarge(t *testing.T) {
	values := url.Values{}
	values.Set("near_lat", "43.6532")
	values.Set("near_lon", "-79.3832")
	values.Set("radius", "101")

	_, _, err := ParseFilters(values)

	assertFilterError(t, err, "radius", "must be 100km or less")
}

func TestParseFiltersProximityRadiusZero(t *testing.T) {
	values := url.Values{}
	values.Set("near_lat", "43.6532")
	values.Set("near_lon", "-79.3832")
	values.Set("radius", "0")

	_, _, err := ParseFilters(values)

	assertFilterError(t, err, "radius", "must be greater than 0")
}

func TestParseFiltersProximityLatWithoutLon(t *testing.T) {
	values := url.Values{}
	values.Set("near_lat", "43.6532")

	_, _, err := ParseFilters(values)

	assertFilterError(t, err, "near_lat,near_lon", "both near_lat and near_lon must be provided for proximity search")
}

func TestParseFiltersProximityLonWithoutLat(t *testing.T) {
	values := url.Values{}
	values.Set("near_lon", "-79.3832")

	_, _, err := ParseFilters(values)

	assertFilterError(t, err, "near_lat,near_lon", "both near_lat and near_lon must be provided for proximity search")
}

func TestParseFiltersProximityInvalidLatNumber(t *testing.T) {
	values := url.Values{}
	values.Set("near_lat", "not-a-number")
	values.Set("near_lon", "-79.3832")

	_, _, err := ParseFilters(values)

	assertFilterError(t, err, "near_lat", "must be a valid number")
}

func TestParseFiltersProximityInvalidLonNumber(t *testing.T) {
	values := url.Values{}
	values.Set("near_lat", "43.6532")
	values.Set("near_lon", "not-a-number")

	_, _, err := ParseFilters(values)

	assertFilterError(t, err, "near_lon", "must be a valid number")
}

func TestParseFiltersProximityInvalidRadiusNumber(t *testing.T) {
	values := url.Values{}
	values.Set("near_lat", "43.6532")
	values.Set("near_lon", "-79.3832")
	values.Set("radius", "not-a-number")

	_, _, err := ParseFilters(values)

	assertFilterError(t, err, "radius", "must be a valid number")
}

func TestParseFiltersNearPlaceWithRadiusOnly(t *testing.T) {
	// When near_place is used with radius but no lat/lon,
	// radius should be parsed and returned without error.
	values := url.Values{}
	values.Set("near_place", "Toronto City Hall")
	values.Set("radius", "5")

	filters, _, err := ParseFilters(values)

	require.NoError(t, err)
	require.NotNil(t, filters.NearPlace)
	require.Equal(t, "Toronto City Hall", *filters.NearPlace)
	require.Nil(t, filters.Latitude)
	require.Nil(t, filters.Longitude)
	require.NotNil(t, filters.RadiusKm)
	require.Equal(t, 5.0, *filters.RadiusKm)
}

func TestParseFiltersNearPlaceWithoutRadius(t *testing.T) {
	// When near_place is used without radius, no proximity params returned.
	values := url.Values{}
	values.Set("near_place", "Toronto City Hall")

	filters, _, err := ParseFilters(values)

	require.NoError(t, err)
	require.NotNil(t, filters.NearPlace)
	require.Nil(t, filters.Latitude)
	require.Nil(t, filters.Longitude)
	require.Nil(t, filters.RadiusKm)
}

func TestParseFiltersNearPlaceConflictsWithLatLon(t *testing.T) {
	values := url.Values{}
	values.Set("near_place", "Toronto City Hall")
	values.Set("near_lat", "43.6532")
	values.Set("near_lon", "-79.3832")

	_, _, err := ParseFilters(values)

	assertFilterError(t, err, "near_place,near_lat,near_lon", "cannot use both near_place and near_lat/near_lon - choose one proximity method")
}
