# Review Queue Feature Documentation

## Overview

The **Review Queue** is an admin UI feature that allows administrators to review, approve, reject, or manually fix events with data quality issues. The most common issue handled is **reversed dates**, where an event's `endDate` appears chronologically before its `startDate` (typically caused by timezone conversion errors on overnight events).

Instead of rejecting these events outright, the system:
1. Accepts them with warnings (HTTP 202 Accepted)
2. Auto-corrects obvious errors using heuristics
3. Queues ambiguous cases for admin review
4. Allows event providers to fix and resubmit

This feature is part of the **Event Review Workflow** documented in `docs/architecture/event-review-workflow.md`.

---

## Architecture

### High-Level Data Flow

```
Event Submission
    ↓
Normalization (auto-fix reversed dates)
    ↓
Validation (detect what changed)
    ↓
Has Warnings? → No → Store as Published (201 Created)
    ↓
   Yes
    ↓
Store as Pending Review (202 Accepted)
    ↓
Review Queue Entry Created
    ↓
Admin Reviews in UI
    ↓
Approve / Reject / Fix
    ↓
Event Published or Deleted
```

### Components

The review queue feature spans four layers:

#### 1. Backend Services

**Ingestion Logic** (`internal/domain/events/ingest.go`)
- Coordinates normalization → validation → storage
- Checks for existing reviews (pending or rejected)
- Creates review queue entries when warnings are detected
- Handles resubmissions and deduplication

**Normalization** (`internal/domain/events/normalize.go`)
- Always corrects reversed dates by adding 24 hours to `endDate`
- Required for database CHECK constraint: `end_time >= start_time`
- Preserves original payload for comparison

**Validation** (`internal/domain/events/validation.go`)
- Compares original vs normalized input to detect changes
- Generates warnings with confidence levels
- Returns structured warning objects with codes, messages, and affected fields

#### 2. Database Layer

**Table: `event_review_queue`**

Stores review entries with full context for admin inspection:

```sql
CREATE TABLE event_review_queue (
  id SERIAL PRIMARY KEY,
  event_id TEXT UNIQUE NOT NULL,
  
  -- Payloads for comparison
  original_payload JSONB NOT NULL,
  normalized_payload JSONB NOT NULL,
  warnings JSONB NOT NULL,
  
  -- Deduplication keys
  source_id TEXT,
  source_external_id TEXT,
  dedup_hash TEXT,
  
  -- Event timing (for expiry)
  event_start_time TIMESTAMPTZ NOT NULL,
  event_end_time TIMESTAMPTZ,
  
  -- Review workflow
  status TEXT NOT NULL DEFAULT 'pending',
  reviewed_by TEXT,
  reviewed_at TIMESTAMPTZ,
  review_notes TEXT,
  rejection_reason TEXT,
  
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

**Table: `events`**

Events requiring review are stored with:
- `lifecycle_state = 'pending_review'` (not `published` or `draft`)
- All fields contain **corrected data** (post-normalization)
- Query: `WHERE lifecycle_state = 'published'` excludes pending reviews

**Queries:** See `internal/storage/postgres/queries/event_review_queue.sql`
- `ListReviewQueue` - Paginated list with status filter
- `GetReviewQueueEntry` - Full detail for single entry
- `ApproveReview` - Mark approved and publish event
- `RejectReview` - Mark rejected and delete event
- `FindReviewByDedup` - Check for existing reviews
- Cleanup queries for expiring old reviews

#### 3. API Endpoints

All endpoints in `internal/api/handlers/admin_review_queue.go` (694 lines).

**GET `/api/v1/admin/review-queue`** - List reviews
- Query params: `status` (pending/approved/rejected), `limit` (1-100, default 50), `cursor`
- Returns: Paginated list of review entries with warnings and metadata
- Example: [admin_review_queue.go:75](file:///home/ryankelln/Documents/Projects/Art/togather/server/internal/api/handlers/admin_review_queue.go:75)

**GET `/api/v1/admin/review-queue/:id`** - Get detail
- Returns: Full entry with original vs normalized payloads, changes array
- Example: [admin_review_queue.go:150](file:///home/ryankelln/Documents/Projects/Art/togather/server/internal/api/handlers/admin_review_queue.go:150)

**POST `/api/v1/admin/review-queue/:id/approve`** - Approve review
- Body: `{"notes": "optional"}`
- Action: Sets `lifecycle_state='published'`, marks review approved
- Example: [admin_review_queue.go:191](file:///home/ryankelln/Documents/Projects/Art/togather/server/internal/api/handlers/admin_review_queue.go:191)

**POST `/api/v1/admin/review-queue/:id/reject`** - Reject review
- Body: `{"reason": "required"}`
- Action: Sets `lifecycle_state='deleted'`, marks review rejected
- Example: [admin_review_queue.go:296](file:///home/ryankelln/Documents/Projects/Art/togather/server/internal/api/handlers/admin_review_queue.go:296)

**POST `/api/v1/admin/review-queue/:id/fix`** - Apply manual corrections
- Body: `{"corrections": {"startDate": "...", "endDate": "..."}, "notes": "optional"}`
- Action: **NOTE:** Currently incomplete (see Future Work section)
- Example: [admin_review_queue.go:405](file:///home/ryankelln/Documents/Projects/Art/togather/server/internal/api/handlers/admin_review_queue.go:405)

#### 4. Frontend UI

**Page:** `/admin/review-queue` ([review_queue.html](file:///home/ryankelln/Documents/Projects/Art/togather/server/web/admin/templates/review_queue.html))
- Single-page design with list view and inline detail expansion
- Status filter tabs: Pending / Approved / Rejected
- Table columns: Event Name, Start Time, Warning Badge, Created, Actions
- Inline detail card expands below row when clicked

**JavaScript:** `web/admin/static/js/review-queue.js` (845 lines)
- IIFE pattern, no global variables
- State management: `entries`, `currentFilter`, `expandedId`, `cursor`
- Event delegation via `data-action` attributes (CSP-compliant)
- API calls via shared `API.reviewQueue.*` methods
- See [review-queue.js:1](file:///home/ryankelln/Documents/Projects/Art/togather/server/web/admin/static/js/review-queue.js:1)

**API Client:** Added to `web/admin/static/js/api.js`
```javascript
reviewQueue: {
    list: (params) => API.request('/api/v1/admin/review-queue?' + new URLSearchParams(params)),
    get: (id) => API.request(`/api/v1/admin/review-queue/${id}`),
    approve: (id, data) => API.request(`/api/v1/admin/review-queue/${id}/approve`, { method: 'POST', body: JSON.stringify(data) }),
    reject: (id, data) => API.request(`/api/v1/admin/review-queue/${id}/reject`, { method: 'POST', body: JSON.stringify(data) }),
    fix: (id, data) => API.request(`/api/v1/admin/review-queue/${id}/fix`, { method: 'POST', body: JSON.stringify(data) })
}
```

---

## Warning Codes

Validation generates two machine-readable warning codes to classify correction confidence:

### `reversed_dates_timezone_likely` (High Confidence)

**Criteria:**
- End time is 0-4 AM (local or UTC)
- Duration after correction is < 7 hours
- Pattern matches typical overnight events

**Badge:** Green (`bg-success`)

**Example:**
```json
{
  "field": "endDate",
  "code": "reversed_dates_timezone_likely",
  "message": "endDate was 21h before startDate and ends at 02:00 - likely timezone error. Auto-corrected by adding 24 hours.",
  "confidence": "high"
}
```

**Use Case:** Bulk-approve workflow for trusted sources.

### `reversed_dates_corrected_needs_review` (Low Confidence)

**Criteria:**
- Reversed dates detected and corrected
- Does NOT match high-confidence pattern (end time not 0-4 AM, or duration > 7 hours)

**Badge:** Yellow (`bg-warning`)

**Example:**
```json
{
  "field": "endDate",
  "code": "reversed_dates_corrected_needs_review",
  "message": "Dates were reversed. Auto-corrected but unusual duration (11h). Please verify.",
  "confidence": "low"
}
```

**Use Case:** Requires manual admin verification.

---

## Frontend Components

### Status Filter Tabs

Three tabs styled as Tabler `nav-tabs`:

| Tab | Status Filter | Badge | Description |
|-----|--------------|-------|-------------|
| **Pending** | `pending` | Count badge (e.g., "3") | Default view, shows unreviewed entries |
| **Approved** | `approved` | None | Historical approved reviews |
| **Rejected** | `rejected` | None | Historical rejections |

Implementation: [review_queue.html:33](file:///home/ryankelln/Documents/Projects/Art/togather/server/web/admin/templates/review_queue.html:33)

### List Table

Columns:
1. **Event Name** - Truncated, links to `/admin/events/{eventId}`
2. **Start Time** - Formatted with `formatDate()` (e.g., "Mar 31, 11 PM")
3. **Warning** - Badge colored by confidence (green/yellow)
4. **Created** - Relative time (e.g., "2h ago", "1d ago")
5. **Actions** - Chevron button to expand detail

Implementation: [review-queue.js:155](file:///home/ryankelln/Documents/Projects/Art/togather/server/web/admin/static/js/review-queue.js:155)

### Detail View (Expandable Row)

When a row is clicked, a detail card expands below showing:

#### 1. Warnings Section
Displays all warnings with color-coded badges and full messages.

#### 2. Changes Summary
Shows what was auto-corrected:
```
Field: endDate
From: 2025-03-31T02:00:00Z
To:   2025-04-01T02:00:00Z (green highlight)
Reason: Added 24 hours to fix reversed dates
```

#### 3. Side-by-Side Comparison
Two-column layout (stacked on mobile):
- **Left:** Original payload (red highlight on changed fields)
- **Right:** Normalized payload (green highlight on changed fields)

Fields displayed: Name, Start Date, End Date, Location

#### 4. Action Buttons (Pending Status Only)

Three action buttons:

**Approve** (Green, `btn-success`)
- Quick approval with optional notes
- Publishes event immediately
- See [review-queue.js:403](file:///home/ryankelln/Documents/Projects/Art/togather/server/web/admin/static/js/review-queue.js:403)

**Fix Dates** (Blue, `btn-primary`)
- Opens inline form below buttons
- Pre-filled with current dates
- datetime-local inputs for precise editing
- See [review-queue.js:511](file:///home/ryankelln/Documents/Projects/Art/togather/server/web/admin/static/js/review-queue.js:511)

**Reject** (Red Outline, `btn-outline-danger`)
- Opens Bootstrap modal
- Requires rejection reason (validated)
- Deletes event permanently
- See [review-queue.js:432](file:///home/ryankelln/Documents/Projects/Art/togather/server/web/admin/static/js/review-queue.js:432)

Implementation: [review-queue.js:248](file:///home/ryankelln/Documents/Projects/Art/togather/server/web/admin/static/js/review-queue.js:248)

### Reject Modal

Bootstrap modal requiring rejection reason:

```html
<div class="modal fade" id="reject-modal">
  <textarea id="reject-reason" required>...</textarea>
  <button id="confirm-reject-btn">Reject</button>
