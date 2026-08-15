# Event Source Discovery Strategy (location-agnostic)

How to find and configure event sources for a Togather SEL node in ANY city.
Written 2026-08-15 from two rounds of Toronto discovery; the techniques here
are city-independent, with Toronto examples used only as illustrations.

The goal is not "scrape everything" — it is to reach broad, healthy, verified
coverage of a city's public events with the minimum number of configs. One
platform pattern often beats fifty hand-written scrapers.

---

## 1. Strategy overview

Discovery proceeds in rounds, each round targeting sources that are harder to
see than the last:

- **Round 1 — Visible venues.** Anything a search engine surfaces for
  "events in <city>" or "venue calendar <city>": major venues, museums,
  theatres, festivals, BIAs. Low-hanging fruit; usually 30-80% of a city's
  event volume lives here.
- **Round 2 — Less-visible sources.** Orgs that don't rank: small companies,
  niche presenters, community orgs. Found via *catalog enumeration* (grant
  lists, cultural directories, festival registries) and *ticketing-platform
  mining* (one platform account = one config).
- **Round 3 — Strategic platform patterns.** Not individual venues at all,
  but the shared infrastructure they sit on: a ticketing platform's public
  API, a venue-network's booking system. One config unlocks dozens of orgs.

Each round produces a candidate list (provenance-tagged), which becomes
per-source worker tickets, which become validated YAML configs. Verdicts
(DONE/BLOCKED) are written back to the candidate list so nothing is
re-discovered twice.

---

## 2. Round 1: visible venues

1. **SEO mining**: search "events in <city>", "things to do <city>",
   "<city> events this weekend", venue-guide sites. Collect venue/org names.
2. **BIA / district guides**: Business Improvement Areas, neighbourhood
   tourism sites, "arts district" pages — they list member venues with
   calendars.
3. **Known anchor orgs**: the city's big museums, symphony, opera, main
   theatres, major festivals. Check them even if you expect they're already
   configured — re-verification catches dead/stale configs.
4. **Cross-reference against existing configs** and dedupe by domain
   (see §8).

---

## 3. Round 2: catalog enumeration (less-visible orgs)

The trick: find LISTS of cultural orgs, not the orgs themselves. Most cities
have several public lists that enumerate the "long tail":

### a) Arts council grant databases
Regional/national arts councils publish who they funded. Toronto used:
- Toronto Arts Council open data (ZIP of grant allocations, ~950 rows,
  ~480 unique orgs; may need a Referer header).
- Ontario Arts Council program-result pages (server-rendered; month index
  pages are JS shells — parse the annual pages, not the month pages).

Generalize: *"<region> arts council grants"* or *"<region> arts fund open
data"*. Grantees are active orgs by definition (they just got money) and
often small enough to be invisible to search engines. Filter to orgs whose
mandate is live events (music, theatre, dance, festivals, public art) and
skip purely-funding/advocacy bodies.

