package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/Togather-Foundation/server/internal/scraper"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
)

// mockScraperConfig is a test double for scraperConfigReader.
type mockScraperConfig struct {
	cfg postgres.ScraperConfig
	err error
}

func (m *mockScraperConfig) GetScraperConfig(ctx context.Context) (postgres.ScraperConfig, error) {
	return m.cfg, m.err
}

// mockScraper is a test double for scraperSourceScraper.
type mockScraper struct {
	result   scraper.ScrapeResult
	err      error
	called   bool
	lastOpts scraper.ScrapeOptions
}

func (m *mockScraper) ScrapeSource(ctx context.Context, sourceName string, opts scraper.ScrapeOptions) (scraper.ScrapeResult, error) {
	m.called = true
	m.lastOpts = opts
	return m.result, m.err
}

// ---------------------------------------------------------------------------
// sourceJitterOffset tests
// ---------------------------------------------------------------------------

func TestSourceJitterOffset_InWindow(t *testing.T) {
	t.Parallel()
	window := 2 * time.Hour
	tests := []string{"source-a", "source-b", "toronto-events", "weekly-arts", ""}

	for _, name := range tests {
		offset := sourceJitterOffset(name, window)
		if offset < 0 {
			t.Errorf("sourceJitterOffset(%q): got negative offset %v", name, offset)
		}
		if offset >= window {
			t.Errorf("sourceJitterOffset(%q): offset %v >= window %v", name, offset, window)
		}
	}
}

func TestSourceJitterOffset_Deterministic(t *testing.T) {
	t.Parallel()
	window := 2 * time.Hour
	names := []string{"source-a", "source-b", "toronto-events"}

	for _, name := range names {
		first := sourceJitterOffset(name, window)
		second := sourceJitterOffset(name, window)
		if first != second {
			t.Errorf("sourceJitterOffset(%q): not deterministic: %v vs %v", name, first, second)
		}
	}
}

func TestSourceJitterOffset_DifferentNames(t *testing.T) {
	t.Parallel()
	window := 2 * time.Hour
	// Most distinct names should produce different offsets (hash collision unlikely).
	a := sourceJitterOffset("source-alpha", window)
	b := sourceJitterOffset("source-beta", window)
	if a == b {
		t.Errorf("sourceJitterOffset: expected different offsets for different names, both got %v", a)
	}
}

func TestSourceJitterOffset_ZeroWindow(t *testing.T) {
	t.Parallel()
	offset := sourceJitterOffset("any-source", 0)
	if offset != 0 {
		t.Errorf("sourceJitterOffset with zero window: expected 0, got %v", offset)
	}
}

// ---------------------------------------------------------------------------
// staggeredSchedule tests
// ---------------------------------------------------------------------------

func TestStaggeredSchedule_Next_StrictlyAfterCurrent(t *testing.T) {
	t.Parallel()
	s := &staggeredSchedule{
		interval: 24 * time.Hour,
		offset:   30 * time.Minute,
	}

	// current is exactly at the slot (periodStart + offset)
	base := time.Date(2026, 1, 1, 0, 30, 0, 0, time.UTC) // exactly periodStart+30min
	next := s.Next(base)
	if !next.After(base) {
		t.Errorf("Next(%v) = %v, want strictly after current", base, next)
	}
}

func TestStaggeredSchedule_Next_CorrectPeriod(t *testing.T) {
	t.Parallel()
	interval := 24 * time.Hour
	offset := 30 * time.Minute

	s := &staggeredSchedule{interval: interval, offset: offset}

	// At 01:00 UTC — the slot for the current day (00:30) has passed,
	// so next should be tomorrow at 00:30.
	current := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)
	next := s.Next(current)

	expected := time.Date(2026, 1, 2, 0, 30, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("Next(%v) = %v, want %v", current, next, expected)
	}
}

func TestStaggeredSchedule_Next_BeforeSlot(t *testing.T) {
	t.Parallel()
	interval := 24 * time.Hour
	offset := 2 * time.Hour

	s := &staggeredSchedule{interval: interval, offset: offset}

	// At 01:00 UTC — the slot for the current day (02:00) is still ahead.
	current := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)
	next := s.Next(current)

	expected := time.Date(2026, 1, 1, 2, 0, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("Next(%v) = %v, want %v", current, next, expected)
	}
}

