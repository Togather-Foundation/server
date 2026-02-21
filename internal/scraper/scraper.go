package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"

	"github.com/Togather-Foundation/server/internal/domain/events"
	domainScraper "github.com/Togather-Foundation/server/internal/domain/scraper"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
)

// ScrapeOptions controls scraper behaviour.
type ScrapeOptions struct {
	DryRun     bool
	Limit      int    // 0 = no limit
	SourcesDir string // default: "configs/sources"
}

// ScrapeResult holds aggregated outcomes for one scrape run.
type ScrapeResult struct {
	SourceName      string
	SourceURL       string
	Tier            int
	EventsFound     int
	EventsSubmitted int
	EventsCreated   int
	EventsDuplicate int
	EventsFailed    int
	Error           error
	DryRun          bool
}

// Scraper orchestrates fetching, normalising, and ingesting events from
// configured sources.
type Scraper struct {
	ingest     *IngestClient
	queries    *postgres.Queries        // may be nil — DB tracking skipped when nil
	sourceRepo domainScraper.Repository // may be nil — falls back to YAML when nil
	logger     zerolog.Logger
}

// NewScraper constructs a Scraper. queries may be nil; DB run tracking is
// skipped in that case (useful in tests and dry-run contexts).
func NewScraper(ingest *IngestClient, queries *postgres.Queries, logger zerolog.Logger) *Scraper {
	return &Scraper{
		ingest:  ingest,
		queries: queries,
		logger:  logger,
	}
}

// NewScraperWithSourceRepo constructs a Scraper with a DB-backed source
// repository. When sourceRepo is non-nil, ScrapeSource and ScrapeAll load
// configs from the DB first and fall back to YAML only if the DB returns
// empty or an error.
func NewScraperWithSourceRepo(
	ingest *IngestClient,
	queries *postgres.Queries,
	sourceRepo domainScraper.Repository,
	logger zerolog.Logger,
) *Scraper {
	return &Scraper{
		ingest:     ingest,
		queries:    queries,
		sourceRepo: sourceRepo,
		logger:     logger,
	}
}

// loadSourceConfigs returns the active SourceConfig slice. It tries the DB
// repository first (when available); if that yields nothing or fails it falls
// back to loading YAML files from opts.SourcesDir.
func (s *Scraper) loadSourceConfigs(ctx context.Context, opts ScrapeOptions) ([]SourceConfig, error) {
	if s.sourceRepo != nil {
		t := true
		sources, err := s.sourceRepo.List(ctx, &t) // enabled only
		if err != nil {
			s.logger.Warn().Err(err).Msg("scraper: DB source list failed, falling back to YAML")
		} else if len(sources) > 0 {
			configs := make([]SourceConfig, 0, len(sources))
			for _, src := range sources {
				cfg, convErr := dbSourceToSourceConfig(src)
				if convErr != nil {
					s.logger.Warn().Err(convErr).Str("source", src.Name).
						Msg("scraper: skipping DB source with conversion error")
					continue
				}
				configs = append(configs, cfg)
			}
			if len(configs) > 0 {
				return configs, nil
			}
		}
	}

	// Fall back to YAML.
	dir := opts.SourcesDir
	if dir == "" {
		dir = "configs/sources"
	}
	return LoadSourceConfigs(dir)
}

