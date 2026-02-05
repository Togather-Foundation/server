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

## Testing Considerations

### Manual Testing Checklist

When modifying admin UI:
- [ ] Test with empty data (0 events)
- [ ] Test with many items (pagination)
- [ ] Test error states (network failure)
- [ ] Test unauthorized access (expired token)
- [ ] Test mobile viewport (responsive design)
- [ ] Test keyboard navigation (accessibility)
- [ ] Test browser console (no errors)

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
