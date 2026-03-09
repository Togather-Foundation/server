# Disabled Scraper Sources — Status and Fix Paths

**Last reviewed:** 2026-03-09  
**Audit bead:** `srv-mo1xw` (orpheus-choir-toronto re-inspection) — re-inspected orpheus-choir-toronto, confirmed structural blocker, documented fix path  
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

| Category | Sources | Effort |
|----------|---------|--------|
| Seasonal — re-enable on a calendar trigger | heritage-toronto, imagine-native, inside-out | None |
| T3 REST API — unblocked by `srv-hi014` | burdock-brewery, workman-arts | Low (config only once `srv-hi014` lands) |
| T2 widget never renders (third-party embed) | rcmusic, hot-docs | Low–Medium (find underlying API endpoint) |
| Freeform layout — no repeating container | orpheus-choir-toronto | Low (contact venue for minor markup edit) |
| Need depth-2 (detail-page) scraping | obsidian-theatre, east-end-arts, theatre-passe-muraille | Medium (new feature) |
| Cross-origin iframe — config written, pending verification | lula-lounge | Low (verify selectors, flip enabled) |
| ~~Cross-origin iframe~~ — **now Tier 1 static** | reel-asian | **Done** (enabled) |
| Blocked by third-party widget / no listing page | luminato, church-wellesley-village-bia | Low–Medium (contact/API) |
| Blocked by bot protection | ago, ~~crows-theatre~~, st-lawrence-market, glad-day-bookshop | Medium–High |
| No events listing page | mammalian-diving-reflex | Blocked |

---

## 1. Seasonal — re-enable on a calendar trigger

No code changes required. These sources have working selectors and were disabled only
because they had zero events at review time.

### heritage-toronto
- **URL:** `https://www.heritagetoronto.org/events/`
- **Tier:** 1 (static WordPress)
- **Selectors:** `li.wp-block-post` / `h2.wp-block-post-title a` / `time.wp-element-button`
- **Action:** Set `enabled: true` and run `server scrape sync` when the spring/summer
  walking-tour season begins (typically April–May).

### imagine-native
- **URL:** `https://imaginenative.org/year-round/events/` (currently 404)
- **Tier:** 0
- **Action:** The year-round events page was removed. The festival runs in October.
  Revisit in September — check if a new listing URL has been added for the season.
  If not, consider a seasonal config pointing at the festival programme page.

### inside-out
- **URL:** `https://insideout.ca/`
- **Tier:** 0
- **Action:** Annual LGBTQ+ film festival, typically held in May. The Eventive widget
  that powers the programme page is empty off-season. Revisit in April; if the
  programme is published in server-side HTML before the festival, Tier 1 selectors
  may be feasible at that point.

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

**Two new T2 flags are now available** (`srv-n8qi1`) and should be tried against
each of these sources before escalating to an API/contact approach:

- `wait_network_idle: true` — after `wait_selector` resolves, waits an additional
  500 ms idle window for all in-flight XHR/fetch requests to settle. This is the
  primary fix for async widget embeds that fire cross-origin requests after the DOM
  is ready. The `--source-file` flag lets you test a draft config without touching
  the DB:

  ```bash
  SCRAPER_HEADLESS_ENABLED=true server scrape source \
    --source-file /tmp/draft.yaml --dry-run
  ```

- `undetected: true` — launches with `go-rod/stealth` evasions (patches
  `navigator.webdriver`, fake plugins, etc.). Useful when a widget refuses to load
  because it detects headless Chrome.

The recommended test sequence is:
1. Add `wait_network_idle: true` + a `wait_timeout_ms: 20000` to a draft config.
2. If still empty, also add `undetected: true`.
3. Capture rendered HTML with `server scrape capture <URL> --format inspect` to
   confirm widget content is present before writing selectors.

The fix path (API/contact) remains valid if both flags still fail to produce content.

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

### hot-docs
- **URL:** `https://hotdocs.ca/whats-on/watch-cinema`
- **Platform:** Agile Technologies box office widget
  (`boxoffice.hotdocs.ca/websales/agile_widget.ashx?orgid=2338&epgid=210`)
- **Blocked by:** Widget loads from a third-party domain; events never appeared in DOM
  after 25 s headless wait.
- **Next step:** Try `wait_network_idle: true` + `undetected: true`. The widget is a
  cross-origin embed so stealth evasions may help if the widget JS performs a
  bot-detection check before rendering.
- **Fallback:** Inspect the Agile widget JS to find the underlying data endpoint (the
  `.ashx` URL may accept a JSON format parameter). If publicly accessible, that endpoint
  could be scraped directly as Tier 3. The festival is annual (May) and low-volume enough
  that individual film detail pages could be scraped once slugs are known.

---

## 4. Need depth-2 (detail-page) scraping

These sources have usable listing pages but dates appear only on individual
event/show detail pages. The current scraper fetches only the listing URL; a
depth-2 mode would follow each event URL and extract additional fields.

### obsidian-theatre
- **URL:** `https://www.obsidiantheatre.com/season-listings/`
- **Platform:** WordPress Gutenberg, JS-rendered (`div.proditem`)
- **Extracts now:** Show title, image, URL to detail page.
- **Missing:** Performance dates — only on individual show pages.
- **Action:** Implement depth-2 scraping OR add a supplementary Tier 1 selector
  config that targets each show page's date field. Season has ~3–5 shows so manual
  entry is also viable as a short-term workaround.

