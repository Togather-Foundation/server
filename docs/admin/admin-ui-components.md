# Admin UI Components

**Version:** 1.0  
**Created:** 2026-02-04  
**Framework:** Tabler v1.4.0 (Bootstrap 5)

This is the component and implementation reference for the admin UI. It covers the reusable component library, non-user-management page templates, JavaScript architecture, mobile responsiveness, and the implementation checklist.

**Related documents:**
- **[Admin UI Overview](admin-ui-overview.md)** — Design system, file structure, core concepts
- **[User Management](user-management.md)** — User lifecycle, forms, invitation flow

---

## Table of Contents

1. [Component Library](#component-library)
2. [Page Templates](#page-templates)
3. [JavaScript Architecture](#javascript-architecture)
4. [Mobile Responsiveness](#mobile-responsiveness)
5. [Implementation Checklist](#implementation-checklist)

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

> **Note:** User Management page templates are documented in [User Management](user-management.md).

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
