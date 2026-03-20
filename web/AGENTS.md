# Web / Admin UI

## Constraints

**CSP: `script-src 'self'` — no inline scripts or event handlers.**

```html
<!-- BAD: CSP violation -->
<button onclick="doSomething()">Click</button>

<!-- GOOD: data attribute + event delegation in external .js -->
<button data-action="do-something">Click</button>
```

**Embedded assets:** JS/HTML/CSS are embedded in the Go binary. After changing static files, rebuild: `make stop && make build && make run` (or use `make dev` with air for auto-reload).

## Generated files

**`robots.txt` and `sitemap.xml` are generated at deploy time** by `cmd/server/cmd/webfiles.go` — do NOT edit `web/robots.txt` or `web/sitemap.xml` directly; changes will be wiped on the next deploy.

To make permanent changes:
1. Edit `generateRobotsTxt()` or `generateSitemapXML()` in `cmd/server/cmd/webfiles.go`.
2. Regenerate and rebuild: `server webfiles --domain <domain> --output ./web && make build`.

**Committed copies use `localhost:8080`** as a placeholder domain — this is expected and correct for local dev.

**`robots.txt.template` / `sitemap.xml.template`** in `web/` are reference files only; the generator does NOT read them. Changes there have no effect.

**`llms.txt`** (`web/llms.txt`) is a static file — edit it directly (unlike robots.txt/sitemap.xml). Tool names must match those registered in `internal/mcp/server.go`.

**Vendor files — never edit:** `tabler.min.css`, `tabler.min.js`, `bootstrap.bundle.min.js`

## API Client

Always use `api.js` — never call `fetch()` directly (bypasses JWT auth and error handling):

```javascript
const events = await API.events.list({ limit: 100 });
```

## Public API Response Shapes (JSON-LD footguns)

The public API returns JSON-LD. Field names differ from what you might expect:

| Wrong assumption | Actual field |
|---|---|
| `entity.ulid` | Extract from `entity['@id']` URI |
| `entity.city` | `entity.address.addressLocality` |
| `entity.status` / `entity.lifecycle` | Not exposed on public API |

**ULID extraction pattern:**
```javascript
function extractId(entity, pathSegment) {
    if (entity?.['@id']) {
        const m = entity['@id'].match(new RegExp(pathSegment + '/([A-Z0-9]{26})', 'i'));
        if (m) return m[1];
    }
    return entity?.id ?? entity?.ulid ?? null;
}
```
Reference: `events.js:156-172` (`extractEventId`).

**Public vs admin API:** `API.places.list()` → `/api/v1/places` (JSON-LD, no auth). `API.places.similar()` → `/api/v1/admin/places/{id}/similar` (flat JSON, JWT required). Response shapes differ.

## Authentication

- Credentials in `.env` — never hardcode. Use `ADMIN_PASSWORD`, not `admin`.
- Token stored in `localStorage` AND `auth_token` HttpOnly cookie.
- Logout: POST `/api/v1/admin/logout`, clear localStorage, redirect to login.

### SSE / EventSource

`EventSource` cannot set custom headers, so SSE endpoints **must** use `adminCookieAuth`
(not Bearer JWT). The `auth_token` HttpOnly cookie is sent automatically for same-origin
requests — no JS-side auth plumbing is needed.

```javascript
// Correct: cookie auth is automatic for same-origin EventSource
const es = new EventSource('/api/v1/admin/scraper/events');
es.addEventListener('job_update', e => { /* ... */ });

// Do NOT use API.* or fetch() for SSE — use EventSource directly
```

Reconnect with exponential back-off; close the `EventSource` when the component is
torn down to avoid leaked connections. Use `pagehide` (not `beforeunload`) to close
`EventSource` on navigation; register the handler once at module init level (not inside
the connect function) to avoid accumulation.

## Shared Utilities (`components.js`)

```javascript
showToast('Message', 'success');
confirmAction('Title', 'Msg', fn);
escapeHtml(userInput);          // required before innerHTML
setLoading(button, true/false);
debounce(fn, 300);
```

## Shared Frontend Modules

Beyond `components.js`, these modules are loaded globally via `_footer.html`:

- `field-picker.js` — `window.FieldPicker`: chip-based field comparison table for consolidation/review workflows
- `occurrence-rendering.js` — `window.OccurrenceRendering`: occurrence list HTML rendering (used by review-queue fold-down)
- `warning-badges.js` — `window.WarningBadges`: warning badge HTML generation (used by review-queue table + detail cards)

These are only functionally used by `review-queue.js` and `consolidate.js` but are loaded on all admin pages via `_footer.html` for simplicity.

## JS Module Design Rules

When writing a new shared module or extracting logic into one:

**Design param-first.** Functions must receive everything they need as explicit parameters — no closing over page-specific DOM IDs, module-level state, or globals beyond the utilities in `components.js`. This keeps modules context-agnostic and reusable across pages.

```javascript
// BAD: closes over page-specific DOM and module state
function renderTable() {
    const container = document.getElementById('my-specific-table');
    container.innerHTML = buildRows(loadedEvents);   // loadedEvents is a closure var
}

// GOOD: explicit params — works on any page, testable in isolation
function renderTable(containerEl, events) {
    if (!containerEl) return;
    containerEl.innerHTML = buildRows(events);
}
```

**Guard optional globals.** If a shared module is loaded via `_footer.html` but only used by some pages, guard calls with a `typeof` check so a missing or failed load doesn't crash the whole page IIFE:

```javascript
if (typeof FieldPicker !== 'undefined') {
    FieldPicker.renderFieldPickerTable(container, events);
}
```

**IIFE + `window.*` namespace.** All shared modules follow this structure:

```javascript
(function () {
    'use strict';
    function foo(param) { ... }
    function bar(param) { ... }
    window.MyModule = { foo, bar };
})();
```

**No internal cross-namespace calls.** Inside the IIFE, call sibling functions by their local name (`foo()`), not by the namespace (`MyModule.foo()`). The namespace is only for external callers.

## Template Partials

- `_footer.html` — shared scripts, toast container, confirm modal; included by all pages
- `_header.html` — navigation bar; included by all pages
- `_modals.html` — page-specific modals (reject, merge, delete event); included by `review_queue.html` and `events_list.html`
- `_user_modal.html` — user management modal; included via `_footer.html`

## Testing

```bash
make e2e   # requires running server + uvx; catches CSP violations, console errors, auth flows
```

Rebuild before running E2E if static assets changed.

## XSS

Use `escapeHtml()` or `element.textContent` — never insert raw user input via `innerHTML`.
