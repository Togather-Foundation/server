# Event Platform Knowledge Base

**Purpose:** Agent-readable reference for the `/configure-source` command. Before
inspecting an unknown site's DOM, check the recognition cheatsheet below to identify
the platform. If a match is found, use the known selectors and headless flags as a
starting point rather than deriving everything from scratch.

**Last updated:** 2026-04-17

---

## Recognition Cheatsheet

Quick lookup by DOM signal. Find the first matching row; go to the named section for details.

| Signal | Platform | Recommended tier |
|--------|----------|-----------------|
| `<script id="__NEXT_DATA__">` in HTML | **Next.js** | T0 (JSON-LD in SSR HTML) |
| `data-wf-site` or `data-wf-page` attribute | **Webflow** | T1 (server-rendered CMS) |
| `class="tribe-events*"` or `/wp-json/tribe/events` | **WordPress + The Events Calendar (Tribe)** | T0 preferred, T1/T2 fallback |
| `li.wp-block-post` or `class="wp-block-*"` | **WordPress (Gutenberg block theme)** | T1 |
| `class="elementor-*"` | **WordPress + Elementor** | T1 or T2 |
| `window.wixBiSession` or `data-hook="*"` attributes | **Wix** | T2 (see Wix section) |
| `Shopify.theme` in JS or `product-form` attributes | **Shopify** | T2 or T3 (check third-party embed) |
| `data-events-calendar-app` attribute | **eventscalendar.co embed** | T2 (`wait_network_idle: true`) |
| `<script src="*squarespace*">` or `class="sqsrte-*"` | **Squarespace** | T0 or T1 |
| `class="eventlist-*"` on articles | **Squarespace (event block)** | T1 |
| `graphql.datocms.com` token in page JS | **DatoCMS (GraphQL)** | T3 (GraphQL) |
| `data-template="TPSPT.*"` | **AWS CloudSearch widget** | T2 (`wait_network_idle: true`) |
| `elevent-cdn.azureedge.net` in source/network | **Elevent widget** | T2 (`iframe:` config block) |
| `geteventviewer.com` or `ticketspotapp.com` iframe | **Ticket Spot** (Wix embed) | T2 (`iframe:` config block) |
| `.ashx?orgid=` URL pattern | **Agile Technologies box office** | T2 (`wait_network_idle: true`) |
| `<title>Just a moment...</title>` or `window._cf_chl_opt` or `id="challenge-error-text"` in curl output | **Cloudflare-protected** | T2 (`undetected: true`) |
| `showpass.com` link or `showpass-widget` | **Showpass** | T3 (REST API) |
| `showclix.com` link or `eventsbucket.s3.amazonaws.com` in network | **Showclix** | T3 (REST via S3 JSON) |
| `eventbrite.com/o/<org-id>` link pattern | **Eventbrite** | T2 (no public API) |
| `class="nuxt-*"` or `window.__NUXT__` | **Nuxt.js** | T0 or T3 (check DatoCMS) |
| `class="elementor-post__title"` | **WordPress + Elementor (news loop)** | T1 |
| `__vue_app__` in global JS | **Vue SPA** | T2 |
| `[data-reactroot]` or `react-root` id | **React SPA** | T2 |
| Angular `ng-version` attribute | **Angular SPA** | T2 |

---

## Platform Profiles

### 1. JSON-LD / schema.org (Tier 0)

**What it is:** Any site that embeds `<script type="application/ld+json">` with
`@type: Event` (or `MusicEvent`, `ExhibitionEvent`, etc.) in the page source.

**Detection signals:**
- `<script type="application/ld+json">` in `<head>` or `<body>`
- `"@type": "Event"` inside a JSON blob
- `inspect` output shows "JSON-LD events found: N"

**Recommended approach:** Use tier `0`. No selectors needed. The scraper extracts
`name`, `startDate`, `endDate`, `location`, `description`, `image`, `url` automatically.

**Known good patterns:**
- WordPress + The Events Calendar (Tribe) — emits one JSON-LD block per event
- WordPress block theme (Gutenberg) — depends on plugins; check
- Next.js SSR sites — JSON-LD commonly embedded by CMS or SEO plugins
- Squarespace event detail pages — full schema.org/Event; use `follow_event_urls`
  if the listing page is JS-rendered

**Config template:**
```yaml
tier: 0
follow_event_urls: false   # set true + event_url_pattern if listing is JS-rendered
```

**Working examples:**
- `buddies-in-bad-times.yaml` — WordPress + Tribe, 24 events
- `cabbagetown-south-bia.yaml` — WordPress + Tribe, JSON-LD on list page
- `bloor-west-village-bia.yaml` — JSON-LD Event array on list page
- `jazz-bistro.yaml` — Tribe WordPress, 60 events
- `mod-club.yaml`, `the-danforth.yaml`, `rbc-amphitheatre.yaml` — Next.js SSR sites

**Known issue — Next.js compression:** Some Next.js sites return gzip/brotli that
the T0 extractor does not decompress, causing "No events found" even though JSON-LD
is present in the raw HTML. Bug tracked. Workaround: T2 headless (Rod decompresses).

---

### 2. WordPress + The Events Calendar (Tribe)

**What it is:** The most popular WordPress events plugin. Adds `/events/` routes and
a dedicated list/month/day view. Emits JSON-LD (use T0 when possible).

**Detection signals:**
- URL path contains `/events/`, `/event/`, or `/tribe-events/`
- Body classes: `events-archive`, `tribe-events-*`
- CSS classes: `tribe-events-calendar-list__event-*`
- Source comment: `<!-- The Events Calendar` 

**URLs to try first:** `/events/list/` forces list view which is most scrapable.
Month view (`/events/`) is often AJAX-paginated — avoid it.

