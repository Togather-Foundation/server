# Web Development Guidelines for AI Agents

Best practices for working with HTML, CSS, and JavaScript in the `/web/` directory.

## Core Principles

- **DRY** - Use template partials (`_header.html`, `_footer.html`), `components.js` for shared JS, `custom.css` for shared styles
- **KISS** - Vanilla JavaScript, progressive enhancement, minimal dependencies, Fetch API over axios
- **CSP compliance** - `script-src 'self'` only; NO inline scripts or event handlers (see [Security](#security))

## Directory Structure

```
web/
├── AGENTS.md                         # This file
├── README.md                         # Human-facing web docs
├── index.html                        # Public landing page
├── static.go                         # Go embed for static files
├── landing.go                        # Landing page handler
├── landing_test.go                   # Landing page tests
├── api_docs.go                       # API docs handler
├── api_docs_test.go                  # API docs tests
├── robots.txt                        # Generated SEO file
├── robots.txt.template               # Template for robots.txt
├── sitemap.xml                       # Generated SEO file
├── sitemap.xml.template              # Template for sitemap.xml
│
├── admin/
│   ├── docs/
│   │   └── review-queue.md           # Review queue feature docs
│   │
│   ├── static/
│   │   ├── test-retry.html           # Test retry page
│   │   │
│   │   ├── css/
│   │   │   ├── tabler.min.css        # Tabler UI framework (vendor, DO NOT EDIT)
│   │   │   ├── custom.css            # Project-wide admin customizations
│   │   │   └── admin.css             # Login page styles
│   │   │
│   │   ├── icons/                    # Favicons, social preview, webmanifest
│   │   │   ├── android-chrome-*.png  # Android icons (192, 512)
│   │   │   ├── apple-touch-icon.png
│   │   │   ├── favicon-*.png         # Favicons (16, 32)
│   │   │   ├── site.webmanifest
│   │   │   └── togather_social_preview_v1.jpg
│   │   │
│   │   └── js/
│   │       ├── tabler.min.js           # Tabler framework (vendor, DO NOT EDIT)
│   │       ├── bootstrap.bundle.min.js # Bootstrap JS (vendor, DO NOT EDIT)
│   │       ├── api.js                  # Centralized API client (JWT auth)
│   │       ├── components.js           # Reusable UI (toasts, modals, logout, dates)
│   │       ├── dashboard.js            # Dashboard page
│   │       ├── events.js               # Events list page
│   │       ├── event-edit.js           # Event editor page
│   │       ├── duplicates.js           # Duplicate review page
│   │       ├── places.js               # Places management page (list, search, merge)
│   │       ├── organizations.js        # Organizations management page (list, search, merge)
│   │       ├── api-keys.js             # API key management page
│   │       ├── review-queue.js         # Review queue page
│   │       ├── federation.js           # Federation page
│   │       ├── users.js                # User management page
│   │       ├── user-activity.js        # User activity page
│   │       ├── accept-invitation.js    # Invitation acceptance page
│   │       └── login.js                # Login form
│   │
│   └── templates/
│       ├── README.md                   # Template conventions
│       ├── _header.html                # Shared nav header (partial)
│       ├── _footer.html                # Shared footer with scripts (partial)
│       ├── _head_meta.html             # Shared <head> meta tags (partial)
│       ├── _user_modal.html            # Shared user detail modal (partial)
│       ├── login.html                  # Login (no header/footer)
│       ├── dashboard.html              # Admin dashboard
│       ├── events_list.html            # Events management
│       ├── event_edit.html             # Event edit form
│       ├── duplicates.html             # Duplicate review
│       ├── places_list.html            # Places management (list, search, merge)
│       ├── organizations_list.html     # Organizations management (list, search, merge)
│       ├── review_queue.html           # Review queue
│       ├── api_keys.html               # API key management
│       ├── federation.html             # Federation management
│       ├── users_list.html             # User management
│       ├── user_activity.html          # User activity log
│       └── accept_invitation.html      # Invitation acceptance
│
├── api-docs/
│   └── dist/
│       ├── index.html                  # Scalar API docs viewer
│       └── scalar-standalone.js        # Scalar JS (vendor, 3.5MB)
│
└── email/
    └── templates/
        └── invitation.html             # Email invitation template
```

## HTML Templates

### Template Organization

**Shared partials (included via `{{ template "_header.html" . }}`):**
- `_header.html` — Navigation menu, user dropdown, logout
- `_footer.html` — Common modals, toast container, shared JS includes
- `_head_meta.html` — Meta tags, favicons, Open Graph
- `_user_modal.html` — User detail/edit modal (used by users and activity pages)

**Page handler data contract:**
```go
data := map[string]interface{}{
    "Title":      "Page Title - SEL Admin",
    "ActivePage": "dashboard", // For nav highlighting
    // Page-specific data...
}
```

### HTML Best Practices

- Semantic HTML (`<nav>`, `<main>`, `<article>`, `<section>`)
- Accessibility: ARIA labels, alt text, keyboard navigation
- Mobile-first: Tabler responsive grid (`col-sm-*`, `col-lg-*`)
- IDs for JavaScript targets, classes for styling

## CSS Guidelines

**Framework:** Tabler v1.4.0 (Bootstrap 5-based)

- Use Tabler utility classes first; override in `custom.css` only when needed
- Never modify vendor files (`tabler.min.css`)
- No inline styles — use classes
- Mobile-first; BEM naming for custom components
- Avoid `!important`; keep specificity low
- Match theme classes: `navbar-light` for light backgrounds, `navbar-dark` for dark

## JavaScript Architecture

### File Loading Order

**Core files (loaded on every admin page via `_footer.html`):**
1. `tabler.min.js` — UI framework
2. `bootstrap.bundle.min.js` — Bootstrap JS (modals, tooltips)
3. `api.js` — Centralized HTTP client with JWT auth
4. `components.js` — Shared utilities (toasts, modals, logout, dates)

**Page-specific files** — loaded per page after core files.

### API Client (`api.js`)

Single source for all API calls. Handles JWT tokens from localStorage automatically.

```javascript
// Always use the API client
const events = await API.events.list({ limit: 100 });

// Never use direct fetch (bypasses auth, error handling)
```

**Available API sections:**
- `API.events.*` — list, get, update, delete, retry, bulkRetry, stats
- `API.places.*` — list, get, similar (admin), merge (admin), delete (admin)
- `API.organizations.*` — list, get, similar (admin), merge (admin), delete (admin)
- `API.duplicates.*` — list
- `API.federation.*` — list, get, create, update, delete, sync, status
- `API.users.*` — list, get, create, update, delete, activity
- `API.apiKeys.*` — list, create, revoke
- `API.reviewQueue.*` — list, approve, reject, bulkApprove

Note: `places.list()` and `organizations.list()` hit the **public** API (`/api/v1/places`, `/api/v1/organizations`), while `similar()` and `merge()` hit **admin** endpoints (`/api/v1/admin/...`) that require JWT auth.

### Public API Response Shapes (JSON-LD)

**Critical:** The public API returns JSON-LD objects, NOT flat objects. Field names differ from what you might expect. Always verify response shapes against the actual API before writing JS that reads them.

**Events** (`GET /api/v1/events`):
```json
{
  "@context": "...",
  "@id": "https://host/api/v1/events/01ABCDEF...",
  "@type": "Event",
  "name": "Event Name",
  "startDate": "2026-03-15T19:00:00-04:00",
  "location": { "@type": "Place", "name": "Venue", ... },
  "organizer": { "@type": "Organization", "name": "Org", ... }
}
```

**Places** (`GET /api/v1/places`):
```json
{
  "@id": "https://host/places/01ABCDEF...",
  "@type": "Place",
  "name": "Venue Name",
  "address": {
    "@type": "PostalAddress",
    "streetAddress": "123 Main St",
    "addressLocality": "Toronto",
    "addressRegion": "ON",
    "postalCode": "M5V 1A1",
    "addressCountry": "CA"
  },
  "geo": { "latitude": 43.65, "longitude": -79.38 }
}
```

**Organizations** (`GET /api/v1/organizations`):
```json
{
  "@id": "https://host/organizations/01ABCDEF...",
  "@type": "Organization",
  "name": "Org Name",
  "address": {
    "@type": "PostalAddress",
    "addressLocality": "Toronto",
    "addressRegion": "ON"
  }
}
```

**Key gotchas:**
- **No `ulid` field** — The ULID is embedded in the `@id` URI. Extract it with a regex (see [ULID Extraction](#ulid-extraction-from-id)).
- **No `city`/`region` fields** — Location is nested under `address.addressLocality` / `address.addressRegion`.
- **No `lifecycle`/`status` field** — The public API does not expose lifecycle state. Only non-deleted items are returned.
- **`@id` is a full URI**, not a relative path — e.g., `https://staging.toronto.togather.foundation/places/01KHP...`

### ULID Extraction from `@id`

All entities in the public API use `@id` (a full URI) instead of a bare `ulid` field. Use this pattern to extract the ULID:

```javascript
/**
 * Extract ULID from an entity's @id URI.
 * @param {Object} entity - API response object with @id field
 * @param {string} pathSegment - URL path segment (e.g., 'events', 'places', 'organizations')
 * @returns {string|null} 26-character ULID or null
 */
function extractId(entity, pathSegment) {
    if (!entity) return null;
    if (entity['@id']) {
        var match = entity['@id'].match(new RegExp(pathSegment + '/([A-Z0-9]{26})', 'i'));
        if (match) return match[1];
    }
    // Fallbacks for admin API responses that may use flat fields
    if (entity.id) return entity.id;
    if (entity.ulid) return entity.ulid;
    return null;
}
```

See `events.js:156-172` for the reference implementation (`extractEventId`).

### Shared Utilities (`components.js`)

```javascript
showToast('Message', 'success');        // Toast notifications
confirmAction('Title', 'Msg', fn);      // Confirmation dialogs
escapeHtml(userInput);                  // XSS protection
formatDate('2026-03-15T19:00:00Z');     // Date formatting
setLoading(button, true/false);         // Button loading states
debounce(fn, 300);                      // Search debouncing
setupLogout();                          // Auto-wired on all pages
```

### Standard Page Pattern

```javascript
(function() {
    'use strict';
    document.addEventListener('DOMContentLoaded', init);

    function init() {
        setupEventHandlers();
        loadData();
    }

    async function loadData() {
        try {
            const data = await API.events.list({ limit: 100 });
            renderEvents(data.items || []);
        } catch (err) {
            console.error('Failed to load:', err);
            showToast('Failed to load events', 'error');
        }
    }
})();
```

### Authentication Flow

1. POST `/api/v1/admin/login` with credentials
2. Response: `{ token: "...", user: {...} }`
3. Token stored in `localStorage` (for JS API calls) AND `auth_token` HttpOnly cookie (for HTML page auth)
4. API calls: `Authorization: Bearer <token>` header via `api.js`
5. HTML pages: cookie auth via `AdminAuthCookie` middleware
6. Logout: POST `/api/v1/admin/logout`, clear localStorage, redirect to login

**Admin credentials are in `.env`** — never hardcode. Common mistake: using `admin/admin` instead of the actual `ADMIN_PASSWORD` from `.env`.

## Security

### CSP Compliance (`script-src 'self'`)

**NEVER use inline scripts or event handlers:**
```html
<!-- BAD: CSP violation -->
<script>console.log('blocked')</script>
<button onclick="doSomething()">Click</button>

<!-- GOOD: External script + event delegation -->
<script src="/admin/static/js/my-script.js"></script>
<button data-action="do-something">Click</button>
```

**Event delegation pattern:**
```javascript
document.addEventListener('click', (e) => {
    const target = e.target.closest('[data-action]');
    if (!target) return;
    if (target.dataset.action === 'do-something') doSomething();
});
```

**Verify CSP compliance:**
```bash
grep -rn "onclick=\|onerror=\|onload=\|onchange=\|onsubmit=" web/admin/templates/
grep -rn "<script[^>]*>[^<]" web/admin/templates/
```

### XSS Prevention

- Use `escapeHtml()` from `components.js` for innerHTML
- Prefer `element.textContent` (auto-escapes) over innerHTML
- Never insert raw user input into DOM

## Development

### Server

```bash
make dev              # Live reload with air (recommended)
make build && make run  # Build and run manually
```

**Embedded assets:** JS/HTML/CSS are embedded in the Go binary. Changes require rebuild unless using `air` (which auto-rebuilds on `.go` file changes). For static asset changes without `air`, run `make stop && make build && make run`.

### Multi-Agent Coordination

Only ONE agent should start the server. Others should check first:
```bash
lsof -ti:8080 > /dev/null 2>&1 && echo "Server running" || make dev &
```

## Testing

### E2E Tests (Primary Verification)

**Always run after UI changes.** See `tests/e2e/README.md` for full docs.

```bash
# Ensure server is running first
source .env
make e2e              # Run all E2E tests (recommended)
# Or run specific Python tests:
uvx --from playwright --with playwright python tests/e2e/test_admin_ui_python.py
```

Tests capture console errors, CSP violations, auth flows, and page rendering. Screenshots saved to `/tmp/admin_*.png`.

### Before Claiming Work is Complete

1. **Rebuild** if static assets changed: `make stop && make build && make run`
2. **Run E2E tests** — they catch console errors, CSP violations, broken pages
3. **Document what you verified** — never claim "fixed" without runtime verification
4. **Check console errors** in test output summary

### Admin Page URLs (for manual testing)

```
http://localhost:8080/admin/login
http://localhost:8080/admin/dashboard
http://localhost:8080/admin/events
http://localhost:8080/admin/review-queue
http://localhost:8080/admin/duplicates
http://localhost:8080/admin/places
http://localhost:8080/admin/organizations
http://localhost:8080/admin/federation
http://localhost:8080/admin/users
http://localhost:8080/admin/api-keys
```

### Manual Testing Checklist

- [ ] Run E2E tests (catches most issues)
- [ ] Check console error summary from test output
- [ ] Test with empty data (0 items)
- [ ] Test with many items (pagination)
- [ ] Test error states (network failure, expired token)
- [ ] Test mobile viewport (responsive)

## Common Pitfalls

- **Wrong API field names** — Public API returns JSON-LD with `@id`, `address.addressLocality`, etc. NOT `ulid`, `city`, `region`. Always check [API Response Shapes](#public-api-response-shapes-json-ld) before writing rendering code.
- **Missing ULID extraction** — Never assume `entity.ulid` exists. Use `extractId(entity, 'places')` pattern to get ULID from `@id` URI. See [ULID Extraction](#ulid-extraction-from-id).
- **Assuming lifecycle/status exists** — The public API does not return `lifecycle` or `status` fields. Don't add Status columns for data from public endpoints.
- **Public vs admin API confusion** — `API.places.list()` hits `/api/v1/places` (public, JSON-LD). `API.places.similar()` hits `/api/v1/admin/places/{id}/similar` (admin, flat JSON). Response shapes differ.
- **Duplicated code** — Use `components.js` utilities, not per-page copies
- **Hardcoded URLs** — Use `API.*` methods, not direct `fetch()` calls
- **Silent errors** — Always `try/catch` with `showToast()` feedback
- **Inline styles** — Use Tabler classes (`text-danger h3` not `style="color:red"`)
- **Brittle selectors** — Use IDs or `data-*` attributes, not deep CSS paths
- **Theme mismatch** — `navbar-light` for light backgrounds, `navbar-dark` for dark

## Resources

- [Tabler Docs](https://tabler.io/docs) | [MDN Web Docs](https://developer.mozilla.org/)
- [Go HTML Templates](https://pkg.go.dev/html/template) | [Fetch API](https://developer.mozilla.org/en-US/docs/Web/API/Fetch_API)
- `web/admin/docs/review-queue.md` — Review queue feature documentation
- `web/admin/templates/README.md` — Template conventions and patterns
