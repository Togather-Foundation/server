package events

import (
	"context"
	"sort"
	"strings"
	"time"
)

// Series-candidate detection defaults. These are discovery-surface tuning
// knobs for agents, not operator configuration; callers override them per
// request via the admin endpoint or CLI flags.
const (
	// DefaultSeriesCandidateDays is the look-ahead window.
	DefaultSeriesCandidateDays = 90
	// DefaultSeriesCandidateLookbackDays includes recent past occurrences so a
	// programme that already started is still discoverable.
	DefaultSeriesCandidateLookbackDays = 14
	// DefaultSeriesCandidateMinEvents is the smallest cluster reported.
	DefaultSeriesCandidateMinEvents = 3
	// DefaultSeriesCandidateLimit caps the number of returned clusters.
	DefaultSeriesCandidateLimit = 50
)

// SeriesCandidatesParams controls a series-candidate scan.
type SeriesCandidatesParams struct {
	// Now anchors the look-ahead and look-back windows. Zero means time.Now().
	Now time.Time
	// Days is the look-ahead window in days.
	Days int
	// LookbackDays includes this many days of recent past occurrences.
	LookbackDays int
	// MinEvents is the smallest number of distinct events a cluster must contain.
	MinEvents int
	// VenueULID optionally restricts the scan to one place.
	VenueULID string
	// Limit caps the number of returned clusters.
	Limit int
}

// WithDefaults fills unset/invalid fields with the documented defaults.
func (p SeriesCandidatesParams) WithDefaults() SeriesCandidatesParams {
	if p.Days == 0 {
		p.Days = DefaultSeriesCandidateDays
	}
	if p.LookbackDays < 0 {
		p.LookbackDays = DefaultSeriesCandidateLookbackDays
	}
	if p.MinEvents == 0 {
		p.MinEvents = DefaultSeriesCandidateMinEvents
	}
	if p.Limit == 0 {
		p.Limit = DefaultSeriesCandidateLimit
	}
	return p
}

// SeriesCandidatesQuery is the storage-level window for pair discovery.
type SeriesCandidatesQuery struct {
	// WindowStart is the inclusive first local date (normally a date at 00:00).
	// Storage widens it by one day because request dates are UTC and occurrence
	// dates are venue-local; treat the boundary as approximate.
	WindowStart time.Time
	// WindowEnd is the inclusive last local date (normally a date at 00:00).
	// Like WindowStart, storage widens it by one day for venue-local dates.
	WindowEnd time.Time
	// Now is the instant used to pick the earliest upcoming occurrence.
	Now time.Time
	// VenueULID optionally restricts the scan to one place.
	VenueULID string
}

// SeriesCandidateMember is one event that belongs to a detected series.
// StartDate/LastDate are civil dates (YYYY-MM-DD) derived from the occurrence's
// local_date; OccurrenceCount counts in-window occurrences.
type SeriesCandidateMember struct {
	ULID                string
	Name                string
	StartDate           string
	LastDate            string
	OccurrenceCount     int
	LifecycleState      string
	NextOccurrenceDate  *string
	FirstLocalStartTime string
	FirstDayOfWeek      int
	VenueULID           string
	VenueName           string
}

// SeriesCandidatePair is one qualifying 7-day edge between two distinct events.
type SeriesCandidatePair struct {
	A SeriesCandidateMember
	B SeriesCandidateMember
}

// SeriesCandidate is a connected component of events that look like one weekly
// programme scraped as N separate events.
type SeriesCandidate struct {
	SuggestedCanonicalULID string
	Name                   string
	VenueID                string
	VenueName              string
	DayOfWeek              int
	LocalStartTime         string
	EventCount             int
	OccurrenceCount        int
	FirstDate              string
	LastDate               string
	Members                []SeriesCandidateMember
}

// SeriesCandidatesResult is the complete discovery result before/after limit.
type SeriesCandidatesResult struct {
	Items         []SeriesCandidate
	TotalClusters int
	Truncated     bool
}

// FindSeriesCandidates discovers conservative weekly-series clusters. It is a
// read-only operation: it never writes events, occurrences, or review rows.
func (s *AdminService) FindSeriesCandidates(ctx context.Context, params SeriesCandidatesParams) (SeriesCandidatesResult, error) {
	params = params.WithDefaults()
	now := params.Now
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()

	query := SeriesCandidatesQuery{
		WindowStart: startOfDateUTC(now.AddDate(0, 0, -params.LookbackDays)),
		WindowEnd:   startOfDateUTC(now.AddDate(0, 0, params.Days)),
		Now:         now,
		VenueULID:   params.VenueULID,
	}

	pairs, err := s.repo.ListSeriesCandidatePairs(ctx, query)
	if err != nil {
		return SeriesCandidatesResult{}, err
	}

	return BuildSeriesCandidates(pairs, params), nil
}

