# Review Queue UI Design

## Overview

Admin UI for the event review queue where admins can review events with data quality issues (e.g., reversed dates), see original vs normalized data, and approve/reject/fix events.

**Parent bead:** srv-349
**Architecture doc:** `docs/architecture/event-review-workflow.md`

## Existing API (fully implemented)

All endpoints in `internal/api/handlers/admin_review_queue.go` (687 lines).
All wrapped with: `jwtAuth(adminRateLimit(middleware.AdminRequestSize(...)))`.

### GET /api/v1/admin/review-queue

List review queue entries with cursor pagination.

**Query params:** `status` (default "pending"), `limit` (1-100, default 50), `cursor` (int)

**Response:**
```json
{
  "items": [{
    "id": 1,
    "eventId": "01ABCDEF...",
    "eventName": "Late Night Jazz",
    "eventStartTime": "2026-03-31T23:00:00Z",
    "eventEndTime": "2026-04-01T02:00:00Z",
    "warnings": [{"field": "endDate", "message": "...", "code": "reversed_dates_timezone_likely"}],
    "status": "pending",
    "createdAt": "2026-02-07T14:00:00Z",
    "reviewedBy": null,
    "reviewedAt": null
  }],
  "next_cursor": "42"
}
```

### GET /api/v1/admin/review-queue/{id}

Detail view with original vs corrected data.

**Response:**
```json
{
  "id": 1,
  "eventId": "01ABCDEF...",
  "status": "pending",
  "warnings": [{"field": "endDate", "message": "...", "code": "reversed_dates_timezone_likely"}],
  "original": {"name": "...", "startDate": "...", "endDate": "...", "location": {}},
  "normalized": {"name": "...", "startDate": "...", "endDate": "...", "location": {}},
  "changes": [{"field": "endDate", "original": "2026-03-31T02:00:00Z", "corrected": "2026-04-01T02:00:00Z", "reason": "Added 24 hours to fix reversed dates"}],
  "createdAt": "...",
  "reviewedBy": null,
  "reviewedAt": null,
  "reviewNotes": null,
  "rejectionReason": null
}
```

### POST /api/v1/admin/review-queue/{id}/approve

**Request:** `{"notes": "optional"}`
**Response:** Full review detail object.

### POST /api/v1/admin/review-queue/{id}/reject

**Request:** `{"reason": "required"}`
**Response:** Full review detail object.

### POST /api/v1/admin/review-queue/{id}/fix

**Request:** `{"corrections": {"startDate": "...", "endDate": "..."}, "notes": "optional"}`
**Response:** Full review detail object.

## Warning Codes

| Code | Confidence | Badge Color | Description |
|------|-----------|-------------|-------------|
| `reversed_dates_timezone_likely` | High | `bg-success` (green) | End 0-4 AM, duration < 7h. Likely overnight timezone error. |
| `reversed_dates_corrected_needs_review` | Low | `bg-warning` (yellow) | Reversed dates but doesn't match high-confidence pattern. |

## UI Design

### Page Layout: `/admin/review-queue`

Single-page design with list view and inline detail expansion. No separate detail page.

```
+------------------------------------------------------------------+
| [Dashboard] [Events] [Users] [Duplicates] [Review] [API Keys]    |
+------------------------------------------------------------------+
| Review Queue                                                      |
| Review events with data quality issues                            |
|                                                                    |
| [Pending (3)] [Approved] [Rejected]         [status filter tabs]  |
+------------------------------------------------------------------+
| Event Name       | Start Time      | Warning    | Created | Act. |
|------------------|-----------------|------------|---------|------|
| Late Night Jazz  | Mar 31, 11 PM   | High Conf. | 2h ago  | ...  |
|   [Expanded detail card when clicked]                             |
|   +-----------------------------------------------------------+  |
|   | Original               | Corrected                        |  |
|   | endDate: Mar 31 02:00  | endDate: Apr 1 02:00 (green)     |  |
|   | Warning: timezone_likely - End at 02:00, duration 3h       |  |
|   | [Approve] [Fix Dates...] [Reject...]                       |  |
|   +-----------------------------------------------------------+  |
| Open Mic Night   | Apr 2, 10 PM    | Low Conf.  | 1d ago  | ...  |
| Friday Salsa     | Apr 5, 9 PM     | Low Conf.  | 3d ago  | ...  |
+------------------------------------------------------------------+
| Showing 3 items                              [< Prev] [Next >]   |
+------------------------------------------------------------------+
```

### Status Filter Tabs

Three tabs at the top, styled as Tabler nav-tabs:
- **Pending** (default) - with count badge
- **Approved** - historical approved reviews
- **Rejected** - historical rejections

### List Table Columns

| Column | Content | Notes |
|--------|---------|-------|
| Event Name | `eventName` with link to `/admin/events/{eventId}` | Truncate at ~40 chars |
| Start Time | `eventStartTime` formatted | `formatDate()` from components.js |
| Warning | Warning code badge | Green for high confidence, yellow for low |
| Created | `createdAt` relative | e.g., "2h ago", "1d ago" |
| Actions | Quick action buttons | Expand chevron |

### Inline Detail Card

When a row is clicked, a detail card expands below it showing:

1. **Side-by-side comparison** (2-column layout):
   - Left: Original payload (dates highlighted red if changed)
   - Right: Normalized/corrected payload (dates highlighted green if changed)

