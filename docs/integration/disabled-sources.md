# Disabled Scraper Sources — Status and Fix Paths

**Last reviewed:** 2026-04-17  
**Audit bead:** `srv-mo1xw` (review all disabled sources) — re-inspected the disabled source set, deep-dived REST APIs, discovered new viable paths  
**Previous audits:** 2026-03-05 (`srv-2oipr`, `srv-n8qi1`, `srv-mwy3y`)  
**Rod stealth/network-idle flags added:** 2026-03-05 (`srv-n8qi1`, closed)  
**Cross-origin iframe extraction added:** 2026-03-05 (`srv-mwy3y`, closed) — adds `headless.iframe:` config block; unblocks reel-asian and lula-lounge  
**T3 REST tier bead:** `srv-hi014` (open) — adds `rest:` config block; unblocks burdock-brewery, workman-arts, and others

This document summarises every source currently set `enabled: false`, the reason it
is disabled, and the recommended fix path. It is intended as a reference for future
contributors picking up this work.

For the authoritative per-source selector notes see `configs/sources/README.md`.  
For scraper architecture and how to add new sources see `docs/integration/scraper.md`.

---

## Summary by Fix Category

Scraper tiers: T0 = JSON-LD/microdata, T1 = static HTML CSS selectors, T2 = JS-rendered headless (Rod), T3 = API (GraphQL or REST JSON — `srv-hi014`).

**Cross-origin iframe extraction implemented:** bead `srv-mwy3y` (closed) adds `headless.iframe:` config block. The same-origin blocker that previously prevented CSS access to cross-origin iframe content is now resolved via CDP frame navigation.

**::attribute selector syntax added:** bead `srv-mo1xw` adds `selector::attr(name)` syntax to both Colly and Rod extractors for extracting HTML attribute values instead of text content.

| Category | Sources | Effort |
|----------|---------|--------|
| Seasonal — re-enable on a calendar trigger | heritage-toronto, imagine-native, inside-out, st-lawrence-market | None |
| T2 widget / SPA never renders | another-story-bookshop | Medium (find the underlying API or feed) |
| Freeform layout — no repeating container | orpheus-choir-toronto | Low (contact venue for minor markup edit) |
| Need depth-2 / multi-URL scraping | obsidian-theatre | Medium (new feature) |
| Narrative date text (no parseable dates) | reel-asian | Blocked by `srv-054rj` (P3) |
| Dates only on detail pages | tafelmusik | Blocked by `srv-qxj09` (P3) |
| Content model mismatch (news posts, not events) | bloor-annex-bia, bloor-yorkville-bia, moca | Blocked |
| Attribute-only / URL hidden in data-* | xtsc | Low (attribute extraction already exists; URL still needs JS parsing) |
| Bot protection / robots | glad-day-bookshop | Medium–High |
| Site URL changed / content pending | hot-docs | Low (re-check after festival announcement) |
| No events listing page | mammalian-diving-reflex | Blocked |

---

## 1. Seasonal — re-enable on a calendar trigger

No code changes required. These sources have working selectors and were disabled only
because they had zero events at review time.

### heritage-toronto
- **URL:** `https://www.heritagetoronto.org/events/`
- **Tier:** 1 (static WordPress)
- **Status (2026-03-09):** Site is UP and accessible. Page explicitly states "There are
  no upcoming events scheduled." Selectors confirmed matching actual DOM structure.
- **Selectors:** `li.wp-block-post.tribe_events` / `h3.wp-block-post-title a` / `div.is-meta-field.event-start-end-block div.value`
- **Action:** Set `enabled: true` and run `server scrape sync` when the spring/summer
  walking-tour season begins (typically April–May).

### imagine-native
- **URL:** `https://imaginenative.org/festival/schedule/`
- **Tier:** 2 (FacetWP widget) / potentially T3 REST
- **Status (2026-03-09):** Site is UP. WordPress REST API confirmed with Elevent custom
  post types: `events`, `event_showtimes`, `institute-events`, `artists`, `works`.
  Taxonomies include `event_dates`, `event_types`, `venue`, `genre`. All endpoints
  currently return empty results (seasonal — festival is June/October).
