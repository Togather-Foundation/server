package events

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	paginationpkg "github.com/Togather-Foundation/server/internal/api/pagination"
	"github.com/Togather-Foundation/server/internal/domain"
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

func ParseFilters(values url.Values, loc *time.Location) (Filters, Pagination, []string, error) {
	if loc == nil {
		loc = time.UTC
	}

	filters := Filters{}
	pagination := Pagination{Limit: 50}
	var warnings []string

	// Resolve snake_case aliases before parsing.
	startDateRaw := domain.ResolveAlias(values, "startDate", "start_date", &warnings)
	endDateRaw := domain.ResolveAlias(values, "endDate", "end_date", &warnings)
	venueIDRaw := domain.ResolveAlias(values, "venueId", "venue_id", &warnings)
	organizerIDRaw := domain.ResolveAlias(values, "organizerId", "organizer_id", &warnings)
	stateRaw := domain.ResolveAlias(values, "state", "lifecycle_state", &warnings)
	domainRaw := domain.ResolveAlias(values, "domain", "event_domain", &warnings)

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
	// endDate is inclusive of the entire endDate day (events starting any time
	// on that day). Advance to next-day midnight so the SQL bound
	// (start_time <= endDate) covers the whole day, not just the 00:00:00 instant.
	if endDate != nil {
		advanced := endDate.AddDate(0, 0, 1)
		endDate = &advanced
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

	q := domain.ResolveAlias(values, "q", "search", &warnings)
	filters.Query = strings.TrimSpace(q)

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

	appendUnknownParamWarnings(values, &warnings)

	return filters, pagination, warnings, nil
}

// knownFilterParams is the complete set of query parameters the events list
// endpoint accepts, including snake_case aliases. Any other parameter is
// silently ignored by the API; surface that so clients don't trust results
// that were never filtered by their intended param (e.g. a guessed geo param).
var knownFilterParams = map[string]bool{
	"startDate": true, "start_date": true,
	"endDate": true, "end_date": true,
	"venueId": true, "venue_id": true,
	"organizerId": true, "organizer_id": true,
	"state": true, "lifecycle_state": true,
	"domain": true, "event_domain": true,
	"city": true, "region": true,
	"q": true, "search": true,
	"keywords": true, "limit": true, "after": true,
}

// appendUnknownParamWarnings adds a warning for every query parameter that is
// not recognised by the events list endpoint. Recognised parameters are still
// processed as before; this only surfaces typos and unsupported params (like
// proximity search) instead of silently dropping them.
func appendUnknownParamWarnings(values url.Values, warnings *[]string) {
	for key := range values {
		if knownFilterParams[key] {
			continue
		}
		*warnings = append(*warnings,
			fmt.Sprintf("Unrecognised query parameter %q — it was ignored. See /api/v1/openapi.json for supported parameters.", key))
	}
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
