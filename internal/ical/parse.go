package ical

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	ics "github.com/arran4/golang-ical"
)

// maxSummaryRunes is the maximum length of a SUMMARY field in runes.
const maxSummaryRunes = 500

// maxDescriptionBytes is the maximum length of a DESCRIPTION field in bytes.
const maxDescriptionBytes = 100 * 1024 // 100 KB

// ParsedCalendar holds the result of parsing a VCALENDAR.
type ParsedCalendar struct {
	ProdID   string // PRODID value
	Name     string // X-WR-CALNAME (informal calendar name)
	Source   string // SOURCE property (RFC 7986)
	Events   []ParsedEvent
	Warnings []string // Non-fatal parse issues (skipped events, etc.)
}

// ParsedEvent holds a single VEVENT extracted from a VCALENDAR.
type ParsedEvent struct {
	UID            string            // VEVENT UID (used for dedup + provenance)
	Summary        string            // SUMMARY → EventInput.Name
	Description    string            // DESCRIPTION (may contain HTML)
	Location       string            // LOCATION text
	URL            string            // URL property
	Start          time.Time         // DTSTART (resolved to UTC or explicit TZ)
	End            time.Time         // DTEND (zero if absent — caller infers)
	Duration       time.Duration     // DURATION property (zero if absent; used when DTEND missing)
	AllDay         bool              // True when DTSTART is DATE (not DATE-TIME)
	RRULE          string            // Raw RRULE string, empty if non-recurring
	RecurrenceID   time.Time         // RECURRENCE-ID (zero if absent; identifies exception to RRULE series)
	ExDates        []time.Time       // EXDATE values
	RDates         []time.Time       // RDATE values
	TZID           string            // IANA timezone ID from DTSTART TZID parameter (empty if UTC/floating)
	Organizer      string            // ORGANIZER CN parameter (display name)
	OrganizerEmail string            // ORGANIZER mailto: value (email address)
	Categories     []string          // CATEGORIES values (split on comma per value)
	GeoLat         float64           // GEO latitude (0 if absent)
	GeoLon         float64           // GEO longitude (0 if absent)
	HasGeo         bool              // True when GEO was successfully parsed
	Created        time.Time         // CREATED timestamp
	LastMod        time.Time         // LAST-MODIFIED timestamp
	Sequence       int               // SEQUENCE (change counter)
	Status         string            // STATUS: CONFIRMED, TENTATIVE, CANCELLED
	RawProps       map[string]string // Other properties preserved for payload provenance
}

// WindowsTZIDAliases maps common Windows timezone names (emitted by
// Outlook/Exchange VTIMEZONE components) to IANA timezone identifiers.
var WindowsTZIDAliases = map[string]string{
	"Eastern Standard Time":        "America/New_York",
	"Central Standard Time":        "America/Chicago",
	"Mountain Standard Time":       "America/Denver",
	"Pacific Standard Time":        "America/Los_Angeles",
	"Atlantic Standard Time":       "America/Halifax",
	"Newfoundland Standard Time":   "America/St_Johns",
	"GMT Standard Time":            "Europe/London",
	"W. Europe Standard Time":      "Europe/Berlin",
	"Romance Standard Time":        "Europe/Paris",
	"Central Europe Standard Time": "Europe/Budapest",
	"E. Europe Standard Time":      "Europe/Chisinau",
	"FLE Standard Time":            "Europe/Kiev",
	"GTB Standard Time":            "Europe/Bucharest",
	"Russian Standard Time":        "Europe/Moscow",
	"AUS Eastern Standard Time":    "Australia/Sydney",
	"Tokyo Standard Time":          "Asia/Tokyo",
	"China Standard Time":          "Asia/Shanghai",
	"India Standard Time":          "Asia/Kolkata",
	"Singapore Standard Time":      "Asia/Singapore",
	"Korea Standard Time":          "Asia/Seoul",
}

