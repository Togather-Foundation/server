package events

import "time"

// All-day invariant — one rule, one place.
//
// A stored occurrence row is date-only when, and only when:
//
//	is_all_day = true   AND   start_time is local midnight in the row's own
//	                          timezone (and end_time likewise, when present)
//
// and a genuinely timed occurrence keeps its instant and carries no marker.
//
// Ingestion has enforced this since the all-day/date-only work (see
// parseAllDayDate in ingest.go). Every other writer must route through
// NormalizeOccurrenceAllDay so the two halves of the pair are written together
// and cannot drift apart.
//
// The drift is not cosmetic. The wire layer renders an is_all_day row as a bare
// YYYY-MM-DD date, so a row holding 2026-11-05T19:00:00-05:00 with
// is_all_day = true is *read* as "2026-11-05" while ordering, dedupe, overlap
// and recurrence all see 19:00 — every "all-day rows are local midnight"
// assumption downstream becomes false.
//
// NormalizeOccurrenceAllDay returns the canonical (start, end, error) tuple
// for a single occurrence write:
//
//   - allDay true  → start and end are re-anchored to local midnight of their
//     civil date in loc (the occurrence's own zone);
//   - allDay false → the instants are returned verbatim and the marker is
//     cleared.
//
// Callers MUST write all three returned values. Writing a requested allDay flag
// next to un-normalised instants is exactly the bug this function exists to
// prevent; so is leaving is_all_day = true on a row whose instant was just
// retimed off local midnight.
func NormalizeOccurrenceAllDay(start time.Time, end *time.Time, allDay bool, loc *time.Location) (time.Time, *time.Time, error) {
	if !allDay {
		return start, end, nil
	}

	nStart, err := anchorAllDayInstant(start, loc)
	if err != nil {
		return time.Time{}, nil, err
	}

	var nEnd *time.Time
	if end != nil {
		e, err := anchorAllDayInstant(*end, loc)
		if err != nil {
			return time.Time{}, nil, err
		}
		nEnd = &e
	}

	return nStart, nEnd, nil
}

// anchorAllDayInstant delegates the anchoring rule to parseAllDayDate — the
// same function ingestion uses — so exactly one implementation decides "which
// civil date, and midnight in which zone".
//
// The civil date is the date in the occurrence's own (target) zone, not in the
// value's own offset. A DB-read instant carries the server's local zone (pgx
// ScanLocation is nil → time.Unix → time.Local), so formatting it verbatim
// would take the civil date a day early for any row east of the server zone.
// We therefore re-express the instant in loc first (nil → UTC), then
// round-trip through RFC3339 so parseAllDayDate picks the date in that zone and
// re-anchors to midnight there. This is idempotent for canonical rows (already
// midnight in loc) and correct for client instants interpreted in the
// occurrence zone.
func anchorAllDayInstant(t time.Time, loc *time.Location) (time.Time, error) {
	if loc == nil {
		loc = time.UTC
	}
	return parseAllDayDate(t.In(loc).Format(time.RFC3339), loc)
}

// isLocalMidnight reports whether t is exactly midnight in loc. It is the
// "agrees with the instant" half of the invariant: a row may only carry
// is_all_day = true when this holds for its start_time.
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
