# Review Queue Regression Test Plan

Manual regression test plan for the event review queue, covering duplicate detection, consolidate, approve, reject, and near-duplicate workflows. Uses the `--review-events` fixture set (11 scenario groups, 22 events).

**Fixture source:** `tests/testdata/fixtures.go` (`BatchReviewEventInputs`)
**Generate command:** `server generate review-fixtures.json --review-events`
**Helper script:** `scripts/review-regression-test.sh`

---

## How to Run This Test Plan

### 1. Check the server is built and running on the right version

```bash
make build                                     # rebuild after any code change
curl -s http://localhost:8080/health | jq .version   # confirm expected git SHA
```

> **Important for agents:** multiple `./server` processes can accumulate across sessions.
> If the version doesn't match, kill stale instances and restart:
>
> ```bash
> pkill -f "^./server"           # kill old binaries
> nohup ./server serve &         # start the freshly built one
> sleep 3
> curl -s http://localhost:8080/health | jq '{version,status}'
> ```

```bash
make run        # local (builds + runs in one step)
# or for staging: connect/deploy as needed
```

### 2. Clean any prior fixture data (if re-running)

```bash
scripts/review-regression-test.sh --clean local
```

### 3. Generate and ingest all 22 fixture events

```bash
scripts/review-regression-test.sh local       # local
scripts/review-regression-test.sh staging     # staging
```

The script prints each event's ingest result (201/202/409) and a summary. Expected: ~11 published directly, ~11 routed to review queue.

### 4. Get an admin JWT

```bash
curl -s -X POST http://localhost:8080/api/v1/admin/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"<ADMIN_PASSWORD>"}' | jq -r .token
# Credentials are in .env: ADMIN_USERNAME / ADMIN_PASSWORD
```

Store the token (JWTs expire after 24 h — refresh with the same command if you get 401):

```bash
export JWT="<token from above>"
export BASE="http://localhost:8080/api/v1"
export DB="postgresql://ryankelln@localhost:5432/togather?sslmode=disable"
```

### 5. Work through each scenario