// Parse parses raw ICS bytes into a ParsedCalendar.
// Lenient mode: malformed VEVENTs are skipped with a warning appended to
// ParsedCalendar.Warnings. Only returns error if the overall VCALENDAR
// structure is unparseable.
func Parse(data []byte) (*ParsedCalendar, error) {
	result := &ParsedCalendar{}

	// Use AcceptUnknownPropertyHandler so that non-standard properties
	// (e.g. X-WR-CALNAME appearing after END:VTIMEZONE in real-world Meetup
	// feeds) are silently accepted instead of causing a parse error.
	//
	// Use WithPropertyParseErrorHandler to skip properties with malformed
	// parameter names (e.g. Tockify X-TKF-* properties containing underscores
	// in parameter names like "skip_details=false"). These are non-RFC-compliant
	// but harmless — the property values are presentation hints we don't need.
	cal, err := ics.ParseCalendarWithOptions(bytes.NewReader(data),
		ics.WithUnknownPropertyHandler(ics.AcceptUnknownPropertyHandler),
		ics.WithPropertyParseErrorHandler(func(rawLine ics.ContentLine, parseErr error) (*ics.BaseProperty, error) {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("skipped malformed property: %s", parseErr))
			return nil, nil // skip the property
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("ics parse: %w", err)
	}

	// Extract calendar-level properties.
	for _, prop := range cal.CalendarProperties {
		switch prop.IANAToken {
		case string(ics.PropertyProductId):
			result.ProdID = prop.Value
		case string(ics.PropertyXWRCalName):
			result.Name = prop.Value
		case "SOURCE":
			result.Source = prop.Value
		}
	}

	// Track seen UIDs for duplicate detection.
	seenUIDs := make(map[string]bool)

	for _, vevent := range cal.Events() {
		parsed, warnings, err := parseVEvent(vevent, seenUIDs)
		result.Warnings = append(result.Warnings, warnings...)
		if err != nil {
			// Event was skipped — warning already recorded.
			continue
		}
		if parsed == nil {
			continue
		}
		result.Events = append(result.Events, *parsed)
	}

	return result, nil
}

// parseVEvent extracts a ParsedEvent from a VEvent.
// Returns (nil, warnings, skip-error) if the event should be skipped.
func parseVEvent(ve *ics.VEvent, seenUIDs map[string]bool) (*ParsedEvent, []string, error) {
	var warnings []string
	uid := ve.Id()

	// --- SUMMARY (required) ---
	summaryProp := ve.GetProperty(ics.ComponentPropertySummary)
	if summaryProp == nil || strings.TrimSpace(summaryProp.Value) == "" {
		w := fmt.Sprintf("missing SUMMARY (UID: %s)", uid)
		return nil, []string{w}, fmt.Errorf("%s", w)
	}
	summary := strings.TrimSpace(ics.FromText(summaryProp.Value))

	// Truncate SUMMARY if too long.
	if utf8.RuneCountInString(summary) > maxSummaryRunes {
		runes := []rune(summary)
		summary = string(runes[:maxSummaryRunes])
		warnings = append(warnings, fmt.Sprintf("SUMMARY truncated (UID: %s)", uid))
	}

	// --- DTSTART (required) ---
	dtStartProp := ve.GetProperty(ics.ComponentPropertyDtStart)
	if dtStartProp == nil {
		w := fmt.Sprintf("missing DTSTART (UID: %s)", uid)
		return nil, append(warnings, w), fmt.Errorf("%s", w)
	}

	allDay := isAllDayProp(dtStartProp)
	start, err := parseTimeProp(dtStartProp)
	if err != nil {
		w := fmt.Sprintf("unparseable DTSTART (UID: %s): %v", uid, err)
		return nil, append(warnings, w), fmt.Errorf("%s", w)
	}

	// Extract TZID from DTSTART for series timezone resolution.
	tzid := extractTZID(dtStartProp)

	// --- Duplicate UID detection ---
	// RECURRENCE-ID events are allowed to share a UID with the master event.
	recurrenceIDProp := ve.GetProperty(ics.ComponentPropertyRecurrenceId)
	hasRecurrenceID := recurrenceIDProp != nil

	if !hasRecurrenceID {
		if seenUIDs[uid] {
			w := fmt.Sprintf("duplicate UID (UID: %s)", uid)
			return nil, append(warnings, w), fmt.Errorf("%s", w)
		}
		seenUIDs[uid] = true
	}

	// --- DTEND ---
	var end time.Time
	dtEndProp := ve.GetProperty(ics.ComponentPropertyDtEnd)
	if dtEndProp != nil {
		endParsed, err := parseTimeProp(dtEndProp)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("unparseable DTEND, using DTSTART (UID: %s): %v", uid, err))
		} else {
			end = endParsed
		}
	}

	// --- DTEND before DTSTART check ---
	if !end.IsZero() && end.Before(start) {
		w := fmt.Sprintf("DTEND before DTSTART (UID: %s)", uid)
		return nil, append(warnings, w), fmt.Errorf("%s", w)
	}

	// --- DURATION ---
	var duration time.Duration
	durationProp := ve.GetProperty(ics.ComponentPropertyDuration)
	if durationProp != nil && durationProp.Value != "" {
		d, err := parseISO8601Duration(durationProp.Value)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("unparseable DURATION (UID: %s): %v", uid, err))
		} else {
			duration = d
			// If DTEND is missing, compute from DURATION.
			if end.IsZero() {
				end = start.Add(duration)
			}
		}
	}

	// If still no end, set End = Start (zero-duration per RFC 5545 default).
	if end.IsZero() {
		end = start
	}

	// --- DESCRIPTION ---
	description := ""
	descProp := ve.GetProperty(ics.ComponentPropertyDescription)
	if descProp != nil {
		description = ics.FromText(descProp.Value)
		if len(description) > maxDescriptionBytes {
			description = description[:maxDescriptionBytes]
			warnings = append(warnings, fmt.Sprintf("DESCRIPTION truncated (UID: %s)", uid))
		}
	}

	// --- LOCATION ---
	location := ""
	locProp := ve.GetProperty(ics.ComponentPropertyLocation)
	if locProp != nil {
		location = ics.FromText(locProp.Value)
	}

	// --- URL ---
	urlStr := ""
	urlProp := ve.GetProperty(ics.ComponentPropertyUrl)
	if urlProp != nil {
		urlStr = urlProp.Value
	}

	// --- GEO ---
	var geoLat, geoLon float64
	var hasGeo bool
	geoProp := ve.GetProperty(ics.ComponentPropertyGeo)
	if geoProp != nil && geoProp.Value != "" {
		lat, lon, err := parseGeo(geoProp.Value)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("invalid GEO (UID: %s): %v", uid, err))
		} else {
			geoLat, geoLon, hasGeo = lat, lon, true
		}
	}

	// --- CATEGORIES ---
	var categories []string
	catProps := ve.GetProperties(ics.ComponentPropertyCategories)
	for _, cp := range catProps {
		// Each CATEGORIES property can have comma-separated values.
		vals := strings.Split(ics.FromText(cp.Value), ",")
		for _, v := range vals {
			v = strings.TrimSpace(v)
			if v != "" {
				categories = append(categories, v)
			}
		}
	}

	// --- ORGANIZER ---
	var organizer, organizerEmail string
	orgProp := ve.GetProperty(ics.ComponentPropertyOrganizer)
	if orgProp != nil {
		// CN parameter → display name.
		if cn, ok := orgProp.ICalParameters[string(ics.ParameterCn)]; ok && len(cn) > 0 {
			organizer = cn[0]
		}
		// Value is typically "mailto:user@example.com".
		val := orgProp.Value
		if strings.HasPrefix(strings.ToLower(val), "mailto:") {
			organizerEmail = val[len("mailto:"):]
		}
	}

	// --- RRULE ---
	var rruleStr string
	rruleProp := ve.GetProperty(ics.ComponentPropertyRrule)
	if rruleProp != nil {
		rruleStr = rruleProp.Value
	}

	// --- EXDATE ---
	exdates, err := ve.GetExDates()
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("unparseable EXDATE (UID: %s): %v", uid, err))
		exdates = nil
	}

	// --- RDATE ---
	rdates, err := ve.GetRDates()
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("unparseable RDATE (UID: %s): %v", uid, err))
		rdates = nil
	}

	// --- RECURRENCE-ID ---
	var recurrenceID time.Time
	if hasRecurrenceID {
		recID, err := parseTimeProp(recurrenceIDProp)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("unparseable RECURRENCE-ID (UID: %s): %v", uid, err))
		} else {
			recurrenceID = recID
		}
	}

	// --- CREATED ---
	var created time.Time
	createdProp := ve.GetProperty(ics.ComponentPropertyCreated)
	if createdProp != nil {
		t, err := parseTimeProp(createdProp)
		if err == nil {
			created = t
		}
	}

	// --- LAST-MODIFIED ---
	var lastMod time.Time
	lastModProp := ve.GetProperty(ics.ComponentPropertyLastModified)
	if lastModProp != nil {
		t, err := parseTimeProp(lastModProp)
		if err == nil {
			lastMod = t
		}
	}

	// --- SEQUENCE ---
	var sequence int
	seqProp := ve.GetProperty(ics.ComponentPropertySequence)
	if seqProp != nil {
		if n, err := strconv.Atoi(seqProp.Value); err == nil {
			sequence = n
		}
	}

	// --- STATUS ---
	status := ""
	statusProp := ve.GetProperty(ics.ComponentPropertyStatus)
	if statusProp != nil {
		status = strings.ToUpper(strings.TrimSpace(statusProp.Value))
	}

	// --- RawProps (non-standard and unhandled properties) ---
	// Collect X-* extended properties and other properties not already
	// extracted above, preserving source provenance per SEL requirements.
	rawProps := collectRawProps(ve)

	return &ParsedEvent{
		UID:            uid,
		Summary:        summary,
		Description:    description,
		Location:       location,
		URL:            urlStr,
		Start:          start,
		End:            end,
		Duration:       duration,
		AllDay:         allDay,
		RRULE:          rruleStr,
		RecurrenceID:   recurrenceID,
		ExDates:        exdates,
		RDates:         rdates,
		TZID:           tzid,
		Organizer:      organizer,
		OrganizerEmail: organizerEmail,
		Categories:     categories,
		GeoLat:         geoLat,
		GeoLon:         geoLon,
		HasGeo:         hasGeo,
		Created:        created,
		LastMod:        lastMod,
		Sequence:       sequence,
		Status:         status,
		RawProps:       rawProps,
	}, warnings, nil
}

