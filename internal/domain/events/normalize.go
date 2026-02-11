package events

import (
	"sort"
	"strings"
	"time"
)

// normalizeURL fixes common URL issues in external data sources:
// - Adds https:// prefix to URLs starting with "www."
// - Adds https:// to domain-like strings without protocol
// - Normalizes social media shorthand (@username patterns)
// - Preserves mailto: URLs
// - Returns empty string if input is empty
func normalizeURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	// Preserve mailto: and other non-http schemes
	if strings.HasPrefix(trimmed, "mailto:") {
		return trimmed
	}

	// Already has protocol
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		return trimmed
	}

	// Social media shorthand: @username -> don't try to normalize
	// These should be caught by validation if they're in a URL field
	if strings.HasPrefix(trimmed, "@") {
		return trimmed // Let validation handle @mentions in URL fields
	}

	// Starts with www. -> add https://
	if strings.HasPrefix(trimmed, "www.") {
		return "https://" + trimmed
	}

	// Looks like a domain without protocol (has dot and doesn't start with special chars)
	// Examples: example.com, sub.example.com, short.link
	if len(trimmed) > 0 && !strings.ContainsAny(trimmed[:1], "/@#") {
		// Simple heuristic: if it has a dot and looks domain-like, add https://
		if strings.Contains(trimmed, ".") && !strings.Contains(trimmed, " ") {
			return "https://" + trimmed
		}
	}

	// Otherwise return as-is (will be caught by validation if invalid)
	return trimmed
}

// eventSubtypeDomains maps schema.org Event subtypes to SEL event_domain values.
// Only subtypes that map to recognized domains are included.
var eventSubtypeDomains = map[string]string{
	"MusicEvent":      "music",
	"DanceEvent":      "arts",
	"Festival":        "arts",
	"TheaterEvent":    "arts",
	"ScreeningEvent":  "arts",
	"LiteraryEvent":   "arts",
	"EducationEvent":  "education",
	"ExhibitionEvent": "arts",
	"FoodEvent":       "community",
	"SportsEvent":     "sports",
	"SaleEvent":       "community",
	"SocialEvent":     "community",
	"ComedyEvent":     "arts",
	"ChildrensEvent":  "community",
	"BusinessEvent":   "general",
	"VisualArtsEvent": "arts",
	// "Event" is generic — no domain mapping
}

// NormalizeEventInput trims and normalizes values for consistent storage and hashing.
// Also auto-corrects common data quality issues like timezone errors and malformed URLs.
func NormalizeEventInput(input EventInput) EventInput {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.StartDate = strings.TrimSpace(input.StartDate)
	input.EndDate = strings.TrimSpace(input.EndDate)
	input.DoorTime = strings.TrimSpace(input.DoorTime)
	input.Image = normalizeURL(input.Image)
	input.URL = normalizeURL(input.URL)
	input.License = strings.TrimSpace(input.License)

	// Map schema.org Event subtypes to event_domain if not already set
	if input.EventDomain == "" && input.Type != "" {
		if domain, ok := eventSubtypeDomains[input.Type]; ok {
			input.EventDomain = domain
		}
	}

	// Auto-correct timezone errors where endDate appears before startDate
	// This typically happens with midnight-spanning events that were incorrectly
	// converted from local time to UTC
	input = correctEndDateTimezoneError(input)

	input.Keywords = normalizeStringSlice(input.Keywords, true)
	input.InLanguage = normalizeStringSlice(input.InLanguage, true)
	input.SameAs = normalizeStringSlice(input.SameAs, false)

	if input.Location != nil {
		input.Location = normalizePlaceInput(*input.Location)
	}
	if input.VirtualLocation != nil {
		input.VirtualLocation = normalizeVirtualLocationInput(*input.VirtualLocation)
	}
	if input.Organizer != nil {
		input.Organizer = normalizeOrganizationInput(*input.Organizer)
	}
	if input.Offers != nil {
		input.Offers = normalizeOfferInput(*input.Offers)
	}
	if input.Source != nil {
		input.Source = normalizeSourceInput(*input.Source)
	}
	if len(input.Occurrences) > 0 {
		input.Occurrences = normalizeOccurrences(input.Occurrences)
	}

	return input
}

func normalizePlaceInput(place PlaceInput) *PlaceInput {
	place.ID = strings.TrimSpace(place.ID)
	place.Name = strings.TrimSpace(place.Name)
	place.StreetAddress = strings.TrimSpace(place.StreetAddress)
	place.AddressLocality = strings.TrimSpace(place.AddressLocality)
	place.AddressRegion = strings.TrimSpace(place.AddressRegion)
	place.PostalCode = strings.TrimSpace(place.PostalCode)
	place.AddressCountry = strings.TrimSpace(place.AddressCountry)
	return &place
}

func normalizeVirtualLocationInput(location VirtualLocationInput) *VirtualLocationInput {
	location.Type = strings.TrimSpace(location.Type)
	location.URL = normalizeURL(location.URL)
	location.Name = strings.TrimSpace(location.Name)
	return &location
}