// ScrapeURL fetches rawURL, extracts JSON-LD events, normalises them, and
// either submits or dry-runs the batch. The source name is derived from the
// URL hostname.
func (s *Scraper) ScrapeURL(ctx context.Context, rawURL string, opts ScrapeOptions) (ScrapeResult, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return ScrapeResult{Error: err}, nil
	}
	sourceName := parsedURL.Hostname()

	source := SourceConfig{
		Name:       sourceName,
		URL:        rawURL,
		Tier:       0,
		TrustLevel: 5,
	}

	result := ScrapeResult{
		SourceName: sourceName,
		SourceURL:  rawURL,
		Tier:       0,
		DryRun:     opts.DryRun,
	}

	// Insert scraper run record (best-effort).
	var runID int64
	if s.queries != nil {
		params := postgres.InsertScraperRunParams{
			SourceName: result.SourceName,
			SourceUrl:  result.SourceURL,
			Tier:       int32(result.Tier),
		}
		id, insertErr := s.queries.InsertScraperRun(ctx, params)
		if insertErr != nil {
			s.logger.Warn().Err(insertErr).Msg("scraper: failed to insert scraper run")
		} else {
			runID = id
		}
	}

	rawEvents, err := FetchAndExtractJSONLD(ctx, rawURL)
	if err != nil {
		result.Error = err
		if s.queries != nil && runID != 0 {
			params := postgres.UpdateScraperRunFailedParams{
				ID:           runID,
				ErrorMessage: pgtype.Text{String: err.Error(), Valid: true},
			}
			if err2 := s.queries.UpdateScraperRunFailed(ctx, params); err2 != nil {
				s.logger.Warn().Err(err2).Msg("scraper: failed to update scraper run failure")
			}
		}
		return result, nil
	}

	result.EventsFound = len(rawEvents)

	validEvents, skipped := s.normalizeJSONLDEvents(rawEvents, source, opts.Limit)
	result.EventsSubmitted = len(validEvents)

	if skipped > 0 {
		s.logger.Warn().
			Str("source", sourceName).
			Int("skipped", skipped).
			Msg("scraper: events skipped during normalisation")
	}

	if len(validEvents) == 0 {
		if s.queries != nil && runID != 0 {
			params := postgres.UpdateScraperRunCompletedParams{
				ID:           runID,
				EventsFound:  int32(result.EventsFound),
				EventsNew:    0,
				EventsDup:    0,
				EventsFailed: 0,
			}
			if err2 := s.queries.UpdateScraperRunCompleted(ctx, params); err2 != nil {
				s.logger.Warn().Err(err2).Msg("scraper: failed to update scraper run")
			}
		}
		return result, nil
	}

	ingestResult, err := s.submitEvents(ctx, validEvents, opts.DryRun)
	if err != nil {
		result.Error = err
		if s.queries != nil && runID != 0 {
			params := postgres.UpdateScraperRunFailedParams{
				ID:           runID,
				ErrorMessage: pgtype.Text{String: err.Error(), Valid: true},
			}
			if err2 := s.queries.UpdateScraperRunFailed(ctx, params); err2 != nil {
				s.logger.Warn().Err(err2).Msg("scraper: failed to update scraper run failure")
			}
		}
		return result, nil
	}

	result.EventsCreated = ingestResult.EventsCreated
	result.EventsDuplicate = ingestResult.EventsDuplicate
	result.EventsFailed = ingestResult.EventsFailed

	if s.queries != nil && runID != 0 {
		params := postgres.UpdateScraperRunCompletedParams{
			ID:           runID,
			EventsFound:  int32(result.EventsFound),
			EventsNew:    int32(result.EventsCreated),
			EventsDup:    int32(result.EventsDuplicate),
			EventsFailed: int32(result.EventsFailed),
		}
		if err2 := s.queries.UpdateScraperRunCompleted(ctx, params); err2 != nil {
			s.logger.Warn().Err(err2).Msg("scraper: failed to update scraper run")
		}
	}

	return result, nil
}

// ScrapeSource loads source configs (DB-first, YAML fallback), locates the
// named source, and scrapes it according to its Tier.
func (s *Scraper) ScrapeSource(ctx context.Context, sourceName string, opts ScrapeOptions) (ScrapeResult, error) {
	if opts.SourcesDir == "" {
		opts.SourcesDir = "configs/sources"
	}

	configs, err := s.loadSourceConfigs(ctx, opts)
	if err != nil {
		return ScrapeResult{}, fmt.Errorf("loading source configs: %w", err)
	}

	var found *SourceConfig
	for i := range configs {
		if strings.EqualFold(configs[i].Name, sourceName) {
			found = &configs[i]
			break
		}
	}

	if found == nil {
		return ScrapeResult{}, fmt.Errorf("source not found: %s", sourceName)
	}

	if !found.Enabled {
		return ScrapeResult{}, fmt.Errorf("source is disabled: %s", sourceName)
	}

	switch found.Tier {
	case 0:
		return s.scrapeTier0(ctx, *found, opts)
	case 1:
		return s.scrapeTier1(ctx, *found, opts)
	default:
		return ScrapeResult{}, fmt.Errorf("unknown tier %d for source %s", found.Tier, sourceName)
	}
}

// ScrapeAll loads all enabled sources (DB-first, YAML fallback) and scrapes
// each one, collecting results. Per-source errors are recorded in each
// ScrapeResult.Error rather than aborting the entire run.
func (s *Scraper) ScrapeAll(ctx context.Context, opts ScrapeOptions) ([]ScrapeResult, error) {
	if opts.SourcesDir == "" {
		opts.SourcesDir = "configs/sources"
	}

	configs, err := s.loadSourceConfigs(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("loading source configs: %w", err)
	}

	var results []ScrapeResult
	for _, cfg := range configs {
		if ctx.Err() != nil {
			break
		}
		if !cfg.Enabled {
			continue
		}
		var (
			res       ScrapeResult
			scrapeErr error
		)
		switch cfg.Tier {
		case 0:
			res, scrapeErr = s.scrapeTier0(ctx, cfg, opts)
		case 1:
			res, scrapeErr = s.scrapeTier1(ctx, cfg, opts)
		default:
			scrapeErr = fmt.Errorf("unknown tier %d for source %s", cfg.Tier, cfg.Name)
		}
		if scrapeErr != nil {
			res.Error = scrapeErr
		}
		results = append(results, res)
	}

	return results, nil
}

