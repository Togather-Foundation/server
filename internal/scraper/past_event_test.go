package scraper

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/rs/zerolog"
)

func TestFilterPastEvents(t *testing.T) {
	t.Parallel()

	loc, err := time.LoadLocation("America/Toronto")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	now := time.Date(2026, time.November, 7, 14, 0, 0, 0, loc)
	rfc := func(d time.Duration) string { return now.Add(d).Format(time.RFC3339) }
	day := func(d int) string { return now.AddDate(0, 0, d).Format(time.DateOnly) }

	event := func(name string, start, end string) events.EventInput {
		return events.EventInput{Name: name, StartDate: start, EndDate: end}
	}

	tests := []struct {
		name        string
		events      []events.EventInput
		grace       time.Duration
		wantNames   []string
		wantSkipped int
	}{
		{
			name:        "ended event is dropped",
			events:      []events.EventInput{event("ended", rfc(-48*time.Hour), rfc(-24*time.Hour))},
			wantNames:   []string{},
			wantSkipped: 1,
		},
		{
			name:      "ongoing multi-day event is kept",
			events:    []events.EventInput{event("ongoing", rfc(-48*time.Hour), rfc(24*time.Hour))},
			wantNames: []string{"ongoing"},
		},
		{
			name:      "future event is kept",
			events:    []events.EventInput{event("future", rfc(24*time.Hour), rfc(48*time.Hour))},
			wantNames: []string{"future"},
		},
		{
			name:      "event with no dates is kept",
			events:    []events.EventInput{event("unknown", "", "")},
			wantNames: []string{"unknown"},
		},
		{
			name:      "event with unparseable start date is kept",
			events:    []events.EventInput{event("bad-date", "not-a-date", "")},
			wantNames: []string{"bad-date"},
		},
		{
			name:      "unparseable end date is kept",
			events:    []events.EventInput{event("bad-end", rfc(-48*time.Hour), "not-a-date")},
			wantNames: []string{"bad-end"},
		},
		{
			name:      "unparseable end date with no start is kept",
			events:    []events.EventInput{event("bad-end-only", "", "not-a-date")},
			wantNames: []string{"bad-end-only"},
		},
		{
			name:        "absent end date with past start is dropped",
			events:      []events.EventInput{event("no-end-past", rfc(-48*time.Hour), "")},
			wantNames:   []string{},
			wantSkipped: 1,
		},
		{
			name:      "ended within grace window is kept",
			events:    []events.EventInput{event("recent", rfc(-4*time.Hour), rfc(-2*time.Hour))},
			grace:     24 * time.Hour,
			wantNames: []string{"recent"},
		},
		{
			name:        "ended outside grace window is dropped",
			events:      []events.EventInput{event("recent", rfc(-4*time.Hour), rfc(-2*time.Hour))},
			grace:       0,
			wantNames:   []string{},
			wantSkipped: 1,
		},
		{
			name: "mixed batch preserves order of kept events",
			events: []events.EventInput{
				event("ended-1", rfc(-72*time.Hour), rfc(-48*time.Hour)),
				event("future-1", rfc(24*time.Hour), rfc(48*time.Hour)),
				event("unknown", "", ""),
				event("ended-2", rfc(-4*time.Hour), rfc(-2*time.Hour)),
				event("ongoing", rfc(-24*time.Hour), rfc(2*time.Hour)),
			},
			wantNames:   []string{"future-1", "unknown", "ongoing"},
			wantSkipped: 2,
		},
		{
			name:        "bare date-only past date is dropped",
			events:      []events.EventInput{event("yesterday", day(-1), "")},
			wantNames:   []string{},
			wantSkipped: 1,
		},
		{
			name:      "bare date-only today is kept as all-day",
			events:    []events.EventInput{event("today", day(0), "")},
			wantNames: []string{"today"},
		},
		{
			name:      "bare date-only future date is kept",
			events:    []events.EventInput{event("tomorrow", day(1), "")},
			wantNames: []string{"tomorrow"},
		},
		{
			name:      "all-day event today is kept",
			events:    []events.EventInput{{Name: "all-day-today", StartDate: day(0), AllDay: true}},
			wantNames: []string{"all-day-today"},
		},
		{
			name: "multi-day all-day event ending today is kept",
			events: []events.EventInput{{
				Name:      "multi-day",
				StartDate: day(-2),
				EndDate:   day(0),
				AllDay:    true,
			}},
			wantNames: []string{"multi-day"},
		},
		{
			name:        "offset-less datetime in the past is dropped",
			events:      []events.EventInput{event("offsetless-past", now.AddDate(0, 0, -1).Format("2006-01-02T15:04:05"), "")},
			wantNames:   []string{},
			wantSkipped: 1,
		},
		{
			name:      "offset-less datetime in the future is kept",
			events:    []events.EventInput{event("offsetless-future", now.AddDate(0, 0, 1).Format("2006-01-02T15:04"), "")},
			wantNames: []string{"offsetless-future"},
		},
		{
			name: "occurrence end date keeps a series alive",
			events: []events.EventInput{{
				Name:      "series",
				StartDate: rfc(-48 * time.Hour),
				Occurrences: []events.OccurrenceInput{
					{StartDate: rfc(-48 * time.Hour), EndDate: rfc(-47 * time.Hour)},
					{StartDate: rfc(48 * time.Hour), EndDate: rfc(49 * time.Hour)},
				},
			}},
			wantNames: []string{"series"},
		},
		{
			name: "occurrence start without an end keeps a series alive",
			events: []events.EventInput{{
				Name:      "series-no-end",
				StartDate: rfc(-48 * time.Hour),
				Occurrences: []events.OccurrenceInput{
					{StartDate: rfc(-48 * time.Hour)},
					{StartDate: rfc(48 * time.Hour)},
				},
			}},
			wantNames: []string{"series-no-end"},
		},
		{
			name: "multi-session series entirely in the past is dropped",
			events: []events.EventInput{{
				Name:      "old-series",
				StartDate: rfc(-96 * time.Hour),
				Occurrences: []events.OccurrenceInput{
					{StartDate: rfc(-96 * time.Hour)},
					{StartDate: rfc(-48 * time.Hour)},
				},
			}},
			wantNames:   []string{},
			wantSkipped: 1,
		},
		{
			name:      "end date before start date falls back to end of start day",
			events:    []events.EventInput{event("bad-end-order", day(0)+"T10:00:00", day(-1))},
			wantNames: []string{"bad-end-order"},
		},
		{
			name:      "event ending exactly at the cutoff is kept",
			events:    []events.EventInput{event("at-cutoff", rfc(-48*time.Hour), now.Format(time.RFC3339))},
			wantNames: []string{"at-cutoff"},
		},
		{
			name:        "event ending one second before the cutoff is dropped",
			events:      []events.EventInput{event("before-cutoff", rfc(-48*time.Hour), now.Add(-time.Second).Format(time.RFC3339))},
			wantNames:   []string{},
			wantSkipped: 1,
		},
		{
			name:      "whitespace around dates is tolerated",
			events:    []events.EventInput{event("spaced", "  "+day(0)+"  ", "")},
			wantNames: []string{"spaced"},
		},
		{
			name:      "empty input returns empty slice",
			events:    nil,
			wantNames: []string{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			kept, skipped := filterPastEvents(tt.events, now, tt.grace, loc)

			if skipped != tt.wantSkipped {
				t.Errorf("skipped = %d, want %d", skipped, tt.wantSkipped)
			}
			gotNames := make([]string, 0, len(kept))
			for _, evt := range kept {
				gotNames = append(gotNames, evt.Name)
			}
			if !reflect.DeepEqual(gotNames, tt.wantNames) {
				t.Errorf("kept names = %v, want %v", gotNames, tt.wantNames)
			}
		})
	}
}