// extractTZID returns the IANA timezone identifier from a time property's
// TZID parameter, with Windows TZID aliases resolved to their IANA equivalents.
// Returns empty string if no TZID parameter is present.
func extractTZID(prop *ics.IANAProperty) string {
	if tzids, ok := prop.ICalParameters[string(ics.ParameterTzid)]; ok && len(tzids) > 0 {
		tzid := tzids[0]
		loc, err := time.LoadLocation(tzid)
		if err == nil {
			_ = loc // valid IANA
			return tzid
		}
		// Try Windows TZID alias map.
		if iana, ok := WindowsTZIDAliases[tzid]; ok {
			return iana
		}
	}
	return ""
}

// isAllDayProp checks if a time property has VALUE=DATE (all-day event).
func isAllDayProp(prop *ics.IANAProperty) bool {
	if vals, ok := prop.ICalParameters[string(ics.ParameterValue)]; ok {
		for _, v := range vals {
			if strings.EqualFold(v, "DATE") {
				return true
			}
		}
	}
	return false
}

// parseTimeProp parses a time property, handling TZID parameters including
// Windows timezone aliases from Outlook/Exchange.
func parseTimeProp(prop *ics.IANAProperty) (time.Time, error) {
	value := prop.Value
	params := prop.ICalParameters

	// Check for VALUE=DATE (all-day).
	isDate := false
	if vals, ok := params[string(ics.ParameterValue)]; ok {
		for _, v := range vals {
			if strings.EqualFold(v, "DATE") {
				isDate = true
			}
		}
	}

	// Handle TZID parameter.
	if tzids, ok := params[string(ics.ParameterTzid)]; ok && len(tzids) > 0 {
		tzid := tzids[0]
		loc, err := time.LoadLocation(tzid)
		if err != nil {
			// Try Windows TZID alias map.
			if iana, ok := WindowsTZIDAliases[tzid]; ok {
				loc, err = time.LoadLocation(iana)
				if err != nil {
					// Alias exists but IANA lookup failed — fall back to UTC.
					loc = time.UTC
				}
			} else {
				// Unknown TZID — fall back to UTC.
				loc = time.UTC
			}
		}

		if isDate {
			return time.ParseInLocation("20060102", value, loc)
		}
		return time.ParseInLocation("20060102T150405", value, loc)
	}

	// No TZID — use the library's built-in parsing which handles Z suffix.
	if isDate {
		if strings.HasSuffix(value, "Z") {
			return time.ParseInLocation("20060102Z", value, time.UTC)
		}
		return time.ParseInLocation("20060102", value, time.Local)
	}

	if strings.HasSuffix(value, "Z") {
		return time.ParseInLocation("20060102T150405Z", value, time.UTC)
	}
	// Floating time (no Z, no TZID) — parsed in Local for now.
	// The mapper will apply the configured timezone fallback.
	return time.ParseInLocation("20060102T150405", value, time.Local)
}

