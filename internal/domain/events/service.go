package events

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

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

// resolveAlias checks if a snake_case alias is present when the canonical camelCase param is absent.
// If so, it appends a warning and returns the alias value.
func resolveAlias(values url.Values, canonical, alias string, warnings *[]string) string {
	if values.Get(canonical) != "" {
		return values.Get(canonical)
	}
	if v := values.Get(alias); v != "" {
		*warnings = append(*warnings, fmt.Sprintf("Unrecognised parameter alias %q — use %q instead", alias, canonical))
		return v
	}
	return ""
}

func ParseFilters(values url.Values, loc *time.Location) (Filters, Pagination, []string, error) {
	if loc == nil {
		loc = time.UTC
	}

	filters := Filters{}
	pagination := Pagination{Limit: 50}
	var warnings []string

	// Resolve snake_case aliases before parsing.
	startDateRaw := resolveAlias(values, "startDate", "start_date", &warnings)
	endDateRaw := resolveAlias(values, "endDate", "end_date", &warnings)
	venueIDRaw := resolveAlias(values, "venueId", "venue_id", &warnings)
	organizerIDRaw := resolveAlias(values, "organizerId", "organizer_id", &warnings)
	stateRaw := resolveAlias(values, "state", "lifecycle_state", &warnings)
	domainRaw := resolveAlias(values, "domain", "event_domain", &warnings)

	startDate, err := parseDate("startDate", startDateRaw, loc)
	if err != nil {
		return filters, pagination, nil, err
	}
	endDate, err := parseDate("endDate", endDateRaw, loc)
	if err != nil {
		return filters, pagination, nil, err
	}
	if startDate != nil && endDate != nil && endDate.Before(*startDate) {
		return filters, pagination, nil, FilterError{Field: "endDate", Message: "must be on or after startDate"}
	}
	filters.StartDate = startDate
	filters.EndDate = endDate

	// Apply default: if caller provided no date constraint at all, default to startDate=today
	// so that past events are excluded unless explicitly requested.
	if filters.StartDate == nil && filters.EndDate == nil {
		now := time.Now().In(loc)
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
		filters.StartDate = &today
	}

	filters.City = strings.TrimSpace(values.Get("city"))
	filters.Region = strings.TrimSpace(values.Get("region"))

	filters.VenueULID = strings.TrimSpace(venueIDRaw)
	if filters.VenueULID != "" {
		if err := ids.ValidateULID(filters.VenueULID); err != nil {
			return filters, pagination, nil, FilterError{Field: "venueId", Message: "invalid ULID"}
		}
	}

	filters.OrganizerULID = strings.TrimSpace(organizerIDRaw)
	if filters.OrganizerULID != "" {
		if err := ids.ValidateULID(filters.OrganizerULID); err != nil {
			return filters, pagination, nil, FilterError{Field: "organizerId", Message: "invalid ULID"}
		}
	}

	filters.LifecycleState = parseLifecycleStateFromString(stateRaw)
	if filters.LifecycleState == "" {
		if err := parseLifecycleStateErrFromString(stateRaw); err != nil {
			return filters, pagination, nil, err
		}
	}

	filters.Query = strings.TrimSpace(values.Get("q"))

	filters.Domain = parseDomainFromString(domainRaw)
	if filters.Domain == "" {
		if err := parseDomainErrFromString(domainRaw); err != nil {
			return filters, pagination, nil, err
		}
	}

	filters.Keywords = parseKeywords(values.Get("keywords"))

	limit, err := parseLimit(values)
	if err != nil {
		return filters, pagination, nil, err
	}
	pagination.Limit = limit

	after := strings.TrimSpace(values.Get("after"))
	if after != "" {
		// Validate cursor format by attempting to decode it
		_, err := paginationpkg.DecodeEventCursor(after)
		if err != nil {
			return filters, pagination, nil, FilterError{Field: "after", Message: "must be a valid cursor"}
		}
	}
	pagination.After = after

	return filters, pagination, warnings, nil
}

func parseDate(field string, value string, loc *time.Location) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	// Parse as a calendar date in the server's configured timezone so that
	// explicit date params (e.g. startDate=2026-01-01) are consistent with the
	// default startDate=today, both of which resolve to midnight in loc.
	parsed, err := time.ParseInLocation("2006-01-02", value, loc)
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

func parseLifecycleStateFromString(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if isAllowedLifecycleState(value) {
		return value
	}
	return ""
}

func parseLifecycleStateErrFromString(raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}
	if !isAllowedLifecycleState(strings.ToLower(value)) {
		return FilterError{Field: "state", Message: "unsupported lifecycle state"}
	}
	return nil
}

func parseDomainFromString(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if isAllowedDomain(value) {
		return value
	}
	return ""
}

func parseDomainErrFromString(raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}
	if !isAllowedDomain(strings.ToLower(value)) {
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
