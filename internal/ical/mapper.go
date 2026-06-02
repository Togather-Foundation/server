package ical

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/sanitize"
)

// deriveSeriesEnd parses the UNTIL value from an RRULE string and returns the
// date it represents. Returns nil if UNTIL is absent (COUNT-only or infinite
// series). The RRULE value string may or may not start with "RRULE:"; both
// forms are accepted.
func deriveSeriesEnd(rruleStr string) *time.Time {
	if rruleStr == "" {
		return nil
	}

	val := rruleStr
	if strings.HasPrefix(strings.ToUpper(val), "RRULE:") {
		val = val[len("RRULE:"):]
	}

	for _, part := range strings.Split(val, ";") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		if strings.EqualFold(kv[0], "UNTIL") {
			// UNTIL may be DATE or DATE-TIME. Try DATE-TIME first, then DATE.
			formats := []string{
				"20060102T150405Z",
				"20060102T150405",
				"20060102Z",
				"20060102",
				time.RFC3339,
			}
			for _, f := range formats {
				if t, err := time.Parse(f, kv[1]); err == nil {
					return &t
				}
			}
			// Could not parse UNTIL — treat as nil (honest over guessing).
			return nil
		}
	}
	return nil
}

// MapperOptions controls how parsed ICS events are converted to EventInputs.
type MapperOptions struct {
	SourceURL       string             // Feed URL for provenance (→ Source.URL)
	SourceName      string             // Source config name (→ Source.Name)
	TrustLevel      int                // Assigned trust level
	License         string             // Default "CC0-1.0"
	Timezone        string             // Fallback TZ for floating times (IANA name)
	CountryCode     string             // Default country code for address decomposition (default: "CA")
	DefaultLocation *events.PlaceInput // Fallback location when VEVENT LOCATION is empty
	HorizonDays     int                // RRULE expansion window (default: 90)
	MaxOccurrences  int                // Safety cap on expanded occurrences (default: 100)
	// Now is the reference time used to filter out past events. If zero,
	// time.Now() is called once at the start of MapToEventInputs.
	Now time.Time
}