**Recommended tier:** T0 (JSON-LD emitted by Tribe). Fall back to T1 if JSON-LD
is absent; T2 only if the list view is also AJAX-loaded.

**Known selectors (T1/T2 fallback):**

List view (current Tribe versions):
```yaml
event_list: "article.tribe-events-calendar-list__event-article"
name: "h3.tribe-events-calendar-list__event-title a"
start_date: "time.tribe-events-calendar-list__event-datetime"
url: "h3.tribe-events-calendar-list__event-title a"
```

Legacy list view / alternate class name variant:
```yaml
event_list: "article.tribe-events-calendar-list__event"
name: "a.tribe-events-calendar-list__event-title-link"
start_date: "time.tribe-events-calendar-list__event-datetime"
url: "a.tribe-events-calendar-list__event-title-link"
```

"Latest past" fallback (when no upcoming events):
```yaml
event_list: "article.tribe-events-calendar-latest-past__event"
name: "h3.tribe-events-calendar-latest-past__event-title a"
start_date: "time.tribe-events-calendar-latest-past__event-datetime"
url: "h3.tribe-events-calendar-latest-past__event-title a"
```

**Headless note:** Most Tribe sites render the list view in static HTML (T1).
Some sites use JS-lazy-loading; add T2 if the static inspect shows empty containers.

**Working examples:**
- `918-bathurst.yaml` — T2, list view, 0 events off-season but selectors correct
- `bayview-leaside-bia.yaml` — T1, list view
- `ludwig-van-toronto.yaml` — older loop class variant

---

### 3. WordPress (Gutenberg block theme)

**What it is:** WordPress sites using the built-in Full Site Editor (FSE) with block
themes. The Query Loop block renders posts with `li.wp-block-post` wrappers.

**Detection signals:**
- `class="wp-block-post"` or `li.wp-block-post` in HTML
- `class="wp-block-post-title"`, `wp-block-post-date`
- Body class: `wp-site-blocks`, `is-layout-flow`

**Recommended tier:** T1 (server-side rendered).

**Known selectors:**
```yaml
event_list: "li.wp-block-post"
name: "h2.wp-block-post-title a"
start_date: "time.wp-block-post-date"   # if block is present
url: "h2.wp-block-post-title a"
```

**Working examples:**
- `heritage-toronto.yaml` — T1, `li.wp-block-post`, seasonal

---

### 4. WordPress + Elementor

**What it is:** Elementor is a drag-and-drop WordPress page builder. It produces
heavily nested `div.elementor-*` markup. Pages are usually server-rendered (T1)
unless the site uses AJAX to load posts.

**Detection signals:**
- `class="elementor elementor-*"` wrapper divs
- `class="elementor-widget-*"` for individual widgets
- `class="elementor-post__title"`, `elementor-post-date`
- `<link rel="stylesheet" id="elementor-*">`

**Recommended tier:** T1 for static post archives; T2 if content appears after scroll
or AJAX trigger.

**Known selector patterns:**

Post/news archive:
```yaml
event_list: "article.elementor-post"
name: "h3.elementor-post__title a"
start_date: "span.elementor-post-date"
url: "h3.elementor-post__title a"
```

Custom events section (no standard class):
```yaml
event_list: "#concerts .elementor-col-25"   # ID varies — inspect DOM
name: "h3.elementor-heading-title a"
start_date: ".elementor-widget-text-editor .elementor-widget-container"
url: "h3.elementor-heading-title a"
```

**Working examples:**
- `amici-ensemble.yaml` — T2, custom concerts section
- `bloor-yorkville-bia.yaml` — T1, post archive
- `imagine-native.yaml` (disabled) — T1, `article.elementor-post`

---

### 5. Webflow CMS

**What it is:** Webflow is a visual site builder that generates clean, semantic HTML
with custom CMS collections. Pages are fully server-rendered — no hydration needed.
Selectors are stable but project-specific (Webflow auto-generates class names like
`w-dyn-item`).

**Detection signals:**
- `data-wf-site` or `data-wf-page` attributes on `<html>`
- Class `w-dyn-list`, `w-dyn-item` on collection wrappers
- Source comment: `<!-- Webflow`

**Recommended tier:** T1 (server-rendered). T2 only if the collection uses lazy load.

**Known selector patterns (project-specific — always verify):**
```yaml
event_list: "div.w-dyn-item"           # generic Webflow CMS item
# OR project-specific class names, e.g.:
event_list: "a.card.is-generic"
name: "h3.heading-style-h3.is-card-title"
start_date: "div.text-size-small.is-inline-block"
url: "a.card.is-generic"

# Webflow often uses a.card as the event wrapper (whole card is a link)
# date is usually plain text — no <time datetime> attr
```

**Working examples:**
- `interaccess.yaml` — T1, `a.card.is-generic`, date as plain text
- `electric-island.yaml` — T2, `div.artist-collection-item`, date inside nested div

---

### 6. Wix

**What it is:** Wix is a hosted website builder. It has two architectures:
1. **Legacy Wix** — pre-renders large HTML (~1 MB), Colly can parse it without headless
2. **Wix Thunderbolt OOI (new)** — server bootstraps a React shell; the Wix Events
   widget requires full JS platform boot to render, which does not complete in Rod

**Detection signals:**
- `window.wixBiSession` in inline JS
- `data-hook="*"` attributes (Wix Events widget)
- `#comp-*` IDs on widget containers (OOI architecture)
- Source comment: `<!-- Wix`

**Wix Events widget selectors (T2 when it renders):**
```yaml
event_list: "li[data-hook=\"event-list-item\"]"
name: "[data-hook=\"ev-list-item-title\"]"
start_date: "[data-hook=\"ev-date\"] span"
url: "a[data-hook=\"ev-rsvp-button\"]"
location: "[data-hook=\"location\"]"
image: "[data-hook=\"ev-image\"] img"
```

