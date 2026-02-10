# Web Development Guidelines for AI Agents

This document provides best practices for working with HTML, CSS, and JavaScript in the `/web/` directory of the Togather server codebase.

## Core Principles

### DRY (Don't Repeat Yourself)
- **Extract common patterns** into reusable components
- **Use template partials** (`_header.html`, `_footer.html`) for shared HTML structure
- **Centralize JavaScript utilities** in `components.js` for cross-page functionality
- **Share CSS styles** in `custom.css` rather than duplicating in individual pages

### KISS (Keep It Simple, Stupid)
- **Vanilla JavaScript first** - Use framework-free JS when possible
- **Progressive enhancement** - Pages should work without JavaScript, enhance with JS
- **Minimal dependencies** - Only include libraries when truly needed (Tabler for UI framework)
- **Direct DOM manipulation** - No complex state management unless required
- **Fetch API over axios** - Use built-in browser APIs

## Directory Structure

```
web/
├── admin/                      # Admin UI (authenticated)
│   ├── static/
│   │   ├── css/
│   │   │   ├── tabler.min.css  # Tabler UI framework (vendor)
│   │   │   ├── custom.css      # Project-wide admin customizations
│   │   │   └── admin.css       # Login page styles
│   │   └── js/
│   │       ├── tabler.min.js   # Tabler framework (vendor)
│   │       ├── api.js          # Centralized API client (JWT auth)
│   │       ├── components.js   # Reusable UI components (toasts, modals, logout)
│   │       ├── dashboard.js    # Page-specific: Dashboard
│   │       ├── events.js       # Page-specific: Events list
│   │       ├── event-edit.js   # Page-specific: Event editor
│   │       ├── duplicates.js   # Page-specific: Duplicate review
│   │       ├── api-keys.js     # Page-specific: API key management
│   │       └── login.js        # Page-specific: Login form
│   └── templates/
│       ├── _header.html        # Shared navigation header
│       ├── _footer.html        # Shared footer with scripts/modals
│       ├── dashboard.html      # Admin dashboard page
│       ├── events_list.html    # Events management page
│       ├── event_edit.html     # Event edit form
│       ├── duplicates.html     # Duplicate review interface
│       ├── api_keys.html       # API key management
│       └── login.html          # Login page (no header/footer)
├── static/                     # Public assets (future)
├── index.html                  # Public landing page
└── README.md                   # Web directory documentation
```

## HTML Templates

### Template Organization

**Shared Partials (DRY principle):**
- `_header.html` - Navigation menu, user dropdown, logout button
- `_footer.html` - Common modals, toast container, shared JavaScript includes

**Page Templates:**
- Use `{{ template "_header.html" . }}` to include shared header
- Use `{{ template "_footer.html" . }}` to include shared footer
- Pass `.ActivePage` context for navigation highlighting

### Template Data Contract

Every page handler should provide:
```go
data := map[string]interface{}{
    "Title":      "Page Title - SEL Admin",
    "ActivePage": "dashboard", // For nav highlighting
    // Page-specific data...
}
```

### HTML Best Practices

1. **Semantic HTML** - Use appropriate tags (`<nav>`, `<main>`, `<article>`, `<section>`)
2. **Accessibility** - Include ARIA labels, alt text, keyboard navigation
3. **Mobile-first** - Use Tabler's responsive grid (`col-sm-*`, `col-lg-*`)
4. **ID for JavaScript** - Use IDs for elements that JS needs to manipulate
5. **Classes for styling** - Use Tabler utility classes for styling

**Example:**
```html
<!-- Good: Semantic, accessible, mobile-friendly -->
<div class="col-sm-6 col-lg-3">
    <div class="card">
        <div class="card-body">
            <div class="subheader">Pending Reviews</div>
            <div class="h1 mb-3" id="pending-count">
                <div class="spinner-border spinner-border-sm" role="status" aria-label="Loading">
                    <span class="visually-hidden">Loading...</span>
                </div>
            </div>
            <a href="/admin/events?status=pending" class="text-muted">View all</a>
        </div>
    </div>
</div>
```

## CSS Guidelines

### Tabler Framework

