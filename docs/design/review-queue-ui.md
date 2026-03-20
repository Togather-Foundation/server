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
5. **All mutations go through the consolidate API.** `POST /api/v1/admin/events/consolidate` is the single path for Add as Occurrence and Merge Duplicate. The old `/review-queue/{id}/add-occurrence` and `/review-queue/{id}/merge` endpoints are deprecated and must not be called from new UI code.

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
| RESOLUTION                                                       |
|                                                                  |
| Which event is canonical?                                        |
|   (●) This event  ( ) Related event                             |
|                                                                  |
| Action:                                                          |
|   [⊕ Add as Occurrence]  — absorb related into this event       |
|   [⊗ Merge Duplicate]    — retire related, keep this as-is      |
|   [≠ Not a Duplicate]    — approve both as separate events       |
|   [✕ Reject This Event]  — delete this event                    |
+------------------------------------------------------------------+
```

**Canonical selection radio** controls which ULID goes into `event_ulid` in the consolidate request and which goes into `retire`. The label updates dynamically ("absorb related into **this event**" / "absorb **this event** into related").

**Add as Occurrence** — calls `POST /api/v1/admin/events/consolidate` with:
```json
{ "event_ulid": "<canonical-ulid>", "retire": ["<other-ulid>"] }
```
The consolidate API atomically: retires the non-canonical event (tombstone), adds its occurrence to the canonical, dismisses both companion review entries. On 409 (overlap conflict) the UI shows the conflicting occurrence times and suggests the admin either choose "Merge Duplicate" instead or manually fix the occurrence after approving.

**Merge Duplicate** — same consolidate call. Semantically: the two events represent the same real-world event listed twice (not the same series on different dates). The canonical keeps its data; the retired event's data is only used for gap-filling. On 409 this action is not affected (no occurrence movement).

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

### multi_session_likely with a companion event

RS-01 scenario: two events with `multi_session_likely`, no `potential_duplicate`. The admin can see both in the queue. When expanded, each fold-down shows only the single-event layout (no related event panel, since no near-dup warning links them). The admin's path:
- Approve both as separate events if they are genuinely different
- OR: use the companion's ULID manually via the standalone `/admin/events/consolidate` page to combine them

This is an acceptable limitation. The standalone consolidate page exists precisely for cases not driven by the review queue. When multi-session detection fires on two events that are actually the same recurring series, the admin navigates to `/admin/events/consolidate`, pastes both ULIDs, and consolidates from there.

> **Future improvement:** if the `multi_session_likely` events share the same venue and similar name, surface a "possible companion" banner in the fold-down with a link pre-populating the consolidate page.

### Overlap conflict (RS-05)

When Add as Occurrence returns 409:
- Show an inline error: "The occurrence at [time] conflicts with an existing occurrence on [canonical event name] at [conflicting time]."
- Suggest: "Use Merge Duplicate instead (if this is the same session listed twice), or Reject this entry and manually adjust the occurrence times."
- Do not close the fold-down; keep the resolution panel visible.

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
| **Add as Occurrence** | `POST /api/v1/admin/events/consolidate` |
| **Merge Duplicate** | `POST /api/v1/admin/events/consolidate` |
| Not a Duplicate (approve both) | `POST /api/v1/admin/review-queue/{id}/approve` with `record_not_duplicates: true` |
| Approve (data quality only) | `POST /api/v1/admin/review-queue/{id}/approve` |
| Edit & Approve | `PUT /api/v1/admin/events/{ulid}` → `POST /api/v1/admin/review-queue/{id}/approve` |
| Reject | `POST /api/v1/admin/review-queue/{id}/reject` |

**Deprecated — do not call from new UI code:**
- `POST /api/v1/admin/review-queue/{id}/add-occurrence` — replaced by consolidate
- `POST /api/v1/admin/review-queue/{id}/merge` — replaced by consolidate

---

## What Needs to Change in the Current Implementation

### review-queue.js

1. **Show occurrences** in the Event Information panel. After fetching `GET /api/v1/events/{ulid}`, render `event.subEvent` (or `event.occurrences`) as a compact list below the event fields.
2. **Show related event panel** with its occurrences. Fetch the related event ULID from `warning.details.matches[0].ulid` or `review.duplicateOfEventUlid`.
3. **Wire "Add as Occurrence" to consolidate API** (`API.events.consolidate`) instead of `API.reviewQueue.addOccurrence`. Build request from canonical radio selection.
4. **Wire "Merge Duplicate" to consolidate API** instead of `API.reviewQueue.merge`. Same consolidate call, different label/intent.
5. **Add canonical selection radio** above the action buttons when a related event is present.
6. **Handle 409 overlap** from consolidate with an inline error message and suggested next steps.
7. **Show "Add as Occurrence" for multi_session_likely** entries too — the backend will return 422 with `unsupported-review-for-occurrence` if the warning type doesn't support it, and the UI should surface that message. Don't suppress the button on the frontend based on warning code.
8. **Remove** calls to deprecated endpoints `API.reviewQueue.addOccurrence` and `API.reviewQueue.merge`.

### api.js

- Remove or deprecate `API.reviewQueue.addOccurrence` and `API.reviewQueue.merge` method references once review-queue.js is updated.

### review_queue.html

- No structural changes needed. The fold-down content is entirely JS-rendered.

### Backend (admin_review_queue.go)

- The `/review-queue/{id}/add-occurrence` and `/review-queue/{id}/merge` endpoints can remain for backward compatibility (CLI, scripts) but are deprecated for UI use.
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
| `potential_duplicate` | duplicate | multi-event | Add as Occurrence, Merge Duplicate, Not a Duplicate, Reject |
| `near_duplicate_of_new_event` | duplicate | multi-event | Add as Occurrence, Merge Duplicate, Not a Duplicate, Reject |
| `place_possible_duplicate` | duplicate | multi-event | Not a Duplicate, Reject (Add/Merge not applicable for place dups) |
| `org_possible_duplicate` | duplicate | multi-event | Not a Duplicate, Reject (Add/Merge not applicable for org dups) |

---

*Last updated: 2026-03-19. Supersedes the previous version of this file which predated the consolidate API.*
