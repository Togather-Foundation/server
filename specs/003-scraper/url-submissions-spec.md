# Feature Specification: Public URL Submission Endpoint

**Parent Feature**: 003-scraper
**Created**: 2026-02-27
**Status**: Planned
**Epic**: srv-1cxmi
**Input**: Community members and agents need a way to suggest URLs for scraping without requiring admin access or direct scraper configuration.

## Context

The scraper system (Tiers 0-3) is mature, with DB-backed source configs, scheduling, admin UI, and CLI tooling. However, adding new sources currently requires either:

1. An admin creating a YAML config or DB entry, or
2. An agent authoring selectors via the `/configure-source` workflow.

There is no way for the public to suggest a URL for consideration. Community members who know about great event sources have no channel to contribute them. This feature adds a rate-limited, public-facing endpoint where anyone can submit one or more URLs for admin review and eventual scraping.

### Design Principles

- **Queue, don't scrape**: Submitted URLs are queued for admin review, not automatically scraped. This keeps humans in the loop and prevents abuse.
- **Async validation**: URL reachability and robots.txt checks happen in a background worker, not at request time. The submission endpoint stays fast.
- **Rate-limited and deduped**: Per-IP rate limits (5 submissions/24h) and a 30-day dedup window prevent abuse and redundant submissions.
- **Respectful**: Even the validation step respects robots.txt and uses a lightweight HEAD request.
- **Agent-friendly**: JSON responses are structured for both human admins and agentic workflows that manage the review queue.

### Non-Goals (MVP)

- Automatic scraping of submitted URLs (admin manually triggers via CLI)
- Email notifications to submitters about URL status
- Authenticated/per-user submission quotas (IP-based only for MVP)
- Auto-creation of `scraper_sources` entries from submissions
- Submitter accounts or submission history for non-admins
- CAPTCHA or proof-of-work (rate limiting is sufficient for MVP)

## User Scenarios & Testing

### User Story 1 — Public User Submits URLs for Scraping (Priority: P2)

A community member knows about event sources that aren't in the system and wants to suggest them. They submit one or more URLs via the public API. The system validates the request synchronously (format, dedup, rate limit) and queues accepted URLs for async validation.

**Independent Test**: A `POST /api/v1/scraper/submissions` with a valid URL returns a per-URL result indicating whether each URL was accepted, rejected, or is a duplicate. No authentication required.

**Acceptance Scenarios**:

1. **Given** a well-formed HTTPS URL not previously submitted, **When** a user POSTs `{"urls": ["https://example.com/events"]}`, **Then** the system returns 200 with `[{"url": "https://example.com/events", "status": "accepted", "message": "URL queued for review"}]`
2. **Given** a URL submitted within the last 30 days, **When** the same URL (or its normalised equivalent) is submitted again, **Then** the system returns status `"duplicate"` with message `"Already submitted within 30 days"`
3. **Given** a malformed URL (no host, unsupported scheme), **When** submitted, **Then** the system returns status `"rejected"` with a descriptive message (e.g., `"Invalid URL: missing host"`)
4. **Given** a user has submitted 5 URLs in the last 24 hours, **When** they submit another, **Then** the system returns HTTP 429 with `Retry-After` header and RFC 7807 error body
5. **Given** a batch of 10 valid URLs, **When** submitted in a single request, **Then** the system returns per-URL results for all 10
6. **Given** a batch of 11+ URLs, **When** submitted, **Then** the system returns 400 Bad Request with message `"Maximum 10 URLs per request"`
7. **Given** a mix of valid, duplicate, and malformed URLs in one request, **When** submitted, **Then** each URL gets its own result with the appropriate status

---

### User Story 2 — Background Worker Validates Submitted URLs (Priority: P2)

After URLs are accepted into the queue, a background worker validates them asynchronously by checking reachability and robots.txt compliance. Only URLs that pass validation are surfaced to admins.

**Independent Test**: A URL in `pending_validation` status is picked up by the `ValidateSubmissionsBatchWorker`, which performs a HEAD request and robots.txt check, then updates the status to `pending` (passed) or `rejected` (failed).

**Acceptance Scenarios**:

1. **Given** URLs in `pending_validation` status exist, **When** the `ValidateSubmissionsSchedulerWorker` fires (every 5 minutes), **Then** it enqueues a `ValidateSubmissionsBatchJob` if one is not already running
2. **Given** a `ValidateSubmissionsBatchJob` runs, **Then** it fetches up to 20 `pending_validation` URLs and validates each with a HEAD request (5s timeout) and `scraper.RobotsAllowed()` check
3. **Given** a URL responds with HTTP 2xx/3xx and allows the Togather user-agent in robots.txt, **When** validated, **Then** status is updated to `pending` with `validated_at` timestamp
4. **Given** a URL responds with HTTP 4xx/5xx or times out, **When** validated, **Then** status is updated to `rejected` with `rejection_reason` (e.g., `"HEAD request returned 404"`)
5. **Given** a URL is disallowed by robots.txt, **When** validated, **Then** status is updated to `rejected` with `rejection_reason` `"robots.txt disallows crawling"`
6. **Given** a batch completes and more `pending_validation` rows exist, **When** the batch finishes, **Then** it re-enqueues itself for immediate processing (job chaining)
7. **Given** a batch completes and no more `pending_validation` rows exist, **When** the batch finishes, **Then** it exits cleanly (scheduler checks again in 5 minutes)
8. **Given** a DDOS-scale burst of submissions, **Then** the batch worker processes 20 at a time sequentially, providing natural back-pressure without overwhelming target sites

---

### User Story 3 — Admin Reviews and Manages Submission Queue (Priority: P2)

An admin reviews the queue of validated URLs, decides which ones to scrape, and marks them as processed or rejected. The admin can also view the full history of submissions.

**Independent Test**: An authenticated admin can `GET /api/v1/admin/scraper/submissions?status=pending` to see validated URLs awaiting review, and `PATCH /api/v1/admin/scraper/submissions/{id}` to update their status.

**Acceptance Scenarios**:

1. **Given** validated submissions exist with status `pending`, **When** an admin requests `GET /api/v1/admin/scraper/submissions?status=pending`, **Then** the system returns a paginated JSON list sorted by `submitted_at DESC`
2. **Given** an admin wants to mark a URL as processed (after running `server scrape url <URL>`), **When** they PATCH with `{"status": "processed", "notes": "Created source config"}`, **Then** the submission is updated
3. **Given** an admin wants to reject a URL, **When** they PATCH with `{"status": "rejected", "notes": "Not an event source"}`, **Then** the submission is updated with the rejection note
4. **Given** no `status` query parameter, **When** listing submissions, **Then** all statuses are returned (default behaviour)
5. **Given** pagination parameters `?limit=20&offset=40`, **When** listing submissions, **Then** the system returns the correct page of results

---

### Edge Cases

- What happens when the validation worker encounters a URL that redirects infinitely?
  - HEAD request has a 5-second timeout; the URL is rejected with `"HEAD request timed out"`
- What happens when the same IP submits from different proxies/VPNs?
  - Each apparent IP gets its own 5/24h quota. This is acceptable for MVP; abuse patterns can be addressed with additional heuristics later.
- What happens when a previously rejected URL is re-submitted after 30 days?
  - The dedup window has expired, so it is accepted as a new submission and re-validated
- What happens when the validation worker is down or River is unavailable?
  - Submissions accumulate in `pending_validation`. When the worker resumes, it processes the backlog in batches of 20.
- What if the same URL is submitted with different fragments or trailing slashes?
  - URL normalisation (lowercase scheme+host, strip fragment, strip trailing slash) ensures these are treated as the same URL for dedup purposes

## Technical Design

### API Contract

#### `POST /api/v1/scraper/submissions` (public, no auth)

**Request**:
```json
{
  "urls": [
    "https://example.com/events",
    "https://other.org/calendar"
  ]
}
```

Constraints:
- `urls` array is required, non-empty, max 10 elements
- Each URL must be parseable with a valid host and `http` or `https` scheme

**Response** (200 OK):
```json
{
  "results": [
    {
      "url": "https://example.com/events",
      "status": "accepted",
      "message": "URL queued for review"
    },
    {
      "url": "https://other.org/calendar",
      "status": "duplicate",
      "message": "Already submitted within 30 days"
    }
  ]
}
```

Per-URL `status` values: `accepted` | `duplicate` | `rejected`

**Error responses** (RFC 7807):
- 400: Malformed body, empty `urls`, or >10 URLs
- 429: Per-IP rate limit exceeded (includes `Retry-After` header)
- 500: Unexpected server error

