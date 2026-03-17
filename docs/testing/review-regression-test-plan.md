# Review Queue Regression Test Plan

Manual regression test plan for the event review queue, covering duplicate detection, add-occurrence, merge, approve, reject, and near-duplicate workflows. Uses the `--review-events` fixture set (11 scenario groups, 22 events).

**Fixture source:** `tests/testdata/fixtures.go` (`BatchReviewEventInputs`)
**Generate command:** `server generate review-fixtures.json --review-events`
**Helper script:** `scripts/review-regression-test.sh`

---

## Prerequisites

1. Server running locally (`make run`) or staging deployment.
2. Admin account with `admin` role.
3. API key configured for ingestion (`PERF_AGENT_API_KEY` in `.env` or `.deploy.conf.staging`).
4. Review fixtures generated and ingested (see Quick Start below).

### Quick Start

```bash
# Generate + ingest locally:
scripts/review-regression-test.sh local

# Generate + ingest on staging:
scripts/review-regression-test.sh staging

# Generate only (no ingest):
scripts/review-regression-test.sh --generate-only
```

---

## Scenario Index

| ID | Name | Events | Workflow | Warning Codes |
|----|------|--------|----------|---------------|
| RS-01 | Weekly Yoga | 2 | add-occurrence (forward path) | `potential_duplicate` |
| RS-02 | Book Club | 2 | add-occurrence (near-dup path) | `near_duplicate_of_new_event` |
| RS-03 | Tech Meetup | 2 | add-occurrence on pending series | `potential_duplicate` |
| RS-04 | Art Walk | 2 | add-occurrence on draft target | `potential_duplicate` |
| RS-05 | Workshop | 2 | add-occurrence conflict (overlap) | `potential_duplicate` |
| RS-06 | Jazz Night | 1 | reversed dates + duplicate | `reversed_dates_timezone_likely`, `potential_duplicate` |
| RS-07 | Dance Class | 2 | not-a-duplicate (approve) | `potential_duplicate` |
| RS-08 | Community Potluck | 2 | exact duplicate (merge) | `potential_duplicate` |
| RS-09 | Film Screening | 1 | multi-session detection | `multi_session_likely` |
| RS-10 | Choir Rehearsal | 2 | order-independent consolidation | `near_duplicate_of_new_event` |
| RS-11 | Pottery Studio | 4 | same-day-different-times cluster | `potential_duplicate` / `near_duplicate_of_new_event` |

---

## Test Procedures

### RS-01: Forward-Path Add-Occurrence (Published Series)

**Setup:** Ingest `RS-01 Weekly Yoga -- Base Series` (4 weekly occurrences from Eventbrite). Wait for it to be published. Then ingest `RS-01 Weekly Yoga -- New Occurrence` (week 5, from Lu.ma).

**Expected after ingest:**
- Base series is `published` with 4 occurrences.
- New occurrence event is `pending_review` with `potential_duplicate` warning referencing the base series.
- Review queue shows the new occurrence with a link to the base series.

**Test steps:**
1. Open admin review queue. Find the RS-01 new occurrence review entry.
2. Verify the review shows `potential_duplicate` warning with the base series as the candidate.
3. Click **Add as Occurrence** and select the base series as target.
4. Verify:
   - [ ] Source event (new occurrence) is soft-deleted (`lifecycle_state = 'deleted'`).
   - [ ] Target series now has **5** occurrences (was 4).
   - [ ] New occurrence's start/end times match the week-5 dates.
   - [ ] Review status is `merged`.
   - [ ] Target series `lifecycle_state` remains `published`.

---

### RS-02: Near-Dup Path Add-Occurrence (Companion Reviews)

**Setup:** Ingest `RS-02 Book Club -- Existing Series` (2 Tuesday occurrences from Meetup). After it publishes, ingest `RS-02 Book Club -- Near-Dup New Event` (same date, from BlogTO).

**Expected after ingest:**
- Near-duplicate detection (pg_trgm Layer 2) fires on the similar names/venue.
- **Two** review entries created:
  - On the new event: `duplicate_of_event_id` points to existing series.
  - On the existing series: `duplicate_of_event_id` points to new event (companion review).
