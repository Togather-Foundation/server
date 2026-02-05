# Admin UI Implementation Guide

**Version:** 1.0  
**Created:** 2026-02-04  
**Framework:** Tabler v1.4.0 (Bootstrap 5)  
**Status:** Implementation in Progress

## Overview

This guide provides comprehensive instructions for building the admin UI pages for the SEL Backend server. The admin interface allows administrators to manage events, review duplicates, manage API keys, and configure federation nodes.

## Table of Contents

1. [Design System & Framework](#design-system--framework)
2. [File Structure](#file-structure)
3. [Core Concepts](#core-concepts)
4. [Component Library](#component-library)
5. [Page Templates](#page-templates)
6. [User Management](#user-management)
7. [JavaScript Architecture](#javascript-architecture)
8. [Mobile Responsiveness](#mobile-responsiveness)
9. [Implementation Checklist](#implementation-checklist)

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

## Component Library

### Data Tables

**Basic Table:**
```html
<div class="card">
    <div class="card-header">
        <h3 class="card-title">Events</h3>
        <div class="card-actions">
            <button class="btn btn-primary" onclick="createEvent()">
                Add Event
            </button>
        </div>
    </div>
    <div class="table-responsive">
        <table class="table table-vcenter card-table">
            <thead>
                <tr>
                    <th>Name</th>
                    <th>Start Date</th>
                    <th>Status</th>
                    <th class="w-1">Actions</th>
                </tr>
            </thead>
            <tbody id="events-table">
                <!-- Populated via JS -->
            </tbody>
        </table>
    </div>
    <div class="card-footer d-flex align-items-center">
        <p class="m-0 text-muted">Showing <span id="page-start">1</span> to <span id="page-end">20</span> of <span id="total-count">1,234</span> entries</p>
        <ul class="pagination m-0 ms-auto" id="pagination">
            <!-- Generated via JS -->
        </ul>
    </div>
</div>
```

**Table Row Template (populated via JS):**
```javascript
function renderEventRow(event) {
    return `
        <tr data-event-id="${event.id}">
            <td>${escapeHtml(event.name)}</td>
            <td>${formatDate(event.start_date)}</td>
            <td><span class="badge badge-${event.status}">${event.status}</span></td>
            <td>
                <div class="btn-list flex-nowrap">
                    <a href="/admin/events/${event.id}" class="btn btn-sm">Edit</a>
                    <button onclick="deleteEvent('${event.id}')" class="btn btn-sm btn-ghost-danger">Delete</button>
                </div>
            </td>
        </tr>
    `;
}
```

### Status Badges

```html
<!-- Published -->
<span class="badge bg-success">Published</span>

<!-- Pending Review -->
<span class="badge bg-warning">Pending</span>

<!-- Cancelled -->
<span class="badge bg-danger">Cancelled</span>

<!-- Draft -->
<span class="badge bg-secondary">Draft</span>
```

### Forms

**Standard Form Layout:**
```html
<form id="event-form" class="card">
    <div class="card-body">
        <div class="row">
            <div class="col-md-6">
                <div class="mb-3">
                    <label class="form-label required">Event Name</label>
                    <input type="text" class="form-control" name="name" required>
                    <small class="form-hint">The display name of the event</small>
                </div>
            </div>
            <div class="col-md-6">
                <div class="mb-3">
                    <label class="form-label required">Start Date</label>
                    <input type="datetime-local" class="form-control" name="start_date" required>
                </div>
            </div>
        </div>
        
        <div class="mb-3">
            <label class="form-label">Description</label>
            <textarea class="form-control" name="description" rows="4"></textarea>
        </div>
        
        <div class="mb-3">
            <label class="form-label required">Lifecycle State</label>
            <select class="form-select" name="lifecycle_state" required>
                <option value="draft">Draft</option>
                <option value="published" selected>Published</option>
                <option value="cancelled">Cancelled</option>
            </select>
        </div>
    </div>
    
    <div class="card-footer text-end">
        <button type="button" class="btn" onclick="history.back()">Cancel</button>
        <button type="submit" class="btn btn-primary">Save Event</button>
    </div>
</form>
```

### Modals

**Confirmation Modal (Delete):**
```html
<div class="modal modal-blur fade" id="delete-modal" tabindex="-1" style="display: none;">
    <div class="modal-dialog modal-sm modal-dialog-centered">
        <div class="modal-content">
            <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
            <div class="modal-status bg-danger"></div>
            <div class="modal-body text-center py-4">
                <svg xmlns="http://www.w3.org/2000/svg" class="icon mb-2 text-danger icon-lg" width="24" height="24" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" fill="none">
                    <path stroke="none" d="M0 0h24v24H0z" fill="none"/>
                    <path d="M12 9v2m0 4v.01"/>
                    <path d="M5 19h14a2 2 0 0 0 1.84 -2.75l-7.1 -12.25a2 2 0 0 0 -3.5 0l-7.1 12.25a2 2 0 0 0 1.75 2.75"/>
                </svg>
                <h3>Are you sure?</h3>
                <div class="text-muted" id="delete-message">Do you really want to delete this event? This action cannot be undone.</div>
            </div>
            <div class="modal-footer">
                <div class="w-100">
                    <div class="row">
                        <div class="col"><button type="button" class="btn w-100" data-bs-dismiss="modal">Cancel</button></div>
                        <div class="col"><button type="button" class="btn btn-danger w-100" id="confirm-delete">Delete</button></div>
                    </div>
                </div>
            </div>
        </div>
    </div>
</div>
```

### Toast Notifications

**Success Toast:**
```html
<div class="toast show" role="alert">
    <div class="toast-header">
        <span class="badge bg-success me-2"></span>
        <strong class="me-auto">Success</strong>
        <button type="button" class="btn-close" data-bs-dismiss="toast"></button>
    </div>
    <div class="toast-body">
        Event successfully updated.
    </div>
</div>
```

**Toast Container (add to body):**
```html
<div class="toast-container position-fixed top-0 end-0 p-3" id="toast-container"></div>
```

### Empty States

```html
<div class="empty">
    <div class="empty-icon">
        <svg xmlns="http://www.w3.org/2000/svg" class="icon" width="24" height="24" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" fill="none">
            <path stroke="none" d="M0 0h24v24H0z" fill="none"/>
            <rect x="3" y="5" width="18" height="14" rx="2"/>
            <line x1="3" y1="10" x2="21" y2="10"/>
            <line x1="12" y1="15" x2="12" y2="15.01"/>
        </svg>
    </div>
    <p class="empty-title">No events found</p>
    <p class="empty-subtitle text-muted">
        Try adjusting your search or filter to find what you're looking for.
    </p>
    <div class="empty-action">
        <button class="btn btn-primary" onclick="clearFilters()">
            Clear filters
        </button>
    </div>
</div>
```

### Loading States

**Spinner:**
```html
<div class="text-center py-4">
    <div class="spinner-border" role="status">
        <span class="visually-hidden">Loading...</span>
    </div>
</div>
```

**Skeleton Loader (for tables):**
```html
<tr class="skeleton-row">
    <td><div class="skeleton-line"></div></td>
    <td><div class="skeleton-line"></div></td>
    <td><div class="skeleton-line"></div></td>
</tr>
```

---

## Page Templates

### Dashboard (`dashboard.html`)

**Purpose:** Display summary statistics and quick actions

**Components:**
- Stat cards (pending reviews, total events, API keys, federation nodes)
- Recent activity feed
- Quick action buttons

**Layout:**
```html
<div class="page-body">
    <div class="container-xl">
        <!-- Stats Row -->
        <div class="row row-deck row-cards mb-3">
            <div class="col-sm-6 col-lg-3">
                <div class="card">
                    <div class="card-body">
                        <div class="d-flex align-items-center">
                            <div class="subheader">Pending Reviews</div>
                        </div>
                        <div class="h1 mb-3" id="pending-count">-</div>
                        <a href="/admin/events?status=pending" class="text-muted">View all</a>
                    </div>
                </div>
            </div>
            <!-- More stat cards -->
        </div>
        
        <!-- Recent Activity -->
        <div class="row">
            <div class="col-12">
                <div class="card">
                    <div class="card-header">
                        <h3 class="card-title">Recent Activity</h3>
                    </div>
                    <div class="list-group list-group-flush" id="recent-activity">
                        <!-- Activity items -->
                    </div>
                </div>
            </div>
        </div>
    </div>
</div>
```

### Events List (`events_list.html`)

**Purpose:** Display paginated event list with filters

**Components:**
- Filter bar (status, date range, search)
- Data table with sorting
- Pagination controls
- Bulk actions

**Key Features:**
- Search by name/description
- Filter by lifecycle_state (draft, published, cancelled)
- Filter by date range
- Sort by columns
- Cursor-based pagination

### Event Edit (`event_edit.html`)

**Purpose:** Edit individual event details

**Components:**
- Multi-column form layout
- Validation feedback
- Save/cancel actions
- Occurrence management (add/edit/remove occurrences)

**Key Features:**
- All Schema.org Event fields
- Occurrence CRUD (events can have multiple occurrences)
- Real-time validation
- Auto-save draft support (optional)

### Duplicates (`duplicates.html`)

**Purpose:** Review and merge duplicate events

**Components:**
- Side-by-side event comparison
- Merge confirmation modal
- Field-level selection (choose which fields to keep)

**Layout:**
```html
<div class="row">
    <div class="col-md-6">
        <div class="card">
            <div class="card-header">
                <h3 class="card-title">Event A</h3>
                <div class="card-actions">
                    <span class="badge bg-info">Source: ScraperBot</span>
                </div>
            </div>
            <div class="card-body">
                <dl class="row">
                    <dt class="col-5">Name:</dt>
                    <dd class="col-7">Toronto Jazz Festival</dd>
                    <!-- More fields -->
                </dl>
            </div>
        </div>
    </div>
    <div class="col-md-6">
        <!-- Event B (duplicate) -->
    </div>
</div>
<div class="text-center mt-3">
    <button class="btn btn-primary" onclick="confirmMerge()">Merge Events</button>
</div>
```

### API Keys (`api_keys.html`)

**Purpose:** Manage API keys for agents

**Components:**
- API key list table
- Create key modal
- Revoke confirmation
- Copy-to-clipboard for new keys

**Key Features:**
- Display key prefix (first 8 chars + `...`)
- Show creation date, last used date
- Revoke action with confirmation
- Create modal with role selection

### Federation (`federation.html`)

**Purpose:** Manage trusted federation nodes

**Components:**
- Node list table
- Create/edit node form
- Node status indicators
- Test connection button

**Key Features:**
- Node URI, name, trust level
- Enable/disable nodes
- Test federation sync endpoint
- View sync statistics

---

## User Management

### Overview

The user management interface allows administrators to manage admin user accounts, control access levels, send invitations, and monitor user activity. Access the users page at `/admin/users`.

**Key Features:**
- Create and manage admin user accounts
- Three role levels: admin, editor, viewer
- Email invitation system with 7-day expiration
- User state tracking (active, inactive, pending invitation)
- Activity logging and audit trails
- Soft delete support

**Screenshot:** `web/admin/static/images/users-list.png`

---

### User Roles

The system supports three role levels with progressive permissions:

| Role | Permissions | Use Case |
|------|-------------|----------|
| **Admin** | Full access: manage events, users, API keys, federation nodes | Site administrators, project leads |
| **Editor** | Create/edit events, review duplicates, view reports | Content managers, event curators |
| **Viewer** | Read-only access to events and reports | Auditors, stakeholders, analysts |

**Role Badge Colors:**
```html
<span class="badge bg-danger">Admin</span>
<span class="badge bg-primary">Editor</span>
<span class="badge bg-secondary">Viewer</span>
```

---

### User States

Users can be in one of three states:

1. **Pending Invitation** (Yellow badge)
   - User created but hasn't accepted invitation
   - Cannot log in yet
   - Invitation link expires in 7 days

2. **Active** (Green badge)
   - Invitation accepted and password set
   - Can log in and perform role-based actions
   - Normal operational state

3. **Inactive** (Gray badge)
   - Account deactivated by admin
   - Cannot log in
   - Data preserved for audit trail

**State Badges:**
```html
<span class="badge bg-warning">Pending Invitation</span>
<span class="badge bg-success">Active</span>
<span class="badge bg-secondary">Inactive</span>
```

---

### Creating New Users

#### Step 1: Navigate to Users Page

- Click **Users** in the navigation bar
- Click **Create User** button in the top-right corner

**Screenshot:** `web/admin/static/images/users-create-button.png`

#### Step 2: Fill User Details

The create user modal contains the following fields:

```html
<form id="create-user-form" class="modal-content">
    <div class="modal-header">
        <h5 class="modal-title">Create New User</h5>
        <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
    </div>
    <div class="modal-body">
        <div class="mb-3">
            <label class="form-label required">Username</label>
            <input type="text" class="form-control" name="username" required>
            <small class="form-hint">Alphanumeric, 3-20 characters</small>
        </div>
        
        <div class="mb-3">
            <label class="form-label required">Email</label>
            <input type="email" class="form-control" name="email" required>
            <small class="form-hint">Invitation will be sent to this address</small>
        </div>
        
        <div class="mb-3">
            <label class="form-label required">Role</label>
            <select class="form-select" name="role" required>
                <option value="viewer">Viewer - Read-only access</option>
                <option value="editor">Editor - Create and edit events</option>
                <option value="admin">Admin - Full access</option>
            </select>
        </div>
    </div>
    <div class="modal-footer">
        <button type="button" class="btn" data-bs-dismiss="modal">Cancel</button>
        <button type="submit" class="btn btn-primary">Create & Send Invitation</button>
    </div>
</form>
```

**Validation Rules:**
- Username: 3-20 alphanumeric characters, no spaces
- Email: Valid email format, must be unique
- Role: One of admin, editor, viewer

**Screenshot:** `web/admin/static/images/users-create-modal.png`

#### Step 3: Invitation Email Flow

When you click **Create & Send Invitation**:

1. User account is created with state `pending_invitation`
2. Invitation email is sent to the provided address
3. Invitation link expires in **7 days**
4. Success toast notification confirms creation

**Email Template:**
```
Subject: You've been invited to SEL Admin

Hi [Username],

You've been invited to join SEL Admin as [Role].

Click the link below to accept your invitation and set your password:
[Invitation Link]

This invitation expires in 7 days.

If you didn't expect this invitation, please ignore this email.
```

**Keyboard Shortcut:** `Ctrl+N` (when on users page) opens create user modal

---

### Managing Existing Users

#### Viewing User List

The users list displays all admin accounts with the following information:

```html
<table class="table table-vcenter card-table">
    <thead>
        <tr>
            <th>Username</th>
            <th>Email</th>
            <th>Role</th>
            <th>Status</th>
            <th>Created</th>
            <th>Last Active</th>
            <th class="w-1">Actions</th>
        </tr>
    </thead>
    <tbody id="users-table">
        <!-- Populated via JS -->
    </tbody>
</table>
```

**Filters Available:**
- Role (admin/editor/viewer)
- Status (active/inactive/pending)
- Search by username or email

**Screenshot:** `web/admin/static/images/users-list-filters.png`

#### Editing User Details

Click the **Edit** button on any user row to open the edit modal:

**Editable Fields:**
- Username (must remain unique)
- Email address (must remain unique)
- Role (admin/editor/viewer)

**Non-Editable Fields:**
- User ID (ULID, immutable)
- Created date
- Password (changed via separate flow)

**Edit Modal:**
```html
<form id="edit-user-form">
    <div class="modal-body">
        <div class="mb-3">
            <label class="form-label">User ID</label>
            <input type="text" class="form-control" disabled value="01HQXYZ...">
            <small class="form-hint">Immutable identifier</small>
        </div>
        
        <div class="mb-3">
            <label class="form-label required">Username</label>
            <input type="text" class="form-control" name="username" required>
        </div>
        
        <div class="mb-3">
            <label class="form-label required">Email</label>
            <input type="email" class="form-control" name="email" required>
        </div>
        
        <div class="mb-3">
            <label class="form-label required">Role</label>
            <select class="form-select" name="role" required>
                <option value="viewer">Viewer</option>
                <option value="editor">Editor</option>
                <option value="admin">Admin</option>
            </select>
        </div>
    </div>
    <div class="modal-footer">
        <button type="button" class="btn" data-bs-dismiss="modal">Cancel</button>
        <button type="submit" class="btn btn-primary">Save Changes</button>
    </div>
</form>
```

**Screenshot:** `web/admin/static/images/users-edit-modal.png`

#### Deactivating Users

To deactivate a user (prevent login without deleting):

1. Click the **Deactivate** button in the user row actions
2. Confirm in the modal dialog
3. User state changes to `inactive`
4. User can no longer log in

**Deactivate Modal:**
```html
<div class="modal-body text-center py-4">
    <svg class="icon mb-2 text-warning icon-lg">...</svg>
    <h3>Deactivate User?</h3>
    <div class="text-muted">
        <strong id="deactivate-username"></strong> will no longer be able to log in.
        You can reactivate them later.
    </div>
</div>
<div class="modal-footer">
    <button type="button" class="btn" data-bs-dismiss="modal">Cancel</button>
    <button type="button" class="btn btn-warning" id="confirm-deactivate">Deactivate</button>
</div>
```

**Reactivating:** Click **Reactivate** button on inactive users to restore access.

**Screenshot:** `web/admin/static/images/users-deactivate.png`

#### Deleting Users (Soft Delete)

To permanently remove a user (soft delete):

1. Click the **Delete** button in the user row actions
2. Confirm in the modal dialog (red warning)
3. User is soft-deleted (marked `deleted_at`)
4. User no longer appears in default list
5. Activity logs preserved for audit trail

**Delete Modal:**
```html
<div class="modal-body text-center py-4">
    <svg class="icon mb-2 text-danger icon-lg">...</svg>
    <h3>Delete User?</h3>
    <div class="text-muted">
        This will permanently remove <strong id="delete-username"></strong> from the system.
        This action cannot be undone, but activity logs will be preserved.
    </div>
</div>
<div class="modal-footer">
    <button type="button" class="btn" data-bs-dismiss="modal">Cancel</button>
    <button type="button" class="btn btn-danger" id="confirm-delete">Delete User</button>
</div>
```

**⚠️ Warning:** Deleting yourself will log you out immediately.

**Screenshot:** `web/admin/static/images/users-delete.png`

#### Resending Invitations

For users in `pending_invitation` state whose invitation has expired:

1. Click the **Resend Invitation** button
2. New invitation email sent with fresh 7-day expiration
3. Old invitation link is invalidated
4. Success toast confirms resend

**Button (visible only for pending users):**
```html
<button class="btn btn-sm btn-ghost-secondary" onclick="resendInvitation('${user.id}')">
    Resend Invitation
</button>
```

**Keyboard Shortcut:** `Shift+R` (with user row selected)

---

### User Activity Tracking

#### Viewing Activity Logs

Access user activity from the main users list or individual user detail page:

**Activity Log Table:**
```html
<div class="card">
    <div class="card-header">
        <h3 class="card-title">Activity Log</h3>
        <div class="card-actions">
            <button class="btn btn-sm" onclick="exportActivity()">
                Export CSV
            </button>
        </div>
    </div>
    <div class="table-responsive">
        <table class="table table-vcenter card-table">
            <thead>
                <tr>
                    <th>Timestamp</th>
                    <th>User</th>
                    <th>Event Type</th>
                    <th>Resource</th>
                    <th>IP Address</th>
                    <th>Details</th>
                </tr>
            </thead>
            <tbody id="activity-table">
                <!-- Populated via JS -->
            </tbody>
        </table>
    </div>
</div>
```

**Screenshot:** `web/admin/static/images/users-activity-log.png`

#### Event Types

Activity logs track the following event types:

| Event Type | Description | Example |
|------------|-------------|---------|
| `user.login` | Successful login | User logged in from 192.168.1.1 |
| `user.logout` | User-initiated logout | User logged out |
| `user.login_failed` | Failed login attempt | Invalid password attempt |
| `event.created` | Event created | Created event "Jazz Festival" |
| `event.updated` | Event modified | Updated event "Jazz Festival" |
| `event.deleted` | Event deleted | Deleted event "Jazz Festival" |
| `event.merged` | Events merged | Merged event A into event B |
| `apikey.created` | API key generated | Created API key "bot-scraper" |
| `apikey.revoked` | API key revoked | Revoked API key "bot-scraper" |
| `user.created` | User account created | Created user "john_doe" |
| `user.updated` | User account modified | Updated user "john_doe" role to editor |
| `user.deactivated` | User deactivated | Deactivated user "john_doe" |
| `user.deleted` | User deleted | Deleted user "john_doe" |

**Event Type Badge Colors:**
```javascript
function getEventTypeColor(eventType) {
    const colors = {
        'user.login': 'success',
        'user.logout': 'info',
        'user.login_failed': 'danger',
        'event.created': 'success',
        'event.updated': 'info',
        'event.deleted': 'danger',
        'event.merged': 'warning',
        'apikey.created': 'success',
        'apikey.revoked': 'danger',
        'user.created': 'success',
        'user.updated': 'info',
        'user.deactivated': 'warning',
        'user.deleted': 'danger'
    };
    return colors[eventType] || 'secondary';
}
```

#### Filtering Activity

Filter activity logs by:

**Date Range:**
```html
<div class="row g-2">
    <div class="col-auto">
        <input type="date" class="form-control" id="start-date" placeholder="Start date">
    </div>
    <div class="col-auto">
        <input type="date" class="form-control" id="end-date" placeholder="End date">
    </div>
</div>
```

**Event Type:**
```html
<select class="form-select" id="event-type-filter">
    <option value="">All Event Types</option>
    <option value="user.login">Logins</option>
    <option value="user.login_failed">Failed Logins</option>
    <option value="event.created">Event Created</option>
    <option value="event.updated">Event Updated</option>
    <option value="event.deleted">Event Deleted</option>
</select>
```

**User (admin view only):**
```html
<select class="form-select" id="user-filter">
    <option value="">All Users</option>
    <!-- Populated from user list -->
</select>
```

**Screenshot:** `web/admin/static/images/users-activity-filters.png`

#### Understanding Audit Trail

Each activity log entry contains:

- **Timestamp:** Exact time of action (ISO 8601 format)
- **User ID:** ULID of user who performed action
- **Username:** Display name of user
- **Event Type:** Action category (see table above)
- **Resource Type:** Type of resource affected (event, user, apikey)
- **Resource ID:** ULID of affected resource
- **IP Address:** Source IP of request
- **User Agent:** Browser/client information
- **Details:** JSON payload with additional context

**Example Activity Entry:**
```json
{
    "id": "01HQXYZ123...",
    "timestamp": "2026-02-05T14:30:00Z",
    "user_id": "01HQABC456...",
    "username": "admin_user",
    "event_type": "event.updated",
    "resource_type": "event",
    "resource_id": "01HQDEF789...",
    "ip_address": "192.168.1.100",
    "user_agent": "Mozilla/5.0...",
    "details": {
        "changed_fields": ["name", "start_date"],
        "old_values": {"name": "Jazz Fest"},
        "new_values": {"name": "Toronto Jazz Festival"}
    }
}
```

#### Exporting Activity Data

Export activity logs to CSV for external analysis:

1. Apply desired filters (date range, event type, user)
2. Click **Export CSV** button
3. File downloads as `activity_log_YYYYMMDD.csv`

**CSV Format:**
```csv
Timestamp,User,Event Type,Resource,IP Address,Details
2026-02-05T14:30:00Z,admin_user,event.updated,event:01HQ...,192.168.1.100,"Changed: name, start_date"
```

**Keyboard Shortcut:** `Ctrl+E` (export current filtered view)

**Screenshot:** `web/admin/static/images/users-activity-export.png`

---

### Invitation Acceptance (User Perspective)

#### What Users Receive

When an admin creates a user account, the invited user receives:

**Email Subject:** "You've been invited to SEL Admin"

**Email Body:**
```
Hi [Username],

You've been invited to join the SEL Admin interface as a [Role].

Click the link below to accept your invitation and set your password:
https://toronto.togather.foundation/admin/accept-invitation?token=abc123xyz...

This invitation expires on [Date] (7 days from now).

Your Role: [admin/editor/viewer]
- [List of permissions for this role]

If you didn't expect this invitation, please ignore this email.

Questions? Contact your administrator.
```

**Screenshot:** `web/admin/static/images/invitation-email.png`

#### Accepting the Invitation

**Step 1: Click Invitation Link**
- Link opens to `/admin/accept-invitation?token=...`
- Token is validated (not expired, not already used)
- Username and email are pre-filled

**Step 2: Set Password**

```html
<form id="accept-invitation-form" class="card card-md">
    <div class="card-body">
        <h2 class="card-title text-center mb-4">Set Your Password</h2>
        
        <div class="mb-3">
            <label class="form-label">Username</label>
            <input type="text" class="form-control" disabled value="john_doe">
        </div>
        
        <div class="mb-3">
            <label class="form-label">Email</label>
            <input type="email" class="form-control" disabled value="john@example.com">
        </div>
        
        <div class="mb-3">
            <label class="form-label required">Password</label>
            <input type="password" class="form-control" name="password" required>
            <small class="form-hint">
                Minimum 12 characters, must include:
                <ul class="mb-0">
                    <li>Uppercase letter (A-Z)</li>
                    <li>Lowercase letter (a-z)</li>
                    <li>Number (0-9)</li>
                    <li>Special character (!@#$%^&*)</li>
                </ul>
            </small>
        </div>
        
        <div class="mb-3">
            <label class="form-label required">Confirm Password</label>
            <input type="password" class="form-control" name="password_confirm" required>
        </div>
        
        <div class="form-footer">
            <button type="submit" class="btn btn-primary w-100">
                Accept Invitation & Set Password
            </button>
        </div>
    </div>
</form>
```

**Password Requirements:**
- Minimum 12 characters
- At least one uppercase letter
- At least one lowercase letter
- At least one number
- At least one special character (!@#$%^&*)
- Passwords must match

**Screenshot:** `web/admin/static/images/accept-invitation-form.png`

**Step 3: Confirmation**

Upon successful password set:
1. User state changes from `pending_invitation` to `active`
2. Invitation token is marked as used
3. User is redirected to login page
4. Success message: "Password set successfully. Please log in."

**Screenshot:** `web/admin/static/images/invitation-accepted.png`

#### First Login After Acceptance

**Step 1: Login**
- Navigate to `/admin/login`
- Enter username and password
- Click **Sign In**

**Step 2: Welcome Screen (First-Time Users)**

```html
<div class="modal modal-blur" id="welcome-modal">
    <div class="modal-dialog modal-lg modal-dialog-centered">
        <div class="modal-content">
            <div class="modal-header">
                <h5 class="modal-title">Welcome to SEL Admin!</h5>
            </div>
            <div class="modal-body">
                <p>You're logged in as <strong>[Username]</strong> with <strong>[Role]</strong> access.</p>
                
                <h6>Quick Start:</h6>
                <ul>
                    <li><strong>Dashboard:</strong> View system statistics and recent activity</li>
                    <li><strong>Events:</strong> Manage events, duplicates, and occurrences</li>
                    <li><strong>API Keys:</strong> [Admin only] Generate keys for agents</li>
                    <li><strong>Federation:</strong> [Admin only] Manage trusted nodes</li>
                    <li><strong>Users:</strong> [Admin only] Manage admin accounts</li>
                </ul>
                
                <h6>Keyboard Shortcuts:</h6>
                <ul>
                    <li><kbd>Ctrl+K</kbd> - Quick search</li>
                    <li><kbd>Ctrl+N</kbd> - Create new (context-aware)</li>
                    <li><kbd>Ctrl+S</kbd> - Save current form</li>
                </ul>
            </div>
            <div class="modal-footer">
                <button type="button" class="btn btn-primary" data-bs-dismiss="modal">
                    Get Started
                </button>
            </div>
        </div>
    </div>
</div>
```

**Screenshot:** `web/admin/static/images/first-login-welcome.png`

#### Expired Invitation Handling

If invitation token is expired:

**Error Page:**
```html
<div class="empty">
    <div class="empty-icon">
        <svg class="icon text-warning icon-lg">...</svg>
    </div>
    <p class="empty-title">Invitation Expired</p>
    <p class="empty-subtitle text-muted">
        This invitation link has expired. Please contact your administrator to resend the invitation.
    </p>
</div>
```

**Screenshot:** `web/admin/static/images/invitation-expired.png`

---

### Troubleshooting User Management

#### Common Issues

**Issue: User not receiving invitation email**

- Check spam/junk folder
- Verify email address is correct (admin can edit and resend)
- Check server email configuration (SMTP settings)
- View email logs: `grep "invitation" /var/log/sel-backend/email.log`

**Issue: Invitation link shows "Invalid token"**

- Token may be expired (7-day limit)
- Token may have already been used
- Admin should resend invitation

**Issue: Cannot set password (validation errors)**

- Ensure password meets all requirements:
  - Minimum 12 characters
  - Uppercase, lowercase, number, special character
- Passwords must match
- Avoid common passwords (checked against list)

**Issue: User still can't log in after accepting invitation**

- Verify user state is `active` (not `pending_invitation` or `inactive`)
- Clear browser cookies and try again
- Check authentication logs: `grep "login.*username" /var/log/sel-backend/auth.log`

**Issue: Deleted user still appears in activity logs**

- This is expected behavior (audit trail preservation)
- Soft-deleted users show as `[Deleted User]` in logs
- Original user ID is preserved in logs

#### Admin Actions Log Location

All admin actions are logged to:
```
/var/log/sel-backend/admin-audit.log
```

**Example Log Entry:**
```
2026-02-05T14:30:00Z [INFO] user_id=01HQABC... action=user.created target_user_id=01HQDEF... target_username=john_doe role=editor ip=192.168.1.100
```

#### Database Queries for Troubleshooting

**Check user status:**
```sql
SELECT id, username, email, role, status, created_at, deleted_at
FROM users
WHERE username = 'john_doe';
```

**Check pending invitations:**
```sql
SELECT u.username, u.email, i.token, i.expires_at, i.used_at
FROM users u
JOIN invitations i ON u.id = i.user_id
WHERE u.status = 'pending_invitation'
AND i.expires_at > NOW();
```

**Check recent activity for user:**
```sql
SELECT timestamp, event_type, resource_type, resource_id, ip_address
FROM activity_logs
WHERE user_id = '01HQABC...'
ORDER BY timestamp DESC
LIMIT 50;
```

---

### User Management API Reference

For developers building integrations:

**Create User:**
```bash
POST /api/v1/admin/users
Content-Type: application/json

{
    "username": "john_doe",
    "email": "john@example.com",
    "role": "editor"
}
```

**List Users:**
```bash
GET /api/v1/admin/users?status=active&role=editor&limit=50&after=cursor
```

**Update User:**
```bash
PUT /api/v1/admin/users/01HQABC...
Content-Type: application/json

{
    "username": "john_doe",
    "email": "john@example.com",
    "role": "admin"
}
```

**Deactivate User:**
```bash
POST /api/v1/admin/users/01HQABC.../deactivate
```

**Delete User:**
```bash
DELETE /api/v1/admin/users/01HQABC...
```

**Resend Invitation:**
```bash
POST /api/v1/admin/users/01HQABC.../resend-invitation
```

**Get Activity Logs:**
```bash
GET /api/v1/admin/activity?user_id=01HQABC...&event_type=user.login&start_date=2026-02-01&limit=50
```

See full API documentation in `specs/001-sel-backend/contracts/openapi.yaml`

---

## JavaScript Architecture

### API Client (`api.js`)

Centralized API wrapper for all backend calls:

```javascript
const API = {
    // Base request method
    async request(url, options = {}) {
        const response = await fetch(url, {
            ...options,
            headers: {
                'Content-Type': 'application/json',
                ...options.headers
            },
            credentials: 'include' // Include cookies for auth
        });
        
        if (!response.ok) {
            const error = await response.json();
            throw new Error(error.detail || 'Request failed');
        }
        
        return response.json();
    },
    
    // Events API
    events: {
        list: (params = {}) => {
            const query = new URLSearchParams(params);
            return API.request(`/api/v1/admin/events?${query}`);
        },
        
        get: (id) => API.request(`/api/v1/admin/events/${id}`),
        
        update: (id, data) => API.request(`/api/v1/admin/events/${id}`, {
            method: 'PUT',
            body: JSON.stringify(data)
        }),
        
        delete: (id) => API.request(`/api/v1/admin/events/${id}`, {
            method: 'DELETE'
        }),
        
        merge: (sourceId, targetId) => API.request('/api/v1/admin/events/merge', {
            method: 'POST',
            body: JSON.stringify({ source_id: sourceId, target_id: targetId })
        }),
        
        pending: () => API.request('/api/v1/admin/events/pending')
    },
    
    // API Keys
    apiKeys: {
        list: () => API.request('/api/v1/admin/api-keys'),
        create: (data) => API.request('/api/v1/admin/api-keys', {
            method: 'POST',
            body: JSON.stringify(data)
        }),
        revoke: (id) => API.request(`/api/v1/admin/api-keys/${id}`, {
            method: 'DELETE'
        })
    },
    
    // Federation Nodes
    federation: {
        list: () => API.request('/api/v1/admin/federation/nodes'),
        get: (id) => API.request(`/api/v1/admin/federation/nodes/${id}`),
        create: (data) => API.request('/api/v1/admin/federation/nodes', {
            method: 'POST',
            body: JSON.stringify(data)
        }),
        update: (id, data) => API.request(`/api/v1/admin/federation/nodes/${id}`, {
            method: 'PUT',
            body: JSON.stringify(data)
        }),
        delete: (id) => API.request(`/api/v1/admin/federation/nodes/${id}`, {
            method: 'DELETE'
        })
    },
    
    // Users
    users: {
        list: (params = {}) => {
            const query = new URLSearchParams(params);
            return API.request(`/api/v1/admin/users?${query}`);
        },
        get: (id) => API.request(`/api/v1/admin/users/${id}`),
        create: (data) => API.request('/api/v1/admin/users', {
            method: 'POST',
            body: JSON.stringify(data)
        }),
        update: (id, data) => API.request(`/api/v1/admin/users/${id}`, {
            method: 'PUT',
            body: JSON.stringify(data)
        }),
        deactivate: (id) => API.request(`/api/v1/admin/users/${id}/deactivate`, {
            method: 'POST'
        }),
        reactivate: (id) => API.request(`/api/v1/admin/users/${id}/reactivate`, {
            method: 'POST'
        }),
        delete: (id) => API.request(`/api/v1/admin/users/${id}`, {
            method: 'DELETE'
        }),
        resendInvitation: (id) => API.request(`/api/v1/admin/users/${id}/resend-invitation`, {
            method: 'POST'
        })
    },
    
    // Activity Logs
    activity: {
        list: (params = {}) => {
            const query = new URLSearchParams(params);
            return API.request(`/api/v1/admin/activity?${query}`);
        },
        export: (params = {}) => {
            const query = new URLSearchParams(params);
            return API.request(`/api/v1/admin/activity/export?${query}`);
        }
    }
};
```

### Reusable Components (`components.js`)

Common UI components:

```javascript
// Toast notifications
function showToast(message, type = 'success') {
    const container = document.getElementById('toast-container');
    const colors = {
        success: 'bg-success',
        error: 'bg-danger',
        warning: 'bg-warning',
        info: 'bg-info'
    };
    
    const toast = document.createElement('div');
    toast.className = 'toast show';
    toast.setAttribute('role', 'alert');
    toast.innerHTML = `
        <div class="toast-header">
            <span class="badge ${colors[type]} me-2"></span>
            <strong class="me-auto">${type.charAt(0).toUpperCase() + type.slice(1)}</strong>
            <button type="button" class="btn-close" data-bs-dismiss="toast"></button>
        </div>
        <div class="toast-body">${escapeHtml(message)}</div>
    `;
    
    container.appendChild(toast);
    setTimeout(() => toast.remove(), 5000);
}

// Confirmation modal
function confirmAction(title, message, onConfirm) {
    const modal = document.getElementById('confirm-modal');
    modal.querySelector('.modal-title').textContent = title;
    modal.querySelector('.modal-body').textContent = message;
    
    const confirmBtn = modal.querySelector('#confirm-action');
    confirmBtn.onclick = () => {
        onConfirm();
        bootstrap.Modal.getInstance(modal).hide();
    };
    
    new bootstrap.Modal(modal).show();
}

// HTML escaping
function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

// Date formatting
function formatDate(dateString) {
    const date = new Date(dateString);
    return date.toLocaleDateString('en-US', {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit'
    });
}

// Copy to clipboard
async function copyToClipboard(text) {
    try {
        await navigator.clipboard.writeText(text);
        showToast('Copied to clipboard', 'success');
    } catch (err) {
        showToast('Failed to copy', 'error');
    }
}

// Loading indicator
function setLoading(element, loading) {
    if (loading) {
        element.disabled = true;
        element.dataset.originalText = element.innerHTML;
        element.innerHTML = '<span class="spinner-border spinner-border-sm me-2"></span>Loading...';
    } else {
        element.disabled = false;
        element.innerHTML = element.dataset.originalText;
    }
}
```

### Page-Specific JavaScript Pattern

Each page has its own JS file following this structure:

```javascript
// events.js example
(function() {
    'use strict';
    
    let currentPage = null;
    let filters = {
        status: '',
        search: '',
        startDate: '',
        endDate: ''
    };
    
    // Initialize on page load
    document.addEventListener('DOMContentLoaded', init);
    
    function init() {
        loadEvents();
        setupEventListeners();
    }
    
    function setupEventListeners() {
        // Filter changes
        document.getElementById('status-filter').addEventListener('change', (e) => {
            filters.status = e.target.value;
            loadEvents();
        });
        
        // Search
        document.getElementById('search-input').addEventListener('input', debounce((e) => {
            filters.search = e.target.value;
            loadEvents();
        }, 300));
    }
    
    async function loadEvents() {
        try {
            const tbody = document.getElementById('events-table');
            tbody.innerHTML = '<tr><td colspan="4" class="text-center"><div class="spinner-border"></div></td></tr>';
            
            const params = {
                ...filters,
                limit: 50
            };
            if (currentPage) params.after = currentPage;
            
            const data = await API.events.list(params);
            renderEvents(data.items);
            updatePagination(data.next_cursor);
        } catch (error) {
            showToast(error.message, 'error');
        }
    }
    
    function renderEvents(events) {
        const tbody = document.getElementById('events-table');
        if (events.length === 0) {
            tbody.innerHTML = '<tr><td colspan="4" class="text-center text-muted">No events found</td></tr>';
            return;
        }
        
        tbody.innerHTML = events.map(event => `
            <tr>
                <td><a href="/admin/events/${event.id}">${escapeHtml(event.name)}</a></td>
                <td>${formatDate(event.start_date)}</td>
                <td><span class="badge bg-${getStatusColor(event.lifecycle_state)}">${event.lifecycle_state}</span></td>
                <td>
                    <div class="btn-list">
                        <a href="/admin/events/${event.id}" class="btn btn-sm">Edit</a>
                        <button onclick="deleteEvent('${event.id}')" class="btn btn-sm btn-ghost-danger">Delete</button>
                    </div>
                </td>
            </tr>
        `).join('');
    }
    
    function getStatusColor(status) {
        const colors = {
            published: 'success',
            pending: 'warning',
            cancelled: 'danger',
            draft: 'secondary'
        };
        return colors[status] || 'secondary';
    }
    
    // Utility: debounce for search
    function debounce(func, wait) {
        let timeout;
        return function executedFunction(...args) {
            const later = () => {
                clearTimeout(timeout);
                func(...args);
            };
            clearTimeout(timeout);
            timeout = setTimeout(later, wait);
        };
    }
    
    // Expose functions needed by inline onclick handlers
    window.deleteEvent = async function(id) {
        confirmAction(
            'Delete Event',
            'Are you sure you want to delete this event?',
            async () => {
                try {
                    await API.events.delete(id);
                    showToast('Event deleted successfully', 'success');
                    loadEvents();
                } catch (error) {
                    showToast(error.message, 'error');
                }
            }
        );
    };
})();
```

---

## Mobile Responsiveness

### Responsive Design Principles

1. **Mobile-First:** All layouts start mobile and scale up
2. **Touch-Friendly:** Buttons/links minimum 44x44px touch target
3. **Readable:** Text size scales appropriately (min 14px on mobile)
4. **Navigation:** Collapses to hamburger menu on mobile
5. **Tables:** Horizontal scroll or card layout on mobile

### Responsive Utilities

**Hide/Show by Breakpoint:**
```html
<!-- Show only on mobile -->
<div class="d-block d-md-none">Mobile content</div>

<!-- Show only on desktop -->
<div class="d-none d-md-block">Desktop content</div>

<!-- Responsive columns -->
<div class="col-12 col-md-6 col-lg-4">
    <!-- Full width mobile, half tablet, third desktop -->
</div>
```

**Responsive Tables:**
```html
<!-- Option 1: Horizontal scroll -->
<div class="table-responsive">
    <table class="table">
        <!-- Table content -->
    </table>
</div>

<!-- Option 2: Card layout on mobile -->
<div class="row row-cards">
    <div class="col-12 col-md-6 col-lg-4">
        <div class="card">
            <!-- Event details -->
        </div>
    </div>
</div>
```

**Responsive Navigation:**
- Navbar collapses to hamburger on `< md` breakpoint (768px)
- User dropdown always visible
- Nav items stack vertically in collapsed menu

### Testing Responsive Design

**Browser DevTools:**
1. Open Chrome DevTools (F12)
2. Click device toolbar icon (Ctrl+Shift+M)
3. Test breakpoints:
   - Mobile: 375px (iPhone SE)
   - Tablet: 768px (iPad)
   - Desktop: 1920px

**Real Device Testing:**
- Test on actual iOS/Android devices
- Verify touch targets are large enough
- Check that modals work properly on mobile

---

## Implementation Checklist

### Phase 1: Foundation
- [ ] Download and vendor Tabler CSS/JS files
- [ ] Create `custom.css` with project-specific overrides
- [ ] Create `api.js` with API client wrapper
- [ ] Create `components.js` with reusable UI functions
- [ ] Create `base.html` template with navigation
- [ ] Test authentication flow (login -> dashboard)

### Phase 2: Dashboard
- [ ] Update `dashboard.html` with Tabler layout
- [ ] Create `dashboard.js` to fetch stats from API
- [ ] Implement stat cards (pending reviews, total events)
- [ ] Add recent activity feed (optional)
- [ ] Test mobile responsiveness

### Phase 3: Events Management
- [ ] Create `events_list.html` with table and filters
- [ ] Create `events.js` for list pagination/filtering
- [ ] Create `event_edit.html` with form
- [ ] Create `event-edit.js` for form handling
- [ ] Implement occurrence management (add/edit/remove)
- [ ] Add delete confirmation modal
- [ ] Test CRUD operations end-to-end

### Phase 4: Duplicates Review
- [ ] Create `duplicates.html` with side-by-side layout
- [ ] Create `duplicates.js` for comparison/merge
- [ ] Implement merge confirmation flow
- [ ] Add field selection UI (optional)
- [ ] Test merge operation

### Phase 5: API Keys Management
- [ ] Create `api_keys.html` with table
- [ ] Create `api-keys.js` for CRUD operations
- [ ] Add create key modal
- [ ] Implement copy-to-clipboard for new keys
- [ ] Add revoke confirmation
- [ ] Test key lifecycle

### Phase 6: Federation (Optional)
- [ ] Create `federation.html` with node list
- [ ] Create `federation.js` for node management
- [ ] Add node status indicators
- [ ] Implement test connection feature
- [ ] Test node CRUD operations

### Phase 7: User Management
- [ ] Create `users.html` with user list table
- [ ] Create `users.js` for user CRUD operations
- [ ] Implement create user modal with role selection
- [ ] Add edit user modal
- [ ] Implement deactivate/reactivate functionality
- [ ] Add delete user confirmation with soft delete
- [ ] Implement resend invitation feature
- [ ] Create `activity.html` for activity logs
- [ ] Create `activity.js` with filtering and export
- [ ] Create `accept-invitation.html` for invitation acceptance
- [ ] Create `accept-invitation.js` for password setting
- [ ] Test invitation email flow (7-day expiration)
- [ ] Test user lifecycle (create → invite → accept → login)
- [ ] Test role-based permissions

### Phase 8: Handler Wiring
- [ ] Create handler functions in `admin_html.go`
- [ ] Update `router.go` to replace `AdminHTMLPlaceholder`
- [ ] Wire all routes with proper middleware (cookie auth, CSRF)
- [ ] Test all routes return correct templates
- [ ] Add user management routes (`/admin/users`, `/admin/accept-invitation`)
- [ ] Add activity log routes (`/admin/activity`)

### Phase 9: Testing & Polish
- [ ] Test all pages on mobile viewports
- [ ] Test all CRUD operations
- [ ] Test user management flows (create → invite → accept → login)
- [ ] Verify error handling (network errors, validation errors)
- [ ] Check accessibility (keyboard navigation, screen readers)
- [ ] Performance: ensure pages load < 1s on staging
- [ ] Cross-browser testing (Chrome, Firefox, Safari)

---

## Reference Links

- **Tabler Documentation:** https://docs.tabler.io
- **Tabler Components:** https://preview.tabler.io
- **Bootstrap 5 Docs:** https://getbootstrap.com/docs/5.3/
- **Backend API Endpoints:** See `specs/001-sel-backend/contracts/openapi.yaml`
- **Design Mockups:** (Add link to Figma/design files if available)

---

## Questions & Support

**For questions or clarifications:**
- Check existing beads: `bd list --status open`
- Review backend API contracts: `specs/001-sel-backend/contracts/openapi.yaml`
- Test backend endpoints directly: `curl -H "Authorization: Bearer $TOKEN" https://staging.toronto.togather.foundation/api/v1/admin/events`

**Common Issues:**
- **CSRF token errors:** Ensure CSRF middleware is properly configured
- **Auth cookies not working:** Check `SameSite` and `Secure` cookie attributes
- **Mobile layout broken:** Verify `viewport` meta tag in `<head>`
- **API calls failing:** Check CORS config and cookie credentials

---

**This guide will be updated as implementation progresses.**