</div>
```

- Validates reason is non-empty
- Shows error message if validation fails
- Closes modal on success
- Implementation: [review_queue.html:107](file:///home/ryankelln/Documents/Projects/Art/togather/server/web/admin/templates/review_queue.html:107)

### Fix Dates Form

Inline form that replaces action buttons:

```html
<div id="fix-form-{id}">
  <input type="datetime-local" id="fix-start-{id}" value="...">
  <input type="datetime-local" id="fix-end-{id}" value="...">
  <textarea id="fix-notes-{id}">...</textarea>
  <button>Cancel</button>
  <button>Apply Fix</button>
</div>
```

- Pre-fills current event dates
- Validates both dates are provided
- Converts datetime-local to ISO 8601 for API
- Implementation: [review-queue.js:511](file:///home/ryankelln/Documents/Projects/Art/togather/server/web/admin/static/js/review-queue.js:511)

### Pagination

Cursor-based pagination (not offset):
- "Next" button appears when `next_cursor` is present in API response
- Clicking loads next page and scrolls to top
- Smooth scroll for better UX

Implementation: [review-queue.js:678](file:///home/ryankelln/Documents/Projects/Art/togather/server/web/admin/static/js/review-queue.js:678)

---

## Data Flow

### Scenario 1: High-Confidence Auto-Fix

**Input:**
```json
{
  "name": "Late Night Jazz",
  "startDate": "2025-03-31T23:00:00Z",  // 11 PM
  "endDate": "2025-03-31T02:00:00Z"      // 2 AM (WRONG - should be next day)
}
```

**Processing:**
1. **Normalize:** `endDate` → `2025-04-01T02:00:00Z` (add 24h)
2. **Validate:** End hour = 2 AM (0-4), duration = 3h (< 7h) → High confidence
3. **Warning:** `reversed_dates_timezone_likely`
4. **Store:** `lifecycle_state='pending_review'` in `events` table, entry in `event_review_queue`
5. **Response:** HTTP 202 Accepted with warning

**Admin sees in UI:**
- Badge: Green "High Confidence"
- Changes: `endDate: Mar 31 02:00 → Apr 1 02:00`
- Message: "Likely overnight event with timezone error"

**Admin action:** Click "Approve" → Event published

---

### Scenario 2: Low-Confidence Correction

**Input:**
```json
{
  "startDate": "2025-03-31T23:00:00Z",  // 11 PM
  "endDate": "2025-03-31T10:00:00Z"      // 10 AM (WRONG)
}
```

**Processing:**
1. **Normalize:** `endDate` → `2025-04-01T10:00:00Z` (add 24h)
2. **Validate:** End hour = 10 AM (not 0-4), duration = 11h → Low confidence
3. **Warning:** `reversed_dates_corrected_needs_review`
4. **Store:** `lifecycle_state='pending_review'`, queue entry created
5. **Response:** HTTP 202 Accepted with warning

**Admin sees in UI:**
- Badge: Yellow "Low Confidence"
- Changes: `endDate: Mar 31 10:00 → Apr 1 10:00`
- Message: "Unusual duration (11h). Please verify."

**Admin actions:**
- **Option A:** Approve if correct (e.g., all-night festival)
- **Option B:** Click "Fix Dates" and manually correct
- **Option C:** Reject if cannot verify

---

### Scenario 3: Resubmission (Fixed)

**Initial submission:**
```json
{"startDate": "2025-03-31T23:00:00Z", "endDate": "2025-03-31T02:00:00Z"}
```
→ Queued for review (pending)

**Resubmission (Provider fixed it):**
```json
{"startDate": "2025-03-31T23:00:00Z", "endDate": "2025-04-01T02:00:00Z"}
```

**Processing:**
1. **Normalize:** No change needed (dates valid)
2. **Validate:** No warnings
3. **Dedup:** Finds pending review with same `source_id + source_external_id`
4. **Check warnings:** Now empty (fixed!)
5. **Action:** 
   - Update `events.lifecycle_state = 'published'`
   - Update `event_review_queue.status = 'superseded'`
6. **Response:** HTTP 201 Created (or 409 Conflict depending on dedup strategy)

**Result:** Event auto-published, no admin action needed.

---

### Scenario 4: Rejected, Then Resubmission (Still Wrong)

**Initial:**
```json
{"startDate": "2025-03-31T23:00:00Z", "endDate": "2025-03-31T10:00:00Z"}
```
→ Admin reviews → Rejects with reason "Cannot verify correct time"

**Resubmission (Same bad data):**
```json
{"startDate": "2025-03-31T23:00:00Z", "endDate": "2025-03-31T10:00:00Z"}
```

**Processing:**
1. **Normalize + Validate:** Same warnings
2. **Dedup:** Finds rejected review
3. **Check:** Event hasn't passed yet, same warnings detected
4. **Response:** **HTTP 400 Bad Request**
   ```json
   {
     "type": "https://sel.events/problems/previously-rejected",
     "title": "Previously Rejected",
     "detail": "This event was reviewed on 2025-02-07 and rejected: Cannot verify correct time. Please fix the data before resubmitting.",
     "reviewedAt": "2025-02-07T14:30:00Z",
     "reviewedBy": "admin@example.com"
   }
   ```

**Result:** Prevents repeated resubmission of known-bad data. Provider must fix before resubmitting.

---

## Testing

### E2E Tests

**Location:** `tests/e2e/test_admin_ui_python.py`

The existing admin UI E2E tests cover general navigation and authentication but do NOT yet include review queue-specific tests.

**What should be tested:**
- Navigate to `/admin/review-queue` page
- Verify status tabs render
- Verify table renders (or empty state if no reviews)
- Click expand chevron to show detail card
- Click approve/reject/fix buttons (if reviews exist)
- Verify no console errors or CSP violations

**Fixtures:**

Use the fixture generation script to create review queue test data:

```bash
# Generate 5 review queue fixtures
tests/e2e/setup_fixtures.sh 5