- **Action:** Re-check WP REST API in May when 2026 festival events are expected.
  If `/wp-json/wp/v2/events?per_page=100` returns structured dates, upgrade to T3 REST.
  Also check `/wp-json/wp/v2/event_showtimes` for individual screening times.

### inside-out
- **URL:** `https://insideout.ca/festival/program/`
- **Tier:** 2 (Eventive widget)
- **Status (2026-03-09):** 2026 programme page is LIVE. Festival dates: May 22-31, 2026.
  Eventive widget shows filters (Streams, Interests, Locations), date selectors, and
  "Download Program" link. But film list containers remain empty in headless — Eventive
  API requires authentication, JS widget never renders event content.
- **Action:** Check if "Download Program" link provides parseable data closer to festival.
  Look for alternative data endpoints (RSS, iCal). Eventive widget remains unscrapeable.

### st-lawrence-market
- **URL:** `https://www.stlawrencemarket.com/events/`
- **Status (2026-03-09):** Headless mode works with stealth evasions, but no events
  currently listed (seasonal market events typically start April).
- **Action:** Re-check in April when spring events are posted. T2 headless with
  `undetected: true` should work once content is present.

---

## 2. Need depth-2 / multi-URL scraping

These sources have usable content but dates appear only on individual event/show
detail pages, or the listing page is unscrapeable while individual pages work.
A depth-2 mode (follow URLs from listing → extract from detail pages) or multi-URL
config would unlock these sources.

### obsidian-theatre
- **URL:** `https://www.obsidiantheatre.com/season-listings/`
- **Platform:** WordPress Gutenberg, JS-rendered (`div.proditem`)
- **Status (2026-03-09):** Deep-dived WP REST API:
  - `/wp-json/wp/v2/event?per_page=100` — 6 items (special events). Dates embedded in
    content HTML `<h5>` tags. WordPress `date` field = publish timestamp, NOT event date.
  - `/wp-json/wp/v2/show?per_page=100` — 52 items (full archive). No dates in content
    or custom fields. Performance dates ONLY on individual show detail pages.
  - Current season: "How to Catch Creation" (April 23 - May 17, 2026 at Soulpepper Theatre).
- **Blockers:** REST API `date` = publish date; shows have no date fields; need depth-2 scraping.
- **Viable paths:**
  1. T3 REST for events CPT once HTML content date parsing is supported.
  2. Depth-2 scraping: listing page → show detail pages → extract from SHOW DETAILS section.
  3. Contact venue to add schema.org Event JSON-LD (one-time theme change).

## 3. Freeform layout — no repeating container wrapper

These venues use CMS layouts (WordPress Gutenberg, Elementor, etc.) where content
blocks are freeform siblings without a shared repeating container div. CSS selectors
require a repeating container to group related fields (name, date, location). These
sites need either a markup edit from the venue or a different extraction strategy.

### orpheus-choir-toronto
- **URL:** `https://orpheuschoirtoronto.com/2025-2026-concert-season/`
- **Platform:** WordPress Gutenberg (static HTML, no JS rendering)
- **Re-inspected:** 2026-03-09 via /configure-source
- **Content:** ~3–4 concerts per season (verified 4 events in current year)
- **Structure:** Concerts are `<h2 class="wp-block-heading">` (title) followed by
  `<h3>` siblings for date/location and `<p>` for description, all separated by
  `<hr class="wp-block-separator">` elements. Metadata is nested in different
  `wp-block-column` divs (left: text; right: poster image).
- **Blocker:** No wrapping div around each concert. CSS selector architecture
  expects `event_list` to be a repeating container with descendants. This page
  has: `h2` → `hr` → `h2`, with loose sibling elements in `wp-block-column` divs.
  Extracting "all siblings until next h2" requires custom logic beyond CSS selectors.
