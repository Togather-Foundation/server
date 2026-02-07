package events

import (
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Togather-Foundation/server/internal/domain/ids"
)

const (
	maxNameLength        = 500
	maxDescriptionLength = 10000
)

type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	if e.Field == "" {
		return e.Message
	}
	return fmt.Sprintf("invalid %s: %s", e.Field, e.Message)
}

type ValidationWarning struct {
	Field   string
	Message string
	Code    string // Machine-readable code (e.g., "reversed_dates", "suspicious_duration")
}

type ValidationResult struct {
	Input    EventInput
	Warnings []ValidationWarning
}

type EventInput struct {
	ID                  string                `json:"@id,omitempty"`
	Type                string                `json:"@type,omitempty"`
	Name                string                `json:"name,omitempty"`
	Description         string                `json:"description,omitempty"`
	StartDate           string                `json:"startDate,omitempty"`
	EndDate             string                `json:"endDate,omitempty"`
	DoorTime            string                `json:"doorTime,omitempty"`
	Location            *PlaceInput           `json:"location,omitempty"`
	VirtualLocation     *VirtualLocationInput `json:"virtualLocation,omitempty"`
	Organizer           *OrganizationInput    `json:"organizer,omitempty"`
	Image               string                `json:"image,omitempty"`
	URL                 string                `json:"url,omitempty"`
	Keywords            []string              `json:"keywords,omitempty"`
	InLanguage          []string              `json:"inLanguage,omitempty"`
	IsAccessibleForFree *bool                 `json:"isAccessibleForFree,omitempty"`
	Offers              *OfferInput           `json:"offers,omitempty"`
	SameAs              []string              `json:"sameAs,omitempty"`
	License             string                `json:"license,omitempty"`
	Source              *SourceInput          `json:"source,omitempty"`
	Occurrences         []OccurrenceInput     `json:"occurrences,omitempty"`
}

type PlaceInput struct {
	ID              string  `json:"@id,omitempty"`
	Name            string  `json:"name,omitempty"`
	StreetAddress   string  `json:"streetAddress,omitempty"`
	AddressLocality string  `json:"addressLocality,omitempty"`
	AddressRegion   string  `json:"addressRegion,omitempty"`
	PostalCode      string  `json:"postalCode,omitempty"`
	AddressCountry  string  `json:"addressCountry,omitempty"`
	Latitude        float64 `json:"latitude,omitempty"`
	Longitude       float64 `json:"longitude,omitempty"`
}

type VirtualLocationInput struct {
	Type string `json:"@type,omitempty"`
	URL  string `json:"url,omitempty"`
	Name string `json:"name,omitempty"`
}

type OrganizationInput struct {
	ID   string `json:"@id,omitempty"`
	Name string `json:"name,omitempty"`
	URL  string `json:"url,omitempty"`
}

type OfferInput struct {
	URL           string `json:"url,omitempty"`
	Price         string `json:"price,omitempty"`
	PriceCurrency string `json:"priceCurrency,omitempty"`
}

type SourceInput struct {
	URL     string `json:"url,omitempty"`
	EventID string `json:"eventId,omitempty"`
	Name    string `json:"name,omitempty"`
	License string `json:"license,omitempty"`
}

type OccurrenceInput struct {
	StartDate  string `json:"startDate,omitempty"`
	EndDate    string `json:"endDate,omitempty"`
	Timezone   string `json:"timezone,omitempty"`
	DoorTime   string `json:"doorTime,omitempty"`
	VenueID    string `json:"venueId,omitempty"`
	VirtualURL string `json:"virtualUrl,omitempty"`
}

func ValidateEventInput(input EventInput, nodeDomain string) (EventInput, error) {
	result, err := ValidateEventInputWithWarnings(input, nodeDomain, nil)
	if err != nil {
		return input, err
	}
	return result.Input, nil
}