### b) Festival registries
National/regional festival aggregators list member festivals with dates.
Toronto used Culture Days (national festival registry; Laravel SPA with
per-event ICS routes — `/events/{id}/ics` — but no server-rendered
enumeration, so it's a discovery source, not a scrape target).

### c) Linked-data / reconciliation services
If the node has an event-knowledge-graph integration, use its reconciliation
API to enumerate organizations by city. Toronto used Artsdata
(W3C reconciliation at `api.artsdata.ca/recon`, SPARQL at
`query.artsdata.ca/query` — note: NOT `/sparql`). Query for orgs with a
city/country location, filter to those with event-producing types, extract
their homepages. This is the most efficient enumeration source when
available.

### d) Cultural directories
City cultural plans, museum associations, theatre company directories,
artist-run centre networks — usually a static HTML list of members with
websites. Cheap to harvest.

---

## 4. Round 2: ticketing-platform mining

One platform account = one config pattern = many orgs. This is the highest
leverage work in the whole strategy.

### How to find which platforms your city uses
1. Take 10-20 known venue event pages and look at the URLs/domains: do they
   share a ticketing host? (`*.mhrth.com`, `*.eventbrite.ca/o/...`,
   `showpass.com/...`).
2. Search the platform's own directory: Eventbrite `/o/` pages, Showpass
   `/discover/<city>/`, any "venues using us" pages.
3. Check the platform cheatsheet (`docs/integration/event-platforms.md`) for
   the extraction patterns already supported by the scraper.

### Known platform patterns (from Toronto, all city-independent)
- **Eventbrite organizer pages**: `/o/<slug>-<org_id>` embeds all upcoming
  events in `__NEXT_DATA__` but contains ZERO `ld+json` blocks (Tier 0 dead).
  The Next.js organizer-profile frontend calls an UNAUTHENTICATED JSON API:
  `eventbrite.<tld>/organizer-profile/api/organizers/<ORG_ID>/events/?page=1&pageSize=200`
  — HTTP 200 with the scraper's own UA, no cookies/Cloudflare/OAuth.
  Response `{events:[{name,url,start_date,start_time,...}], hasMore, total}`.
  One Tier 3 REST config template covers every org; only `org_id` changes
  (it's the numeric suffix of the `/o/` slug). Caveat: start_date/start_time
  come as separate fields → times lost (all_midnight) until field_map
  templating exists; dates/names/URLs/venues correct.
- **Showpass**: public REST `showpass.com/api/public/events/?venue=<id>`
  (T3). Venue discovery via `/discover/<city>/`. Works for comedy clubs,
  concert halls, indie venues. Watch for `is_test:true` venue flags — a
  test account would pollute the feed; drop those.
- **Showclix → Leap Event Technology**: Showclix rebranded; the old
  S3-eventsbucket pattern is dead (NoSuchBucket). Live feed:
  `events.leapevents.com/events/<org>/<start>/<end>.json` but NESTED
  (`events_by_month.<month>.dates.<day>[]`) — needs a flatten engine feature
  (see §9). Orgs found via the platform's org numbers.
- **Tessitura TNEW instances** (many large venues/museums use Tessitura):
  `mass-tnew-prod.<host>.cloud` style endpoints. Toronto found a real
  public API (`POST /api/products/productionseasons`, no auth, one instance
  covers ALL venues under a management company — 77 productions / 102
  performances). Caveat: POST-only → blocked until the engine supports POST
  (see §9).

### Why this matters
A city where 30 venues all use Eventbrite needs ONE config template + 30
one-line org configs, not 30 bespoke scrapers. Always check platforms before
writing per-venue CSS scrapers.

---

## 5. Round 3: strategic tickets

When a platform pattern is identified, create a STRATEGIC ticket (not a
per-source ticket) to: (a) find the public API/feed, (b) validate one config
template, (c) write 2-3 proof configs, (d) document the org-ID lookup for
the rest. Strategic tickets outperform per-source tickets and should be
prioritized ahead of them.

---

## 6. Tier decision framework

For each candidate URL, detect the platform and pick a tier (see
`docs/integration/event-platforms.md` for the full cheatsheet):

- **Tier 0 — structured data in HTML**: JSON-LD (`ld+json` Event blocks),
  ICS/iCal feeds, GraphQL embedded state. Best tier; no JS needed.
  Check: grep for `ld+json`, look for `?ical=1` / `calendar.ics` / Google
  Calendar embed URLs.
- **Tier 1 — static HTML**: server-rendered pages with event list markup.
  Carbonhouse, Webflow, some WordPress themes. Extract via CSS selectors.
- **Tier 2 — JS-rendered (headless)**: Wix, React SPAs, JS-populated
  containers. Requires headless browser + `wait_selector` on the POPULATED
  container (not `body`) + `wait_network_idle` for async widgets. Always the
  LAST resort — check for an API behind the SPA first (many "headless" sites
  have a public JSON endpoint; see §4).
- **Tier 3 — REST API**: public JSON endpoints, paginated or full-dump.
  Prefer over Tier 2 whenever a public API exists — even if it's not
  documented, it's often visible in the page source's `fetch` calls.

Key heuristic: **T3 REST always beats T2 headless when a public API
exists.** A "React SPA" is usually a Tier 3 REST candidate with extra steps.

---

## 7. Blockers and how to recognize them

- **Cloudflare / bot walls**: 403 or a challenge page to the scraper's UA
  while plain curl gets 200. Options: try a browser UA; try the API behind
  the wall (often unauthenticated); undetected headless; or document as
  BLOCKED with the fix path. Some walls are fingerprint-based — plain curl
  will NOT reproduce them; test with the scraper's actual UA.
- **Seasonal lulls**: a calendar with 0 upcoming events in August may be
  fully working in September. Check the venue's season pattern before
  declaring BLOCKED; write the config disabled with a "revisit" date.
- **Off-season festivals**: annual festivals show nothing 10 months/year.
  Document and re-check at next season start.
- **Test accounts**: platforms sometimes expose `is_test:true` venues —
  drop them; they pollute the feed with garbage events.
- **Rebrands/domain moves**: a dead domain may have moved (showclix →
  leapevents, venue A → venue B). Search the org name before giving up.
- **Missing times**: listing pages often have date-only text (times live on
  detail pages). Acceptable as `all_midnight` if dates/names/URLs/venues are
  correct; note it in the config comment.
- **Engine capability gaps** (see §9): when a feed exists but the scraper
  can't parse it, write the config DISABLED with the exact blocker + the
  engine feature that would unblock it. This turns dead-ends into
  follow-up tickets.

---

## 8. Dedup and bookkeeping

- Maintain ONE candidate doc per discovery round (`docs/research/
  source-candidates-v2.md` style) with provenance tags per row
  ([Artsdata]/[TAC]/[OAC]/[EB]/[SP]/[retry]).
- Dedup candidates against ALL existing configs — including DISABLED ones
  (a disabled config is "known, fix-not-new", not "new").
- Dedup by DOMAIN, not by name — orgs share domains (school faculties,
  sub-venues of a campus).
- Mark each row DONE/BLOCKED with the config slug or blocker when the worker
  finishes, so future rounds don't re-discover it.
- When a candidate's URL redirects to an already-configured domain, that's a
  match — drop it.

---

## 9. Engine capability gaps (feed exists, scraper can't parse)

When you hit one, write the config disabled with the blocker documented and
create a follow-up ticket referencing the exact code location. Known gaps as
of 2026-08-15 (all discovered live, all unblock candidates):

1. **Flatten nested results in Tier 3 REST** — feeds returning
   `events_by_month.<month>.dates.<day>[]`-style shapes can't be parsed
   (`results_field` must resolve directly to an array; `.` requires a bare
   array). Fix: `flatten: true` option walking the subtree collecting leaf
   arrays. See `internal/scraper/rest.go` (~L160-183).
2. **POST support in Tier 3 REST** — Tessitura TNEW is POST-only; engine is
   GET-only. Fix: allow method + body in the REST config.
3. **field_map value templating** — Eventbrite splits `start_date`/`start_time`
   into separate fields; field_map maps one source key per target field.
   Fix: templating like `{{.start_date}}T{{.start_time}}`.

Each gap is worth a strategic ticket — fixing one unblocks every city that
uses that platform, not just the current node.

---

## 10. Validation gates (per source)

A config is DONE only when:
- `./server scrape source <slug> --source-file configs/sources/<slug>.yaml
  --dry-run --verbose` yields **>= 3 events with non-empty names**.
- Quality warnings fixed where possible (all_midnight is acceptable if
  dates/names/URLs/venues are correct — document it).
- Domain assigned from site content (music/arts/culture/community/
  education/general). trust_level: 8 for museums/gov, 5 default.
- Enabled only after validation; write `enabled: false` FIRST, validate,
  then flip.

BLOCKED is a valid outcome — with a documented reason + fix path. Zero-event
configs must never be enabled silently.

---

## 11. Ticket workflow (agentic)

1. Discovery worker produces candidates doc (output only — no configs, no
   git).
2. Orchestrator dedups, then creates per-source tickets (self-briefing:
   READ-FIRST paths, condensed workflow, acceptance criteria, file scope,
   constraints) + strategic tickets for platform patterns + headless tickets
   for Tier 2 (staged for an environment with working headless).
3. Workers write configs only; orchestrator consolidates verdicts, commits
   as the agent identity, updates the candidates doc.
4. Human reviews and pushes.

---

## 12. What NOT to do

- Don't scrape venues already covered by a platform pattern (one Eventbrite
  org config beats a bespoke scraper).
- Don't burn headless cycles before checking for a public API.
- Don't enable configs with < 3 events, or test-account venues, or
  all-empty feeds — they pollute the review queue and the feed.
- Don't re-discover: always check the candidates doc + config list first.
- Don't treat a rebrand as a dead source — search the org name.
