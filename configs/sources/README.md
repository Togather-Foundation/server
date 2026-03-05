# GTA Event Source Configs

This directory contains YAML configuration files for each scrape source.

## Tiers

| Tier | Description | Tool |
|------|-------------|------|
| 0 | JSON-LD in static HTML — no selectors needed | `scrape url` |
| 1 | CSS selectors on static HTML | `scrape url` / `scrape source` |
| 2 | CSS selectors on JS-rendered HTML (headless Chromium via go-rod) | `scrape url --headless` / `scrape source` with `SCRAPER_HEADLESS_ENABLED=true` |

Tier 2 sources require `SCRAPER_HEADLESS_ENABLED=true` in the environment. Set
`SCRAPER_CHROME_PATH` to specify a custom Chromium binary (default: Rod download-on-demand).
Use `server scrape capture <URL> --format inspect` to analyze a JS-rendered page before
writing selectors.

## Headless Config Fields (Tier 2 YAML)

| Field | Type | Default | Notes |
|-------|------|---------|-------|
| `headless.wait_selector` | string | `body` | CSS selector to wait for before extracting. Use the most specific stable element on the page. |
| `headless.wait_timeout_ms` | int | 10000 | Max ms to wait for `wait_selector`. Increase for slow SPAs. |
| `headless.wait_network_idle` | bool | false | After `wait_selector` resolves, also wait for XHR/fetch requests to settle (500 ms idle window). Use for async widget embeds that populate the DOM after the initial selector appears. |
| `headless.undetected` | bool | false | Enable stealth evasions (patches `navigator.webdriver`, fake plugins) for sites that block headless Chrome. |
| `headless.pagination_button` | string | — | CSS selector for a JS "next page" button. If the site uses URL-based pagination, use `selectors.pagination` instead. |
| `headless.rate_limit_ms` | int | 1000 | Delay between page loads in ms. |
| `headless.headers` | map[string]string | — | Extra HTTP headers to inject (e.g. `Accept-Language`). |

## Status Summary

### Tier 1 (validated 2026-03-04)

| Source | Status | Events | Notes |
|--------|--------|--------|-------|
| harbourfront-centre | **enabled** | 106/page | CSS selectors on /whats-on/ |
| toronto-reference-library | **enabled** | 21/page | tpl.bibliocommons.com/v2/events |
| gardiner-museum | **enabled** | 7/page | gardinermuseum.on.ca/whats-on/ |
| soulpepper | **enabled** | 9/page | soulpepper.ca/performances |
| moca | **enabled** | 20/page | moca.ca/events; Elementor template ID hook — fragile |
| factory-theatre | **enabled** | 5/page | factorytheatre.ca/whats-on/ |
| tarragon-theatre | **enabled** | 13/page | tarragontheatre.com/whats-on/; no dates on listing page |
| coc | **enabled** | 7/page | coc.ca/tickets/2526-season; season URL — needs annual update |
| national-ballet | **enabled** | 9/page | national.ballet.ca/performances/202627-season/; season URL |
| rom | **enabled** | 120/page | rom.on.ca/whats-on/events; Drupal hidden-span duplication in names |
| dance-immersion | **enabled** | 10 | `/category/events/` WordPress archive; `article.post-entry` containers |
| heritage-toronto | disabled | 0 (seasonal) | Selectors ready (`li.wp-block-post`); off-season, re-enable in spring |
| east-end-arts | disabled | — | `article` containers present but no date elements in DOM |
| hot-docs | disabled | — | Agile box office widget (third-party JS); events never appear in DOM even headless |
| rcmusic | disabled | — | AWS CloudSearch JS widget into empty div — confirmed non-viable |
| ago | disabled | — | 403 Cloudflare bot protection |
| glad-day-bookshop | disabled | — | `robots.txt` has `Disallow: /*` — contact site owner |
| crows-theatre | disabled | — | Context deadline / Rod navigate timeout on both Tier 1 and Tier 2; likely Cloudflare |
| imagine-native | disabled | — | `/year-round/events/` returns 404; no replacement events listing found |
| inside-out | disabled | — | Eventive JS widget; DOM empty after headless render |
| theatre-passe-muraille | disabled | — | Elementor; dates in unstyled `<p>` tags with no consistent selector |
| workman-arts | disabled | — | AJAX filter + Showpass widget; events never in static or headless DOM |
| st-lawrence-market | disabled | — | Anti-bot skeleton response (all event slots empty); not viable |

### Tier 2 (validated 2026-03-04)

Requires `SCRAPER_HEADLESS_ENABLED=true`. All sources below were evaluated with
`server scrape capture <URL> --format inspect` against a headless Rod browser.

