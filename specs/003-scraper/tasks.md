---
description: "Task list for integrated event scraper"
---

# Tasks: Integrated Event Scraper

**Input**: Design documents from /specs/003-scraper/
**Prerequisites**: plan.md, spec.md, existing batch ingest API, source registry
**Tests**: REQUIRED (TDD where practical). Target 80%+ coverage for normalizer, config loader, extraction logic.

## Format: [ID] [P?] [Story] Description

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1..US4)
- **File paths** are required in every task description

---

## Phase 1: Foundation + Tier 0 (JSON-LD Extraction)

**Purpose**: End-to-end `server scrape url <URL>` with JSON-LD extraction

### Setup

- [ ] S001 [P] Add goquery and colly dependencies via `go get github.com/PuerkitoBio/goquery github.com/gocolly/colly/v2`
- [ ] S002 [P] Create `scraper_runs` database migration in `internal/storage/postgres/migrations/NNNNNN_add_scraper_runs.{up,down}.sql` — table with source_name, source_url, tier, timestamps, status, event counts, error_message, metadata JSONB
- [ ] S003 [P] Add SQLc queries for scraper_runs in `internal/storage/postgres/queries/scraper_runs.sql` — InsertRun, UpdateRunCompleted, UpdateRunFailed, GetLatestRunBySource, ListRecentRuns

### Core Scraper Library

- [ ] S004 [P] [US2] Implement source config types and YAML loader in `internal/scraper/config.go` — SourceConfig struct (name, url, tier, schedule, trust_level, license, enabled, selectors), LoadSourceConfigs(dir) function, ValidateConfig, unit tests for loading/validation
- [ ] S005 [US1] Implement JSON-LD extraction in `internal/scraper/jsonld.go` — FetchAndExtractJSONLD(ctx, url) function: HTTP GET with User-Agent, parse HTML with goquery, find `<script type="application/ld+json">`, parse JSON, filter for @type Event/EventSeries, handle @graph and ItemList wrappers, handle arrays. Return []json.RawMessage. Include robots.txt checking. Unit tests with sample HTML fixtures.
- [ ] S006 [US1] Implement schema.org → EventInput normalizer in `internal/scraper/normalize.go` — NormalizeJSONLDEvent(raw json.RawMessage, source SourceConfig) (EventInput, error): handle location as string vs Place object, handle offers as object vs array, handle date formats, extract source URL + event ID, set license from config. Unit tests for each variant shape documented in plan.md.
- [ ] S007 [P] [US1] Implement SEL batch ingest HTTP client in `internal/scraper/ingest.go` — IngestClient struct with base URL + API key, SubmitBatch(ctx, []EventInput) (IngestResult, error): POST to /api/v1/events:batch, parse response for batch_id/counts, handle error responses. Unit tests with httptest server.
- [ ] S008 [US1] [US4] Implement scraper service orchestrator in `internal/scraper/scraper.go` — Scraper struct holding config, ingest client, DB queries. ScrapeURL(ctx, url, opts) method: try Tier 0, normalize, optionally ingest (dry-run), track run in DB. ScrapeSource(ctx, sourceName, opts) method: load config, dispatch to appropriate tier. ScrapeAll(ctx, opts). Options: DryRun, Limit, ServerURL, APIKey.

### CLI

- [ ] S009 [US1] Implement `server scrape url` subcommand in `cmd/server/cmd/scrape.go` — cobra command with url subcommand, flags: --dry-run, --limit, --server, --key. Loads env, creates scraper, runs ScrapeURL, prints results. Follow existing patterns from ingest.go and reconcile.go.
- [ ] S010 [US2] Add `server scrape list` subcommand to `cmd/server/cmd/scrape.go` — lists all source configs from configs/sources/ with name, url, tier, enabled, and last run status (from DB if available)
- [ ] S011 [US2] Add `server scrape source <name>` subcommand to `cmd/server/cmd/scrape.go` — scrapes a single named source using its config
- [ ] S012 [US3] Add `server scrape all` subcommand to `cmd/server/cmd/scrape.go` — scrapes all enabled sources, reports per-source and aggregate results

