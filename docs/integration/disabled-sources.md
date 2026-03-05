# Disabled Scraper Sources — Status and Fix Paths

**Last reviewed:** 2026-03-05  
**Audit bead:** `srv-2oipr` (closed)

This document summarises every source currently set `enabled: false`, the reason it
is disabled, and the recommended fix path. It is intended as a reference for future
contributors picking up this work.

For the authoritative per-source selector notes see `configs/sources/README.md`.  
For scraper architecture and how to add new sources see `docs/integration/scraper.md`.

---

## Summary by Fix Category

| Category | Sources | Effort |
|----------|---------|--------|
| Seasonal — re-enable on a calendar trigger | heritage-toronto, imagine-native, inside-out | None |
| Need Tier 3 JSON/REST API adapter | burdock-brewery, rcmusic, hot-docs | High (new feature) |
| Need depth-2 (detail-page) scraping | obsidian-theatre, east-end-arts, theatre-passe-muraille, orpheus-choir-toronto | Medium (new feature) |
| Blocked by third-party widget | luminato, reel-asian, church-wellesley-village-bia, lula-lounge, workman-arts | Low–Medium (contact/API) |
| Blocked by bot protection | ago, crows-theatre, st-lawrence-market, glad-day-bookshop | Medium–High |
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

## 2. Need Tier 3 JSON/REST API adapter

These sources have publicly accessible JSON APIs but the scraper currently only
supports JSON-LD (Tier 0), CSS selectors (Tier 1/2), and the DatoCMS GraphQL adapter
(Tier 3 prototype). A generalised Tier 3 field-mapping config (URL → JSON path mapping)
would unlock these and many future venues on the same platforms.

### burdock-brewery
- **Platform:** Showpass (InLight Labs embed)
- **API:** `https://www.showpass.com/api/public/events/?venue=17330`
  — returns 31 events with name, date, image, URL. No auth required.
- **Blocked by:** Scraper has no raw-JSON field-mapping adapter.
- **Action:** Implement a Tier 3 JSON adapter (see below). Map `name`, `starts_at`,
  `ends_at`, `url`, `image` from the Showpass response envelope.
- **Related bead:** `srv-71948`

### rcmusic
- **URL:** `https://www.rcmusic.com/events-and-performances`
- **Platform:** AWS CloudSearch JS widget
- **Blocked by:** The page renders ~7 KB of empty containers; events are fetched from
  an AWS endpoint that is not directly visible in page source.