### east-end-arts ✅ RESOLVED
- **URL:** `https://eastendarts.ca/`
- **Platform:** WordPress with REST API
- **Previous blocker:** Dates only in excerpt text, not structured `<time>` elements on listing page.
- **Resolution:** Upgraded to **Tier 3 REST API**. WordPress REST API endpoint
  `/wp-json/wp/v2/posts?per_page=10` returns structured RFC 3339 dates in the
  `date` field. Tested: 10 events extracted with title, start_date (RFC 3339), url.
- **Config:** `tier: 3`, `results_field: "."`, field_map maps `title.rendered`,
  `date`, `link`. Note: per_page=10 (not 100) to avoid HTTP client buffer issues.
- **Status:** `enabled: true` as of 2026-03-09. Config: `configs/sources/east-end-arts.yaml`.

### theatre-passe-muraille
- **URL:** `https://passemuraille.ca/25-26-season/`
- **Platform:** Elementor
- **Extracts now:** Show names in `h5` headings.
- **Missing:** Dates are in freeform `<p>` text (e.g. "Playing Feb. 1–21, 2026")
  with no CSS class. The season page nav submenu lists individual show URLs.
- **Action:** Depth-2 scraping to each show page + NLP date parsing from `<p>` text,
  OR contact the venue to request they add `<time>` elements or JSON-LD Event markup
  to their show pages (a single theme change would fix this permanently).

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

### luminato
- **URL:** `https://www.luminatofestival.com/events`
- **Widget:** `eventscalendar.co` (embed.js, `data-project-id="proj_QrmXauVHd8e0ohna92KJg"`)
  — same vendor as the eventscalendar.co embed on burdock-brewery.
- **Blocked by:** Widget JS does not execute within Rod's default timeout.
- **Next step:** Try `wait_network_idle: true` + `wait_timeout_ms: 20000` + `undetected: true`.
  If the widget populates, the selectors may be similar to whatever works for the
  eventscalendar.co vendor. Festival is annual (June).
- **Fallback:** Contact Luminato to request an iCal feed or JSON export. Eventscalendar.co
  may offer an export to the venue — worth asking.

### reel-asian ✅ RESOLVED
- **URL:** `https://www.reelasian.com/year-round/current-events/`
- **Widget:** Elevent JS library is loaded but only handles cart/account UI — events
  are static HTML in WordPress + Visual Composer columns.
- **Resolution:** Downgraded from Tier 2 to **Tier 1 static**. Selectors target
  `.vc_col-sm-4:has(.wpb_text_column)` event cards. 2 events extracted (Oscars
  Watch-Along Party, Fire Horse Award Gala). Config `enabled: true`.

### church-wellesley-village-bia
- **URL:** `https://www.churchwellesleyvillage.ca/events`
- **Platform:** Wix Thunderbolt OOI
- **Blocked by:** Wix OOI widget (`#comp-mbl3z9lq`) never renders within Rod timeout.
  The Wix platform requires full JS bootstrap that does not complete in headless Rod.
- **Action:** Wix exposes a public Events API for sites using Wix Events
  (`www.wixapis.com/events/v1/events`). Check if the site's sitemap
  (`/dynamic-events-sitemap.xml` has 13 event URLs) can be combined with the Wix
  API to retrieve structured event data. Needs a Wix API key from the venue.

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
- **Was blocked by:** Context deadline exceeded — Cloudflare JS challenge.
- **Fix:** T2 headless with `wait_network_idle: true`. No `undetected: true` needed.
  Uses `date_selectors: [".txtbox .date"]` for date-range parsing (e.g.
  "Feb 3, 2026 - Mar 8, 2026"). 7 events extracted. `all_midnight` quality warning
  is expected — listing page shows date ranges, not individual showtimes.
- **Status:** `enabled: true` as of 2026-03-05. Config: `configs/sources/crows-theatre.yaml`.

### st-lawrence-market
- **URL:** `https://www.stlawrencemarket.com/events/`
- **Blocked by:** Returns an anti-bot skeleton (~1 261 bytes, no event content).
- **Next step:** Try T2 with `undetected: true` + a realistic `Accept-Language` header.
  Stealth evasions may satisfy the bot-detection check that serves the skeleton page.
- **Fallback:** The market is City of Toronto-operated so a data feed request through
  the city's open data portal is also worth attempting.

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

## Priority Order

1. **Seasonal re-enables** — heritage-toronto in April, inside-out in April, imagine-native
   in September. Zero engineering effort; add calendar reminders.

2. **T3 REST tier (`srv-hi014`)** — implement the `rest:` config block. Once landed,
   burdock-brewery is a config-only enable (Showpass API confirmed working). Workman-arts
   follows once its Showpass venue ID is found. Lula-lounge follows via Eventbrite API.

3. **Try new T2 flags** — for rcmusic, hot-docs, luminato,
   st-lawrence-market: test `wait_network_idle: true` and/or `undetected: true` using
   `server scrape source --source-file /tmp/draft.yaml --dry-run`. If any widget
   now renders, add selectors and re-enable. Use `server scrape capture <URL> --format inspect`
   to confirm DOM content before writing selectors.

4. **Contact campaign** — email glad-day, luminato, lula-lounge, church-wellesley
   requesting an iCal/JSON feed or scraping permission. Low effort, potentially high yield
   for venues that are community-oriented.

5. **DevTools API hunting** — for rcmusic, hot-docs: if T2 flags still fail,
   use browser DevTools Network tab to capture the XHR endpoint each widget calls. If
   unauthenticated, a Tier 3 REST config (`srv-hi014`) can replace the broken T2 approach.

6. **Depth-2 scraping** — unlocks obsidian-theatre, east-end-arts, and potentially
   theatre-passe-muraille. Medium engineering effort.

7. **Bot-protected sites** (ago, st-lawrence-market) — defer if T2 stealth
   fails. High complexity, low reliability. Prefer contact/feed approach first.