- **REST API:** WordPress exposes `/wp-json/wp/v2/pages?slug=2025-2026-concert-season`
  which includes the full rendered HTML in `content.rendered`, but the same sibling-grouping
  limitation applies.
- **Action:** Contact the choir to add a `<div class="concert">...</div>` wrapper
  around each concert block in the Gutenberg editor (a minor block-grouping edit,
  requires no custom CSS). Once wrapped, selectors become:
  ```yaml
  event_list: "div.concert"
  name: "h2.wp-block-heading"
  start_date: "h3.wp-block-heading"
  location: "h3.wp-block-heading:last-of-type"
  url: "a[href*='ticketspice']"
  ```
  Alternatively, given the very low volume (~4 events/year), manual entry via the admin
  UI is a pragmatic short-term solution. Long-term, a Gutenberg block group will solve
  this permanently.

---

## 4. Blocked by third-party widget

These sites delegate event rendering to an embedded third-party widget. The widget
either (a) never renders in Rod, (b) renders inside a cross-origin iframe, or (c) the
events page no longer exists.

### church-wellesley-village-bia
- **URL:** `https://www.churchwellesleyvillage.ca/events`
- **Platform:** Wix Thunderbolt OOI
- **Blocked by:** Wix OOI widget (`#comp-mbl3z9lq`) never renders within Rod timeout.
  The Wix platform requires full JS bootstrap that does not complete in headless Rod.
- **Action:** Wix exposes a public Events API for sites using Wix Events
  (`www.wixapis.com/events/v1/events`). Check if the site's sitemap
  (`/dynamic-events-sitemap.xml` has 13 event URLs) can be combined with the Wix
  API to retrieve structured event data. Needs a Wix API key from the venue.

### hot-docs
- **URL:** `https://hotdocs.ca/whats-on` (updated — old `/whats-on/cinema` returns 404)
- **Platform:** ApostropheCMS (custom Node.js CMS)
- **Status (2026-03-09):** Site is **BACK UP** (was returning 502 Bad Gateway). Main
  domain loads successfully. `/whats-on` shows series overview (Doc Soup, Jukedocs,
  For Viola, Stories We Told, Truth and Dare). Festival: April 23 - May 3, 2026
  ("Films and full lineup announced March 24").
- **URL changes:**
  - `/whats-on/cinema-films-events` → 404 (old URL, removed)
  - `/whats-on/cinema` → 404 (old URL, removed)
  - `/whats-on/film` → 404
  - `/api` → 404 (no REST API)
- **Blocker:** No cinema listings page found at current URL structure. Festival lineup
  expected to be announced March 24 — new URLs may appear then.
- **Next steps:**
  1. Re-check after March 24 lineup announcement for new film listings URLs.
  2. Inspect Agile widget JS for underlying data endpoint.
  3. Individual film detail pages may be scrapeable once slug discovery is possible.

### church-wellesley-village-bia
- **URL:** `https://www.churchwellesleyvillage.ca/events`
- **Platform:** Wix Thunderbolt OOI
- **Blocked by:** Wix OOI widget (`#comp-mbl3z9lq`) never renders within Rod timeout.
  The Wix platform requires full JS bootstrap that does not complete in headless Rod.
- **Action:** Wix exposes a public Events API for sites using Wix Events
  (`www.wixapis.com/events/v1/events`). Check if the site's sitemap
  (`/dynamic-events-sitemap.xml` has 13 event URLs) can be combined with the Wix
  API to retrieve structured event data. Needs a Wix API key from the venue.

### bloor-annex-bia
- **URL:** `https://www.blooryorkville.com/events`
- **Platform:** Wix
- **Blocked by:** The "events" page contains news articles and blog posts about the
  neighbourhood, not dated event listings. Content model is "What's New" / "News" not
  "Calendar of Events".
- **Action:** None — this source is fundamentally incompatible with event scraping.

