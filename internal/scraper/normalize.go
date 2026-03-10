package scraper

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
)

// rawEvent is an intermediate struct that can unmarshal all the variant
// JSON-LD shapes emitted by schema.org structured data.
type rawEvent struct {
	AtType              string          `json:"@type"`
	AtID                string          `json:"@id"`
	Name                json.RawMessage `json:"name"`                // may be string or {"@value":"..."}
	Description         json.RawMessage `json:"description"`         // may be string or {"@value":"..."}
	StartDate           json.RawMessage `json:"startDate"`           // may be string or {"@type":"Date","@value":"..."}
	EndDate             json.RawMessage `json:"endDate"`             // same
	DoorTime            json.RawMessage `json:"doorTime"`            // same
	Location            json.RawMessage `json:"location"`            // may be string or Place object
	Organizer           json.RawMessage `json:"organizer"`           // may be object or array
	Image               json.RawMessage `json:"image"`               // may be string or ImageObject
	URL                 json.RawMessage `json:"url"`                 // may be string
	Offers              json.RawMessage `json:"offers"`              // may be object or array
	Keywords            json.RawMessage `json:"keywords"`            // may be string or []string
	InLanguage          json.RawMessage `json:"inLanguage"`          // may be string or []string
	IsAccessibleForFree json.RawMessage `json:"isAccessibleForFree"` // may be bool or "True"/"False"
	SameAs              json.RawMessage `json:"sameAs"`              // may be string or []string
	Identifier          json.RawMessage `json:"identifier"`          // may be string or {"@value":"..."}
}

// NormalizeJSONLDEvent converts a raw JSON-LD Event object (schema.org) to an
// EventInput suitable for submission to the SEL ingest API.
func NormalizeJSONLDEvent(raw json.RawMessage, source SourceConfig) (events.EventInput, error) {
	var re rawEvent
	if err := json.Unmarshal(raw, &re); err != nil {
		return events.EventInput{}, fmt.Errorf("unmarshal raw event: %w", err)
	}

	name := extractStringValue(re.Name)
	if name == "" {
		return events.EventInput{}, fmt.Errorf("event has no name")
	}

	startDate := parseDate(re.StartDate)
	if startDate == "" {
		return events.EventInput{}, fmt.Errorf("event has no startDate")
	}

	urlStr := extractStringValue(re.URL)

	// Preserve schema.org subtype (MusicEvent, TheaterEvent, etc.) with "Event" as fallback.
	eventType := re.AtType
	if eventType == "" {
		eventType = "Event"
	}

	multiSessionThreshold, err := parseMultiSessionThreshold(source.MultiSessionDurationThreshold)
	if err != nil {
		return events.EventInput{}, err
	}

	evt := events.EventInput{
		Type:                          eventType,
		Name:                          name,
		Description:                   extractStringValue(re.Description),
		StartDate:                     startDate,
		EndDate:                       parseDate(re.EndDate),
		DoorTime:                      parseDate(re.DoorTime),
		Location:                      parseLocation(re.Location),
		Organizer:                     parseOrganizer(re.Organizer),
		Image:                         parseImage(re.Image),
		URL:                           urlStr,
		Offers:                        parseOffer(re.Offers),
		Keywords:                      parseStringOrArray(re.Keywords),
		InLanguage:                    parseStringOrArray(re.InLanguage),
		IsAccessibleForFree:           parseBool(re.IsAccessibleForFree),
		SameAs:                        parseStringOrArray(re.SameAs),
		License:                       source.License,
		SkipMultiSessionCheck:         source.SkipMultiSessionCheck,
		MultiSessionDurationThreshold: multiSessionThreshold,
		Source: &events.SourceInput{
			URL:     source.URL,
			EventID: extractEventID(raw),
			Name:    source.Name,
			License: source.License,
		},
	}

	return evt, nil
}

// RawEvent holds extracted text fields from Tier 1 CSS selector scraping.
type RawEvent struct {
	Name        string
	StartDate   string
	EndDate     string
	Location    string
	Description string
	URL         string
	Image       string
	// DateParts holds text extracted from date_selectors (one entry per
	// selector). When populated, the smart date assembler combines these
	// into RFC 3339 start/end dates, replacing StartDate/EndDate.
	DateParts []string
}

