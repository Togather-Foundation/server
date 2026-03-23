# Review Queue UI Design

Admin UI for resolving events flagged for review. The review queue is the **primary admin workspace** — everything an admin needs to resolve a review entry must be accessible inline, without navigating away.

**Architecture doc:** `docs/architecture/event-review-workflow.md`  
**Regression test plan:** `docs/testing/review-regression-test-plan.md` — the RS-01 through RS-12 scenarios are the definitive set of cases this UI must handle.

---

## Design Principles

1. **No navigation required.** Clicking a review row unfolds it. All resolution actions happen inside the fold-down. Links to external sources are provided so admins can verify, but they do not need to leave the page.
2. **Show occurrences.** The existing occurrence list of the event under review — and any related events — must be shown. An admin cannot make a good dedup decision without knowing whether Event A already has 4 weekly dates and Event B is week 5.
3. **Show related events.** When a review entry has a duplicate-type warning, the related event(s) are fetched and shown side by side with field diffs highlighted.
4. **One action set, all warning types.** The UI offers the full set of resolution actions for every review entry. Availability of specific actions is gated by the backend (it returns 422 with a clear reason when an action is not applicable); the UI does not hide buttons based on warning codes, except where the backend has already rejected a path as ambiguous.
5. **Duplicate resolution uses a pre-flight patch + consolidate sequence.** The old "Add as Occurrence" / "Merge Duplicate" two-button model is replaced by a single **Consolidate** action. The flow is: (1) `PUT /api/v1/admin/events/{ulid}` if the admin selected any field overrides or edited field values, (2) `POST /api/v1/admin/events/{ulid}/occurrences` for each related occurrence the admin included, (3) `POST /api/v1/admin/events/consolidate` with `event_ulid` (promote existing, never create) and `transfer_occurrences: false`. The old `/review-queue/{id}/add-occurrence` and `/review-queue/{id}/merge` endpoints are removed and must not be called.

---

## Page Layout

Route: `/admin/review-queue`

```
+------------------------------------------------------------------+
| [Pending (18)] [Approved] [Rejected] [Merged]                    |
+------------------------------------------------------------------+
| EVENT NAME              | START TIME   | WARNING        | CREATED |
|-------------------------|--------------|----------------|---------|
| Weekly Yoga Base Series | Apr 5, 10 AM | Multi-session  | 2h ago  |
|  ▼ [fold-down — see below]                                       |
| Book Club Tuesday Eve.  | Apr 8, 7 PM  | Possible Dup   | 3h ago  |
| Jazz Night Late Show    | Apr 9, 11 PM | Reversed Dates | 1d ago  |
+------------------------------------------------------------------+
```

The list table, status tabs, and pagination are unchanged from the current implementation.

---

## Fold-Down: Single Event (data quality warnings only)

For entries with no duplicate-type warning (e.g. `multi_session_likely`, `reversed_dates_timezone_likely`, `missing_description`).

```
+------------------------------------------------------------------+
| WARNING BANNER                                                    |
|   ⚠ Multi-session likely: title contains "Weekly"               |
|   ⚠ Missing description                                          |
+------------------------------------------------------------------+
| EVENT INFORMATION                          [Edit] [Source ↗]    |
|   Name:        Weekly Yoga Base Series                           |
|   Date:        Saturdays Apr 5 – May 3, 2026                    |
|   Venue:       High Park Forest School [maps ↗]                  |
|   Organizer:   High Park Nature Centre                           |
|   URL:         https://eventbrite.com/... [↗]                    |
|   Description: —                                                 |
|                                                                  |
|   OCCURRENCES (4)                                                |
|   ┌─────────────────────────────────────────┐                   |
|   │ Apr 5, 10:00 AM – 11:30 AM              │                   |
|   │ Apr 12, 10:00 AM – 11:30 AM             │                   |
|   │ Apr 19, 10:00 AM – 11:30 AM             │                   |
|   │ Apr 26, 10:00 AM – 11:30 AM             │                   |
|   └─────────────────────────────────────────┘                   |
+------------------------------------------------------------------+
| NORMALIZED CHANGES (if any)                                      |
|   endDate: Mar 31 02:00 → Apr 1 02:00 (timezone correction)     |
+------------------------------------------------------------------+
| ACTIONS                                                          |
|   [✓ Approve]  [✎ Edit & Approve]  [✕ Reject]                  |
+------------------------------------------------------------------+
```