Use the [Scenario Index](#scenario-index) below to find the relevant events in the review queue, then follow the step-by-step procedure for each RS-XX section.

**Route cheat-sheet** — two distinct namespaces, easy to confuse:

| Action | Route |
|--------|-------|
| List pending events (event ULIDs, names, lifecycle) | `GET $BASE/admin/events/pending` → `.items[]` |
| List review queue (review IDs, warnings, event ULIDs) | `GET $BASE/admin/review-queue` → `.items[]` |
| Create/update/delete occurrence manually (not via review queue) | `POST/PUT/DELETE $BASE/admin/events/{event-ulid}/occurrences[/{occ-id}]` |
| Consolidate events (retire N, keep/create 1) | `POST $BASE/admin/events/consolidate` |

> **Important:** Use `GET $BASE/admin/review-queue` (not `/admin/events/pending`) when you need review IDs and warning codes. The `/admin/events/pending` endpoint returns event summaries only — it does not include review queue IDs or warnings.

```bash
# List review queue — find review IDs and event ULIDs
curl -s -H "Authorization: Bearer $JWT" "$BASE/admin/review-queue" | jq '.items[]'

# Approve a review entry  (integer review id, NOT event ulid)
curl -s -X POST -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" \
  "$BASE/admin/review-queue/<review-id>/approve" -d '{}'

# Reject
curl -s -X POST -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" \
  "$BASE/admin/review-queue/<review-id>/reject" \
  -d '{"reason":"<reason>"}'

# Manual occurrence creation (NOT the review-queue path — requires start_time + timezone + venue)
# venue_ulid: look up via: psql $DB -c "SELECT p.ulid FROM places p JOIN events e ON e.primary_venue_id=p.id WHERE e.name='<name>';"
curl -s -X POST -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" \
  "$BASE/admin/events/<event-ulid>/occurrences" \
  -d '{"start_time":"<RFC3339>","end_time":"<RFC3339>","timezone":"America/Toronto","venue_ulid":"<venue-ulid>"}'
```

```bash
# Consolidate: promote existing event as canonical, retire duplicates
curl -s -X POST -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" \
  "$BASE/admin/events/consolidate" \
  -d '{"event_ulid":"<canonical-ulid>","retire":["<dup-ulid-1>","<dup-ulid-2>"]}'

# Consolidate: create new canonical event, retire source events
curl -s -X POST -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" \
  "$BASE/admin/events/consolidate" \
  -d '{"event":{"name":"...","startDate":"...","location":{"name":"..."}},"retire":["<old-ulid>"]}'

### 6. Verify with SQL

After completing all scenarios, run the [SQL verification queries](#sql-verification-queries) at the bottom of this doc to confirm no orphaned review entries, correct occurrence counts, tombstones, and not-duplicate records.

### 7. Clean up

```bash
scripts/review-regression-test.sh --clean local
scripts/agent-cleanup.sh    # remove agent output files if run by an agent
```

---

## Prerequisites

1. Server running locally (`make run`) or staging deployment.
2. Admin account with `admin` role — credentials in `.env` (`ADMIN_USERNAME` / `ADMIN_PASSWORD`).
3. `API_KEY` (or `PERF_AGENT_API_KEY`) in `.env` for fixture ingestion.
4. `jq` installed for JSON parsing.

---

## Scenario Index

| ID | Name | Events | Workflow | Warning Codes | Auto-dedup? |
|----|------|--------|----------|---------------|-------------|
| RS-01 | Weekly Yoga | 2 | `multi_session_likely` + near-dup → merge duplicate or approve separately | `multi_session_likely`, `near_duplicate_of_new_event`, `potential_duplicate` | Yes (same venue + same date, similarity ~0.65) |
| RS-02 | Book Club | 2 | add-occurrence (near-dup path) | `near_duplicate_of_new_event`, `potential_duplicate` | Yes (similarity ~0.63) |
| RS-03 | Tech Meetup | 2 | manual add-occurrence on pending series | _(none — publishes directly)_ | No (different dates, low similarity) |
| RS-04 | Art Walk | 2 | manual add-occurrence on draft target | _(none — publishes directly)_ | No (different dates, low similarity) |
| RS-05 | Workshop | 2 | add-occurrence conflict (overlap) | `near_duplicate_of_new_event`, `potential_duplicate` | Yes (similarity ~0.49) |
| RS-06 | Jazz Night | 1 | reversed dates auto-correction | `reversed_dates_timezone_likely` | N/A (single event) |
| RS-07 | Dance Class | 2 | not-a-duplicate (approve) | `near_duplicate_of_new_event`, `potential_duplicate` | Yes (similarity ~0.71) |
| RS-08 | Community Potluck | 2 | exact duplicate (merge) | `near_duplicate_of_new_event`, `potential_duplicate` | Yes (similarity ~0.49) |
| RS-09 | Film Screening | 1 | multi-session detection | `multi_session_likely` | N/A (single event) |
| RS-10 | Choir Rehearsal | 2 | order-independent consolidation | `near_duplicate_of_new_event`, `potential_duplicate` | Yes (similarity ~0.88) |
| RS-11 | Pottery Studio | 4 | same-day-different-times cluster | `near_duplicate_of_new_event`, `potential_duplicate` | Yes (same-day pairs only) |
| RS-12 | Consolidation Endpoint | 2+ | consolidate (promote + create paths) | _(varies)_ | N/A (tests consolidation itself) |

---

## Test Procedures

### RS-01: Multi-Session Keyword + Near-Dup Companion Pair → Merge Duplicate

**Setup:** Ingest `RS-01 Weekly Yoga at The Tranzac` (4 weekly occurrences from Eventbrite) and `RS-01 Weekly Yoga` (same venue, same date as week 1, from Lu.ma — a different scraper picking up the same session).

**Expected after ingest:**
- Both events land in review:
  - `multi_session_likely` — the word "Weekly" in the title matches the multi-session keyword pattern `(?i)\bweekly\b` on both events.
  - Near-duplicate detection fires (same venue + same date + pg_trgm name similarity ~0.65, well above the 0.4 threshold): a companion pair is created.
  - One entry receives `near_duplicate_of_new_event` pointing at its companion; the other receives `potential_duplicate` pointing back.
- Both events are `pending_review`.

**Test steps:**
1. Open admin review queue. Find both RS-01 entries — they form a companion pair.
2. Verify both show `multi_session_likely` AND a duplicate warning (`near_duplicate_of_new_event` / `potential_duplicate`).
3. Expand either entry. Verify the side-by-side panel shows both events with their occurrence lists.
4. The base series has 4 occurrences; the Lu.ma event has 1 occurrence on the same date as week 1.

**Path A — Merge Duplicate (same session listed twice, different scraper):**

5. Choose the Eventbrite base series as canonical. Click **Merge Duplicate** (this retires the Lu.ma standalone event and keeps the Eventbrite series with its 4 occurrences intact — the Lu.ma event is the same session, not an additional occurrence).
6. Verify:
   - [ ] Lu.ma event soft-deleted (tombstone with `deletion_reason = 'consolidated'` and `superseded_by_uri` pointing to the base series).
   - [ ] Base series unchanged — still 4 occurrences, same lifecycle.
   - [ ] Both review entries dismissed (`review_entries_dismissed` in consolidate response).
   - [ ] Base series transitions to `published` if no other pending review rows remain.

**Path B — Not a Duplicate (approve separately):**

5. On either review entry, click **Not a Duplicate**.
6. Verify:
   - [ ] An `event_not_duplicates` record is written for the pair (prevents future near-dup pairings between these two).
   - [ ] The companion review entry is updated: the duplicate warning is cleared; only `multi_session_likely` remains.
7. **Approve** each event independently.
8. Verify:
   - [ ] Both events transition to `published` as separate listings.
   - [ ] No stale `pending_review` entries remain for RS-01.

**Path C — Approve both without resolving the duplicate (smoke test):**

5. Approve both events without taking any merge/not-a-dup action.
6. Verify:
   - [ ] Both events transition to `published`.
   - [ ] Companion review entries are resolved as `approved` (not left in a stale `pending_review` state).

**Note:** The "Weekly" keyword trigger is by design — it's a useful guardrail for real-world events where "Weekly Yoga" might actually be a multi-session course sold as a single ticket.

---

### RS-02: Near-Dup Path Add-Occurrence (Companion Reviews)

**Setup:** Ingest `RS-02 Book Club — Tuesday Evening` (2 Tuesday occurrences from Meetup) and `RS-02 Book Club — Tuesday Night` (same date, from BlogTO).

**Expected after ingest:**
- Near-duplicate detection (pg_trgm Layer 2) fires: similarity ~0.63 + same venue + same date → above 0.4 threshold.
- **Two** review entries created (companion pair):
  - On one event: `near_duplicate_of_new_event` pointing to the other.
  - On the other: `potential_duplicate` pointing back.
- Both events are `pending_review`.

**Test steps:**
1. Open review queue. Find both RS-02 entries (they form a companion pair).
2. In the fold-down for the near-duplicate review entry, both events are shown side-by-side with their occurrence lists. The base series has 2 occurrences; the new event has 1.
   - Click **Add as Occurrence** to absorb the new event into the base series (the new event's occurrence is added to the series, the new event is retired).
   - Alternatively, call the consolidate API directly:
     ```bash
     curl -s -X POST -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" \
       "$BASE/admin/events/consolidate" \
       -d '{"event_ulid":"<canonical-ulid>","retire":["<absorbed-ulid>"]}'
     ```
3. Verify:
    - [ ] Source event (absorbed event) is soft-deleted.
    - [ ] Target event now has **3** occurrences (was 2, absorbed the new event's occurrence).
    - [ ] The companion review entry is also dismissed (`merged` — no stale pending companion row should remain after successful consolidation).
    - [ ] Target event recomputes lifecycle: transitions to `published` if no other pending review rows remain, otherwise stays `pending_review`.

---

### RS-03: Lifecycle-Stays-Pending After Add-Occurrence

**Setup:** Ingest `RS-03 Tech Meetup — Pending Series` (2 occurrences, from Meetup) and `RS-03 Tech Meetup — Additional Occurrence` (3 weeks later, from Lu.ma).

**Expected after ingest:**
- Both events **publish directly** — near-duplicate detection does not fire because the events are on different dates (3 weeks apart) and the name similarity (~0.33) is below the 0.4 threshold.
- Neither event has a review queue entry after ingest.

**Pre-test setup (manual):**
This scenario tests lifecycle preservation during add-occurrence. To exercise it:
1. Place the Tech Meetup series in `pending_review` state and create a review entry for it:

```sql
-- Step 1: set lifecycle state
UPDATE events SET lifecycle_state = 'pending_review' WHERE name LIKE 'RS-03%Pending%';

-- Step 2: insert a review entry (quality_issue warning; all NOT NULL columns required)
INSERT INTO event_review_queue (event_id, original_payload, normalized_payload, warnings, event_start_time, status)
SELECT id,
       '{"name":"RS-03 Tech Meetup — Pending Series"}'::jsonb,
       '{"name":"RS-03 Tech Meetup — Pending Series"}'::jsonb,
       '[{"code":"quality_issue","message":"Manual test setup"}]'::jsonb,
       (SELECT eo.start_time FROM event_occurrences eo WHERE eo.event_id = events.id LIMIT 1),
       'pending'
FROM events WHERE name LIKE 'RS-03%Pending%';
```

**Test steps:**
1. With the series now in `pending_review`, use the manual occurrence API to add the additional occurrence to the tech meetup series:
   ```bash
   # Look up the target event ULID and venue ULID
   psql $DB -c "SELECT ulid, name FROM events WHERE name LIKE 'RS-03%Pending%';"
   psql $DB -c "SELECT p.ulid FROM places p JOIN events e ON e.primary_venue_id=p.id WHERE e.name LIKE 'RS-03%Pending%';"
   # Look up the start/end time of the source occurrence to absorb
   psql $DB -c "SELECT eo.start_time, eo.end_time FROM event_occurrences eo JOIN events e ON eo.event_id=e.id WHERE e.name LIKE 'RS-03%Additional%';"

   curl -s -X POST -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" \
     "$BASE/admin/events/<pending-series-ulid>/occurrences" \
     -d '{"start_time":"<RFC3339>","end_time":"<RFC3339>","timezone":"America/Toronto","venue_ulid":"<venue-ulid>"}'
   ```
2. Soft-delete the additional occurrence event (the manual API does not do this automatically — soft-deletion only happens via the review-queue `add-occurrence` action):
   ```sql
   UPDATE events SET lifecycle_state = 'deleted' WHERE name LIKE 'RS-03%Additional%';
   ```
3. Verify:
   - [ ] Target series now has **3** occurrences (was 2).
   - [ ] Target series `lifecycle_state` stays `pending_review` (the pre-existing review is still unresolved).

**Consolidation Alternative (preferred):**

Instead of the manual occurrence API + SQL soft-delete workflow above, use the consolidation endpoint:

1. First add the occurrence manually (same as step 1 above — consolidation doesn't move occurrences, it retires whole events):
   ```bash
   curl -s -X POST -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" \
     "$BASE/admin/events/<pending-series-ulid>/occurrences" \
     -d '{"start_time":"<RFC3339>","end_time":"<RFC3339>","timezone":"America/Toronto","venue_ulid":"<venue-ulid>"}'
   ```
2. Then retire the additional occurrence event via consolidation:
   ```bash
   curl -s -X POST -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" \
     "$BASE/admin/events/consolidate" \
     -d '{"event_ulid":"<pending-series-ulid>","retire":["<additional-ulid>"]}'
   ```
3. Verify:
   - [ ] Additional event is soft-deleted with tombstone (`superseded_by_uri` pointing to the series, `deletion_reason = 'consolidated'`).
   - [ ] Any pending reviews on the additional event are dismissed.
   - [ ] Series `lifecycle_state` stays `pending_review` (the pre-existing quality review is still unresolved).
   - [ ] Series has 3 occurrences.

**Note:** This scenario deliberately does not rely on automatic dedup. It tests that the add-occurrence action preserves a pending lifecycle when other unresolved reviews exist on the target. Because neither event lands in the review queue automatically, the add-occurrence must be performed via the manual occurrence API (`POST /admin/events/{ulid}/occurrences`) — **not** via the review-queue `add-occurrence` action (which requires a pending review row on the source event). The manual API only adds the occurrence; soft-deletion of the source event requires a separate SQL step.

---

### RS-04: Add-Occurrence on Draft Target

**Setup:** Ingest `RS-04 Art Walk — Draft Series` (2 Saturday occurrences from Eventbrite) and `RS-04 Art Walk — New Occurrence` (week 3 Saturday, from Showpass).

**Expected after ingest:**
- Both events **publish directly** — near-duplicate detection does not fire because the events are on different dates (3 weeks apart) and the name similarity (~0.35) is below the 0.4 threshold.
- Neither event has a review queue entry after ingest.

**Pre-test setup (manual):**
This scenario tests add-occurrence behaviour on a draft target. To exercise it:
1. Set the Art Walk series to `draft` state via SQL: `UPDATE events SET lifecycle_state = 'draft' WHERE name LIKE 'RS-04%Draft%'`.

**Test steps:**
1. With the series now in `draft`, use the manual occurrence API to add the new occurrence to the draft series:
   ```bash
   psql $DB -c "SELECT ulid FROM events WHERE name LIKE 'RS-04%Draft%';"
   psql $DB -c "SELECT p.ulid FROM places p JOIN events e ON e.primary_venue_id=p.id WHERE e.name LIKE 'RS-04%Draft%';"
   psql $DB -c "SELECT eo.start_time, eo.end_time FROM event_occurrences eo JOIN events e ON eo.event_id=e.id WHERE e.name LIKE 'RS-04%New%';"

   curl -s -X POST -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" \
     "$BASE/admin/events/<draft-series-ulid>/occurrences" \
     -d '{"start_time":"<RFC3339>","end_time":"<RFC3339>","timezone":"America/Toronto","venue_ulid":"<venue-ulid>"}'
   ```
2. Soft-delete the source event (the manual API does not do this automatically):
   ```sql
   UPDATE events SET lifecycle_state = 'deleted' WHERE name LIKE 'RS-04%New%';
   ```
3. Verify:
   - [ ] Target series has **3** occurrences.
   - [ ] Target series `lifecycle_state` remains `draft` (not auto-promoted to published by add-occurrence).

**Consolidation Alternative (preferred):**

Instead of the manual occurrence API + SQL soft-delete workflow above, use the consolidation endpoint:

1. First add the occurrence manually (same as step 1 above — consolidation doesn't move occurrences, it retires whole events):
   ```bash
   curl -s -X POST -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" \
     "$BASE/admin/events/<draft-series-ulid>/occurrences" \
     -d '{"start_time":"<RFC3339>","end_time":"<RFC3339>","timezone":"America/Toronto","venue_ulid":"<venue-ulid>"}'
   ```
2. Then retire the source event via consolidation:
   ```bash
   curl -s -X POST -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" \
     "$BASE/admin/events/consolidate" \
     -d '{"event_ulid":"<draft-series-ulid>","retire":["<new-occurrence-ulid>"]}'
   ```
3. Verify:
   - [ ] Source event is soft-deleted with tombstone (`superseded_by_uri` pointing to the draft series, `deletion_reason = 'consolidated'`).
   - [ ] Any pending reviews on the source event are dismissed.
   - [ ] Draft series `lifecycle_state` remains `draft` — consolidation does not auto-promote the canonical.
   - [ ] Draft series has 3 occurrences.

**Rationale:** Draft events should remain drafts even when occurrences are added; publication is a separate admin decision. This scenario deliberately does not rely on automatic dedup — it tests manual add-occurrence on a draft target. Because neither event lands in the review queue automatically, the add-occurrence must be performed via the manual occurrence API (`POST /admin/events/{ulid}/occurrences`) — **not** via the review-queue `add-occurrence` action. The SQL state change is required before the add-occurrence so that the draft-preservation invariant can be verified. Soft-deletion of the source event requires a separate SQL step (the manual API does not absorb/delete the source automatically).

---

### RS-05: Overlapping Occurrence Conflict (409)

**Setup:** Ingest `RS-05 Workshop — Overlap Target` (2 Wednesday occurrences from Lu.ma). Then ingest `RS-05 Workshop — Overlapping Occurrence` (starts 30 min into the first existing occurrence, from Meetup).

**Expected after ingest:**
- Near-duplicate detection fires: similarity ~0.49 + same venue + same date → above 0.4 threshold.
- Workshop overlap target is `pending_review` with `near_duplicate_of_new_event` warning.
- Overlapping occurrence is `pending_review` with `potential_duplicate` warning.
- They form a companion review pair.

**Test steps:**
1. In the fold-down for either RS-05 review entry, click **Add as Occurrence** (or call consolidate directly):
   ```bash
   curl -s -X POST -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" \
     "$BASE/admin/events/consolidate" \
     -d '{"event_ulid":"<workshop-series-ulid>","retire":["<overlapping-ulid>"]}'
   ```
2. Verify:
   - [ ] Response is **409 Conflict** — the consolidate API detects the occurrence overlap and rejects the retirement
   - [ ] No changes made to either event (both still exist, same occurrences)
   - [ ] Review entries stay `pending`
   - [ ] UI shows inline error: "The occurrence at [time] conflicts with an existing occurrence"

**Follow-up:** The admin should either:
- Use **Merge Duplicate** instead (if it's truly the same session listed twice — same time, two sources), or
- **Reject** the overlapping entry and handle it manually, or
- Edit the occurrence times on one event and retry

---

### RS-06: Multi-Warning (Reversed Dates + Potential Duplicate)

**Setup:** Ingest `RS-06 Jazz Night — Reversed Dates Late Show` (11pm start, 2am "end" on same calendar date -- reversed dates).

**Expected after ingest:**
- Event is `pending_review`.
- Review entry has `reversed_dates_timezone_likely` warning (overnight event: 11pm start, 2am end on same calendar date).
- No `potential_duplicate` warning unless a similar jazz event already exists in the database (from a previous test run or real data).

**Test steps:**
1. Open the review entry. Verify the `reversed_dates_timezone_likely` warning is displayed.
2. Verify the auto-corrected end time (should be 2am the **next** day after adding 24h).
3. Compare original payload vs normalized payload in the review detail view.
4. **Approve** the event.
5. Verify:
   - [ ] Event transitions to `published`.
   - [ ] Occurrence dates use the corrected end time.
   - [ ] Review status is `approved`.

---

### RS-07: Not-a-Duplicate (Approve with record_not_duplicates)

**Setup:** Ingest `RS-07 Dance Class — Wednesday Series` (3 Wednesday occurrences from Eventbrite) and `RS-07 Dance Class — Wednesday Social` (same venue, same day, later time, from Lu.ma — a social dance event, not the structured class).

**Expected after ingest:**
- Near-duplicate detection fires: similarity ~0.71 + same venue + same date → above 0.4 threshold.
- Both events are `pending_review` with companion near-duplicate review entries.
- Despite the similar names, these are genuinely different events (a structured class vs a social dance).

**Test steps:**
1. Open review queue. Find both RS-07 review entries.
2. Inspect both events: confirm they are genuinely different events at the same venue.
3. On **one** of the review entries, click **Approve** with the `record_not_duplicates: true` option checked.
4. Verify:
   - [ ] The approved event transitions to `published`.
   - [ ] A `not_duplicates` record is created pairing the two events (prevents future false positives).
   - [ ] The companion review entry on the **other** event is rechecked after duplicate warnings are removed.
   - [ ] If no issues remain on the companion, it is auto-approved and disappears from the pending review queue.
   - [ ] If other warnings remain on the companion, it stays `pending` with refreshed warnings that no longer reference the acted-on event.
5. Open the review queue and confirm the second RS-07 entry matches the expected branch from step 4.
6. Verify both events remain separately listed.

**Note:** `record_not_duplicates: true` still directly approves only the actioned review entry. The companion pending event is then rechecked: it is auto-approved if no issues remain, otherwise it stays pending with updated warnings.

---

### RS-08: Exact Duplicate Merge

**Setup:** Ingest `RS-08 Community Potluck — Original` (Sunday, from Meetup). Then ingest `RS-08 Community Potluck — Exact Duplicate` (identical details, from BlogTO).

**Expected after ingest:**
- Layer 1 exact dedup does **not** fire (different source URLs → different dedup hashes).
- Near-duplicate detection fires: similarity ~0.49 + same venue + same date → above 0.4 threshold.
- Both events are `pending_review` with companion near-duplicate review entries:
  - Original: `near_duplicate_of_new_event`
  - Exact duplicate: `potential_duplicate`

**Test steps:**
1. Open review queue. Find both RS-08 review entries.
2. Expand either entry. In the fold-down, both events are shown side-by-side — identical name, venue, date, and organizer, different source URLs.
3. Click **Merge Duplicate** (the duplicate has no unique occurrences worth preserving — it's the same session listed twice). Or call consolidate directly:
   ```bash
   curl -s -X POST -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" \
     "$BASE/admin/events/consolidate" \
     -d '{"event_ulid":"<original-ulid>","retire":["<duplicate-ulid>"]}'
   ```
4. Verify:
   - [ ] Duplicate event is soft-deleted (`lifecycle_state = 'deleted'`)
   - [ ] Tombstone created with `deletion_reason = 'consolidated'` and `superseded_by_uri` pointing to the original
   - [ ] Original event unchanged (same 1 occurrence, same lifecycle state)
   - [ ] Both review entries dismissed (`review_entries_dismissed` in response)

---

### RS-09: Multi-Session Detection

**Setup:** Ingest `RS-09 Film Screening (8 sessions) — Multi-Session` (6-hour event with "(8 sessions)" in title, from Eventbrite).

**Expected after ingest:**
- Title pattern heuristic fires on "(8 sessions)" substring.
- Event is `pending_review` with `multi_session_likely` warning.

**Test steps:**
1. Open review queue. Find the RS-09 entry.
2. Verify the `multi_session_likely` warning is present.
3. Decide:
   - If this should be published as a single long event: **Approve**.
   - If this should be split into sessions: **Reject**, then use the inline occurrence editor in the fold-down to add individual session occurrences, or create them manually via `POST /admin/events/{ulid}/occurrences` after approving the base event.
4. Test the **Approve** path:
   - [ ] Event transitions to `published`.
   - [ ] Review status is `approved`.
5. (Optional) Reset and test the **Reject** path:
   - [ ] Event transitions to `deleted`.
   - [ ] Review status is `rejected` with reason recorded.

---

### RS-10: Order-Independent Consolidation

**Setup:** Ingest `RS-10 Choir Rehearsal — Source A` (Wednesday, from Google Calendar) and `RS-10 Choir Rehearsal — Source B` (same Wednesday, from Lu.ma) in either order.

**Expected after ingest:**
- Near-duplicate detection fires: similarity ~0.88 + same venue + same date → well above 0.4 threshold.
- Both events are `pending_review` with companion near-duplicate review entries.
- The final state should be the same regardless of which event is ingested first.

**Test steps:**
1. Verify both events appear in the review queue with near-duplicate warnings.
2. Expand either event's review entry. The fold-down shows both Choir Rehearsal events side-by-side — same venue, same date, nearly identical names. Click **Add as Occurrence** to absorb one into the other. Or use the API directly:
   ```bash
   curl -s -X POST -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" \
     "$BASE/admin/events/consolidate" \
     -d '{"event_ulid":"<chosen-canonical-ulid>","retire":["<other-ulid>"]}'
   ```
3. Verify:
    - [ ] Source event soft-deleted.
    - [ ] Target event now has **2** occurrences (was 1; the source event's sole occurrence is added to the target).
    - [ ] Companion review dismissed (`merged` status — no stale pending row should remain on the target after successful consolidation).
    - [ ] Target lifecycle recomputes: transitions to `published` if no other pending review rows remain, otherwise stays `pending_review`.

**Consolidation path (preferred for this scenario):**

1. Choose which event has better data (Source A or Source B) — that becomes the canonical.
2. Consolidate:
   ```bash
   curl -s -X POST -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" \
     "$BASE/admin/events/consolidate" \
     -d '{"event_ulid":"<chosen-canonical-ulid>","retire":["<other-ulid>"]}'
   ```
3. Verify:
   - [ ] Retired event soft-deleted with tombstone (`deletion_reason = 'consolidated'`, `superseded_by_uri` pointing to canonical).
   - [ ] Both companion review entries dismissed (`review_entries_dismissed` in response).
   - [ ] Canonical event transitions to `published` (if no other issues) or `pending_review` (if flagged).
   - [ ] Note: consolidation does NOT move occurrences — the canonical keeps its own occurrence(s). If the admin wants to add the retired event's occurrence to the canonical, they should use the manual occurrence API first, then consolidate.

**Order-independence check:** Reset the database and re-run, ingesting Source B first, then Source A. Verify the same final state.

---

### RS-11: Same-Day-Different-Times Cluster

**Setup:** Ingest all 4 RS-11 events:
- `RS-11 Pottery Studio — Mon 10am Session`
- `RS-11 Pottery Studio — Mon 2pm Session`
- `RS-11 Pottery Studio — Mon+7 10am Session`
- `RS-11 Pottery Studio — Mon+7 2pm Session`

All are from Eventbrite, at The Tranzac, with similar names but different times.

**Expected after ingest:**
- Near-duplicate detection fires on **same-day pairs** (similarity ~0.80 + same venue + same date → well above 0.4 threshold):
  - Mon 10am ↔ Mon 2pm (same Monday)
  - Mon+7 10am ↔ Mon+7 2pm (same following Monday)
- Cross-week pairs (Mon 10am ↔ Mon+7 10am) do **not** trigger dedup because they are on different dates.
- All 4 events are `pending_review`. Each same-day pair has companion near-duplicate review entries:
  - Mon 10am: `near_duplicate_of_new_event` (paired with Mon 2pm)
  - Mon 2pm: `potential_duplicate` (paired with Mon 10am)
  - Mon+7 10am: `near_duplicate_of_new_event` (paired with Mon+7 2pm)
  - Mon+7 2pm: `potential_duplicate` (paired with Mon+7 10am)

**Test steps:**
1. Open the review queue. Identify all RS-11 entries.
2. Determine the correct consolidation:
   - **2 series** (morning series + afternoon series): use **Add as Occurrence** (or consolidate API) twice — once per same-day pair. Then manually add the Mon+7 occurrence to each series via `POST /admin/events/{ulid}/occurrences`.
   - **1 series** with 4 occurrences: use **Add as Occurrence** / consolidate three times, promoting the earliest event. Then manually add 3 occurrences.
   - **4 separate events**: click **Not a Duplicate** on each pair (or approve with `record_not_duplicates: true`).
3. Execute your chosen consolidation strategy.
4. Verify:
   - [ ] Final state matches your intent (correct number of events, correct occurrences on each).
   - [ ] All review entries are resolved (no orphaned pending reviews).
   - [ ] Surviving events are `published`.

**Consolidation example (2 morning+afternoon series):**

1. Consolidate the morning pair:
   ```bash
   curl -s -X POST -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" \
     "$BASE/admin/events/consolidate" \
     -d '{"event_ulid":"<mon-10am-ulid>","retire":["<mon7-10am-ulid>"]}'
   ```
   Then manually add the Mon+7 occurrence to the morning series.

2. Consolidate the afternoon pair similarly:
   ```bash
   curl -s -X POST -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" \
     "$BASE/admin/events/consolidate" \
     -d '{"event_ulid":"<mon-2pm-ulid>","retire":["<mon7-2pm-ulid>"]}'
   ```
   Then manually add the Mon+7 occurrence to the afternoon series.

3. If instead you want all 4 as one series:
   ```bash
   curl -s -X POST -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" \
     "$BASE/admin/events/consolidate" \
     -d '{"event_ulid":"<mon-10am-ulid>","retire":["<mon-2pm-ulid>","<mon7-10am-ulid>","<mon7-2pm-ulid>"]}'
   ```
   Then manually add 3 occurrences from the retired events to the canonical. Consolidation retires whole events; occurrence migration is a manual step.

**This scenario intentionally has no single "right" answer -- it tests the admin's judgment and the system's ability to support multiple valid consolidation paths.**

---

### RS-12: Consolidation Endpoint (Create + Promote Paths)

**Setup:** This scenario reuses events from earlier scenarios. It should be run AFTER RS-08 (which creates a merged pair) but can also be run independently with fresh fixture data.

**Pre-test: Ingest two similar events for testing:**
- Use any two events that would normally be near-duplicates (e.g., create two events at the same venue, same date, similar names via the ingest API).

**Test A — Promote Path:**

1. Identify two pending or published events that should be consolidated.
2. Choose one as canonical (better data, more occurrences, etc.).
3. Consolidate:
   ```bash
   curl -s -X POST -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" \
     "$BASE/admin/events/consolidate" \
     -d '{"event_ulid":"<canonical-ulid>","retire":["<dup-ulid>"]}'
   ```
4. Verify:
   - [ ] Response 200 with `retired` containing the retired ULID.
   - [ ] `review_entries_dismissed` lists any dismissed review IDs.
   - [ ] Retired event: `lifecycle_state = 'deleted'`, tombstone with `deletion_reason = 'consolidated'` and `superseded_by_uri` pointing to canonical.
   - [ ] Canonical event: unchanged (same occurrences, same data), `lifecycle_state` reflects post-consolidation validation.

**Test B — Create Path:**

1. Consolidate by creating a new canonical event and retiring both:
   ```bash
   curl -s -X POST -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" \
     "$BASE/admin/events/consolidate" \
     -d '{
       "event": {
         "name": "RS-12 Consolidated Event",
         "startDate": "<RFC3339>",
         "endDate": "<RFC3339>",
         "location": {"name": "The Tranzac", "@id": "<tranzac-place-uri>"}
       },
       "retire": ["<event-a-ulid>", "<event-b-ulid>"]
     }'
   ```
2. Verify:
   - [ ] Response 200. New event created with ULID in response.
   - [ ] Both retired events soft-deleted with tombstones (`deletion_reason = 'consolidated'`).
   - [ ] New event has correct location, dates, lifecycle.
   - [ ] If the new event triggers near-dup detection against a non-retired event, `needs_review: true` and `warnings` array is non-empty.

**Test C — Error Cases:**

1. Both event and event_ulid → 400
2. Neither event nor event_ulid → 400
3. Empty retire list → 400
4. Canonical ULID in retire list → 400
5. Non-existent retire ULID → 404
6. Already-deleted retire target → 422

```bash
# Test: both fields
curl -s -X POST -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" \
  "$BASE/admin/events/consolidate" \
  -d '{"event_ulid":"<ulid>","event":{"name":"test"},"retire":["<ulid2>"]}'