// NormalizeRawEvent converts a CSS-extracted RawEvent to EventInput.
// More lenient than JSON-LD normalization since Tier 1 data is unstructured.
//
// Date handling: when DateParts is populated (from date_selectors config),
// the smart date assembler combines the parts into RFC 3339. Otherwise,
// StartDate/EndDate are normalized from human-readable text to RFC 3339 as
// a fallback. If the date is already RFC 3339, it passes through unchanged.
func NormalizeRawEvent(raw RawEvent, source SourceConfig) (events.EventInput, error) {
	if raw.Name == "" {
		return events.EventInput{}, fmt.Errorf("raw event has no name")
	}

	// Normalize dates: DateParts → assemble, or StartDate/EndDate → fuzzy parse.
	startDate, endDate := normalizeStartDate(raw, source.GetTimezone())

	if startDate == "" {
		return events.EventInput{}, fmt.Errorf("raw event has no startDate")
	}

	multiSessionThreshold, err := parseMultiSessionThreshold(source.MultiSessionDurationThreshold)
	if err != nil {
		return events.EventInput{}, err
	}

	var loc *events.PlaceInput
	if raw.Location != "" {
		loc = &events.PlaceInput{Name: raw.Location}
	}

	return events.EventInput{
		Type:                          "Event",
		Name:                          raw.Name,
		StartDate:                     startDate,
		EndDate:                       endDate,
		Description:                   raw.Description,
		URL:                           raw.URL,
		Image:                         raw.Image,
		Location:                      loc,
		License:                       source.License,
		SkipMultiSessionCheck:         source.SkipMultiSessionCheck,
		MultiSessionDurationThreshold: multiSessionThreshold,
		Source: &events.SourceInput{
			URL:     source.URL,
			EventID: eventIDFromRaw(raw, source),
			Name:    source.Name,
			License: source.License,
		},
	}, nil
}

// consolidateOccurrences takes a set of RawEvents that were extracted from the same
// event_list container (and thus share Name, Location, Description, URL, and Image),
// and consolidates them into a single EventInput with multiple occurrences.
//
// This handles the multi-row date table case where a show detail page lists
// multiple performance dates (e.g., Luminato Festival with 3 performances per show).
//
// Returns the consolidated EventInput if there are multiple rows, or the original
// single event if there's only one.
func consolidateOccurrences(raws []RawEvent, source SourceConfig) (events.EventInput, error) {
	if len(raws) == 0 {
		return events.EventInput{}, fmt.Errorf("consolidateOccurrences: empty input")
	}

	// Normalize each raw event to get dates and build occurrence list.
	var occurrences []events.OccurrenceInput
	tz := source.GetTimezone()

	for _, raw := range raws {
		// Normalize the date parts for this row.
		startDate, endDate := normalizeStartDate(raw, tz)
		if startDate == "" {
			// Skip rows that don't have a valid start date.
			continue
		}

		occ := events.OccurrenceInput{
			StartDate: startDate,
			EndDate:   endDate,
			// Note: Timezone, DoorTime, VenueID, and VirtualURL would need additional
			// extraction logic if the config provided selectors for them. For now,
			// they remain empty.
		}
		occurrences = append(occurrences, occ)
	}

	if len(occurrences) == 0 {
		return events.EventInput{}, fmt.Errorf("consolidateOccurrences: no valid dates found in any row")
	}

	// Use the first raw event's fields for the consolidated event.
	first := raws[0]
	multiSessionThreshold, err := parseMultiSessionThreshold(source.MultiSessionDurationThreshold)
	if err != nil {
		return events.EventInput{}, err
	}

	var loc *events.PlaceInput
	if first.Location != "" {
		loc = &events.PlaceInput{Name: first.Location}
	}

	// Use the first occurrence's start date as the event's primary StartDate.
	// This is a convention for backward compatibility.
	startDate := ""
	if len(occurrences) > 0 {
		startDate = occurrences[0].StartDate
	}

	input := events.EventInput{
		Type:                          "Event",
		Name:                          first.Name,
		StartDate:                     startDate,
		Description:                   first.Description,
		URL:                           first.URL,
		Image:                         first.Image,
		Location:                      loc,
		Occurrences:                   occurrences,
		License:                       source.License,
		SkipMultiSessionCheck:         source.SkipMultiSessionCheck,
		MultiSessionDurationThreshold: multiSessionThreshold,
		Source: &events.SourceInput{
			URL:     source.URL,
			EventID: eventIDFromRaw(first, source),
			Name:    source.Name,
			License: source.License,
		},
	}

	return input, nil
}

