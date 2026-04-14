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

type SerializeOptions struct {
	CalendarName        string
	CalendarDescription string
}

type SerializeResult struct {
	Data     []byte
	Warnings []string
}

func SerializeEvents(evts []events.Event, opts SerializeOptions) (SerializeResult, error) {
	return serializeEventsCore(evts, opts)
}

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