We use **Tabler v1.4.0** as our base UI framework (Bootstrap 5-based).

**DO:**
- Use Tabler utility classes for common patterns
- Override in `custom.css` for project-specific customizations
- Follow Tabler's component structure

**DON'T:**
- Modify `tabler.min.css` (vendor file)
- Create custom CSS when Tabler provides a utility
- Use inline styles (use classes instead)

### Custom CSS Organization

File: `web/admin/static/css/custom.css`

```css
/* Structure by component, not by page */

/* 1. Layout overrides */
.navbar { /* ... */ }

/* 2. Component customizations */
.card-stat { /* ... */ }

/* 3. Utility additions (when Tabler doesn't provide) */
.text-truncate-2 { /* ... */ }

/* 4. Page-specific (minimal, prefer reusable classes) */
.dashboard-welcome { /* ... */ }
```

### CSS Best Practices

1. **Mobile-first** - Base styles for mobile, `@media` for larger screens
2. **BEM naming** (when creating custom components) - `.block__element--modifier`
3. **Avoid !important** - Use specificity correctly instead
4. **Use CSS variables** - For theme colors (Tabler provides these)
5. **Keep specificity low** - Avoid deep nesting (`#id .class .class`)
6. **Check color contrast** - Use Playwright to verify text/background combinations
7. **Match theme classes** - Use `navbar-light` for light backgrounds, `navbar-dark` for dark backgrounds

## JavaScript Architecture

### File Organization (DRY)

**Core Files (loaded on every admin page):**
1. `tabler.min.js` - UI framework (modals, dropdowns, etc.)
2. `api.js` - Centralized HTTP client with JWT authentication
3. `components.js` - Reusable utilities (toasts, modals, logout, date formatting)

**Page-specific Files (loaded per page):**
- `dashboard.js` - Dashboard-specific logic
- `events.js` - Events list page
- `login.js` - Login form handling

### API Client Pattern (api.js)

**Single source for all API calls:**
```javascript
const API = {
    async request(url, options = {}) {
        // Handles JWT token from localStorage
        // Handles error responses
        // Returns JSON
    },
    
    events: {
        list: (params) => API.request('/api/v1/admin/events?' + new URLSearchParams(params)),
        get: (id) => API.request(`/api/v1/admin/events/${id}`),
        update: (id, data) => API.request(`/api/v1/admin/events/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
        // ...
    },
    
    apiKeys: { /* ... */ },
    federation: { /* ... */ }
}
```

**Usage:**
```javascript
// Good: Use API client
const events = await API.events.list({ limit: 100 });

// Bad: Direct fetch (bypasses auth, error handling)
const response = await fetch('/api/v1/admin/events');
```

### Component Utilities (components.js)

**Shared functionality for all pages:**
```javascript
// Toast notifications
showToast('Success!', 'success');
showToast('Error occurred', 'error');

// Confirmation dialogs
confirmAction('Delete Event', 'Are you sure?', () => {
    // User confirmed
});

// XSS protection
const safe = escapeHtml(userInput);

// Date formatting
const formatted = formatDate('2026-03-15T19:00:00Z');

// Copy to clipboard
await copyToClipboard('some text');

// Loading states
setLoading(button, true);  // Shows spinner
setLoading(button, false); // Restores original text

// Search debouncing
const debouncedSearch = debounce((query) => {
    API.events.list({ q: query });
}, 300);

// Logout (auto-wired on all pages)
setupLogout(); // Called automatically by components.js
```

### Authentication Flow

**Login:**
1. POST to `/api/v1/admin/login` with credentials
2. Response includes JWT token: `{ token: "...", user: {...} }`
3. Store token in localStorage: `localStorage.setItem('admin_token', token)`
4. Redirect to dashboard

**API Calls:**
1. `api.js` reads token from localStorage
2. Sends `Authorization: Bearer <token>` header
3. Backend validates JWT

**Logout:**
1. POST to `/api/v1/admin/logout` (clears server-side cookie)
2. Clear localStorage: `localStorage.removeItem('admin_token')`
3. Redirect to login

### JavaScript Best Practices

1. **Use async/await** - Cleaner than promise chains
2. **Handle errors properly** - Use try/catch, show user feedback
3. **Avoid global variables** - Use IIFE or modules
4. **Use const/let** - Never use var
5. **Destructure when helpful** - `const { items } = data`
6. **Template literals** - Use backticks for strings with variables
7. **Arrow functions** - For callbacks and short functions
8. **Optional chaining** - `data.items?.length || 0`

**Good patterns:**
```javascript
// IIFE to avoid globals
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

