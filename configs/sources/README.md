# GTA Event Source Configs

This directory contains YAML configuration files for each scrape source.

## Status Summary (validated 2026-02-21)

| Source | Tier | Status | Events | Notes |
|--------|------|--------|--------|-------|
| harbourfront-centre | 1 | **enabled** | 106/page | CSS selectors on /whats-on/ |
| toronto-reference-library | 1 | **enabled** | 21/page | tpl.bibliocommons.com/v2/events |
| gardiner-museum | 1 | **enabled** | 7/page | gardinermuseum.on.ca/whats-on/ |
| soulpepper | 1 | **enabled** | 9/page | soulpepper.ca/performances |
| moca | 1 | **enabled** | 20/page | moca.ca/events; Elementor template ID hook — fragile |
| factory-theatre | 1 | **enabled** | 5/page | factorytheatre.ca/whats-on/ |
| tarragon-theatre | 1 | **enabled** | 13/page | tarragontheatre.com/whats-on/; no dates on listing page |
| coc | 1 | **enabled** | 7/page | coc.ca/tickets/2526-season; season URL — needs annual update |
| national-ballet | 1 | **enabled** | 9/page | national.ballet.ca/performances/202627-season/; season URL |
| rom | 1 | **enabled** | 120/page | rom.on.ca/whats-on/events; Drupal hidden-span duplication in names |
| hot-docs | — | disabled | — | JS-rendered; static `<main>` is empty — needs Tier 2 |
| toronto-symphony-orchestra | — | disabled | — | JS-rendered React SPA — see bead srv-h264z |
| roy-thomson-massey-hall | — | disabled | — | JS-rendered SPA (~75 bytes static HTML) — see bead srv-h264z |
| thepowerplant | — | disabled | — | Next.js SPA — needs Tier 2 |
| rcmusic | — | disabled | — | AWS CloudSearch dynamic load — needs Tier 2 or API |
| ago | — | disabled | — | 403 Cloudflare bot protection |
| glad-day-bookshop | — | disabled | — | robots.txt `Disallow: /*` — contact site owner |

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

### toronto-symphony-orchestra.yaml (disabled)
- JS-rendered React SPA — requires Tier 2 headless browser. See bead srv-h264z.

### roy-thomson-massey-hall.yaml (disabled)
- JS-rendered SPA (~75 bytes static HTML) — requires Tier 2. See bead srv-h264z.

### hot-docs.yaml (disabled)
- 22KB static HTML; `<main>` is empty — JS-rendered, needs Tier 2.

### thepowerplant (not configured)
- Next.js SPA — only `jsx-` classes in static HTML, no event content. Needs Tier 2.

### rcmusic (not configured)
- Events loaded dynamically via AWS CloudSearch into an empty `<div id="tps-aws-results">`. Needs Tier 2 or API reverse-engineering.

### ago (not configured)
- `ago.ca/exhibitions-events` returns 403 — Cloudflare bot protection. Try different User-Agent or find public API.

### glad-day-bookshop.yaml (disabled)
- `robots.txt` has `Disallow: /*` — contact site owner for permission.

---

## Research Archive: Unverified Candidates

The following sites from `docs/gta-events-report.md` have not yet been tested with
`server scrape inspect`. Run `/generate-selectors <url>` to evaluate:

- **Burlington PAC**: https://burlingtonpac.ca/events
- **TIFF**: https://www.tiff.net/events
- **The Rex Jazz Bar**: https://therex.ca/
- **Anirevo Toronto**: https://toronto.animerevolution.ca/home-2/
  - Has Event JSON-LD but with quality issues (bad dates, placeholder images) — try Tier 0 first
