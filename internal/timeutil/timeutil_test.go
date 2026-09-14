package timeutil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func toronto(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Toronto")
	require.NoError(t, err, "America/Toronto must be loadable (tzdata required)")
	return loc
}

func mustParse(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	require.NoError(t, err, "failed to parse %q", s)
	return ts
}

func TestRFC3339InFallBack(t *testing.T) {
	loc := toronto(t)

	// DST ends 2026-11-01 at 02:00 EDT (06:00 UTC): clocks fall back to 01:00 EST.
	// Before the transition the offset is -04:00 (EDT); after it is -05:00 (EST).
	before := mustParse(t, "2026-11-01T05:30:00Z")
	after := mustParse(t, "2026-11-01T06:30:00Z")

	require.Equal(t, "2026-11-01T01:30:00-04:00", RFC3339In(before, loc))
	require.Equal(t, "2026-11-01T01:30:00-05:00", RFC3339In(after, loc))
}

func TestRFC3339InSpringForward(t *testing.T) {
	loc := toronto(t)

	// DST begins 2027-03-14 at 02:00 EST (07:00 UTC): clocks jump to 03:00 EDT.
	// Before the transition the offset is -05:00 (EST); after it is -04:00 (EDT).
	before := mustParse(t, "2027-03-14T06:30:00Z")
	after := mustParse(t, "2027-03-14T07:30:00Z")

	require.Equal(t, "2027-03-14T01:30:00-05:00", RFC3339In(before, loc))
	require.Equal(t, "2027-03-14T03:30:00-04:00", RFC3339In(after, loc))
}

func TestRFC3339InPreservesInstant(t *testing.T) {
	loc := toronto(t)

	for _, in := range []string{
		"2026-11-01T05:30:00Z",
		"2026-11-01T06:30:00Z",
		"2027-03-14T06:30:00Z",
		"2027-03-14T07:30:00Z",
	} {
		in := in
		t.Run(in, func(t *testing.T) {
			orig := mustParse(t, in)
			rendered := RFC3339In(orig, loc)
			parsed, err := time.Parse(time.RFC3339, rendered)
			require.NoError(t, err)
			require.Equal(t, orig.Unix(), parsed.Unix(), "instant must survive the round-trip")
		})
	}
}

func TestRFC3339InNilLocationFallsBackToUTC(t *testing.T) {
	ts := mustParse(t, "2026-07-10T19:00:00Z")
	require.Equal(t, "2026-07-10T19:00:00Z", RFC3339In(ts, nil))
}

func TestRFC3339InPtr(t *testing.T) {
	loc := toronto(t)
	require.Equal(t, "", RFC3339InPtr(nil, loc))

	ts := mustParse(t, "2026-11-01T05:30:00Z")
	require.Equal(t, "2026-11-01T01:30:00-04:00", RFC3339InPtr(&ts, loc))
}

func TestRFC3339InPtrNilLocationFallsBackToUTC(t *testing.T) {
	ts := mustParse(t, "2026-07-10T19:00:00Z")
	require.Equal(t, "2026-07-10T19:00:00Z", RFC3339InPtr(&ts, nil))
}

func TestDateIn(t *testing.T) {
	loc := toronto(t)

	cases := []struct {
		name string
		in   string
		want string
	}{
		// 02:30 UTC on 2026-07-11 is 22:30 EDT on 2026-07-10 — the reported symptom.
		{"lateUTCIsPreviousLocalDay", "2026-07-11T02:30:00Z", "2026-07-10"},
		{"middaySameLocalDay", "2026-07-11T16:00:00Z", "2026-07-11"},
		{"afterFallBackSameLocalDay", "2026-11-01T06:30:00Z", "2026-11-01"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, DateIn(mustParse(t, tc.in), loc))
		})
	}
}

func TestDateInNilLocationFallsBackToUTC(t *testing.T) {
	ts := mustParse(t, "2026-07-11T02:30:00Z")
	require.Equal(t, "2026-07-11", DateIn(ts, nil))
}