func TestFilterPastEventsDoesNotMutateInput(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.November, 7, 12, 0, 0, 0, time.UTC)
	input := []events.EventInput{
		{Name: "ended", StartDate: now.Add(-48 * time.Hour).Format(time.RFC3339)},
		{Name: "future", StartDate: now.Add(24 * time.Hour).Format(time.RFC3339)},
	}

	kept, skipped := filterPastEvents(input, now, 0, nil)
	if skipped != 1 {
		t.Fatalf("skipped = %d, want 1", skipped)
	}
	if len(input) != 2 || input[0].Name != "ended" || input[1].Name != "future" {
		t.Fatalf("input slice was mutated: %+v", input)
	}
	if len(kept) != 1 || kept[0].Name != "future" {
		t.Fatalf("kept = %+v, want just %q", kept, "future")
	}
}

func TestSetPastEventGraceClampsNegative(t *testing.T) {
	t.Parallel()

	s := NewScraper(nil, nil, zerolog.Nop())
	s.SetPastEventGrace(-2 * time.Hour)
	if s.pastEventGrace != 0 {
		t.Fatalf("pastEventGrace = %s, want 0", s.pastEventGrace)
	}
}

// TestOngoingHoursVaryEventSurvivesScrape is a regression test for the
// "Every Day • Hours Vary" placeholder: consolidation creates an occurrence at
// midnight today, which must not be dropped when the scrape runs later that day.
func TestOngoingHoursVaryEventSurvivesScrape(t *testing.T) {
	t.Parallel()

	loc, err := time.LoadLocation("America/Toronto")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	current := time.Now().In(loc)
	now := time.Date(current.Year(), current.Month(), current.Day(), 14, 0, 0, 0, loc)

	source := SourceConfig{
		Name:     "ongoing-source",
		URL:      "https://example.com/events",
		Timezone: "America/Toronto",
		License:  "CC0-1.0",
	}
	raws := []RawEvent{
		{Name: "Hours Vary Show", URL: "https://example.com/events/hours-vary", StartDate: "Every Day • Hours Vary"},
		{Name: "Hours Vary Show", URL: "https://example.com/events/hours-vary", StartDate: "Hours Vary"},
	}

	evts, skipped := normalizeRawEvents(raws, source, 0, zerolog.Nop(), now)
	if skipped != 0 {
		t.Fatalf("normalizeRawEvents skipped = %d, want 0", skipped)
	}
	if len(evts) != 1 {
		t.Fatalf("normalizeRawEvents returned %d events, want 1", len(evts))
	}
	if len(evts[0].Occurrences) != 1 || evts[0].Occurrences[0].EndDate == "" {
		t.Fatalf("placeholder occurrence = %+v, want a single whole-day occurrence", evts[0].Occurrences)
	}

	s := NewScraper(NewIngestClient("http://unused", "test-key"), nil, zerolog.Nop())
	s.SetPastEventGrace(0)
	s.SetDefaultLocation(loc)
	result := &ScrapeResult{SourceName: source.Name, DryRun: true}

	got := s.runWithTracking(t.Context(), result, func(ctx context.Context) (int, []events.EventInput, []string, error) {
		return len(evts), evts, nil, nil
	})

	if got.Error != nil {
		t.Fatalf("runWithTracking error: %v", got.Error)
	}
	if got.EventsSubmitted != 1 {
		t.Errorf("EventsSubmitted = %d, want 1", got.EventsSubmitted)
	}
	if len(got.DryRunEvents) != 1 {
		t.Errorf("DryRunEvents = %+v, want the ongoing event", got.DryRunEvents)
	}
}