// eventIDFromRaw returns a stable dedup key for a Tier 1 scraped event.
// Prefers the event URL. Falls back to a hash of name+startDate+sourceName.
func eventIDFromRaw(raw RawEvent, source SourceConfig) string {
	if raw.URL != "" {
		return raw.URL
	}
	// Generate deterministic ID from available fields when URL is missing.
	h := fmt.Sprintf("%s|%s|%s", raw.Name, raw.StartDate, source.Name)
	sum := 0
	for _, c := range h {
		sum = sum*31 + int(c)
	}
	return fmt.Sprintf("scraped:%s:%d", source.Name, sum)
}

// parseDate attempts to extract a date/datetime string from a json.RawMessage.
// It handles plain strings, {"@value":"..."}, and {"@type":"Date","@value":"..."}.
// The value is returned as-is without reformatting — let the server validate.
func parseDate(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// Try plain string first.
	if s := extractStringValue(raw); s != "" {
		return s
	}
	// Try typed value object: {"@type":"Date","@value":"..."} or {"@type":"DateTime","@value":"..."}
	var obj struct {
		AtType string `json:"@type"`
		Value  string `json:"@value"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil && obj.Value != "" {
		return obj.Value
	}
	return ""
}

// parseLocation converts a location JSON-LD value to a PlaceInput.
// Handles plain string names, Place objects with nested PostalAddress, and
// Place objects with flat address fields or geo coordinates.
func parseLocation(raw json.RawMessage) *events.PlaceInput {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}

	// Plain string → name-only place.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if s != "" {
			return &events.PlaceInput{Name: s}
		}
		return nil
	}

	// Object — may be a single Place or an array; take the first element.
	raw = firstElement(raw)
	if raw == nil {
		return nil
	}

	var obj struct {
		AtType  string          `json:"@type"`
		AtID    string          `json:"@id"`
		Name    json.RawMessage `json:"name"`
		Address json.RawMessage `json:"address"`
		// Flat address fields (sometimes placed at top level).
		StreetAddress   json.RawMessage `json:"streetAddress"`
		AddressLocality json.RawMessage `json:"addressLocality"`
		AddressRegion   json.RawMessage `json:"addressRegion"`
		PostalCode      json.RawMessage `json:"postalCode"`
		AddressCountry  json.RawMessage `json:"addressCountry"`
		Geo             json.RawMessage `json:"geo"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}

	place := &events.PlaceInput{
		ID:   obj.AtID,
		Name: extractStringValue(obj.Name),
	}

	// Nested address object.
	if len(obj.Address) > 0 && string(obj.Address) != "null" {
		var addr struct {
			StreetAddress   json.RawMessage `json:"streetAddress"`
			AddressLocality json.RawMessage `json:"addressLocality"`
			AddressRegion   json.RawMessage `json:"addressRegion"`
			PostalCode      json.RawMessage `json:"postalCode"`
			AddressCountry  json.RawMessage `json:"addressCountry"`
		}
		if err := json.Unmarshal(obj.Address, &addr); err == nil {
			place.StreetAddress = extractStringValue(addr.StreetAddress)
			place.AddressLocality = extractStringValue(addr.AddressLocality)
			place.AddressRegion = extractStringValue(addr.AddressRegion)
			place.PostalCode = extractStringValue(addr.PostalCode)
			place.AddressCountry = extractStringValue(addr.AddressCountry)
		}
	}

	// Flat address fields override nested when nested is empty.
	if place.StreetAddress == "" {
		place.StreetAddress = extractStringValue(obj.StreetAddress)
	}
	if place.AddressLocality == "" {
		place.AddressLocality = extractStringValue(obj.AddressLocality)
	}
	if place.AddressRegion == "" {
		place.AddressRegion = extractStringValue(obj.AddressRegion)
	}
	if place.PostalCode == "" {
		place.PostalCode = extractStringValue(obj.PostalCode)
	}
	if place.AddressCountry == "" {
		place.AddressCountry = extractStringValue(obj.AddressCountry)
	}

	// Geo coordinates.
	if len(obj.Geo) > 0 && string(obj.Geo) != "null" {
		var geo struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		}
		if err := json.Unmarshal(obj.Geo, &geo); err == nil {
			place.Latitude = geo.Latitude
			place.Longitude = geo.Longitude
		}
	}

	// Return nil if nothing useful was extracted.
	if place.Name == "" && place.StreetAddress == "" && place.AddressLocality == "" && place.ID == "" {
		return nil
	}
	return place
}