## Security Considerations

### Content Security Policy (CSP) Compliance

**Our CSP policy: `script-src 'self'` - Only external scripts from same origin allowed**

**🚨 NEVER use inline scripts or event handlers:**

```html
<!-- ❌ BAD: Inline script block (CSP violation) -->
<script>
  console.log('This will be blocked by CSP!');
</script>

<!-- ❌ BAD: Inline event handler (CSP violation) -->
<button onclick="doSomething()">Click me</button>
<img onerror="handleError()" src="...">

<!-- ✅ GOOD: External script -->
<script src="/admin/static/js/my-script.js"></script>

<!-- ✅ GOOD: Event delegation in external JS -->
<button data-action="do-something">Click me</button>
```

**Event Delegation Pattern (CSP-compliant):**

```javascript
// In external JS file (e.g., events.js)
document.addEventListener('click', (e) => {
    const target = e.target.closest('[data-action]');
    if (!target) return;
    
    const action = target.dataset.action;
    if (action === 'do-something') {
        doSomething();
    }
});
```

**How to verify CSP compliance:**

```bash
# Check for inline event handlers (should return nothing)
grep -rn "onclick=\|onerror=\|onload=\|onchange=\|onsubmit=" web/admin/templates/

# Check for inline script blocks (should only show <script src=...>)
grep -rn "<script[^>]*>[^<]" web/admin/templates/

# Run E2E tests - they capture CSP violations
uvx --from playwright --with playwright python tests/e2e/test_admin_ui_python.py
```

**If you see CSP errors in console:**
1. Find the offending inline script/handler
2. Move the code to an external `.js` file
3. Use `data-action` attributes + event delegation for event handlers
4. Re-run E2E tests to verify fix

### XSS Prevention

**Always escape user input before inserting into DOM:**
```javascript
// Good: Use escapeHtml helper
element.innerHTML = escapeHtml(userInput);

// Good: Use textContent (auto-escapes)
element.textContent = userInput;

// Bad: Direct innerHTML with user data
element.innerHTML = userInput; // DANGEROUS!
```

### CSRF Protection

- **API routes** (`/api/v1/admin/*`) use Bearer tokens (CSRF-resistant)
- **HTML form routes** would use CSRF tokens (not currently used)
- Never send Bearer tokens in URLs or cookies (use Authorization header)

### Authentication

- JWT tokens stored in **localStorage** (for JavaScript API calls)
- Cookies used for **HTML page auth** (redirects)
- Always validate token on backend
- Auto-redirect to login on 401 errors

## Performance Guidelines

### Page Load

1. **Inline critical CSS** - Consider inlining above-the-fold styles
2. **Defer non-critical JS** - Use `defer` attribute
3. **Minimize HTTP requests** - Bundle scripts where appropriate
4. **Cache static assets** - Set appropriate Cache-Control headers

### JavaScript Performance

1. **Debounce search inputs** - Use `debounce()` helper (300ms typical)
2. **Pagination for large lists** - Don't load 1000+ items at once
3. **Use event delegation** - Single listener for many elements
4. **Avoid layout thrashing** - Batch DOM reads/writes
5. **Lazy load images** - Use `loading="lazy"` attribute

## Common Patterns

### Loading States

```javascript
async function saveEvent(id, data) {
    const button = document.getElementById('save-btn');
    try {
        setLoading(button, true);
        await API.events.update(id, data);
        showToast('Event saved', 'success');
    } catch (err) {
        showToast('Failed to save event', 'error');
    } finally {
        setLoading(button, false);
    }
}
```

### Confirmation Dialogs

```javascript
function deleteEvent(id) {
    confirmAction(
        'Delete Event',
        'This action cannot be undone. Continue?',
        async () => {
            try {
                await API.events.delete(id);
                showToast('Event deleted', 'success');
                window.location.reload();
            } catch (err) {
                showToast('Failed to delete event', 'error');
            }
        }
    );
}
```