// BuildSeriesCandidates turns qualifying edges into connected components with
// union-find, then selects canonical members and sorts the clusters.
func BuildSeriesCandidates(pairs []SeriesCandidatePair, params SeriesCandidatesParams) SeriesCandidatesResult {
	params = params.WithDefaults()
	if params.MinEvents < 2 {
		params.MinEvents = 2
	}
	if params.Limit < 1 {
		params.Limit = DefaultSeriesCandidateLimit
	}

	members := make(map[string]SeriesCandidateMember, len(pairs)*2)
	parent := make(map[string]string, len(pairs)*2)

	var find func(string) string
	find = func(ulid string) string {
		p, ok := parent[ulid]
		if !ok || p == ulid {
			parent[ulid] = ulid
			return ulid
		}
		root := find(p)
		parent[ulid] = root
		return root
	}
	union := func(a, b string) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[rb] = ra
		}
	}

	addMember := func(m SeriesCandidateMember) {
		if m.ULID == "" {
			return
		}
		if _, ok := members[m.ULID]; !ok {
			members[m.ULID] = m
			parent[m.ULID] = m.ULID
		}
	}

	for _, pair := range pairs {
		if pair.A.ULID == "" || pair.B.ULID == "" || pair.A.ULID == pair.B.ULID {
			continue
		}
		addMember(pair.A)
		addMember(pair.B)
		union(pair.A.ULID, pair.B.ULID)
	}

	components := make(map[string][]SeriesCandidateMember)
	for ulid, member := range members {
		root := find(ulid)
		components[root] = append(components[root], member)
	}

	clusters := make([]SeriesCandidate, 0, len(components))
	for _, component := range components {
		if len(component) < params.MinEvents {
			continue
		}
		clusters = append(clusters, buildSeriesCluster(component))
	}

	sort.Slice(clusters, func(i, j int) bool {
		if clusters[i].EventCount != clusters[j].EventCount {
			return clusters[i].EventCount > clusters[j].EventCount
		}
		if clusters[i].FirstDate != clusters[j].FirstDate {
			return clusters[i].FirstDate < clusters[j].FirstDate
		}
		if clusters[i].Name != clusters[j].Name {
			return clusters[i].Name < clusters[j].Name
		}
		if clusters[i].VenueID != clusters[j].VenueID {
			return clusters[i].VenueID < clusters[j].VenueID
		}
		return clusters[i].SuggestedCanonicalULID < clusters[j].SuggestedCanonicalULID
	})

	total := len(clusters)
	truncated := total > params.Limit
	if truncated {
		clusters = clusters[:params.Limit]
	}

	return SeriesCandidatesResult{Items: clusters, TotalClusters: total, Truncated: truncated}
}

func buildSeriesCluster(component []SeriesCandidateMember) SeriesCandidate {
	sort.Slice(component, func(i, j int) bool {
		if component[i].StartDate != component[j].StartDate {
			return component[i].StartDate < component[j].StartDate
		}
		return component[i].ULID < component[j].ULID
	})

	name := mostFrequentName(component)
	canonical := chooseCanonicalMember(component)

	firstDate := component[0].StartDate
	lastDate := component[0].LastDate
	occurrenceCount := 0
	for _, member := range component {
		if member.StartDate < firstDate {
			firstDate = member.StartDate
		}
		if member.LastDate > lastDate {
			lastDate = member.LastDate
		}
		occurrenceCount += member.OccurrenceCount
	}

	venueID, venueName := canonical.VenueULID, canonical.VenueName
	if venueID == "" {
		venueID, venueName = component[0].VenueULID, component[0].VenueName
	}

	return SeriesCandidate{
		SuggestedCanonicalULID: canonical.ULID,
		Name:                   name,
		VenueID:                venueID,
		VenueName:              venueName,
		DayOfWeek:              canonical.FirstDayOfWeek,
		LocalStartTime:         canonical.FirstLocalStartTime,
		EventCount:             len(component),
		OccurrenceCount:        occurrenceCount,
		FirstDate:              firstDate,
		LastDate:               lastDate,
		Members:                component,
	}
}

// mostFrequentName picks the modal member name. Ties resolve to the name with
// the earliest first occurrence, then lexicographically.
func mostFrequentName(component []SeriesCandidateMember) string {
	counts := make(map[string]int, len(component))
	earliest := make(map[string]string, len(component))
	for _, member := range component {
		name := strings.TrimSpace(member.Name)
		counts[name]++
		if prev, ok := earliest[name]; !ok || member.StartDate < prev {
			earliest[name] = member.StartDate
		}
	}

	best := ""
	bestCount := -1
	bestDate := ""
	for name, count := range counts {
		date := earliest[name]
		switch {
		case count > bestCount:
			best, bestCount, bestDate = name, count, date
		case count == bestCount && date < bestDate:
			best, bestDate = name, date
		case count == bestCount && date == bestDate && name < best:
			best = name
		}
	}
	return best
}

// chooseCanonicalMember prefers the member with the earliest upcoming
// occurrence, falling back to the earliest overall. Ties break on most
// occurrences, then ULID.
func chooseCanonicalMember(component []SeriesCandidateMember) SeriesCandidateMember {
	hasUpcoming := false
	for _, member := range component {
		if member.NextOccurrenceDate != nil {
			hasUpcoming = true
			break
		}
	}

	best := component[0]
	if hasUpcoming && best.NextOccurrenceDate == nil {
		for _, member := range component[1:] {
			if member.NextOccurrenceDate != nil {
				best = member
				break
			}
		}
	}
	if hasUpcoming && best.NextOccurrenceDate == nil {
		// Defensive: should be unreachable when hasUpcoming is true.
		hasUpcoming = false
	}

	better := func(candidate, current SeriesCandidateMember) bool {
		if hasUpcoming {
			candidateDate := derefString(candidate.NextOccurrenceDate)
			currentDate := derefString(current.NextOccurrenceDate)
			if candidateDate != currentDate {
				return candidateDate < currentDate
			}
		} else {
			if candidate.StartDate != current.StartDate {
				return candidate.StartDate < current.StartDate
			}
		}
		if candidate.OccurrenceCount != current.OccurrenceCount {
			return candidate.OccurrenceCount > current.OccurrenceCount
		}
		return candidate.ULID < current.ULID
	}

	for _, member := range component {
		if hasUpcoming && member.NextOccurrenceDate == nil {
			continue
		}
		if better(member, best) {
			best = member
		}
	}
	return best
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func startOfDateUTC(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