### moca
- **URL:** `https://moca.ca/exhibitions/`
- **Platform:** Elementor / Jet Engine
- **Blocked by:** Elementor widgets use ambiguous CSS class naming (`.elementor-element`,
  `.jet-engine-equal-height`) that cannot be reliably distinguished from other pages on
  the same site. Multiple widget types (exhibitions, events, blog posts) share the same
  DOM structure.
- **Action:** Contact venue for a dedicated events page or data feed.

### glad-day-bookshop
- **URL:** `https://www.gladdaybookshop.com/events`
- **Blocked by:** `robots.txt: Disallow: /*` for all user agents.
- **Action:** Legal blocker — contact the bookshop to request permission or a JSON/iCal
  feed. Glad Day is a culturally significant queer bookshop and likely supportive of
  inclusive event aggregation once the project is explained.

---

## 7a. Content model mismatch (not events)

These sites have event-adjacent content but the scraper cannot extract usable events
because the content model differs from what the scraper expects.

### bloor-yorkville-bia
- **URL:** `https://www.blooryorkville.com/events`
- **Platform:** Wix
- **Blocked by:** The "events" page contains news articles and blog posts about the
  neighbourhood, not dated event listings. Content model is "What's New" / "News" not
  "Calendar of Events".
- **Action:** None — this source is fundamentally incompatible with event scraping.

### moca
- **URL:** `https://moca.ca/exhibitions/`
- **Platform:** Elementor / Jet Engine
- **Blocked by:** Elementor widgets use ambiguous CSS class naming (`.elementor-element`,
  `.jet-engine-equal-height`) that cannot be reliably distinguished from other pages on
  the same site. Multiple widget types (exhibitions, events, blog posts) share the same
  DOM structure.
- **Action:** Contact venue for a dedicated events page or data feed.

---

## 7b. Narrative date text (no parseable dates)

These sources embed dates in freeform narrative text that cannot be parsed with
standard date extraction logic. Blocked by `srv-054rj` (P3 — regex date extraction
for narrative text).