**Edit & Approve** opens an inline form (within the fold-down) pre-populated from the normalized payload, allowing the admin to correct any fields before approving. Submits via `PUT /api/v1/admin/events/{ulid}` then `POST /review-queue/{id}/approve`.

---

## Fold-Down: Event with Related Events (duplicate-type warnings)

For entries with `potential_duplicate`, `near_duplicate_of_new_event`, `place_possible_duplicate`, or `org_possible_duplicate` warnings. The related event(s) are fetched via `GET /api/v1/events/{ulid}` (public endpoint) when the fold-down is expanded.

```
+------------------------------------------------------------------+
| WARNING BANNER                                                    |
|   ⚠ Possible duplicate: found 1 similar event at same venue/date |
|   Near-duplicate of: [01KKJGG3... ↗]                            |
+------------------------------------------------------------------+
| THIS EVENT                    | RELATED EVENT                    |
| Book Club Tuesday Evening     | Book Club Tuesday Night          |
| Apr 8, 7:00 PM – 9:00 PM     | Apr 8, 7:00 PM – 9:00 PM        |
| ══════ DIFF: name ════════    | ══════ DIFF: name ════════       |
| Venue: Bento Sushi ✓ same    | Venue: Bento Sushi ✓ same       |
| Org:   Meetup                 | Org:   BlogTO                    |
| URL:   meetup.com/... [↗]    | URL:   blogto.com/... [↗]        |
| Desc:  Join us for our ...   | Desc:  —                         |
|                               |                                  |
| OCCURRENCES (2)               | OCCURRENCES (1)                  |
| Apr 8,  7:00 PM – 9:00 PM   | Apr 8,  7:00 PM – 9:00 PM       |
| Apr 15, 7:00 PM – 9:00 PM   |                                  |
+------------------------------------------------------------------+
| FIELD PICKER                                                     |
|   Which value to use for each differing field:                   |
|                                                                  |
|   Name:  [Book Club Tuesday Evening ✓] [Book Club Tuesday Night] |
|   URL:   [meetup.com/... ✓]            [blogto.com/...]          |
|   Desc:  [Join us for our... ✓]        [—]                       |
|                                                                  |
|   Venue:     Bento Sushi  (read-only, set by canonical choice)   |
|   Organizer: Meetup       (read-only, set by canonical choice)   |
|                                                                  |
|   Clicking a chip converts it to an inline editable input.       |
|   The canonical event's chip is pre-selected (btn-primary).      |
+------------------------------------------------------------------+
| OCCURRENCE PICKER                                                |
|   Which occurrences to include? (sorted by start time)          |
|                    THIS EVENT              RELATED EVENT         |
|   Apr 8    [Apr 8, 7:00–9:00 PM  🔒]                           |
|   Apr 8                            [⚠ Apr 8, 7:00–9:00 PM]    |
|   Apr 8    [Apr 8, 10:00–11:00 AM 🔒]                          |
|   Apr 15   [Apr 15, 7:00–9:00 PM 🔒]                           |
|   Apr 22                           [Apr 22, 7:00–9:00 PM]      |
+------------------------------------------------------------------+
| RESOLUTION                                                       |
|                                                                  |
| Which event is canonical?                                        |
|   (●) This event  ( ) Related event                             |
|                                                                  |
|   [⊕ Consolidate]  [≠ Not a Duplicate]  [✕ Reject This Event]  |
+------------------------------------------------------------------+
```

**Canonical selection radio** controls which ULID goes into `event_ulid` in the consolidate request and which goes into `retire`. Changing the selection rebuilds both the field picker (swapping the "pre-selected" chip) and the occurrence picker (swapping which occurrences are locked vs. toggleable).

**Field picker** — a 3-state chip table for each differing pickable field (`name`, `description`, `url`, `image`). The canonical event's chip is pre-highlighted (`btn-primary`); the other event's chip is outlined. Clicking either chip:
- Makes that chip's value the selected override (calls `onPick` callback)
- Converts the chip in-place to an inline `<input>` or `<textarea>` so the admin can edit the value
- Reverts the sibling chip to outline
`location.name` and `organizer.name` are shown as read-only reference rows (no chips); their value is determined solely by the canonical selection.

**Occurrence picker** — two columns (This Event | Related Event), with one row per occurrence per event, strictly ordered by start time across both events. The date is the row label. There is no row-merging — if two occurrences share the same date, they each get their own row with their chip in their own column. If two related occurrences both overlap a single canonical one, they each still get their own row.

