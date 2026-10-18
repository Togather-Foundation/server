package events

import (
	"context"
	"time"
)

// anchorAllDayInZone is the in-memory stand-in for the Postgres anchoring
// authority used by hand-built fixtures and mocks. Production always uses
// EventRepository.AnchorAllDayOccurrence (AT TIME ZONE), the same expression
// the generated columns use; tests that pin the authority use a fake anchorer
// with an explicit location instead.
func anchorAllDayInZone(start time.Time, end *time.Time, timezone string) (time.Time, *time.Time, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
	}
	y, m, d := start.Date()
	startOut := time.Date(y, m, d, 0, 0, 0, 0, loc)

	var endOut *time.Time
	if end != nil {
		ey, em, ed := end.Date()
		e := time.Date(ey, em, ed, 0, 0, 0, 0, loc)
		endOut = &e
	}
	return startOut, endOut, nil
}

// fakeAllDayAnchorer is a deterministic stand-in for the database's tzdata. It
// lets a test make the "host Go" and "database" offsets disagree.
type fakeAllDayAnchorer struct {
	loc *time.Location
}

func (f fakeAllDayAnchorer) AnchorAllDayOccurrence(_ context.Context, start time.Time, end *time.Time, timezone string) (time.Time, *time.Time, error) {
	loc := f.loc
	if loc == nil {
		var err error
		loc, err = time.LoadLocation(timezone)
		if err != nil {
			loc = time.UTC
		}
	}
	y, m, d := start.Date()
	startOut := time.Date(y, m, d, 0, 0, 0, 0, loc)
	var endOut *time.Time
	if end != nil {
		ey, em, ed := end.Date()
		e := time.Date(ey, em, ed, 0, 0, 0, 0, loc)
		endOut = &e
	}
	return startOut, endOut, nil
}