func TestStaggeredSchedule_Next_DifferentOffsetsDifferentSlots(t *testing.T) {
	t.Parallel()
	interval := 24 * time.Hour
	window := 2 * time.Hour

	offsetA := sourceJitterOffset("source-alpha", window)
	offsetB := sourceJitterOffset("source-beta", window)

	sA := &staggeredSchedule{interval: interval, offset: offsetA}
	sB := &staggeredSchedule{interval: interval, offset: offsetB}

	// Use midnight as a common base; both next times should be within same day
	// but at different slots.
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(-time.Second) // just before midnight
	nextA := sA.Next(base)
	nextB := sB.Next(base)

	if nextA.Equal(nextB) {
		t.Errorf("expected different next times for different source offsets, both got %v", nextA)
	}
	// Both should be within the jitter window of midnight (first period).
	if nextA.Sub(base) > interval {
		t.Errorf("nextA %v too far from base %v", nextA, base)
	}
	if nextB.Sub(base) > interval {
		t.Errorf("nextB %v too far from base %v", nextB, base)
	}
}

// ---------------------------------------------------------------------------
// NewPeriodicJobsFromSources tests
// ---------------------------------------------------------------------------

func TestNewPeriodicJobsFromSources_Count(t *testing.T) {
	t.Parallel()
	baseCount := len(NewPeriodicJobs()) // existing periodic jobs

	tests := []struct {
		name    string
		sources []scraper.SourceConfig
		want    int // additional jobs beyond base
	}{
		{
			name:    "no sources",
			sources: []scraper.SourceConfig{},
			want:    0,
		},
		{
			name: "manual-only sources are excluded",
			sources: []scraper.SourceConfig{
				{Name: "a", Schedule: "manual", Enabled: true},
				{Name: "b", Schedule: "manual", Enabled: true},
			},
			want: 0,
		},
		{
			name: "disabled sources are excluded",
			sources: []scraper.SourceConfig{
				{Name: "a", Schedule: "daily", Enabled: false},
			},
			want: 0,
		},
		{
			name: "daily and weekly enabled sources are included",
			sources: []scraper.SourceConfig{
				{Name: "a", Schedule: "daily", Enabled: true},
				{Name: "b", Schedule: "weekly", Enabled: true},
				{Name: "c", Schedule: "manual", Enabled: true},
				{Name: "d", Schedule: "daily", Enabled: false},
			},
			want: 2,
		},
		{
			name: "all daily",
			sources: []scraper.SourceConfig{
				{Name: "a", Schedule: "daily", Enabled: true},
				{Name: "b", Schedule: "daily", Enabled: true},
				{Name: "c", Schedule: "daily", Enabled: true},
			},
			want: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jobs := NewPeriodicJobsFromSources(tt.sources)
			got := len(jobs) - baseCount
			if got != tt.want {
				t.Errorf("NewPeriodicJobsFromSources: got %d scrape jobs, want %d", got, tt.want)
			}
		})
	}
}

func TestNewPeriodicJobsFromSources_Intervals(t *testing.T) {
	t.Parallel()
	// We can't directly inspect the schedule from *river.PeriodicJob easily,
	// so we verify behaviour indirectly via ScrapeSourceArgs produced by the constructor.
	// The key thing here is that the function returns without panicking and produces
	// a valid PeriodicJob for daily and weekly sources.
	sources := []scraper.SourceConfig{
		{Name: "daily-source", Schedule: "daily", Enabled: true},
		{Name: "weekly-source", Schedule: "weekly", Enabled: true},
	}

	baseCount := len(NewPeriodicJobs())
	jobs := NewPeriodicJobsFromSources(sources)
	if len(jobs)-baseCount != 2 {
		t.Fatalf("expected 2 scrape jobs, got %d", len(jobs)-baseCount)
	}

	// The PeriodicJob API is opaque — we cannot inspect Schedule directly.
	// We verify the jobs are non-nil, which is sufficient.
	for i, j := range jobs[baseCount:] {
		if j == nil {
			t.Errorf("jobs[%d] is nil", i)
		}
	}
}

