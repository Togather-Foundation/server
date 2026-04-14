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

// MapperOptions controls how parsed ICS events are converted to EventInputs.
type MapperOptions struct {
	SourceURL       string             // Feed URL for provenance (→ Source.URL)
	SourceName      string             // Source config name (→ Source.Name)
	TrustLevel      int                // Assigned trust level
	License         string             // Default "CC0-1.0"
	Timezone        string             // Fallback TZ for floating times (IANA name)
	DefaultLocation *events.PlaceInput // Fallback location when VEVENT LOCATION is empty
	HorizonDays     int                // RRULE expansion window (default: 90)
	MaxOccurrences  int                // Safety cap on expanded occurrences (default: 100)
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
			}
			occurrences, capped, err := ExpandRRule(ev.RRULE, ev.Start, ev.ExDates, ev.RDates, rrOpts)
			if err != nil {
				// Failed to parse RRULE — treat as single non-recurring event.
				warnings = append(warnings, fmt.Sprintf("RRULE expansion failed (UID: %s): %v", ev.UID, err))
				input, ws := mapSingleEvent(ev, ev.Start, ev.End, fallbackLoc, opts)
				warnings = append(warnings, ws...)
				if input != nil {
					results = append(results, *input)
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

			for _, occStart := range occurrences {
				occEnd := occStart.Add(duration)

				// Check if this occurrence has a RECURRENCE-ID exception.
				if exc, ok := exceptions[occStart.Unix()]; ok {
					// Use the exception's data instead.
					input, ws := mapSingleEvent(*exc, exc.Start, exc.End, fallbackLoc, opts)
					warnings = append(warnings, ws...)
					if input != nil {
						// Override Source.EventID with composite UID:startDate.
						input.Source.EventID = ev.UID + ":" + exc.Start.Format(time.RFC3339)
						results = append(results, *input)
					}
					continue
				}

				input, ws := mapSingleEvent(ev, occStart, occEnd, fallbackLoc, opts)
				warnings = append(warnings, ws...)
				if input != nil {
					// Composite Source.EventID for RRULE-expanded occurrences.
					input.Source.EventID = ev.UID + ":" + occStart.Format(time.RFC3339)
					results = append(results, *input)
				}
			}
		} else {
			// Non-recurring event.
			input, ws := mapSingleEvent(ev, ev.Start, ev.End, fallbackLoc, opts)
			warnings = append(warnings, ws...)
			if input != nil {
				results = append(results, *input)
			}
		}
	}

	return results, warnings, nil
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
	location := buildLocation(ev, opts.DefaultLocation)

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
		Name:        name,
		Description: description,
		StartDate:   startDate,
		EndDate:     endDate,
		URL:         eventURL,
		Location:    location,
		Organizer:   organizer,
		Keywords:    keywords,
		License:     opts.License,
		Source:      source,
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

// buildLocation constructs a PlaceInput from the parsed event.
// Falls back to DefaultLocation when LOCATION is empty.
func buildLocation(ev ParsedEvent, defaultLocation *events.PlaceInput) *events.PlaceInput {
	hasLocation := ev.Location != ""
	hasGeo := ev.HasGeo

	if !hasLocation && !hasGeo {
		// No location data in the event — use default.
		if defaultLocation != nil {
			// Return a copy to avoid mutation.
			loc := *defaultLocation
			return &loc
		}
		return nil
	}

	loc := &events.PlaceInput{}
	if hasLocation {
		loc.Name = sanitize.Text(ev.Location)
	}
	if hasGeo {
		loc.Latitude = ev.GeoLat
		loc.Longitude = ev.GeoLon
	}

	return loc
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
