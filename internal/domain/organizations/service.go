package organizations

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	paginationpkg "github.com/Togather-Foundation/server/internal/api/pagination"
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

func (s *Service) GetByULID(ctx context.Context, ulid string) (*Organization, error) {
	return s.repo.GetByULID(ctx, ulid)
}

func (s *Service) Update(ctx context.Context, ulid string, params UpdateOrganizationParams) (*Organization, error) {
	return s.repo.Update(ctx, ulid, params)
}

// TODO(srv-d7cnu): Create removed during rebase
// func (s *Service) Create(ctx context.Context, params CreateParams) (*Organization, error) {
// 	return s.repo.Create(ctx, params)
// }

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

	filters.Query = strings.TrimSpace(values.Get("q"))
	filters.City = strings.TrimSpace(values.Get("city"))

	// Parse sort parameter (default: created_at)
	sort := strings.ToLower(strings.TrimSpace(values.Get("sort")))
	switch sort {
	case "name", "created_at":
		filters.Sort = sort
	case "":
		filters.Sort = "created_at"
	default:
		return filters, pagination, FilterError{Field: "sort", Message: "must be 'name' or 'created_at'"}
	}

	// Parse order parameter (default: asc)
	order := strings.ToLower(strings.TrimSpace(values.Get("order")))
	switch order {
	case "asc", "desc":
		filters.Order = order
	case "":
		filters.Order = "asc"
	default:
		return filters, pagination, FilterError{Field: "order", Message: "must be 'asc' or 'desc'"}
	}

	limit, err := parseLimit(values)
	if err != nil {
		return filters, pagination, err
	}
	pagination.Limit = limit

	after := strings.TrimSpace(values.Get("after"))
	if after != "" {
		// Validate cursor format by attempting to decode it
		_, err := paginationpkg.DecodeEventCursor(after)
		if err != nil {
			return filters, pagination, FilterError{Field: "after", Message: "must be a valid cursor"}
		}
	}
	pagination.After = after

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

func ValidateULID(ulid string) error {
	return ids.ValidateULID(ulid)
}