func TestRunWithTrackingFiltersPastEvents(t *testing.T) {
	t.Parallel()

	s := NewScraper(NewIngestClient("http://unused", "test-key"), nil, zerolog.Nop())
	now := time.Now()
	result := &ScrapeResult{SourceName: "test-source", DryRun: true}

	got := s.runWithTracking(t.Context(), result, func(ctx context.Context) (int, []events.EventInput, []string, error) {
		return 2, []events.EventInput{
			{Name: "ended", StartDate: now.Add(-48 * time.Hour).Format(time.RFC3339)},
			{Name: "future", StartDate: now.Add(48 * time.Hour).Format(time.RFC3339)},
		}, nil, nil
	})

	if got.EventsFound != 2 {
		t.Errorf("EventsFound = %d, want raw source count 2", got.EventsFound)
	}
	if got.EventsSubmitted != 1 {
		t.Errorf("EventsSubmitted = %d, want 1", got.EventsSubmitted)
	}
	if len(got.DryRunEvents) != 1 || got.DryRunEvents[0].Name != "future" {
		t.Errorf("DryRunEvents = %+v, want only %q", got.DryRunEvents, "future")
	}
	if !hasQualityWarning(got.QualityWarnings, "skipped 1 event(s) that already ended") {
		t.Errorf("QualityWarnings = %v, want aggregate past-event warning", got.QualityWarnings)
	}
}

// TestRunWithTrackingSubmitsOnlyKeptEvents verifies the non-dry-run path: only
// events that survive the guard are POSTed to the ingest API.
func TestRunWithTrackingSubmitsOnlyKeptEvents(t *testing.T) {
	t.Parallel()

	var (
		mu     sync.Mutex
		posted []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/events:batch" {
			http.NotFound(w, r)
			return
		}
		var payload struct {
			Events []events.EventInput `json:"events"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode batch payload: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		mu.Lock()
		for _, evt := range payload.Events {
			posted = append(posted, evt.Name)
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"batch_id":       "test-batch",
			"events_created": len(payload.Events),
		})
	}))
	t.Cleanup(srv.Close)

	s := NewScraper(NewIngestClient(srv.URL, "test-key"), nil, zerolog.Nop())
	now := time.Now()
	result := &ScrapeResult{SourceName: "test-source"}

	got := s.runWithTracking(t.Context(), result, func(ctx context.Context) (int, []events.EventInput, []string, error) {
		return 2, []events.EventInput{
			{Name: "ended", StartDate: now.Add(-48 * time.Hour).Format(time.RFC3339)},
			{Name: "future", StartDate: now.Add(48 * time.Hour).Format(time.RFC3339)},
		}, nil, nil
	})

	if got.Error != nil {
		t.Fatalf("runWithTracking error: %v", got.Error)
	}
	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(posted, []string{"future"}) {
		t.Errorf("posted events = %v, want [future]", posted)
	}
	if got.EventsCreated != 1 || got.EventsFailed != 0 {
		t.Errorf("EventsCreated = %d, EventsFailed = %d, want 1 and 0", got.EventsCreated, got.EventsFailed)
	}
}

func hasQualityWarning(warnings []string, substr string) bool {
	for _, w := range warnings {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}
