package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"

	"github.com/Togather-Foundation/server/internal/domain/events"
	domainScraper "github.com/Togather-Foundation/server/internal/domain/scraper"
	"github.com/Togather-Foundation/server/internal/metrics"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
)

// ScrapeOptions controls scraper behaviour.
type ScrapeOptions struct {
	DryRun           bool
	Limit            int               // 0 = no limit
	SourcesDir       string            // default: "configs/sources"
	SourceFile       string            // if set, load a single YAML config from this path (bypasses DB and SourcesDir)
	TierFilter       int               // -1 = all tiers; 0, 1, … = restrict to that tier
	Transport        http.RoundTripper // optional custom transport (e.g. CachingTransport); nil = http.DefaultTransport
	RequestTimeout   time.Duration     // 0 = use the fetchTimeout package const
	RateLimitMs      int32             // 0 = use CollyExtractor default (1 s); >0 overrides per-domain delay
	HeadlessOverride bool              // if true and rodExtractor is configured, ScrapeURL uses Tier 2 headless path
}

// HTTPClient returns an http.Client using the configured transport (if any).
// When RequestTimeout is non-zero it is used as the client timeout; otherwise
// the provided fallback is used. Used by all scraper HTTP code to ensure
// consistent transport usage.
func (o ScrapeOptions) HTTPClient(fallback time.Duration) *http.Client {
	transport := o.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	timeout := fallback
	if o.RequestTimeout > 0 {
		timeout = o.RequestTimeout
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
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
	IngestErrors    []IngestError // per-event errors returned by the batch API
	Error           error
	DryRun          bool
}

// Scraper orchestrates fetching, normalising, and ingesting events from
// configured sources.
type Scraper struct {
	ingest         *IngestClient
	queries        *postgres.Queries        // may be nil — DB tracking skipped when nil
	sourceRepo     domainScraper.Repository // may be nil — falls back to YAML when nil
	logger         zerolog.Logger
	slot           string                  // deployment slot for Prometheus metrics labeling; empty = no metrics
	scraperMetrics *metrics.ScraperMetrics // may be nil — falls back to package-level globals
	rodExtractor   *RodExtractor           // nil when headless is disabled/unconfigured
}

// NewScraper constructs a Scraper. queries may be nil; DB run tracking is
// skipped in that case (useful in tests and dry-run contexts).
func NewScraper(ingest *IngestClient, queries *postgres.Queries, logger zerolog.Logger) *Scraper {
	return NewScraperWithSlot(ingest, queries, logger, "")
}

// NewScraperWithSlot constructs a Scraper with an explicit deployment slot for
// Prometheus metrics labeling. When slot is empty, no metrics are recorded.
func NewScraperWithSlot(ingest *IngestClient, queries *postgres.Queries, logger zerolog.Logger, slot string) *Scraper {
	var sm *metrics.ScraperMetrics
	if slot != "" {
		sm = metrics.NewScraperMetrics(metrics.Registry)
	}
	return &Scraper{
		ingest:         ingest,
		queries:        queries,
		logger:         logger,
		slot:           slot,
		scraperMetrics: sm,
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
	return NewScraperWithSourceRepoAndSlot(ingest, queries, sourceRepo, logger, "")
}

// NewScraperWithSourceRepoAndSlot constructs a Scraper with a DB-backed source
// repository and an explicit deployment slot for Prometheus metrics labeling.
// When slot is empty, no metrics are recorded.
func NewScraperWithSourceRepoAndSlot(
	ingest *IngestClient,
	queries *postgres.Queries,
	sourceRepo domainScraper.Repository,
	logger zerolog.Logger,
	slot string,
) *Scraper {
	var sm *metrics.ScraperMetrics
	if slot != "" {
		sm = metrics.NewScraperMetrics(metrics.Registry)
	}
	return &Scraper{
		ingest:         ingest,
		queries:        queries,
		sourceRepo:     sourceRepo,
		logger:         logger,
		slot:           slot,
		scraperMetrics: sm,
	}
}

// SetRodExtractor sets the Tier 2 headless browser extractor. Call after
// construction to enable Tier 2 scraping. When r is nil, Tier 2 sources
// return an error describing that headless scraping is unconfigured.
func (s *Scraper) SetRodExtractor(r *RodExtractor) {
	s.rodExtractor = r
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
				cfg, convErr := SourceConfigFromDomain(src)
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
//
// When opts.HeadlessOverride is true and a RodExtractor is configured, the URL
// is scraped using the Tier 2 headless browser path instead (with WaitSelector
// defaulting to "body"). This is intended for CLI use only.
//
// NOTE: The hostname is used as the Prometheus "source" label. This is safe
// today because ScrapeURL is only called from the CLI (bounded set of URLs).
// Do NOT expose this method from a user-facing HTTP endpoint with
// operator-supplied URLs — the label would become unbounded and cause
// Prometheus memory growth. If that call-site is ever added, normalise the
// label (e.g. strip subdomains, cap length) or use a fixed "ad_hoc" value.
func (s *Scraper) ScrapeURL(ctx context.Context, rawURL string, opts ScrapeOptions) (ScrapeResult, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return ScrapeResult{Error: err}, nil
	}
	sourceName := parsedURL.Hostname()

	// When --headless is requested, route to Tier 2.
	if opts.HeadlessOverride {
		source := SourceConfig{
			Name:       sourceName,
			URL:        rawURL,
			Tier:       2,
			TrustLevel: 5,
			Headless: HeadlessConfig{
				WaitSelector: "body",
			},
		}
		return s.scrapeTier2(ctx, source, opts)
	}

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

	return s.runWithTracking(ctx, &result, func(ctx context.Context) (int, []events.EventInput, error) {
		rawEvents, err := FetchAndExtractJSONLD(ctx, rawURL, opts.HTTPClient(fetchTimeout))
		if err != nil {
			return 0, nil, err
		}
		valid, skipped := s.normalizeJSONLDEvents(rawEvents, source, opts.Limit)
		if skipped > 0 {
			s.logger.Warn().Str("source", sourceName).Int("skipped", skipped).
				Msg("scraper: events skipped during normalisation")
		}
		return len(rawEvents), valid, nil
	}), nil
}

// ScrapeSource loads source configs (DB-first, YAML fallback), locates the
// named source, and scrapes it according to its Tier.
//
// When opts.SourceFile is set the config is loaded directly from that YAML
// file, bypassing the DB and SourcesDir lookup entirely. The source is run
// regardless of its enabled flag (useful for testing disabled or draft configs).
// sourceName may be empty when SourceFile is set — the name is taken from the
// file's name field.
func (s *Scraper) ScrapeSource(ctx context.Context, sourceName string, opts ScrapeOptions) (ScrapeResult, error) {
	var found *SourceConfig

	if opts.SourceFile != "" {
		cfg, err := LoadSourceConfig(opts.SourceFile)
		if err != nil {
			return ScrapeResult{}, fmt.Errorf("loading source file %q: %w", opts.SourceFile, err)
		}
		found = &cfg
	} else {
		if opts.SourcesDir == "" {
			opts.SourcesDir = "configs/sources"
		}

		configs, err := s.loadSourceConfigs(ctx, opts)
		if err != nil {
			return ScrapeResult{}, fmt.Errorf("loading source configs: %w", err)
		}

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
	}

	// --headless flag: override source tier to 2 and set a default WaitSelector.
	if opts.HeadlessOverride && found.Tier != 2 {
		found.Tier = 2
		if found.Headless.WaitSelector == "" {
			found.Headless.WaitSelector = "body"
		}
	}

	switch found.Tier {
	case 0:
		return s.scrapeTier0(ctx, *found, opts)
	case 1:
		return s.scrapeTier1(ctx, *found, opts)
	case 2:
		return s.scrapeTier2(ctx, *found, opts)
	case 3:
		return s.scrapeTier3(ctx, *found, opts)
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
	for i, cfg := range configs {
		if ctx.Err() != nil {
			break
		}
		// Skip disabled sources. DB-loaded configs are pre-filtered (loadSourceConfigs
		// passes enabled=true), but the YAML fallback returns all sources regardless
		// of enabled state, so this guard is required for correctness.
		if !cfg.Enabled {
			continue
		}
		// Apply tier filter when set (non-negative value).
		if opts.TierFilter >= 0 && cfg.Tier != opts.TierFilter {
			continue
		}

		s.logger.Info().
			Str("source", cfg.Name).
			Int("tier", cfg.Tier).
			Int("index", i+1).
			Int("total", len(configs)).
			Msg("scraper: starting source")

		var (
			res       ScrapeResult
			scrapeErr error
		)
		switch cfg.Tier {
		case 0:
			res, scrapeErr = s.scrapeTier0(ctx, cfg, opts)
		case 1:
			res, scrapeErr = s.scrapeTier1(ctx, cfg, opts)
		case 2:
			res, scrapeErr = s.scrapeTier2(ctx, cfg, opts)
		case 3:
			res, scrapeErr = s.scrapeTier3(ctx, cfg, opts)
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

	return s.runWithTracking(ctx, &result, func(ctx context.Context) (int, []events.EventInput, error) {
		extractor := NewCollyExtractor(s.logger)
		extractor.SetTransport(opts.Transport)
		if opts.RateLimitMs > 0 {
			extractor.SetRateLimit(time.Duration(opts.RateLimitMs) * time.Millisecond)
		}

		var allRaw []RawEvent
		urlList := source.GetURLs()
		failCount := 0
		for _, u := range urlList {
			clone := source
			clone.URL = u
			rawEvts, fetchErr := extractor.ScrapeWithSelectors(ctx, clone)
			if fetchErr != nil {
				s.logger.Warn().Str("source", source.Name).Str("url", u).Err(fetchErr).
					Msg("scraper: tier 1 URL failed, continuing")
				failCount++
				continue
			}
			allRaw = append(allRaw, rawEvts...)
		}
		// failCount > 0 guards against the empty-list case (0 == 0 would fire spuriously).
		if failCount > 0 && failCount == len(urlList) {
			s.logger.Warn().Str("source", source.Name).Int("urls", len(urlList)).
				Msg("scraper: all URLs failed for source — returning 0 events")
		}
		rawEvents := allRaw

		limit := opts.Limit
		var validEvents []events.EventInput
		skipped := 0

		for i, raw := range rawEvents {
			if limit > 0 && i >= limit {
				break
			}
			input, normErr := NormalizeRawEvent(raw, source)
			if normErr != nil {
				s.logger.Warn().Str("source", source.Name).Err(normErr).
					Msg("scraper: skipping raw event that failed normalisation")
				skipped++
				continue
			}
			validEvents = append(validEvents, input)
		}

		if skipped > 0 {
			s.logger.Warn().Str("source", source.Name).Int("skipped", skipped).
				Msg("scraper: tier 1 events skipped during normalisation")
		}

		return len(rawEvents), validEvents, nil
	}), nil
}

// scrapeTier2 fetches and processes a Tier 2 (headless browser) source.
func (s *Scraper) scrapeTier2(ctx context.Context, source SourceConfig, opts ScrapeOptions) (ScrapeResult, error) {
	result := ScrapeResult{
		SourceName: source.Name,
		SourceURL:  source.URL,
		Tier:       2,
		DryRun:     opts.DryRun,
	}

	if s.rodExtractor == nil {
		result.Error = fmt.Errorf("tier 2 scraping requires a RodExtractor (set SCRAPER_HEADLESS_ENABLED=true)")
		return result, nil
	}

	return s.runWithTracking(ctx, &result, func(ctx context.Context) (int, []events.EventInput, error) {
		var allRaw []RawEvent
		urlList := source.GetURLs()
		failCount := 0
		for _, u := range urlList {
			clone := source
			clone.URL = u
			rawEvts, fetchErr := s.rodExtractor.ScrapeWithBrowser(ctx, clone)
			if fetchErr != nil {
				s.logger.Warn().Str("source", source.Name).Str("url", u).Err(fetchErr).
					Msg("scraper: tier 2 URL failed, continuing")
				failCount++
				continue
			}
			allRaw = append(allRaw, rawEvts...)
		}
		// failCount > 0 guards against the empty-list case (0 == 0 would fire spuriously).
		if failCount > 0 && failCount == len(urlList) {
			s.logger.Warn().Str("source", source.Name).Int("urls", len(urlList)).
				Msg("scraper: all URLs failed for source — returning 0 events")
		}
		rawEvents := allRaw

		var validEvents []events.EventInput
		skipped := 0
		limit := opts.Limit

		for i, raw := range rawEvents {
			if limit > 0 && i >= limit {
				break
			}
			input, normErr := NormalizeRawEvent(raw, source)
			if normErr != nil {
				s.logger.Warn().Str("source", source.Name).Err(normErr).
					Msg("scraper: skipping raw event that failed normalisation (tier 2)")
				skipped++
				continue
			}
			validEvents = append(validEvents, input)
		}

		if skipped > 0 {
			s.logger.Warn().Str("source", source.Name).Int("skipped", skipped).
				Msg("scraper: tier 2 events skipped during normalisation")
		}

		return len(rawEvents), validEvents, nil
	}), nil
}

// scrapeTier0 fetches and processes a Tier 0 (JSON-LD) source.
func (s *Scraper) scrapeTier0(ctx context.Context, source SourceConfig, opts ScrapeOptions) (ScrapeResult, error) {
	result := ScrapeResult{
		SourceName: source.Name,
		SourceURL:  source.URL,
		Tier:       0,
		DryRun:     opts.DryRun,
	}

	return s.runWithTracking(ctx, &result, func(ctx context.Context) (int, []events.EventInput, error) {
		var allRawEvents []json.RawMessage
		urlList := source.GetURLs()
		failCount := 0
		for _, u := range urlList {
			rawEvts, fetchErr := FetchAndExtractJSONLD(ctx, u, opts.HTTPClient(fetchTimeout))
			if fetchErr != nil {
				s.logger.Warn().Str("source", source.Name).Str("url", u).Err(fetchErr).
					Msg("scraper: tier 0 URL failed, continuing")
				failCount++
				continue
			}
			allRawEvents = append(allRawEvents, rawEvts...)
		}
		// failCount > 0 guards against the empty-list case (0 == 0 would fire spuriously).
		if failCount > 0 && failCount == len(urlList) {
			s.logger.Warn().Str("source", source.Name).Int("urls", len(urlList)).
				Msg("scraper: all URLs failed for source — returning 0 events")
		}
		rawEvents := allRawEvents

		valid, skipped := s.normalizeJSONLDEvents(rawEvents, source, opts.Limit)
		if skipped > 0 {
			s.logger.Warn().Str("source", source.Name).Int("skipped", skipped).
				Msg("scraper: events skipped during normalisation")
		}

		// Follow individual event URLs to fetch full descriptions (e.g. Tribe Events /
		// WordPress sources that truncate descriptions in the listing page JSON-LD).
		if source.FollowEventURLs {
			for i := range valid {
				if i > 0 {
					select {
					case <-ctx.Done():
						return len(rawEvents), valid, nil
					case <-time.After(500 * time.Millisecond):
					}
				}
				evt := &valid[i]
				if HasTruncatedDescription(evt.Description) && evt.URL != "" {
					full := FetchFullDescription(ctx, evt.URL, opts.HTTPClient(fetchTimeout))
					if len([]rune(full)) > len([]rune(evt.Description)) {
						s.logger.Debug().
							Str("source", source.Name).
							Str("url", evt.URL).
							Int("old_len", len([]rune(evt.Description))).
							Int("new_len", len([]rune(full))).
							Msg("scraper: replaced truncated description with full text")
						evt.Description = full
					}
				}
				// After follow-URL step: if description is still truncated, route to review.
				if HasTruncatedDescription(evt.Description) {
					s.logger.Debug().
						Str("source", source.Name).
						Str("url", evt.URL).
						Msg("scraper: description still truncated after follow, routing to review")
					evt.LifecycleState = "review"
				}
			}
		}

		return len(rawEvents), valid, nil
	}), nil
}

// scrapeTier3 fetches and processes a Tier 3 (GraphQL API) source.
func (s *Scraper) scrapeTier3(ctx context.Context, source SourceConfig, opts ScrapeOptions) (ScrapeResult, error) {
	result := ScrapeResult{
		SourceName: source.Name,
		SourceURL:  source.URL,
		Tier:       3,
		DryRun:     opts.DryRun,
	}

	extractor := NewGraphQLExtractor(s.logger)

	return s.runWithTracking(ctx, &result, func(ctx context.Context) (int, []events.EventInput, error) {
		// Tier 3 uses the single cfg.GraphQL.Endpoint rather than iterating GetURLs().
		// source.URL is stored as metadata (SourceURL in scraper_run records) but
		// is not used for fetching — only Endpoint drives the HTTP request.
		rawEvents, err := extractor.FetchAndExtractGraphQL(ctx, source, opts.HTTPClient(fetchTimeout))
		if err != nil {
			return 0, nil, err
		}

		var validEvents []events.EventInput
		skipped := 0
		limit := opts.Limit

		for i, raw := range rawEvents {
			if limit > 0 && i >= limit {
				break
			}
			input, normErr := NormalizeRawEvent(raw, source)
			if normErr != nil {
				s.logger.Warn().Str("source", source.Name).Err(normErr).
					Msg("scraper: skipping raw event that failed normalisation (tier 3)")
				skipped++
				continue
			}
			validEvents = append(validEvents, input)
		}

		if skipped > 0 {
			s.logger.Warn().Str("source", source.Name).Int("skipped", skipped).
				Msg("scraper: tier 3 events skipped during normalisation")
		}

		return len(rawEvents), validEvents, nil
	}), nil
}

// scrapeFunc is the signature for the inner scraping work done by each tier.
// It returns the raw event count, submit-ready events, and any error.
type scrapeFunc func(ctx context.Context) (eventsFound int, validEvents []events.EventInput, err error)

// runWithTracking wraps a scrapeFunc with DB scraper_run insert/update bookkeeping.
// If queries is nil all DB operations are skipped (best-effort).
// It returns the final ScrapeResult (Error field set on failure).
func (s *Scraper) runWithTracking(
	ctx context.Context,
	result *ScrapeResult,
	fn scrapeFunc,
) ScrapeResult {
	start := time.Now()

	// Insert run record (best-effort).
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

	eventsFound, validEvents, err := fn(ctx)
	result.EventsFound = eventsFound

	if err != nil {
		result.Error = err
		s.updateRunFailed(ctx, runID, err)
		s.recordMetrics(*result, time.Since(start))
		return *result
	}

	result.EventsSubmitted = len(validEvents)

	if len(validEvents) == 0 {
		s.updateRunCompleted(ctx, runID, result)
		s.recordMetrics(*result, time.Since(start))
		return *result
	}

	ingestResult, err := s.submitEvents(ctx, validEvents, result.DryRun)
	if err != nil {
		result.Error = err
		s.updateRunFailed(ctx, runID, err)
		s.recordMetrics(*result, time.Since(start))
		return *result
	}

	result.EventsCreated = ingestResult.EventsCreated
	result.EventsDuplicate = ingestResult.EventsDuplicate
	result.EventsFailed = ingestResult.EventsFailed
	result.IngestErrors = ingestResult.Errors

	if len(ingestResult.Errors) > 0 {
		for _, ie := range ingestResult.Errors {
			s.logger.Warn().
				Str("source", result.SourceName).
				Int("event_index", ie.Index).
				Str("error", ie.Message).
				Msg("scraper: ingest rejected event")
		}
	}

	s.updateRunCompleted(ctx, runID, result)
	s.recordMetrics(*result, time.Since(start))
	return *result
}

// recordMetrics records Prometheus metrics for a completed scrape run.
// It is a no-op when s.slot is empty (metrics disabled).
// s.scraperMetrics is guaranteed non-nil whenever s.slot is non-empty
// (see NewScraperWithSlot and NewScraperWithSourceRepoAndSlot).
func (s *Scraper) recordMetrics(result ScrapeResult, duration time.Duration) {
	if s.slot == "" {
		return
	}

	tier := fmt.Sprintf("%d", result.Tier)

	// Determine result label. Error takes priority: a dry-run that also
	// returns an error should be counted as "error" so failures are never
	// silently hidden behind the "dry_run" bucket.
	resultLabel := "success"
	if result.Error != nil {
		resultLabel = "error"
	} else if result.DryRun {
		resultLabel = "dry_run"
	}

	s.scraperMetrics.RunDuration.WithLabelValues(result.SourceName, tier, s.slot).Observe(duration.Seconds())
	s.scraperMetrics.RunsTotal.WithLabelValues(result.SourceName, tier, resultLabel, s.slot).Inc()

	// Increment per-outcome event counters (skip zero values to avoid label pollution).
	type outcomeCount struct {
		outcome string
		count   int
	}
	outcomes := []outcomeCount{
		{"found", result.EventsFound},
		{"submitted", result.EventsSubmitted},
		{"created", result.EventsCreated},
		{"duplicate", result.EventsDuplicate},
		{"failed", result.EventsFailed},
	}
	for _, oc := range outcomes {
		if oc.count != 0 {
			s.scraperMetrics.EventsTotal.WithLabelValues(result.SourceName, tier, oc.outcome, s.slot).Add(float64(oc.count))
		}
	}
}

// updateRunFailed updates the scraper_run record with a failure message.
func (s *Scraper) updateRunFailed(ctx context.Context, runID int64, err error) {
	if s.queries == nil || runID == 0 {
		return
	}
	params := postgres.UpdateScraperRunFailedParams{
		ID:           runID,
		ErrorMessage: pgtype.Text{String: err.Error(), Valid: true},
	}
	if err2 := s.queries.UpdateScraperRunFailed(ctx, params); err2 != nil {
		s.logger.Warn().Err(err2).Msg("scraper: failed to update scraper run failure")
	}
}

// updateRunCompleted updates the scraper_run record with completion counts.
func (s *Scraper) updateRunCompleted(ctx context.Context, runID int64, result *ScrapeResult) {
	if s.queries == nil || runID == 0 {
		return
	}
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