**Legacy Wix (pre-rendered):**
- Use the auto-generated obfuscated class names (e.g. `div.KaEeLN`, `h5.font_5`)
- These are **fragile** — they change after any site redesign
- Tier 1 or Tier 2 (T2 is safer; T1 works if the HTML is pre-rendered)

**Wix Events public API (alternative to scraping):**
If the site uses Wix Events, a public API may be available:
`GET https://www.wixapis.com/events/v1/events` (requires Wix API key from venue).
The `/dynamic-events-sitemap.xml` path often lists all event URLs.

**Known failures — Thunderbolt OOI:**
The OOI widget container appears in HTML as an empty `<div id="comp-*">` and never
populates under Rod regardless of wait timeout. `wait_network_idle: true` and
`undetected: true` have not been confirmed to fix this. If the widget stays empty,
the only paths are the Wix API (needs venue cooperation) or scraping individual
event pages from the sitemap.

**Working examples:**
- `artnow-global.yaml` — T2, Wix Events widget (`data-hook`), `follow_event_urls: true`
- `beaches-jazz.yaml` — T2 config but effectively T1 (Wix pre-renders full HTML)
- `church-wellesley-village-bia.yaml` — disabled, Thunderbolt OOI, widget never renders

---

### 7. Shopify

**What it is:** Shopify is an e-commerce platform. Venues use it for ticketing pages
(e.g. Burdock Brewery). Events are usually not native Shopify features — they appear
as third-party JS embeds (eventscalendar.co, Showpass, etc.) loaded into a Shopify page.

**Detection signals:**
- `window.Shopify` or `Shopify.theme` in JS
- `product-form`, `shopify-section` in HTML
- URL patterns: `*.myshopify.com` or Shopify CDN references

**Recommended approach:** Check what third-party event widget is embedded. The
Shopify page itself is usually T1-scrapable but will only contain the widget's empty
container div — not the events. See `eventscalendar.co` section.

**Working examples:**
- `burdock-brewery.yaml` — Shopify page with eventscalendar.co embed (currently disabled)

---

### 8. eventscalendar.co embed

**What it is:** A third-party event calendar SaaS. Venues embed it via a `<div>` with
`data-events-calendar-app` and `data-project-id` attributes. The widget JS loads
event data via XHR after the DOM is ready.

**Detection signals:**
- `data-events-calendar-app` attribute on a div
- `data-project-id="proj_*"` attribute
- Script tag loading from `embed.eventscalendar.co`

**Recommended approach:** T2 with `wait_network_idle: true` + `wait_timeout_ms: 20000`.
The XHR requests fire after page load; `wait_network_idle` waits for them to settle.

```yaml
headless:
  wait_selector: "div[data-events-calendar-app]"
  wait_timeout_ms: 20000
  wait_network_idle: true
  undetected: true   # add if widget still empty
```

**Expected selectors once widget renders (unconfirmed — inspect DOM after capture):**
- Container: `div[data-events-calendar-app]` or `.cl-event-card` (TBC)
- Use `server scrape capture <URL> --format inspect` to confirm DOM after rendering

**Known project IDs:**
- `proj_T8vacNv8cWWeEQAQwLKHb` — Burdock Brewery
- `proj_QrmXauVHd8e0ohna92KJg` — Luminato Festival

**Fallback:** Contact venue or eventscalendar.co for iCal/JSON export.

**Known examples:**
- `burdock-brewery.yaml` — disabled, Shopify + eventscalendar.co embed
- `luminato.yaml` — disabled, same widget vendor

---

### 9. Squarespace

