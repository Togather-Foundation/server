# Implementation Plan: Integrated Event Scraper

**Branch**: `003-scraper` | **Date**: 2026-02-21 | **Spec**: [spec.md](./spec.md)
**Status**: Phases 1–3 complete. Code review fixes **COMPLETE** — all 13 review beads closed across 3 waves (Wave 1: srv-43rad, srv-i0lhn, srv-e3sk8, srv-kezsj; Wave 2: srv-v5a2n, srv-14cib, srv-9nwr7, srv-ewuhx, srv-0z4fu, srv-ooakk; Wave 3: srv-r1x5p, srv-0b33l, srv-dnq72).
**Input**: Feature specification from `/specs/003-scraper/spec.md`

## Summary

Add an integrated event scraper to the SEL server that extracts events from Toronto-area arts/culture websites and ingests them via the existing batch API. Two-tier extraction: Tier 0 (JSON-LD, zero-config) and Tier 1 (Colly + CSS selectors, per-site YAML config). Community contributes source URLs as YAML files; the SEL handles dedup, reconciliation, and provenance automatically.

## Technical Context

**New Dependencies** (added):
- `github.com/PuerkitoBio/goquery` v1.11.0 — HTML parsing and JSON-LD extraction
- `github.com/gocolly/colly/v2` v2.3.0 — Web scraping framework with rate limiting, robots.txt, caching
- `gopkg.in/yaml.v3` — promoted from indirect to direct for source config loading

**Existing Infrastructure Leveraged**:
- Batch ingest API (`POST /api/v1/events:batch`) — scraper is a consumer
- Source registry with trust levels and provenance tracking
- 3-layer deduplication (source external ID, content hash, manual merge)
- Artsdata reconciliation for places and organizations
- Cobra CLI framework for `server scrape` command
- PostgreSQL for `scraper_runs` tracking table
- zerolog for structured logging

**Not Used (Yet)**:
- River job queue (Phase 3 — scheduling)
- Rod/Chromedp (Tier 2 — JS-heavy sites, future)

## Architecture

```
                   ┌─────────────────────────────────────┐
                   │         CLI: server scrape           │
                   │   url │ source │ all │ list          │
                   └──────────────┬──────────────────────┘
                                  │
                   ┌──────────────▼──────────────────────┐
                   │         Scraper Service              │
                   │  Load config → Pick tier → Extract   │
                   └──┬───────────────────────────────┬──┘
                      │                               │
           ┌──────────▼──────────┐       ┌───────────▼──────────┐
           │  Tier 0: JSON-LD    │       │  Tier 1: Colly       │
           │  net/http + goquery │       │  CSS selectors       │
           │  Zero-config        │       │  Per-site YAML       │
           └──────────┬──────────┘       └───────────┬──────────┘
                      │                               │
                      └───────────┬───────────────────┘
                                  │
                   ┌──────────────▼──────────────────────┐
                   │         Normalizer                   │
                   │  schema.org JSON-LD → EventInput     │
                   └──────────────┬──────────────────────┘
                                  │
                   ┌──────────────▼──────────────────────┐
                   │         Ingest Client                │
                   │  POST /api/v1/events:batch           │
                   │  (HTTP, with API key auth)           │
                   └──────────────┬──────────────────────┘
                                  │
                   ┌──────────────▼──────────────────────┐
                   │       Existing SEL Pipeline          │
                   │  Validate → Dedup → Ingest →         │
                   │  Reconcile → Enrich                  │
                   └─────────────────────────────────────┘
```

### Storage: Hybrid YAML + DB

- **YAML files** (`configs/sources/*.yaml`): Source definitions — what to scrape, how, and with what trust level. Version controlled, community-contributable via PRs.
- **Database** (`scraper_sources` table, Phase 3): Runtime store for source configs. `server scrape sync` upserts YAML→DB; `server scrape export` dumps DB→YAML. Runtime scraping reads from DB first, falls back to YAML if DB unavailable or empty.
- **Database** (`scraper_runs` table): Runtime state — when each source was last scraped, what happened, metrics. Queryable for monitoring and debugging.

### JSON-LD Normalization Challenges

Schema.org Event JSON-LD in the wild comes in many shapes:

