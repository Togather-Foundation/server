# tg-review CLI — Admin Review Queue

**Version:** 1.0.0
**Date:** 2026-06-08
**Status:** Implemented (STS token exchange, 9 subcommands, batch operations, aggregate stats)

The `server review` CLI wraps the admin review-queue API for automated event review.
It is designed for AI agents and operators to triage, inspect, and resolve data-quality
issues flagged during event ingestion — near-duplicates, reversed dates, missing
structured fields, and other warnings stored in the `event_review_queue` table.

For the full review workflow state machine and API documentation,
see [docs/architecture/event-review-workflow.md](../architecture/event-review-workflow.md).

---

## Table of Contents

1. [Quick Start](#quick-start)
2. [Auth Flow](#auth-flow)
3. [CLI Reference](#cli-reference)
4. [Subcommand Reference](#subcommand-reference)
5. [Safety Features](#safety-features)
6. [Environment Variables](#environment-variables)
7. [Examples](#examples)

---

## Quick Start

```bash
# List pending review items
server review queue

# Deep inspect a single entry
server review check 42

# Approve a reviewed entry
server review approve 42

# Reject a bad entry with a reason
server review reject 42 --reason "spam"

# Fix and approve with date corrections
server review fix 42 --start-date "2026-03-15T19:00:00Z"

# Batch-approve all events matching a name pattern (dry-run first)
server review batch --name "Astro Coffee" --action approve --dry-run
server review batch --name "Astro Coffee" --action approve

# Get aggregate queue statistics
server review stats

# Merge a duplicate event into a primary
server review merge evt_canonical evt_duplicate

# Consolidate multiple events into one canonical
server review consolidate evt_canonical evt_dup1 evt_dup2 evt_dup3
```

All subcommands use STS token exchange for authentication. An admin API key is
required unless a pre-minted JWT is provided via `--token`.

---

## Auth Flow

The review CLI authenticates via the admin STS endpoint, following a resolution
chain identical to the scraper CLI.

### Auth resolution

```
--token flag (pre-minted JWT, skips STS exchange)
  → --key flag (admin API key for STS exchange)
    → TOGATHER_ADMIN_API_KEY env var
      → ERROR (no key provided)
```

### STS token exchange

When `--token` is not set, the CLI calls `POST /api/v1/auth/token` with the
admin API key as a Bearer token. The response is a short-lived JWT used for all
subsequent API calls.

```bash
# Explicit key + server
server review queue --server https://staging.toronto.togather.foundation --key "$TOGATHER_ADMIN_API_KEY"

# Or source the auto-generated credentials file
source .agent-keys/staging
server review queue
```

After every `deploy.sh staging` run, credentials are written to `.agent-keys/staging`.
Source it to get `TOGATHER_ADMIN_API_KEY` and `TOGATHER_BASE_URL`.

### Server URL resolution

```
--server flag
  → TOGATHER_BASE_URL env var
    → http://localhost:8080
```

---

## CLI Reference

All subcommands are under `server review`. Persistent flags apply to all subcommands.

### Persistent Flags

| Flag | Env | Default | Description |
|------|-----|---------|-------------|
| `--server` | `TOGATHER_BASE_URL` | `http://localhost:8080` | Server base URL |
| `--key` | `TOGATHER_ADMIN_API_KEY` | — | Admin API key for STS token exchange |
| `--token` | — | — | Pre-minted JWT (skips STS exchange) |
| `--json` | — | `false` | JSON output instead of table/detail format |

---

## Subcommand Reference

### `server review queue`

List review queue items with optional filters and grouping. Fetches all items
server-side (paginated) then applies client-side filtering.

```bash
server review queue
server review queue --status pending
server review queue --status approved --limit 100
server review queue --warning reversed_dates_corrected_needs_review
server review queue --name "Astro" --source source-slug
server review queue --group-by name
server review queue --group-by warning
server review queue --group-by source
server review queue --json
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--status` | string | `pending` | Filter by status: `pending`, `approved`, `rejected`, `merged` |
| `--limit` | int | `50` | Maximum items to return (1–200) |
| `--warning` | string | `""` | Filter by warning code (client-side substring match) |
| `--name` | string | `""` | Filter by event name substring (client-side, case-insensitive) |
| `--source` | string | `""` | Filter by source_id (client-side exact match) |
| `--group-by` | string | `""` | Group results by: `name`, `source`, `warning` |

**Plain table output (default):**
```
ID  ULID       NAME               START DATE  WARNINGS      OCC  AGE
42  evt_abc123 Astro Coffee       2026-03-15  potential_dup  3   5d
```

**Grouped output (`--group-by name`):**
```
NAME             COUNT  WARNINGS        AGE RANGE
Astro Coffee     8      potential_dup   5d-2w
Comedy Show      3      reversed_dates  1d-3d
```

Groups are sorted by count descending. Age range shows newest–oldest entries.

### `server review check <review-id>`

Deep inspect a single review queue entry. Shows the event, its warnings, any
automatic corrections applied during ingestion, related near-duplicate events
with similarity scores, review notes, and rejection reason.

```bash
server review check 42
server review check 42 --json
```

**Output:**
```
Review #42 — pending
  Event: Astro Coffee (evt_abc123)
  Date: 2026-03-15
  Age: 5d
  Source: astro-coffee

Warnings (2):
  [near_duplicate_of_new_event] name: Near-duplicate of event evt_xyz789 (similarity 0.94)
  [missing_description] description: Description is empty

Changes:
  startDate: 2026-03-15T00:00:00Z → 2026-03-15T19:00:00Z (auto-corrected from schema)

Related Events:
  evt_xyz789 — Astro Coffee Night (0.94)
  evt_def456 — Astro Coffee & Jazz (0.87)
```

### `server review approve <id>`

Approve a review queue entry. Publishes the event and sets review status to
`approved`. Can optionally record duplicate warnings as not-actually-duplicates
via `--record-not-duplicates`.

```bash
server review approve 42
server review approve 42 --notes "Venue confirmed this is correct"
server review approve 42 --record-not-duplicates
server review approve 42 --json
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--notes` | string | `""` | Optional review notes |
| `--record-not-duplicates` | bool | `false` | Record duplicate warnings as not-duplicates, dismissing companion reviews |

### `server review reject <id>`

Reject a review queue entry. Soft-deletes the event with a tombstone and sets
review status to `rejected`. Requires `--reason`.

```bash
server review reject 42 --reason "spam"
server review reject 42 --reason "Duplicate of existing event" --notes "See evt_xyz789"
server review reject 42 --reason "Bad dates" --json
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--reason` | string | **required** | Rejection reason |
| `--notes` | string | `""` | Optional review notes |

### `server review fix <id>`

Manually correct occurrence dates and approve. Applies the corrections, publishes
the event, and sets review status to `approved`. Requires at least one date flag.

```bash
server review fix 42 --start-date "2026-03-15T19:00:00Z"
server review fix 42 --start-date "2026-03-15T19:00:00Z" --end-date "2026-03-15T22:00:00Z"
server review fix 42 --start-date "2026-03-15T19:00:00Z" --notes "Corrected from venue website"
server review fix 42 --start-date "2026-03-15T19:00:00Z" --json
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--start-date` | string (RFC3339) | `""` | Corrected start date |
| `--end-date` | string (RFC3339) | `""` | Corrected end date |
| `--notes` | string | `""` | Optional review notes |

Dates must be valid RFC 3339 timestamps (e.g. `2026-03-15T19:00:00Z` or
`2026-03-15T19:00:00-04:00`).

### `server review batch`

Batch approve, reject, fix, or merge multiple review items matching a name
pattern. Always run with `--dry-run` first to preview the affected items.

```bash
# Dry-run to preview
server review batch --name "Astro Coffee" --action approve --dry-run
server review batch --name "Comedy" --source "comedy-bar" --dry-run

# Execute after preview
server review batch --name "Astro Coffee" --action approve

# Batch reject
server review batch --name "spam-event" --action reject --reason "spam"

# Batch fix with date correction
server review batch --name "Tranzac Open Stage" --action fix --start-date "2026-01-07T13:30:00Z"

# Batch merge into a primary
server review batch --name "Astro Coffee" --action merge-into-primary --primary-id evt_canonical
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | **required** | Filter by event name substring (case-insensitive) |
| `--source` | string | `""` | Filter by source_id |
| `--warning` | string | `""` | Filter by warning code |
| `--action` | string | `""` | Action: `approve`, `reject`, `fix`, `merge-into-primary` |
| `--reason` | string | `""` | Rejection reason (required for `--action reject`) |
| `--primary-id` | string | `""` | Primary event ULID (required for `--action merge-into-primary`) |
| `--dry-run` | bool | `false` | Preview what would be changed without executing |
| `--limit` | int | `200` | Maximum items to process |
| `--notes` | string | `""` | Optional review notes for approve/fix actions |
| `--start-date` | string (RFC3339) | `""` | Corrected start date (required for `--action fix`) |
| `--end-date` | string (RFC3339) | `""` | Corrected end date (optional for `--action fix`) |

**Batch execution details:**
- Items are processed in chunks of `REVIEW_BATCH_MAX_SIZE` (default 100)
- A `REVIEW_BATCH_DELAY_MS` delay (default 50 ms) is inserted between items
- On auth errors (401/403), processing stops immediately with a clear error
- A final summary shows succeeded and failed counts with per-item error details

### `server review stats`

Show aggregate review queue statistics — warning type breakdown with percentages
and age ranges, name groups (events with ≥2 instances of the same name), and age
buckets with ASCII bar charts. Spike detection highlights buckets with >40% of
total items.

```bash
server review stats
server review stats --json
```

**Output**
```
Queue: 248 pending (oldest: 4w, newest: 1h, median: 3d)

WARNING TYPE                      COUNT   %        AGE RANGE
potential_duplicate               156     62.9%    1h-4w
missing_description               82      33.1%    2d-2w
reversed_dates_corrected_needs    31      12.5%    1d-1w

NAME GROUPS (≥2 instances)   18 groups / 76 items
  Astro Coffee Night                         8  (1w-3w)
  Comedy Bar Showcase                        5  (2d-1w)

AGE BUCKETS
  0-9 days: 142  ████████████████████████████████████████
  10-19 days: 67  ██████████████████
  20-29 days: 31  █████████
  30-39 days: 5   █
  40-49 days: 3
  50-59 days: 0
  60-69 days: 0
  70+ days: 0
```

### `server review merge <primary-id> <duplicate-id>`

Consolidate a single duplicate event into a canonical primary event via the
events consolidate API. The duplicate event is soft-deleted with a tombstone.

```bash
server review merge evt_canonical evt_duplicate
server review merge evt_canonical evt_duplicate --json
```

### `server review consolidate <canonical-id> <id2> [id3...]`

Consolidate multiple duplicate events into one canonical event. All
non-canonical events are soft-deleted. Accepts 2+ positional arguments.

```bash
server review consolidate evt_canonical evt_dup1 evt_dup2 evt_dup3
server review consolidate evt_canonical evt_dup1 evt_dup2 --json
```

---

## Safety Features

### Dry-run preview on batch

The `--dry-run` flag on `server review batch` prints exactly which items would
be affected without making any changes:

```bash
server review batch --name "Astro Coffee" --action approve --dry-run
# Would approve 8 items:
#   #42 — Astro Coffee (evt_abc123)
#   #58 — Astro Coffee Night (evt_def456)
#   ...
```

Always dry-run before executing a batch action.

### Default dry-run when no action

If `--action` is omitted from `server review batch`, the command behaves as a
dry-run regardless of the `--dry-run` flag:

```bash
server review batch --name "Astro Coffee"
# Would processed 8 items:
#   ...
```

### Stop on auth errors

During batch execution, if any request returns 401 or 403, processing stops
immediately. This prevents wasted API calls and provides a clear diagnostic:

```
✓ 5 approved, ✗ 1 failed (stopped: auth error)
  #58 Astro Coffee Night: authentication failed (401)
```

### Chunked processing with rate limiting

Batch operations are chunked and rate-limited to avoid overwhelming the server:

- `REVIEW_BATCH_MAX_SIZE` (default 100) — max items per chunk
- `REVIEW_BATCH_DELAY_MS` (default 50) — inter-request delay in milliseconds

### Required flags for destructive actions

- `reject` requires `--reason`
- `merge-into-primary` requires `--primary-id`
- `fix` requires `--start-date` or `--end-date`

These are enforced before any API call is made.

---

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `TOGATHER_ADMIN_API_KEY` | — | Admin API key for STS token exchange |
| `TOGATHER_BASE_URL` | `http://localhost:8080` | Server base URL |
| `REVIEW_BATCH_MAX_SIZE` | `100` | Maximum items per batch chunk |
| `REVIEW_BATCH_DELAY_MS` | `50` | Inter-request delay (ms) for rate limiting |

---

## Examples

### Triaging a new warning spike

```bash
# 1. Get aggregate statistics to understand the queue
server review stats

# 2. Inspect the most common warning type
server review queue --warning potential_duplicate --limit 10

# 3. Deep inspect a specific entry to see related events
server review check 42
```

### Resolving an Astro Coffee duplicate group

```bash
# 1. Find all Astro Coffee entries in the queue
server review queue --name "Astro Coffee" --group-by name

# 2. Inspect one entry to see related events with similarity scores
server review check 42

# 3. Pick the canonical (e.g. the oldest or most complete)
# Dry-run the merge batch first
server review batch --name "Astro Coffee" --action merge-into-primary \
  --primary-id evt_canonical --dry-run

# 4. Execute
server review batch --name "Astro Coffee" --action merge-into-primary \
  --primary-id evt_canonical
```

### Consolidating Comedy Bar duplicates

```bash
# Use review consolidate to handle N events at once
server review consolidate evt_comedy_bar evt_comedy_bar_dup1 evt_comedy_bar_dup2

# Or as a batch from the queue by name
server review batch --name "Comedy Bar" --source "comedy-bar" --action approve --dry-run
server review batch --name "Comedy Bar" --source "comedy-bar" --action approve
```

### Dry-run batch with date correction

```bash
# Preview Tranzac Open Stage entries needing date fixes
server review batch --name "Tranzac Open Stage" --action fix \
  --start-date "2026-01-07T13:30:00Z" --dry-run

# Execute
server review batch --name "Tranzac Open Stage" --action fix \
  --start-date "2026-01-07T13:30:00Z"
```

### Grouped view for pattern spotting

```bash
# Group by warning type to see which issues dominate
server review queue --group-by warning

# Group by source to identify problematic scraper configs
server review queue --group-by source

# Group by name to find duplicate/recurring event groups
server review queue --group-by name
```

### Direct merge without queue lookup

```bash
# When you already know the primary and duplicate ULIDs
server review merge evt_primary evt_duplicate

# Consolidate many known duplicates at once
server review consolidate evt_canonical evt_dup1 evt_dup2 evt_dup3 evt_dup4
```
