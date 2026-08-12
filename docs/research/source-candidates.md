# GTA Event Source Candidates — To Test

Candidate scrape sources identified via research agents (2026-08-11). These have
**not** been validated with `server scrape inspect` / `/configure-source`. Work them
in priority order, then update this file with the verdict.

Test command per candidate:

```bash
./server scrape inspect <URL>
# then, once viable:
/configure-source <URL>
```

---

## Tier 0 / Tier 1 quick wins (recommended first)

| Name | URL (listing) | Platform | Est. tier | Est. events | Notes |
|------|---------------|----------|-----------|-------------|-------|
| Scotiabank Arena | `https://www.scotiabankarena.com/events` | carbonhouse (MLSE) | 1 | ~20+ | Server-rendered HTML, Ticketmaster deep links. High volume. |
| History Toronto | `https://www.historytoronto.com/events` | carbonhouse (Live Nation) | 1 | ~8 | Same platform as Scotiabank Arena. |
| The Rex Jazz Bar | `https://www.therex.ca` | Squarespace | 1 | ~10–15 | Active venue (2–3 shows/day). Squarespace events module. |
| Yuk Yuk's Toronto | `https://www.yukyuks.com/toronto` | Laravel custom | 1 | ~10/mo | Server-rendered. Unlocks 27+ city pages same template. |
| Paradise Theatre | `https://paradiseonbloor.com/home/` | WordPress | 0 | ~2/day | JSON-LD `ScreeningEvent`. Also `/live-events/`. |
| SummerWorks | `https://summerworks.ca` | WP + The Events Calendar | 0 | 27+ | WP REST API + JSON-LD. Immediate win. |
| Pride Toronto | `https://pridetoronto.com/events/` | WP + Elementor | 0–1 | 100+ | Events RSS feed at `/events/feed/`. |
| TIFA (Festival of Authors) | `https://festivalofauthors.ca` | WordPress | 0–1 | ~80+ | Yoast JSON-LD + RSS + `/wp-json/`. |
| AGO Events | `https://ago.ca/events/browse` | Drupal 11 | 0–1 | 50+ | Drupal JSON:API at `/jsonapi/`. |
| Toronto Fringe | `https://fringetoronto.com` | Drupal 10 | 1 | ~150 | June–July festival. CSS scrapable. |
| The Power Plant | `https://thepowerplant.org/whats-on/` | WordPress | 0 | ~10–15 | Major gallery, known gap. |
| Bata Shoe Museum | `https://batashoemuseum.ca` | WordPress | 0 | ~5–8 | Exhibitions + workshops. |
| The Theatre Centre | `https://theatrecentre.org` | WordPress | 0 | ~3–5 | Hub — captures resident co. events. |
| Nightwood Theatre | `https://nightwoodtheatre.net` | WordPress | 0 | ~4–6 | Major feminist theatre. |
| Toronto Dance Theatre | `https://tdt.org` | Custom | 1 | ~2–4 | Contemporary dance. |
| Liberty Village BIA | `https://libertyvillagebia.com/events/` | Squarespace | 0 | ~8 | Per-event ICS links. |
| Downtown Yonge BIA | `https://downtownyonge.com/events/` | WordPress | 1 | ~14+ | Rich dated listings. |
| UofT University College | `https://www.uc.utoronto.ca/about-connect-us-events` | Drupal | 1 | 12+ | Public lectures. |
| Toronto Field Naturalists | `https://torontofieldnaturalists.org/events/` | WordPress | 1 | seasonal | Nature walks. |
| Istituto Italiano di Cultura | `https://iictoronto.esteri.it/en/gli_eventi/calendario/` | Italian gov CMS | 1 | ~2 | Clean HTML. |
| Caribana / Toronto Carnival | `https://caribanatoronto.com/events` | Custom | 1 | ~40 | Structured listing. |
| CNE (The Ex) | `https://theex.com/schedule/` | WordPress | 1 | 100+ | Aug–Sep schedule page. |

## Tier 2 / headless candidates

| Name | URL (listing) | Platform | Est. events | Notes |
|------|---------------|----------|-------------|-------|
| TIFF | `https://www.tiff.net/films` | AWS WAF | ~200–300 | **Blocked** by WAF challenge; Rod needed. High value. |
| Danforth Music Hall | `https://www.thedanforth.com/shows` | Next.js SPA | ~10+ | `__NEXT_DATA__` extraction path. |
| Distillery District | `https://www.thedistillerydistrict.com/events/` | React/JS | unknown | JS-rendered, empty HTML fetch. |
| Goethe-Institut Toronto | `https://www.goethe.de/ins/ca/en/sta/tor/ver.cfm` | Adobe CQ/AEM | ~8+ | CineZeit, talks, installations. |
| Ontario Place | `https://ontarioplace.com/en/whats-on/` | React | ~5 | Sub-page crawling needed. |
| Meridian Hall (TO Live) | `https://www.tolive.com/meridian-hall` | Angular SPA | unknown | Pure `<app-root>`, no SSR. |
| Absolute Comedy Toronto | `https://www.absolutecomedy.ca/toronto` | Ember SSR | ~1 wk | JS-loaded cards. |

## Tier 3 / API candidates

| Name | URL | Platform | Est. events | Notes |
|------|-----|----------|-------------|-------|
| Luma Toronto community calendars | `https://lu.ma/discover?location=toronto` | Luma | 49 | Individual org calendars (happy town, Reading Rhythms, Build Club, Cursor Community). Same pattern as `1rg-space`. |
| Drake Hotel events RSS | `https://www.thedrake.ca/events/?feed=rss2` | WordPress | unknown | WP REST API likely available. |

## Blocked / deferred (skip unless unblocked)

- **TIFF** — AWS WAF (needs Rod; keep on Tier 2 list)
- **George Brown College** — 403 bot blocking
- **events.utoronto.ca** — times out; try ICS feed discovery / sitemap
- **~12 BIAs** — DNS/transport errors (Kensington Market, Little Italy, Corso Italia, Gerrard India Bazaar, Danforth Mosaic, Mount Pleasant, St. Clair West, Bloordale, Hillcrest Village, Ossington, The Beaches, Leslieville) — domains defunct or renamed
- **Canadian Music Week / Canadian Film Festival / Level UP Canada** — sites down/moved
- **Design Exchange** — domain compromised (casino spam); do not scrape
- **El Mocambo** — `.ca` is a content-farm; real venue is social-only
- **Gallery 44, FADO, Revival, Baby G, Rivoli, Lee's Palace, Axis Club, Velvet Underground, Cameron House, Dakota Tavern, Budweiser Stage** — transport errors during research; retry separately

## Low priority / low volume

- Toronto Zoo (12+, but not arts-focused)
- Black Creek Pioneer Village, Casa Loma, A Space Gallery, Prefix ICA, Trinity Square Video, Native Earth Performing Arts, Volcano, Bad Dog Theatre, Aluna Theatre, ProArteDanza, CCDT, fu-GEN, Theatre Gargantua
- Humber / Centennial College (seasonal / admissions-only)
- Toronto History Museums (shared Drupal calendar on toronto.ca — would need a shared config)

---

## Workflow

1. Work the "quick wins" rows via `/configure-source <URL>`.
2. For Tier 2 candidates, use `SCRAPER_HEADLESS_ENABLED=true` during validation.
3. Update this file: move tested rows to `configs/sources/README.md`, mark blocked ones here.
