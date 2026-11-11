package events

import (
	"context"
	"testing"
)

// Feed-scoped event identity: (feed URL, sourceEventID).
//
// The `sources` row is not the identity scope. A feed used to acquire two
// source rows (the sources upsert key is the host-only base_url plus the config
// name), and before this rule the same feed event looked new under the second
// row.
//
// The URL is stored in its normalised form (validation strips the query and
// fragment) and compared with trailing slashes ignored on both sides; the stored
// value itself is never rewritten after that.

// TestIngest_FeedIdentity_LegacyTrailingSlash is the regression guard for the
// pre-existing corpus: thousands of event_sources rows were written with a
// trailing slash, while the live feed URLs are entered without one (and vice
// versa). Either form must resolve to the same event.
//
// The second observation also renames the event, so a missed identity match
// cannot be masked by the dedup-hash fallback (a same-name re-scrape would merge
// by hash and hide the defect).
func TestIngest_FeedIdentity_LegacyTrailingSlash(t *testing.T) {
	cases := []struct {
		name   string
		stored string
		live   string
	}{
		{
			name:   "stored with slash, incoming without",
			stored: "https://example.com/eventslist/",
			live:   "https://example.com/eventslist",
		},
		{
			name:   "stored without slash, incoming with",
			stored: "https://example.com/eventslist",
			live:   "https://example.com/eventslist/",
		},
		{
			name:   "root-only difference",
			stored: "https://example.com/",
			live:   "https://example.com",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewMockRepository()
			service := newTestService(repo)
			startDate := validFutureDate()

			first := completeEventInputWithSource("Gravitational Waves", tt.stored, "evt-1", "Example Source")
			first.StartDate = startDate
			firstResult, err := service.Ingest(context.Background(), first)
			if err != nil {
				t.Fatalf("first Ingest() unexpected error = %v", err)
			}

			second := completeEventInputWithSource("GW Focus Group", tt.live, "evt-1", "Example Source")
			second.StartDate = startDate
			secondResult, err := service.Ingest(context.Background(), second)
			if err != nil {
				t.Fatalf("second Ingest() unexpected error = %v", err)
			}

			if !secondResult.IsDuplicate {
				t.Error("a trailing slash must not make the feed event look new")
			}
			if secondResult.Event.ULID != firstResult.Event.ULID {
				t.Fatalf("second Ingest() created event %s, want the existing %s",
					secondResult.Event.ULID, firstResult.Event.ULID)
			}
			if got := len(repo.events); got != 1 {
				t.Fatalf("expected 1 event, got %d", got)
			}
		})
	}
}

// TestIngest_FeedIdentity_ProvenanceURLIsVerbatim pins the other half of the
// rule: attribution writes store the feed URL exactly as the feed reported it.
// Rewriting it would stop matching the UNIQUE (event_id, source_id, source_url)
// upsert target and accrue a second provenance row per event.
func TestIngest_FeedIdentity_ProvenanceURLIsVerbatim(t *testing.T) {
	repo := NewMockRepository()
	service := newTestService(repo)
	startDate := validFutureDate()

	const (
		feedURL     = "https://example.com/eventslist/"
		sourceEvent = "evt-1"
	)

	first := completeEventInputWithSource("Gravitational Waves", feedURL, sourceEvent, "Example Source")
	first.StartDate = startDate
	if _, err := service.Ingest(context.Background(), first); err != nil {
		t.Fatalf("first Ingest() unexpected error = %v", err)
	}

	// Same feed event, same URL, renamed: the identity path must record the
	// identical URL so the repository's upsert collapses it onto the existing
	// attribution row.
	second := completeEventInputWithSource("GW Focus Group", feedURL, sourceEvent, "Example Source")
	second.StartDate = startDate
	if _, err := service.Ingest(context.Background(), second); err != nil {
		t.Fatalf("second Ingest() unexpected error = %v", err)
	}

	if got := len(repo.createdSources); got != 2 {
		t.Fatalf("expected 2 attribution writes, got %d", got)
	}
	for i, attribution := range repo.createdSources {
		if attribution.SourceURL != feedURL {
			t.Errorf("attribution %d stored source_url = %q, want the verbatim %q",
				i, attribution.SourceURL, feedURL)
		}
	}
}