### Pagination

```javascript
async function loadEvents(cursor = null) {
    const params = { limit: 50 };
    if (cursor) params.cursor = cursor;
    
    const data = await API.events.list(params);
    renderEvents(data.items);
    
    // Show "Load More" if there's a next cursor
    if (data.next_cursor) {
        showLoadMoreButton(data.next_cursor);
    }
}
```

### Search with Debounce

```javascript
const searchInput = document.getElementById('search');
const debouncedSearch = debounce(async (query) => {
    const data = await API.events.list({ q: query });
    renderEvents(data.items);
}, 300);

searchInput.addEventListener('input', (e) => {
    debouncedSearch(e.target.value);
});
```

## Development Server

### Starting the Server

**Use `make dev` for development with live reload:**
```bash
make dev
```

This will:
- Check for existing processes on port 8080 and stop them
- Start the server using `air` (live reload) if installed
- Fall back to `go run ./cmd/server` if `air` not found
- Watch for file changes and auto-rebuild (`.go`, `.html`, `.css`, `.js` files)

**Install air for live reload (one-time setup):**
```bash
make install-tools  # Installs air, golangci-lint, sqlc, etc.
```

### Multi-Agent Coordination

**IMPORTANT: When multiple AI agents work in parallel:**

1. **Only ONE agent should start the server**
2. **Other agents should check if server is running before starting**
3. **Share the same server instance**

```bash
# Check if server is already running
if lsof -ti:8080 > /dev/null 2>&1; then
    echo "✓ Server already running on port 8080"
    # Continue with testing/verification
else
    echo "Starting server..."
    make dev &
    sleep 5  # Wait for server to start
fi
```

**Why this matters:**
- `make dev` automatically kills existing servers on port 8080
- If Agent A starts the server, then Agent B runs `make dev`, it will kill Agent A's server
- With `air`, all agents see changes automatically (no restart needed)
- One server instance is sufficient for all parallel work

### Verifying Server is Running

```bash
# Check process
ps aux | grep -E "(air|go run)" | grep -v grep

# Check port
lsof -ti:8080

# Check health (returns 401, but that's expected without auth)
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/admin/healthz
```

## Testing Considerations

### Frontend Layout Testing Workflow

**🚨 CRITICAL: When fixing CSS/HTML layout issues, you MUST follow this workflow:**

#### 1. Make Changes Locally (DO NOT COMMIT YET)
- Edit the HTML/CSS/JavaScript files
- Keep changes uncommitted until verified

#### 2. Test on Local Server
```bash
# Check if server is already running
if ! lsof -ti:8080 > /dev/null 2>&1; then
    echo "Starting development server..."
    make dev &
    sleep 8  # Wait for server startup
else
    echo "✓ Server already running on port 8080"
    # Air will auto-reload your changes
fi
```

#### 3. Verify Layout with Screenshot
Create a Playwright script to capture the specific layout issue:

```python
# tests/e2e/verify_layout.py
import os
from playwright.sync_api import sync_playwright

def test_layout():
    admin_password = os.getenv('ADMIN_PASSWORD', 'XXKokg60kd8hLXgq')
    
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=False)  # Set False to see browser
        context = browser.new_context(viewport={'width': 1280, 'height': 720})
        page = context.new_page()
        
        # Login
        page.goto('http://localhost:8080/admin/login')
        page.fill('#username', 'admin')
        page.fill('#password', admin_password)
        page.click('button[type="submit"]')
        page.wait_for_url('**/admin/dashboard')
        
        # Navigate to page with layout issue
        page.goto('http://localhost:8080/admin/events')  # or whatever page
        
        # Wait for specific element to be visible
        page.wait_for_selector('.your-layout-element', timeout=10000)
        
        # Take screenshot
        page.screenshot(path='/tmp/layout_verification.png')
        print("✅ Screenshot saved to /tmp/layout_verification.png")
        
        # Optional: Check specific layout properties
        element = page.query_selector('.your-element')
        box = element.bounding_box()
        print(f"Element position: x={box['x']}, y={box['y']}, width={box['width']}, height={box['height']}")
        
        browser.close()

if __name__ == '__main__':
    test_layout()
```

