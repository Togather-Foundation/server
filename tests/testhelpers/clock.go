package testhelpers

import (
	"time"

	"github.com/Togather-Foundation/server/tests/testhelpers/clock"
)

// Tests must never depend on today's date. The implementation lives in the
// dependency-free tests/testhelpers/clock package so that in-package tests
// (package postgres, ...), which testhelpers itself imports and therefore
// cannot import back, can use the same helpers. See that package for details.
// Tests outside such packages should call these wrappers.

// FutureTime returns 19:00 UTC, days days after today. days must be >= 1.
func FutureTime(days int) time.Time { return clock.FutureTime(days) }

// FutureTimeAt is FutureTime with an explicit UTC time of day.
func FutureTimeAt(days, hour, minute int) time.Time { return clock.FutureTimeAt(days, hour, minute) }

// PastTime returns 19:00 UTC, days days before today. days must be >= 1.
func PastTime(days int) time.Time { return clock.PastTime(days) }

// PastTimeAt is PastTime with an explicit UTC time of day.
func PastTimeAt(days, hour, minute int) time.Time { return clock.PastTimeAt(days, hour, minute) }

// FutureDate is FutureTime(days) as a "2006-01-02" string.
func FutureDate(days int) string { return clock.FutureDate(days) }

// PastDate is PastTime(days) as a "2006-01-02" string.
func PastDate(days int) string { return clock.PastDate(days) }

// WinterDate is January (10+k) of next year as a "2006-01-02" string, 0 <= k <= 21.
// Use it when asserting fixed UTC offsets; see clock.WinterDate.
func WinterDate(k int) string { return clock.WinterDate(k) }

// WinterTime is hour:minute on WinterDate(k) in loc.
func WinterTime(k, hour, minute int, loc *time.Location) time.Time {
	return clock.WinterTime(k, hour, minute, loc)
}