// TestIngest_FeedIdentity_RenameUpdatesOneEvent is the reported case: the same
// Google Calendar occurrence, attributed by two source rows (`Google Calendar`
// and `cita-local-events`), with the feed having renamed the event.
func TestIngest_FeedIdentity_RenameUpdatesOneEvent(t *testing.T) {
	repo := NewMockRepository()
	service := newTestService(repo)

	const (
		feedURL      = "https://calendar.google.com/calendar/ical/qfg129/public/basic.ics"
		sourceEvent  = "h144f2ve4hp5q7nj4pc63sk6ff_R20250311T190000@google.com:2026-11-10T15:00:00-05:00"
		oldName      = "Gravitational Waves"
		newName      = "GW Focus Group"
		firstSource  = "Google Calendar"
		secondSource = "cita-local-events"
	)
	startDate := validFutureDate()

	first := completeEventInputWithSource(oldName, feedURL, sourceEvent, firstSource)
	first.StartDate = startDate

	firstResult, err := service.Ingest(context.Background(), first)
	if err != nil {
		t.Fatalf("first Ingest() unexpected error = %v", err)
	}
	if firstResult.IsDuplicate {
		t.Fatal("first Ingest() should not be a duplicate")
	}
	eventULID := firstResult.Event.ULID
	// Copy, not a reference: the mock mutates the stored event in place.
	firstHash := firstResult.Event.DedupHash

	// Simulate reconciliation switching resolution from a historical catch-all
	// to the canonical per-feed row. Event identity must remain independent of
	// whichever registry row resolves, including when the feed renames the event.
	repo.sources[NormalizeFeedURL(feedURL)] = "canonical-after-repair"
	second := completeEventInputWithSource(newName, feedURL, sourceEvent, secondSource)
	second.StartDate = startDate
	second.Description = first.Description
	second.Image = first.Image

	secondResult, err := service.Ingest(context.Background(), second)
	if err != nil {
		t.Fatalf("second Ingest() unexpected error = %v", err)
	}

	if !secondResult.IsDuplicate {
		t.Error("second Ingest() should be a duplicate (same feed event id)")
	}
	if !secondResult.IsMerged {
		t.Error("second Ingest() should report a merge (name changed)")
	}
	if secondResult.Event.ULID != eventULID {
		t.Fatalf("second Ingest() returned event %s, want the renamed event %s", secondResult.Event.ULID, eventULID)
	}
	if got := len(repo.events); got != 1 {
		t.Fatalf("expected exactly one event after the rename, got %d", got)
	}

	stored := repo.events[eventULID]
	if stored.Name != newName {
		t.Errorf("stored name = %q, want the feed's new name %q", stored.Name, newName)
	}

	// Pure rename: the rest of the identity is unchanged, so the hash must be
	// rewritten to the new name.
	wantHash := BuildDedupHash(DedupCandidate{
		Name:      newName,
		VenueID:   NormalizeVenueKey(second),
		StartDate: startDate,
	})
	if stored.DedupHash != wantHash {
		t.Errorf("dedup_hash = %q, want recomputed hash %q", stored.DedupHash, wantHash)
	}
	if stored.DedupHash == firstHash {
		t.Error("dedup_hash was not rewritten by the rename")
	}
	if _, ok := repo.eventsByDedupHash[wantHash]; !ok {
		t.Error("recomputed dedup hash is not indexed for Layer-1 dedupe")
	}

	// Both source rows must be recorded against the one event.
	if got := len(repo.createdSources); got != 2 {
		t.Fatalf("expected 2 source attributions, got %d", got)
	}
	for _, attribution := range repo.createdSources {
		if attribution.SourceEventID != sourceEvent {
			t.Errorf("attribution source_event_id = %q, want %q", attribution.SourceEventID, sourceEvent)
		}
		if attribution.SourceURL != feedURL {
			t.Errorf("attribution source_url = %q, want %q", attribution.SourceURL, feedURL)
		}
		if attribution.EventID != stored.ID {
			t.Errorf("attribution event_id = %q, want %q", attribution.EventID, stored.ID)
		}
	}
	if repo.createdSources[0].SourceID == repo.createdSources[1].SourceID {
		t.Error("expected two distinct source rows for the same feed")
	}
}

// TestIngest_FeedIdentity_DifferentFeedKeepsTwoEvents is the negative control:
// the same sourceEventID from two feeds on the SAME host and under the SAME
// source-row name must stay two events. Row-scoped identity would have merged
// them (same source row + same id).
func TestIngest_FeedIdentity_DifferentFeedKeepsTwoEvents(t *testing.T) {
	repo := NewMockRepository()
	service := newTestService(repo)

	const (
		sourceEvent = "shared-event-id"
		sourceName  = "Google Calendar"
		feedA       = "https://calendar.google.com/calendar/ical/alpha/public/basic.ics"
		feedB       = "https://calendar.google.com/calendar/ical/beta/public/basic.ics"
	)
	startDate := validFutureDate()

	first := completeEventInputWithSource("Alpha Meetup", feedA, sourceEvent, sourceName)
	first.StartDate = startDate
	firstResult, err := service.Ingest(context.Background(), first)
	if err != nil {
		t.Fatalf("first Ingest() unexpected error = %v", err)
	}

	second := completeEventInputWithSource("Beta Meetup", feedB, sourceEvent, sourceName)
	second.StartDate = startDate
	secondResult, err := service.Ingest(context.Background(), second)
	if err != nil {
		t.Fatalf("second Ingest() unexpected error = %v", err)
	}

	if secondResult.IsDuplicate {
		t.Error("a different feed with the same source event id must not be a duplicate")
	}
	if secondResult.Event.ULID == firstResult.Event.ULID {
		t.Fatal("different feeds sharing an id were merged into one event")
	}
	if got := len(repo.events); got != 2 {
		t.Fatalf("expected 2 events (one per feed), got %d", got)
	}
}

