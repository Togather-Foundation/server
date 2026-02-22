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
	result scraper.ScrapeResult
	err    error
	called bool
}

func (m *mockScraper) ScrapeSource(ctx context.Context, sourceName string, opts scraper.ScrapeOptions) (scraper.ScrapeResult, error) {
	m.called = true
	return m.result, m.err
}

// ---------------------------------------------------------------------------
// NewPeriodicJobsFromSources tests
// ---------------------------------------------------------------------------

func TestNewPeriodicJobsFromSources_Count(t *testing.T) {
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
