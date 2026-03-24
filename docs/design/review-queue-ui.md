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
5. **Duplicate resolution uses an atomic consolidate request.** The old "Add as Occurrence" / "Merge Duplicate" two-button model is replaced by a single **Consolidate** action. The flow is: (1) `DELETE /api/v1/admin/events/{ulid}/occurrences/{id}` for each excluded canonical occurrence, (2) `POST /api/v1/admin/events/{ulid}/occurrences` for each related occurrence the admin included, (3) `POST /api/v1/admin/events/consolidate` with `event_ulid` (canonical to promote) and optionally an `event` object carrying field overrides — field patches and promotion happen atomically in a single transaction. The old `/review-queue/{id}/add-occurrence` and `/review-queue/{id}/merge` endpoints are removed and must not be called.

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

**Field picker** — a 3-state chip table for each differing pickable field (`name`, `description`, `url`, `image`). The canonical event's chip is pre-highlighted (`btn-primary`); the other event's chip is outlined. Clicking a chip:
- Turns the clicked chip green (`btn-success`) — user's explicit selection
- Reverts the sibling chip to outline (`btn-outline-secondary`)
- Calls the `onPick` callback with `edited: false` (value unchanged, just selected)

Each blue or green chip shows a ✎ pencil icon on its right edge. Clicking the pencil:
- If the chip is **blue** (auto-canonical, not yet touched): atomically selects it (→ green) and opens the inline editor in one step.
- If the chip is **green** (already selected): opens the inline editor directly.
- The inline editor replaces the chip `<td>` with an `<input>` (single-line fields) or `<textarea>` (description), plus **Save** and **Cancel** buttons.
- **Save** commits the custom text, restores a green chip with the new value, and calls `onPick` with `edited: true`. **Cancel** restores the previous chip state.
- Pencil icons are `visibility:hidden` (not `display:none`) on outline chips so layout stays stable.

`location.name` and `organizer.name` are shown as read-only reference rows (no chips); their value is determined solely by the canonical selection.

**Occurrence picker** — two columns (This Event | Related Event), with one row per occurrence per event, strictly ordered by start time across both events. The date is the row label. There is no row-merging — if two occurrences share the same date, they each get their own row with their chip in their own column. If two related occurrences both overlap a single canonical one, they each still get their own row.

- **All chips** (canonical and related) are toggleable via `data-action="toggle-occurrence"`. Canonical occurrences can be excluded so they are deleted from the canonical event during consolidation.
- **Chip colour states:** Blue (`btn-primary`) = auto-included (not yet user-touched), Green (`btn-success`) = user explicitly locked in, Outline (`btn-outline-secondary`) = excluded.
- **Overlap conflict:** A ⚠ icon in a fixed 1.5rem slot on the inner edge (using `visibility:hidden` when no overlap so chip text stays centred) shows when `overlapsWith` is non-empty. Enabling an excluded chip is silently blocked client-side if any peer in its `overlapsWith` list is currently included. One chip per occurrence — never merged.

**Consolidate** — executes a three-step sequence:
1. **Delete excluded canonical occurrences**: for each canonical-side occurrence the admin excluded, `DELETE /api/v1/admin/events/{ulid}/occurrences/{id}`. Shows a toast on failure but continues.
2. **Add included non-canonical occurrences**: for each non-canonical occurrence marked included, `POST /api/v1/admin/events/{ulid}/occurrences`. On 409 (overlap) shows a toast error and continues; does not abort the overall operation.
3. **Promote + optional field patch**: `POST /api/v1/admin/events/consolidate` with `{ "event_ulid": "<canonical-ulid>", "retire": ["<other-ulid>"], "transfer_occurrences": false }` and, if the admin selected any field override from the non-canonical event, an `"event"` object carrying those overrides (`name`, `description`, `url`, `image`, `keywords`, `eventDomain`) — field patch and promotion are applied atomically in a single transaction. On success dismisses both review entries and reloads the queue.

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
- The ⚠ icon appears on the overlapping chip. The chip is still visible and included/excluded states still render, but enabling an excluded chip is silently blocked client-side if any peer in its `overlapsWith` list is currently included — the toggle is a no-op.
- If a `POST /api/v1/admin/events/{ulid}/occurrences` still returns 409 (e.g. due to a race), show a toast error: "Conflict: overlaps with existing occurrence." The overall Consolidate operation continues for non-conflicting occurrences.
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
| **Consolidate: delete excluded canonical occurrences** (per occurrence, step 1) | `DELETE /api/v1/admin/events/{ulid}/occurrences/{id}` |
| **Consolidate: add selected non-canonical occurrences** (per occurrence, step 2) | `POST /api/v1/admin/events/{ulid}/occurrences` |
| **Consolidate: promote + optional field patch** (step 3) | `POST /api/v1/admin/events/consolidate` with `event_ulid`, and optionally `event` (field overrides), `transfer_occurrences: false` |
| Not a Duplicate (approve both) | `POST /api/v1/admin/review-queue/{id}/approve` with `record_not_duplicates: true` |
| Approve (data quality only) | `POST /api/v1/admin/review-queue/{id}/approve` |
| Edit & Approve | `PUT /api/v1/admin/events/{ulid}` → `POST /api/v1/admin/review-queue/{id}/approve` |
| Reject | `POST /api/v1/admin/review-queue/{id}/reject` |