// ValidateEventInputWithWarnings validates event input and returns warnings for suspicious data
// that should trigger admin review rather than outright rejection.
//
// If original is provided (non-nil), it compares original dates with normalized dates to detect
// auto-corrections (like correctEndDateTimezoneError) and generates appropriate warnings.
func ValidateEventInputWithWarnings(input EventInput, nodeDomain string, original *EventInput) (*ValidationResult, error) {
	var warnings []ValidationWarning

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, ValidationError{Field: "name", Message: "required"}
	}
	if utf8.RuneCountInString(name) > maxNameLength {
		return nil, ValidationError{Field: "name", Message: "too long"}
	}

	if utf8.RuneCountInString(strings.TrimSpace(input.Description)) > maxDescriptionLength {
		return nil, ValidationError{Field: "description", Message: "too long"}
	}

	startTime, err := parseRFC3339("startDate", input.StartDate)
	if err != nil {
		return nil, err
	}

	endTime, err := parseRFC3339Optional("endDate", input.EndDate)
	if err != nil {
		return nil, err
	}

	// Detect if normalization auto-corrected reversed dates
	// This happens when original had reversed dates but normalized input has corrected dates
	if original != nil && original.EndDate != "" && input.EndDate != "" && original.EndDate != input.EndDate {
		// Parse original dates to check if they were reversed
		origStart, origStartErr := time.Parse(time.RFC3339, strings.TrimSpace(original.StartDate))
		origEnd, origEndErr := time.Parse(time.RFC3339, strings.TrimSpace(original.EndDate))

		if origStartErr == nil && origEndErr == nil && origEnd.Before(origStart) {
			// Original dates were reversed, normalization corrected them
			// Generate appropriate warning based on the pattern
			gap := origStart.Sub(origEnd)
			origEndHour := origEnd.Hour() // 0-23 in UTC

			// Check if this matches the timezone error pattern:
			// - Early morning end (0-4 AM)
			// - Corrected duration < 7h
			if origEndHour <= 4 && endTime != nil {
				correctedDuration := endTime.Sub(*startTime)
				if correctedDuration > 0 && correctedDuration < 7*time.Hour {
					// High-confidence timezone error correction
					warnings = append(warnings, ValidationWarning{
						Field:   "endDate",
						Message: fmt.Sprintf("endDate was %v before startDate (ending at %02d:00) - auto-corrected as likely timezone error", gap, origEndHour),
						Code:    "reversed_dates_timezone_likely",
					})
				} else {
					// Early morning but unreasonable duration - needs review
					warnings = append(warnings, ValidationWarning{
						Field:   "endDate",
						Message: fmt.Sprintf("endDate was %v before startDate - auto-corrected but needs review", gap),
						Code:    "reversed_dates_corrected_needs_review",
					})
				}
			} else {
				// Not early morning - needs review
				warnings = append(warnings, ValidationWarning{
					Field:   "endDate",
					Message: fmt.Sprintf("endDate was %v before startDate - auto-corrected but needs review", gap),
					Code:    "reversed_dates_corrected_needs_review",
				})
			}
		}
	}

	// Check for reversed dates - this triggers admin review instead of rejection
	if endTime != nil && endTime.Before(*startTime) {
		gap := startTime.Sub(*endTime)
		endHour := endTime.Hour() // 0-23 in UTC

		// Two categories: timezone_likely (auto-fixed) or needs review
		if endHour <= 4 {
			// Early morning end suggests overnight event
			// Check if corrected event (adding 24h) would be reasonable duration
			correctedEnd := endTime.Add(24 * time.Hour)
			correctedDuration := correctedEnd.Sub(*startTime)

			if correctedDuration > 0 && correctedDuration < 7*time.Hour {
				// Very likely a timezone error on an overnight event
				// (normalization should have fixed this, but if not, flag it)
				warnings = append(warnings, ValidationWarning{
					Field:   "endDate",
					Message: fmt.Sprintf("endDate is %v before startDate and ends at %02d:00 - likely timezone conversion error", gap, endHour),
					Code:    "reversed_dates_timezone_likely",
				})
			} else {
				// Early morning but corrected duration is unreasonable - needs review
				warnings = append(warnings, ValidationWarning{
					Field:   "endDate",
					Message: fmt.Sprintf("endDate is %v before startDate - needs review", gap),
					Code:    "reversed_dates",
				})
			}
		} else {
			// Not early morning - needs review
			warnings = append(warnings, ValidationWarning{
				Field:   "endDate",
				Message: fmt.Sprintf("endDate is %v before startDate - needs review", gap),
				Code:    "reversed_dates",
			})
		}
	}

	if _, err := parseRFC3339Optional("doorTime", input.DoorTime); err != nil {
		return nil, err
	}

	if input.Location == nil && input.VirtualLocation == nil {
		return nil, ValidationError{Field: "location", Message: "location or virtualLocation required"}
	}
	if input.Location != nil {
		if err := validatePlaceInput(*input.Location, nodeDomain); err != nil {
			return nil, err
		}
	}
	if input.VirtualLocation != nil {
		if err := validateVirtualLocationInput(*input.VirtualLocation); err != nil {
			return nil, err
		}
	}
	if input.Organizer != nil {
		if err := validateOrganizationInput(*input.Organizer, nodeDomain); err != nil {
			return nil, err
		}
	}

	if input.Image != "" {
		if err := validateURL(input.Image); err != nil {
			return nil, ValidationError{Field: "image", Message: "invalid URI"}
		}
	}
	if input.URL != "" {
		if err := validateURL(input.URL); err != nil {
			return nil, ValidationError{Field: "url", Message: "invalid URI"}
		}
	}

	if input.ID != "" {
		if err := validateCanonicalURI(nodeDomain, "events", input.ID); err != nil {
			return nil, ValidationError{Field: "@id", Message: "invalid canonical URI"}
		}
	}

	if input.License != "" && !isCC0License(input.License) {
		return nil, ValidationError{Field: "license", Message: "must be CC0"}
	}

	if len(input.SameAs) > 0 {
		normalized, err := normalizeSameAs(nodeDomain, "events", input.SameAs)
		if err != nil {
			return nil, fmt.Errorf("normalize sameAs: %w", err)
		}
		input.SameAs = normalized
	}

	if input.Source != nil && input.Source.URL != "" {
		if err := validateURL(input.Source.URL); err != nil {
			return nil, ValidationError{Field: "source.url", Message: "invalid URI"}
		}
		if strings.TrimSpace(input.Source.EventID) == "" {
			return nil, ValidationError{Field: "source.eventId", Message: "required"}
		}
	}

	if err := validateOccurrences(input, nodeDomain); err != nil {
		return nil, err
	}

	input.Name = name
	return &ValidationResult{Input: input, Warnings: warnings}, nil
}

