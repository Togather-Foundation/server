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

func (s *Service) Create(ctx context.Context, params CreateParams) (*Place, error) {
	return s.repo.Create(ctx, params)
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

	if nearLat, nearLon, radius, ok, err := parseGeoFilters(values); err != nil {
		return filters, pagination, err
	} else if ok {
		filters.NearLat = &nearLat
		filters.NearLon = &nearLon
		filters.RadiusMeters = radius
	}

	limit, err := parseLimit(values)
	if err != nil {
		return filters, pagination, err
	}
	pagination.Limit = limit

	pagination.After = strings.TrimSpace(values.Get("after"))

	return filters, pagination, nil
}

func parseGeoFilters(values url.Values) (float64, float64, float64, bool, error) {
	latValue := strings.TrimSpace(values.Get("near_lat"))
	lonValue := strings.TrimSpace(values.Get("near_lon"))
	radiusValue := strings.TrimSpace(values.Get("radius"))
	if latValue == "" && lonValue == "" && radiusValue == "" {
		return 0, 0, 0, false, nil
	}
	if latValue == "" || lonValue == "" || radiusValue == "" {
		return 0, 0, 0, false, FilterError{Field: "near", Message: "near_lat, near_lon, and radius are required together"}
	}
	lat, err := strconv.ParseFloat(latValue, 64)
	if err != nil {
		return 0, 0, 0, false, FilterError{Field: "near_lat", Message: "must be a number"}
	}
	if lat < -90 || lat > 90 {
		return 0, 0, 0, false, FilterError{Field: "near_lat", Message: "must be between -90 and 90"}
	}
	lon, err := strconv.ParseFloat(lonValue, 64)
	if err != nil {
		return 0, 0, 0, false, FilterError{Field: "near_lon", Message: "must be a number"}
	}
	if lon < -180 || lon > 180 {
		return 0, 0, 0, false, FilterError{Field: "near_lon", Message: "must be between -180 and 180"}
	}
	radius, err := strconv.ParseFloat(radiusValue, 64)
	if err != nil {
		return 0, 0, 0, false, FilterError{Field: "radius", Message: "must be a number"}
	}
	if radius <= 0 {
		return 0, 0, 0, false, FilterError{Field: "radius", Message: "must be greater than 0"}
	}

	return lat, lon, radius, true, nil
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

func ValidateULID(ulid string) error {
	return ids.ValidateULID(ulid)
}