- **Canonical chip**: `btn-primary`, lock icon, full date+time label, no toggle.
- **Related chip (no overlap)**: full date+time label; `btn-primary` if included, `btn-outline-secondary` if excluded; `data-action="toggle-occurrence"`.
- **Related chip (overlap)**: greyed ⚠ chip, full date+time label, not toggleable. One chip per overlapping occurrence — never merged.

**Consolidate** — executes a three-step sequence:
1. If any field override has `source === 'related'` or `edited === true`: `PUT /api/v1/admin/events/{ulid}` with the patched fields (`name`, `description`, `public_url`, `image_url` — venue and organizer are not patchable via this endpoint)
2. For each related occurrence the admin included: `POST /api/v1/admin/events/{ulid}/occurrences` — on 409 (overlap) shows an inline error on that occurrence row, does not abort the full operation
3. `POST /api/v1/admin/events/consolidate` with `{ "event_ulid": "<canonical-ulid>", "retire": ["<other-ulid>"], "transfer_occurrences": false }` — on success dismisses both entries and reloads the queue

**Not a Duplicate** — calls `POST /api/v1/admin/review-queue/{id}/approve` with `{ "record_not_duplicates": true }`. Both events are published. A `not_duplicates` record is created to suppress future false positives.

**Reject This Event** — calls `POST /api/v1/admin/review-queue/{id}/reject` with a required reason. Opens an inline reason textarea before confirming.

---

## Key Behaviour Details

### Occurrences are always shown

The fold-down fetches the live event via `GET /api/v1/events/{ulid}` (public endpoint) when expanded. The `subEvent` array contains all occurrences. These are rendered in a compact list (date, start time – end time, venue name if different from the primary). This is the authoritative source — not the `normalized` payload, which is a snapshot from ingest time.

For related events, the same fetch is performed for each related event ULID found in the warning `details.matches` array.

### Canonical selection for multi-event scenarios

The canonical radio applies to all duplicate-resolution actions. Default: "This event" (the one whose review row was expanded). If the admin selects "Related event" as canonical:
- Add as Occurrence: this event's occurrence is absorbed into the related event
- Merge Duplicate: this event is retired, related event survives

### multi_session_likely — three cases

`multi_session_likely` is not a different category from `potential_duplicate`. It is a stronger signal that an event is likely a recurring series. Three cases:

**Case 1 — Standalone** (e.g. RS-09 Film Screening, a single event with "(8 sessions)" in the title): The system detected that *one* event internally describes multiple sessions. There is no companion ULID, no linked duplicate. The fold-down uses the single-event layout and shows the description prominently so the admin can read it and decide:
- **Approve as-is** — it's legitimately one long event.
- **Add occurrences inline** — the fold-down exposes an occurrence editor (date/time inputs, add row, remove row) backed by `POST /api/v1/admin/events/{ulid}/occurrences`. Admin reads the description ("every Tuesday for 8 weeks"), adds those dates, then approves. The original single long-duration occurrence may need to be deleted first via `DELETE /api/v1/admin/events/{ulid}/occurrences/{id}` — the UI should offer a "remove" control on existing occurrences.
- **Reject** — the event should not be published; admin will ingest the individual sessions separately.

No Add as Occurrence / Merge buttons — there is no other event to pair with.

**Case 2 — With a companion** (e.g. RS-01 two Weekly Yoga events): Two events are both flagged `multi_session_likely`, and one has a companion ULID set (`DuplicateOfEventULID`), or both a `potential_duplicate` / `near_duplicate_of_new_event` warning co-exists. Treat identically to a `potential_duplicate` pair. Show both events side by side, offer Add as Occurrence / Merge / Not a Duplicate. The `multi_session_likely` is additional signal that they are the same series.

**Case 3 — Both**: `multi_session_likely` + `near_duplicate_of_new_event` on the same entry. Treat as Case 2.

The UI determines which case applies by checking whether a companion ULID exists in the `relatedEvents` array returned by the detail endpoint. If `relatedEvents` is empty → Case 1. If non-empty → Case 2/3.

### Overlap conflict (RS-05)

When a related occurrence overlaps a canonical occurrence:
- The occurrence picker replaces the related chip with a greyed ⚠ badge showing "overlaps [conflicting time]" — computed client-side before any API call. The row is still visible; only the chip is replaced.
- If a `POST /api/v1/admin/events/{ulid}/occurrences` still returns 409 (e.g. due to a race), show an inline error on that row: "Conflict: overlaps [time] on canonical event."
- The overall Consolidate operation continues for non-conflicting occurrences; only the conflicting row is marked failed inline.
- The admin does not need to "use Merge Duplicate instead" — there is no Merge Duplicate button. The occurrence picker is the mechanism for handling overlap.

