package events

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seriesTestMember(ulid, name, start string) SeriesCandidateMember {
	next := start
	return SeriesCandidateMember{
		ULID:                ulid,
		Name:                name,
		StartDate:           start,
		LastDate:            start,
		OccurrenceCount:     1,
		LifecycleState:      "published",
		NextOccurrenceDate:  &next,
		FirstLocalStartTime: "18:00",
		FirstDayOfWeek:      2,
		VenueULID:           "01HPLACE000000000000000001",
		VenueName:           "Main Branch",
	}
}

func seriesTestPair(a, b SeriesCandidateMember) SeriesCandidatePair {
	return SeriesCandidatePair{A: a, B: b}
}

func TestBuildSeriesCandidatesTransitiveAndFields(t *testing.T) {
	a := seriesTestMember("01HEVENT000000000000000001", "Family Storytime", "2026-11-10")
	b := seriesTestMember("01HEVENT000000000000000002", "Family Storytime", "2026-11-17")
	c := seriesTestMember("01HEVENT000000000000000003", "Family Story Time", "2026-11-24")

	result := BuildSeriesCandidates([]SeriesCandidatePair{
		seriesTestPair(a, b),
		seriesTestPair(b, c),
	}, SeriesCandidatesParams{MinEvents: 2, Limit: 50})

	require.Len(t, result.Items, 1)
	require.Equal(t, 1, result.TotalClusters)
	require.False(t, result.Truncated)

	cluster := result.Items[0]
	assert.Equal(t, 3, cluster.EventCount)
	assert.Equal(t, 3, cluster.OccurrenceCount)
	assert.Equal(t, "2026-11-10", cluster.FirstDate)
	assert.Equal(t, "2026-11-24", cluster.LastDate)
	assert.Equal(t, a.ULID, cluster.SuggestedCanonicalULID)
	assert.Equal(t, "Family Storytime", cluster.Name)
	assert.Equal(t, "01HPLACE000000000000000001", cluster.VenueID)
	assert.Equal(t, "Main Branch", cluster.VenueName)
	assert.Equal(t, "18:00", cluster.LocalStartTime)
	assert.Equal(t, 2, cluster.DayOfWeek)
	require.Len(t, cluster.Members, 3)
	assert.Equal(t, []string{a.ULID, b.ULID, c.ULID}, seriesTestMemberULIDs(cluster))
}

func TestBuildSeriesCandidatesMinEventsClamp(t *testing.T) {
	a := seriesTestMember("01HEVENT000000000000000001", "A", "2026-11-10")
	b := seriesTestMember("01HEVENT000000000000000002", "A", "2026-11-17")

	for _, minEvents := range []int{1, 2} {
		t.Run(fmt.Sprintf("minEvents=%d", minEvents), func(t *testing.T) {
			result := BuildSeriesCandidates([]SeriesCandidatePair{seriesTestPair(a, b)}, SeriesCandidatesParams{
				MinEvents: minEvents,
				Limit:     50,
			})
			require.Len(t, result.Items, 1, "MinEvents below 2 must clamp to 2")
		})
	}

	// Unset MinEvents (0) takes the documented default of 3, so a 2-event pair
	// is dropped.
	result := BuildSeriesCandidates([]SeriesCandidatePair{seriesTestPair(a, b)}, SeriesCandidatesParams{
		MinEvents: 0,
		Limit:     50,
	})
	require.Empty(t, result.Items, "MinEvents=0 must default to 3")

	// Explicitly requiring more events than present also drops the pair.
	result = BuildSeriesCandidates([]SeriesCandidatePair{seriesTestPair(a, b)}, SeriesCandidatesParams{
		MinEvents: 3,
		Limit:     50,
	})
	require.Empty(t, result.Items)
}

func TestBuildSeriesCandidatesLimitClamp(t *testing.T) {
	var pairs []SeriesCandidatePair
	for i := 0; i < 55; i++ {
		a := seriesTestMember(fmt.Sprintf("01HEVENT%019dA", i), "A", "2026-11-10")
		b := seriesTestMember(fmt.Sprintf("01HEVENT%019dB", i), "A", "2026-11-17")
		pairs = append(pairs, seriesTestPair(a, b))
	}

	for _, limit := range []int{-5, 0} {
		t.Run(fmt.Sprintf("limit=%d", limit), func(t *testing.T) {
			result := BuildSeriesCandidates(pairs, SeriesCandidatesParams{MinEvents: 2, Limit: limit})
			assert.Equal(t, 55, result.TotalClusters)
			assert.Len(t, result.Items, DefaultSeriesCandidateLimit)
			assert.True(t, result.Truncated)
		})
	}
}

