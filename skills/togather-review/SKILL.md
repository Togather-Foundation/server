---
name: togather-review
description: Guidebook for triaging the Togather SEL event review queue — warning types, batch strategies, source patterns, and CLI reference. Use when operating a Togather server and items are piling up in the review queue.
license: MIT
compatibility: Requires a Togather server binary built from source (make build). Requires admin API access (admin-role API key for token exchange).
metadata:
  author: togather-foundation
  version: "2.0"
  hermes:
    tags: [togather, review, data-quality, scraping, operations]
    category: devops
---

# Togather Review Queue Guidebook

Systematic approach to triaging the Togather SEL event review queue.

**THE RULE: This is data janitor work, NOT curation.** The review queue exists
to fix bad data and bad scrapes. Taste, interest, and personal preference are
handled downstream by the curation agent. Your ONLY job: is this event data
correct and does it describe an actual gathering?

**Approve when:** A real gathering is happening (concert, class, meetup,
seminar, storytime, workshop — yes, ALL of these). Data may be imperfect
(missing description, weird venue name) but the event is real.

**Reject when:**
- Wrong geography (not the city your instance serves)
- Not an event (closure notice, job posting, announcement, catalog entry)
- Garbage data (destroyed name, no usable information, wrong language)
- Non-events masquerading as events ("Wanted: Event Hosts", "Closed for the day")

## When to Use

- You operate a Togather SEL instance
- Items are piling up in the review queue (run `./server review stats`)
- You want to clear the queue efficiently (batch operations, source-first strategy)
- You're setting up a cron job for automated review passes

## Quick Start

The `server review` CLI is built into the Togather server binary. Build it first:

```bash
cd /path/to/togather-server
make build
```

Then source your admin API key and run:

```bash
source .env  # contains TOGATHER_ADMIN_API_KEY

# Get the lay of the land
./server review stats --server https://your-instance.example.com \
  --key "$TOGATHER_ADMIN_API_KEY"

# Spot patterns
./server review queue --group-by name --server ... --key ...
./server review queue --group-by source --server ... --key ...

# Write queue data to file (avoids terminal truncation)
./server review queue --limit 200 --json --output /tmp/togather_queue.json \
  --server ... --key ...
```

## Procedure

### Phase 1: Survey the Queue

Start by understanding what's in the queue at the source level, not the name
level. This reveals patterns faster.

```bash
./server review stats --server ... --key ...
./server review queue --group-by source --server ... --key ...
./server review queue --group-by name --server ... --key ...
```

Note the total count, age buckets, and source groupings. Write queue data
to a file for analysis:

```bash
./server review queue --limit 200 --json --output /tmp/queue.json \
  --server ... --key ...
```

### Phase 2: Check for Disabled-Source Orphans

Cross-reference the source groups against your `configs/sources/*.yaml`
directory. Sources with `enabled: false` may have left orphaned items
from before they were turned off. These need batch rejection by source:

```bash
./server review batch --source "<uuid>" --action reject \
  --reason "Source disabled — global feed with no city filter" --dry-run \
  --server ... --key ...
```

Do this early — it clears volume with a single command per source rather
than processing items individually.

### Phase 3: Identify Clean Sources

Pull items from each source with `--json` and inspect patterns. Source IDs
are stable between passes. Known-clean sources can be batch-approved quickly
using name substrings.

**Batch approve by source:**
```bash
./server review batch --source "<uuid>" --action approve \
  --notes "Real events, clean data" --dry-run --server ... --key ...
```

**Batch approve by name pattern:**
```bash
./server review batch --name "Symphony" --action approve \
  --notes "Orchestra events" --dry-run --server ... --key ...
```

Always `--dry-run` first. The count may differ from actual execution if items
were modified between check and run. 409 "already approved" errors are harmless.

### Phase 4: Handle Recurring Series

Recurring events (weekly seminars, regular meetups, open mics) get scraped
as separate events, each with a `cross_week_series_companion` warning.

**Two modes:**

**Normal weekly** (one new occurrence, primary still alive):
```bash
./server review merge <canonical-primary-ulid> <new-event-ulid> \
  --transfer-occurrences --server ... --key ...
```

**Backlog recovery** (many items, companion ULIDs point to deleted events):
If `merge-into-primary` fails with 422 "Event has been deleted", fall back:
```bash
./server review batch --name "Series Name" --action approve \
  --notes "Recurring — companions deleted, merge unavailable" \
  --server ... --key ...
```

### Phase 5: Process Remaining Items

Work through name groups from largest to smallest. For individual items,
inspect before deciding:

```bash
./server review check <id> --json --server ... --key ...
```

Reject junk with a descriptive reason:
```bash
./server review reject <id> --reason "not an event — job posting" \
  --server ... --key ...
```

### Phase 6: Verify

```bash
./server review stats --server ... --key ...
```

Confirm the queue is smaller or empty. Note any patterns for future passes.

## CLI Reference