```jsonld
// Shape 1: Single event at top level
{"@type": "Event", "name": "Concert", ...}

// Shape 2: Array of events
[{"@type": "Event", ...}, {"@type": "Event", ...}]

// Shape 3: @graph container
{"@graph": [{"@type": "Event", ...}, ...]}

// Shape 4: ItemList wrapping events
{"@type": "ItemList", "itemListElement": [{"@type": "ListItem", "item": {"@type": "Event", ...}}]}

// Shape 5: Nested location as string vs object
"location": "Massey Hall"  vs  "location": {"@type": "Place", "name": "Massey Hall", ...}

// Shape 6: Date as string vs structured
"startDate": "2026-03-15T20:00:00-05:00"  vs  "startDate": {"@type": "Date", ...}

// Shape 7: offers as object vs array
"offers": {"@type": "Offer", ...}  vs  "offers": [{"@type": "Offer", ...}]
```

The normalizer must handle all of these gracefully.

## Phases

### Phase 1: Foundation + Tier 0 (JSON-LD Extraction) ✅ COMPLETE

**Goal**: `server scrape url <URL>` works end-to-end with JSON-LD sites.

1. ✅ Add Goquery + Colly dependencies (`srv-p6vbo`)
2. ✅ Create `scraper_runs` migration + SQLc queries (`srv-0hk2l`)
3. ✅ Implement `internal/scraper/config.go` — types and YAML loader (`srv-rje5r`)
4. ✅ Implement `internal/scraper/jsonld.go` — Tier 0 extraction (`srv-5atjx`)
5. ✅ Implement `internal/scraper/normalize.go` — schema.org → EventInput mapping (`srv-2gby0`)
6. ✅ Implement `internal/scraper/ingest.go` — HTTP client for batch API (`srv-qiii1`)
7. ✅ Implement `internal/scraper/scraper.go` — orchestrator (`srv-4xupn`)
8. ✅ Implement `cmd/server/cmd/scrape.go` — CLI with `url` subcommand (`srv-xyp0r`)
9. ✅ Unit tests: JSON-LD extraction (22 tests), normalization (35 tests), 7 HTML fixtures
10. ✅ Integration tests: httptest-based scrape tests

### Phase 2: Tier 1 (Colly) + Source Configs ✅ COMPLETE

**Goal**: Config-driven scraping with CSS selectors for sites without JSON-LD.

1. ✅ Implement `internal/scraper/colly.go` — Colly-based extraction (`srv-rnb2s`)
2. ✅ Create `configs/sources/_example.yaml` — documented template (`srv-ztij5`)
3. ✅ Create 6 GTA source configs: TSO, Roy Thomson, Hot Docs, Glad Day, TPL, Harbourfront (`srv-ztij5`)
4. ✅ Implement `server scrape source`, `server scrape all`, `server scrape list` (`srv-xyp0r`)
5. ✅ Add `scraper_runs` DB tracking wired into all tiers (`srv-0mnq0`)
6. ✅ Code review + security fixes: body limits, no-redirect clients, signal contexts, EventID dedup, @type preservation, CHECK constraint
7. ✅ Deployed to staging — all smoke tests passing

**Known issue**: The 6 GTA source configs contain placeholder URLs that have not been validated against live sites. See `srv-aany8`.

### Phase 3: DB-Backed Source Configs ✅ COMPLETE

**Goal**: `scraper_sources` table is the runtime store; YAML files remain version-controlled canonical source. Sync and export CLI commands; runtime reads DB-first.

1. ✅ Migration `000032_scraper_sources.up.sql` + SQLc queries (`srv-65kvw`)
2. ✅ `internal/domain/scraper/source.go` — `Source` type, `Repository` interface, `Service` (`srv-iorfa`)
3. ✅ `internal/storage/postgres/scraper_sources_repository.go` — postgres impl (`srv-iorfa`)
4. ✅ `server scrape sync` (YAML→DB upsert) + `server scrape export` (DB→YAML) CLI commands (`srv-2nu7e`)
5. ✅ `internal/scraper/db_source.go` — `domain/scraper.Source` → `SourceConfig` converter (`srv-l71q1`)
6. ✅ `internal/scraper/scraper.go` — added `sourceRepo` field, `NewScraperWithSourceRepo`, `loadSourceConfigs` (DB-first + YAML fallback) (`srv-l71q1`)
7. ✅ `server scrape list` — DB-first listing with YAML fallback (`srv-l71q1`)
8. ✅ `newScraperWithDB` wires `ScraperSourceRepository` into all scrape subcommands (`srv-l71q1`)
9. ✅ `sel:scraperSource` exposed on org/place JSON-LD `Get` API responses; optional `scraperRepo` field + `WithScraperSourceRepo()` on both handlers; router wired; tests added (`srv-17zth`)