// MapToEventInputs converts parsed ICS events to SEL EventInputs.
//
// Non-recurring events produce a single EventInput.
// Recurring events (RRULE present) are expanded via ExpandRRule into
// multiple EventInputs, one per occurrence within the horizon window.
// Each expanded occurrence has its own startDate/endDate (preserving
// original duration) and shares all other fields.
//
// ctx is used for cancellation during large RRULE expansions.
//
// Returns the EventInputs and any warnings (e.g., RRULE cap hit).
func MapToEventInputs(ctx context.Context, cal *ParsedCalendar, opts MapperOptions) ([]events.EventInput, []string, error) {
	if opts.License == "" {
		opts.License = "CC0-1.0"
	}
	horizonDays := opts.HorizonDays
	if horizonDays <= 0 {
		horizonDays = DefaultHorizonDays
	}
	maxOcc := opts.MaxOccurrences
	if maxOcc <= 0 {
		maxOcc = DefaultMaxOccurrences
	}

	// Load fallback timezone.
	fallbackLoc := time.UTC
	if opts.Timezone != "" {
		loc, err := time.LoadLocation(opts.Timezone)
		if err == nil {
			fallbackLoc = loc
		}
	}

	// Index RECURRENCE-ID exceptions by their RecurrenceID time for merging
	// into RRULE-expanded occurrences.
	exceptions := make(map[int64]*ParsedEvent) // unix timestamp → exception event
	var masterEvents []ParsedEvent
	for i := range cal.Events {
		ev := &cal.Events[i]
		if !ev.RecurrenceID.IsZero() {
			exceptions[ev.RecurrenceID.Unix()] = ev
		} else {
			masterEvents = append(masterEvents, *ev)
		}
	}

	var results []events.EventInput
	var warnings []string
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}

	for _, ev := range masterEvents {
		select {
		case <-ctx.Done():
			return results, warnings, ctx.Err()
		default:
		}

		// Skip cancelled events.
		if ev.Status == "CANCELLED" {
			slog.Debug("skipping cancelled event", "uid", ev.UID)
			continue
		}

		if ev.RRULE != "" {
			// Recurring event: expand RRULE.
			rrOpts := RRuleOptions{
				HorizonDays:    horizonDays,
				MaxOccurrences: maxOcc,
				Now:            now,
			}
			occurrences, capped, err := ExpandRRule(ev.RRULE, ev.Start, ev.ExDates, ev.RDates, rrOpts)
			if err != nil {
				// Failed to parse RRULE — treat as single non-recurring event.
				// Still apply past-event filter before adding.
				warnings = append(warnings, fmt.Sprintf("RRULE expansion failed (UID: %s): %v", ev.UID, err))
				if !isOccurrencePast(ev.Start, ev.End, now) {
					input, ws := mapSingleEvent(ev, ev.Start, ev.End, fallbackLoc, opts)
					warnings = append(warnings, ws...)
					if input != nil {
						results = append(results, *input)
					}
				}
				continue
			}

			if len(occurrences) == 0 {
				// All occurrences in the past or excluded.
				continue
			}

			if capped {
				warnings = append(warnings, fmt.Sprintf("RRULE capped at %d occurrences (UID: %s)", maxOcc, ev.UID))
			}

			duration := ev.End.Sub(ev.Start)

			// Build shared RecurrenceInput for all occurrences of this master.
			seriesStartLocal := ev.Start
			if loc := ev.Start.Location(); loc != nil && loc != time.UTC {
				seriesStartLocal = ev.Start.In(loc)
			}
			seriesStartLocal = seriesStartLocal.Truncate(24 * time.Hour)

			seriesEnd := deriveSeriesEnd(ev.RRULE)

			rruleValue := ev.RRULE
			if strings.HasPrefix(strings.ToUpper(rruleValue), "RRULE:") {
				rruleValue = rruleValue[len("RRULE:"):]
			}

			tzid := ev.TZID
			if tzid == "" {
				tzid = opts.Timezone
			}
			if tzid == "" {
				tzid = "UTC"
			}

			recurrence := &events.RecurrenceInput{
				ExternalKey: opts.SourceName + ":" + ev.UID,
				SeriesName:  sanitize.Text(ev.Summary),
				SeriesStart: seriesStartLocal,
				SeriesEnd:   seriesEnd,
				RRule:       rruleValue,
				ExDates:     ev.ExDates,
				RDates:      ev.RDates,
				TZID:        tzid,
			}

			for _, occStart := range occurrences {
				occEnd := occStart.Add(duration)

				// Skip occurrences that have already ended. ExpandRRule's
				// windowStart already excludes most past occurrences, but this
				// guard also covers zero-duration events and ensures a consistent
				// cutoff when opts.Now is injected.
				if isOccurrencePast(occStart, occEnd, now) {
					continue
				}

				// Check if this occurrence has a RECURRENCE-ID exception.
				if exc, ok := exceptions[occStart.Unix()]; ok {
					// The exception may reschedule to a different (potentially
					// past) time — apply the filter against the exception's
					// actual start/end, not the original occurrence slot.
					if isOccurrencePast(exc.Start, exc.End, now) {
						continue
					}
					// Use the exception's data instead.
					input, ws := mapSingleEvent(*exc, exc.Start, exc.End, fallbackLoc, opts)
					warnings = append(warnings, ws...)
					if input != nil {
						// Override Source.EventID with composite UID:startDate.
						input.Source.EventID = ev.UID + ":" + exc.Start.Format(time.RFC3339)
						input.Recurrence = recurrence
						results = append(results, *input)
					}
					continue
				}

				input, ws := mapSingleEvent(ev, occStart, occEnd, fallbackLoc, opts)
				warnings = append(warnings, ws...)
				if input != nil {
					// Composite Source.EventID for RRULE-expanded occurrences.
					input.Source.EventID = ev.UID + ":" + occStart.Format(time.RFC3339)
					input.Recurrence = recurrence
					results = append(results, *input)
				}
			}
		} else {
			// Non-recurring event: skip if it has already ended.
			if isOccurrencePast(ev.Start, ev.End, now) {
				slog.Debug("skipping past event", "uid", ev.UID, "start", ev.Start)
				continue
			}
			input, ws := mapSingleEvent(ev, ev.Start, ev.End, fallbackLoc, opts)
			warnings = append(warnings, ws...)
			if input != nil {
				results = append(results, *input)
			}
		}
	}

	return results, warnings, nil
}

// isOccurrencePast reports whether an event occurrence has already ended
// relative to the provided now snapshot. If endTime is zero or not after
// startTime (zero-duration, malformed, or all-day with no explicit end),
// only startTime is used. An event ending exactly at now is considered past
// (it has just ended).
func isOccurrencePast(startTime, endTime time.Time, now time.Time) bool {
	if !endTime.IsZero() && endTime.After(startTime) {
		return !endTime.After(now)
	}
	return !startTime.After(now)
}