func TestBuildSeriesCandidatesOrdering(t *testing.T) {
	// Bigger cluster first.
	bigA := seriesTestMember("01HEVENT000000000000000001", "Alpha", "2026-11-10")
	bigB := seriesTestMember("01HEVENT000000000000000002", "Alpha", "2026-11-17")
	bigC := seriesTestMember("01HEVENT000000000000000003", "Alpha", "2026-11-24")
	// Same count/date, later name.
	zetaA := seriesTestMember("01HEVENT000000000000000004", "Zeta", "2026-11-10")
	zetaB := seriesTestMember("01HEVENT000000000000000005", "Zeta", "2026-11-17")
	// Same count, earlier date.
	earlyA := seriesTestMember("01HEVENT000000000000000006", "Beta", "2026-11-09")
	earlyB := seriesTestMember("01HEVENT000000000000000007", "Beta", "2026-11-16")

	result := BuildSeriesCandidates([]SeriesCandidatePair{
		seriesTestPair(bigA, bigB),
		seriesTestPair(bigB, bigC),
		seriesTestPair(zetaA, zetaB),
		seriesTestPair(earlyA, earlyB),
	}, SeriesCandidatesParams{MinEvents: 2, Limit: 50})

	require.Len(t, result.Items, 3)
	assert.Equal(t, "Alpha", result.Items[0].Name)
	assert.Equal(t, "Beta", result.Items[1].Name)
	assert.Equal(t, "Zeta", result.Items[2].Name)
}

func TestBuildSeriesCandidatesOffset(t *testing.T) {
	// Three deterministic clusters, in sorted order: Alpha (3 events), Beta (2,
	// earlier first date), Zeta (2).
	bigA := seriesTestMember("01HEVENT000000000000000001", "Alpha", "2026-11-10")
	bigB := seriesTestMember("01HEVENT000000000000000002", "Alpha", "2026-11-17")
	bigC := seriesTestMember("01HEVENT000000000000000003", "Alpha", "2026-11-24")
	zetaA := seriesTestMember("01HEVENT000000000000000004", "Zeta", "2026-11-12")
	zetaB := seriesTestMember("01HEVENT000000000000000005", "Zeta", "2026-11-19")
	earlyA := seriesTestMember("01HEVENT000000000000000006", "Beta", "2026-11-09")
	earlyB := seriesTestMember("01HEVENT000000000000000007", "Beta", "2026-11-16")

	pairs := []SeriesCandidatePair{
		seriesTestPair(bigA, bigB),
		seriesTestPair(bigB, bigC),
		seriesTestPair(zetaA, zetaB),
		seriesTestPair(earlyA, earlyB),
	}

	t.Run("offset zero unchanged", func(t *testing.T) {
		result := BuildSeriesCandidates(pairs, SeriesCandidatesParams{MinEvents: 2, Limit: 50})
		require.Len(t, result.Items, 3)
		require.Equal(t, 3, result.TotalClusters)
		require.False(t, result.Truncated)
		assert.Equal(t, "Alpha", result.Items[0].Name)
	})

	t.Run("middle slice", func(t *testing.T) {
		result := BuildSeriesCandidates(pairs, SeriesCandidatesParams{MinEvents: 2, Offset: 1, Limit: 1})
		require.Len(t, result.Items, 1)
		require.Equal(t, 3, result.TotalClusters)
		assert.Equal(t, "Beta", result.Items[0].Name)
		// offset (1) + returned (1) < total (3): more clusters remain.
		assert.True(t, result.Truncated)
	})

	t.Run("last page is not truncated", func(t *testing.T) {
		result := BuildSeriesCandidates(pairs, SeriesCandidatesParams{MinEvents: 2, Offset: 2, Limit: 1})
		require.Len(t, result.Items, 1)
		assert.Equal(t, "Zeta", result.Items[0].Name)
		assert.False(t, result.Truncated)
	})

	t.Run("offset past the end", func(t *testing.T) {
		result := BuildSeriesCandidates(pairs, SeriesCandidatesParams{MinEvents: 2, Offset: 5, Limit: 10})
		require.Empty(t, result.Items)
		require.Equal(t, 3, result.TotalClusters)
		assert.False(t, result.Truncated, "nothing remains past the end")
	})

	t.Run("offset equal to total", func(t *testing.T) {
		result := BuildSeriesCandidates(pairs, SeriesCandidatesParams{MinEvents: 2, Offset: 3, Limit: 10})
		require.Empty(t, result.Items)
		assert.False(t, result.Truncated)
	})

	t.Run("negative offset clamps to zero", func(t *testing.T) {
		result := BuildSeriesCandidates(pairs, SeriesCandidatesParams{MinEvents: 2, Offset: -1, Limit: 1})
		require.Len(t, result.Items, 1)
		assert.Equal(t, "Alpha", result.Items[0].Name)
		assert.True(t, result.Truncated)
	})
}