// TestIngest_FeedIdentity_RenameWithVenueMoveKeepsHash pins the hash-drift
// guard: when a re-scrape renames the event AND moves it to another venue in one
// payload, ingest still does not update the venue (it has no place-resolution or
// occurrence update on this path), so the dedup hash must not be rebuilt from
// the incoming venue. Rebuilding it would leave the hash describing an identity
// the stored event does not have, and Layer-1 would then miss or mis-merge.
func TestIngest_FeedIdentity_RenameWithVenueMoveKeepsHash(t *testing.T) {
	repo := NewMockRepository()
	service := newTestService(repo)
	startDate := validFutureDate()

	const (
		feedURL     = "https://calendar.google.com/calendar/ical/qfg129/public/basic.ics"
		sourceEvent = "gcal-event-1"
	)

	first := completeEventInputWithSource("Gravitational Waves", feedURL, sourceEvent, "Google Calendar")
	first.StartDate = startDate
	firstResult, err := service.Ingest(context.Background(), first)
	if err != nil {
		t.Fatalf("first Ingest() unexpected error = %v", err)
	}
	storedHash := repo.events[firstResult.Event.ULID].DedupHash
	if storedHash == "" {
		t.Fatal("first ingest produced no dedup hash")
	}

	// Rename and move the venue in the same payload.
	second := completeEventInputWithSource("GW Focus Group", feedURL, sourceEvent, "Google Calendar")
	second.StartDate = startDate
	second.Location = &PlaceInput{Name: "Bridge Lab", AddressLocality: "Toronto", AddressRegion: "ON"}

	secondResult, err := service.Ingest(context.Background(), second)
	if err != nil {
		t.Fatalf("second Ingest() unexpected error = %v", err)
	}
	if secondResult.Event.ULID != firstResult.Event.ULID {
		t.Fatal("the rename with a venue move created a second event")
	}
	if got := len(repo.events); got != 1 {
		t.Fatalf("expected 1 event, got %d", got)
	}

	stored := repo.events[firstResult.Event.ULID]
	if stored.Name != "GW Focus Group" {
		t.Errorf("stored name = %q, want the feed's new name", stored.Name)
	}
	if stored.DedupHash != storedHash {
		t.Errorf("dedup_hash = %q, want it left untouched at %q (venue differs)", stored.DedupHash, storedHash)
	}
	if _, ok := repo.eventsByDedupHash[storedHash]; !ok {
		t.Error("the unchanged dedup hash is no longer indexed for Layer-1 dedupe")
	}
}

// TestFindByFeedEventID_EmptyArguments pins the guard: without either half of
// the identity pair there is nothing to match, so the caller falls through to
// the dedup-hash path exactly as before. (Ingest validation also rejects a
// source with a URL but no eventId, so an HTTP submission cannot reach the
// lookup with an empty eventId — this is the defensive repository contract.)
func TestFindByFeedEventID_EmptyArguments(t *testing.T) {
	repo := NewMockRepository()
	event := &Event{ID: "event-1", ULID: "01ARZ3NDEKTSV4RRFFQ69G5FAV", Name: "Jazz Night"}
	repo.AddExistingEvent("https://example.com/feed", "ext-1", "source-1", event)

	tests := []struct {
		name          string
		feedURL       string
		sourceEventID string
	}{
		{"empty feed URL", "", "ext-1"},
		{"blank feed URL", "   ", "ext-1"},
		{"empty source event id", "https://example.com/feed", ""},
		{"blank source event id", "https://example.com/feed", "   "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repo.FindByFeedEventID(context.Background(), tt.feedURL, tt.sourceEventID)
			if err != ErrNotFound {
				t.Errorf("FindByFeedEventID() error = %v, want ErrNotFound", err)
			}
			if got != nil {
				t.Errorf("FindByFeedEventID() = %v, want nil", got)
			}
		})
	}
}