### Multi-event clusters (RS-11)

When a review entry's related event is itself paired with a third event (e.g. Pottery Studio same-day cluster), the UI shows only the directly linked pair — it does not recurse into secondary links. The admin resolves pairs one at a time, which is sufficient. The consolidate API handles N-to-1 retirement if needed via the standalone page.

---

## API Usage Summary

| Action | API Call |
|--------|----------|
| Load review queue list | `GET /api/v1/admin/review-queue?status=pending` |
| Expand fold-down (detail) | `GET /api/v1/admin/review-queue/{id}` |
| Fetch live event data + occurrences | `GET /api/v1/events/{ulid}` (public) |
| Fetch related event data + occurrences | `GET /api/v1/events/{ulid}` (public, per warning match ULID) |
| **Consolidate: patch field overrides** (conditional, step 1) | `PUT /api/v1/admin/events/{ulid}` |
| **Consolidate: add selected related occurrences** (per occurrence, step 2) | `POST /api/v1/admin/events/{ulid}/occurrences` |
| **Consolidate: retire duplicate** (step 3) | `POST /api/v1/admin/events/consolidate` with `event_ulid`, `transfer_occurrences: false` |
| Not a Duplicate (approve both) | `POST /api/v1/admin/review-queue/{id}/approve` with `record_not_duplicates: true` |
| Approve (data quality only) | `POST /api/v1/admin/review-queue/{id}/approve` |
| Edit & Approve | `PUT /api/v1/admin/events/{ulid}` → `POST /api/v1/admin/review-queue/{id}/approve` |
| Reject | `POST /api/v1/admin/review-queue/{id}/reject` |

**Removed — do not use:**
- `POST /api/v1/admin/review-queue/{id}/add-occurrence` — removed
- `POST /api/v1/admin/review-queue/{id}/merge` — removed
- `POST /api/v1/admin/events/consolidate` with `event:` create path — never used from review queue UI; always use `event_ulid` (promote existing)

---

## What Needs to Change in the Current Implementation

### field-picker.js

1. **Add `onPick` callback** — new option `options.onPick(fieldKey, subfieldKey, value, source)` replaces the `data-action="pick-field"` delegation approach, keeping the module self-contained.
2. **3-state chip behaviour** — clicking any chip converts it in-place to an `<input>` or `<textarea>` (inline editable); sibling chip reverts to outline. Canonical chip is pre-selected (`btn-primary`).
3. **`canonicalIndex` option** — which event index (0 or 1) is canonical; controls which chips are pre-highlighted. Defaults to 0 for backward compatibility.
4. **`readOnlyFields` option** — fields in this set (e.g. `location.name`, `organizer.name`) are rendered as plain text rows (no chips).
5. **New signature**: `renderFieldPickerTable(containerEl, events, options)` where `options = { canonicalIndex, overrides, onPick, readOnlyFields }`. Must remain backward-compatible: `consolidate.js` calls without `options` → defaults to old behaviour.

### occurrence-rendering.js

1. **New `renderMergedPickerList(pickerEntries, entryId)` function** — two columns (canonical | related), one row per occurrence per event, ordered by start time. No row-merging. The date is the row label; the chip contains the full date+time.
   - **Canonical chip**: `btn-primary`, lock icon, full date+time label, no `data-action`.
   - **Related chip (no overlap)**: `btn-primary` if included, `btn-outline-secondary` if excluded; full date+time label; `data-action="toggle-occurrence"` + `data-entry-id` + `data-occ-key`.
   - **Related chip (overlap)**: greyed ⚠ `btn-secondary disabled`, full date+time label, no `data-action`. One chip per occurrence — never merged even if multiple related occurrences overlap the same canonical one.

### review-queue.js

**Remove:**
- `[⊕ Add as Occurrence]` and `[⊗ Merge Duplicate]` buttons and their `consolidateEvent()` dispatch
- `fieldOverrides` object and chip-picking logic (`'pick-field'` delegation case, `updateOverridesDisplay()`)
- The separate "Selected field overrides" card (`field-overrides-${id}`)