# Expect 400

# Test: empty retire
curl -s -X POST -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" \
  "$BASE/admin/events/consolidate" \
  -d '{"event_ulid":"<ulid>","retire":[]}'
# Expect 400
```

---

## Verification Checklist

After completing all scenarios, verify:

- [ ] No orphaned `pending_review` entries remain from the fixture set (all should be resolved).
- [ ] No orphaned events in `pending_review` state from the fixture set (all should be published or deleted).
- [ ] Tombstone records exist for all soft-deleted events with correct `superseded_by_uri` references (column is `superseded_by_uri`, not `superseded_by`).
- [ ] Tombstone records from consolidation have `deletion_reason = 'consolidated'` and correct `superseded_by_uri`.
- [ ] RS-12 consolidated events appear in the events list with correct lifecycle states.
- [ ] The `event_not_duplicates` table has entries from RS-07 (and RS-11 if applicable).
- [ ] Event occurrence counts match expectations after all add-occurrence actions (verify via the SQL query below or via `GET /api/v1/events/{ulid}` — the `subEvent` array is populated for all occurrences and is the authoritative count used by the admin detail page):
  - RS-01: 4 (base series unchanged after merge; Lu.ma event retired) — Path A (merge duplicate); or 4 + 1 as separate published events for Paths B/C
  - RS-02: 3 (was 2, +1 from near-dup add-occurrence)
  - RS-03: 3 (was 2, +1 from manual add-occurrence — requires manual SQL setup)
  - RS-04: 3 (was 2, +1 from manual add-occurrence — requires manual SQL setup)
  - RS-10: 2 (1+1 — source's sole occurrence is added to the target, giving 2 total)
  - RS-12: canonical event has its own occurrences (Test A: unchanged from pre-consolidation; Test B: new event with dates from request)

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
SELECT e.name, t.deletion_reason, t.superseded_by_uri
FROM event_tombstones t
JOIN events e ON t.event_id = e.id
WHERE e.name LIKE 'RS-%';

-- Check not-duplicates table
SELECT e1.name AS event_a, e2.name AS event_b
FROM event_not_duplicates nd
JOIN events e1 ON e1.ulid = nd.event_id_a
JOIN events e2 ON e2.ulid = nd.event_id_b
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

DELETE FROM event_not_duplicates
WHERE event_id_a IN (SELECT id::text FROM events WHERE name LIKE 'RS-%')
   OR event_id_b IN (SELECT id::text FROM events WHERE name LIKE 'RS-%');

DELETE FROM events WHERE name LIKE 'RS-%';
```

Or for a full database reset (local only): `make migrate-down && make migrate-up && make migrate-river`.
