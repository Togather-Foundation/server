package events

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

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

func (s *Service) GetByULID(ctx context.Context, ulid string) (*Event, error) {
	return s.repo.GetByULID(ctx, ulid)
}

func (s *Service) GetTombstoneByEventULID(ctx context.Context, eventULID string) (*Tombstone, error) {
	return s.repo.GetTombstoneByEventULID(ctx, eventULID)
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

	startDate, err := parseDate("startDate", values.Get("startDate"))
	if err != nil {
		return filters, pagination, err
	}
	endDate, err := parseDate("endDate", values.Get("endDate"))
	if err != nil {
		return filters, pagination, err
	}
	if startDate != nil && endDate != nil && endDate.Before(*startDate) {
		return filters, pagination, FilterError{Field: "endDate", Message: "must be on or after startDate"}
	}
	filters.StartDate = startDate
	filters.EndDate = endDate

	filters.City = strings.TrimSpace(values.Get("city"))
	filters.Region = strings.TrimSpace(values.Get("region"))

	filters.VenueULID = strings.TrimSpace(values.Get("venueId"))
	if filters.VenueULID != "" {
		if err := ids.ValidateULID(filters.VenueULID); err != nil {
			return filters, pagination, FilterError{Field: "venueId", Message: "invalid ULID"}
		}
	}

	filters.OrganizerULID = strings.TrimSpace(values.Get("organizerId"))
	if filters.OrganizerULID != "" {
		if err := ids.ValidateULID(filters.OrganizerULID); err != nil {
			return filters, pagination, FilterError{Field: "organizerId", Message: "invalid ULID"}
		}
	}

	filters.LifecycleState = parseLifecycleState(values)
	if filters.LifecycleState == "" {
		if err := parseLifecycleStateErr(values); err != nil {
			return filters, pagination, err
		}
	}

	filters.Query = strings.TrimSpace(values.Get("q"))

	filters.Domain = parseDomain(values)
	if filters.Domain == "" {
		if err := parseDomainErr(values); err != nil {
			return filters, pagination, err
		}
	}

	filters.Keywords = parseKeywords(values.Get("keywords"))

	limit, err := parseLimit(values)
	if err != nil {
		return filters, pagination, err
	}
	pagination.Limit = limit

	after := strings.TrimSpace(values.Get("after"))
	if after != "" {
		if err := ids.ValidateULID(after); err != nil {
			return filters, pagination, FilterError{Field: "after", Message: "must be a valid ULID (e.g., 01HQZX3Y4K6F7G8H9J0K1M2N3P)"}
		}
	}
	pagination.After = after

	return filters, pagination, nil
}

func parseDate(field string, value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil, FilterError{Field: field, Message: "must be ISO8601 date"}
	}
	return &parsed, nil
}

func parseKeywords(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	keywords := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item != "" {
			keywords = append(keywords, item)
		}
	}
	return keywords
}

func parseLifecycleState(values url.Values) string {
	value := strings.TrimSpace(values.Get("state"))
	if value == "" {
		return ""
	}
	value = strings.ToLower(value)
	if isAllowedLifecycleState(value) {
		return value
	}
	return ""
}

func parseLifecycleStateErr(values url.Values) error {
	value := strings.TrimSpace(values.Get("state"))
	if value == "" {
		return nil
	}
	value = strings.ToLower(value)
	if !isAllowedLifecycleState(value) {
		return FilterError{Field: "state", Message: "unsupported lifecycle state"}
	}
	return nil
}

func parseDomain(values url.Values) string {
	value := strings.TrimSpace(values.Get("domain"))
	if value == "" {
		return ""
	}
	value = strings.ToLower(value)
	if isAllowedDomain(value) {
		return value
	}
	return ""
}

func parseDomainErr(values url.Values) error {
	value := strings.TrimSpace(values.Get("domain"))
	if value == "" {
		return nil
	}
	value = strings.ToLower(value)
	if !isAllowedDomain(value) {
		return FilterError{Field: "domain", Message: "unsupported event domain"}
	}
	return nil
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

func isAllowedLifecycleState(value string) bool {
	switch value {
	case "draft", "published", "postponed", "rescheduled", "sold_out", "cancelled", "completed":
		return true
	default:
		return false
	}
}

func isAllowedDomain(value string) bool {
	switch value {
	case "arts", "music", "culture", "sports", "community", "education", "general":
		return true
	default:
		return false
	}
}
