# Review Queue UI

Admin UI for the event review queue. Admins can review events with data quality issues, compare original vs normalized data, and approve/reject/fix events.

**Architecture doc:** `docs/architecture/event-review-workflow.md`

## API Reference

All endpoints in `internal/api/handlers/admin_review_queue.go`. All wrapped with `jwtAuth(adminRateLimit(middleware.AdminRequestSize(...)))`.

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

## Page Layout

Single-page design: list view with inline detail expansion. No separate detail page. Route: `/admin/review-queue`.

```
+------------------------------------------------------------------+
| [Dashboard] [Events] [Users] [Duplicates] [Review] [API Keys]    |
+------------------------------------------------------------------+
| Review Queue                                                      |
|                                                                   |
| [Pending (3)] [Approved] [Rejected]         [status filter tabs] |
+------------------------------------------------------------------+
| Event Name       | Start Time      | Warning    | Created | Act. |
|------------------|-----------------|------------|---------|------|
| Late Night Jazz  | Mar 31, 11 PM   | High Conf. | 2h ago  | ...  |
|   [Expanded detail card when clicked]                            |
|   +----------------------------------------------------------+   |
|   | Original               | Corrected                       |   |
|   | endDate: Mar 31 02:00  | endDate: Apr 1 02:00 (green)    |   |
|   | Warning: timezone_likely - End at 02:00, duration 3h      |   |
|   | [Approve] [Fix Dates...] [Reject...]                      |   |
|   +----------------------------------------------------------+   |
| Open Mic Night   | Apr 2, 10 PM    | Low Conf.  | 1d ago  | ...  |
+------------------------------------------------------------------+
| Showing 3 items                              [< Prev] [Next >]   |
+------------------------------------------------------------------+
```

### Status Filter Tabs

Three Tabler nav-tabs: **Pending** (default, with count badge), **Approved**, **Rejected**.

### List Table Columns

| Column | Content |
|--------|---------|
| Event Name | `eventName` with link to `/admin/events/{eventId}`, truncated at ~40 chars |
| Start Time | `eventStartTime` formatted via `formatDate()` from components.js |
| Warning | Warning code badge: green for high confidence, yellow for low |
| Created | `createdAt` relative time, e.g., "2h ago" |
| Actions | Expand chevron |

### Inline Detail Card

Expands below the clicked row:

1. **Side-by-side comparison**: original (dates red if changed) vs normalized (dates green if changed)
2. **Changes summary**: field name, original → corrected value, reason
3. **Warning details**: full message
4. **Actions**:
   - **Approve** (`btn-success`): quick approve with optional notes textarea
   - **Fix Dates** (`btn-primary`): inline datetime-local inputs, pre-filled
   - **Reject** (`btn-outline-danger`): modal requiring reason text

### Reject Modal

```
+----------------------------------+
| Reject Event                     |
+----------------------------------+
| Reason (required):               |
| [textarea                       ]|
| [Cancel]            [Reject]     |
+----------------------------------+
```

### Fix Dates Form

```
+-------------------------------------------------------+
| Correct Dates                                          |
| Start: [datetime-local input, pre-filled]             |
| End:   [datetime-local input, pre-filled]             |
| Notes: [textarea, optional]                           |
| [Cancel] [Apply Fix]                                  |
+-------------------------------------------------------+
```

## CSS

No new CSS file needed. Tabler classes used:

- **Diff highlighting**: `.bg-success-lt` (corrected values), `.bg-danger-lt` (original changed values)
- **Warning badges**: `.badge.bg-success` (high confidence), `.badge.bg-warning` (low confidence)
- **Status badges**: `.badge.bg-warning` (pending), `.badge.bg-success` (approved), `.badge.bg-danger` (rejected)
- **Detail card**: `.card` with `.card-body` inside an expanded `<tr>` with full colspan
- **Comparison columns**: `.row > .col-md-6` for side-by-side on desktop, stacked on mobile

## Implementation Notes

- All click handlers use `data-action` attributes (CSP compliance)
- Template files are auto-discovered via Go's `embed.FS` in `web/embed.go`
- The `_footer.html` template already loads `bootstrap.bundle.min.js`, `tabler.min.js`, `api.js`, and `components.js`
- The `reviewQueue` API namespace in `api.js` exposes `list`, `get`, `approve`, `reject`, `fix`

## Testing

```bash
AGENT=1 make build
# Start dev server, verify /admin/review-queue renders
make e2e
```

Verify: no console errors, no CSP violations, empty state displays, approve/reject/fix flows work if queue has entries.