- **Action:** Use browser DevTools Network tab (or Rod's network interception) to
  capture the XHR/fetch request the widget makes. If the AWS endpoint is unauthenticated,
  add it as a Tier 3 JSON source. Alternatively check if rcmusic exposes an iCal feed
  (many WordPress calendars do via `/events/feed/` or `?ical=1`).

### hot-docs
- **URL:** `https://hotdocs.ca/whats-on/watch-cinema`
- **Platform:** Agile Technologies box office widget
  (`boxoffice.hotdocs.ca/websales/agile_widget.ashx?orgid=2338&epgid=210`)
- **Blocked by:** Widget loads from a third-party domain; events never appear in the
  main page DOM even after 25 s headless wait.
- **Action:** Inspect the Agile widget JS to find the underlying data endpoint. The
  `agile_widget.ashx` URL may accept a JSON format parameter. If publicly accessible,
  add as Tier 3 JSON. Alternatively, scrape individual film detail pages once slugs are
  known (festival is annual — low volume).

### Tier 3 JSON adapter — what needs building

A new source config field `json_path` (or similar) and a corresponding extractor that:

1. Fetches a JSON URL (with optional headers/auth).
2. Walks a dot-path to the events array (e.g. `results`).
3. Maps fields via YAML (e.g. `name: "name"`, `start_date: "starts_at"`).
4. Handles pagination via `next` URL or `page` query param.

This mirrors how `tranzac` uses the DatoCMS GraphQL adapter today but as a generic
REST/JSON variant. See `internal/scraper/` for the existing tier implementations and
`configs/sources/tranzac.yaml` for the DatoCMS Tier 3 reference config.

---

## 3. Need depth-2 (detail-page) scraping

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

### east-end-arts
- **URL:** `https://eastendarts.ca/`
- **Platform:** WordPress (VCEx theme)
- **Extracts now:** Article titles and URLs (`article.vcex-recent-news-entry`).
- **Missing:** Date — appears only in body text of individual posts, not as a
  structured `<time>` element on the listing page.
- **Action:** Depth-2 scraping to extract `<time>` or date metadata from each
  post page. Alternatively, check if the site exposes a WordPress REST API
  (`/wp-json/wp/v2/posts`) which includes `date` as a structured field.

### theatre-passe-muraille
- **URL:** `https://passemuraille.ca/25-26-season/`
- **Platform:** Elementor
- **Extracts now:** Show names in `h5` headings.
- **Missing:** Dates are in freeform `<p>` text (e.g. "Playing Feb. 1–21, 2026")
  with no CSS class. The season page nav submenu lists individual show URLs.
- **Action:** Depth-2 scraping to each show page + NLP date parsing from `<p>` text,
  OR contact the venue to request they add `<time>` elements or JSON-LD Event markup
  to their show pages (a single theme change would fix this permanently).

### orpheus-choir-toronto
- **URL:** `https://orpheuschoirtoronto.com/2025-2026-concert-season/`
- **Platform:** WordPress Gutenberg
- **Situation:** ~4 concerts per season. Dates are in `h3` siblings with no wrapper
  div — concerts are separated by `<hr>` elements, not by a repeating container CSS
  selectors can target.
- **Action:** Given the very low volume (~4 events/year), manual entry via the admin
  UI is the most pragmatic path. Alternatively, contact the choir to add a `div.concert`
  wrapper around each concert block (a minor Gutenberg edit).

---

## 4. Blocked by third-party widget

These sites delegate event rendering to an embedded third-party widget. The widget
either (a) never renders in Rod, (b) renders inside a cross-origin iframe, or (c) the
events page no longer exists.

### luminato
- **URL:** `https://www.luminatofestival.com/events`
- **Widget:** `eventscalendar.co` (embed.js, `data-project-id="proj_QrmXauVHd8e0ohna92KJg"`)
- **Blocked by:** Widget JS does not execute within Rod's timeout. No public
  eventscalendar.co API found.
- **Action:** Contact Luminato to request an iCal feed or JSON export. Eventscalendar.co
  may offer an export to the venue — worth asking. Festival is annual (June).

### reel-asian
- **URL:** `https://www.reelasian.com/year-round/current-events/`
- **Widget:** Elevent (cross-origin iframe, `elevent-cdn.azureedge.net`)
- **Blocked by:** Same-origin policy prevents CSS access to iframe content.
- **Action:** Elevent offers an API for venue partners. Contact Reel Asian to ask
  whether their Elevent account has API access that could be shared for aggregation.

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
- **URL:** `https://www.lula.ca/events` (404)
- **Situation:** Wix removed the events page. The homepage now links to Eventbrite
  and Fever for ticketing; no dedicated events listing exists on the site.
- **Action:** Use the Eventbrite public API with the organizer ID
  (`4108527983` — `eventbrite.ca/o/lula-lounge-toronto-4108527983`) once a Tier 3
  JSON adapter exists (see section 2). Alternatively contact the venue to restore
  their events page.

### workman-arts
- **URL:** `https://workmanarts.com/events/`
- **Platform:** WPBakery + Showpass widget
- **Blocked by:** Two-layer barrier — AJAX filter interaction required before events
  load, then events render via Showpass JS widget. Main programming is the Rendezvous
  With Madness festival (October/November); off-season year-round.
- **Action:** Check if Workman Arts uses Showpass (as Burdock does). If their Showpass
  venue ID is findable, the Tier 3 JSON adapter (section 2) would work here too.

---

## 5. Blocked by bot protection

### ago (Art Gallery of Ontario)
- **URL:** `https://ago.ca/whats-on`
- **Blocked by:** HTTP 403 on all listing pages — Cloudflare or explicit scraper block.
- **Action:** AGO has a collections API (`ago.ca/collections`) — check if an events
  feed is exposed under the same platform. Alternatively, AGO is a high-profile venue
  worth a direct contact request for a data feed.

### crows-theatre
- **URL:** `https://crowstheatre.com/shows-events/`
- **Blocked by:** Context deadline exceeded on static fetch; Rod navigate timeout on
  headless. Consistent across attempts (2026-03-04). Likely Cloudflare JS challenge.
- **Action:** Try a browser User-Agent header that matches a real browser fingerprint.
  If still blocked, contact the venue — they are a major mid-size Toronto theatre and
  likely receptive to an aggregation partnership.

### st-lawrence-market
- **URL:** `https://www.stlawrencemarket.com/events/`
- **Blocked by:** Returns an anti-bot skeleton (~1 261 bytes, no event content).
- **Action:** Headless Rod with a realistic User-Agent and cookie jar may work.
  The market is City of Toronto-operated so a data feed request through the city's
  open data portal is also worth attempting.

### glad-day-bookshop
- **URL:** `https://www.gladdaybookshop.com/events`
- **Blocked by:** `robots.txt: Disallow: /*` for all user agents.
- **Action:** Legal blocker — contact the bookshop to request permission or a JSON/iCal
  feed. Glad Day is a culturally significant queer bookshop and likely supportive of
  inclusive event aggregation once the project is explained.

---

## 6. No events listing page

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

2. **Contact campaign** — email glad-day, luminato, lula-lounge, crows-theatre, church-wellesley
   requesting an iCal/JSON feed or scraping permission. Low effort, potentially high yield
   for venues that are community-oriented.

3. **Tier 3 JSON adapter** — highest engineering leverage. Unlocks burdock-brewery immediately
   and makes lula-lounge (via Eventbrite), workman-arts (via Showpass), and many future venues
   trivial to add. See `srv-71948` for related bead.

4. **Depth-2 scraping** — unlocks obsidian-theatre, east-end-arts, and potentially
   theatre-passe-muraille. Medium engineering effort.

5. **Bot-protected sites** (ago, crows-theatre, st-lawrence-market) — defer. High complexity,
   low reliability. Prefer contact/feed approach first.