2. **Changes summary**: Each change from the `changes` array displayed as:
   - Field name, original value -> corrected value, reason

3. **Warning details**: Full message from warnings array

4. **Action buttons**:
   - **Approve** (green, `btn-success`): Quick approve with optional notes textarea
   - **Fix Dates** (blue, `btn-primary`): Opens inline form with datetime-local inputs
   - **Reject** (red outline, `btn-outline-danger`): Opens modal requiring reason text

### Reject Modal

```
+----------------------------------+
| Reject Event                      |
+----------------------------------+
| Reason (required):               |
| [textarea                        ]|
|                                   |
| [Cancel]            [Reject]     |
+----------------------------------+
```

### Fix Dates Form (inline, replaces action buttons)

```
+-------------------------------------------------------+
| Correct Dates                                          |
| Start: [datetime-local input, pre-filled]              |
| End:   [datetime-local input, pre-filled]              |
| Notes: [textarea, optional]                            |
| [Cancel] [Apply Fix]                                   |
+-------------------------------------------------------+
```

## Files to Create/Modify

### New Files

1. **`web/admin/templates/review_queue.html`** - Page template
   - Follow standard template pattern (see `duplicates.html` for closest reference)
   - Include `_head_meta.html`, `_header.html`, `_footer.html`
   - `ActivePage: "review-queue"`
   - Loading state, empty state, table, detail card, reject modal

2. **`web/admin/static/js/review-queue.js`** - Page logic
   - IIFE pattern matching other pages
   - State: `{ entries: [], currentFilter: 'pending', expandedId: null, cursor: null }`
   - Functions: `init()`, `loadEntries()`, `renderTable()`, `expandDetail()`, `collapseDetail()`, `approve()`, `reject()`, `fixDates()`, `renderPagination()`
   - Event delegation via `data-action` attributes (CSP-compliant)
   - Use shared utilities: `showToast()`, `confirmAction()`, `escapeHtml()`, `formatDate()`, `setLoading()`, `renderLoadingState()`, `renderEmptyState()`

### Modified Files

3. **`web/admin/static/js/api.js`** - Add `reviewQueue` namespace:
   ```javascript
   reviewQueue: {
       list: (params = {}) => {
           const query = new URLSearchParams(params);
           return API.request(`/api/v1/admin/review-queue?${query}`);
       },
       get: (id) => API.request(`/api/v1/admin/review-queue/${id}`),
       approve: (id, data = {}) => API.request(`/api/v1/admin/review-queue/${id}/approve`, {
           method: 'POST', body: JSON.stringify(data)
       }),
       reject: (id, data) => API.request(`/api/v1/admin/review-queue/${id}/reject`, {
           method: 'POST', body: JSON.stringify(data)
       }),
       fix: (id, data) => API.request(`/api/v1/admin/review-queue/${id}/fix`, {
           method: 'POST', body: JSON.stringify(data)
       })
   }
   ```

4. **`web/admin/templates/_header.html`** - Add nav item after "Duplicates":
   ```html
   <li class="nav-item{{ if eq .ActivePage "review-queue" }} active{{ end }}">
       <a class="nav-link" href="/admin/review-queue">Review</a>
   </li>
   ```

5. **`internal/api/handlers/admin_html.go`** - Add `ServeReviewQueue` method:
   ```go
   func (h *AdminHTMLHandler) ServeReviewQueue(w http.ResponseWriter, r *http.Request) {
       // Same pattern as ServeDuplicates
       // ActivePage: "review-queue"
       // Template: "review_queue.html"
   }
   ```

6. **`internal/api/router.go`** - Add HTML route after duplicates route (~line 409):
   ```go
   mux.Handle("/admin/review-queue", csrfMiddleware(adminCookieAuth(http.HandlerFunc(adminHTMLHandler.ServeReviewQueue))))
   ```

## CSS

No new CSS file needed. Use existing Tabler classes and `custom.css`. Specific classes to use:

- **Diff highlighting**: `.bg-success-lt` (green tint) for corrected values, `.bg-danger-lt` (red tint) for original changed values
- **Warning badges**: `.badge.bg-success` (high confidence), `.badge.bg-warning` (low confidence)
- **Status badges**: `.badge.bg-warning` (pending), `.badge.bg-success` (approved), `.badge.bg-danger` (rejected)
- **Detail card**: `.card` with `.card-body` inside an expanded table row (`<tr>` with full colspan)
- **Comparison columns**: `.row > .col-md-6` for side-by-side on desktop, stacked on mobile

## Implementation Notes

- All existing patterns are in the codebase already - follow them exactly
- The API is fully implemented and tested; this is purely frontend work + Go handler wiring
- Use `data-action` attributes for all click handlers (CSP compliance)
- The `_footer.html` template already loads `bootstrap.bundle.min.js`, `tabler.min.js`, `api.js`, and `components.js`
- Template files are embedded via Go's `embed.FS` in `web/embed.go` - new templates are auto-discovered

## Testing

After implementation:
1. `AGENT=1 make build` - verify Go compiles
2. Start dev server, verify `/admin/review-queue` renders
3. Run E2E tests: `uvx --from playwright --with playwright python tests/e2e/test_admin_ui_python.py`
4. Verify no console errors, CSP violations
5. Test with empty queue (empty state displays)
6. Test approve/reject/fix flows if review queue has entries