# This creates events with reversed dates that will appear in review queue
```

See `tests/e2e/README.md` for fixture documentation.

### Unit Tests

**Backend tests:**
- `internal/domain/events/normalize_test.go` - Normalization logic
- `internal/domain/events/validation_test.go` - Warning generation
- `internal/api/handlers/admin_review_queue_test.go` - Handler tests (if exist)

**Frontend tests:**
- Manual testing via browser DevTools Console
- Run E2E tests to catch JavaScript errors

### Manual Testing Checklist

When modifying review queue UI:
- [ ] Run Playwright E2E tests first
- [ ] Test with empty queue (empty state displays)
- [ ] Test with pending reviews (table populates)
- [ ] Expand detail card (original vs normalized comparison)
- [ ] Test approve action (entry removed from pending list)
- [ ] Test reject modal (requires reason, validates)
- [ ] Test fix form (datetime inputs, converts to ISO 8601)
- [ ] Test status filter tabs (pending/approved/rejected)
- [ ] Test pagination (next cursor)
- [ ] Verify no console errors (F12 → Console tab)
- [ ] Verify no CSP violations

---

## Future Work

### 1. FixReview Workflow (srv-trg)

**Current State:** The `/api/v1/admin/review-queue/:id/fix` endpoint exists but is **incomplete**.

**What's Missing:**
- Endpoint only marks review as approved with notes, does NOT actually update event dates
- No occurrence-level date update logic
- No re-normalization after manual correction
- See TODO comment: [admin_review_queue.go:460](file:///home/ryankelln/Documents/Projects/Art/togather/server/internal/api/handlers/admin_review_queue.go:460)

**Needed Implementation:**
1. Update event occurrence dates with corrected values
2. Re-run normalization on updated data
3. Validate corrected data meets SHACL constraints
4. Handle both simple date fixes and complex event structure changes
5. Add occurrence-level update API in `AdminService`

**Impact:** Admins can currently only approve or reject; manual correction via "Fix Dates" button doesn't actually update the event.

### 2. Bulk Review Actions

Allow admins to select multiple entries and approve/reject in batch.

**Use Case:** Trusted source submits 100 events, all with high-confidence corrections → Approve all at once

**Implementation:**
- Checkboxes in table rows
- "Select All" checkbox in header
- Bulk action buttons above table
- New API endpoint: `POST /api/v1/admin/review-queue/bulk-approve`

### 3. Source Trust Levels

Auto-approve high-confidence corrections from verified sources.

**Use Case:** City of Toronto's event feed has consistent timezone issues → Auto-approve without review

**Implementation:**
- Add `trust_level` field to `sources` table
- Modify ingestion logic to skip review queue for trusted sources with high-confidence fixes
- Admin UI to manage source trust levels

### 4. Learning System

Track admin decisions to improve auto-fix heuristics.

**Use Case:** Admin consistently approves 5-8 AM corrections → Expand high-confidence window

**Implementation:**
- Log review decisions with warning codes
- Analyze approval rates per warning code
- Adjust confidence thresholds based on historical data
- ML model to classify corrections

### 5. Notification System

Alert admins when review queue grows large.

**Use Case:** 50+ pending reviews → Email digest to admins

**Implementation:**
- Background job counts pending reviews
- Email/Slack notification when threshold exceeded
- Admin preference settings for notification frequency

### 6. Review Queue Metrics Dashboard

Add analytics section showing:
- Review volume over time
- Approval vs rejection rates
- Average time to review
- Top sources generating warnings

### 7. Occurrence-Level Date Issues

**Current limitation:** Review queue only handles top-level `startDate`/`endDate` fields.

**Issue:** Events with ONLY `occurrences` array (no top-level dates) are NOT handled:
- `normalizeOccurrences()` doesn't apply timezone correction
- `validateOccurrences()` hard-rejects reversed dates (doesn't generate warnings)

**Fix:** See bead srv-oad for implementation plan.

---

## Rate Limiting

All review queue API endpoints are protected by **admin-tier rate limiting** to prevent abuse and ensure system stability.

### What Rate Limits Apply

Review queue endpoints (`/api/v1/admin/review-queue/*`) use the **TierAdmin** rate limit tier:

- **Configuration:** `RATE_LIMIT_ADMIN` in `.env` file
- **Default:** `0` (unlimited) - recommended for admin endpoints
- **Units:** Requests per minute per client IP
- **Scope:** Applies to all authenticated admin API endpoints, not just review queue

**Why rate limit admin endpoints?**
- Protects against compromised admin accounts
- Prevents accidental denial-of-service from buggy scripts
- Ensures fair resource usage when multiple admins work simultaneously
- Limits damage from brute-force token guessing attacks

### Rate Limit Tiers

The server uses five rate limit tiers, each with different thresholds:

| Tier | Env Variable | Default (Dev) | Typical Production | Applies To |
|------|--------------|---------------|-------------------|-----------|
| **Public** | `RATE_LIMIT_PUBLIC` | 60 req/min | 60 req/min | Unauthenticated API calls |
| **Agent** | `RATE_LIMIT_AGENT` | 300 req/min | 300 req/min | API key authenticated calls |
| **Admin** | `RATE_LIMIT_ADMIN` | 0 (unlimited) | 0 (unlimited) | Admin UI/API (including review queue) |
| **Login** | `RATE_LIMIT_LOGIN` | 5 attempts/15min | 5 attempts/15min | `/api/v1/admin/login` endpoint |
| **Federation** | `RATE_LIMIT_FEDERATION` | 500 req/min | 500 req/min | Federation sync endpoints |

**Implementation:** See `internal/api/middleware/ratelimit.go:90-99`

### Configuration for Different Environments

#### Local Development

High limits to avoid throttling during testing:

```bash
# .env (local development)
RATE_LIMIT_PUBLIC=10000    # Effectively unlimited
RATE_LIMIT_AGENT=20000     # Effectively unlimited
RATE_LIMIT_ADMIN=0         # Unlimited (recommended)
RATE_LIMIT_LOGIN=0         # Unlimited (for testing)
RATE_LIMIT_FEDERATION=500
```

**Note:** `RATE_LIMIT_ADMIN=0` disables rate limiting entirely for admin endpoints. This is safe for local development and production (admins are already authenticated).

#### Staging/Production

Conservative limits for public tiers, unlimited for admins:

```bash
# .env (production)
RATE_LIMIT_PUBLIC=60       # 1 request per second
RATE_LIMIT_AGENT=300       # 5 requests per second
RATE_LIMIT_ADMIN=0         # Unlimited (recommended)
RATE_LIMIT_LOGIN=5         # 5 attempts per 15 minutes (aggressive)
RATE_LIMIT_FEDERATION=500  # Federation sync traffic
```

**Rationale for unlimited admin limits:**
- Admins are already authenticated (JWT tokens)
- Admin UI patterns require rapid API calls (e.g., bulk operations, pagination)
- Risk of abuse is low (admins are trusted, token compromise is mitigated by short expiry)
- Better UX for legitimate admin workflows

**If you need to limit admin traffic** (e.g., shared server with multiple tenants):
```bash
RATE_LIMIT_ADMIN=1000      # 1000 requests/minute (16 req/sec)
```

### How Rate Limiting Works

**Token Bucket Algorithm:**
1. Each client IP gets a bucket with a maximum capacity (e.g., 60 tokens for public tier)
2. Each request consumes 1 token
3. Tokens refill at a constant rate (e.g., 1 token/second for 60 req/min limit)
4. When bucket is empty, requests are rejected with HTTP 429

**Client Identification:**
- By default, uses direct connection IP (`r.RemoteAddr`)
- If behind a trusted proxy, uses `X-Forwarded-For` or `X-Real-IP` headers
- Trusted proxies are configured via `TRUSTED_PROXY_CIDRS` in `.env`
- Prevents header spoofing from untrusted sources (Security: server-chgh)

**Bucket Cleanup:**
- Inactive buckets are removed after 15 minutes of no requests
- Prevents unbounded memory growth during attacks (Security: server-g746)
- See `internal/api/middleware/ratelimit.go:149-164`

### What Happens When Rate Limits Are Exceeded

**HTTP 429 Too Many Requests response:**

```http
HTTP/1.1 429 Too Many Requests
Retry-After: 60
Content-Type: application/json

{
  "error": "Too Many Requests"
}
```

**Retry-After header values:**
- **Admin/Public/Agent/Federation:** `60` seconds (1 minute)
- **Login tier:** `180` seconds (3 minutes, matches token refill rate)

**Client behavior:**
- **Browser:** Admin UI will show error toast: "Too many requests. Please try again in 1 minute."
- **API clients:** Should respect `Retry-After` header and implement exponential backoff
- **Scripts:** Use delays between requests to stay under limits

**Implementation:** See `internal/api/middleware/ratelimit.go:62-71`

### Testing Rate Limits

**Verify rate limiting works:**

```bash
# Public endpoint (60 req/min limit)
for i in {1..70}; do
  curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/api/v1/events
done
# First 60 should return 200, next 10 should return 429

# Admin endpoint (unlimited by default)
TOKEN="your-jwt-token"
for i in {1..100}; do
  curl -s -o /dev/null -w "%{http_code}\n" \
    -H "Authorization: Bearer $TOKEN" \
    http://localhost:8080/api/v1/admin/review-queue
done
# All should return 200 (or 401 if token invalid)
```

**Test with custom limits:**

```bash
# Temporarily set admin limit to 10 req/min
RATE_LIMIT_ADMIN=10 make run

# Then test with admin endpoints
```

### Troubleshooting

**Problem: Getting 429 errors in local development**

**Solution:** Check `.env` file and increase limits:
```bash
RATE_LIMIT_PUBLIC=10000
RATE_LIMIT_AGENT=20000
RATE_LIMIT_ADMIN=0  # Unlimited for admins
```

**Problem: Rate limits not working in production**

**Solution:** Verify `RATE_LIMIT_*` values are set:
```bash
# On production server
source .env && env | grep RATE_LIMIT
```

**Problem: False rate limiting behind load balancer**

**Solution:** Configure trusted proxy CIDRs:
```bash
# .env
TRUSTED_PROXY_CIDRS=10.0.0.0/8,172.16.0.0/12
```
This allows the server to trust `X-Forwarded-For` headers from your load balancer's IP range.

**Problem: Want to rate limit specific admin users differently**

**Current limitation:** Rate limits are per-IP, not per-user. All admins from the same IP share the same bucket.

**Workaround:** If needed, modify `clientKey()` in `internal/api/middleware/ratelimit.go` to use JWT subject (user ID) instead of IP for admin tier.

### Related Code

- **Middleware:** `internal/api/middleware/ratelimit.go` - Token bucket implementation
- **Router config:** `internal/api/router.go:318-322` - Review queue endpoints with `adminRateLimit`
- **Environment config:** `.env.example:35-44` - Rate limit settings
- **Config struct:** `internal/config/config.go` - Rate limit configuration

---

## Related Documentation

- **Architecture:** `docs/architecture/event-review-workflow.md` - Complete design rationale
- **UI Design:** `docs/design/review-queue-ui.md` - Original UI spec
- **E2E Testing:** `tests/e2e/README.md` - Testing guide and fixture generation
- **API Handlers:** `internal/api/handlers/admin_review_queue.go` - Backend implementation
- **Database Schema:** Migration in `internal/storage/postgres/migrations/`
- **Queries:** `internal/storage/postgres/queries/event_review_queue.sql` - All SQL queries

---

## Contributing

When modifying the review queue feature:

1. **Read related docs first** - Architecture doc explains design decisions
2. **Follow existing patterns** - UI matches other admin pages (`events.js`, `duplicates.js`)
3. **Test thoroughly** - Run E2E tests BEFORE claiming work complete
4. **Check console errors** - Use Playwright to catch JavaScript failures
5. **Update this README** - Document new features or behavior changes

**CSP Compliance:**
- NEVER use inline scripts or event handlers (`onclick`, `onerror`, etc.)
- Use `data-action` attributes + event delegation pattern
- All JavaScript in external `.js` files
- See `web/AGENTS.md` for CSP guidelines

**API Client Pattern:**
- Use shared `API.reviewQueue.*` methods, never direct `fetch()`
- Handles JWT auth automatically
- Returns JSON or throws error
- See `web/admin/static/js/api.js` for implementation
