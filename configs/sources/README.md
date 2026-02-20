# GTA Event Source Candidates

Research from docs/gta-events-report.md. These sites need raw HTML fetching to
confirm JSON-LD presence (the report's capture method couldn't see `<head>` content).

## Confirmed: Has Event-shaped JSON-LD (needs validation)

- Anirevo Toronto: https://toronto.animerevolution.ca/home-2/
  - Has Event JSON-LD but with quality issues (bad dates, placeholder images)
  - WordPress site, low scrape friction
  - Tier: 0 (JSON-LD present but may need cleanup)

## High-Priority Candidates (official arts orgs, schema.org unverified)

Test these with `curl -s <URL> | grep -i 'application/ld+json'` to confirm JSON-LD:

- Gardiner Museum: https://www.gardinermuseum.on.ca/event/smash-between-worlds-2024/
- Soulpepper Theatre: https://www.soulpepper.ca/performances/witch
- MOCA Toronto: https://moca.ca/events/performances/moca-after-hours-2025/
  - Note: reCAPTCHA mentioned — may block automated fetching
- The Power Plant: https://www.thepowerplant.org/whats-on/calendar/power-ball-21-club
- Royal Conservatory of Music: https://www.rcmusic.com/events-and-performances/royal-conservatory-orchestra-with-conductor-tania
- Harbourfront Centre: https://harbourfrontcentre.com/event/those-who-run-in-the-sky/
- Burlington PAC: https://burlingtonpac.ca/events/amanda-martinez/

## Additional Toronto Venues to Investigate

These weren't in the report but are major GTA arts/culture orgs worth checking:

- Toronto Symphony Orchestra: https://www.tso.ca/concerts-events
- Roy Thomson Hall / Massey Hall: https://www.mfrh.org/
- TIFF: https://www.tiff.net/events
- AGO (Art Gallery of Ontario): https://ago.ca/exhibitions-events
- ROM (Royal Ontario Museum): https://www.rom.on.ca/en/whats-on
- Hot Docs Cinema: https://hotdocs.ca/whats-on
- Tarragon Theatre: https://tarragontheatre.com/whats-on/
- Factory Theatre: https://www.factorytheatre.ca/whats-on/
- Canadian Opera Company: https://www.coc.ca/season
- National Ballet of Canada: https://national.ballet.ca/performances
- The Rex Jazz Bar: https://therex.ca/
- Toronto Public Library (events): https://www.torontopubliclibrary.ca/events/

## Notes

- Most CMS platforms (WordPress, Drupal, Squarespace) inject schema.org JSON-LD
  via SEO plugins (Yoast, RankMath, All-in-One SEO). It lives in `<head>` as
  `<script type="application/ld+json">`. Our Tier 0 extractor handles this natively.
- Sites with reCAPTCHA (MOCA) should be deprioritized or skipped.
- Some URLs above may be for specific events that have since passed. Use the
  events listing page to find current events.