// parseGeo parses a GEO property value in "lat;lon" format.
func parseGeo(value string) (float64, float64, error) {
	parts := strings.SplitN(value, ";", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("expected lat;lon format, got %q", value)
	}
	lat, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid latitude %q: %w", parts[0], err)
	}
	lon, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid longitude %q: %w", parts[1], err)
	}
	if lat < -90 || lat > 90 {
		return 0, 0, fmt.Errorf("latitude %f out of range [-90, 90]", lat)
	}
	if lon < -180 || lon > 180 {
		return 0, 0, fmt.Errorf("longitude %f out of range [-180, 180]", lon)
	}
	return lat, lon, nil
}

// parseISO8601Duration parses an ISO 8601 / RFC 5545 duration string.
// Supports: P1D, PT2H, PT30M, PT2H30M, P1DT12H, PT1H30M15S, etc.
func parseISO8601Duration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if len(s) == 0 || (s[0] != 'P' && s[0] != 'p' && s[0] != '-' && s[0] != '+') {
		return 0, fmt.Errorf("invalid duration %q: must start with P", s)
	}

	negative := false
	switch s[0] {
	case '-':
		negative = true
		s = s[1:]
	case '+':
		s = s[1:]
	}

	if len(s) == 0 || (s[0] != 'P' && s[0] != 'p') {
		return 0, fmt.Errorf("invalid duration %q: must start with P", s)
	}
	s = s[1:] // consume P

	if len(s) == 0 {
		return 0, fmt.Errorf("invalid duration: P with no components")
	}

	var total time.Duration
	inTime := false

	for len(s) > 0 {
		if s[0] == 'T' || s[0] == 't' {
			inTime = true
			s = s[1:]
			continue
		}

		// Parse numeric value.
		i := 0
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == 0 || i >= len(s) {
			return 0, fmt.Errorf("invalid duration component in %q", s)
		}
		n, err := strconv.Atoi(s[:i])
		if err != nil {
			return 0, fmt.Errorf("invalid duration number %q: %w", s[:i], err)
		}

		unit := s[i]
		s = s[i+1:]

		switch {
		case (unit == 'D' || unit == 'd') && !inTime:
			total += time.Duration(n) * 24 * time.Hour
		case (unit == 'W' || unit == 'w') && !inTime:
			total += time.Duration(n) * 7 * 24 * time.Hour
		case (unit == 'H' || unit == 'h') && inTime:
			total += time.Duration(n) * time.Hour
		case (unit == 'M' || unit == 'm') && inTime:
			total += time.Duration(n) * time.Minute
		case (unit == 'S' || unit == 's') && inTime:
			total += time.Duration(n) * time.Second
		default:
			return 0, fmt.Errorf("unknown duration unit %c (inTime=%v)", unit, inTime)
		}
	}

	if negative {
		total = -total
	}
	return total, nil
}