### Tests

- [ ] S013 [P] [US1] Create HTML test fixtures in `internal/scraper/testdata/` — sample pages with: single JSON-LD Event, multiple events in @graph, ItemList wrapper, nested location objects, no JSON-LD, malformed JSON-LD. At least 6 fixture files.
- [ ] S014 [P] [US1] Integration test in `internal/scraper/scraper_test.go` — start httptest server serving fixture HTML, run ScrapeURL against it, verify correct EventInput output. Test both Tier 0 success and fallback behavior.

**Phase 1 Checkpoint**: `server scrape url <URL> --dry-run` works end-to-end on real sites

---

## Phase 2: Tier 1 (Colly CSS Selectors) + Source Configs

**Purpose**: Config-driven scraping for sites without JSON-LD

### Colly Integration

- [ ] S015 [US2] Implement Colly-based extraction in `internal/scraper/colly.go` — CollyExtractor struct: creates Colly collector with rate limiting (1 req/s default), robots.txt compliance, User-Agent, max depth. ScrapeWithSelectors(ctx, config SourceConfig) ([]RawEvent, error): visit URL, apply selectors.event_list to find event containers, extract fields via child selectors, follow pagination if configured, respect max_pages. Unit tests with httptest.
- [ ] S016 [P] [US2] Add RawEvent → EventInput normalization for Tier 1 in `internal/scraper/normalize.go` — NormalizeRawEvent(raw RawEvent, source SourceConfig) (EventInput, error): parse dates from various text formats, build location from text, set source attribution. More lenient than JSON-LD normalization since data is less structured.
- [ ] S017 [US1] [US2] Update scraper orchestrator in `internal/scraper/scraper.go` — ScrapeURL tries Tier 0 first; if no JSON-LD found and source config exists with tier=1, falls back to Colly. ScrapeSource dispatches based on config tier.

### Source Configs

- [ ] S018 [P] Create `configs/sources/_example.yaml` — fully documented example with all fields, comments explaining each option, both Tier 0 and Tier 1 examples
- [ ] S019 Create initial GTA source configs in `configs/sources/` — research and create YAML configs for 5-10 Toronto arts/culture sites. Mix of Tier 0 (sites with JSON-LD) and Tier 1 (sites needing selectors). Document findings (which sites have JSON-LD, which need selectors).

### Database Tracking

- [ ] S020 [US4] Wire scraper_runs DB tracking into scraper service — record start/completion/failure of each run with event counts and metadata. Use existing SQLc queries from S003.

### Tests

- [ ] S021 [P] [US2] Colly extraction tests in `internal/scraper/colly_test.go` — httptest server with sample event listing HTML, verify selector-based extraction produces correct RawEvent structs. Test pagination, error handling, robots.txt blocking.
- [ ] S022 [US3] End-to-end test — run `server scrape all --dry-run` against real staging, verify it completes without errors, produces reasonable event output.

**Phase 2 Checkpoint**: `server scrape all` works against real GTA sources on staging

---

## Phase 3: Scheduling + Production (Future — not in this pass)

- [ ] S023 [US4] Implement River job worker for periodic scraping in `internal/scraper/schedule.go`
- [ ] S024 [US4] Wire scraper worker into server startup in `cmd/server/cmd/serve.go`
- [ ] S025 Add Prometheus metrics for scrape runs (success/failure rates, event counts)

## Phase 4: Agent Feedback + Quality (Future)

- [ ] S026 Event completeness scoring (what % of fields are populated per event)
- [ ] S027 Source quality metrics over time (success rate, data completeness trends)
- [ ] S028 MCP tool for curation agent to report data quality issues
- [ ] S029 Agent-assisted source config generation (analyze URL, propose selectors)
