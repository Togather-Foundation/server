# Disabled Scraper Sources — Status and Fix Paths

**Last reviewed:** 2026-03-09  
**Audit bead:** `srv-mo1xw` (review all disabled sources) — re-inspected all 14 disabled sources, deep-dived REST APIs, discovered new viable paths  
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
| T3 REST API — unblocked by `srv-hi014` | burdock-brewery, workman-arts | Low (config only once `srv-hi014` lands) |
| T2 widget never renders (third-party embed) | rcmusic | Low–Medium (find underlying API endpoint) |
| Freeform layout — no repeating container | orpheus-choir-toronto | Low (contact venue for minor markup edit) |
| Need depth-2 / multi-URL scraping | obsidian-theatre, theatre-passe-muraille, luminato | Medium (new feature) |
| Cross-origin iframe — config written, pending verification | lula-lounge | Low (verify selectors, flip enabled) |
| ~~Cross-origin iframe~~ — **now Tier 1 static** | reel-asian | **Done** (enabled) |
| Blocked by third-party widget / no listing page | church-wellesley-village-bia | Low–Medium (contact/API) |
| Blocked by bot protection | ago, glad-day-bookshop | Medium–High |
| Site URL changed / content pending | hot-docs | Low (re-check after March 24 lineup announcement) |
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

## 2. T3 REST API — unblocked by `srv-hi014`

These sources use Showpass or another JSON REST API for ticketing. Once `srv-hi014`
lands (adds the `rest:` config block to Tier 3), these become trivial config-only
changes.

### burdock-brewery
- **URL:** `https://burdockbrewery.com/pages/music-hall`
- **Platform:** Shopify page embeds a Showpass iframe widget (AngularJS); also has an
  eventscalendar.co embed.
- **Blocked by:** Cross-origin JS barrier — both the Showpass iframe and the
  eventscalendar.co embed never produce DOM content accessible to Rod.
- **Fix:** Showpass public REST API at
  `https://www.showpass.com/api/public/events/?venue=17330` returns 34 events
  across 2 pages (`{ count, next, results: [...] }`). Confirmed working with
  `curl -sL`. Once `srv-hi014` lands:
  ```yaml
  tier: 3
  rest:
    endpoint: "https://www.showpass.com/api/public/events/?venue=17330"
    results_field: "results"
    next_field: "next"
    url_template: "https://www.showpass.com/{{.slug}}"
    field_map:
      name: "name"
      start_date: "starts_on"
      end_date: "ends_on"
      image: "image"
  enabled: true
  ```
- **Related bead:** `srv-hi014`

### workman-arts
- **URL:** `https://workmanarts.com/events/`
- **Platform:** WPBakery + Showpass widget
- **Blocked by:** Two-layer barrier — AJAX filter interaction required before events
  load, then events render via Showpass JS widget. Main programming is the Rendezvous
  With Madness festival (October/November).
- **Fix:** Find the Workman Arts Showpass venue ID (not yet confirmed). If they use
  Showpass, the REST API pattern from burdock-brewery applies directly. Check the
  Showpass iframe src on the page for a venue ID, or search
  `showpass.com/api/public/events/?q=workman`. Once `srv-hi014` lands and the
  venue ID is confirmed, enable with the same `rest:` block pattern as burdock.
- **Related bead:** `srv-hi014`

---

## 3. T2 widget never renders (third-party embed)

These sources load event data via a third-party JS widget that does not render
within Rod's timeout, or at all. Tier 2 headless is working correctly — the widget
simply never produces DOM content within the selector wait.

**Two T2 flags are available** (`srv-n8qi1`) and have been tested against these sources:

- `wait_network_idle: true` — after `wait_selector` resolves, waits an additional
  500 ms idle window for all in-flight XHR/fetch requests to settle.
- `undetected: true` — launches with `go-rod/stealth` evasions.

### rcmusic
- **URL:** `https://www.rcmusic.com/events-and-performances`
- **Platform:** AWS CloudSearch JS widget (`data-template="TPSPT.AWSFacetedSearchResults_Events"`)
- **Blocked by:** Page renders ~7 KB of empty containers; events are fetched via an AWS
  XHR endpoint that is not visible in page source.
- **Next step:** Try `wait_network_idle: true` + `wait_timeout_ms: 20000`. The XHR
  endpoint may fire and populate the DOM once network requests settle. Capture with
  `server scrape capture` to check if the widget content appears.
- **Fallback:** Use browser DevTools Network tab to capture the XHR request the widget
  makes. If the endpoint is unauthenticated, add as a Tier 3 REST config. Alternatively
  check for a WordPress iCal feed (`/events/feed/` or `?ical=1`).

---

## 4. Need depth-2 / multi-URL scraping

These sources have usable content but dates appear only on individual event/show
detail pages, or the listing page is unscrapeable while individual pages work.
A depth-2 mode (follow URLs from listing → extract from detail pages) or multi-URL
config would unlock these sources.

