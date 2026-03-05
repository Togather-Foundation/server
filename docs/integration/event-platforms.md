# Event Platform Knowledge Base

**Purpose:** Agent-readable reference for the `/generate-selectors` command. Before
inspecting an unknown site's DOM, check the recognition cheatsheet below to identify
the platform. If a match is found, use the known selectors and headless flags as a
starting point rather than deriving everything from scratch.

**Last updated:** 2026-03-05

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
| `elevent-cdn.azureedge.net` in source/network | **Elevent widget** | blocked (cross-origin iframe) |
| `.ashx?orgid=` URL pattern | **Agile Technologies box office** | T2 (`wait_network_idle: true`) |
| Cloudflare challenge page body (`<title>Just a moment...</title>`) | **Cloudflare-protected** | T2 (`undetected: true`) |
| `showpass.com` link or `showpass-widget` | **Showpass** | T3 (REST API) |
| `eventbrite.com/o/<org-id>` link pattern | **Eventbrite** | T3 (REST API) |
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

**Known examples:**
- `crows-theatre.yaml` — disabled, Cloudflare JS challenge suspected
- `st-lawrence-market.yaml` — disabled, anti-bot skeleton page
- `ago.yaml` — disabled, HTTP 403

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

```yaml
tier: 3
# (Tier 3 REST adapter pending implementation)
```

**Known venue IDs:**
- Burdock Brewery: `17330` (legacy/parallel integration, verify)

---

### 16. Eventbrite

**What it is:** Global ticketing platform. Organizer event lists are accessible via
the Eventbrite public API.

**Detection signals:**
- Links to `eventbrite.ca/o/<org-slug>-<org-id>` or `eventbrite.com/e/<id>`
- Eventbrite search widget embed

**Recommended approach:** T3 (REST API).
`GET https://www.eventbriteapi.com/v3/organizers/<org_id>/events/`

**Known organizer IDs:**
- Lula Lounge Toronto: `4108527983`

---

### 17. Elevent (ticketing widget)

**What it is:** A cross-origin iframe-based ticketing widget.

**Detection signals:**
- `<iframe src="*elevent-cdn.azureedge.net*">` in HTML
- Elevent branding on the embedded ticket widget

**Status:** Blocked. The widget renders inside a cross-origin iframe; CSS selectors
cannot reach the iframe content from the parent page.

**Only viable path:** Elevent venue partner API (requires coordination with venue).

**Known examples:**
- `reel-asian.yaml` — disabled, Elevent cross-origin iframe

---

### 18. AWS CloudSearch widget

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

### 19. Agile Technologies box office

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

Added 2026-03-05 (bead `srv-n8qi1`):

| Flag | Effect | When to use |
|------|--------|-------------|
| `wait_network_idle: true` | After `wait_selector` resolves, waits 500 ms with no in-flight XHR/fetch | Async widgets (eventscalendar.co, AWS CloudSearch, Agile) |
| `undetected: true` | Launches via go-rod/stealth (patches `navigator.webdriver`, fake plugins) | Cloudflare JS challenge, bot-detection widgets |

**Test sequence for unknown/blocked sources:**
1. Try T2 with `wait_network_idle: true` + `wait_timeout_ms: 20000`
2. If still empty, add `undetected: true`
3. Confirm DOM content via `server scrape capture <URL> --format inspect`
4. If both flags fail, fall back to API/contact approach

**`--source-file` flag (test without DB):**
```bash
SCRAPER_HEADLESS_ENABLED=true server scrape source \
  --source-file /tmp/draft.yaml --dry-run
```