func normalizeOrganizationInput(org OrganizationInput) *OrganizationInput {
	org.ID = strings.TrimSpace(org.ID)
	org.Name = strings.TrimSpace(org.Name)
	org.URL = normalizeURL(org.URL)
	org.Email = strings.TrimSpace(org.Email)
	org.Telephone = strings.TrimSpace(org.Telephone)
	return &org
}

func normalizeOfferInput(offer OfferInput) *OfferInput {
	offer.URL = normalizeURL(offer.URL)
	offer.Price = strings.TrimSpace(offer.Price)
	offer.PriceCurrency = strings.TrimSpace(offer.PriceCurrency)
	return &offer
}

func normalizeSourceInput(source SourceInput) *SourceInput {
	source.URL = normalizeURL(source.URL)
	source.EventID = strings.TrimSpace(source.EventID)
	return &source
}

func normalizeOccurrences(values []OccurrenceInput) []OccurrenceInput {
	result := make([]OccurrenceInput, 0, len(values))
	for _, occ := range values {
		occ.StartDate = strings.TrimSpace(occ.StartDate)
		occ.EndDate = strings.TrimSpace(occ.EndDate)
		occ.DoorTime = strings.TrimSpace(occ.DoorTime)
		occ.Timezone = strings.TrimSpace(occ.Timezone)
		occ.VenueID = strings.TrimSpace(occ.VenueID)
		occ.VirtualURL = normalizeURL(occ.VirtualURL)

		// Apply timezone error correction to occurrence dates (same as top-level)
		occ = correctOccurrenceEndDateTimezoneError(occ)

		result = append(result, occ)
	}
	return result
}

// correctOccurrenceEndDateTimezoneError applies the same timezone correction logic
// as correctEndDateTimezoneError but for individual occurrences.
// Only corrects if end time is in early morning (0-4 AM) and corrected duration < 7h.
func correctOccurrenceEndDateTimezoneError(occ OccurrenceInput) OccurrenceInput {
	if occ.EndDate == "" {
		return occ
	}

	startTime, err := time.Parse(time.RFC3339, occ.StartDate)
	if err != nil {
		return occ // Invalid startDate, let validation handle it
	}

	endTime, err := time.Parse(time.RFC3339, occ.EndDate)
	if err != nil {
		return occ // Invalid endDate, let validation handle it
	}

	// Check if endDate is before startDate (the error condition)
	if !endTime.Before(startTime) {
		return occ // No correction needed
	}

	endHour := endTime.Hour() // 0-23 in UTC

	// Only auto-correct if end time is in early morning (0-4 AM)
	if endHour <= 4 {
		correctedEnd := endTime.Add(24 * time.Hour)

		// Check if the corrected event duration is reasonable (< 7 hours)
		duration := correctedEnd.Sub(startTime)
		if duration > 0 && duration < 7*time.Hour {
			// Apply correction: add 24 hours to endDate
			occ.EndDate = correctedEnd.Format(time.RFC3339)
		}
	}

	return occ
}

func normalizeStringSlice(values []string, lower bool) []string {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if lower {
			trimmed = strings.ToLower(trimmed)
		}
		set[trimmed] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

// correctEndDateTimezoneError detects and fixes common timezone conversion errors
// where endDate appears chronologically before startDate.
//
// This typically occurs with midnight-spanning events that were incorrectly converted
// from local time to UTC. For example:
//   - Event: 11 PM - 2 AM local time (EST/EDT)
//   - Incorrect conversion: 2025-03-31T23:00Z to 2025-03-31T02:00Z
//   - Should be: 2025-03-31T23:00Z to 2025-04-01T02:00Z
//
// Auto-correction is conservative and only applies when:
//   - End time is in early morning (0-4 AM UTC), AND
//   - Corrected duration would be < 7 hours
//
// This filters out bad data while fixing typical overnight events.
// Validation adds warnings with different confidence levels for corrected events.
func correctEndDateTimezoneError(input EventInput) EventInput {
	if input.EndDate == "" {
		return input // No endDate to correct
	}

	startTime, err := time.Parse(time.RFC3339, input.StartDate)
	if err != nil {
		return input // Invalid startDate, let validation handle it
	}

	endTime, err := time.Parse(time.RFC3339, input.EndDate)
	if err != nil {
		return input // Invalid endDate, let validation handle it
	}

	// Check if endDate is before startDate (the error condition)
	if !endTime.Before(startTime) {
		return input // No correction needed, dates are in correct order
	}

	endHour := endTime.Hour() // 0-23 in UTC

	// Only auto-correct if end time is in early morning (0-4 AM)
	// This strongly suggests a legitimate overnight event
	if endHour <= 4 {
		correctedEnd := endTime.Add(24 * time.Hour)

		// Check if the corrected event duration is reasonable (< 7 hours)
		// This filters out bad data while allowing typical overnight events
		duration := correctedEnd.Sub(startTime)
		if duration > 0 && duration < 7*time.Hour {
			// Apply correction: add 24 hours to endDate
			input.EndDate = correctedEnd.Format(time.RFC3339)
		}
	}
	// If conditions aren't met, leave as-is and let validation handle it

	return input
}
