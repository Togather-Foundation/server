package clock

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFutureTimeIsStrictlyFutureAtFixedHour(t *testing.T) {
	for _, days := range []int{1, 2, 30, 400} {
		got := FutureTime(days)
		require.True(t, got.After(time.Now()), "FutureTime(%d) = %s must be after now", days, got)
		require.Equal(t, time.UTC, got.Location())
		require.Equal(t, anchorHourUTC, got.Hour())
		require.Zero(t, got.Minute()+got.Second()+got.Nanosecond())
	}
}

func TestPastTimeIsStrictlyPastAtFixedHour(t *testing.T) {
	for _, days := range []int{1, 2, 30, 400} {
		got := PastTime(days)
		require.True(t, got.Before(time.Now()), "PastTime(%d) = %s must be before now", days, got)
		require.Equal(t, time.UTC, got.Location())
		require.Equal(t, anchorHourUTC, got.Hour())
	}
}

func TestTimeAtVariantsKeepRequestedClockTime(t *testing.T) {
	got := FutureTimeAt(5, 23, 30)
	require.Equal(t, 23, got.Hour())
	require.Equal(t, 30, got.Minute())
	require.Equal(t, FutureTime(5).Format(time.DateOnly), got.Format(time.DateOnly))

	past := PastTimeAt(5, 2, 15)
	require.Equal(t, 2, past.Hour())
	require.Equal(t, 15, past.Minute())
	require.Equal(t, PastTime(5).Format(time.DateOnly), past.Format(time.DateOnly))
	require.Panics(t, func() { FutureTimeAt(0, 23, 0) })
	require.Panics(t, func() { PastTimeAt(0, 23, 0) })
}

func TestHelpersAreCalendarDaysApart(t *testing.T) {
	require.Equal(t, 24*time.Hour, FutureTime(2).Sub(FutureTime(1)))
	require.Equal(t, FutureTime(1), FutureTime(1))
	require.Equal(t, FutureTime(3).Format(time.DateOnly), FutureDate(3))
	require.Equal(t, PastTime(3).Format(time.DateOnly), PastDate(3))
}

func TestHelpersRejectNonPositiveOffsets(t *testing.T) {
	require.Panics(t, func() { FutureTime(0) })
	require.Panics(t, func() { FutureTime(-1) })
	require.Panics(t, func() { PastTime(0) })
	require.Panics(t, func() { PastDate(-3) })
}

func TestWinterHelpersStayInFutureStandardTime(t *testing.T) {
	toronto, err := time.LoadLocation("America/Toronto")
	require.NoError(t, err)
	paris, err := time.LoadLocation("Europe/Paris")
	require.NoError(t, err)

	for _, k := range []int{0, 1, 11, 21} {
		got := WinterTime(k, 19, 0, toronto)
		require.True(t, got.After(time.Now()), "WinterTime(%d) must be in the future", k)
		require.Less(t, time.Until(got), 730*24*time.Hour, "must stay inside the ingest far-future cap")
		_, offset := got.Zone()
		require.Equal(t, -5*3600, offset, "Toronto must be on EST in January")
		_, offset = WinterTime(k, 12, 0, paris).Zone()
		require.Equal(t, 1*3600, offset, "Paris must be on CET in January")
		require.Equal(t, time.January, got.Month())
	}
	require.Equal(t, WinterTime(3, 0, 0, time.UTC).Format(time.DateOnly), WinterDate(3))
	require.Panics(t, func() { WinterDate(-1) })
	require.Panics(t, func() { WinterDate(22) })
}
