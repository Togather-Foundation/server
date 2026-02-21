# GTA Event Source Configs

This directory contains YAML configuration files for each scrape source. All sources
are currently **disabled** pending Tier 1/Tier 2 support or data-access resolution.

## Status Summary (validated 2026-02-21)

| Source | Status | Reason | Fix Required |
|--------|--------|--------|--------------|
| toronto-symphony-orchestra | disabled | JS-rendered React SPA — no Event JSON-LD in static HTML | Tier 2 (Rod headless) — see bead srv-h264z |
| roy-thomson-massey-hall | disabled | JS-rendered SPA (both roythomsonhall.com + masseyhall.com) — ~75 bytes static HTML | Tier 2 (Rod headless) — see bead srv-h264z |
| hot-docs | disabled | No Event JSON-LD on any page (listing or detail) | Tier 1 CSS selectors for hotdocs.ca |
| glad-day-bookshop | disabled | robots.txt `Disallow: /*` for all user agents | Contact site owner / find alternate feed |
| harbourfront-centre | disabled | Only Yoast WebPage JSON-LD on event pages, not Event @type | Tier 1 CSS selectors for /whats-on/ + /event/* |
| toronto-reference-library | disabled | /events/ listing has only Library @type JSON-LD (org info), not Event | Tier 1 CSS selectors or investigate detail pages |

## Individual Source Notes

### toronto-symphony-orchestra.yaml
- Old URL `/concerts-events` returns 404. Correct listing: `/concerts-and-events/calendar`.
- Site is a React SPA — fetching static HTML yields no Event JSON-LD.
- Requires Tier 2 headless browser (Rod) to execute JS and extract structured data.

### roy-thomson-massey-hall.yaml
- Old URL was `mfrh.org/events` (404). Canonical domains are `roythomsonhall.com` and `masseyhall.com`.
- Both domains serve a JS-rendered SPA; static HTML response is ~75 bytes.
- Requires Tier 2 headless browser.

### hot-docs.yaml
- URL `hotdocs.ca/whats-on/films` is valid (200 OK).
- Neither the listing page nor individual event detail pages contain Event JSON-LD.
- Requires Tier 1 CSS selectors to extract title, date, description from DOM.

### glad-day-bookshop.yaml
- URL `gladdaybookshop.com/events` is accessible.
- `robots.txt` has `Disallow: /*` for all user agents — scraping is not permitted.
- Resolution: contact site owner for permission or locate a public data feed.

### harbourfront-centre.yaml
- Old URL `/events/` returns 301 redirect to `/`; the working listing is at `/whats-on/`.
- Individual event pages (`/event/*`) have JSON-LD but it is Yoast `WebPage` type, not `Event @type`.
- Requires Tier 1 CSS selectors to extract structured event data.

### toronto-reference-library.yaml
- Old URL `torontopubliclibrary.ca/programs-and-learning/events/index.jsp` returns 403.
- Canonical domain is `tpl.ca`; `/events/` listing (200 OK) contains only `Library @type` JSON-LD.
- Resolution: investigate individual event detail pages on `tpl.ca` for Event JSON-LD,
  or implement Tier 1 CSS selectors.

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