**Removed — do not use:**
- `POST /api/v1/admin/review-queue/{id}/add-occurrence` — removed
- `POST /api/v1/admin/review-queue/{id}/merge` — removed
- `POST /api/v1/admin/events/consolidate` with `event` only (create path) — never used from review queue UI; always use `event_ulid` (promote existing). Supplying `event` alongside `event_ulid` is valid and sends field overrides as an atomic patch on the promote path.

---

## Implementation Status

This section tracks what has been built vs. what remains. The "What Needs to Change" section below is the original plan — check this section first.

### Completed (srv-7roii)

**field-picker.js:**
- `onPick(fieldKey, subfieldKey, value, source, edited)` callback wired; `canonicalIndex` and `readOnlyFields` options implemented.
- 3-state chip colour: Blue (`btn-primary`) = canonical default, Green (`btn-success`) = user-selected, Outline = sibling/unselected. Sibling revert logic is row-scoped.
- Pencil icon (✎) inside blue and green chips. Clicking pencil on blue: atomically selects (→ green) + opens inline editor. Clicking pencil on green: opens inline editor directly.
- Inline editor: `<input>` for single-line fields, `<textarea>` for `description`. Enter to save (single-line), Escape to cancel. Save calls `onPick` with `edited: true`; plain chip click calls `onPick` with `edited: false`.
- `selectedOverrides` snapshot restored correctly when canonical radio changes; all user selections (not just `edited`) are preserved.

**occurrence-rendering.js:**
- `renderMergedPickerList(pickerEntries, entryId)` implemented. Both canonical and related chips use `data-action="toggle-occurrence"`. ⚠ icon uses fixed 1.5rem slot with `visibility:hidden` when no overlap. `w-100 text-center` on chips.

**review-queue.js:**
- `occurrencePicker` module-level state map; `buildOccurrencePicker` with symmetric `overlapsWith` cross-event annotation; `toggleOccurrence` with silent block on overlap conflict; `rebuildPickers` preserving `userToggled` across canonical radio changes.
- `consolidate()` three-step sequence: (1) delete excluded canonical occs, (2) add included non-canonical occs, (3) POST consolidate with `event_ulid` + optional `event` field-patch object (atomic promote+patch in one transaction).
- `imageUrl` bug fixed: `relatedEvent.image` → `relatedEvent.imageUrl` in both initial render and `rebuildPickers`.
- Old `Add as Occurrence` / `Merge Duplicate` buttons and associated API methods removed.

**api.js:** `API.events.occurrences.delete` available; no other changes needed.

**review-queue.js (Case 1 — single-event fold-down):**
- `Edit & Approve` button added to Case 1 action bar (between Approve and Fix Dates). Opens an inline form pre-populated from `currentEntryDetail.normalized`: name, description, public_url (via `normalized.url`), image_url (via `normalized.image`). On submit: builds patch from changed fields only → `PUT /admin/events/{ulid}` → `API.reviewQueue.approve(id, {})` directly (not via `approve()`, since the approve button is hidden). Inline error display on failure.
- `Fix Dates` button demoted to `btn-outline-secondary` (Edit & Approve covers general editing).

**ingest.go:**
- `endDate` in normalized payload snapshot now uses last occurrence `EndTime` (with fallback to first occurrence `EndTime`), not first occurrence `EndTime`. Fixes stale `endDate` on multi-occurrence events.

**review-queue.js — date display:**
- `startDate`/`endDate` are derived entirely from `event_occurrences`; the `events` table has no date columns. They appear in the normalized JSONB snapshot (ingest-time) and in `event_review_queue.event_start_time`/`event_end_time` (for sorting/filtering only). They are **not patchable** via `PUT /admin/events/{ulid}`.
- `startDate`/`endDate` removed from `field-picker.js` `TOP_LEVEL_FIELDS` — they are display-only.
- Three `normalizedWithDates` overlays added: (1) initial HTML render block (line ~706), (2) post-render DOM init block for field picker (line ~937), (3) `rebuildPickers()` (line ~1538). Each overlay computes `startDate`/`endDate` from live occurrences fetched from the server, ensuring the field picker always shows the correct series span even for old payload snapshots.
- `relatedAsNormalized` (side-by-side card) now uses last occurrence for `endDate` (not first).
- `relatedForPicker` in both the initial render post-DOM block and `rebuildPickers()` now uses last occurrence for `endDate`.
- `extractMergeFields` fallback (for old payload snapshots without top-level dates) now uses last occurrence for `endDate`.

### Deferred

- *(Nothing currently deferred — inline chip editing is implemented.)*

### Known Issues

