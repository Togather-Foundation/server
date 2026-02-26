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

## Shared Utilities (`components.js`)

```javascript
showToast('Message', 'success');
confirmAction('Title', 'Msg', fn);
escapeHtml(userInput);          // required before innerHTML
setLoading(button, true/false);
debounce(fn, 300);
```

## Testing

```bash
make e2e   # requires running server + uvx; catches CSP violations, console errors, auth flows
```

Rebuild before running E2E if static assets changed.

## XSS

Use `escapeHtml()` or `element.textContent` — never insert raw user input via `innerHTML`.
