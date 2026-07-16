# Warning Types

Reference for each warning code the review queue can generate. Load this
file when an unfamiliar warning appears in queue output.

## `missing_description`

Most common. Two cases:
- **Real event, scraper couldn't extract description** → approve. Check the URL
  in a browser; if the event is clearly real, publish it.
- **Junk event** → reject (caught by other patterns).

## `cross_week_series_companion`

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
422 "Event has been deleted" (pre-fix orphaned items), fall back to batch
approval by name.

## `place_possible_duplicate`

Same venue with slightly different name strings. Usually safe to approve — the
venue resolver deduplicates later. Only reject if the place is clearly wrong
(wrong city, wrong type).

## `potential_duplicate` / `near_duplicate_of_new_event`

Event might be a duplicate of an existing event.
- Truly identical → `consolidate`
- Different dates of same series → `merge` into primary
- Different events, similar names → approve with `record_not_duplicates`

## `multi_session_likely`

Event spans multiple dates/sessions. Usually legitimate for festivals,
conferences, multi-day workshops. Check the URL; if real → approve.

## `zero_duration_occurrence`

Start and end time are identical. Often a data entry error or the scraper
couldn't find an end time. If the event is otherwise real → approve.