| Source | Status | Events | Platform | Notes |
|--------|--------|--------|----------|-------|
| 918-bathurst | **enabled** | 0 (seasonal) | WordPress | URL changed to `/ourevents/list/`; off-season at time of review |
| amici-ensemble | **enabled** | 4 | Elementor WP | `#concerts` section; column-based layout |
| beaches-jazz | **enabled** | 6 (seasonal) | WordPress | Seasonal festival; date headers used as name fallback |
| comedy-bar | **enabled** | 634 | Custom WP | URL changed to `/events/1` (Bloor venue); `div.card` containers |
| electric-island | **enabled** | 28 | Webflow | URL changed to `/artists` (Webflow CMS items); `/events` only has season headers |
| four-oh-one-richmond | **enabled** | 2 | Alpine.js/WordPress | `a.tiles__tile--event` containers; Alpine hydrates after load |
| history-toronto | **enabled** | 12 | Custom WP | Date from 3 child `<span>` elements concatenated |
| images-festival | **enabled** | 13 | Custom WP | Also fixed JSON tags bug (`SelectorConfig` — `srv-2db1q`) |
| mercer-union | **enabled** | 9 | Vue SPA | `div.grid-item` containers |
| roy-thomson-massey-hall | **enabled** | 12 | SPA | URL changed to masseyhall.mhrth.com/tickets/; `div.event-row` containers |
| toronto-holocaust-museum | **enabled** | 4 | Angular SPA | Requires 20s wait; Angular hydration on `app-root` |
| toronto-society-of-architects | **enabled** | 9 | Custom WP | `div.tsa-event` containers |
| toronto-symphony-orchestra | **enabled** | 189 | React SPA | tso.ca/concerts-and-events/calendar; `li.event-card` containers |
| west-queen-west-bia | **enabled** | 26 | WordPress + EventON | EventON AJAX; waits via `:has()` CSS selector |
| yohomo | **enabled** | 107 | Webflow | URL changed to homepage; date format "Tue . Feb 24" from `p.text-style-allcaps` |
| burdock-brewery | disabled | 0 | InLight Labs embed | Async embed times out; Showpass API has 31 events (`srv-71948` tracks future work) |
| church-wellesley-village-bia | disabled | 0 | Wix OOI | Widget never renders within Rod timeout; no extractable DOM |
| lula-lounge | disabled | — | Wix | `/events` URL returns 404; no replacement listing page found |
| mammalian-diving-reflex | disabled | 0 | Custom | `/shows/` returns 404; no events listing page found |
| obsidian-theatre | disabled | 0 | WordPress | Dates only on detail pages, not on listing |
| orpheus-choir-toronto | disabled | 0 | Gutenberg WP | No repeating container; `wp-block-columns` with `hr` separators — not selectable |
| reel-asian | disabled | 0 | WordPress + Elevent | Cross-origin Elevent iframe; same-origin policy blocks CSS access |
| tranzac | disabled | 25 | DatoCMS | No ISO dates in rendered DOM; DatoCMS API adapter needed (`srv-wz0h7`) |
| xtsc | disabled | 0 | Zuluru | Event titles in `data-event` attributes; CSS selectors can't extract attribute values |

## Individual Source Notes

### harbourfront-centre.yaml (Tier 1, enabled)
- Listing: `https://harbourfrontcentre.com/whats-on/` — 310KB static HTML.
- 106 events/page via `.wo-event` container.

### toronto-reference-library.yaml (Tier 1, enabled)
- Listing: `https://tpl.bibliocommons.com/v2/events` — 1.2MB static HTML, 21 events/page.
- Known: `cp-screen-reader-message` spans cause duplicated text in name/location fields. Acceptable — ingest normalization handles it.

### gardiner-museum.yaml (Tier 1, enabled)
- Listing: `https://gardinermuseum.on.ca/whats-on/` — 7 events/page via `article[role=item]`.
- Mix of exhibitions and events on one page; dates vary (ranges vs. specific times).

### soulpepper.yaml (Tier 1, enabled)
- Listing: `https://www.soulpepper.ca/performances` — 9 events/page via `article.listing-item`.
- Dates in `.run` div as ranges (e.g. "January 29 - March 1").

### moca.yaml (Tier 1, enabled)
- Listing: `https://moca.ca/events` — 20 events/page.
- Elementor/JetEngine site; uses template ID `div.elementor-20414 section` as stable hook — fragile if MOCA rebuilds its template.
- Dates concatenated from multiple `.jet-listing-dynamic-field__content` spans (includes cost + date + time).

### factory-theatre.yaml (Tier 1, enabled)
- Listing: `https://www.factorytheatre.ca/whats-on/` — 5 events/page via `.shows-block .column`.
- Dates in `<p>` with en-dash range format.

### tarragon-theatre.yaml (Tier 1, enabled)
- Listing: `https://tarragontheatre.com/whats-on/` — 13 productions/page.
- No dates on listing page — events will be dropped at normalisation without a start date.
- Dates only available on individual detail pages; needs Tier 2 depth scraping to unlock.

### coc.yaml (Tier 1, enabled)
- Listing: `https://www.coc.ca/tickets/2526-season` — 7 events via `div.events-grid__item`.
- Season-specific URL — needs annual update when COC publishes next season.

