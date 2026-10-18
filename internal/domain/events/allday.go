package events

import (
	"context"
	"fmt"
	"time"
)

// All-day invariant — one rule, one place.
//
// A stored occurrence row is date-only when, and only when:
//
//	is_all_day = true   AND   start_time is local midnight in the row's own
//	                          timezone (and end_time likewise, when present)
//
// and a genuinely timed occurrence keeps its instant and carries no marker.
//
// Ingestion has enforced this since the all-day/date-only work. Every other
// writer must route through NormalizeOccurrenceAllDay so the two halves of the
// pair are written together and cannot drift apart.
//
// The drift is not cosmetic. The wire layer renders an is_all_day row as a bare
// YYYY-MM-DD date, so a row holding 2026-11-05T19:00:00-05:00 with
// is_all_day = true is *read* as "2026-11-05" while ordering, dedupe, overlap
// and recurrence all see 19:00 — every "all-day rows are local midnight"
// assumption downstream becomes false.
//
// One anchoring authority. The local date and the local clock are derived by
// PostgreSQL (`start_time AT TIME ZONE timezone`): the generated columns
// local_date/local_start_time use exactly that expression. Go must therefore
// never turn a civil date into an instant with its own tzdata — the host Go
// tzdata and the database image's tzdata can disagree on a zone's offset (for
// example America/Vancouver after BC's permanent-time rule), and then a row
// Go calls "midnight" reads back as 23:00 or 01:00 and the civil date drifts.
//
// The split is:
//   - Go determines the civil date a write intends: the date the input value
//     carries in its own offset (the same rule as ingest).
//   - AllDayAnchorer (the Postgres repository) turns (date, zone) into the
//     stored instant, using the same tzdata as the generated columns.
//
// A read that has to re-anchor an existing row takes the civil date from the
// database (`Occurrence.LocalDate` / `LocalEndDate`, both computed with
// AT TIME ZONE), never from `instant.In(hostLocation)`.
type AllDayAnchorer interface {
	// AnchorAllDayOccurrence returns the instants PostgreSQL computes for the
	// civil dates carried by start and end (time-of-day is ignored) in the
	// given zone. It is the single authority for all-day anchoring.
	AnchorAllDayOccurrence(ctx context.Context, start time.Time, end *time.Time, timezone string) (time.Time, *time.Time, error)
}

// NormalizeOccurrenceAllDay returns the canonical (start, end, error) tuple
// for a single occurrence write:
//
//   - allDay true  → start and end are anchored to local midnight in the
//     occurrence's zone by anchorer, for the civil date each value carries —
//     the same date rule ingestion applies to startDate, so one wire value
//     anchors to one date through either entry point;
//   - allDay false → the instants are returned verbatim and the marker is
//     cleared.
//
// Callers MUST write all three returned values. Writing a requested allDay flag
// next to un-normalised instants is exactly the bug this function exists to
// prevent; so is leaving is_all_day = true on a row whose instant was just
// retimed off local midnight.
func NormalizeOccurrenceAllDay(ctx context.Context, anchorer AllDayAnchorer, start time.Time, end *time.Time, allDay bool, timezone string) (time.Time, *time.Time, error) {
	if !allDay {
		return start, end, nil
	}
	if anchorer == nil {
		return time.Time{}, nil, fmt.Errorf("normalize all-day occurrence: no anchorer")
	}

	nStart, nEnd, err := anchorer.AnchorAllDayOccurrence(ctx, start, end, timezone)
	if err != nil {
		return time.Time{}, nil, fmt.Errorf("anchor all-day occurrence: %w", err)
	}
	return nStart, nEnd, nil
}

// civilDateUTC returns the civil date carried by t, as midnight UTC. It reads
// t's own wall-clock date (its own offset/zone, or the wall fields the caller
// constructed), never the host's tzdata: that is the date the value carries.
// Use it only to pass a civil date to the anchoring authority — never to turn a
// stored instant back into a date.
func civilDateUTC(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// occurrenceCivilDate returns the database-computed civil date of a stored
// occurrence edge. The fallback exists only for in-memory values that never
// came from a row (tests, hand-built fixtures); the production read paths
// always populate LocalDate/LocalEndDate from the DB.
func occurrenceCivilDate(local time.Time, fallback time.Time) time.Time {
	if !local.IsZero() {
		return local
	}
	return civilDateUTC(fallback)
}

// occurrenceCivilDatePtr is occurrenceCivilDate for an optional edge.
func occurrenceCivilDatePtr(local *time.Time, fallback time.Time) *time.Time {
	if local != nil {
		d := *local
		return &d
	}
	d := civilDateUTC(fallback)
	return &d
}

// occurrenceEndBeforeStart compares an occurrence window. For a date-only row
// it compares the civil dates the values carry: two midnights of the same civil
// date can be different instants when one value carries a positive offset.
func occurrenceEndBeforeStart(start, end time.Time, allDay bool) bool {
	if allDay {
		return civilDateUTC(end).Before(civilDateUTC(start))
	}
	return end.Before(start)
}

// isLocalMidnight reports whether t is exactly midnight in loc. It is used for
// the admin retime heuristic (is the *requested* value still a local
// midnight?), not to anchor a write. All-day anchoring itself is done by
// AllDayAnchorer.
func isLocalMidnight(t time.Time, loc *time.Location) bool {
	if loc == nil {
		loc = time.UTC
	}
	local := t.In(loc)
	if local.Nanosecond() != 0 {
		return false
	}
	h, m, s := local.Clock()
	return h == 0 && m == 0 && s == 0
}

// sameInstant reports whether two optional instants denote the same point in
// time. Two nils are the same; one nil and one set are not.
func sameInstant(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

// occurrenceZone resolves the timezone an occurrence row is anchored in: the
// row's own Timezone column is authoritative, with the request's value and the
// service default as fallbacks (empty or unloadable names fall back to UTC, as
// in ingest).
func occurrenceZone(requestTZ string, rowTZ string, defaultTZ string) *time.Location {
	zone := requestTZ
	if zone == "" {
		zone = rowTZ
	}
	if zone == "" {
		zone = defaultTZ
	}
	return locationOrUTC(zone)
}

// occurrenceZoneName is occurrenceZone as the IANA name the anchoring authority
// needs. The resolved location's name is used so an empty or unloadable input
// becomes "UTC" consistently on both sides.
func occurrenceZoneName(requestTZ string, rowTZ string, defaultTZ string) string {
	return occurrenceZone(requestTZ, rowTZ, defaultTZ).String()
}