Run the verification:
```bash
source .env  # Load ADMIN_PASSWORD
uvx --from playwright --with playwright python tests/e2e/verify_layout.py
```

#### 4. Visually Inspect Screenshot
```bash
# Open screenshot to verify layout is correct
xdg-open /tmp/layout_verification.png  # Linux
# or
open /tmp/layout_verification.png      # macOS
```

**Verify:**
- ✅ Elements are positioned correctly (side-by-side, aligned, etc.)
- ✅ Spacing/margins look right
- ✅ Text is readable (good contrast)
- ✅ No overlapping elements
- ✅ Responsive on different viewport sizes
- ✅ Works in both light and dark themes (if applicable)

#### 5. Run Full E2E Tests
```bash
uvx --from playwright --with playwright python tests/e2e/test_admin_ui_python.py
```

Check for:
- ❌ Console errors
- ❌ CSP violations
- ❌ JavaScript exceptions

#### 6. ONLY THEN Commit and Deploy
**DO NOT commit until screenshots confirm the layout is correct!**

```bash
git add <files>
git commit -m "Fix: Description of layout issue fixed"
git push origin <branch>
```

#### 7. Deploy to Staging
```bash
scripts/agent-run.sh ./deploy/scripts/deploy.sh staging --version HEAD
```

#### 8. Verify on Staging
Re-run verification script against staging:
```bash
# Modify script to use staging URL
uvx --from playwright --with playwright python tests/e2e/verify_layout_staging.py
```

### MANDATORY Error Verification Workflow

**🚨 CRITICAL: Before claiming any UI work is "fixed" or "complete", you MUST:**

1. **Check if server is already running (important for parallel agents):**
   ```bash
   # Check if port 8080 is in use
   lsof -ti:8080 > /dev/null 2>&1 && echo "Server already running" || echo "Server not running"
   
   # If server is NOT running, start it:
   make dev
   
   # If server IS running, skip to step 2 (use existing server)
   # IMPORTANT: Multiple agents should share one server instance
   ```

2. **Why check first?**
   - `make dev` kills any existing server on port 8080
   - If parallel agents both run `make dev`, they'll fight over the port
   - One shared server instance is sufficient for all agents
   - Air provides live reload, so code changes are picked up automatically

3. **Run E2E tests to capture console errors:**
   ```bash
   # The server must be running first (see step 1)
   uvx --from playwright --with playwright python tests/e2e/test_admin_ui_python.py
   ```

3. **Manually verify in browser (if E2E tests pass but user reports errors):**
   ```bash
   # Open each admin page in browser and check DevTools Console:
   open http://localhost:8080/admin/login
   open http://localhost:8080/admin/dashboard
   open http://localhost:8080/admin/events
   open http://localhost:8080/admin/duplicates
   open http://localhost:8080/admin/api-keys
   open http://localhost:8080/admin/federation
   # Check F12 > Console tab for errors on EACH page
   ```