func TestMostFrequentName(t *testing.T) {
	t.Run("modal name wins", func(t *testing.T) {
		component := []SeriesCandidateMember{
			seriesTestMember("01HEVENT000000000000000001", "Modal", "2026-11-10"),
			seriesTestMember("01HEVENT000000000000000002", "Modal", "2026-11-17"),
			seriesTestMember("01HEVENT000000000000000003", "Other", "2026-11-24"),
		}
		assert.Equal(t, "Modal", mostFrequentName(component))
	})

	t.Run("tie breaks on earliest occurrence", func(t *testing.T) {
		component := []SeriesCandidateMember{
			seriesTestMember("01HEVENT000000000000000001", "Later", "2026-11-17"),
			seriesTestMember("01HEVENT000000000000000002", "Earlier", "2026-11-10"),
		}
		assert.Equal(t, "Earlier", mostFrequentName(component))
	})

	t.Run("tie breaks lexicographically", func(t *testing.T) {
		component := []SeriesCandidateMember{
			seriesTestMember("01HEVENT000000000000000001", "Zeta", "2026-11-10"),
			seriesTestMember("01HEVENT000000000000000002", "Alpha", "2026-11-10"),
		}
		assert.Equal(t, "Alpha", mostFrequentName(component))
	})

	t.Run("trims trailing whitespace before counting", func(t *testing.T) {
		component := []SeriesCandidateMember{
			seriesTestMember("01HEVENT000000000000000001", "Family Storytime", "2026-11-10"),
			seriesTestMember("01HEVENT000000000000000002", "Family Storytime ", "2026-11-17"),
			seriesTestMember("01HEVENT000000000000000003", "Other", "2026-11-24"),
		}
		assert.Equal(t, "Family Storytime", mostFrequentName(component))
	})
}

func TestBuildSeriesCandidatesTieBreakDeterminism(t *testing.T) {
	newCluster := func(venueID, canonical string, start string) []SeriesCandidatePair {
		a := seriesTestMember(canonical, "Same Name", start)
		a.VenueULID = venueID
		b := seriesTestMember(canonical+"B", "Same Name", start)
		b.VenueULID = venueID
		return []SeriesCandidatePair{seriesTestPair(a, b)}
	}

	venues := []string{"01HPLACE00000000000000000Z", "01HPLACE00000000000000000A"}
	pairs := append(newCluster(venues[0], "01HEVENT000000000000000001", "2026-11-10"),
		newCluster(venues[1], "01HEVENT000000000000000002", "2026-11-10")...)

	for i := 0; i < 20; i++ {
		result := BuildSeriesCandidates(pairs, SeriesCandidatesParams{MinEvents: 2, Limit: 50})
		require.Len(t, result.Items, 2)
		assert.Equal(t, venues[1], result.Items[0].VenueID, "venue ID is the deterministic tiebreaker")
		assert.Equal(t, venues[0], result.Items[1].VenueID)
	}
}

func TestChooseCanonicalMember(t *testing.T) {
	t.Run("upcoming preferred over past", func(t *testing.T) {
		past := seriesTestMember("01HEVENT000000000000000001", "Past", "2026-11-01")
		past.NextOccurrenceDate = nil
		upcoming := seriesTestMember("01HEVENT000000000000000002", "Upcoming", "2026-11-20")
		assert.Equal(t, upcoming.ULID, chooseCanonicalMember([]SeriesCandidateMember{past, upcoming}).ULID)
	})

	t.Run("no upcoming falls back to earliest", func(t *testing.T) {
		earlier := seriesTestMember("01HEVENT000000000000000001", "Earlier", "2026-11-01")
		earlier.NextOccurrenceDate = nil
		later := seriesTestMember("01HEVENT000000000000000002", "Later", "2026-11-08")
		later.NextOccurrenceDate = nil
		assert.Equal(t, earlier.ULID, chooseCanonicalMember([]SeriesCandidateMember{later, earlier}).ULID)
	})

	t.Run("tie breaks on occurrence count", func(t *testing.T) {
		fewer := seriesTestMember("01HEVENT000000000000000001", "Fewer", "2026-11-10")
		fewer.OccurrenceCount = 1
		more := seriesTestMember("01HEVENT000000000000000002", "More", "2026-11-10")
		more.OccurrenceCount = 4
		assert.Equal(t, more.ULID, chooseCanonicalMember([]SeriesCandidateMember{fewer, more}).ULID)
	})

	t.Run("tie breaks on ULID", func(t *testing.T) {
		first := seriesTestMember("01HEVENT000000000000000001", "First", "2026-11-10")
		second := seriesTestMember("01HEVENT000000000000000002", "Second", "2026-11-10")
		assert.Equal(t, first.ULID, chooseCanonicalMember([]SeriesCandidateMember{second, first}).ULID)
	})
}