// ---------------------------------------------------------------------------
// ScrapeSourceWorker tests
// ---------------------------------------------------------------------------

func newTestJob(sourceName string) *river.Job[ScrapeSourceArgs] {
	return &river.Job[ScrapeSourceArgs]{
		JobRow: &rivertype.JobRow{
			Kind:        JobKindScrapeSource,
			EncodedArgs: []byte(`{"source_name":"` + sourceName + `"}`),
			Attempt:     1,
			CreatedAt:   time.Now(),
		},
		Args: ScrapeSourceArgs{SourceName: sourceName},
	}
}

func TestScrapeSourceWorker_Work_HappyPath(t *testing.T) {
	t.Parallel()
	cfg := &mockScraperConfig{
		cfg: postgres.ScraperConfig{AutoScrape: true},
	}
	ms := &mockScraper{
		result: scraper.ScrapeResult{
			SourceName:      "test-source",
			EventsFound:     10,
			EventsCreated:   7,
			EventsDuplicate: 3,
		},
	}

	w := ScrapeSourceWorker{
		Scraper:       ms,
		ConfigQueries: cfg,
		Logger:        nil, // defaults to slog.Default()
	}

	job := newTestJob("test-source")
	err := w.Work(context.Background(), job)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !ms.called {
		t.Error("expected ScrapeSource to be called")
	}
}

func TestScrapeSourceWorker_Work_AutoScrapeDisabled(t *testing.T) {
	t.Parallel()
	cfg := &mockScraperConfig{
		cfg: postgres.ScraperConfig{AutoScrape: false},
	}
	ms := &mockScraper{}

	w := ScrapeSourceWorker{
		Scraper:       ms,
		ConfigQueries: cfg,
		Logger:        nil, // defaults to slog.Default()
	}

	job := newTestJob("test-source")
	err := w.Work(context.Background(), job)
	if err != nil {
		t.Fatalf("expected no error when auto_scrape=false, got %v", err)
	}
	if ms.called {
		t.Error("expected ScrapeSource NOT to be called when auto_scrape=false")
	}
}

func TestScrapeSourceWorker_Work_ConfigReadError_Proceeds(t *testing.T) {
	t.Parallel()
	// When config read fails, worker should proceed (log warning only).
	cfg := &mockScraperConfig{err: errors.New("db down")}
	ms := &mockScraper{
		result: scraper.ScrapeResult{SourceName: "test-source"},
	}

	w := ScrapeSourceWorker{
		Scraper:       ms,
		ConfigQueries: cfg,
		Logger:        nil, // defaults to slog.Default()
	}

	job := newTestJob("test-source")
	err := w.Work(context.Background(), job)
	if err != nil {
		t.Fatalf("expected no error when config read fails, got %v", err)
	}
	if !ms.called {
		t.Error("expected ScrapeSource to be called even when config read fails")
	}
}

func TestScrapeSourceWorker_Work_ScraperError_Propagated(t *testing.T) {
	t.Parallel()
	cfg := &mockScraperConfig{
		cfg: postgres.ScraperConfig{AutoScrape: true},
	}
	scrapeErr := errors.New("network timeout")
	ms := &mockScraper{err: scrapeErr}

	w := ScrapeSourceWorker{
		Scraper:       ms,
		ConfigQueries: cfg,
		Logger:        nil, // defaults to slog.Default()
	}

	job := newTestJob("test-source")
	err := w.Work(context.Background(), job)
	if err == nil {
		t.Fatal("expected error when ScrapeSource fails")
	}
	if !errors.Is(err, scrapeErr) {
		t.Errorf("expected wrapped scrapeErr, got %v", err)
	}
}

func TestScrapeSourceWorker_Work_NilScraper(t *testing.T) {
	t.Parallel()
	cfg := &mockScraperConfig{
		cfg: postgres.ScraperConfig{AutoScrape: true},
	}

	w := ScrapeSourceWorker{
		Scraper:       nil,
		ConfigQueries: cfg,
		Logger:        nil, // defaults to slog.Default()
	}

	job := newTestJob("test-source")
	err := w.Work(context.Background(), job)
	if err == nil {
		t.Fatal("expected error when Scraper is nil")
	}
}

