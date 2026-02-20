# Admin UI Overview

**Version:** 1.0  
**Created:** 2026-02-04  
**Framework:** Tabler v1.4.0 (Bootstrap 5)  
**Status:** Implementation in Progress

This is the entry point for the admin UI documentation. It covers the design system, file structure, and core concepts shared across all admin pages.

**Related documents:**
- **[Admin UI Components](admin-ui-components.md)** — Component library, page templates, JavaScript architecture, mobile responsiveness, implementation checklist
- **[User Management](user-management.md)** — User lifecycle, forms, invitation flow, activity tracking, troubleshooting

---

## Table of Contents

1. [Design System & Framework](#design-system--framework)
2. [File Structure](#file-structure)
3. [Core Concepts](#core-concepts)
4. [Reference Links](#reference-links)
5. [Questions & Support](#questions--support)

---

## Design System & Framework

### Tabler Framework

**Why Tabler?**
- Built on Bootstrap 5 (modern CSS: Flexbox, Grid)
- Designed specifically for admin dashboards
- Fully responsive and mobile-friendly
- MIT licensed - free to use and modify
- 40k+ GitHub stars, actively maintained
- Rich component library: tables, forms, modals, badges

**Key Files:**
```
web/admin/static/css/
├── tabler.min.css          # Core Tabler framework (~50KB minified, ~10KB gzipped)
└── custom.css              # Our custom overrides and additions

web/admin/static/js/
├── tabler.min.js           # Tabler JS for modals/dropdowns (~5KB)
├── api.js                  # Shared API client wrapper
├── components.js           # Reusable UI components (toasts, modals)
└── [page-specific].js      # Individual page scripts
```

### Color System

```css
/* Status Colors */
--tblr-success: #2fb344;    /* Published, active, approved */
--tblr-warning: #f76707;    /* Pending review, needs attention */
--tblr-danger: #d63939;     /* Cancelled, errors, delete actions */
--tblr-info: #4299e1;       /* Info badges, notifications */

/* Lifecycle State Colors */
.badge-published { background: var(--tblr-success); }
.badge-pending { background: var(--tblr-warning); }
.badge-cancelled { background: var(--tblr-danger); }
.badge-draft { background: var(--tblr-secondary); }
```

### Typography

- **System font stack:** `-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif`
- **Base size:** 14px (0.875rem)
- **Headings:** `.h1` through `.h6` classes available
- **Text utilities:** `.text-muted`, `.text-secondary`, `.fw-bold`, `.small`

---

## File Structure

```
web/admin/
├── static/
│   ├── css/
│   │   ├── tabler.min.css       # Tabler core (vendored)
│   │   └── custom.css           # Our customizations
│   └── js/
│       ├── tabler.min.js        # Tabler JS (vendored)
│       ├── api.js               # API client wrapper
│       ├── components.js        # Reusable components (modals, toasts)
│       ├── login.js             # Login page (existing)
│       ├── dashboard.js         # Dashboard page
│       ├── events.js            # Events list page
│       ├── event-edit.js        # Event editor
│       ├── duplicates.js        # Duplicate review
│       ├── api-keys.js          # API key management
│       ├── federation.js        # Federation node management
│       ├── users.js             # User management
│       ├── activity.js          # Activity log viewer
│       └── accept-invitation.js # Invitation acceptance
└── templates/
    ├── base.html                # Base template with navigation
    ├── login.html               # Login page (existing)
    ├── dashboard.html           # Dashboard with stats
    ├── events_list.html         # Events table with filters
    ├── event_edit.html          # Event form editor
    ├── duplicates.html          # Side-by-side duplicate comparison
    ├── api_keys.html            # API key CRUD interface
    ├── federation.html          # Federation node management
    ├── users.html               # User management list
    ├── activity.html            # Activity log viewer
    └── accept_invitation.html   # Invitation acceptance page
```

---

## Core Concepts

### Template Structure

All admin pages (except login) use a consistent structure:

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}} - SEL Admin</title>
    <link rel="stylesheet" href="/admin/static/css/tabler.min.css">
    <link rel="stylesheet" href="/admin/static/css/custom.css">
</head>
<body>
    <!-- Navigation (shared across all pages) -->
    <header class="navbar navbar-expand-md navbar-dark d-print-none">
        <!-- Nav content -->
    </header>
    
    <!-- Page Content -->
    <div class="page">
        <div class="page-wrapper">
            <!-- Page header -->
            <div class="page-header d-print-none">
                <div class="container-xl">
                    <div class="row g-2 align-items-center">
                        <div class="col">
                            <h2 class="page-title">{{.Title}}</h2>
                        </div>
                        <div class="col-auto ms-auto">
                            <!-- Action buttons -->
                        </div>
                    </div>
                </div>
            </div>
            
            <!-- Page body -->
            <div class="page-body">
                <div class="container-xl">
                    <!-- Page content -->
                </div>
            </div>
        </div>
    </div>
    
    <script src="/admin/static/js/tabler.min.js"></script>
    <script src="/admin/static/js/api.js"></script>
    <script src="/admin/static/js/components.js"></script>
    <script src="/admin/static/js/{{.PageScript}}.js"></script>
</body>
</html>
```

### Navigation Bar

Consistent across all admin pages:

```html
<header class="navbar navbar-expand-md navbar-dark d-print-none">
    <div class="container-xl">
        <button class="navbar-toggler" type="button" data-bs-toggle="collapse" data-bs-target="#navbar-menu">
            <span class="navbar-toggler-icon"></span>
        </button>
        <h1 class="navbar-brand navbar-brand-autodark d-none-navbar-horizontal pe-0 pe-md-3">
            <a href="/admin/dashboard">SEL Admin</a>
        </h1>
        <div class="navbar-nav flex-row order-md-last">
            <div class="nav-item dropdown">
                <a href="#" class="nav-link d-flex lh-1 text-reset p-0" data-bs-toggle="dropdown">
                    <span class="avatar avatar-sm">{{.User.Initials}}</span>
                    <div class="d-none d-xl-block ps-2">
                        <div>{{.User.Username}}</div>
                    </div>
                </a>
                <div class="dropdown-menu dropdown-menu-end">
                    <a class="dropdown-item" href="/api/v1/admin/logout">Logout</a>
                </div>
            </div>
        </div>
        <div class="collapse navbar-collapse" id="navbar-menu">
            <div class="d-flex flex-column flex-md-row flex-fill align-items-stretch align-items-md-center">
                <ul class="navbar-nav">
                    <li class="nav-item {{if eq .Page "dashboard"}}active{{end}}">
                        <a class="nav-link" href="/admin/dashboard">Dashboard</a>
                    </li>
                    <li class="nav-item {{if eq .Page "events"}}active{{end}}">
                        <a class="nav-link" href="/admin/events">Events</a>
                    </li>
                    <li class="nav-item {{if eq .Page "duplicates"}}active{{end}}">
                        <a class="nav-link" href="/admin/duplicates">Duplicates</a>
                    </li>
                    <li class="nav-item {{if eq .Page "api-keys"}}active{{end}}">
                        <a class="nav-link" href="/admin/api-keys">API Keys</a>
                    </li>
                    <li class="nav-item {{if eq .Page "federation"}}active{{end}}">
                        <a class="nav-link" href="/admin/federation">Federation</a>
                    </li>
                    <li class="nav-item {{if eq .Page "users"}}active{{end}}">
                        <a class="nav-link" href="/admin/users">Users</a>
                    </li>
                </ul>
            </div>
        </div>
    </div>
</header>
```

---

## Reference Links

- **Tabler Documentation:** https://docs.tabler.io
- **Tabler Components:** https://preview.tabler.io
- **Bootstrap 5 Docs:** https://getbootstrap.com/docs/5.3/
- **Backend API Endpoints:** See `../api/openapi.yaml`
- **Design Mockups:** (Add link to Figma/design files if available)

---

## Questions & Support

**For questions or clarifications:**
- Check existing beads: `bd list --status open`
- Review backend API contracts: `../api/openapi.yaml`
- Test backend endpoints directly: `curl -H "Authorization: Bearer $TOKEN" https://staging.toronto.togather.foundation/api/v1/admin/events`

**Common Issues:**
- **CSRF token errors:** Ensure CSRF middleware is properly configured
- **Auth cookies not working:** Check `SameSite` and `Secure` cookie attributes
- **Mobile layout broken:** Verify `viewport` meta tag in `<head>`
- **API calls failing:** Check CORS config and cookie credentials

---

**This guide will be updated as implementation progresses.**