#### `GET /api/v1/admin/scraper/submissions` (admin, JWT auth)

**Query parameters**:
- `status` (optional): `pending_validation` | `pending` | `rejected` | `processed`
- `limit` (optional, default 50, max 200)
- `offset` (optional, default 0)

**Response** (200 OK):
```json
{
  "submissions": [
    {
      "id": 42,
      "url": "https://example.com/events",
      "url_norm": "https://example.com/events",
      "submitted_at": "2026-02-27T15:00:00Z",
      "submitter_ip": "203.0.113.42",
      "status": "pending",
      "rejection_reason": null,
      "notes": null,
      "validated_at": "2026-02-27T15:01:30Z"
    }
  ],
  "total": 1,
  "limit": 50,
  "offset": 0
}
```

#### `PATCH /api/v1/admin/scraper/submissions/{id}` (admin, JWT auth)

**Request**:
```json
{
  "status": "processed",
  "notes": "Created source config and ran initial scrape"
}
```

Valid target statuses: `processed` | `rejected`

**Response** (200 OK): Updated submission object.

### Status Flow

```
POST /api/v1/scraper/submissions
         |
         |-- reject immediately --> (not stored; returned in response)
         |                          reasons: malformed URL, unsupported scheme
         |
         |-- duplicate -----------> (not stored; returned in response)
         |                          reason: same url_norm within 30 days
         |
         +-- insert --------------> pending_validation
                                         |
                              ValidateSubmissionsBatchWorker
                              (enqueued by 5-min scheduler)
                                         |
                         +---------------+---------------+
                         v                               v
                      pending                         rejected
                   (HEAD 2xx/3xx,                 (unreachable,
                    robots.txt OK)                disallowed, timeout)
                         |
                  Admin reviews via
                  GET .../submissions?status=pending
                         |
                  +------+------+
                  v             v
               processed     rejected
            (admin ran      (admin decided
             scrape)         not relevant)
```

### URL Normalisation

Before dedup checks and storage, URLs are normalised:

1. Parse with `net/url.Parse()`
2. Lowercase scheme and host
3. Strip fragment (`#...`)
4. Strip trailing slash (unless path is just `/`)
5. Sort query parameters alphabetically
6. Store both original `url` and normalised `url_norm`

### Database Schema

New migration `000037_scraper_submissions`:

```sql
CREATE TABLE scraper_submissions (
  id               BIGSERIAL PRIMARY KEY,
  url              TEXT NOT NULL,
  url_norm         TEXT NOT NULL,
  submitted_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  submitter_ip     INET NOT NULL,
  status           TEXT NOT NULL DEFAULT 'pending_validation'
                   CHECK (status IN ('pending_validation', 'pending', 'rejected', 'processed')),
  rejection_reason TEXT,
  notes            TEXT,
  validated_at     TIMESTAMPTZ
);

CREATE INDEX scraper_submissions_url_norm_idx
  ON scraper_submissions(url_norm, submitted_at DESC);
CREATE INDEX scraper_submissions_status_idx
  ON scraper_submissions(status, submitted_at DESC);
CREATE INDEX scraper_submissions_ip_idx
  ON scraper_submissions(submitter_ip, submitted_at DESC);
```

No `UNIQUE` constraint on `url_norm` — re-submission after the 30-day window is intentional.

### River Worker Design

Two-job chain for self-throttling periodic pickup:

**`ValidateSubmissionsSchedulerWorker`** (periodic, 5-minute interval):
- Runs `CountPendingValidation` query
- If count > 0: enqueues `ValidateSubmissionsBatchArgs` with `UniqueOpts` (prevents double-enqueue while a batch is running)
- If count == 0: exits (next check in 5 minutes)

**`ValidateSubmissionsBatchWorker`** (on-demand, enqueued by scheduler):
- Fetches up to 20 `pending_validation` rows (oldest first)
- For each URL:
  - HEAD request with 5-second timeout
  - `scraper.RobotsAllowed()` check (reuses existing function from `internal/scraper/jsonld.go`)
  - Updates status to `pending` or `rejected` with reason
- After processing: checks `CountPendingValidation`
  - If more rows: re-enqueues self via `river.ClientFromContextSafely` (job chaining)
  - If none: exits cleanly
- Max attempts: 1 (no retry; scheduler re-enqueues on next tick if rows remain)