// mapSingleEvent maps a single ParsedEvent (or occurrence) to an EventInput.
// startTime and endTime may differ from ev.Start/ev.End for RRULE-expanded occurrences.
func mapSingleEvent(ev ParsedEvent, startTime, endTime time.Time, fallbackLoc *time.Location, opts MapperOptions) (*events.EventInput, []string) {
	var warnings []string

	// Resolve floating times using the fallback timezone.
	startTime = resolveFloatingTime(startTime, fallbackLoc)
	endTime = resolveFloatingTime(endTime, fallbackLoc)

	// Format as RFC 3339.
	startDate := startTime.Format(time.RFC3339)
	endDate := endTime.Format(time.RFC3339)

	// Sanitize name and description.
	name := sanitize.Text(ev.Summary)
	description := sanitize.Text(ev.Description)

	// URL validation and normalization.
	eventURL := normalizeURL(ev.URL, &warnings, ev.UID)

	// Location.
	location, virtualLoc := buildLocation(ev, opts)

	// Organizer.
	organizer := buildOrganizer(ev)

	// Keywords from CATEGORIES.
	var keywords []string
	for _, cat := range ev.Categories {
		cat = strings.TrimSpace(sanitize.Text(cat))
		if cat != "" {
			keywords = append(keywords, cat)
		}
	}

	// Source provenance.
	source := &events.SourceInput{
		URL:     opts.SourceURL,
		EventID: ev.UID,
		Name:    opts.SourceName,
		License: opts.License,
	}

	return &events.EventInput{
		Name:            name,
		Description:     description,
		StartDate:       startDate,
		EndDate:         endDate,
		URL:             eventURL,
		Location:        location,
		VirtualLocation: virtualLoc,
		Organizer:       organizer,
		Keywords:        keywords,
		License:         opts.License,
		Source:          source,
	}, warnings
}

// resolveFloatingTime converts a floating time (in time.Local) to the
// given fallback location. Times already in a specific timezone (UTC or
// a named location) are left as-is.
func resolveFloatingTime(t time.Time, fallback *time.Location) time.Time {
	if t.Location() == time.Local && fallback != time.Local {
		// Re-interpret the wall-clock reading in the fallback timezone.
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(),
			t.Second(), t.Nanosecond(), fallback)
	}
	return t
}

// normalizeURL validates and normalizes an event URL.
// webcal:// is converted to https://. Non-http(s) schemes are rejected.
func normalizeURL(rawURL string, warnings *[]string, uid string) string {
	if rawURL == "" {
		return ""
	}

	// Normalize webcal:// to https://.
	if strings.HasPrefix(strings.ToLower(rawURL), "webcal://") {
		rawURL = "https://" + rawURL[len("webcal://"):]
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("invalid URL (UID: %s): %v", uid, err))
		return ""
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		*warnings = append(*warnings, fmt.Sprintf("non-http(s) URL scheme %q skipped (UID: %s)", parsed.Scheme, uid))
		return ""
	}

	return rawURL
}

// buildLocation constructs a PlaceInput and/or VirtualLocationInput from the
// parsed event. Resolution order:
//  1. ev.Location present → PlaceInput (with Geo if also present)
//  2. ev.HasGeo only (no name) → PlaceInput with coordinates only
//  3. ev.Description → check virtual signals first, then location patterns
//  4. opts.DefaultLocation fallback → copy of PlaceInput
//  5. nil, nil (reject)
func buildLocation(ev ParsedEvent, opts MapperOptions) (*events.PlaceInput, *events.VirtualLocationInput) {
	if ev.Location != "" {
		loc := &events.PlaceInput{Name: sanitize.Text(ev.Location)}
		if ev.HasGeo {
			loc.Latitude = ev.GeoLat
			loc.Longitude = ev.GeoLon
		}
		return loc, nil
	}

	if ev.HasGeo {
		return &events.PlaceInput{Latitude: ev.GeoLat, Longitude: ev.GeoLon}, nil
	}

	if ev.Description != "" {
		if IsVirtualDescription(ev.Description) {
			return nil, &events.VirtualLocationInput{URL: ev.URL}
		}
		if extracted, ok := ExtractLocationFromDescription(ev.Description); ok {
			var decomposeOpts DecomposeOpts
			if opts.DefaultLocation != nil {
				decomposeOpts.DefaultLocality = opts.DefaultLocation.AddressLocality
				decomposeOpts.DefaultRegion = opts.DefaultLocation.AddressRegion
				decomposeOpts.DefaultCountry = opts.DefaultLocation.AddressCountry
			}
			if decomposeOpts.DefaultCountry == "" {
				decomposeOpts.DefaultCountry = strings.ToUpper(opts.CountryCode)
			}
			loc := DecomposeLocation(extracted, decomposeOpts)
			return &loc, nil
		}
	}

	if opts.DefaultLocation != nil {
		loc := *opts.DefaultLocation
		return &loc, nil
	}

	return nil, nil
}

// buildOrganizer constructs an OrganizationInput from the parsed event.
func buildOrganizer(ev ParsedEvent) *events.OrganizationInput {
	if ev.Organizer == "" && ev.OrganizerEmail == "" {
		return nil
	}

	name := ev.Organizer
	if name == "" && ev.OrganizerEmail != "" {
		// Fall back to email local-part as name.
		if idx := strings.Index(ev.OrganizerEmail, "@"); idx > 0 {
			name = ev.OrganizerEmail[:idx]
		}
	}

	return &events.OrganizationInput{
		Name:  sanitize.Text(name),
		Email: ev.OrganizerEmail,
	}
}