### luminato — BREAKTHROUGH: Individual show pages fully scrapeable
- **URL:** `https://www.luminatofestival.com/events` (listing — unscrapeable widget)
- **Platform:** DM website-builder + eventscalendar.co embed (Shadow DOM)
- **Status (2026-03-09):** The /events listing page uses an eventscalendar.co widget
  that wraps DOM in Shadow DOM — confirmed unscrapeable even with all headless flags.
  **However**: Individual show pages have **full structured data in static HTML**:
  - `/PennTeller` — Penn & Teller (June 5-6, 2026 at Meridian Hall)
  - `/10-days-in-a-madhouse` — 10 Days in a Madhouse (June 16-21, 2026 at Bluma Appel Theatre)
  - `/play-dead` — Play Dead (June 25-28, 2026 at Meridian Arts Centre)
  - Each page has: title, dates & times table, venue, run time, ticket links, artist bios.
  - Sitemap at `/sitemap.xml` has ~150 pages including all show URLs.
- **Viable paths:**
  1. **Multi-URL config** — List known show page URLs, scrape each individually (requires scraper enhancement).
  2. **Depth-2 scraping** — Follow URLs from nav menu or sitemap to discover show pages, extract structured data.
  3. **Sitemap-based discovery** — Parse `/sitemap.xml` to find show page URLs, filter by pattern.
- **Festival:** Annual, June. 2026 festival confirmed with active show pages.

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

### east-end-arts ✅ RESOLVED
- **URL:** `https://eastendarts.ca/`
- **Platform:** WordPress with REST API
- **Resolution:** Upgraded to **Tier 3 REST API**. WordPress REST API endpoint
  `/wp-json/wp/v2/posts?per_page=10` returns structured RFC 3339 dates.
- **Status:** `enabled: true` as of 2026-03-09.

### theatre-passe-muraille
- **URL:** `https://passemuraille.ca/25-26-season/`
- **Platform:** Custom CMS (not Elementor — corrected from earlier reports)
- **Status (2026-03-09):** Site is **UP and working** (previously reported unreachable).
  Full 25.26 season with 11+ productions. Dates in freeform `<p>` text (e.g.
  "Playing Feb. 1-21, 2026") with no CSS class. No repeating container per show.
  Individual show pages (`/butch-femme/`, `/through-the-eyes-of-god/`) have cleaner
  "Show Dates" sections. Nav submenu lists all show URLs.
- **Viable paths:**
  1. Depth-2 scraping: follow show URLs from nav submenu, extract dates from detail pages.
  2. NLP date parsing of freeform `<p>` text from season page (fragile).
  3. Contact venue to add `<time>` elements or JSON-LD Event markup.

---

## 4b. Freeform layout — no repeating container wrapper

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

## 5. Blocked by third-party widget

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

### hot-docs — site recovered, URL structure changed
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

### lula-lounge
- **URL:** `https://www.lula.ca/events` (404 — Wix removed the events page)
- **Situation:** The homepage now links to Eventbrite and Fever for ticketing, and to
  a Ticket Spot iframe widget (Wix embed from `geteventviewer.com`/`ticketspotapp.com`)
  on some pages. No dedicated events listing exists directly in the Wix page DOM.
- **Cross-origin iframe blocker resolved:** The same-origin policy that previously
  prevented CSS access to Ticket Spot iframe content is now resolved via the
  `headless.iframe:` config block (implemented in srv-mwy3y). The scraper uses CDP
  frame navigation to enter the iframe's execution context.
- **Status:** A working iframe config has been written for lula-lounge but the source
  remains `enabled: false` pending manual verification that selectors correctly extract
  events from the Ticket Spot iframe DOM.
- **Next step:** Run `SCRAPER_HEADLESS_ENABLED=true server scrape source lula-lounge --source-file configs/sources/lula-lounge.yaml --dry-run` to confirm ≥ 3 events are extracted, then set `enabled: true`.
- **Fallback:** Use the Eventbrite public API with the organizer ID
  (`4108527983` — `eventbrite.ca/o/lula-lounge-toronto-4108527983`) once `srv-hi014`
  (T3 REST tier) lands. Alternatively contact the venue to restore their events page.

### reel-asian ✅ RESOLVED
- **URL:** `https://www.reelasian.com/year-round/current-events/`
- **Resolution:** Downgraded from Tier 2 to **Tier 1 static**. Selectors target
  `.vc_col-sm-4:has(.wpb_text_column)` event cards. `enabled: true`.

---

## 6. Blocked by bot protection

### ago (Art Gallery of Ontario)
- **URL:** `https://ago.ca/whats-on`
- **Blocked by:** HTTP 403 on all listing pages — Cloudflare or explicit scraper block.
- **Action:** AGO has a collections API (`ago.ca/collections`) — check if an events
  feed is exposed under the same platform. Alternatively, AGO is a high-profile venue
  worth a direct contact request for a data feed.

### ~~crows-theatre~~ — ✅ RESOLVED

- **URL:** `https://crowstheatre.com/shows-events/`
- **Fix:** T2 headless with `wait_network_idle: true`. 7 events extracted. `enabled: true`.

### glad-day-bookshop
- **URL:** `https://www.gladdaybookshop.com/events`
- **Blocked by:** `robots.txt: Disallow: /*` for all user agents.
- **Action:** Legal blocker — contact the bookshop to request permission or a JSON/iCal
  feed. Glad Day is a culturally significant queer bookshop and likely supportive of
  inclusive event aggregation once the project is explained.

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
