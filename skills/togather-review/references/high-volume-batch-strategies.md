# High-Volume Batch Strategies

> **CLI FIXES LANDED June 9 2026 (commits fca23d53, 33ef8f55, f0191279):**
> - `--name` no longer required — batch works with `--source` or `--warning` standalone
> - `--offset N` added for pagination  
> - `--output FILE` added to queue and stats commands
> - Stale companion ULID fix: consolidate now refreshes pending review items pointing
>   to retired events — merge-into-primary 422 from stale companion refs is FIXED
> - Error messages include item ID, name, and parsed problem detail
>
> This doc has been updated to reflect the fixed state.

## Context

On June 9, 2026, a 599-item backlog was cleared from the Togather staging review queue in a single pass (~130 batch calls, ~2 hours of agent time). This doc captures the strategies that made it feasible.

## Strategy: Name-Substring Profiling (pre-fix, still useful for narrow targeting)

Before the `--source` standalone batch fix, `--name` was always required. For sources with diverse event names (TSO, Ballet, Jazz), you had to find name substrings. This is still useful when you want to approve a subset of a source:

```
# Identify a source with many pending items
./server review queue --group-by source

# Get all items from that source
./server review queue --source <uuid> --json --output /tmp/items.json

# Find common substrings
python3 -c "
import json
items = json.load(open('/tmp/items.json'))
names = [i.get('eventName','') for i in items]
from collections import Counter
words = Counter()
for n in names:
    for w in n.split()[:2]:
        words[w.lower()] += 1
for w, c in words.most_common(20):
    if c >= 2 and len(w) > 3:
        print(f'{w}: {c}')
"
```

## Proven Batch Patterns (worked on staging)

### Single-source, single-event items (TSO, Ballet, Jazz, NatGeo)
Each source has 1-54 items with unique names. No single substring covers all. Use multiple passes:

| Source | Substrings Used | Items Caught |
|--------|----------------|--------------|
| TSO (source 11f98d04) | `--name "Symphony"`, `--name "’"`, `--name "&"`, `--name "Plays"`, `--name "In Concert"`, `--name "The "`, `--name "Broadway"`, `--name "Holiday"`, `--name "Elden"`, `--name "Lunar"`, `--name "Samara"` | 51/51 |
| Ballet (source 99a16f47) | `--name "Dr."`, `--name "Romeo"`, `--name "Nutcracker"`, `--name "Emergence"`, `--name "Echoes"`, `--name "Erik"`, `--name "Sleeping"`, `--name "Beginning"` | 9/9 |
| Jazz (source 38289172) | `--name "Hiromi"`, `--name "Ibrahim"`, `--name "Kokoroko"`, `--name "Dip"`, `--name "Isaiah"`, `--name "Sullivan"`, `--name "Soul Rebels"`, `--name "Avishai"`, `--name "Laila"`, `--name "Stephane"`, `--name "Atlantic Jazz"`, `--name "Mei Semones"`, `--name "Gentiane"`, `--name "Kassa Overall"` | 17/17 |
| NatGeo (source 9c76893a) | `--name "National Geographic"` | 3/3 |

### Recurring series by name
Best handled individually by name since each group is a known event series:

| Name Pattern | Action | Count |
|-------------|--------|-------|
| `--name "Astro Coffee"` | approve | 62 |
| `--name "Pulsar Coffee"` | approve | 13 |
| `--name "Cosmology Discussion"` | approve | 13 |
| `--name "Scintillometry"` | approve | 13 |
| `--name "Gravitational Waves"` | approve | 13 |
| `--name "Murray Group"` | approve | 13 |
| `--name "Galaxy Journal Club"` | approve | 13 |
| `--name "CITA-PI"` | approve | 12 |
| `--name "Bi-Weekly Stars"` | approve | 7 |
| `--name "SMILE Seminar"` | approve | 6 |
| `--name "Tranzac Open Stage"` | approve | 17 |
| `--name "ToQue Trad"` | approve | 5 |
| `--name "The Pro Show"` | approve | 6 |
| `--name "HEADLINER"` | approve | 6 |
| `--name "COMEDY SLAM"` | approve | 6 |
| `--name "Laugh Sabbath"` | approve | 6 |
| `--name "Open Mic: The Bucket"` | approve | 6 |
| `--name "Late Mic T.O."` | approve | 5 |
| `--name "Her Ghost!"` | approve | 4 |
| `--name "Nice Time"` | approve | 4 |
| `--name "Comedy Bar: After Dark"` | approve | 3 |
| `--name "Wheel Of Comedy"` | approve | 3 |