// scrapeTier1 fetches and processes a Tier 1 (Colly CSS-selector) source.
func (s *Scraper) scrapeTier1(ctx context.Context, source SourceConfig, opts ScrapeOptions) (ScrapeResult, error) {
	result := ScrapeResult{
		SourceName: source.Name,
		SourceURL:  source.URL,
		Tier:       1,
		DryRun:     opts.DryRun,
	}

	// Insert scraper run record (best-effort).
	var runID int64
	if s.queries != nil {
		params := postgres.InsertScraperRunParams{
			SourceName: result.SourceName,
			SourceUrl:  result.SourceURL,
			Tier:       int32(result.Tier),
		}
		id, insertErr := s.queries.InsertScraperRun(ctx, params)
		if insertErr != nil {
			s.logger.Warn().Err(insertErr).Msg("scraper: failed to insert scraper run")
		} else {
			runID = id
		}
	}

	extractor := NewCollyExtractor(s.logger)
	rawEvents, err := extractor.ScrapeWithSelectors(ctx, source)
	if err != nil {
		result.Error = err
		if s.queries != nil && runID != 0 {
			params := postgres.UpdateScraperRunFailedParams{
				ID:           runID,
				ErrorMessage: pgtype.Text{String: err.Error(), Valid: true},
			}
			if err2 := s.queries.UpdateScraperRunFailed(ctx, params); err2 != nil {
				s.logger.Warn().Err(err2).Msg("scraper: failed to update scraper run failure")
			}
		}
		return result, nil
	}

	result.EventsFound = len(rawEvents)

	limit := opts.Limit
	var validEvents []events.EventInput
	skipped := 0

	for i, raw := range rawEvents {
		if limit > 0 && i >= limit {
			break
		}
		input, normErr := NormalizeRawEvent(raw, source)
		if normErr != nil {
			s.logger.Warn().
				Str("source", source.Name).
				Err(normErr).
				Msg("scraper: skipping raw event that failed normalisation")
			skipped++
			continue
		}
		validEvents = append(validEvents, input)
	}

	result.EventsSubmitted = len(validEvents)

	if skipped > 0 {
		s.logger.Warn().
			Str("source", source.Name).
			Int("skipped", skipped).
			Msg("scraper: tier 1 events skipped during normalisation")
	}

	if len(validEvents) == 0 {
		if s.queries != nil && runID != 0 {
			params := postgres.UpdateScraperRunCompletedParams{
				ID:           runID,
				EventsFound:  int32(result.EventsFound),
				EventsNew:    0,
				EventsDup:    0,
				EventsFailed: 0,
			}
			if err2 := s.queries.UpdateScraperRunCompleted(ctx, params); err2 != nil {
				s.logger.Warn().Err(err2).Msg("scraper: failed to update scraper run")
			}
		}
		return result, nil
	}

	ingestResult, err := s.submitEvents(ctx, validEvents, opts.DryRun)
	if err != nil {
		result.Error = err
		if s.queries != nil && runID != 0 {
			params := postgres.UpdateScraperRunFailedParams{
				ID:           runID,
				ErrorMessage: pgtype.Text{String: err.Error(), Valid: true},
			}
			if err2 := s.queries.UpdateScraperRunFailed(ctx, params); err2 != nil {
				s.logger.Warn().Err(err2).Msg("scraper: failed to update scraper run failure")
			}
		}
		return result, nil
	}

	result.EventsCreated = ingestResult.EventsCreated
	result.EventsDuplicate = ingestResult.EventsDuplicate
	result.EventsFailed = ingestResult.EventsFailed

	if s.queries != nil && runID != 0 {
		params := postgres.UpdateScraperRunCompletedParams{
			ID:           runID,
			EventsFound:  int32(result.EventsFound),
			EventsNew:    int32(result.EventsCreated),
			EventsDup:    int32(result.EventsDuplicate),
			EventsFailed: int32(result.EventsFailed),
		}
		if err2 := s.queries.UpdateScraperRunCompleted(ctx, params); err2 != nil {
			s.logger.Warn().Err(err2).Msg("scraper: failed to update scraper run")
		}
	}

	return result, nil
}

