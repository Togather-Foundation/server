# Warning Types

Reference for each warning code the review queue can generate. Load this
file when an unfamiliar warning appears in queue output.

## Source-level suppression

A source can stop selected benign warnings from gating review by listing them in
`suppress_review_warnings` in its YAML config (then `server scrape sync`). The
warning is still generated and passed through ingest — it just does not push that
event into `pending_review` (it is stored on any review entry that is created for
another reason). Canonical allowlist:
[Per-source review-warning suppression](https://github.com/Togather-Foundation/server/blob/main/docs/integration/scraper.md#per-source-review-warning-suppression)
— `cross_week_series_companion`, `low_confidence`, `missing_description`,
`missing_image`, `multi_session_likely`; the doc wins if copies differ. Everything
else is never suppressible — including `exact_duplicate`, `potential_duplicate`,
`near_duplicate_of_new_event`, `outside_geo_boundary`, `place_possible_duplicate`,
`org_possible_duplicate`, `reversed_dates_*`, `zero_duration_occurrence`,
`too_far_future`, `ambiguous_year_far_future` and the inert `link_check_failed` —
because an agent key must not be able to switch off duplicate, boundary or
date-plausibility safety checks.

Prefer suppression over repeated batch approval when a confirmed-legitimate source
trips the same benign warning on nearly every event. It is not retroactive: it only
affects future ingest, so clear the items already queued for that source.

## `missing_description`

**Suppressible** (benign content heuristic).

Most common. Two cases:
- **Real event, scraper couldn't extract description** → approve. Check the URL
  in a browser; if the event is clearly real, publish it.
- **Junk event** → reject (caught by other patterns).

## `cross_week_series_companion`

**Suppressible** (benign shape heuristic). Suppressing it also skips companion
cross-linking for that source.

Recurring event where each occurrence was scraped as a separate event.
Weekly seminars, regular meetups, recurring shows.

**The `companion_ulid` in the warning is the nearest neighbor, not the
surviving canonical primary.** The warning says "this event matches existing
event X on date Y" — but X may have already been retired.

**After the fix (commit 33ef8f55):** When events are consolidated, the system
refreshes all pending review items whose companion_ulid pointed to the retired
event, updating them with the canonical's data. The canonical ULID propagates
through the chain automatically.

If `merge <companion-ulid> <new-event-ulid> --transfer-occurrences` fails with
422 "Event has been deleted" (pre-fix orphaned companion row), that is unrelated
to weekly-series detection: skip that item and report it, or batch-approve it by
name if the event is otherwise fine. For a published weekly series from a
suppressed source, use `server review series` to find the cluster and
`server review consolidate ... --transfer-occurrences` to make it one event with
N occurrences. Batch approval only applies to entries already in the queue.

## `place_possible_duplicate`

**Never suppressible** (place safety).

Same venue with slightly different name strings. Usually safe to approve — the
venue resolver deduplicates later. Only reject if the place is clearly wrong
(wrong city, wrong type).

## `potential_duplicate` / `near_duplicate_of_new_event`

**Never suppressible** (duplicate safety).

Event might be a duplicate of an existing event.
- Truly identical → `consolidate`
- Different dates of same series → `merge` into primary
- Different events, similar names → approve with `record_not_duplicates`

## `multi_session_likely`

**Suppressible** (benign shape heuristic).

Event spans multiple dates/sessions. Usually legitimate for festivals,
conferences, multi-day workshops. Check the URL; if real → approve.

## `zero_duration_occurrence`

**Never suppressible** (date plausibility).

Start and end time are identical. Often a data entry error or the scraper
couldn't find an end time. If the event is otherwise real → approve.