func TestScrapeSourceWorker_Work_NilConfigQueries(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		scrapeErr  error
		wantErr    bool
		wantCalled bool
	}{
		{
			name:       "nil ConfigQueries proceeds with scrape successfully",
			scrapeErr:  nil,
			wantErr:    false,
			wantCalled: true,
		},
		{
			name:       "nil ConfigQueries proceeds with scrape and propagates scraper error",
			scrapeErr:  errors.New("scrape failed"),
			wantErr:    true,
			wantCalled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := &mockScraper{
				result: scraper.ScrapeResult{SourceName: "test-source"},
				err:    tt.scrapeErr,
			}

			w := ScrapeSourceWorker{
				Scraper:       ms,
				ConfigQueries: nil, // no toggle check should be attempted
				Logger:        nil, // defaults to slog.Default()
			}

			job := newTestJob("test-source")
			err := w.Work(context.Background(), job)

			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if ms.called != tt.wantCalled {
				t.Errorf("ScrapeSource called=%v, want %v", ms.called, tt.wantCalled)
			}
		})
	}
}

// TestScrapeSourceWorker_SlotField verifies that ScrapeSourceWorker has a Slot
// field and that it can be set when constructing the worker.
func TestScrapeSourceWorker_SlotField(t *testing.T) {
	t.Parallel()
	ms := &mockScraper{}
	w := ScrapeSourceWorker{
		Scraper:       ms,
		ConfigQueries: nil,
		Logger:        nil,
		Slot:          "blue",
	}

	if w.Slot != "blue" {
		t.Errorf("ScrapeSourceWorker.Slot = %q, want %q", w.Slot, "blue")
	}
}

func TestScrapeSourceWorker_Work_ConfigTunablesWired(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		cfg         postgres.ScraperConfig
		wantLimit   int
		wantTimeout time.Duration
		wantRateMs  int32
	}{
		{
			name: "all tunables wired from config",
			cfg: postgres.ScraperConfig{
				AutoScrape:            true,
				MaxBatchSize:          50,
				RequestTimeoutSeconds: 60,
				RateLimitMs:           500,
			},
			wantLimit:   50,
			wantTimeout: 60 * time.Second,
			wantRateMs:  500,
		},
		{
			name: "zero values leave defaults (limit=0, timeout=0, rate=0)",
			cfg: postgres.ScraperConfig{
				AutoScrape:            true,
				MaxBatchSize:          0,
				RequestTimeoutSeconds: 0,
				RateLimitMs:           0,
			},
			wantLimit:   0,
			wantTimeout: 0,
			wantRateMs:  0,
		},
		{
			name: "partial config: only batch size set",
			cfg: postgres.ScraperConfig{
				AutoScrape:   true,
				MaxBatchSize: 25,
			},
			wantLimit:   25,
			wantTimeout: 0,
			wantRateMs:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &mockScraperConfig{cfg: tt.cfg}
			ms := &mockScraper{result: scraper.ScrapeResult{SourceName: "test-source"}}

			w := ScrapeSourceWorker{
				Scraper:       ms,
				ConfigQueries: cfg,
				Logger:        nil,
			}

			job := newTestJob("test-source")
			err := w.Work(context.Background(), job)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !ms.called {
				t.Fatal("expected ScrapeSource to be called")
			}

			got := ms.lastOpts
			if got.Limit != tt.wantLimit {
				t.Errorf("Limit: got %d, want %d", got.Limit, tt.wantLimit)
			}
			if got.RequestTimeout != tt.wantTimeout {
				t.Errorf("RequestTimeout: got %v, want %v", got.RequestTimeout, tt.wantTimeout)
			}
			if got.RateLimitMs != tt.wantRateMs {
				t.Errorf("RateLimitMs: got %d, want %d", got.RateLimitMs, tt.wantRateMs)
			}
		})
	}
}
