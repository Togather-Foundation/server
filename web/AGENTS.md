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
