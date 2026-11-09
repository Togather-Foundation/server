---
name: togather-review
description: Guidebook for triaging the Togather SEL event review queue — warning types, batch strategies, source patterns, and CLI reference. Use when operating a Togather server and items are piling up in the review queue.
license: MIT
compatibility: Requires a Togather server binary built from source (make build). Requires admin API access (admin-role API key for token exchange).
metadata:
  author: togather-foundation
  version: "3.2"
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

The steps below are the routine path, not a checklist. You have the same
information an operator would; when the evidence disagrees with the guidance,
decide with your judgement and say what you saw. What matters is that every item
ends in a defensible decision — not that the commands ran in a fixed order.
Queue items are still handled in batches; see Hard Constraints.

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

**A weekly programme scraped as N separate events is usually ONE event with N
occurrences.** The public API renders the occurrences as `subEvent`s. Whether a
cluster should become one event is a judgement call, and you have the evidence
to make it. Sources that set `suppress_review_warnings: [cross_week_series_companion]`
publish these events directly, so they never enter the queue and Steps 2–3 will
not see them. Find them explicitly:

```bash
source .env && ./server review series --days 90 --key "$TOGATHER_ADMIN_API_KEY" --server "$TOGATHER_BASE_URL"
```

Work one source at a time when the list is long. `--source` accepts a provenance
source name (`tpl-events`) or UUID, and a scoped run **only sees that
source's events** — a cluster that spans sources is narrowed to the requested
source's members, so never retire members that a scoped run did not show. Historical
events may live on a differently-named provenance row after a feed rename, so if a
source looks empty, try its previous name as well. An event attributed to two
sources appears in **both** scopes, so a scoped consolidate can retire an event
that also carries another source's provenance — verify the member list.

To resolve a source UUID, fetch one event with `?include_provenance=true` to see
its source name, then run `./server review queue --group-by source` (or `--json`,
`sourceId`) to get the matching UUID.

```bash
source .env && ./server review series --source tpl-events --limit 20 \
  --key "$TOGATHER_ADMIN_API_KEY" --server "$TOGATHER_BASE_URL"
```

Page with `--offset`. When there are more clusters than fit in one page the header
prints `showing X-Y of N; next: --offset <N>`, for example:

```text
526 series candidate(s) (showing 201-220 of 526; next: --offset 220)
```

The detector covers **all published events** (unless `--source` scopes the run),
not only suppressed sources: it will also return series that Steps 2–3 just
approved. A series that appeared in the queue may already be partly consolidated,
so always inspect the member list before acting. `--json` emits the same cluster
data for scripting.

#### Gather the evidence

The detector only proposes clusters that meet these mechanical preconditions: one
primary venue, the same weekday, a 7-day step between linked members whose local
start times are within 30 minutes, and similar member names. Clusters are built
from pairwise links, so a long chain can drift more than 30 minutes end to end —
the header prints the first member's time, not a shared one. Different branches
cannot appear at all, and `--transfer-occurrences` keeps every occurrence. So the
only question left is the one a machine cannot answer: **are these events the
same programme, for the same audience, published once per week?**

For each cluster, **read every member name, date, and the description evidence**:

```text
1) Family Storytime — Main Branch — Wed 18:00 — 3 events — 2026-11-11..2026-11-25
   names differ: inspect carefully
   descriptions differ (2 distinct, 2/3 members described): decide, do not skip
     - Looking for something to do after school? Join us for stories, songs...
     - Family storytime with crafts and a snack. This event is full.
   undescribed: 1 of 3 members have no description; fetch their events before deciding
   [descriptions shortened to 200 chars for display; full text on the event]
   canonical: 01ABC...
   members:
     * 01ABC...  2026-11-11  Family Storytime
       01ABD...  2026-11-18  Family Storytime
       01ABE...  2026-11-25  Family Storytime (Toddler)
```

The evidence usually settles it. Typical cases:

- **One series** — same title, same audience, descriptions that differ only in
  logistics or wording: capacity notices (`N spots remaining`, `this event is
  full`), room or location suffixes, registration phrasing, restated dates,
  punctuation, casing, trimming. A recurring club, class, or storytime whose
  week-to-week variation is incidental belongs here.
- **Separate programmes** — titles that name different things (`Part 1`/`Part 2`,
  `Session 1`/`Session 3`, `I`/`II`/`III`, Beginners/Advanced); a different
  audience or participation mode (baby vs toddler vs teen, drop-in vs
  registered); or a description on each date that carries that date's content —
  a named film, guest, activity, or topic. Occurrences cannot carry per-date
  text, so merging those erases the detail a reader would use to choose a week.
- **Neither is automatic.** `descriptions differ (N distinct, ...)` is a reason to
  look, never a reason to skip: most of the time the difference is a capacity
  line. If the printed evidence is thin, truncated, or unclear, fetch each member
  event (`GET /api/v1/events/{ulid}` returns the full `description`, `url`,
  venue, and organizer) and open the source page when the payload still leaves
  you guessing.

Then preview and apply the merge:

```bash
source .env && ./server review consolidate <canonical> <member-ulids...> \
  --transfer-occurrences --dry-run --key "$TOGATHER_ADMIN_API_KEY" --server "$TOGATHER_BASE_URL"
source .env && ./server review consolidate <canonical> <member-ulids...> \
  --transfer-occurrences --key "$TOGATHER_ADMIN_API_KEY" --server "$TOGATHER_BASE_URL"
```

Every cluster ends in a decision: **consolidated**, or **kept separate with a
one-line reason**. Both are resolutions. Escalate only when the details genuinely
cannot separate two programmes, and name the missing detail — that is a
data-quality report, not a third kind of resolution.

**Work budget:** at most 20 consolidations per pass. That bounds the work, not
the decisions: carry the remainder to the next batch and report it as
outstanding.

This step is the deliberate exception to the batch rule in Hard Constraints:
consolidation is per-cluster and must be chosen cluster by cluster. Queue items
themselves still follow the batch rules in Steps 2–3.

### Step 5: Verify and report

```bash
source .env && ./server review stats --key "$TOGATHER_ADMIN_API_KEY" --server "$TOGATHER_BASE_URL"
source .env && ./server review queue --group-by source --key "$TOGATHER_ADMIN_API_KEY" --server "$TOGATHER_BASE_URL"
```

Report the final stats. If items remain, report them with their decision state
(undecided, kept separate with its reason, or blocked by missing evidence) and
their source. A non-zero queue is not by itself a reason to hand work to a human.

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
3. Report any remaining items with names, sources, and their decision state