4. **Check for specific error types:**
   - ❌ `ReferenceError: X is not defined` - Missing function/variable
   - ❌ `Content-Security-Policy` violations - Inline scripts/handlers
   - ❌ `Uncaught (in promise)` - Unhandled async errors
   - ❌ Network errors (404, 401, 500, etc.)
   - ❌ React/framework warnings (shouldn't exist in vanilla JS project)

5. **Document what you actually verified:**
   ```
   ✅ Ran E2E tests - 0 console errors found
   ✅ Manually loaded /admin/events - no console errors
   ✅ Tested feature X - works as expected
   ```

**❌ NEVER say "fixed" or "no errors found" based on:**
- Just reading the code without running it
- Assuming tests passed because build succeeded
- Previous test results (always re-run after changes)
- Grep/search results without runtime verification

**✅ Tools to help verify errors:**

1. **Automated Console Error Detection:**
   ```python
   # The E2E test already captures console errors:
   # See tests/e2e/test_admin_ui_python.py
   console_errors = []
   page.on("console", lambda msg: console_errors.append(msg.text) if msg.type == "error" else None)
   ```

2. **Manual Browser Inspection:**
   ```bash
   # Use playwright to open browser with DevTools
   PWDEBUG=1 python tests/e2e/test_admin_ui_python.py
   ```

3. **Check All Script Loading:**
   ```bash
   # Verify components.js loads before page-specific JS
   grep -A 10 "<script src=" web/admin/templates/_footer.html
   ```

4. **Search for CSP Violations:**
   ```bash
   # Find inline event handlers
   grep -rn "onclick=\|onerror=\|onload=" web/admin/templates/
   
   # Find inline scripts (should only return <script src=...>)
   grep -rn "<script[^>]*>[^<]" web/admin/templates/
   ```

### Admin Authentication for Local Testing

**IMPORTANT: The admin credentials are in `.env`, NOT hardcoded!**

```bash
# Check .env file for current admin credentials
grep "ADMIN_" .env

# Typical output:
# ADMIN_USERNAME=admin
# ADMIN_PASSWORD=XXKokg60kd8hLXgq  # <-- Use THIS password, not "admin"!
# ADMIN_EMAIL=ryan.kelln@gmail.com
```

**Common mistake:** Using `admin/admin` for testing will result in **401 Unauthorized** errors.

**When writing Playwright tests:**

```python
# ✅ CORRECT: Read password from environment
import os
admin_password = os.getenv('ADMIN_PASSWORD', 'XXKokg60kd8hLXgq')
page.fill('#password', admin_password)

# ❌ WRONG: Hardcoded "admin" password
page.fill('#password', 'admin')  # This will fail!
```

**Authentication Flow:**
1. Login form submits to `/api/v1/admin/login`
2. Server validates credentials and returns JWT token in response body
3. Server also sets `auth_token` HttpOnly cookie for HTML page auth
4. JavaScript stores token in `localStorage` for API calls
5. HTML pages use cookie auth (via `AdminAuthCookie` middleware)
6. API calls use Bearer token from localStorage (via `Authorization` header)

**Debugging auth issues:**
```python
# Check if login succeeded
response_status = page.evaluate('(await fetch("/api/v1/admin/login", {method: "POST", headers: {"Content-Type": "application/json"}, body: JSON.stringify({username: "admin", password: "XXKokg60kd8hLXgq"})})).status')
# Should be 200, not 401

# Check cookies after login
cookies = context.cookies()
auth_cookie = [c for c in cookies if c['name'] == 'auth_token']
# Should have auth_token cookie with httpOnly=True

# Check localStorage
storage = page.evaluate('() => ({ ...localStorage })')
# Should have 'admin_token' key
```

### Automated E2E Testing with Playwright

**Run comprehensive E2E tests against the live admin UI:**

```bash
# One-time setup: Install Playwright browsers
uvx --from playwright playwright install chromium

# IMPORTANT: Ensure server is running FIRST
# Check if already running (for parallel agents):
if ! lsof -ti:8080 > /dev/null 2>&1; then
    echo "Starting development server..."
    make dev &
    sleep 8  # Wait for server startup
fi

# Now run E2E tests (in another terminal or after backgrounding server)
# The test automatically reads ADMIN_PASSWORD from .env
uvx --from playwright --with playwright python tests/e2e/test_admin_ui_python.py
```

**What the E2E tests cover:**
- Login flow (success and failure cases)
- Dashboard loads and displays stats correctly
- All admin pages render (events, duplicates, API keys, federation)
- Navigation between pages works
- Logout functionality
- JavaScript console errors are captured
- CSP violations are detected
- Screenshots saved to `/tmp/admin_*.png` for debugging

**Override admin password for tests:**
```bash
# Use custom password (if .env has different credentials)
ADMIN_PASSWORD=mypassword uvx --from playwright --with playwright python tests/e2e/test_admin_ui_python.py
```

**When to run E2E tests:**
- **ALWAYS** after modifying any admin UI page
- **ALWAYS** after changing JavaScript in `web/admin/static/js/`
- **ALWAYS** after updating API endpoints used by admin UI
- **ALWAYS** before committing significant UI changes
- **ALWAYS** when debugging console errors
- **ALWAYS** before claiming work is "complete" or "fixed"

**Test artifacts:**
- Test script: `tests/e2e/test_admin_ui_python.py`
- Documentation: `tests/e2e/README.md`
- Screenshots: `/tmp/admin_*.png` (generated on each run)

### Testing Best Practices

**ALWAYS run E2E tests after UI changes** - The tests catch:
- Console errors (JavaScript failures, API errors)
- CSP violations (security policy issues)
- Visual issues (can check colors programmatically)
- Navigation/authentication flows

**Check console errors summary** at end of test run - it reports all errors found.

**If user reports errors after you claimed "no errors":**
- You failed to run the actual tests
- You must re-verify with the server running
- Document the actual verification steps you took

### Manual Testing Checklist

When modifying admin UI:
- [ ] **Run Playwright E2E tests first** (catches most issues automatically)
- [ ] Check console error summary from test output
- [ ] Test with empty data (0 events)
- [ ] Test with many items (pagination)
- [ ] Test error states (network failure)
- [ ] Test unauthorized access (expired token)
- [ ] Test mobile viewport (responsive design)
- [ ] Test keyboard navigation (accessibility)

### Browser Testing

**Primary targets:**
- Chrome/Edge (latest)
- Firefox (latest)
- Safari (latest)

**Mobile:**
- iOS Safari
- Chrome Android

## Common Pitfalls to Avoid

### ❌ Don't Repeat Yourself

```javascript
// Bad: Duplicated logout code in every page
function logout() {
    localStorage.removeItem('admin_token');
    window.location.href = '/admin/login';
}

// Good: Use shared components.js
setupLogout(); // Already handles logout for all pages
```

### ❌ Don't Hardcode URLs

```javascript
// Bad: Hardcoded URLs scattered everywhere
fetch('http://localhost:8080/api/v1/admin/events');

// Good: Use API client with relative URLs
API.events.list();
```

### ❌ Don't Ignore Errors

```javascript
// Bad: Silent failure
async function loadData() {
    const data = await API.events.list();
    render(data);
}

// Good: Handle errors, show user feedback
async function loadData() {
    try {
        const data = await API.events.list();
        render(data);
    } catch (err) {
        console.error('Load failed:', err);
        showToast('Failed to load data', 'error');
    }
}
```

### ❌ Don't Use Inline Styles

```html
<!-- Bad: Inline styles -->
<div style="color: red; font-size: 20px;">Error</div>

<!-- Good: Use Tabler classes -->
<div class="text-danger h3">Error</div>
```

### ❌ Don't Create Brittle Selectors

```javascript
// Bad: Deep, fragile selector
document.querySelector('#page > div.container > div.card > div.body > span.count');

// Good: Direct ID or data attribute
document.getElementById('event-count');
```

### ❌ Don't Mix Dark/Light Theme Classes

```html
<!-- Bad: White text on white background (invisible!) -->
<header class="navbar navbar-dark" style="background: white">

<!-- Good: Dark text on white background -->
<header class="navbar navbar-light" style="background: white">

<!-- Good: White text on dark background -->
<header class="navbar navbar-dark bg-dark">
```

## Refactoring Checklist

When refactoring HTML/CSS/JS:

1. **Identify duplication** - Look for repeated HTML blocks, CSS rules, JS functions
2. **Extract to shared files** - Move to `_header.html`, `custom.css`, `components.js`
3. **Update references** - Use `{{ template }}` syntax, utility functions
4. **Test all pages** - Ensure changes work across all admin pages
5. **Check for regressions** - Verify no functionality broke
6. **Update documentation** - Keep this guide current

## Resources

- **Tabler Documentation**: https://tabler.io/docs
- **MDN Web Docs**: https://developer.mozilla.org/
- **Go HTML Templates**: https://pkg.go.dev/html/template
- **Fetch API**: https://developer.mozilla.org/en-US/docs/Web/API/Fetch_API

## Getting Help

When asking for help or creating issues:
1. **Specify the page** - Which admin page has the issue?
2. **Include browser console** - Copy/paste any errors
3. **Describe expected behavior** - What should happen?
4. **Describe actual behavior** - What actually happens?
5. **Steps to reproduce** - How to trigger the issue?