// scrapeTier0 fetches and processes a Tier 0 (JSON-LD) source.
func (s *Scraper) scrapeTier0(ctx context.Context, source SourceConfig, opts ScrapeOptions) (ScrapeResult, error) {
	result := ScrapeResult{
		SourceName: source.Name,
		SourceURL:  source.URL,
		Tier:       0,
		DryRun:     opts.DryRun,
	}

	// Insert scraper run record (best-effort).
	var runID int64
	if s.queries != nil {
		params := postgres.InsertScraperRunParams{
			SourceName: result.SourceName,
			SourceUrl:  result.SourceURL,
			Tier:       int32(result.Tier),
		}
		id, insertErr := s.queries.InsertScraperRun(ctx, params)
		if insertErr != nil {
			s.logger.Warn().Err(insertErr).Msg("scraper: failed to insert scraper run")
		} else {
			runID = id
		}
	}

	rawEvents, err := FetchAndExtractJSONLD(ctx, source.URL)
	if err != nil {
		result.Error = err
		if s.queries != nil && runID != 0 {
			params := postgres.UpdateScraperRunFailedParams{
				ID:           runID,
				ErrorMessage: pgtype.Text{String: err.Error(), Valid: true},
			}
			if err2 := s.queries.UpdateScraperRunFailed(ctx, params); err2 != nil {
				s.logger.Warn().Err(err2).Msg("scraper: failed to update scraper run failure")
			}
		}
		return result, nil
	}

	result.EventsFound = len(rawEvents)

	validEvents, skipped := s.normalizeJSONLDEvents(rawEvents, source, opts.Limit)
	result.EventsSubmitted = len(validEvents)

	if skipped > 0 {
		s.logger.Warn().
			Str("source", source.Name).
			Int("skipped", skipped).
			Msg("scraper: events skipped during normalisation")
	}

	if len(validEvents) == 0 {
		if s.queries != nil && runID != 0 {
			params := postgres.UpdateScraperRunCompletedParams{
				ID:           runID,
				EventsFound:  int32(result.EventsFound),
				EventsNew:    0,
				EventsDup:    0,
				EventsFailed: 0,
			}
			if err2 := s.queries.UpdateScraperRunCompleted(ctx, params); err2 != nil {
				s.logger.Warn().Err(err2).Msg("scraper: failed to update scraper run")
			}
		}
		return result, nil
	}

	ingestResult, err := s.submitEvents(ctx, validEvents, opts.DryRun)
	if err != nil {
		result.Error = err
		if s.queries != nil && runID != 0 {
			params := postgres.UpdateScraperRunFailedParams{
				ID:           runID,
				ErrorMessage: pgtype.Text{String: err.Error(), Valid: true},
			}
			if err2 := s.queries.UpdateScraperRunFailed(ctx, params); err2 != nil {
				s.logger.Warn().Err(err2).Msg("scraper: failed to update scraper run failure")
			}
		}
		return result, nil
	}

	result.EventsCreated = ingestResult.EventsCreated
	result.EventsDuplicate = ingestResult.EventsDuplicate
	result.EventsFailed = ingestResult.EventsFailed

	if s.queries != nil && runID != 0 {
		params := postgres.UpdateScraperRunCompletedParams{
			ID:           runID,
			EventsFound:  int32(result.EventsFound),
			EventsNew:    int32(result.EventsCreated),
			EventsDup:    int32(result.EventsDuplicate),
			EventsFailed: int32(result.EventsFailed),
		}
		if err2 := s.queries.UpdateScraperRunCompleted(ctx, params); err2 != nil {
			s.logger.Warn().Err(err2).Msg("scraper: failed to update scraper run")
		}
	}

	return result, nil
}

// normalizeJSONLDEvents converts raw JSON-LD messages to EventInputs, applying
// opts.Limit to the number processed. Returns valid events and the count of
// skipped (failed) events.
func (s *Scraper) normalizeJSONLDEvents(rawEvents []json.RawMessage, source SourceConfig, limit int) ([]events.EventInput, int) {
	toProcess := rawEvents
	if limit > 0 && len(toProcess) > limit {
		toProcess = toProcess[:limit]
	}

	var valid []events.EventInput
	skipped := 0

	for _, raw := range toProcess {
		evt, err := NormalizeJSONLDEvent(raw, source)
		if err != nil {
			s.logger.Debug().
				Str("source", source.Name).
				Err(err).
				Msg("scraper: skipping event that failed normalisation")
			skipped++
			continue
		}
		valid = append(valid, evt)
	}

	return valid, skipped
}

// submitEvents calls either SubmitBatch or SubmitBatchDryRun depending on
// the dryRun flag.
func (s *Scraper) submitEvents(ctx context.Context, evts []events.EventInput, dryRun bool) (IngestResult, error) {
	if dryRun {
		return s.ingest.SubmitBatchDryRun(ctx, evts)
	}
	return s.ingest.SubmitBatch(ctx, evts)
}