// parseOrganizer converts an organizer JSON-LD value to an OrganizationInput.
// Handles a single object or an array (uses first element).
func parseOrganizer(raw json.RawMessage) *events.OrganizationInput {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}

	raw = firstElement(raw)
	if raw == nil {
		return nil
	}

	var obj struct {
		AtID      string          `json:"@id"`
		Name      json.RawMessage `json:"name"`
		URL       json.RawMessage `json:"url"`
		Email     json.RawMessage `json:"email"`
		Telephone json.RawMessage `json:"telephone"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}

	org := &events.OrganizationInput{
		ID:        obj.AtID,
		Name:      extractStringValue(obj.Name),
		URL:       extractStringValue(obj.URL),
		Email:     extractStringValue(obj.Email),
		Telephone: extractStringValue(obj.Telephone),
	}

	if org.Name == "" && org.URL == "" && org.ID == "" {
		return nil
	}
	return org
}

// parseOffer converts an offers JSON-LD value to an OfferInput.
// Handles a single Offer object or an array (uses first element).
func parseOffer(raw json.RawMessage) *events.OfferInput {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}

	raw = firstElement(raw)
	if raw == nil {
		return nil
	}

	var obj struct {
		Price         json.RawMessage `json:"price"`
		PriceCurrency json.RawMessage `json:"priceCurrency"`
		URL           json.RawMessage `json:"url"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}

	offer := &events.OfferInput{
		Price:         extractStringValue(obj.Price),
		PriceCurrency: extractStringValue(obj.PriceCurrency),
		URL:           extractStringValue(obj.URL),
	}

	if offer.Price == "" && offer.PriceCurrency == "" && offer.URL == "" {
		return nil
	}
	return offer
}

// parseImage extracts an image URL from a JSON-LD image value.
// Handles plain string URLs, ImageObject with "url", and ImageObject with "contentUrl".
func parseImage(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}

	// Plain string.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}

	// Array — try first element.
	raw = firstElement(raw)
	if raw == nil {
		return ""
	}

	var obj struct {
		URL        string `json:"url"`
		ContentURL string `json:"contentUrl"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return ""
	}
	if obj.URL != "" {
		return obj.URL
	}
	return obj.ContentURL
}

// parseBool extracts a boolean from a JSON-LD value.
// Handles JSON booleans and string representations "True", "true", "False", "false".
func parseBool(raw json.RawMessage) *bool {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}

	// JSON bool.
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		result := b
		return &result
	}

	// String representation.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		switch strings.ToLower(s) {
		case "true":
			t := true
			return &t
		case "false":
			f := false
			return &f
		}
	}

	return nil
}

// parseStringOrArray extracts a []string from a JSON-LD value that may be a
// plain string or an array of strings.
func parseStringOrArray(raw json.RawMessage) []string {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}

	// Try plain string.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if s == "" {
			return nil
		}
		return []string{s}
	}

	// Try array.
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		if len(arr) == 0 {
			return nil
		}
		return arr
	}

	return nil
}

// extractEventID attempts to find a stable external identifier for the event.
// Priority: @id → identifier → url.
func extractEventID(raw json.RawMessage) string {
	var obj struct {
		AtID       string          `json:"@id"`
		Identifier json.RawMessage `json:"identifier"`
		URL        json.RawMessage `json:"url"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return ""
	}

	if obj.AtID != "" {
		return obj.AtID
	}

	if id := extractStringValue(obj.Identifier); id != "" {
		return id
	}

	if u := extractStringValue(obj.URL); u != "" {
		return u
	}

	return ""
}

// extractStringValue extracts a plain string from a json.RawMessage that may
// be a JSON string or a {"@value":"..."} object.
func extractStringValue(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}

	// Plain string.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}

	// {"@value":"..."} object.
	var obj struct {
		Value string `json:"@value"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		return obj.Value
	}

	return ""
}

// firstElement returns the first element of a JSON array, or the original
// value if it is not an array. Returns nil if the array is empty.
func firstElement(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if !strings.HasPrefix(trimmed, "[") {
		return raw
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil || len(arr) == 0 {
		return nil
	}
	return arr[0]
}

// HasTruncatedDescription reports whether desc appears to be truncated,
// indicated by ending with an ellipsis character or HTML entity.
func HasTruncatedDescription(desc string) bool {
	desc = strings.TrimSpace(desc)
	return strings.HasSuffix(desc, "…") || // U+2026
		strings.HasSuffix(desc, "...") ||
		strings.HasSuffix(desc, "\u2026")
}

// parseMultiSessionThreshold parses a Go duration string (e.g. "720h") from a
// SourceConfig and returns the duration. Returns 0 and no error when the value
// is empty (caller should use the default). Returns an error for non-empty but
// unparseable values.
func parseMultiSessionThreshold(raw string) (time.Duration, error) {
	if raw == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("parse multi_session_duration_threshold %q: %w", raw, err)
	}
	return d, nil
}
