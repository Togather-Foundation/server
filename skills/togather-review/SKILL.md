---
name: togather-review
description: Guidebook for triaging the Togather SEL event review queue — warning types, batch strategies, source patterns, and CLI reference. Use when operating a Togather server and items are piling up in the review queue.
license: MIT
compatibility: Requires a Togather server binary built from source (make build). Requires admin API access (admin-role API key for token exchange).
metadata:
  author: togather-foundation
  version: "3.1"
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

**Recurring-flood exception.** If a source legitimately generates the same benign
warning on nearly every event (e.g. a public-library calendar where
`cross_week_series_companion` fires on each recurring programme), repeated batch
approval is the wrong fix. Configure per-source `suppress_review_warnings` in the
source YAML, run `server scrape sync`, and let future runs publish without a review
row. Suppression is not retroactive: it only affects future ingest, so clear the
items already queued for that source
(`server review batch --source <name> --action approve`). Only the benign allowlist
can be suppressed (never duplicate, boundary or date-plausibility codes); warnings
are still generated, stored only if a review entry is created for another reason,
and a suppressed code that was present is audit-INFO-logged. See
[Per-source review-warning suppression](https://github.com/Togather-Foundation/server/blob/main/docs/integration/scraper.md#per-source-review-warning-suppression).

### Step 3: Batch approve remaining patterns by name

For any pending items not caught by Step 2, group by name and batch-approve
recurring series by name substring. Try the most prominent words first
(splitting on spaces). Handle unicode apostrophes with U+2019 character:

```bash
source .env && ./server review batch --name "substring" --action approve \
  --notes "Real recurring event" --key "$TOGATHER_ADMIN_API_KEY" \
  --server "$TOGATHER_BASE_URL"
```

### Step 4: Consolidate weekly series (including suppressed sources)

**A weekly programme scraped as N separate events is ONE event with N occurrences.**
Consolidating it is the correct end state: the public API renders the occurrences
as `subEvent`s. Sources that set `suppress_review_warnings: [cross_week_series_companion]`
publish these events directly, so they never enter the queue and Steps 2–3 will
not see them. Find them explicitly:

```bash
source .env && ./server review series --days 90 --key "$TOGATHER_ADMIN_API_KEY" --server "$TOGATHER_BASE_URL"
```

The detector covers **all published events**, not only suppressed sources: it will
also return series that Steps 2–3 just approved. A series that appeared in the
queue may already be partly consolidated, so always inspect the member list before
acting. `--json` emits the same cluster data for scripting.

For each cluster, **read every member name and date**:

```text
1) Family Storytime — Main Branch — Wed 18:00 — 3 events — 2026-11-11..2026-11-25
   names differ: inspect carefully
   canonical: 01ABC...
   members:
     * 01ABC...  2026-11-11  Family Storytime
       01ABD...  2026-11-18  Family Storytime
       01ABE...  2026-11-25  Family Storytime (Toddler)
```

**If member names or audiences diverge, do not consolidate.** The detector cannot
distinguish different age groups, drop-in vs registered sessions, or different
programmes that share a venue, weekday, and time. Only the member list can.

Then preview and apply the merge:

```bash
source .env && ./server review consolidate <canonical> <member-ulids...> \
  --transfer-occurrences --dry-run --key "$TOGATHER_ADMIN_API_KEY" --server "$TOGATHER_BASE_URL"
source .env && ./server review consolidate <canonical> <member-ulids...> \
  --transfer-occurrences --key "$TOGATHER_ADMIN_API_KEY" --server "$TOGATHER_BASE_URL"
```

**Consolidate ONLY when all of these are true:** same programme, same branch
(venue), same weekday, same start time, exact 7-day cadence, and every member name
describes the same audience.

**Do NOT consolidate:** different branches of the same library/system, different
age groups or audiences (baby vs toddler vs teen), drop-in vs registered sessions,
different weekday or start time, or titles that imply different programmes even when
the names are similar.

**Cap the work:** consolidate at most 20 clusters per pass. Report the remainder
with their names and venues instead of forcing ambiguous clusters.

This step is the deliberate exception to the "do not process items individually"
rule: consolidation is per-cluster and must be chosen cluster by cluster. Queue
items themselves still follow the batch rules in Steps 2–3.

### Step 5: Verify and report

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

## Guard allowlist

If the commands in this skill work in a plain shell but are blocked when you run
them from an agent gateway, a cron wrapper, or another sandboxed runner, the
runner's referenced-file guard is the likely cause. Some gateways inspect the
files a command references and fail closed when a referenced file is large. The
built `./server` binary is tens of megabytes, so every command here can trip that
guard.

To fix it, allowlist the repository or server path (or the built binary path) in
the gateway's referenced-file guard, or invoke the binary by an absolute path
that is already allowlisted. For example, an operator-managed guard entry for the
repository directory is enough for the `./server` form used throughout this
skill. If the gateway cannot be configured that way, run the workflow from a
plain shell instead — the review-pass logic is identical.

## Reference Files

Every path below is relative to **this skill's directory**, not to your working
directory or filesystem root. Load a file with whatever skill-file reader your
agent provides, rather than by resolving an absolute filesystem path.

Load on demand only when encountering an unfamiliar situation:

| File | When to load |
|------|-------------|
| `references/warning-types.md` | Unfamiliar warning code appears in queue output |
| `references/cli-reference.md` | Need the full CLI command table |
| `references/troubleshooting.md` | Batch commands fail, unicode issues, stale companions |
| `references/source-profiles.md` | Pattern-matching examples for recurring series |
| `references/high-volume-batch-strategies.md` | Queue is large and batch filtering needs a strategy (bulk approvals, mixed sources, caps) |

`scripts/review-pass.sh` is not a reference doc. It runs the standard pass:
survey (stats and source breakdown), then — with `--source <uuid>` — batch-approves
that one source (`--limit 200`, add `--dry-run` to preview) and reprints final
stats; without `--source` it reprints the remaining source breakdown and expects
you to batch per group. Its path is relative to the skill directory.

## Verification

After a review pass:
1. `./server review stats` — queue size should be 0 or near-zero
2. No items older than a week remain (on production)
3. Report any remaining items with names and sources