- **`tx is closed` on third RS-11 consolidation** — backend bug in `internal/domain/events/admin_service.go` `consolidateRetireEvents` (~line 1714): soft-deleting the retired event fails with "tx is closed" when a third event pair is consolidated in the same session. Root cause not yet identified. Not a blocker for the UI redesign.

---

## What Needs to Change in the Current Implementation

> **Note:** Most items in this section are now **done**. See [Implementation Status](#implementation-status) above. Items remain here for traceability.

### field-picker.js

1. ✅ **Add `onPick` callback** — `options.onPick(fieldKey, subfieldKey, value, source, edited)`.
2. ✅ **3-state chip behaviour + inline editing** — clicking a chip selects it (green); pencil icon on blue/green chips opens an inline `<input>` or `<textarea>`. Clicking pencil on blue atomically selects + opens editor. Save calls `onPick` with `edited: true`; plain chip click calls `onPick` with `edited: false`.
3. ✅ **`canonicalIndex` option** — which event index (0 or 1) is canonical.
4. ✅ **`readOnlyFields` option** — fields rendered as plain text rows (no chips).
5. ✅ **New signature**: `renderFieldPickerTable(containerEl, events, options)` where `options = { canonicalIndex, overrides, onPick, readOnlyFields }`.

### occurrence-rendering.js

1. **New `renderMergedPickerList(pickerEntries, entryId)` function** — ✅ implemented. Two columns, one row per occurrence per event, ordered by start time. Chip colour: blue = auto-included, green = user-locked, outline = excluded. Both canonical and related chips have `data-action="toggle-occurrence"` (canonical chips are toggleable — excluding one causes it to be deleted from the canonical event during consolidation). ⚠ icon in fixed 1.5rem slot. The original spec said canonical chips had no toggle — this was revised during implementation.

### review-queue.js

**Remove:**
- `[⊕ Add as Occurrence]` and `[⊗ Merge Duplicate]` buttons and their `consolidateEvent()` dispatch
- `fieldOverrides` object and chip-picking logic (`'pick-field'` delegation case, `updateOverridesDisplay()`)
- The separate "Selected field overrides" card (`field-overrides-${id}`)

**Add:**
- ✅ Module-level state: `const occurrencePicker = {}` (entryId → sorted picker entries)
- ✅ `fieldOverrides` shape: `{ [entryId]: { [fieldKey]: { value, eventIndex, edited } } }` where `eventIndex` is the absolute event index (0 = this event, 1 = related event); patch is sent when `override.eventIndex !== canonicalIndex` (i.e. user chose the non-canonical event's value)
- ✅ Field picker card wired via `onPick` callback → `handleFieldPick(entryId, fieldKey, subfieldKey, value, source)`
- ✅ Occurrence picker card (`id="occurrence-picker-${safeId}"`) rendered via `OccurrenceRendering.renderMergedPickerList`
- ✅ Single `[Consolidate]` button (`data-action="consolidate"`), keeping `[Not a Duplicate]` and `[Reject]`
- ✅ Side-by-side event cards continue to show occurrences read-only (context only) via existing `OccurrenceRendering.renderList(..., false)`
- ✅ New event delegation cases: `'consolidate'`, `'toggle-occurrence'`, `'canonical-select'`
- ✅ New `consolidate(entryId)` async function — three steps:
  1. For each canonical-side occurrence that was excluded: `DELETE /api/v1/admin/events/{canonicalUlid}/occurrences/{id}` — toast on error, continue.
  2. For each non-canonical occurrence with `included === true`: `POST /api/v1/admin/events/{canonicalUlid}/occurrences` — toast on 409, continue.
  3. `POST /api/v1/admin/events/consolidate` with `{ event_ulid: canonicalUlid, retire: [retireUlid], transfer_occurrences: false }` and, if the admin selected any field value from the non-canonical event, an `event` object carrying the overrides (`name`, `description`, `url`, `image`, `keywords`, `eventDomain`) — field patch and promotion run atomically in a single transaction. Inline error on failure; on success: dismiss entries, `loadEntries()`
- ✅ Helper `buildOccurrencePicker(thisEventOccs, relatedEventOccs, canonicalIndex)` — merges, annotates source, computes symmetric `overlapsWith` via interval intersection, sorts by `startTime`; both sides default-included based on `canonicalIndex`
- ✅ `collapseDetail` update: `delete occurrencePicker[expandedId]`
- ✅ `API.reviewQueue.addOccurrence` and `API.reviewQueue.merge` removed (srv-bexw4).

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

*Last updated: 2026-03-24. Consolidate step reduced from 4 to 3: pre-flight PUT removed; field overrides sent inline as `event` in the consolidate body, applied atomically with promotion in a single transaction (srv-7roii, ccb6cbd). Inline chip editing implemented: pencil icon on blue/green chips; blue pencil atomically selects+opens editor; textarea for description field; edited flag threaded through onPick and consolidate patch logic. Edit & Approve added to Case 1 fold-down. Date display fixed: startDate/endDate now derived from live occurrences (last occurrence for endDate); removed from field picker chip rows. ingest.go endDate bug fixed. See srv-7roii.*
