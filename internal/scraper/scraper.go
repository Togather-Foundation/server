package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
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
	Verbose          bool              // when true with DryRun, ScrapeResult.DryRunEvents is populated
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

	// DryRunEvents holds the normalized EventInput payloads when DryRun &&
	// Verbose are both true. Populated by runWithTracking so the CLI can
	// display individual events. Empty in non-verbose or non-dry-run modes
	// to avoid unnecessary memory allocation.
	DryRunEvents []events.EventInput

	// QualityWarnings holds structured quality warnings detected during
	// extraction and normalisation. Examples: "date_selector_never_matched:
	// selector #2 matched 0/15 events", "all_midnight: 15/15 events have
	// T00:00:00 start times". These are logged at WARN level during the
	// scrape and surfaced in CLI verbose mode.
	QualityWarnings []string
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

	return s.runWithTracking(ctx, &result, func(ctx context.Context) (int, []events.EventInput, []string, error) {
		rawEvents, err := FetchAndExtractJSONLD(ctx, rawURL, opts.HTTPClient(fetchTimeout))
		if err != nil {
			return 0, nil, nil, err
		}
		valid, skipped := s.normalizeJSONLDEvents(rawEvents, source, opts.Limit)
		if skipped > 0 {
			s.logger.Warn().Str("source", sourceName).Int("skipped", skipped).
				Msg("scraper: events skipped during normalisation")
		}
		return len(rawEvents), valid, checkAllMidnight(valid), nil
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

	// Sitemap-based scraping: dispatch before the tier switch since sitemap
	// is a URL discovery mechanism that delegates to the configured tier.
	if found.Sitemap != nil {
		return s.scrapeSitemap(ctx, *found, opts)
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
		// Sitemap-based scraping.
		if cfg.Sitemap != nil {
			res, scrapeErr = s.scrapeSitemap(ctx, cfg, opts)
		} else {
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
		}
		if scrapeErr != nil {
			res.Error = scrapeErr
		}
		results = append(results, res)
	}

	return results, nil
}

// normalizeRawEvents groups RawEvents by (URL, Name) and consolidates multi-row
// groups into single EventInputs with Occurrences. Single-row groups are normalized
// individually. Returns valid EventInputs and the count of skipped (failed) groups.
//
// A limit of 0 means no limit. The logger is used for per-group warning messages.
func normalizeRawEvents(rawEvents []RawEvent, source SourceConfig, limit int, logger zerolog.Logger) ([]events.EventInput, int) {
	// Group RawEvents by (URL, Name) so we can detect multi-row cases.
	// Map key: "url|||name" (using triple-pipe to avoid collisions).
	// An ordered slice tracks insertion order so results are deterministic.
	type group struct {
		key  string
		rows []RawEvent
	}
	groupMap := make(map[string]*group)
	var groupOrder []string

	ungroupedIdx := 0
	for _, raw := range rawEvents {
		var key string
		if raw.URL != "" {
			// URL present — group by URL+Name (multi-occurrence detection).
			key = fmt.Sprintf("%s|||%s", raw.URL, raw.Name)
		} else {
			// No URL — cannot reliably group; treat each as its own event
			// to avoid merging unrelated events that happen to share a name.
			key = fmt.Sprintf("__nourl_%d|||%s", ungroupedIdx, raw.Name)
			ungroupedIdx++
		}
		if _, ok := groupMap[key]; !ok {
			groupMap[key] = &group{key: key}
			groupOrder = append(groupOrder, key)
		}
		groupMap[key].rows = append(groupMap[key].rows, raw)
	}

	var validEvents []events.EventInput
	skipped := 0

	for _, key := range groupOrder {
		if limit > 0 && len(validEvents) >= limit {
			break
		}

		g := groupMap[key]

		var input events.EventInput
		var normErr error

		if len(g.rows) > 1 {
			// Multi-row case: consolidate into a single EventInput with Occurrences.
			input, normErr = consolidateOccurrences(g.rows, source)
		} else {
			// Single-row case: normalize as before.
			input, normErr = NormalizeRawEvent(g.rows[0], source)
		}

		if normErr != nil {
			logger.Warn().Str("source", source.Name).
				Str("name", g.rows[0].Name).
				Err(normErr).
				Msg("scraper: skipping raw event(s) that failed normalisation")
			skipped++
			continue
		}

		validEvents = append(validEvents, input)
	}

	return validEvents, skipped
}

// scrapeTier1 fetches and processes a Tier 1 (Colly CSS-selector) source.
func (s *Scraper) scrapeTier1(ctx context.Context, source SourceConfig, opts ScrapeOptions) (ScrapeResult, error) {
	result := ScrapeResult{
		SourceName: source.Name,
		SourceURL:  source.URL,
		Tier:       1,
		DryRun:     opts.DryRun,
	}

	return s.runWithTracking(ctx, &result, func(ctx context.Context) (int, []events.EventInput, []string, error) {
		extractor := NewCollyExtractor(s.logger)
		extractor.SetTransport(opts.Transport)
		if opts.RateLimitMs > 0 {
			extractor.SetRateLimit(time.Duration(opts.RateLimitMs) * time.Millisecond)
		}

		var allRaw []RawEvent
		var firstProbes []DateSelectorProbe
		urlList := source.GetURLs()
		failCount := 0
		for _, u := range urlList {
			clone := source
			clone.URL = u
			rawEvts, probes, fetchErr := extractor.ScrapeWithSelectors(ctx, clone)
			if fetchErr != nil {
				s.logger.Warn().Str("source", source.Name).Str("url", u).Err(fetchErr).
					Msg("scraper: tier 1 URL failed, continuing")
				failCount++
				continue
			}
			if firstProbes == nil {
				firstProbes = probes
			}
			allRaw = append(allRaw, rawEvts...)
		}
		// failCount > 0 guards against the empty-list case (0 == 0 would fire spuriously).
		if failCount > 0 && failCount == len(urlList) {
			s.logger.Warn().Str("source", source.Name).Int("urls", len(urlList)).
				Msg("scraper: all URLs failed for source — returning 0 events")
		}
		rawEvents := allRaw

		// Quality check: date_selectors partial match detection.
		var warnings []string
		warnings = append(warnings, checkDateSelectorQuality(rawEvents, source, firstProbes)...)

		validEvents, skipped := normalizeRawEvents(rawEvents, source, opts.Limit, s.logger)
		if skipped > 0 {
			s.logger.Warn().Str("source", source.Name).Int("skipped", skipped).
				Msg("scraper: tier 1 events skipped during normalisation")
		}

		// Quality check: all-midnight heuristic.
		warnings = append(warnings, checkAllMidnight(validEvents)...)

		return len(rawEvents), validEvents, warnings, nil
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

	return s.runWithTracking(ctx, &result, func(ctx context.Context) (int, []events.EventInput, []string, error) {
		var allRaw []RawEvent
		var firstProbes []DateSelectorProbe
		urlList := source.GetURLs()
		failCount := 0
		for _, u := range urlList {
			clone := source
			clone.URL = u
			rawEvts, probes, fetchErr := s.rodExtractor.ScrapeWithBrowser(ctx, clone)
			if fetchErr != nil {
				s.logger.Warn().Str("source", source.Name).Str("url", u).Err(fetchErr).
					Msg("scraper: tier 2 URL failed, continuing")
				failCount++
				continue
			}
			if firstProbes == nil {
				firstProbes = probes
			}
			allRaw = append(allRaw, rawEvts...)
		}
		// failCount > 0 guards against the empty-list case (0 == 0 would fire spuriously).
		if failCount > 0 && failCount == len(urlList) {
			s.logger.Warn().Str("source", source.Name).Int("urls", len(urlList)).
				Msg("scraper: all URLs failed for source — returning 0 events")
		}
		rawEvents := allRaw

		// Quality check: date_selectors partial match detection.
		var warnings []string
		warnings = append(warnings, checkDateSelectorQuality(rawEvents, source, firstProbes)...)

		validEvents, skipped := normalizeRawEvents(rawEvents, source, opts.Limit, s.logger)
		if skipped > 0 {
			s.logger.Warn().Str("source", source.Name).Int("skipped", skipped).
				Msg("scraper: tier 2 events skipped during normalisation")
		}

		// Quality check: all-midnight heuristic.
		warnings = append(warnings, checkAllMidnight(validEvents)...)

		return len(rawEvents), validEvents, warnings, nil
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

	return s.runWithTracking(ctx, &result, func(ctx context.Context) (int, []events.EventInput, []string, error) {
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
						return len(rawEvents), valid, checkAllMidnight(valid), nil
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

		return len(rawEvents), valid, checkAllMidnight(valid), nil
	}), nil
}

// scrapeTier3 fetches and processes a Tier 3 (API: GraphQL or REST JSON) source.
func (s *Scraper) scrapeTier3(ctx context.Context, source SourceConfig, opts ScrapeOptions) (ScrapeResult, error) {
	result := ScrapeResult{
		SourceName: source.Name,
		SourceURL:  source.URL,
		Tier:       3,
		DryRun:     opts.DryRun,
	}

	return s.runWithTracking(ctx, &result, func(ctx context.Context) (int, []events.EventInput, []string, error) {
		var rawEvents []RawEvent
		var err error

		extractor, extErr := NewExtractor(source, s.logger)
		if extErr != nil {
			return 0, nil, nil, extErr
		}
		rawEvents, err = extractor.Extract(ctx, source, opts.HTTPClient(fetchTimeout))
		if err != nil {
			return 0, nil, nil, err
		}

		validEvents, skipped := normalizeRawEvents(rawEvents, source, opts.Limit, s.logger)
		if skipped > 0 {
			s.logger.Warn().Str("source", source.Name).Int("skipped", skipped).
				Msg("scraper: tier 3 events skipped during normalisation")
		}

		// Quality check: all-midnight heuristic.
		return len(rawEvents), validEvents, checkAllMidnight(validEvents), nil
	}), nil
}

// scrapeSitemap discovers URLs from a sitemap XML and scrapes each one
// individually using the source's configured tier. Results are aggregated
// into a single ScrapeResult.
func (s *Scraper) scrapeSitemap(ctx context.Context, source SourceConfig, opts ScrapeOptions) (ScrapeResult, error) {
	result := ScrapeResult{
		SourceName: source.Name,
		SourceURL:  source.Sitemap.URL, // Use sitemap URL as the "source URL" for tracking
		Tier:       source.Tier,
		DryRun:     opts.DryRun,
	}

	return s.runWithTracking(ctx, &result, func(ctx context.Context) (int, []events.EventInput, []string, error) {
		// 1. Compile filter regex
		pattern, err := regexp.Compile(source.Sitemap.FilterPattern)
		if err != nil {
			return 0, nil, nil, fmt.Errorf("compile sitemap filter pattern: %w", err)
		}

		// 1b. Compile optional exclude regex
		var exclude *regexp.Regexp
		if source.Sitemap.ExcludePattern != "" {
			exclude, err = regexp.Compile(source.Sitemap.ExcludePattern)
			if err != nil {
				return 0, nil, nil, fmt.Errorf("compile sitemap exclude pattern: %w", err)
			}
		}

		// 2. Fetch sitemap
		entries, err := FetchSitemap(ctx, source.Sitemap.URL, opts.HTTPClient(fetchTimeout))
		if err != nil {
			return 0, nil, nil, fmt.Errorf("fetch sitemap: %w", err)
		}

		totalInSitemap := len(entries)

		// 3. Filter by include regex + optional exclude regex + lastmod
		filtered := FilterSitemapEntries(entries, pattern, exclude, source.LastScrapedAt)

		afterFilter := len(filtered)

		// 4. Apply MaxURLs cap
		maxURLs := source.Sitemap.MaxURLs
		if maxURLs <= 0 {
			maxURLs = defaultSitemapMaxURLs
		}
		if len(filtered) > maxURLs {
			filtered = filtered[:maxURLs]
		}

		s.logger.Info().
			Str("source", source.Name).
			Int("sitemap_total", totalInSitemap).
			Int("after_filter", afterFilter).
			Int("scraping", len(filtered)).
			Msg("scraper: sitemap URL discovery complete")

		if len(filtered) == 0 {
			return 0, nil, nil, nil
		}

		// 5. Rate limit config
		rateLimitMs := source.Sitemap.RateLimitMs
		if rateLimitMs <= 0 {
			rateLimitMs = defaultSitemapRateLimitMs
		}
		rateDelay := time.Duration(rateLimitMs) * time.Millisecond

		// 6. Build the selector scrape function once (outside the loop).
		// CollyExtractor is safe to reuse across URLs because ScrapeWithSelectors
		// creates a fresh Colly collector on every call — it holds no per-URL state.
		var selectorScrape selectorPageScrapeFunc
		switch source.Tier {
		case 1:
			extractor := NewCollyExtractor(s.logger)
			extractor.SetTransport(opts.Transport)
			if opts.RateLimitMs > 0 {
				extractor.SetRateLimit(time.Duration(opts.RateLimitMs) * time.Millisecond)
			}
			selectorScrape = extractor.ScrapeWithSelectors
		case 2:
			if s.rodExtractor == nil {
				return 0, nil, nil, fmt.Errorf("tier 2 scraping requires a RodExtractor")
			}
			selectorScrape = s.rodExtractor.ScrapeWithBrowser
		}

		// 7. Scrape each URL
		var allEvents []events.EventInput
		var warnings []string
		totalFound := 0
		failCount := 0

		for i, entry := range filtered {
			if ctx.Err() != nil {
				break
			}

			// Rate limit (skip delay before first URL)
			if i > 0 {
				select {
				case <-ctx.Done():
					break // will be caught by the loop check above next iteration
				case <-time.After(rateDelay):
				}
			}

			pageURL := entry.URL
			clone := source
			clone.URL = pageURL
			clone.Sitemap = nil // prevent recursion
			clone.URLs = nil    // single URL mode

			s.logger.Debug().
				Str("source", source.Name).
				Str("url", pageURL).
				Int("index", i+1).
				Int("total", len(filtered)).
				Msg("scraper: sitemap scraping detail page")

			var pageEvents []events.EventInput
			var pageErr error

			switch source.Tier {
			case 0:
				rawEvts, fetchErr := FetchAndExtractJSONLD(ctx, pageURL, opts.HTTPClient(fetchTimeout))
				if fetchErr != nil {
					pageErr = fetchErr
				} else {
					totalFound += len(rawEvts)
					valid, skipped := s.normalizeJSONLDEvents(rawEvts, clone, opts.Limit)
					if skipped > 0 {
						s.logger.Warn().Str("source", source.Name).Str("url", pageURL).Int("skipped", skipped).
							Msg("scraper: sitemap T0 events skipped during normalisation")
					}
					pageEvents = valid
				}

			case 1, 2:
				var warns []string
				var found int
				pageEvents, warns, found, pageErr = s.scrapeSitemapSelectorPage(ctx, selectorScrape, clone, fmt.Sprintf("T%d", source.Tier))
				warnings = append(warnings, warns...)
				totalFound += found

			default:
				pageErr = fmt.Errorf("sitemap scraping not supported for tier %d", source.Tier)
			}

			if pageErr != nil {
				s.logger.Warn().Str("source", source.Name).Str("url", pageURL).Err(pageErr).
					Msg("scraper: sitemap detail page failed, continuing")
				failCount++
				continue
			}

			allEvents = append(allEvents, pageEvents...)

			// Respect global limit
			if opts.Limit > 0 && len(allEvents) >= opts.Limit {
				allEvents = allEvents[:opts.Limit]
				break
			}
		}

		if failCount > 0 {
			s.logger.Warn().Str("source", source.Name).
				Int("failed", failCount).Int("total", len(filtered)).
				Msg("scraper: sitemap detail page failures")
		}

		// Quality check: all-midnight heuristic.
		warnings = append(warnings, checkAllMidnight(allEvents)...)

		return totalFound, allEvents, warnings, nil
	}), nil
}

// scrapeFunc is the signature for the inner scraping work done by each tier.
// It returns the raw event count, submit-ready events, quality warnings, and
// any error. Quality warnings are accumulated into ScrapeResult.QualityWarnings
// by runWithTracking.
type scrapeFunc func(ctx context.Context) (eventsFound int, validEvents []events.EventInput, qualityWarnings []string, err error)

// selectorPageScrapeFunc extracts raw events from a single page using CSS selectors.
// Both CollyExtractor.ScrapeWithSelectors and RodExtractor.ScrapeWithBrowser satisfy
// this signature.
type selectorPageScrapeFunc func(ctx context.Context, cfg SourceConfig) ([]RawEvent, []DateSelectorProbe, error)

// scrapeSitemapSelectorPage scrapes a single sitemap detail page using a CSS-selector
// extractor (tier 1 Colly or tier 2 Rod). It normalizes the results and returns
// page events, quality warnings, the raw event count, and any error.
func (s *Scraper) scrapeSitemapSelectorPage(
	ctx context.Context,
	scrapeFn selectorPageScrapeFunc,
	clone SourceConfig,
	tierLabel string,
) (pageEvents []events.EventInput, warnings []string, rawCount int, err error) {
	rawEvts, probes, fetchErr := scrapeFn(ctx, clone)
	if fetchErr != nil {
		return nil, nil, 0, fetchErr
	}
	rawCount = len(rawEvts)
	warnings = checkDateSelectorQuality(rawEvts, clone, probes)
	// Pass limit=0; the global limit is enforced after pageEvents are
	// appended to allEvents in scrapeSitemap.
	valid, skipped := normalizeRawEvents(rawEvts, clone, 0, s.logger)
	if skipped > 0 {
		s.logger.Warn().Str("source", clone.Name).Str("url", clone.URL).Int("skipped", skipped).
			Msgf("scraper: sitemap %s events skipped during normalisation", tierLabel)
	}
	return valid, warnings, rawCount, nil
}

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

	eventsFound, validEvents, qualityWarnings, err := fn(ctx)
	result.EventsFound = eventsFound
	result.QualityWarnings = append(result.QualityWarnings, qualityWarnings...)

	// Log quality warnings at WARN level so they appear in structured logs
	// even without --verbose CLI mode.
	for _, w := range result.QualityWarnings {
		s.logger.Warn().Str("source", result.SourceName).Str("quality_warning", w).
			Msg("scraper: quality warning detected")
	}

	if err != nil {
		result.Error = err
		s.updateRunFailed(ctx, runID, err)
		s.recordMetrics(*result, time.Since(start))
		return *result
	}

	result.EventsSubmitted = len(validEvents)

	// Stash normalized events for CLI verbose output in dry-run mode.
	// The slice is only a reference — no deep copy needed.
	if result.DryRun {
		result.DryRunEvents = validEvents
	}

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

// normalizeJSONLDEvents groups raw JSON-LD events by URL+Name and delegates
// to groupJSONLDEvents. Returns valid EventInputs and count of skipped events.
func (s *Scraper) normalizeJSONLDEvents(rawEvents []json.RawMessage, source SourceConfig, limit int) ([]events.EventInput, int) {
	return groupJSONLDEvents(rawEvents, source, limit, s.logger)
}

// groupJSONLDEvents groups raw JSON-LD Event objects by (URL, Name) composite
// key so that multiple separate Event objects describing different dates of the
// same show are consolidated into a single EventInput with Occurrences.
//
// This handles Pattern 1 from schema.org: Google-recommended separate Event
// objects sharing URL+Name but with different startDates.
//
// Events without a URL are not grouped (each becomes its own EventInput) to
// avoid false merges on events that merely share a name.
func groupJSONLDEvents(rawEvents []json.RawMessage, source SourceConfig, limit int, logger zerolog.Logger) ([]events.EventInput, int) {
	// Peek struct to extract URL+Name without full normalization.
	type peekEvent struct {
		Name json.RawMessage `json:"name"`
		URL  json.RawMessage `json:"url"`
	}

	type group struct {
		key  string
		raws []json.RawMessage
	}
	groupMap := make(map[string]*group)
	var groupOrder []string
	ungroupedIdx := 0

	for _, raw := range rawEvents {
		var peek peekEvent
		if err := json.Unmarshal(raw, &peek); err != nil {
			logger.Debug().Str("source", source.Name).Err(err).
				Msg("scraper: skipping unparseable JSON-LD event during grouping")
			continue
		}

		name := extractStringValue(peek.Name)
		url := extractStringValue(peek.URL)

		var key string
		if url != "" {
			key = fmt.Sprintf("%s|||%s", url, name)
		} else {
			key = fmt.Sprintf("__nourl_%d|||%s", ungroupedIdx, name)
			ungroupedIdx++
		}

		if _, ok := groupMap[key]; !ok {
			groupMap[key] = &group{key: key}
			groupOrder = append(groupOrder, key)
		}
		groupMap[key].raws = append(groupMap[key].raws, raw)
	}

	var valid []events.EventInput
	skipped := 0

	for _, key := range groupOrder {
		if limit > 0 && len(valid) >= limit {
			break
		}

		g := groupMap[key]

		if len(g.raws) == 1 {
			// Single event — normalize directly.
			evt, err := NormalizeJSONLDEvent(g.raws[0], source)
			if err != nil {
				logger.Debug().Str("source", source.Name).Err(err).
					Msg("scraper: skipping event that failed normalisation")
				skipped++
				continue
			}
			valid = append(valid, evt)
		} else {
			// Multi-event group — consolidate into one EventInput with Occurrences.
			evt, err := NormalizeJSONLDEvent(g.raws[0], source)
			if err != nil {
				logger.Debug().Str("source", source.Name).Err(err).
					Msg("scraper: skipping event group that failed normalisation")
				skipped++
				continue
			}

			// If NormalizeJSONLDEvent already populated Occurrences from
			// subEvent data, preserve those (they carry richer per-occurrence
			// metadata like endDate and doorTime). Only build occurrences
			// from the group's top-level dates when subEvent didn't provide any.
			if len(evt.Occurrences) == 0 {
				if occs := occurrencesFromRawMessages(g.raws); len(occs) > 0 {
					evt.Occurrences = occs
				}
			}
			valid = append(valid, evt)
		}
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
