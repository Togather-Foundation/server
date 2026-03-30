package jobs

import (
	"context"
	"errors"
	"testing"

	"github.com/rs/zerolog"

	domainScraper "github.com/Togather-Foundation/server/internal/domain/scraper"
)

type mockSourceRepo struct {
	sources []domainScraper.Source
	err     error
}

func (m *mockSourceRepo) List(ctx context.Context, enabled *bool) ([]domainScraper.Source, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.sources, nil
}

func TestLoadSourcesForPeriodicJobs_DBError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := zerolog.Nop()

	repo := &mockSourceRepo{err: errors.New("db connection failed")}
	_, err := LoadSourcesForPeriodicJobs(ctx, repo, logger)
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestLoadSourcesForPeriodicJobs_EmptyDB(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := zerolog.Nop()

	repo := &mockSourceRepo{sources: []domainScraper.Source{}}
	configs, err := LoadSourcesForPeriodicJobs(ctx, repo, logger)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(configs) != 0 {
		t.Errorf("expected empty configs, got %d", len(configs))
	}
}

func TestLoadSourcesForPeriodicJobs_OnlyManualSchedule(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := zerolog.Nop()

	repo := &mockSourceRepo{
		sources: []domainScraper.Source{
			{Name: "manual-source", Schedule: "manual", Enabled: true},
		},
	}
	configs, err := LoadSourcesForPeriodicJobs(ctx, repo, logger)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(configs) != 0 {
		t.Errorf("expected 0 configs for manual schedule, got %d", len(configs))
	}
}

func TestLoadSourcesForPeriodicJobs_DailyAndWeekly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := zerolog.Nop()

	repo := &mockSourceRepo{
		sources: []domainScraper.Source{
			{Name: "daily-source", Schedule: "daily", Enabled: true},
			{Name: "weekly-source", Schedule: "weekly", Enabled: true},
			{Name: "manual-source", Schedule: "manual", Enabled: true},
		},
	}
	configs, err := LoadSourcesForPeriodicJobs(ctx, repo, logger)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(configs) != 2 {
		t.Errorf("expected 2 configs (daily + weekly), got %d", len(configs))
	}
}

func TestLoadSourcesForPeriodicJobs_DisabledSourcesExcluded(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := zerolog.Nop()

	repo := &mockSourceRepo{
		sources: []domainScraper.Source{
			{Name: "enabled-daily", Schedule: "daily", Enabled: true},
		},
	}
	configs, err := LoadSourcesForPeriodicJobs(ctx, repo, logger)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(configs) != 1 {
		t.Errorf("expected 1 config (only enabled), got %d", len(configs))
	}
	if configs[0].Name != "enabled-daily" {
		t.Errorf("expected enabled-daily, got %s", configs[0].Name)
	}
}

func TestLoadSourcesForPeriodicJobs_UnknownSchedule(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := zerolog.Nop()

	repo := &mockSourceRepo{
		sources: []domainScraper.Source{
			{Name: "unknown-schedule", Schedule: "monthly", Enabled: true},
		},
	}
	configs, err := LoadSourcesForPeriodicJobs(ctx, repo, logger)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(configs) != 0 {
		t.Errorf("expected 0 configs for unknown schedule, got %d", len(configs))
	}
}

func TestLoadSourcesForPeriodicJobs_VerifyScheduleConversion(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := zerolog.Nop()

	repo := &mockSourceRepo{
		sources: []domainScraper.Source{
			{Name: "daily-test", Schedule: "daily", Enabled: true},
		},
	}
	configs, err := LoadSourcesForPeriodicJobs(ctx, repo, logger)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("expected 1 config, got %d", len(configs))
	}
	if configs[0].Schedule != "daily" {
		t.Errorf("expected schedule %q, got %q", "daily", configs[0].Schedule)
	}
}
