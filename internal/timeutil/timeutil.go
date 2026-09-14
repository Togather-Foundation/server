// Package timeutil provides shared helpers for rendering times in a target
// location, centralising the node-local civil time formatting used by the
// public API surface.
package timeutil

import (
	"strings"
	"time"
)

// RFC3339In renders t in loc using RFC3339 (with a DST-correct numeric offset).
// A nil loc falls back to UTC.
func RFC3339In(t time.Time, loc *time.Location) string {
	if loc == nil {
		loc = time.UTC
	}
	return t.In(loc).Format(time.RFC3339)
}

// RFC3339InPtr renders *t in loc, returning "" for a nil pointer.
func RFC3339InPtr(t *time.Time, loc *time.Location) string {
	if t == nil {
		return ""
	}
	return RFC3339In(*t, loc)
}

// DateIn renders t in loc as a date-only string (2006-01-02). A nil loc falls
// back to UTC so the local civil day is correct without panicking.
func DateIn(t time.Time, loc *time.Location) string {
	if loc == nil {
		loc = time.UTC
	}
	return t.In(loc).Format("2006-01-02")
}

// DateInPtr renders *t in loc as a date-only string, returning "" for a nil
// pointer.
func DateInPtr(t *time.Time, loc *time.Location) string {
	if t == nil {
		return ""
	}
	return DateIn(*t, loc)
}

// OccLoc resolves an occurrence's IANA timezone to a *time.Location, falling
// back to fallback when the zone is empty or unloadable. All-day occurrences
// are stored as local midnight in their own target zone, so callers must render
// them in that zone (not the node zone) to avoid off-by-one date shifts.
func OccLoc(timezone string, fallback *time.Location) *time.Location {
	if tz := strings.TrimSpace(timezone); tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			return loc
		}
	}
	return fallback
}