**Architecture decisions (Phase 3)**:
- `scraper_sources` table is separate from `organizations`/`places`; linked via `org_scraper_sources` and `place_scraper_sources` join tables (many-to-many).
- `internal/scraper` imports `internal/domain/scraper` with alias `domainScraper` — no circular dependency.
- `loadSourceConfigs` only passes `enabled: true` to the DB query; the YAML fallback runs `LoadSourceConfigs` which returns all valid configs (enabled + disabled). Both `ScrapeSource` and `ScrapeAll` check `cfg.Enabled` before scraping.
- `ScrapeAll` now dispatches tier directly (avoids double `loadSourceConfigs` call that would occur if it called `ScrapeSource`).
- `scrape list` uses `repo.List(ctx, nil)` (all sources, not just enabled) so operators can see disabled sources too.
- `server scrape sync` smoke-tested: loaded all 14 YAML sources into DB. `server scrape export` smoke-tested: wrote them back to YAML correctly.
- `sel:scraperSource` is only populated on the single-item `Get` handler (not `List`) — best-effort, omitted on error or when empty.
- Handler pattern: optional dependency injected via `WithScraperSourceRepo(repo)` fluent method (same as `WithGeocodingService`).
- `scraperDomain.Repository` interface assigned to a local variable in `router.go` to satisfy the typed parameter — `postgres.NewScraperSourceRepository(pool)` returns the concrete type.

### Phase 4: Scheduling + Production (Future)

1. River job worker for periodic scraping — `srv-pfeud`
2. Config-driven schedules (daily, weekly)
3. Prometheus metrics for scrape success/failure rates
4. Admin UI page for scrape status — `srv-5127b`

### Phase 5: Agent Feedback + Quality (Future)

1. Event completeness scoring
2. Source quality metrics over time
3. MCP tool for agent to report data quality issues
4. Agent-assisted source config generation

## Reflect (Phase 6)

### Design Decisions That Worked Well

- **Two-tier split was clean** — JSON-LD zero-config path vs Colly selector path mapped naturally to separate files with no coupling.
- **Submitting via batch API** — exercises the full auth/dedup/reconciliation pipeline during dogfooding; scraper stays loosely coupled.
- **Optional DB tracking** (`queries *postgres.Queries` may be nil) — unit tests stay simple; CLI gracefully degrades when `DATABASE_URL` is absent.
- **7 HTML fixture files** covering all 7 schema.org event shapes — gave high confidence in the normalizer before hitting live sites.
- **DB-first source config loading with YAML fallback** — `loadSourceConfigs` in `Scraper` tries the repository first; falls back to YAML transparently. The scraper works identically before and after `server scrape sync` is run.
- **`ScrapeAll` dispatches tier directly** (not via `ScrapeSource`) — avoids a double `loadSourceConfigs` call on every run while keeping the iteration logic in one place.

### Issues Found in Code Review (all fixed across 3 waves)

**Wave 1** (commit `0678950`) — context, docs, minor cleanup:

| Issue | Fix |
|-------|-----|
| Missing `scraperSource`/`tier`/`distanceKm` terms in JSON-LD context | Added to `contexts/sel/v0.1.jsonld` |
| Redundant interface var check in `router.go` | Removed |
| Sparse godoc on `ScraperSourceSummary` | Expanded |
| Unclear `enabled` guard comment in `ScrapeAll` | Added explanation comment |

**Wave 2** (commit `c4a679f`) — structural/code quality:

| Issue | Fix |
|-------|-----|
| `dbSourceToSourceConfig` duplicated in two files | Exported as `SourceConfigFromDomain`; removed duplicate |
| `SourceConfig.Notes` lost during YAML round-trip | Added `Notes string \`yaml:"notes,omitempty"\`` field |
| `GetByName` + `Upsert` TOCTOU race in `scrapeSyncCmd` | Single `Upsert` + compare `UpdatedAt == CreatedAt` for insert-vs-update detection |
| `context.Background()` in CLI DB ops (ignores SIGINT) | Replaced with `cmd.Context()` throughout |
| `scraper.go` run-tracking logic scattered across 3 scrape methods | Extracted `runWithTracking`, `updateRunFailed`, `updateRunCompleted` helpers |
| Pure-delegation `Service` wrapper in `domain/scraper` added unnecessary indirection | Removed; callers use `Repository` directly |

**Wave 3** (post-review tests + migration):

| Issue | Fix |
|-------|-----|
| No integration tests for `ScraperSourceRepository` | Added `scraper_sources_repository_test.go` with 13 tests covering all CRUD, list/filter, link/unlink, error paths |
| No test for `ListByOrg`/`ListByPlace` error → still 200 + omit field | Added `TestOrganizationsHandlerGetScraperSourcesError` and `TestPlacesHandlerGetScraperSourcesError` |
| Redundant `idx_scraper_sources_name` index (UNIQUE already creates one) | Migration `000033` drops it |

**Learning**: `insertOrganization`/`insertPlace` test helpers return `seededEntity{ID, ULID}` where `ID` is the DB-generated UUID and `ULID` is the application ULID text. Repository methods that reference join tables (`LinkToOrg`, `ListByOrg`, etc.) take the UUID — use `entity.ID`, not `entity.ULID`.

### Spec Divergences (cosmetic, intentional)

- SQLc field names differ slightly from spec: `SourceUrl` (not `SourceURL`), `EventsNew` (not `EventsCreated`), `EventsDup` (not `EventsDuplicate`). Column semantics are correct.
- Batch ingest response is async (202 Accepted); event created/dup counts come from the async status result, not the initial submit response. `IngestResult` handles both shapes.

### Follow-up Beads Filed

| Bead | Title | Priority |
|------|-------|----------|
| `srv-aany8` | Validate and fix GTA source config URLs against live sites | P3 |
| `srv-pfeud` | River job scheduling for periodic automated scrapes | P4 |
| `srv-5127b` | Admin UI for source management and run history | P4 |
| `srv-h264z` | Tier 2 headless browser support via Rod | P4 |

---

## Source Config Findings (Phase 2 expansion)

### Recon Script (`scripts/recon-venues.py`)

Crawls `https://t0ronto.ca/` directory (689 communities, 486 arts/culture org URLs).
Probes each URL for: tech stack, JSON-LD event count, `/events/` subpage, tribe REST API, candidate hrefs.
Output: `scripts/recon-output.tsv` — columns: `domain`, `final_url`, `status`, `tech_stack`, `jsonld_event_count`, `events_subpage`, `tribe_api`, `tier_guess`, `candidate_urls`.

**Tier guess logic:**
- `T0` — JSON-LD Event blocks found on page
- `T1` — WordPress/Drupal + events subpage or tribe API detected (server-rendered HTML)
- `T2` — Squarespace/Wix/React/Elementor/JetEngine detected (JS-rendered, needs headless)
- `SKIP` — 404, no event content, or ticketing-only aggregator

**Results from full run (486 orgs):** 13 T0, 249 T1, 148 T2, 76 SKIP.

### The Events Calendar (Tribe) WordPress Plugin

Very common in Toronto arts/culture WordPress sites. Detected via REST API endpoint at `/?rest_route=/tribe/events/v1/events` or `/wp-json/tribe/events/v1/events`.

**Two scraping approaches:**

1. **Tribe REST API (preferred for Tier 1)** — JSON endpoint, no CSS selectors needed:
   - `GET <base>/wp-json/tribe/events/v1/events?per_page=50`
   - Returns JSON with `events[]` array; each event has `title`, `start_date`, `url`, `description`, `venue`
   - **Cannot be used directly** — Tier 1 Colly scraper works on HTML, not JSON APIs. A future Tier 1.5 (API-first) or Tier 0 extension could support this.