func seriesTestMemberULIDs(cluster SeriesCandidate) []string {
	ulids := make([]string, 0, len(cluster.Members))
	for _, member := range cluster.Members {
		ulids = append(ulids, member.ULID)
	}
	return ulids
}

// seriesTestMemberWithDescription builds a member carrying an opaque
// description fingerprint (the empty string means "no description").
func seriesTestMemberWithDescription(ulid, start, fingerprint string) SeriesCandidateMember {
	member := seriesTestMember(ulid, "Family Storytime", start)
	member.DescriptionFingerprint = fingerprint
	return member
}

func TestBuildSeriesCandidatesDescriptionDivergence(t *testing.T) {
	const (
		fingerprintA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		fingerprintB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	)

	t.Run("one distinct description", func(t *testing.T) {
		pairs := []SeriesCandidatePair{seriesTestPair(
			seriesTestMemberWithDescription("01HEVENT000000000000000001", "2026-11-10", fingerprintA),
			seriesTestMemberWithDescription("01HEVENT000000000000000002", "2026-11-17", fingerprintA),
		)}

		result := BuildSeriesCandidates(pairs, SeriesCandidatesParams{MinEvents: 2, Limit: 50})
		require.Len(t, result.Items, 1)
		assert.Equal(t, 1, result.Items[0].DistinctDescriptionCount)
		assert.False(t, result.Items[0].ContentVaries)
	})

	t.Run("two distinct descriptions", func(t *testing.T) {
		pairs := []SeriesCandidatePair{seriesTestPair(
			seriesTestMemberWithDescription("01HEVENT000000000000000001", "2026-11-10", fingerprintA),
			seriesTestMemberWithDescription("01HEVENT000000000000000002", "2026-11-17", fingerprintB),
		)}

		result := BuildSeriesCandidates(pairs, SeriesCandidatesParams{MinEvents: 2, Limit: 50})
		require.Len(t, result.Items, 1)
		assert.Equal(t, 2, result.Items[0].DistinctDescriptionCount)
		assert.True(t, result.Items[0].ContentVaries)
	})

	t.Run("all descriptions empty", func(t *testing.T) {
		pairs := []SeriesCandidatePair{seriesTestPair(
			seriesTestMemberWithDescription("01HEVENT000000000000000001", "2026-11-10", ""),
			seriesTestMemberWithDescription("01HEVENT000000000000000002", "2026-11-17", ""),
		)}

		result := BuildSeriesCandidates(pairs, SeriesCandidatesParams{MinEvents: 2, Limit: 50})
		require.Len(t, result.Items, 1)
		assert.Equal(t, 0, result.Items[0].DistinctDescriptionCount,
			"empty descriptions must not count as a distinct value")
		assert.False(t, result.Items[0].ContentVaries)
	})

	t.Run("one non-empty plus several empty", func(t *testing.T) {
		a := seriesTestMemberWithDescription("01HEVENT000000000000000001", "2026-11-10", fingerprintA)
		b := seriesTestMemberWithDescription("01HEVENT000000000000000002", "2026-11-17", "")
		c := seriesTestMemberWithDescription("01HEVENT000000000000000003", "2026-11-24", "")

		result := BuildSeriesCandidates([]SeriesCandidatePair{
			seriesTestPair(a, b),
			seriesTestPair(b, c),
		}, SeriesCandidatesParams{MinEvents: 2, Limit: 50})
		require.Len(t, result.Items, 1)
		assert.Equal(t, 1, result.Items[0].DistinctDescriptionCount,
			"the count is distinct non-empty descriptions, not described members")
		assert.True(t, result.Items[0].ContentVaries,
			"a mix of described and undescribed members loses content on consolidation")
	})
}