- Both events are `pending_review`.

**Test steps:**
1. Open review queue. Find both RS-02 entries (they form a pair).
2. On the **existing series' review entry**, click **Add as Occurrence** (near-dup path -- no target_event_ulid needed).
3. Verify:
   - [ ] Source event (new event) is soft-deleted.
   - [ ] Existing series now has **3** occurrences (was 2, absorbed the new event's occurrence).
   - [ ] The companion review entry (on the new event) is also dismissed (`merged`).
   - [ ] Existing series recomputes lifecycle: if no other pending reviews remain, transitions to `published`.

---

### RS-03: Lifecycle-Stays-Pending After Add-Occurrence

**Setup:** Ingest `RS-03 Tech Meetup -- Pending Series` (2 occurrences, from Meetup). Separately create another review reason that keeps the series in `pending_review`. Then ingest `RS-03 Tech Meetup -- Additional Occurrence`.

**Expected after ingest:**
- Tech Meetup series is `pending_review` (has at least one unresolved review).
- Additional occurrence is `pending_review` with `potential_duplicate` warning.

**Test steps:**
1. Click **Add as Occurrence** on the additional occurrence's review entry, targeting the tech meetup series.
2. Verify:
   - [ ] Source event soft-deleted.
   - [ ] Target series now has **3** occurrences (was 2).
   - [ ] Review status on the additional occurrence is `merged`.
   - [ ] Target series `lifecycle_state` stays `pending_review` (other unresolved reviews exist).

**Note:** This scenario requires the series to already have another pending review entry besides the one being resolved. If near-duplicate detection doesn't create the companion review, you may need to manually create one via SQL for the test.

---

### RS-04: Add-Occurrence on Draft Target

**Setup:** Ingest `RS-04 Art Walk -- Draft Series` (2 Saturday occurrences from Eventbrite). Manually set its `lifecycle_state` to `draft` via admin UI or SQL. Then ingest `RS-04 Art Walk -- New Occurrence` (week 3 Saturday, from Showpass).

**Expected after ingest:**
- Art Walk series is `draft`.
- New occurrence is `pending_review` with `potential_duplicate` warning.

**Test steps:**
1. Click **Add as Occurrence** targeting the draft series.
2. Verify:
   - [ ] Source event soft-deleted.
   - [ ] Target series has **3** occurrences.
   - [ ] Review status is `merged`.
   - [ ] Target series `lifecycle_state` remains `draft` (not auto-promoted to published by add-occurrence).

**Rationale:** Draft events should remain drafts even when occurrences are added; publication is a separate admin decision.

---

### RS-05: Overlapping Occurrence Conflict (409)

**Setup:** Ingest `RS-05 Workshop -- Overlap Target` (2 Wednesday occurrences from Lu.ma). Then ingest `RS-05 Workshop -- Overlapping Occurrence` (starts 30 min into the first existing occurrence, from Meetup).

**Expected after ingest:**
- Workshop series is published.
- Overlapping occurrence is `pending_review` with `potential_duplicate` warning.

**Test steps:**
1. Click **Add as Occurrence** targeting the workshop series.
2. Verify:
   - [ ] Response is **409 Conflict** (occurrence time overlaps existing).
   - [ ] No changes made to target series (still has 2 occurrences).
   - [ ] Source event is NOT deleted.
   - [ ] Review entry stays `pending` (action failed).

**Follow-up:** The admin should either:
- **Merge** (if it's truly the same session listed twice), or
- **Reject** the overlapping entry, or
- **Fix** the occurrence times and retry add-occurrence.

---

### RS-06: Multi-Warning (Reversed Dates + Potential Duplicate)

**Setup:** Ingest `RS-06 Jazz Night -- Reversed Dates Late Show` (11pm start, 2am "end" on same calendar date -- reversed dates).

**Expected after ingest:**
- Event is `pending_review`.
- Review entry has **two** warnings:
  - `reversed_dates_timezone_likely` (overnight event, high confidence auto-fix)
  - `potential_duplicate` (if a similar jazz event already exists)

**Test steps:**
1. Open the review entry. Verify both warnings are displayed.
2. Verify the auto-corrected end time (should be 2am the **next** day after adding 24h).
3. Compare original payload vs normalized payload in the review detail view.
4. **Approve** the event.
5. Verify:
   - [ ] Event transitions to `published`.
   - [ ] Occurrence dates use the corrected end time.
   - [ ] Review status is `approved`.

**Note:** If no similar jazz event exists, only the `reversed_dates` warning will appear. The `potential_duplicate` warning depends on whether there's an existing event with similar name/venue in the database.

---

### RS-07: Not-a-Duplicate (Approve with record_not_duplicates)

**Setup:** Ingest `RS-07 Dance Class -- Existing Series` (3 Wednesday occurrences from Eventbrite). After it publishes, ingest `RS-07 Dance Class -- Not A Duplicate` (same venue, same day, later time, from Lu.ma -- a social dance event, not the structured class).

**Expected after ingest:**
- Near-duplicate detection may fire (same venue + "Dance" in name).
- New event is `pending_review` with `potential_duplicate` warning.

**Test steps:**
1. Open review queue. Find the RS-07 new event review entry.
2. Inspect both events: confirm they are genuinely different events at the same venue.
3. Click **Approve** with the `record_not_duplicates: true` option checked.
4. Verify:
   - [ ] New event transitions to `published`.
   - [ ] A `not_duplicates` record is created pairing the two events (prevents future false positives).
   - [ ] Companion review on the existing series (if created) is dismissed.
   - [ ] Both events remain separately listed.

---

### RS-08: Exact Duplicate Merge

**Setup:** Ingest `RS-08 Community Potluck -- Original` (Sunday, from Meetup). Then ingest `RS-08 Community Potluck -- Exact Duplicate` (identical details, from BlogTO).

**Expected after ingest:**
- Layer 1 exact dedup may auto-merge (same dedup hash). If not:
- Exact duplicate is `pending_review` with `potential_duplicate` warning.

**Test steps (if not auto-merged):**
1. Open review queue. Find the RS-08 duplicate's review entry.
2. Click **Merge** into the original event.
3. Verify:
   - [ ] Duplicate event is soft-deleted (`lifecycle_state = 'deleted'`).
   - [ ] Tombstone record created with reason `duplicate_merged` and `superseded_by` pointing to the original.
   - [ ] Original event unchanged (same occurrences, same lifecycle state).
   - [ ] Review status is `merged`.

**Note:** If Layer 1 auto-merges, both events share the same dedup hash and the second ingest returns 409 or updates the existing event. In that case, verify no review entry is created and the original event is unaffected.

---

### RS-09: Multi-Session Detection

**Setup:** Ingest `RS-09 Film Screening (8 sessions) -- Multi-Session` (6-hour event with "(8 sessions)" in title, from Eventbrite).

**Expected after ingest:**
- Title pattern heuristic fires on "(8 sessions)" substring.
- Event is `pending_review` with `multi_session_likely` warning.

**Test steps:**
1. Open review queue. Find the RS-09 entry.
2. Verify the `multi_session_likely` warning is present.
3. Decide:
   - If this should be published as a single long event: **Approve**.
   - If this should be split into 8 separate sessions: **Reject** (and manually create separate events).
4. Test the **Approve** path:
   - [ ] Event transitions to `published`.
   - [ ] Review status is `approved`.
5. (Optional) Reset and test the **Reject** path:
   - [ ] Event transitions to `deleted`.
   - [ ] Review status is `rejected` with reason recorded.

---

### RS-10: Order-Independent Consolidation

**Setup:** Ingest `RS-10 Choir Rehearsal -- Source A` (Wednesday, from Google Calendar) and `RS-10 Choir Rehearsal -- Source B` (following Wednesday, from Lu.ma) in either order.

**Expected after ingest:**
- Near-duplicate detection fires on both (same name + venue, different dates).
- Both events are `pending_review` with companion reviews.
- The final state should be the same regardless of which event is ingested first.

**Test steps:**
1. Verify both events appear in the review queue with `near_duplicate_of_new_event` warnings.
2. On either event's review entry, click **Add as Occurrence**.
3. Verify:
   - [ ] Source event soft-deleted.
   - [ ] Target event now has **2** occurrences.
   - [ ] Companion review dismissed.
   - [ ] Target published (if no other pending reviews).

**Order-independence check:** Reset the database and re-run, ingesting Source B first, then Source A. Verify the same final state.

---

### RS-11: Same-Day-Different-Times Cluster

**Setup:** Ingest all 4 RS-11 events:
- `RS-11 Pottery Studio -- Mon 10am Session`
- `RS-11 Pottery Studio -- Mon 2pm Session`
- `RS-11 Pottery Studio -- Mon+7 10am Session`
- `RS-11 Pottery Studio -- Mon+7 2pm Session`

All are from Eventbrite, at The Tranzac, with similar names but different times.

**Expected after ingest:**
- Near-duplicate and/or potential_duplicate detection may fire on some or all pairs.
- Multiple review entries created, potentially with complex cross-references.

**Test steps:**
1. Open the review queue. Identify all RS-11 entries.
2. Determine the correct consolidation:
   - **2 series** (morning series + afternoon series), each with 2 weekly occurrences? Use add-occurrence twice.
   - **1 series** with 4 occurrences? Use add-occurrence three times.
   - **4 separate events** (different time slots are genuinely different events)? Approve all with `record_not_duplicates`.
3. Execute your chosen consolidation strategy.
4. Verify:
   - [ ] Final state matches your intent (correct number of events, correct occurrences on each).
   - [ ] All review entries are resolved (no orphaned pending reviews).
   - [ ] Surviving events are `published`.

**This scenario intentionally has no single "right" answer -- it tests the admin's judgment and the system's ability to support multiple valid consolidation paths.**

---

## Verification Checklist

After completing all scenarios, verify:

- [ ] No orphaned `pending_review` entries remain from the fixture set (all should be resolved).
- [ ] No orphaned events in `pending_review` state from the fixture set (all should be published or deleted).
- [ ] Tombstone records exist for all soft-deleted events with correct `superseded_by` references.
- [ ] The `not_duplicates` table has entries from RS-07 (and RS-11 if applicable).
- [ ] Event occurrence counts match expectations (RS-01: 5, RS-02: 3, RS-03: 3, RS-04: 3, RS-10: 2).

### SQL Verification Queries

```sql
-- Check for orphaned pending reviews from fixture set
SELECT erq.id, erq.status, e.name
FROM event_review_queue erq
JOIN events e ON erq.event_id = e.id
WHERE e.name LIKE 'RS-%'
AND erq.status = 'pending';

-- Check event states
SELECT ulid, name, lifecycle_state,
       (SELECT COUNT(*) FROM event_occurrences WHERE event_id = events.id) AS occ_count
FROM events
WHERE name LIKE 'RS-%'
ORDER BY name;

-- Check tombstones
SELECT e.name, t.reason, t.superseded_by
FROM event_tombstones t
JOIN events e ON t.event_id = e.id
WHERE e.name LIKE 'RS-%';

-- Check not-duplicates table
SELECT e1.name AS event_a, e2.name AS event_b
FROM not_duplicates nd
JOIN events e1 ON nd.event_id_a = e1.id
JOIN events e2 ON nd.event_id_b = e2.id
WHERE e1.name LIKE 'RS-%' OR e2.name LIKE 'RS-%';
```

---

## Resetting Between Runs

To re-run the full test plan, clean up fixture events:

```sql
-- Delete all RS-XX fixture events and associated data
DELETE FROM event_review_queue
WHERE event_id IN (SELECT id FROM events WHERE name LIKE 'RS-%');

DELETE FROM event_tombstones
WHERE event_id IN (SELECT id FROM events WHERE name LIKE 'RS-%');

DELETE FROM event_occurrences
WHERE event_id IN (SELECT id FROM events WHERE name LIKE 'RS-%');

DELETE FROM not_duplicates
WHERE event_id_a IN (SELECT id FROM events WHERE name LIKE 'RS-%')
   OR event_id_b IN (SELECT id FROM events WHERE name LIKE 'RS-%');

DELETE FROM events WHERE name LIKE 'RS-%';
```

Or for a full database reset (local only): `make migrate-down && make migrate-up && make migrate-river`.