### reel-asian
- **URL:** `https://www.reelasian.com/year-round/current-events/`
- **Platform:** WordPress Visual Composer
- **Blocked by:** Dates appear in narrative prose (e.g., "On Sunday, March 15, 2026,
  Reel Asian will host...") rather than in discrete date elements. CSS selectors
  cannot extract partial text from within a paragraph.
- **Status:** `enabled: false`
- **Related bead:** `srv-054rj` — Engine: regex date extraction for narrative text

---

## 7c. Dates only on detail pages

These sources have listing pages but dates only appear on individual event detail
pages. The current scraper architecture does not support following URLs from listing
to detail pages for date extraction (Tier 1 only). Blocked by `srv-qxj09` (P3).

### tafelmusik
- **URL:** `https://www.tafelmusik.org/calendar`
- **Platform:** WordPress + custom theme
- **Blocked by:** Calendar/listings page shows event names and venues but NO dates.
  Dates appear only on individual event detail pages (`/event/...`). The current
  `follow_event_urls` feature only applies to Tier 0 JSON-LD sources, not Tier 1
  CSS selectors.
- **Status:** `enabled: false`
- **Related bead:** `srv-qxj09` — Engine: Tier 1 follow_event_urls with detail-page selectors

## 7d. SPA / hidden URL blockers

### another-story-bookshop
- **URL:** `https://www.anotherstory.ca/events/`
- **Platform:** React SPA with BookManager integration
- **Blocked by:** Headless browser never resolves the event list; page spins on an
  Ant Design loading state and no public API/feed is visible.
- **Action:** Re-check for a public API or feed URL; otherwise contact the venue.

### xtsc
- **URL:** `https://www.xtsc.ca/zuluru/events/`
- **Platform:** Zuluru
- **Blocked by:** Event names are available in `data-event`, but the actual event URLs
  are embedded in HTML-encoded data attributes and require custom parsing beyond the
  current selector model.
- **Action:** Keep disabled until URL extraction or a better data source is available.

---

## 7. No events listing page

### mammalian-diving-reflex
- **URL:** `https://www.mammalian.ca/shows/` (404)
- **Situation:** No events or shows listing page exists anywhere on the site. The
  `/projects/` section lists productions by status (current, in-development, archive)
  but these are project pages, not dated performance listings.
- **Action:** Monitor the site for a shows/events page being added. In the meantime,
  performances are typically listed on partner venue pages (e.g. TPM, Theatre Centre)
  which are already scraped or in scope.

---

## 8. ::attribute selector syntax (new capability)

Bead `srv-mo1xw` added `selector::attr(name)` syntax to both Colly (`colly.go`) and
Rod (`rod.go`) extractors. This allows extracting HTML attribute values (e.g. `datetime`,
`href`, `data-*`) instead of text content.

**Syntax:** `time[datetime]::attr(datetime)` extracts the `datetime` attribute from
matching `<time>` elements. Falls back to text content if the attribute is not present.

**Impact:** Enables extraction of structured date values from `<time datetime="...">` elements,
`data-*` attributes from widget containers, and `href` values from links. Used by XTSC
config to extract performer names from heading attributes.

---

## 9. Documented limitation — no_startDate (P3 engine work)

These 6 sources have parseable date fields in the DOM but the scraper cannot extract
valid start dates for some events. This is NOT a selector issue — the scraper runs
successfully, finds events, but some events lack parseable dates. These require
engine-level fixes (P3):

| Source | Found | Submit | Skipped | Reason |
|--------|-------|--------|---------|--------|
| aga-khan-museum | 22 | 13 | 6 | Day-of-week-only events (e.g. "BMO Free Wednesdays") with no specific date |
| charles-street-video | 144 | 47 | 97 | Archive events from 2023–2024 with no date span in the listing |
| gardiner-museum | 7 | 5 | 2 | Recurring programs with no specific date (e.g. "Every Thursday") |
| harbourfront-centre | 250 | 4 | 1 | Edge case: single event with unparseable date format |
| koffler-arts | 14 | 6 | 3 | Natural-language dates without year (e.g. "March 15th") |
| music-gallery | 20 | 19 | 1 | Past event using "From...until..." format with month-year only |

All 48 other sources submit successfully. These 6 are documented limitations that
require P3 engine work to resolve:
- `srv-054rj` — regex date extraction for narrative text (would help koffler-arts)
- `srv-qxj09` — Tier 1 follow_event_urls (would help tafelmusik)

---

## Priority Order

1. **Seasonal re-enables** — heritage-toronto in April, inside-out in May, imagine-native
   in May/September, st-lawrence-market in April. Zero engineering effort; add calendar reminders.

2. **T3 REST tier (`srv-hi014`)** — implement the `rest:` config block. Once landed,
   burdock-brewery is a config-only enable (Showpass API confirmed working). Workman-arts
   follows once its Showpass venue ID is found. Lula-lounge follows via Eventbrite API.

3. **Hot Docs re-check (March 24)** — festival lineup announcement expected; check for
   new film listings URLs. Low effort, high potential.

4. **Luminato individual pages** — HIGHEST VALUE new path. Individual show pages have
   full structured data. Requires multi-URL config or depth-2 scraping capability, but
   the data quality is excellent.

5. **Depth-2 scraping** — unlocks obsidian-theatre, theatre-passe-muraille, and luminato.
   Medium engineering effort but unlocks 3 major Toronto arts venues.

6. **Try new T2 flags** — for rcmusic: test `wait_network_idle: true` using
   `server scrape source --source-file /tmp/draft.yaml --dry-run`.

7. **Contact campaign** — email glad-day, luminato, church-wellesley, orpheus-choir
   requesting an iCal/JSON feed or scraping permission / minor markup changes.

8. **Bot-protected sites** (ago) — defer if T2 stealth fails. Prefer contact/feed approach.