func validateOccurrences(input EventInput, nodeDomain string) error {
	if len(input.Occurrences) == 0 {
		if input.StartDate == "" {
			return ValidationError{Field: "occurrences", Message: "required"}
		}
		return nil
	}

	for i, occ := range input.Occurrences {
		fieldPrefix := fmt.Sprintf("occurrences[%d]", i)
		startTime, err := parseRFC3339(fieldPrefix+".startDate", occ.StartDate)
		if err != nil {
			return fmt.Errorf("parse occurrence start date: %w", err)
		}
		endTime, err := parseRFC3339Optional(fieldPrefix+".endDate", occ.EndDate)
		if err != nil {
			return fmt.Errorf("parse occurrence end date: %w", err)
		}
		if endTime != nil && endTime.Before(*startTime) {
			return ValidationError{Field: fieldPrefix + ".endDate", Message: "must be on or after startDate"}
		}
		if _, err := parseRFC3339Optional(fieldPrefix+".doorTime", occ.DoorTime); err != nil {
			return fmt.Errorf("parse occurrence door time: %w", err)
		}
		if occ.Timezone != "" {
			if _, err := time.LoadLocation(strings.TrimSpace(occ.Timezone)); err != nil {
				return ValidationError{Field: fieldPrefix + ".timezone", Message: "invalid timezone"}
			}
		}
		if occ.VirtualURL != "" {
			if err := validateURL(occ.VirtualURL); err != nil {
				return ValidationError{Field: fieldPrefix + ".virtualUrl", Message: "invalid URI"}
			}
		}
		if occ.VenueID != "" {
			if err := validateCanonicalURI(nodeDomain, "places", occ.VenueID); err != nil {
				return ValidationError{Field: fieldPrefix + ".venueId", Message: "invalid canonical URI"}
			}
		}
	}

	return nil
}

func validatePlaceInput(place PlaceInput, nodeDomain string) error {
	if place.ID == "" && strings.TrimSpace(place.Name) == "" {
		return ValidationError{Field: "location.name", Message: "required"}
	}
	if place.ID != "" {
		if err := validateCanonicalURI(nodeDomain, "places", place.ID); err != nil {
			return ValidationError{Field: "location.@id", Message: "invalid canonical URI"}
		}
	}
	return nil
}

func validateOrganizationInput(org OrganizationInput, nodeDomain string) error {
	if org.ID == "" && strings.TrimSpace(org.Name) == "" {
		return ValidationError{Field: "organizer.name", Message: "required"}
	}
	if org.ID != "" {
		if err := validateCanonicalURI(nodeDomain, "organizations", org.ID); err != nil {
			return ValidationError{Field: "organizer.@id", Message: "invalid canonical URI"}
		}
	}
	if org.URL != "" {
		if err := validateURL(org.URL); err != nil {
			return ValidationError{Field: "organizer.url", Message: "invalid URI"}
		}
	}
	return nil
}

func validateVirtualLocationInput(location VirtualLocationInput) error {
	if strings.TrimSpace(location.URL) == "" {
		return ValidationError{Field: "virtualLocation.url", Message: "required"}
	}
	if err := validateURL(location.URL); err != nil {
		return ValidationError{Field: "virtualLocation.url", Message: "invalid URI"}
	}
	return nil
}

func validateCanonicalURI(nodeDomain, entityPath, value string) error {
	parsed, err := ids.ParseEntityURI(nodeDomain, entityPath, value, "")
	if err != nil {
		return err
	}
	if parsed.Role != ids.RoleCanonical {
		return ids.ErrInvalidURI
	}
	return nil
}

func normalizeSameAs(nodeDomain, entityPath string, values []string) ([]string, error) {
	result := make([]string, 0, len(values))
	for _, value := range values {
		normalized, err := ids.NormalizeSameAs(nodeDomain, entityPath, value)
		if err != nil {
			return nil, ValidationError{Field: "sameAs", Message: "invalid URI"}
		}
		result = append(result, normalized)
	}
	return result, nil
}

func parseRFC3339(field, value string) (*time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, ValidationError{Field: field, Message: "required"}
	}
	parsed, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return nil, ValidationError{Field: field, Message: "invalid date-time"}
	}
	return &parsed, nil
}

func parseRFC3339Optional(field, value string) (*time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return nil, ValidationError{Field: field, Message: "invalid date-time"}
	}
	return &parsed, nil
}

func validateURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ids.ErrInvalidURI
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return nil
	default:
		return ids.ErrInvalidURI
	}
}

func isCC0License(value string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	switch trimmed {
	case "https://creativecommons.org/publicdomain/zero/1.0/",
		"http://creativecommons.org/publicdomain/zero/1.0/",
		"cc0",
		"cc0-1.0":
		return true
	default:
		return false
	}
}