This provides:
- Fast processing when work exists (no 5-min wait between batches)
- DDOS protection via batch size cap (20 HEAD requests max per run)
- Natural back-pressure (sequential batch processing, not parallel)
- Quiet when idle (no wasted DB queries between scheduler ticks)

### Package Structure

```
internal/domain/scraper/
  submission.go              -- Submission type, SubmissionResult, SubmissionRepository interface

internal/domain/scraper/
  submission_service.go      -- SubmissionService (sync validation: format, dedup, rate limit)

internal/storage/postgres/queries/
  scraper_submissions.sql    -- SQLc query definitions

internal/storage/postgres/migrations/
  000037_scraper_submissions.{up,down}.sql

internal/jobs/
  validate_submissions.go    -- River scheduler + batch workers

internal/api/handlers/
  scraper_submissions.go     -- Public POST handler

internal/api/handlers/
  admin_scraper.go           -- Extended with GET/PATCH submissions (or new file)
```

### Rate Limiting

Two layers:

1. **Per-IP submission rate limit** (5 URLs / 24 hours): Enforced by the `SubmissionService` via DB query (`CountRecentSubmissionsByIP`). Shared across all server instances (green/blue deploys).

   **Pre-check-only policy**: The rate limit is checked once before processing any URLs. If the IP already has ≥5 accepted submissions in the last 24h, the entire request returns 429. If at least one slot remains, the whole batch goes through — there is no mid-batch quota tracking and no `rate_limited` per-URL status. The maximum over-quota exposure across two back-to-back requests is 2N-1 URLs, which is acceptable for MVP. Valid per-URL statuses are `accepted`, `duplicate`, and `rejected` only.

2. **General public rate limit middleware**: The existing `publicRateLimit` middleware applies to this endpoint like any other public route (60 req/min per IP).

### Dependencies

| Package | Purpose | Notes |
|---------|---------|-------|
| `github.com/riverqueue/river` | Background validation jobs | Existing dependency (v0.30.2) |
| `internal/scraper` | `RobotsAllowed()` reuse | Existing — no new imports needed |
| `net/http` | HEAD requests for URL validation | stdlib |

No new external dependencies required.

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Spam submissions from botnets (many IPs) | Queue flooded with junk URLs | Batch worker processes only 20/tick; admin can bulk-reject; future: add CAPTCHA or proof-of-work |
| Validation HEAD requests trigger WAFs on target sites | Our server IP gets blocked | Use Togather User-Agent; respect robots.txt; 5s timeout prevents hanging |
| Submitters use the endpoint to probe internal URLs (SSRF) | Information leakage | HEAD requests use standard `net/http` with redirect following disabled; private IP ranges could be blocked (future) |
| Admin queue grows unbounded | DB bloat | Periodic cleanup job (future) to purge processed/rejected submissions older than 90 days |
| Rate-limit bypass via IPv6 rotation | Quota evasion | Acceptable for MVP; can add subnet-based limiting (/64 prefix) later |

## Beads

| ID | Title | Depends On |
|----|-------|------------|
| srv-1cxmi | Epic: Public URL Submission Endpoint | -- |
| srv-v5rlp | DB migration: scraper_submissions table | epic |
| srv-mdh2i | SQLc queries for scraper_submissions | srv-v5rlp |
| srv-d01em | Domain layer: submission types, repository, service | srv-mdh2i |
| srv-m9bja | River workers: scheduler + batch validator | srv-d01em |
| srv-nggrk | Public handler: POST /api/v1/scraper/submissions | srv-d01em |
| srv-iwoy6 | Admin handler: GET/PATCH submissions | srv-d01em |
| srv-xrfyh | Router wiring | srv-nggrk, srv-iwoy6, srv-m9bja |
| srv-cu3ws | Tests | srv-d01em, srv-m9bja, srv-nggrk |
| srv-msbmm | Spec doc (this file) | epic |

## Success Metrics

- Public users can submit up to 10 URLs in a single request and receive per-URL feedback within 200ms (p95)
- Validation worker processes a backlog of 100 URLs within 10 minutes
- Admin can list, filter, and update submissions via API
- Rate limiting prevents any single IP from submitting more than 5 URLs in 24 hours
- Duplicate URLs within the 30-day window are correctly identified regardless of fragment/trailing-slash differences
- No submitted URL is scraped without admin review