// handledProperties is the set of VEVENT property names that are already
// extracted into typed ParsedEvent fields. These are excluded from RawProps
// to avoid redundant storage.
var handledProperties = map[string]bool{
	string(ics.ComponentPropertySummary):      true,
	string(ics.ComponentPropertyDtStart):      true,
	string(ics.ComponentPropertyDtEnd):        true,
	string(ics.ComponentPropertyDuration):     true,
	string(ics.ComponentPropertyDescription):  true,
	string(ics.ComponentPropertyLocation):     true,
	string(ics.ComponentPropertyUrl):          true,
	string(ics.ComponentPropertyGeo):          true,
	string(ics.ComponentPropertyCategories):   true,
	string(ics.ComponentPropertyOrganizer):    true,
	string(ics.ComponentPropertyRrule):        true,
	string(ics.ComponentPropertyRecurrenceId): true,
	string(ics.ComponentPropertyCreated):      true,
	string(ics.ComponentPropertyLastModified): true,
	string(ics.ComponentPropertySequence):     true,
	string(ics.ComponentPropertyStatus):       true,
	string(ics.ComponentPropertyUniqueId):     true,
	string(ics.ComponentPropertyExdate):       true,
	string(ics.ComponentPropertyRdate):        true,
	// Standard boilerplate properties that add no diagnostic value.
	string(ics.ComponentPropertyDtstamp): true,
	string(ics.ComponentPropertyTransp):  true,
	string(ics.ComponentPropertyClass):   true,
}

// collectRawProps gathers VEVENT properties not already extracted into
// typed ParsedEvent fields. This preserves non-standard (X-*) and
// lesser-used properties for source provenance per SEL requirements.
// Returns nil (not empty map) when no extra properties exist.
func collectRawProps(ve *ics.VEvent) map[string]string {
	var props map[string]string
	for _, p := range ve.Properties {
		name := p.IANAToken
		if handledProperties[name] {
			continue
		}
		if props == nil {
			props = make(map[string]string)
		}
		// If the same property appears multiple times, keep the first.
		if _, exists := props[name]; !exists {
			props[name] = p.Value
		}
	}
	return props
}