**What it is:** Hosted website builder. Events can be rendered either as server-side
HTML (using Squarespace's native event block) or as a JS calendar (requires T2/T0).

**Detection signals:**
- `class="sqsrte-*"` or `sqs-block*`
- `<script src="*squarespace.com*">` or `*.squarespace.com` CDN URLs
- `class="eventlist-event"` for event listing items

**Squarespace native event block (T1-scrapable):**
```yaml
event_list: "article.eventlist-event"
name: "h1.eventlist-title"
start_date: "time.event-date"
url: "a.eventlist-column-thumbnail"
```

**Squarespace calendar block (JS-rendered):**
- The calendar widget is heavily JS-rendered; the noscript fallback shows ~3 events
- Use T0 with `follow_event_urls: true` — Squarespace injects full JSON-LD on detail pages

**Working examples:**
- `music-toronto.yaml` — T1, native event block, 35 events
- `akin-collective.yaml` — T0, `follow_event_urls: true`, calendar block

---

### 10. DatoCMS / GraphQL (Tier 3)

**What it is:** DatoCMS is a headless CMS used by modern SPAs (often Nuxt.js or Next.js).
The site fetches content from DatoCMS's GraphQL API. Public read-only tokens are often
visible in page JS.

**Detection signals:**
- `graphql.datocms.com` in network requests or page JS
- `window.datocms*` in JS globals
- `class="nuxt-*"` or `window.__NUXT__` (Nuxt.js front end)
- Token pattern in JS: `553eb*` or similar DatoCMS bearer token

**Recommended approach:** T3 (GraphQL). Find the read token in page JS source, then
query `allEvents` (or equivalent) directly.

```yaml
tier: 3
graphql:
  endpoint: "https://graphql.datocms.com/"
  token: "<read-only-token-from-page-js>"
  query: |
    {
      allEvents {
        title
        startDate
        endDate
        slug
        description
        photo { url }
        rooms { name }
      }
    }
  event_field: "allEvents"
  timeout_ms: 30000
  url_template: "https://<site-domain>/events/{{.slug}}"
  # field_map:              # Optional — when the query uses non-standard field names
  #   name: "title"         # Logical key: source JSON path (dot-notation for nested)
  #   image: "photo.url"
  #   location: "rooms.0.name"
```

**How to find the token:**
1. View page source, search for `datocms.com` or `Bearer`
2. Or use browser DevTools → Network → filter by `graphql.datocms.com`
3. Copy the `Authorization: Bearer <token>` header value

**Working examples:**
- `tranzac.yaml` — Nuxt.js SPA, DatoCMS GraphQL, 30 events

---

### 11. React / Next.js SPA

**What it is:** React-based single-page apps and Next.js server-rendered sites.
Next.js often pre-renders JSON-LD into HTML (T0 works). Pure React SPAs require T2.

**Detection signals:**
- `<script id="__NEXT_DATA__">` — Next.js SSR data blob
- `[data-reactroot]` attribute or `react-root` / `__next` id
- `class="c-card--event"`, `calendar-performance` — custom card components

**Next.js (T0 preferred):**
- JSON-LD usually present in SSR HTML
- Known issue: some Next.js sites return compressed responses the T0 extractor
  can't decompress → "No events found" despite JSON-LD being present. Use T2.

**React SPA (T2):**
```yaml
tier: 2
headless:
  wait_selector: ".c-card--event"   # wait for React hydration
  wait_timeout_ms: 15000
```

**Working examples:**
- `roy-thomson-massey-hall.yaml` — React SPA, `.c-card--event`, T2
- `toronto-symphony-orchestra.yaml` — React SPA, `.calendar-performance`, T2
- `images-festival.yaml` — Astro/React, `a.group`, T2
- `mod-club.yaml` — Next.js, T0 (compression issue noted)

---

### 12. Vue / Nuxt SPA

**What it is:** Vue.js or Nuxt.js single-page apps. Content is rendered after Vue
hydration. Nuxt with DatoCMS often exposes a GraphQL endpoint (see DatoCMS section).

**Detection signals:**
- `__vue_app__` or `window.__vue_app__` global
- `window.__NUXT__` data blob
- `class="nuxt-*"` in markup

**Recommended tier:** T2 (wait for Vue hydration), or T3 if DatoCMS is the backend.

**Known selectors (Vue SPAs vary widely — always inspect):**
```yaml
event_list: "div.grid-item"
name: "h3.grid-title"
start_date: "h4.date"
url: "a.gray-hover"
```

**Working examples:**
- `mercer-union.yaml` — Vue SPA, `div.grid-item`, T2

---

### 13. Angular SPA

**What it is:** Angular-based SPAs. Less common for event sites but encountered for
larger institutions.

**Detection signals:**
- `ng-version` attribute on `<html>` or `<body>`
- `ng-app` attribute
- Angular-specific attributes like `_nghost-*`, `_ngcontent-*`

**Recommended tier:** T2.

**Working examples:**
- `toronto-holocaust-museum.yaml` — Angular/custom JS, `li.event`, T2

---

### 14. Cloudflare-protected sites

**What it is:** Sites behind Cloudflare's Bot Management or JS challenge will return
a challenge page to the scraper. Detected by empty body or challenge markup.

**Detection signals:**
- HTTP 403 on static fetch
- Body contains `<title>Just a moment...</title>` (Cloudflare challenge)
- Very small response (< 2 KB) with no event content
- `cf-ray` header in response

**Recommended approach:** T2 with `undetected: true`. The stealth plugin patches
`navigator.webdriver` and other signals Cloudflare's JS challenge checks.

```yaml
tier: 2
headless:
  wait_selector: "<target-selector>"
  wait_timeout_ms: 20000
  undetected: true
  # headers:
  #   Accept-Language: "en-CA,en;q=0.9"
```

**If `undetected: true` still fails:** Contact the venue. Bot protection at this
level (Cloudflare Enterprise + Turnstile) is not reliably bypassable.

**Important nuance — ICS endpoints vs HTML pages:** Cloudflare's JS challenge and
Turnstile usually apply to HTML page requests, not ICS feeds. Always probe the feed
URL directly before treating a site as "Cloudflare-blocked". Toronto's canonical
example is `now-toronto.yaml`: `/events/` returned a Turnstile wall, but
`/events/?ical=1` returned a clean `text/calendar` response with plain HTTP GET
(`cf-cache-status: HIT`).

**Known examples:**
- `crows-theatre.yaml` — resolved with T2 `wait_network_idle: true`
- `st-lawrence-market.yaml` — disabled, anti-bot skeleton page
- `ago.yaml` — disabled, HTTP 403
- ~~`now-toronto.yaml`~~ — initially disabled (Turnstile on HTML page); ICS endpoint works fine with plain HTTP GET

---

### 15. Showpass (ticketing platform)

**What it is:** A Canadian ticketing platform. Venues use Showpass for ticket sales;
events are accessible via a public REST API.

**Detection signals:**
- Links to `showpass.com/e/<slug>` or `showpass.com/o/<org-slug>`
- `showpass-widget` class or `data-showpass` attributes
- Embedded `<iframe>` from `widget.showpass.com`

**Recommended approach:** T3 (REST API). Find the venue ID and use:
`GET https://showpass.com/api/public/events/?venue=<venue_id>`

**API shape:** `{ count, next, results: [...] }` — `next` is a URL string or null.
Key event fields: `name`, `starts_on`, `ends_on`, `slug`, `image` (URL string).

```yaml
tier: 3
rest:
  endpoint: "https://www.showpass.com/api/public/events/?venue=<venue_id>"
  results_field: "results"
  next_field: "next"
  url_template: "https://www.showpass.com/{{.slug}}"
  field_map:
    name: "name"
    start_date: "starts_on"
    end_date: "ends_on"
    image: "image"
```

**How to find a venue ID:**
1. Look for `showpass.com` links in the venue's page source (e.g. `showpass.com/e/<slug>`)
2. Visit `https://www.showpass.com/api/public/events/?venue=<id>` with candidate IDs
3. Or search the Showpass organizer page: `showpass.com/o/<org-slug>` — the venue ID
   appears in API requests visible in browser DevTools (Network tab)

**Known venue IDs:**
- Burdock Brewery: `17330` (34 events; 2 paginated pages)

**Working examples:**
- `burdock-brewery.yaml` — T3 REST, Showpass, 34 events

---

### 16. Showclix (ticketing platform)

**What it is:** Ticketing and event management platform used by venues. Uses a custom
calendar widget that loads event data from S3-hosted JSON files.

**Detection signals:**
- `showclix.com` links in page source
- S3 bucket URLs containing `eventsbucket` in network requests (e.g. `*eventsbucket.s3.amazonaws.com/events.json`)
- Custom calendar widget with non-standard DOM structure (day numbers appear as siblings
  to event links rather than children, making date extraction unreliable via CSS)

**Recommended approach:** T3 REST via S3 JSON API. Events are stored as bare JSON arrays
in S3 buckets. No authentication required (public buckets). No pagination needed —
S3 buckets serve complete event lists.

**URL pattern:** `https://<venue>eventsbucket.s3.amazonaws.com/events.json`

**Example:** `https://horseshoeeventsbucket.s3.amazonaws.com/events.json`

**API shape:** Returns `[{...}, {...}]` — a bare JSON array. Use `results_field: "."`.

**Typical API fields:** `name`, `starts_on`, `ends_on`, `slug`, `image` (URL string), `description`

**Why NOT to CSS-scrape:** The calendar widget DOM is fragile — day numbers appear as
siblings to event links rather than children, making date extraction unreliable. The
S3 JSON API is more stable and complete.

```yaml
- name: "Venue Name (Showclix)"
  url: "https://venuename.com/events"       # human-readable URL for logs
  tier: 3
  rest:
    endpoint: "https://<venue>eventsbucket.s3.amazonaws.com/events.json"
    results_field: "."                        # bare JSON array
    field_map:
      name: "name"
      start_date: "starts_on"
      end_date: "ends_on"
      image: "image"
    url_template: "https://www.showclix.com/event/{{.slug}}"
```

**Notes:**
- No pagination needed — S3 buckets serve complete event lists
- No authentication required (public S3 bucket)
- Rate limiting: S3 standard rate limits apply; no special throttling needed

---

### 17. Eventbrite

**What it is:** Global ticketing platform. Venues link to Eventbrite for ticket sales.

**Detection signals:**
- Links to `eventbrite.ca/o/<org-slug>-<org-id>` or `eventbrite.com/e/<id>`
- Eventbrite search widget embed

**API status:** Eventbrite's REST API (`eventbriteapi.com/v3/`) requires OAuth
authentication and is designed for organizers to manage their own events — **not** a
public read API. There is no unauthenticated endpoint for listing an organizer's events.
T3 REST is **not viable** for Eventbrite.

**Recommended approach:** T2 headless scraping of the organizer page
(`eventbrite.ca/o/<org-slug>-<org-id>`) or the venue's own events page if it embeds
Eventbrite widgets. The organizer page is server-rendered and may yield event cards
via CSS selectors. If the organizer page is JS-rendered or behind Cloudflare, use
`undetected: true`.

**Fallback:** If scraping fails, the only reliable path is venue cooperation (ask the
organizer to share an iCal feed or event data export).

**Known organizer IDs:**
- Lula Lounge Toronto: `4108527983`

---

### 18. Elevent (ticketing widget)

**What it is:** A cross-origin iframe-based ticketing widget.

**Detection signals:**
- `<iframe src="*elevent-cdn.azureedge.net*">` in HTML
- Elevent branding on the embedded ticket widget

**Status:** Supported via `headless.iframe:` config block (added in srv-mwy3y). Configure `iframe.selector` to target the Elevent iframe element. The scraper uses CDP frame navigation to enter the iframe's execution context and extract its rendered HTML. CSS selectors in `selectors:` then apply to the iframe DOM, not the parent page.

**Example config:**
```yaml
headless:
  wait_selector: "body"
  wait_timeout_ms: 15000
  iframe:
    selector: "iframe[src*='elevent-cdn.azureedge.net']"
    wait_selector: ".event-list"
    wait_timeout_ms: 10000
```

**Known examples:**
- `reel-asian.yaml` — disabled, Elevent cross-origin iframe (working iframe config; pending manual verification)

---

### 19. Ticket Spot (Wix embed)

**What it is:** A Wix-native event widget embedded as a cross-origin iframe from
`geteventviewer.com` or `ticketspotapp.com`. Venues that use Wix's Ticket Spot app
have their event listings rendered entirely inside the iframe.

**Detection signals:**
- `<iframe src="*geteventviewer.com*">` or `<iframe src="*ticketspotapp.com*">` in HTML
- `<iframe title="Ticket Spot">` title attribute
- CSS class `ticket-spot-iframe` or similar on the iframe wrapper
- `data-app-id` attribute on the iframe with a known Ticket Spot app ID

**API status:** The widget requires a signed Wix JWT to access its internal API.
T3 REST is not viable without venue cooperation.

**Status:** Supported via `headless.iframe:` config block (added in srv-mwy3y). The
scraper uses CDP frame navigation to enter the iframe's execution context and extract
its rendered HTML. CSS selectors in `selectors:` then apply to the iframe DOM, not
the parent page.

**Example config:**
```yaml
headless:
  wait_selector: "body"
  wait_timeout_ms: 15000
  iframe:
    selector: "iframe[title='Ticket Spot']"
    wait_selector: ".events-container"
    wait_timeout_ms: 10000
selectors:
  event_list: "[class^='list-'] > div"
  name: "[class^='title-']"
  start_date: "time[datetime]"
  url: "a[href*='eventbrite']"
```

**CSS Modules note:** Ticket Spot uses CSS Modules which produces class names with
a stable prefix and a rotating hash suffix (e.g. `list-3PgZT`, `title-2yNb5`).
Use attribute prefix selectors (`[class^='title-']`) instead of exact class selectors
(`.title-2yNb5`) to survive hash rotation across deploys. See "CSS Modules / Hashed
Class Names" below for the general pattern.

See `configs/sources/lula-lounge.yaml` for a working example.

**Known app IDs:**
- Lula Lounge: `14409d52-2a79-437b-9b54-3d6a44e8a6ab` (Wix Ticket Spot app ID, visible in iframe `src`)

**Known examples:**
- `lula-lounge.yaml` — working iframe config; disabled pending manual verification

---

### 20. AWS CloudSearch widget

**What it is:** A custom JS widget backed by AWS CloudSearch. The page renders empty
containers; event data arrives via XHR.

**Detection signals:**
- `data-template="TPSPT.*"` on widget container
- `AWSFacetedSearch*` in page JS

**Recommended approach:** T2 with `wait_network_idle: true`. The XHR fires after
DOM ready; network idle wait may allow it to complete.

```yaml
headless:
  wait_selector: "body"
  wait_timeout_ms: 20000
  wait_network_idle: true
```

**If still empty:** Use browser DevTools → Network to capture the AWS XHR endpoint
URL. If unauthenticated, configure as T3 REST.

**Known examples:**
- `rcmusic.yaml` — disabled, AWS CloudSearch, XHR endpoint not found

---

### 21. Agile Technologies box office

**What it is:** A venue ticketing system. Embeds via a `.ashx` URL widget.

**Detection signals:**
- `*boxoffice.*/websales/agile_widget.ashx?orgid=*` in HTML or iframes

**Recommended approach:** T2 with `wait_network_idle: true` + `undetected: true`.
If widget still doesn't render, inspect the `.ashx` endpoint directly — it may
accept `?format=json`.

**Known examples:**
- `hot-docs.yaml` — disabled, Agile Technologies, widget doesn't render in Rod

---

## Headless Flags Reference

Added 2026-03-05 (bead `srv-n8qi1`); `iframe:` block added 2026-03-05 (bead `srv-mwy3y`):

| Flag | Effect | When to use |
|------|--------|-------------|
| `wait_network_idle: true` | After `wait_selector` resolves, waits 500 ms with no in-flight XHR/fetch | Async widgets (eventscalendar.co, AWS CloudSearch, Agile) |
| `undetected: true` | Launches via go-rod/stealth (patches `navigator.webdriver`, fake plugins) | Cloudflare JS challenge, bot-detection widgets |
| `iframe.selector` | CSS selector for the target cross-origin iframe element | Ticket Spot, Elevent, and other cross-origin iframe embeds |
| `iframe.wait_selector` | CSS selector to wait for inside the iframe DOM before extracting | Any iframe target — waits for content to render inside the frame |
| `iframe.wait_timeout_ms` | Timeout (ms) for `iframe.wait_selector` (default: 10000) | Increase for slow-loading iframe content |

**`iframe:` block config example:**
```yaml
headless:
  wait_selector: "body"
  wait_timeout_ms: 15000
  iframe:
    selector: "iframe[title='Ticket Spot']"   # CSS selector for the iframe element
    wait_selector: ".events-container"          # wait for content inside iframe
    wait_timeout_ms: 10000                      # timeout for iframe content
```

When `iframe:` is configured, the scraper uses Chrome DevTools Protocol (CDP) frame
navigation to enter the iframe's execution context and extract its fully rendered HTML.
CSS selectors in `selectors:` apply to the iframe DOM, not the parent page.

**Test sequence for unknown/blocked sources:**
1. Try T2 with `wait_network_idle: true` + `wait_timeout_ms: 20000`
2. If still empty, add `undetected: true`
3. If content is in a cross-origin iframe, add an `iframe:` block with the iframe selector
4. Confirm DOM content via `server scrape capture <URL> --format inspect`
5. If both flags and iframe block fail, fall back to API/contact approach

**`--source-file` flag (test without DB):**
```bash
SCRAPER_HEADLESS_ENABLED=true server scrape source \
  --source-file /tmp/draft.yaml --dry-run
```

---

## Cross-Origin Iframe Support

Cross-origin iframe extraction is supported via the `headless.iframe:` config block
(implemented in bead `srv-mwy3y`).

**How it works:** When `iframe:` is configured, the scraper uses Chrome DevTools Protocol
(CDP) frame navigation to enter the iframe's execution context. Rather than trying to
reach iframe content from the parent page (which the same-origin policy blocks), the
scraper navigates into the frame's document directly, waits for the specified selector,
and extracts the fully rendered iframe HTML. CSS selectors in `selectors:` then apply
to the iframe DOM.

**Supported platforms:**
- **Ticket Spot** (Wix embed from `geteventviewer.com` / `ticketspotapp.com`) — use `iframe[title='Ticket Spot']`
- **Elevent** (`elevent-cdn.azureedge.net`) — use `iframe[src*='elevent-cdn.azureedge.net']`

**Important:** The config `url` must be the venue's own page — not the iframe `src`.
Iframe targeting requires explicit configuration; there is no auto-detection.

**Config example:**
```yaml
headless:
  wait_selector: "body"
  wait_timeout_ms: 15000
  iframe:
    selector: "iframe[title='Ticket Spot']"   # CSS selector targeting the iframe
    wait_selector: ".events-container"          # selector to wait for inside the iframe
    wait_timeout_ms: 10000
selectors:
  event_list: "[class^='list-'] > div"   # prefix selector survives CSS Modules hash rotation
  name: "[class^='title-']"
  start_date: "time[datetime]"
  url: "a[href*='eventbrite']"
```

---

## CSS Modules / Hashed Class Names

Some platforms (notably Ticket Spot, many React/Vue/Svelte apps) use **CSS Modules**
or similar tooling that appends a hash suffix to class names at build time. The pattern
is `<human-name>-<hash>`, e.g.:

```
list-3PgZT      →  prefix "list-"
title-2yNb5     →  prefix "title-"
details-JMKf7   →  prefix "details-"
container-a1B2c →  prefix "container-"
```

The prefix is derived from the original class name in source code and is **stable
across deploys**. The hash suffix rotates whenever the CSS is rebuilt.

### How to detect

- Multiple class names on the same page following the `word-xxxxx` pattern (5-character
  alphanumeric suffix is the most common, but length varies)
- Class names that look semantic but have a random tail
- Typically seen in React (CSS Modules, styled-components), Vue (scoped styles),
  Svelte, and build tools like Vite/Webpack

### Selector strategy

**Always use attribute prefix selectors** instead of exact class selectors:

| Fragile (breaks on redeploy) | Resilient (survives hash rotation) |
|---|---|
| `.list-3PgZT` | `[class^='list-']` |
| `.title-2yNb5` | `[class^='title-']` |
| `.details-JMKf7 > a` | `[class^='details-'] > a` |

CSS attribute selectors:
- `[class^='prefix-']` — class attribute **starts with** prefix (use when the
  element has only one class)
- `[class*='prefix-']` — class attribute **contains** prefix (use when the element
  has multiple classes and the target isn't first)

**Prefer `^=` (starts-with)** when possible — it's more specific and avoids false
matches. Fall back to `*=` (contains) only when the hashed class isn't the first
class on the element.

### Caveats

- If the platform renames the source-level class (e.g. `title` → `heading`), the
  prefix changes too. This is rare but possible on major redesigns.
- Very short prefixes (e.g. `a-`, `b-`) may cause false matches. Prefer the most
  specific prefix available.
- Verify with `server scrape capture <URL> --format inspect` after writing selectors.

## ICS Feed Discovery

Many event platforms expose calendar data as ICS (iCalendar) feeds. When a valid ICS
feed is available, `extraction_method: ics` bypasses the T0–T3 tier dispatch entirely
and parses the feed directly. This section documents the detection heuristics and URL
patterns for common platforms.

For operational setup, sync, warnings, and recurrence troubleshooting, see
[ics-feeds.md](ics-feeds.md).

### Toronto rollout observations

- `now-toronto.yaml` is the best Cloudflare example: the HTML events page is blocked,
  but `https://nowtoronto.com/events/?ical=1` serves ICS directly.
- Public WordPress Tribe sites often expose `?ical=1`; verify with
  `curl -H "Accept: text/calendar" "<events-url>?ical=1"` before falling back to HTML
  scraping.
- If the response body is HTML instead of `BEGIN:VCALENDAR`, the URL is usually a
  landing page, login page, or challenge wall. Use the HTML scraper path or ask the
  venue for the actual feed URL.

### WordPress + The Events Calendar (Tribe)

**Detection signal**: The Tribe plugin adds a "Subscribe to calendar" link that appends
`?ical=1` to the events page URL. Also look for `class="tribe-events*"` elements or
`/wp-json/tribe/events` API endpoints in HTML source (cross-reference with Platform
Profile #2 in this document).

**ICS URL pattern**: `https://example.com/events/?ical=1`
(append `?ical=1` — or `&ical=1` if query params already exist — to the calendar page URL)

**Verification step**: `curl -H "Accept: text/calendar" "https://example.com/events/?ical=1"`
— should return `text/calendar` content starting with `BEGIN:VCALENDAR`.

**Fallback**: When the ICS feed is incomplete or behind auth, use T0 JSON-LD (Tribe
injects JSON-LD by default) or T1/T2 CSS selectors as documented in Platform Profile #2.

**Config key**: `extraction_method: ics` — see [scraper.md](scraper.md) for full config details.

### WordPress MEC (Modern Events Calendar)

**Detection signal**: MEC adds a calendar export button on event pages. Look for
`class="mec-*"` elements or `/wp-json/mec/` API endpoints in HTML source.

**ICS URL pattern**: `https://example.com/events/?mec-ical-feed=1`

**Verification step**: `curl -H "Accept: text/calendar" "https://example.com/events/?mec-ical-feed=1"`
— should return `text/calendar` content starting with `BEGIN:VCALENDAR`.

**Fallback**: MEC sometimes provides JSON-LD. Use T0 JSON-LD or T1 CSS selectors as
fallback when the ICS feed is unavailable.

**Config key**: `extraction_method: ics`

### WordPress Events Manager

**Detection signal**: Events Manager plugin adds `/feed/` endpoints. Look for
`class="em-*"` elements or `/?ical=1` links in HTML source.

**ICS URL pattern**: `https://example.com/?ical=1` or
`https://example.com/events/feed/?ical=1`

**Verification step**: `curl -H "Accept: text/calendar" "https://example.com/?ical=1"`
— should return `text/calendar` content starting with `BEGIN:VCALENDAR`.

**Fallback**: Use T0 JSON-LD or T1 CSS selectors when ICS is unavailable.

**Config key**: `extraction_method: ics`

### Tockify

**Detection signal**: Tockify embeds a calendar widget via `<script src="*.tockify.com*>`.
The calendar name appears in the embed URL or `data-tockify-calendar` attribute.

**ICS URL pattern**: `https://tockify.com/api/feeds/ics/<calendar-name>`
(replace `<calendar-name>` with the value from the embed attribute)

**Verification step**: `curl -H "Accept: text/calendar" "https://tockify.com/api/feeds/ics/<calendar-name>"`
— should return a valid ICS feed.

**Fallback**: Use T2 headless scraping of the Tockify embed page when the API feed is
unavailable or auth-protected.

**Config key**: `extraction_method: ics`

### Google Calendar

**Detection signal**: Embedded Google Calendar iframes (`calendar.google.com/calendar/embed`)
or links to `calendar.google.com` in page source. The calendar ID is typically the
email address of the calendar owner.

**ICS URL pattern**: `https://calendar.google.com/calendar/ical/<id>/public/basic.ics`
(replace `<id>` with the calendar ID, URL-encoding any `@` symbols as `%40`)

**Verification step**: `curl -H "Accept: text/calendar" "https://calendar.google.com/calendar/ical/<id>/public/basic.ics"`
— should return `text/calendar` content. Calendars must be set to "Make available to
public" for the ICS feed to work.

**Fallback**: Public Google Calendars can also be scraped via T2 headless rendering of
the embedded iframe, but ICS is strongly preferred when available.

**Config key**: `extraction_method: ics`

### Meetup

**Detection signal**: Meetup event pages at `meetup.com/<group>/events/`. Meetup
provides per-group ICS feeds linked from the group's calendar page.

**ICS URL pattern**: `https://www.meetup.com/<group>/events/ical/`
(replace `<group>` with the URL slug of the Meetup group)

**Verification step**: `curl -H "Accept: text/calendar" "https://www.meetup.com/<group>/events/ical/"`
— should return `text/calendar` content with upcoming events.

**Critical gap (discovered Cohort 1, 2026-04-17):** Meetup ICS feeds **omit the `LOCATION`
property** for all events, including outdoor and in-person groups that clearly have physical
venues. The SEL ingest API requires either `location` or `virtualLocation` — without either,
every event in the feed is rejected with `invalid location: location or virtualLocation required`.
All 19 Meetup groups in Cohort 1 failed for this reason.

**Root cause**: Meetup has been reducing ICS field coverage since ~2023. The `URL` field
pointing to the meetup event page IS present in the feed but is not mapped to
`virtualLocation` by the ICS mapper (tracked as `srv-ia1w3`).

**Remediation options** (choose based on group type):

1. **`default_location` in config** (physical-venue groups): Works for groups that always
   meet at the same trailhead, coworking space, or venue. Add `default_location:` block to
   the YAML config. Not viable for groups with rotating or online-only venues.

2. **ICS mapper `URL → virtualLocation` fix** (online groups): When bead `srv-ia1w3` is
   merged, the mapper will use the Meetup event URL as `virtualLocation` for events lacking
   `LOCATION`. This will unblock all 19 Meetup groups automatically.

3. **Meetup API** (preferred if access is available): Meetup's GraphQL API returns
   structured location data that the ICS feed omits. Requires venue or group admin
   cooperation.

**Slug verification**: Canonical slugs for all Toronto Meetup groups are in
`specs/005-ics-integration/toronto-ics-manifest.json`. If a group URL returns 404,
verify the slug in `community-calendar`'s `feeds.txt` (see `ics-feeds.md § 8`).

**Fallback**: Meetup pages have JSON-LD but it can be sparse. Use T1/T2 selectors as
supplement when ICS is incomplete.

**Config key**: `extraction_method: ics`

### Eventbrite

**Detection signal**: Eventbrite event pages (`eventbrite.com/e/` URLs). Eventbrite
does **not** expose a native ICS feed for organizers or events.

**ICS URL pattern**: No native ICS feed available.

**Verification step**: N/A — Eventbrite does not provide machine-readable calendar
feeds without authentication (their API requires an OAuth token).

**Fallback**: Use T1/T2 HTML scraping for Eventbrite organizer pages, or use third-party
calendar export sites that can convert Eventbrite listings to ICS. When ICS is not an
option, T1/T2 selectors on the Eventbrite organizer page are the primary approach.

**Config key**: N/A — Eventbrite sources should use T1/T2 selectors, not
`extraction_method: ics`.

### Static HTML Link

**Detection signal**: Some sites include a `<link rel="alternate" type="text/calendar">`
tag in the `<head>` of their HTML. This is the standard way to advertise an ICS feed.
Inspect the page source and look for:
```html
<link rel="alternate" type="text/calendar" href="/events.ics" title="Events">
```
The `href` attribute contains the ICS URL. It may be relative (resolve against the
page's base URL) or absolute.

**ICS URL pattern**: The value of the `href` attribute on the `<link>` element. Use
the full resolved URL.

**Verification step**: `curl -H "Accept: text/calendar" "<resolved-href-url>"`
— should return `text/calendar` content starting with `BEGIN:VCALENDAR`.

**Fallback**: If the ICS feed is incomplete or stale, use T0 JSON-LD or T1/T2 selectors
on the site's HTML event listing.

**Config key**: `extraction_method: ics`

### ICS Mode Activation

To enable ICS extraction for a source, set `extraction_method: ics` in the YAML config.
This bypasses the T0–T3 tier dispatch entirely — the scraper fetches the URL as an ICS
feed and uses the `ical` package for parsing and mapping. Tier and selector fields are
ignored when `extraction_method: ics` is set. See [scraper.md](scraper.md) for full
configuration details.

After editing YAML configs, run `server scrape sync` to upsert changes into the
`scraper_sources` database table. The scraper reads from the database at runtime, not
from YAML files — syncing is required for config changes to take effect.

### ICS vs Tier-Based Extraction Decision Guide

| Condition | Recommendation |
|---|---|
| ICS feed exists and returns ≥ 2 events | Use `extraction_method: ics` |
| ICS feed exists but incomplete or stale | Use T1/T2 selectors as primary, ICS as supplement |
| ICS feed is behind authentication | Use T1/T2 or T3 headless scrape |
| No ICS feed detected | Use T0 JSON-LD or T1/T2 CSS selectors |
