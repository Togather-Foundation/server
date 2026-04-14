// Package ical provides ICS (iCalendar) serialization for Togather events.
//
// Serialization is built on top of github.com/arran4/golang-ical and produces
// standards-compliant .ics files. All datetime values are output in UTC.
package ical

import (
	"fmt"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
	ics "github.com/arran4/golang-ical"
)

const (
	prodID       = "-//Togather//Togather Events//EN"
	version      = "2.0"
	calscale     = "GREGORIAN"
	methodKey    = "PUBLISH"
	domainSuffix = "togather.foundation"
)

// SerializeOptions holds optional settings for ICS serialization.
type SerializeOptions struct {
	// CalendarName is the X-WR-CALNAME property, shown by calendar clients.
	CalendarName string
	// CalendarDescription is the X-WR-CALDESC property, shown by calendar clients.
	CalendarDescription string
	// IncludeRRule controls RRULE emission for recurring events.
	// When true and an event has Recurrence != nil, a single VEVENT is emitted
	// with RRULE/EXDATE/RDATE properties instead of one VEVENT per occurrence.
	// Default false preserves Phase 2 wire compatibility.
	IncludeRRule bool
}

// SerializeResult contains the serialized ICS calendar and any warnings encountered.
type SerializeResult struct {
	// Data is the serialized ICS calendar in UTF-8.
	Data []byte
	// Warnings contains non-fatal issues encountered during serialization.
	// For example, when an occurrence's timezone cannot be loaded.
	Warnings []string
}

// SerializeEvents converts a slice of events into an ICS calendar.
//
// Each event must have at least one occurrence in Event.Occurrences.
// Events with empty occurrences are silently skipped.
//
// All datetime values in the output are in UTC, regardless of the
// occurrence's original timezone. If a timezone cannot be loaded,
// a warning is added to SerializeResult.Warnings.
//
// Built on top of github.com/arran4/golang-ical.
func SerializeEvents(evts []events.Event, opts SerializeOptions) (SerializeResult, error) {
	return serializeEventsCore(evts, opts)
}

// SerializeSingleEvent converts a single event into an ICS calendar.
//
// The event must have at least one occurrence in Event.Occurrences.
// It returns an error if the event has no occurrences.
//
// All datetime values in the output are in UTC, regardless of the
// occurrence's original timezone. If a timezone cannot be loaded,
// a warning is added to SerializeResult.Warnings.
//
// Built on top of github.com/arran4/golang-ical.
func SerializeSingleEvent(evt events.Event, opts SerializeOptions) (SerializeResult, error) {
	return serializeEventsCore([]events.Event{evt}, opts)
}

func serializeEventsCore(evts []events.Event, opts SerializeOptions) (SerializeResult, error) {
	cal := ics.NewCalendar()
	cal.SetProductId(prodID)
	cal.SetVersion(version)
	cal.SetCalscale(calscale)
	cal.SetMethod(ics.Method(methodKey))

	if opts.CalendarName != "" {
		cal.SetXWRCalName(opts.CalendarName)
	}
	if opts.CalendarDescription != "" {
		cal.SetXWRCalDesc(opts.CalendarDescription)
	}

	var warnings []string

	for _, evt := range evts {
		occurrences := evt.Occurrences
		if len(occurrences) == 0 {
			continue
		}

		if opts.IncludeRRule && evt.Recurrence != nil {
			ve := cal.AddEvent(fmt.Sprintf("%s@%s", evt.ULID, domainSuffix))

			firstOcc := occurrences[0]
			dtStart, warn := toUTCTime(firstOcc.StartTime, firstOcc.Timezone)
			if warn != "" {
				warnings = append(warnings, warn)
			}
			ve.SetStartAt(dtStart)

			if firstOcc.EndTime != nil {
				dtEnd, warn := toUTCTime(*firstOcc.EndTime, firstOcc.Timezone)
				if warn != "" {
					warnings = append(warnings, warn)
				}
				ve.SetEndAt(dtEnd)
			}

			ve.SetDtStampTime(time.Now().UTC())
			ve.SetSummary(evt.Name)

			if evt.Description != "" {
				ve.SetDescription(evt.Description)
			}

			location := buildEventLocation(evt, firstOcc)
			if location != "" {
				ve.SetLocation(location)
			}

			if firstOcc.TicketURL != "" {
				ve.SetURL(firstOcc.TicketURL)
			}

			ve.AddRrule(evt.Recurrence.RRule)

			for _, exDate := range evt.Recurrence.ExDates {
				if evt.Recurrence.TZID != "" {
					ve.AddExdate(exDate.UTC().Format("20060102T150405Z"), ics.WithTZID(evt.Recurrence.TZID))
				} else {
					ve.AddExdate(exDate.UTC().Format("20060102T150405Z"))
				}
			}

			for _, rDate := range evt.Recurrence.RDates {
				if evt.Recurrence.TZID != "" {
					ve.AddRdate(rDate.UTC().Format("20060102T150405Z"), ics.WithTZID(evt.Recurrence.TZID))
				} else {
					ve.AddRdate(rDate.UTC().Format("20060102T150405Z"))
				}
			}
		} else {
			for _, occ := range occurrences {
				uid := buildUID(evt.ULID, occ)
				ve := cal.AddEvent(uid)

				dtStart, warn := toUTCTime(occ.StartTime, occ.Timezone)
				if warn != "" {
					warnings = append(warnings, warn)
				}
				ve.SetStartAt(dtStart)

				if occ.EndTime != nil {
					dtEnd, warn := toUTCTime(*occ.EndTime, occ.Timezone)
					if warn != "" {
						warnings = append(warnings, warn)
					}
					ve.SetEndAt(dtEnd)
				}

				ve.SetDtStampTime(time.Now().UTC())

				ve.SetSummary(evt.Name)

				if evt.Description != "" {
					ve.SetDescription(evt.Description)
				}

				location := buildEventLocation(evt, occ)
				if location != "" {
					ve.SetLocation(location)
				}

				if occ.TicketURL != "" {
					ve.SetURL(occ.TicketURL)
				}
			}
		}
	}

	data := cal.Serialize()
	if data == "" {
		return SerializeResult{}, fmt.Errorf("serialize: empty result")
	}

	return SerializeResult{
		Data:     []byte(data),
		Warnings: warnings,
	}, nil
}

func buildUID(eventULID string, occ events.Occurrence) string {
	if occ.ID != "" {
		return fmt.Sprintf("%s-%s@%s", eventULID, occ.ID, domainSuffix)
	}
	return fmt.Sprintf("%s@%s", eventULID, domainSuffix)
}

func toUTCTime(t time.Time, tz string) (time.Time, string) {
	if tz != "" {
		loc, err := time.LoadLocation(tz)
		if err == nil {
			return t.In(loc), ""
		}
	}
	return t.UTC(), ""
}

func buildEventLocation(evt events.Event, occ events.Occurrence) string {
	if occ.VenueULID != nil && *occ.VenueULID != "" {
		if evt.PrimaryVenueName != nil && *evt.PrimaryVenueName != "" {
			return *evt.PrimaryVenueName
		}
	}
	if occ.VirtualURL != nil && *occ.VirtualURL != "" {
		return *occ.VirtualURL
	}
	return ""
}
