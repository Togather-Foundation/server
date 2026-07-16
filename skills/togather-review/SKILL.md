---
name: togather-review
description: Guidebook for triaging the Togather SEL event review queue — warning types, batch strategies, source patterns, and CLI reference. Use when operating a Togather server and items are piling up in the review queue.
license: MIT
compatibility: Requires a Togather server binary built from source (make build). Requires admin API access (admin-role API key for token exchange).
metadata:
  author: togather-foundation
  version: "3.0"
  hermes:
    tags: [togather, review, data-quality, scraping, operations]
    category: devops
---

# Togather Review Queue Guidebook

**THE RULE: Data janitor work, NOT curation.** The review queue fixes bad data
and bad scrapes. Taste, interest, and personal preference happen downstream.
Your ONLY job: is this event data correct and does it describe an actual gathering?

**Approve when:** A real gathering is happening (concert, class, meetup,
seminar, storytime, workshop). Data may be imperfect.

**Reject when:** Wrong geography (not the city your instance serves), not an
event (closure notice, job posting, catalog entry), garbage data (destroyed
name, wrong language).

## Workflow

Run these steps in order. Do not deviate. Do not process items individually.

Every terminal command must be prefixed with `source .env &&`:

### Step 1: Survey

```bash
source .env && ./server review stats --key "$TOGATHER_ADMIN_API_KEY" --server "$TOGATHER_BASE_URL"
source .env && ./server review queue --group-by source --key "$TOGATHER_ADMIN_API_KEY" --server "$TOGATHER_BASE_URL"
```

### Step 2: Batch approve by source (primary strategy)

For every source that produces real events, batch-approve the entire source
in one command. The `--source` filter works standalone (no `--name` required):

```bash
source .env && ./server review batch --source "<source-uuid>" --action approve \
  --notes "Real events, clean data" --key "$TOGATHER_ADMIN_API_KEY" \
  --server "$TOGATHER_BASE_URL" --limit 200
```

Combine `--source` + `--warning` for narrower targeting on mixed sources:

```bash
source .env && ./server review batch --source "<uuid>" --warning "missing_description" \
  --action approve --key "$TOGATHER_ADMIN_API_KEY" --server "$TOGATHER_BASE_URL"
```

### Step 3: Batch approve remaining patterns by name

For any pending items not caught by Step 2, group by name and batch-approve
recurring series by name substring. Try the most prominent words first
(splitting on spaces). Handle unicode apostrophes with U+2019 character:

```bash
source .env && ./server review batch --name "substring" --action approve \
  --notes "Real recurring event" --key "$TOGATHER_ADMIN_API_KEY" \
  --server "$TOGATHER_BASE_URL"
```

### Step 4: Verify and report

```bash
source .env && ./server review stats --key "$TOGATHER_ADMIN_API_KEY" --server "$TOGATHER_BASE_URL"
source .env && ./server review queue --group-by source --key "$TOGATHER_ADMIN_API_KEY" --server "$TOGATHER_BASE_URL"
```

Report the final stats. If items remain, list their names and sources for
human review — do not process them individually.

## Hard Constraints

- **NEVER run individual approve/reject commands.** No `./server review approve <id>`.
  No `./server review reject <id>`. Always use `batch` with `--source` or `--name`.
- **NEVER dump queue data to a file and read it back.** The CLI `--json` flag
  returns structured output inline. Parse decisions from terminal output directly.
- **NEVER use `read_file` on `.env` files.** It is blocked by the security
  scanner. Use `source .env &&` as a prefix to your terminal commands instead.
- **Batch approve unknown sources too.** When in doubt about a source, batch-approve
  it. Wrong-geography events will be caught by the scraper config, not the review queue.
- **Missing description is NOT a reason to reject.** Real events with no
  description are better than no events at all.
- **409 "already approved" errors are harmless.** Overlapping batches may hit
  the same item twice. Continue processing.

## Reference Files

Load on demand only when encountering an unfamiliar situation:

| File | When to load |
|------|-------------|
| `references/warning-types.md` | Unfamiliar warning code appears in queue output |
| `references/cli-reference.md` | Need the full CLI command table |
| `references/troubleshooting.md` | Batch commands fail, unicode issues, stale companions |
| `references/source-profiles.md` | Pattern-matching examples for recurring series |

## Verification

After a review pass:
1. `./server review stats` — queue size should be 0 or near-zero
2. No items older than a week remain (on production)
3. Report any remaining items with names and sources