### national-ballet.yaml (Tier 1, enabled)
- Listing: `https://www.national.ballet.ca/performances/202627-season/` — 9 events via `li.upcoming-list-item`.
- Season-specific URL — needs annual update.

### rom.yaml (Tier 1, enabled)
- Listing: `https://www.rom.on.ca/whats-on/events` — 120 events across 3 pages with pagination.
- Drupal hidden-span pattern causes duplicated names (e.g. `"DinosaursDinosaurs"`). Ingest normalization handles it.
- Provided URL `/en/whats-on` returns 404; correct URL is `/whats-on/events`.

### toronto-symphony-orchestra.yaml (Tier 2, enabled)
- Fixed 2026-03-04. URL: `https://www.tso.ca/concerts-and-events/calendar`. 189 events.
- `li.event-card` containers; date in `div.event-card__date`; name in `h3.event-card__title`.
- Note: `:has(> child)` combinator is unsupported in goquery/cascadia — use plain `:has()` instead.

### roy-thomson-massey-hall.yaml (Tier 2, enabled)
- Fixed 2026-03-04. URL changed to `https://masseyhall.mhrth.com/tickets/`. 12 events.
- `div.event-row` containers; date in `div.event-date`; name in `div.event-title`.

### hot-docs.yaml (disabled)
- Events served via Agile Technologies box office widget (third-party domain). The widget JS
  loads event listings after page render; events never appear in DOM even with 25s headless wait.
  Not viable without API reverse-engineering.

### thepowerplant (not configured)
- Next.js SPA — only `jsx-` classes in static HTML, no event content. Needs Tier 2.

### rcmusic (not configured / disabled)
- Events loaded dynamically via AWS CloudSearch into an empty `<div id="tps-aws-results">`.
  Confirmed non-viable via headless inspection 2026-03-04. Needs API reverse-engineering.

### ago (not configured / disabled)
- `ago.ca/exhibitions-events` returns 403 — Cloudflare bot protection. Try different User-Agent or find public API.

### glad-day-bookshop.yaml (disabled)
- `robots.txt` has `Disallow: /*` — contact site owner for permission.

### dance-immersion.yaml (Tier 1, enabled)
- Fixed 2026-03-04. URL changed to `https://danceimmersion.ca/category/events/` (WordPress archive).
- `article.post-entry` containers; date in `time.date-container` (ISO 8601 datetime attr). 10 events.
- Note: dates are post-publication dates, not performance dates — acceptable for discovery.

### four-oh-one-richmond.yaml (Tier 2, enabled)
- Fixed 2026-03-04. `https://401richmond.com/events/`. 2 events.
- `a.tiles__tile--event` containers; Alpine.js hydrates after load; name in `h3`, date in `time`.

### lula-lounge.yaml (disabled)
- `lula.ca/events` returns 404 — Wix removed the events page 2026-03-04.
- Homepage links to Eventbrite and Fever; no dedicated events listing page.
- Eventbrite organizer page exists but scraping Eventbrite requires their API.

### imagine-native.yaml (disabled)
- `/year-round/events/` returns 404 as of 2026-03-04. No replacement events listing found.
- Homepage has `article.elementor-post` but these are news/blog posts, not structured events.

### crows-theatre.yaml (disabled)
- Consistently times out on both static fetch (context deadline exceeded) and headless Rod
  (navigate timeout) as of 2026-03-04. Likely behind Cloudflare bot protection or slow CDN.

### inside-out.yaml (disabled)
- Uses Eventive JS widget; DOM is empty after headless Rod render. Not viable without Eventive API.

### east-end-arts.yaml (disabled)
- `article` containers render in static HTML but no date elements appear in DOM.
  Events may be loaded via AJAX after the listing containers.

### theatre-passe-muraille.yaml (disabled)
- Elementor site; season listing has show names but dates are in unstyled `<p>` tags with no
  consistent CSS class or structure. Not selectable without structural change on the site.

### workman-arts.yaml (disabled)
- Events loaded via AJAX filter (Showpass widget). DOM is empty for event listings after headless render.

### st-lawrence-market.yaml (disabled)
- Returns an anti-bot skeleton response (all event content slots are empty spans).
  Not viable via CSS selectors.

### heritage-toronto.yaml (Tier 1, disabled — seasonal)
- Selectors ready: `li.wp-block-post` containers; date in `time.wp-element-button`.
- Currently off-season (0 events). Re-enable in spring when programming resumes.

---

## Research Archive: Unverified Candidates

The following sites from `docs/gta-events-report.md` have not yet been tested with
`server scrape inspect`. Run `/generate-selectors <url>` to evaluate:

- **Burlington PAC**: https://burlingtonpac.ca/events
- **TIFF**: https://www.tiff.net/events
- **The Rex Jazz Bar**: https://therex.ca/
- **Anirevo Toronto**: https://toronto.animerevolution.ca/home-2/
  - Has Event JSON-LD but with quality issues (bad dates, placeholder images) — try Tier 0 first
