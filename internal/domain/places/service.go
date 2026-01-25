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

func ValidateULID(ulid string) error {
	return ids.ValidateULID(ulid)
}
