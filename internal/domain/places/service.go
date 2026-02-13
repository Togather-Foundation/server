package places

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/Togather-Foundation/server/internal/domain/ids"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) List(ctx context.Context, filters Filters, pagination Pagination) (ListResult, error) {
	return s.repo.List(ctx, filters, pagination)
}

func (s *Service) GetByULID(ctx context.Context, ulid string) (*Place, error) {
	return s.repo.GetByULID(ctx, ulid)
}

func (s *Service) SoftDelete(ctx context.Context, ulid string, reason string) error {
	return s.repo.SoftDelete(ctx, ulid, reason)
}

func (s *Service) CreateTombstone(ctx context.Context, params TombstoneCreateParams) error {
	return s.repo.CreateTombstone(ctx, params)
}

func (s *Service) GetTombstoneByULID(ctx context.Context, ulid string) (*Tombstone, error) {
	return s.repo.GetTombstoneByULID(ctx, ulid)
}

type FilterError struct {
	Field   string
	Message string
}

func (e FilterError) Error() string {
	if e.Field == "" {
		return e.Message
	}
	return fmt.Sprintf("invalid %s: %s", e.Field, e.Message)
}

func ParseFilters(values url.Values) (Filters, Pagination, error) {
	filters := Filters{}
	pagination := Pagination{Limit: 50}

	filters.City = strings.TrimSpace(values.Get("city"))
	filters.Query = strings.TrimSpace(values.Get("q"))

	// Parse near_place parameter
	nearPlace := strings.TrimSpace(values.Get("near_place"))
	if nearPlace != "" {
		filters.NearPlace = &nearPlace
	}

	// Parse proximity parameters
	lat, lon, radius, err := parseProximityParams(values)
	if err != nil {
		return filters, pagination, err
	}
	filters.Latitude = lat
	filters.Longitude = lon
	filters.RadiusKm = radius

	// Validate that near_place and near_lat/near_lon are mutually exclusive
	if filters.NearPlace != nil && (filters.Latitude != nil || filters.Longitude != nil) {
		return filters, pagination, FilterError{
			Field:   "near_place,near_lat,near_lon",
			Message: "cannot use both near_place and near_lat/near_lon - choose one proximity method",
		}
	}

	limit, err := parseLimit(values)
	if err != nil {
		return filters, pagination, err
	}
	pagination.Limit = limit

	pagination.After = strings.TrimSpace(values.Get("after"))

	return filters, pagination, nil
}

func parseLimit(values url.Values) (int, error) {
	limit := 50
	rawLimit := strings.TrimSpace(values.Get("limit"))
	if rawLimit == "" {
		return limit, nil
	}
	parsed, err := strconv.Atoi(rawLimit)
	if err != nil {
		return 0, FilterError{Field: "limit", Message: "must be a number"}
	}
	if parsed < 1 || parsed > 200 {
		return 0, FilterError{Field: "limit", Message: "must be between 1 and 200"}
	}
	return parsed, nil
}

func parseProximityParams(values url.Values) (*float64, *float64, *float64, error) {
	rawLat := strings.TrimSpace(values.Get("near_lat"))
	rawLon := strings.TrimSpace(values.Get("near_lon"))
	rawRadius := strings.TrimSpace(values.Get("radius"))

	// If none provided, no proximity search
	if rawLat == "" && rawLon == "" && rawRadius == "" {
		return nil, nil, nil, nil
	}

	// If any lat/lon provided, both must be provided
	if (rawLat == "" && rawLon != "") || (rawLat != "" && rawLon == "") {
		return nil, nil, nil, FilterError{
			Field:   "near_lat,near_lon",
			Message: "both near_lat and near_lon must be provided for proximity search",
		}
	}

	// Parse latitude
	lat, err := strconv.ParseFloat(rawLat, 64)
	if err != nil {
		return nil, nil, nil, FilterError{Field: "near_lat", Message: "must be a valid number"}
	}
	if lat < -90 || lat > 90 {
		return nil, nil, nil, FilterError{Field: "near_lat", Message: "must be between -90 and 90"}
	}

	// Parse longitude
	lon, err := strconv.ParseFloat(rawLon, 64)
	if err != nil {
		return nil, nil, nil, FilterError{Field: "near_lon", Message: "must be a valid number"}
	}
	if lon < -180 || lon > 180 {
		return nil, nil, nil, FilterError{Field: "near_lon", Message: "must be between -180 and 180"}
	}

	// Parse radius (default 10km if not provided)
	radius := 10.0
	if rawRadius != "" {
		parsed, err := strconv.ParseFloat(rawRadius, 64)
		if err != nil {
			return nil, nil, nil, FilterError{Field: "radius", Message: "must be a valid number"}
		}
		if parsed <= 0 {
			return nil, nil, nil, FilterError{Field: "radius", Message: "must be greater than 0"}
		}
		if parsed > 100 {
			return nil, nil, nil, FilterError{Field: "radius", Message: "must be 100km or less"}
		}
		radius = parsed
	}

	return &lat, &lon, &radius, nil
}

func ValidateULID(ulid string) error {
	return ids.ValidateULID(ulid)
}
