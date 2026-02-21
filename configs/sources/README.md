# GTA Event Source Configs

This directory contains YAML configuration files for each scrape source.

## Status Summary (validated 2026-02-21)

| Source | Tier | Status | Events/page | Notes |
|--------|------|--------|-------------|-------|
| harbourfront-centre | 1 | **enabled** | 106 | CSS selectors on /whats-on/ |
| toronto-reference-library | 1 | **enabled** | 21 | CSS selectors on tpl.bibliocommons.com/v2/events |
| hot-docs | 1 | disabled | — | JS-rendered; static HTML is empty — needs Tier 2 |
| toronto-symphony-orchestra | 2 | disabled | — | JS-rendered React SPA — see bead srv-h264z |
| roy-thomson-massey-hall | 2 | disabled | — | JS-rendered SPA (~75 bytes static HTML) — see bead srv-h264z |
| glad-day-bookshop | — | disabled | — | robots.txt `Disallow: /*` — contact site owner |

## Individual Source Notes

### harbourfront-centre.yaml (Tier 1, enabled)
- Listing: `https://harbourfrontcentre.com/whats-on/` — 310KB static HTML, no JS rendering needed.
- Extracts 106 events per page using `.wo-event` container with child selectors for name, date, URL, image.
- Old URL `/events/` returns 301 → `/`; individual event pages have Yoast `WebPage` JSON-LD, not `Event`.

### toronto-reference-library.yaml (Tier 1, enabled)
- Listing: `https://tpl.bibliocommons.com/v2/events` — 1.2MB static HTML, 21 events per page.
- Extracts from `.cp-events-search-item` cards; pagination via `a.cp-pagination-btn--next`.
- Known: `cp-screen-reader-message` spans cause duplicated text in `name` and `location` fields
  (e.g. `"BendaleEvent location: Bendale"`). Acceptable for now; ingest normalization can strip it.
- Old URL `torontopubliclibrary.ca/programs-and-learning/events/index.jsp` returns 403.

### toronto-symphony-orchestra.yaml (Tier 2, disabled)
- Old URL `/concerts-events` returns 404. Correct listing: `/concerts-and-events/calendar`.
- Site is a React SPA — fetching static HTML yields no Event JSON-LD.
- Requires Tier 2 headless browser (Rod) — see bead srv-h264z.

### roy-thomson-massey-hall.yaml (Tier 2, disabled)
- Old URL was `mfrh.org/events` (404). Canonical domains are `roythomsonhall.com` and `masseyhall.com`.
- Both domains serve a JS-rendered SPA; static HTML response is ~75 bytes.
- Requires Tier 2 headless browser — see bead srv-h264z.

### hot-docs.yaml (disabled)
- URL `hotdocs.ca/whats-on/films` is valid (200 OK) but only 22KB static HTML; `<main>` is empty.
- JS-rendered — needs Tier 2, not Tier 1 CSS selectors.

### glad-day-bookshop.yaml (disabled)
- URL `gladdaybookshop.com/events` is accessible.
- `robots.txt` has `Disallow: /*` for all user agents — scraping is not permitted.
- Resolution: contact site owner for permission or locate a public data feed.

---

## Research Archive: Additional GTA Candidates

From `docs/gta-events-report.md` — sites not yet configured:

### Confirmed: Has Event-shaped JSON-LD (needs validation)

- Anirevo Toronto: https://toronto.animerevolution.ca/home-2/
  - Has Event JSON-LD but with quality issues (bad dates, placeholder images)
  - WordPress site, low scrape friction
  - Tier: 0 (JSON-LD present but may need cleanup)

### High-Priority Candidates (official arts orgs, schema.org unverified)

Test these with `curl -s <URL> | grep -i 'application/ld+json'` to confirm JSON-LD:

- Gardiner Museum: https://www.gardinermuseum.on.ca/event/smash-between-worlds-2024/
- Soulpepper Theatre: https://www.soulpepper.ca/performances/witch
- MOCA Toronto: https://moca.ca/events/performances/moca-after-hours-2025/
  - Note: reCAPTCHA mentioned — may block automated fetching
- The Power Plant: https://www.thepowerplant.org/whats-on/calendar/power-ball-21-club
- Royal Conservatory of Music: https://www.rcmusic.com/events-and-performances/royal-conservatory-orchestra-with-conductor-tania
- Burlington PAC: https://burlingtonpac.ca/events/amanda-martinez/

### Additional Toronto Venues to Investigate

- TIFF: https://www.tiff.net/events
- AGO (Art Gallery of Ontario): https://ago.ca/exhibitions-events
- ROM (Royal Ontario Museum): https://www.rom.on.ca/en/whats-on
- Tarragon Theatre: https://tarragontheatre.com/whats-on/
- Factory Theatre: https://www.factorytheatre.ca/whats-on/
- Canadian Opera Company: https://www.coc.ca/season
- National Ballet of Canada: https://national.ballet.ca/performances
- The Rex Jazz Bar: https://therex.ca/

## Notes

- Most CMS platforms (WordPress, Drupal, Squarespace) inject schema.org JSON-LD
  via SEO plugins (Yoast, RankMath, All-in-One SEO). It lives in `<head>` as
  `<script type="application/ld+json">`. Our Tier 0 extractor handles this natively.
- Sites with reCAPTCHA (MOCA) should be deprioritized or skipped.
- Some URLs above may be for specific events that have since passed. Use the
  events listing page to find current events.