2. **Tribe HTML list view (Tier 1)** — server-rendered HTML:
   - URL pattern: `<base>/events/list/` or `<base>/events/`
   - Event items: `article.type-tribe_events` or `.tribe-events-calendar-list__event-wrapper`
   - Title: `h2.tribe-events-list-event__title a` or `.tribe-event-url`
   - Date: `.tribe-event-date-start` or `time[datetime]`
   - Link: `h2.tribe-events-list-event__title a` (href)
   - **Caveat**: Default list view may use AJAX pagination — first page only is safe

3. **Tribe month view JSON-LD (Tier 0)** — some sites embed JSON-LD in the month calendar page:
   - URL: `<base>/events/` (not `list/`) on some configurations
   - Check for `<script type="application/ld+json">` blocks with `@type: Event`
   - If present, scrape as Tier 0 (zero config needed)

**Detection**: `/?rest_route=/tribe/events/v1/events` returning 200 JSON is the most reliable indicator.

### Key Learnings

- **T0 venues with Ticketmaster-backed JSON-LD**: The Danforth, Opera House, Mod Club, RBC Amphitheatre all have rich MusicEvent JSON-LD directly in their HTML — the data comes from Ticketmaster's widget but is embedded in the venue's own page. This is legitimate and scrapeable.
- **The scraper does NOT follow HTTP redirects** — always resolve final URL with `curl -sIL` before putting in config.
- **Squarespace CSS classes are hashed/unstable** — not suitable for Tier 1; always Tier 2.
- **Elementor/JetEngine** — renders via JS, Tier 2 only.
- **`max_pages` in Colly config only controls pagination** (following `selectors.pagination` links), not sub-page crawling.
- **`trust_level` guidelines**: 8–10 major institutional, 6–7 arts orgs/established venues, 5 community orgs, 1–4 aggregators.
- **Tribe `/events/` URL is not always the calendar** — some sites use `/events/` as a static page (e.g. Jazz Bistro's "Private Events" page). Always check the actual tribe calendar URL; often `/event-calendar/` or `/calendar/`.
- **Tribe "photo view" shortcodes are JS-rendered shells** — `tribe-events-view--photo` (and other shortcode-embedded views) render via React/AJAX. The static HTML is an empty shell. Only the classic list/month view is server-rendered.
- **`type-tribe_events` class absent when no upcoming events** — the CSS class only appears when events are actually rendered. A site with only past events or an off-season calendar will have no visible tribe markup on the future-events page.
- **Tribe REST API is the ground truth** — `GET /wp-json/tribe/events/v1/events?per_page=50` returns structured JSON regardless of which WordPress theme/view is used. Use this to verify if a site has events before investing in CSS selectors. (Not yet usable by the Tier 1 Colly scraper — future Tier 1.5.)
- **Standard tribe list-view CSS selector chain** (when server-rendered): `article.tribe-events-calendar-list__event` → `a.tribe-events-calendar-list__event-title-link` (title + href) → `time.tribe-events-calendar-list__event-datetime[datetime]` (ISO date).
- **WAF sites (ModSecurity)**: Some sites (e.g. summerworks.ca) block default curl User-Agent. Use `User-Agent: Mozilla/5.0 ...` in config's `headers` field.
- **`scrape test` is Tier 1 only** — requires `--event-list` flag + URL as first arg. For Tier 0 configs, use `scrape url <URL> --dry-run` to validate.
- **308 Permanent Redirect** blocks the no-redirect scraper — always resolve to final URL with `curl -sIL -o /dev/null -w "%{url_effective}"` before writing config. Trailing slash matters (`/events/` vs `/events`).
- **Tribe default view is month (AJAX)** — must always use `/events/list/` suffix to get server-rendered HTML list view. The month view loads events via JS.
- **ai1ec (All-in-One Event Calendar / Timely)** — another AJAX calendar plugin, similar to Tribe photo view. Tier 2 only.
- **Weebly + unstructured free-text events** — not scrapeable; skip.
- **Custom event card patterns** work well for T1 — e.g. Broadbent Institute's `div.bi-event-card` is stable, fully server-rendered, and clean to select.
- **T1 Webflow pattern**: Server-rendered cards use `.w-dyn-item` container. Individual site classes vary (e.g. InterAccess: `a.card.is-generic`).
- **CraftCMS events**: Server-rendered, typically in `.events-module` sections with custom card classes. Clean T1.
- **Joomla K2 component**: Server-rendered article list; `div.catItemView` container with `span.catItemDateCreated` for dates. Date format: `"Monday, 01 December 2025 00:00"`.
- **Shopify + Showpass/InLight**: Shopify stores that embed ticketing widgets are T2. The Showpass public API (`/api/public/events/?venue=<id>`) may be an alternative data source.
- **Roy Thomson Hall / Massey Hall (mhrth.com)**: React/Next.js SPA on Heroku — empty body without JS. T2.
- **Obsidian Theatre**: WordPress but returns blank body — likely anti-bot JS redirect. T2.
- **Church-Wellesley Village BIA**: Wix Events App, SSR container is empty. T2.
- **Heritage Trust (Elgin & Winter Garden)**: Custom PHP CMS; `div#month_list div.event-list-item`; date in `h2` text; detail link href contains `d=YYYY-MM-DD`. 4 events confirmed T1.
- **Charles Street Video**: Custom PHP CMS; events at `/events.php?submenu=events` (root URL is JS redirect shell but events page is directly accessible without JS). `td.item_card` container.
- **Downtown Yonge BIA**: WordPress + Divi + GeoDirectory AJAX. `/events/` is a marketing overview page, not a dated event feed. SKIP.
- **Cabbagetown South BIA**: Tribe Events WordPress; `@type:Event` JSON-LD confirmed. T0.
- **Caribbeantales Festival / Blood in the Snow / Fivars.net**: Tribe API responds but 0 events (off-season or inactive). SKIP until in-season.
- **Greektown Toronto**: Tribe API 0 events; static `/art-events/` page. SKIP.
- **Small World Music**: WordPress + Visual Composer; custom `marcato_show` post type; `article.archivedEvent` → `div.eventBoxDate` (text date). 7 events T1.
- **Broadbent Institute**: WP custom `broadbent_events` post type; `div.bi-event-card` → `span.bi-event-card__date-start`. T1.
- **Bloor-Yorkville BIA**: WP + Elementor; use `/category/events/` (not `/by-events/` which is nav-only). Date not in listing markup — falls back to link follow.
- **Bloor Annex BIA**: WP archive; `article .blog_postmeta .post-date`. Mixed events/news feed — trust_level 4.
- **Broadview-Danforth BIA**: Joomla K2; `div.catItemView h3.catItemTitle a` + `span.catItemDateCreated`. Mixed news/events.
- **aluCine Festival**: React SPA (base44 platform + Supabase backend). T2/SKIP.
- **Bangiya Parishad**: WP; `div.event-two__single`; split date: day in `span`, month in `p` inside `div.event-two__date`. Very sparse (1–4 events/year).

## Constitution Check

| Requirement | Status | Evidence |
|-------------|--------|----------|
| Schema.org alignment | PASS | Extracts native schema.org/Event JSON-LD; normalizes to SEL's schema.org-aligned EventInput |
| Provenance tracking | PASS | Each scraped event carries source URL, source event ID, scraper trust level |
| License compliance | PASS | Source config declares license; scraper passes to ingest API; SEL rejects non-CC0 |
| Robots.txt compliance | PASS | Colly respects by default; Tier 0 checks manually |
| Rate limiting | PASS | 1 req/sec default; configurable per source; transparent User-Agent |
| Deduplication | PASS | Uses existing 3-layer dedup via batch ingest API |

## Risks

| Risk | Severity | Mitigation |
|------|----------|------------|
| Few Toronto sites have JSON-LD | Medium | Tier 1 CSS selectors cover non-JSON-LD sites; advocacy for schema.org adoption |
| Schema.org variants break parser | Medium | Extensive normalization with fallbacks; test against real sites |
| Sites block scraper | Low | Transparent User-Agent; respect robots.txt; manual outreach |
| Scraper generates low-quality data | Medium | --dry-run for testing; trust_level controls merge priority; admin review queue |

## Test Strategy

- **Unit tests**: JSON-LD extraction from sample HTML, normalization of all schema.org variants, config loading/validation
- **Integration tests**: Scrape a test server (httptest) with known JSON-LD, verify EventInput output
- **E2E tests**: Scrape real staging sources, verify events appear in API responses
- **Contract tests**: Verify scraped JSON-LD produces valid SHACL shapes