**Add:**
- Module-level state: `const occurrencePicker = {}` (entryId → sorted picker entries)
- `fieldOverrides` shape: `{ [entryId]: { [fieldKey]: { value, source, edited } } }` where `source: 'this'` = canonical, `source: 'related'` = other event's value; `edited: true` = user modified inline
- Field picker card wired via `onPick` callback → `handleFieldPick(entryId, fieldKey, subfieldKey, value, source)`
- Occurrence picker card (`id="occurrence-picker-${safeId}"`) rendered via `OccurrenceRendering.renderMergedPickerList`
- Single `[Consolidate]` button (`data-action="consolidate"`), keeping `[Not a Duplicate]` and `[Reject]`
- Side-by-side event cards continue to show occurrences read-only (context only) via existing `OccurrenceRendering.renderList(..., false)`
- New event delegation cases: `'consolidate'`, `'toggle-occurrence'`, `'canonical-select'`
- New `consolidate(entryId)` async function:
  1. Determine `canonicalUlid` / `retireUlid` from canonical radio + `currentEntryDetail`
  2. Build field patch from `fieldOverrides` where `source === 'related'` OR `edited === true` — map `name→name`, `description→description`, `url→public_url`, `image→image_url`; skip `location.name`, `organizer.name`
  3. If patch non-empty: `PUT /api/v1/admin/events/{canonicalUlid}` — inline error on fail
  4. For each `occurrencePicker` entry where `source === 'related'` AND `included === true`: `POST /api/v1/admin/events/{canonicalUlid}/occurrences` — inline error on 409, continue
  5. `POST /api/v1/admin/events/consolidate` with `{ event_ulid: canonicalUlid, retire: [retireUlid], transfer_occurrences: false }` — inline error on failure; on success: dismiss entries, `loadEntries()`
- Helper `buildOccurrencePicker(canonicalOccs, relatedOccs)` — merges, annotates source, computes overlaps via interval intersection, sorts by `startTime`
- `collapseDetail` update: `delete occurrencePicker[expandedId]`
- Completed: `API.reviewQueue.addOccurrence` and `API.reviewQueue.merge` removed (srv-bexw4).

### api.js

- No changes needed. `API.events.update`, `API.events.consolidate`, and `API.events.occurrences.create` already exist.
- Completed: `API.reviewQueue.addOccurrence` and `API.reviewQueue.merge` method references removed (srv-bexw4).

### review_queue.html

- No structural changes needed. The fold-down content is entirely JS-rendered.

### Backend (admin_review_queue.go)

- The `/review-queue/{id}/add-occurrence` and `/review-queue/{id}/merge` handler methods, routes, tests, OpenAPI entries, and all references in scripts/docs must be **fully removed in srv-bexw4**.
- No new backend work required for the fold-down redesign — all needed data is available via existing endpoints.

---

## What Stays Unchanged

- List table columns, status tabs, pagination, filter behaviour
- Approve (data quality only) flow
- Reject flow  
- Fix Dates flow (for `reversed_dates_*` warnings)
- The standalone `/admin/events/consolidate` page — kept as a fallback for cases outside the review queue (e.g. events that published cleanly but later discovered to be duplicates)

---

## Warning Code Reference

| Code | Type | Fold-down mode | Available actions |
|------|------|----------------|-------------------|
| `reversed_dates_timezone_likely` | data quality | single-event | Approve, Fix Dates, Reject |
| `reversed_dates_corrected_needs_review` | data quality | single-event | Approve, Fix Dates, Reject |
| `missing_description` | data quality | single-event | Approve, Edit & Approve, Reject |
| `missing_image` | data quality | single-event | Approve, Edit & Approve, Reject |
| `multi_session_likely` | data quality | single-event | Approve, Reject (Add as Occurrence shown but may 422) |
| `low_confidence` | data quality | single-event | Approve, Edit & Approve, Reject |
| `potential_duplicate` | duplicate | multi-event | **Consolidate**, Not a Duplicate, Reject |
| `near_duplicate_of_new_event` | duplicate | multi-event | **Consolidate**, Not a Duplicate, Reject |
| `place_possible_duplicate` | duplicate | multi-event | Not a Duplicate, Reject (Consolidate not applicable for place dups) |
| `org_possible_duplicate` | duplicate | multi-event | Not a Duplicate, Reject (Consolidate not applicable for org dups) |

---

*Last updated: 2026-03-23. Redesigned duplicate fold-down: replaced Add as Occurrence + Merge Duplicate with 3-state field picker, unified occurrence picker, and single Consolidate action (srv-7roii). Supersedes the 2026-03-19 version.*