### Broad social/meetup categories
These spanned multiple sources. A single `--name` caught everything with that substring:

| Name Substring | Caught | What |
|---------------|--------|------|
| `--name "Language Exchange"` | 24 | Multiple language exchange meetups |
| `--name "Salsa"` | 19 | All salsa/bachata dance events |
| `--name "FRC Toronto :: Saturday Run"` | 10 | Running club |
| `--name "20s 30s Social Party"` | 10 | Meetup social |
| `--name "Casual Board Games"` | 10 | Board game social |
| `--name "Learn Medieval"` | 10 | Medieval arts meetup |
| `--name "High Park Yoga"` | 9 | Yoga |
| `--name "Art Therapy"` | 7 | Art therapy workshop |
| `--name "Weedy Wednesdays"` | 5 | Community stewardship |
| `--name "Port Union Repair"` | 5 | Repair café |
| `--name "Fuego Bachata"` | 4 | Dance |
| `--name "East Weekend Gaming"` | 4 | Gaming |
| `--name "Mid-Town Weekend Gaming"` | 4 | Gaming |
| `--name "JB Piano Bar"` | 9 | Music series |
| `--name "time of the month"` | 4 | Tranzac singing circle |

### Apostrophe handling
HTML-encoded right single quotes (&#8217;) appear in event names. Use the unicode character directly:
- `--name "’"` (right single quote U+2019) — catches "Beethoven's", "Tchaikovsky's", etc.
- `--name "time of the month"` — works even though the full name uses a curly apostrophe and unicode chars

### 409 "already approved" errors
When batches overlap (same item matched by two different `--name` patterns), the second attempt gets a 409. These are harmless — the item was already approved. The batch continues processing the remaining items.

## Strategy: New — Batch by Source Standalone

With the `--name`-no-longer-required fix (commit fca23d53), you can now approve an entire source in one command:

```
# Approve everything from TSO source
./server review batch --source "11f98d04-f875-403f-92ff-d65eccd4dd7f" \
  --action approve --notes "TSO - real Toronto event" --dry-run

# Approve everything from Ballet source
./server review batch --source "99a16f47-e8f5-421a-98b0-5eeb85434def" \
  --action approve --notes "Nat Ballet - real Toronto event"
```

This replaces the old pattern of 9+ name-substring batches per source. The
`--source` filter is stable between passes — source UUIDs don't change.

Combine with `--warning` for narrower targeting:
```
# Approve only items with missing_description from a specific source
./server review batch --source "11f98d04-..." --warning "missing_description" \
  --action approve
```

Use `--limit` to cap a batch:
```
./server review batch --source "14a37f6d-..." --action approve --limit 50
```

## Two Modes: Normal Weekly Merge vs Backlog Recovery

This distinction is critical and was learned from the first real pass.

### Normal Weekly Mode (one merge per week)

When a single new weekly occurrence appears in the queue:
- The `cross_week_series_companion` warning fires correctly
- The stale-companion fix (commit 33ef8f55) ensures the `companion_ulid` points
  to the surviving canonical primary, not a retired event
- `./server review merge <companion-ulid> <new-event-ulid> --transfer-occurrences`
- This adds the new date as an occurrence and retires the new event
- The canonical primary stays alive for next week's merge

**The stale-companion fix propagates the canonical ULID**: when events are
consolidated, all pending review items whose companion_ulid matched the
now-retired event get updated with the canonical's data. The warning's
`companion_ulid` now stays current across merges.

### Backlog Recovery Mode (10+ weeks accumulated — LESS LIKELY after fix)

**Before the fix** (commits 33ef8f55 + f0191279): When the queue had accumulated many
weeks of unmerged occurrences, all companion ULIDs pointed to already-retired events
and merge-into-primary failed with 422 for every item.

**After the fix**: The consolidate endpoint refreshes stale companion ULIDs, so
backlog recovery should work via the normal merge workflow. Very old items
(pre-fix) may still have stale references and need individual approval.

If merge still fails: approve as individuals.
`./server review batch --name "Astro Coffee" --action approve --notes "Pre-fix orphan"`

This was the case on June 9, 2026 when 599 items had accumulated. The 62 Astro
Coffee items couldn't be merged because every companion ULID pointed to a
deleted event. With the fix, future backlogs won't have this problem.

### Cron Interruption Edge Case (FIXED — see companion ULID refresh above)

If a gateway restart kills a cron mid-run:
1. Some events may have been consolidated (retired) but their review items stay "pending"
2. **Before the fix** (commits 33ef8f55 + f0191279): Those review items had stale
   companion ULIDs and merge-into-primary would fail with 422.
3. **After the fix**: When the next consolidate happens, stale companion references
   are refreshed. Items from before the fix may still need individual approval.

**Recovery**: Check if cron ran by looking at `last_run_at`. If null, run the
review pass manually. The stale-companion fix handles most of the cleanup.

## companion_ulid ≠ Surviving Primary (UPDATED — fix landed)

> **STATUS: FIXED** — Commit 33ef8f55 ("fix(consolidate): update stale cross_week_series_companion on third-party reviews") now refreshes all pending review items whose companion_ulid points to a retired event, updating them with the surviving canonical's data. This means merge-into-primary should NOW WORK even with stale companions.

The `cross_week_series_companion` warning's `companion_ulid` field is the **nearest neighbor occurrence**, NOT the surviving canonical primary. Here's why that matters and how it works:

**How the warning is generated**: When a new event is scraped, the server detects it matches an existing recurring series and records the nearest match's ULID as `companion_ulid`. Example: event on June 8 matches event on May 18, so companion_ulid = May 18's ULID.

**What happens during merge**: The normal workflow is to merge the new event INTO the canonical primary (the earliest event in the series that all previous occurrences were merged into). After merge, the new event is retired (deleted). Its ULID no longer exists.

**Before the fix**: After repeated merges, every `companion_ulid` pointed to an already-retired event. merge-into-primary failed with 422 "Event has been deleted".

**After the fix (commit 33ef8f55)**: When events are consolidated, the system scans all pending review items and updates any whose `companion_ulid` matched the now-retired ULID, replacing it with the surviving canonical's data. This means the canonical ULID gets propagated through the chain of companions automatically. Next week's new event's warning will correctly reference the surviving canonical.

### Finding the surviving primary

Since the fix propagates the canonical ULID, you should be able to:
1. Request the full review item via `./server review check <id> --json`
2. Look at the `cross_week_series_companion` warning's `details.companion_ulid`
3. This should now point to the surviving canonical, not a retired event

If it still fails (very old items from before the fix was deployed), use the backlog recovery approach below.

### Backlog recovery (pre-fix orphaned items)

Items that were orphaned before commit 33ef8f55 may still have stale companion ULIDs. For those:
- merge-into-primary will fail with 422
- Approve as individuals: `./server review batch --name "Astro Coffee" --action approve --notes "Pre-fix orphan"`

### Error path reference (code traced June 2026)

Before the fix, the error chain was:
- `review_batch.go` sends `POST /api/v1/admin/events/consolidate` with `event_ulid` (your --primary-id)
- `admin_service.go` line 1623-1624 checks if canonical is deleted → returns `ErrEventDeleted`
- Handler maps to status 422 "Event has been deleted"
- Now: errors include item ID, name, and parsed problem detail (commit fca23d53)