| Task | Command |
|------|---------|
| Overview | `./server review stats` |
| List pending | `./server review queue` |
| Paginate | `./server review queue --limit 200 --offset 200` |
| Output to file | `./server review queue --json --output /tmp/queue.json` |
| Group by name | `./server review queue --group-by name` |
| Group by source | `./server review queue --group-by source` |
| Group by warning | `./server review queue --group-by warning` |
| Inspect one | `./server review check <id>` |
| Approve one | `./server review approve <id> --notes "..."` |
| Reject one | `./server review reject <id> --reason "..."` |
| Batch by source | `./server review batch --source <uuid> --action approve` |
| Batch by name | `./server review batch --name "substring" --action approve` |
| Batch by warning | `./server review batch --warning missing_description --action approve` |
| Merge into primary | `./server review merge <primary-ulid> <new-ulid> --transfer-occurrences` |
| Consolidate (3+ events) | `./server review consolidate <canonical-ulid> <dup1> <dup2>` |
| Fix dates | `./server review fix <id> --start-date 2026-06-15T19:00:00-04:00` |
| Dry-run any batch | append `--dry-run` to batch commands |

## Warning Types

### `missing_description`
Most common. Two cases:
- **Real event, scraper couldn't extract description** → approve. Check the URL
  in a browser; if the event is clearly real, publish it.
- **Junk event** → reject (caught by other patterns below).

### `cross_week_series_companion`
Recurring event where each occurrence was scraped as a separate event.
Weekly seminars, regular meetups, recurring shows.

**IMPORTANT: The `companion_ulid` in the warning is the NEAREST NEIGHBOR,
not the surviving canonical primary.** The warning says "this event matches
existing event X on date Y" — but X may have already been retired. Using X
as `--primary-id` will fail with 422.

To find the actual surviving primary, check your records from prior sessions.
If you don't have it, fall back to approving as an individual event.

### `place_possible_duplicate`
Same venue with slightly different name strings.
Usually safe to approve — the venue resolver deduplicates later.
Only reject if the place is clearly wrong (wrong city, wrong type).

### `potential_duplicate` / `near_duplicate_of_new_event`
Event might be a duplicate of an existing event.
- Truly identical → `consolidate`
- Different dates of same series → `merge` into primary
- Different events, similar names → approve with `record_not_duplicates`

### `multi_session_likely`
Event spans multiple dates/sessions. Usually legitimate for festivals,
conferences, multi-day workshops. Check the URL; if real → approve.

### `zero_duration_occurrence`
Start and end time are identical. Often a data entry error or the scraper
couldn't find an end time. If the event is otherwise real → approve.

## CLI Limitations (from real-world use)

- **Name substring matching is fragile** — apostrophes (both `'` and `'`),
  ampersands, and emoji flags can cause batch to silently skip items. Try
  alternative substrings.
- **JSON control characters** — unescaped HTML entities (`&amp;`, `&#39;`)
  may remain in event name and location fields. Use `json.loads(strict=False)`
  if parsing inline, and `html.unescape(text)` before matching.
- **`queue --limit` max is 200** — use `--offset N` for pagination.
- **No `--offset` pagination on older builds** — if `--offset` isn't recognized,
  update your server binary.
- **Batch `--name` was required on older builds** — if `--source` alone fails,
  update the binary. Newer builds accept `--source` or `--warning` standalone.

## Rules of Thumb

- **When in doubt, check the URL.** Browse to the event page. If it looks
  real, approve.
- **Missing description is not a reason to reject.** Real events with no
  description are better than no events at all.
- **Wrong city = instant reject.** Check the venue name for non-local locations.
- **Recurring isn't wrong.** Weekly seminars, open mics, and regular meetups
  are real events. Merge them, don't reject them.
- **Curation ≠ review.** The review queue is for data quality triage.
  Personal taste (no yoga, no library programs) is handled by the curation
  layer. Approve real events even if you wouldn't personally attend.
- **Note the source.** If the same source keeps producing bad data, flag it
  for scraper config improvement.
- **Age matters on production.** Prioritize older items so nothing sits in
  review forever. On staging, just clear the queue.

## Example Source Profiles

These are patterns observed on a real Togather instance. Your sources will
differ, but the patterns are instructive.

**Academic talks (university source):** Massive recurring series by design.
Weekly seminars scraped as separate events. ~165 items in a single pass,
all with `cross_week_series_companion`. Action: merge-into-primary or
approve as individuals.

**Comedy clubs:** Recurring weekly shows. ~52 items, same weekly-series
pattern as academic talks. Action: merge or approve as individuals.

**Performing arts venues:** Cleanest sources. Orchestras, ballet companies,
theatres produce structured data. Only `missing_description` warnings.
Action: batch-approve by name substring.

**Global ICS feeds (DISABLED):** Some scrapers ingest events from worldwide
feeds with no city filter. Items from New York, London, San Francisco end
up in the queue. If you find a source producing only wrong-city events,
disable it in config and batch-reject its orphans.

## Pitfalls

- **Disabled sources leave orphaned queue items**: Setting `enabled: false`
  stops new scrapes but does NOT clear items already in the queue. Before
  starting a full pass, check source groups against the YAML config directory.
- **merge-into-primary may fail on stale companions**: Items created before
  the stale-companion fix may have invalid companion ULIDs. Fallback to
  `--action approve` — the events are real, stale warnings are harmless.
- **Name substring matching is fragile**: Apostrophes, ampersands, and emoji
  can cause batch to silently skip items. Try alternative substrings.
- **JSON control characters**: Unescaped HTML entities may remain in event
  name fields. Use `json.loads(strict=False)` if parsing inline.
- **Recurring series are intentional**: Scrapers push every occurrence as a
  separate event. The review agent handles consolidation. This is by design —
  dumb scrapers, intelligent review.

## Verification

After a review pass:
1. `./server review stats` — queue size decreased
2. No items older than a week remain (on production)
3. No disabled-source orphans lurking
4. Recurring series consolidated or approved as individuals
