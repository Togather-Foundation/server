package clock

import "time"

// Tests must never depend on today's date. Several code paths read the wall
// clock (the events list defaults to startDate=today, ingest and the review
// queue treat already-started events differently, stats and feeds split
// past/upcoming), so a hardcoded date that is "tomorrow" when the test is
// written silently becomes "last month" and the test rots. Anchor every
// date that reaches the system under test to the helpers below instead.
//
// All helpers return the same fixed time of day (19:00 UTC) so a test never
// straddles a midnight or DST boundary by accident, and so two calls with the
// same offset are equal.

// anchorHourUTC is the time of day returned by every helper in this file.
const anchorHourUTC = 19

// FutureTime returns 19:00 UTC on the day that is days days after today (UTC).
// days must be >= 1: FutureTime(1) is always strictly in the future, whatever
// time of day the test runs. FutureTime(0) is not guaranteed to be.
func FutureTime(days int) time.Time {
	return FutureTimeAt(days, anchorHourUTC, 0)
}

// FutureTimeAt is FutureTime with an explicit UTC time of day, for tests whose
// behaviour depends on the clock time (for example end-before-start heuristics
// that compare 23:00 with 02:00).
func FutureTimeAt(days, hour, minute int) time.Time {
	if days < 1 {
		panic("clock.FutureTime: days must be >= 1 (use PastTime for past dates)")
	}
	return anchorTime(days, hour, minute)
}

// PastTime returns 19:00 UTC on the day that is days days before today (UTC).
// days must be >= 1: PastTime(1) is always strictly in the past.
func PastTime(days int) time.Time {
	return PastTimeAt(days, anchorHourUTC, 0)
}

// PastTimeAt is PastTime with an explicit UTC time of day.
func PastTimeAt(days, hour, minute int) time.Time {
	if days < 1 {
		panic("clock.PastTime: days must be >= 1 (use FutureTime for future dates)")
	}
	return anchorTime(-days, hour, minute)
}

// FutureDate is FutureTime(days) formatted as a date-only "2006-01-02" string,
// for query parameters such as startDate/endDate.
func FutureDate(days int) string {
	return FutureTime(days).Format(time.DateOnly)
}

// PastDate is PastTime(days) formatted as a date-only "2006-01-02" string.
func PastDate(days int) string {
	return PastTime(days).Format(time.DateOnly)
}

func anchorTime(offsetDays, hour, minute int) time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day()+offsetDays, hour, minute, 0, 0, time.UTC)
}

// winterYear is fixed at process start so every call in one test run agrees.
var winterYear = time.Now().UTC().Year() + 1

// maxWinterOffset keeps WinterDate/WinterTime inside January.
const maxWinterOffset = 21

// WinterDate returns the date-only string for January (10+k) of next calendar
// year, with 0 <= k <= 21.
//
// Use it when a test asserts a fixed UTC offset (for example "-05:00" for
// America/Toronto or "+01:00" for Europe/Paris). January is standard time in
// every northern-hemisphere zone, so the offset stays valid whatever day the
// test runs on, and the date is always in the future (never more than about 13
// months out, well inside the 730-day ingest cap). FutureTime is not suitable
// there: depending on the run date it can land in daylight-saving time.
func WinterDate(k int) string {
	return WinterTime(k, 0, 0, time.UTC).Format(time.DateOnly)
}

// WinterTime is the instant at hour:minute on WinterDate(k) in loc.
func WinterTime(k, hour, minute int, loc *time.Location) time.Time {
	if k < 0 || k > maxWinterOffset {
		panic("clock.WinterTime: k must be within 0..21 so the date stays in January")
	}
	return time.Date(winterYear, time.January, 10+k, hour, minute, 0, 0, loc)
}
