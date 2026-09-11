# Phase 2 Specification: Agent Adjudication Loop

**Spec**: 007-entity-identity-adjudication / Phase 2 | **Date**: 2026-09-11 | **Status**: Draft
**Parent**: `specs/007-entity-identity-adjudication/plan.md`
**Goal**: An external agent can fetch identity review items, propose a `merge` (or `undo`) with
citations, and the server **validates the proposal in code** before executing a reversible,
provenance-tracked change. Target: a scripted agent completes fetch → propose → execute →
`undo` with **0 unvalidated destructive actions** and **100% of merges reversible to the
recorded pre-image**.

> Phase 1 (delivered): primary slot, election, transactional merge internals, append-only
> `identity_decisions`, signal-scoped suppression, read + link/reject REST/CLI. Phase 2 adds
> the queue, the policy gate, and the reversible `merge`/`undo` loop.

---

## Context

### What Exists Today

| Component | Status | Code reference |
|---|---|---|
| Identity package | Delivered | `internal/identity/{ref,rank,store,execute,record,notduplicate,fingerprint,service,tidy}.go` |
| Primary slot + partial unique index | Delivered | `internal/storage/postgres/migrations/000051_entity_identity_primary.up.sql` |
| `identity_decisions` (append-only, has empty `undo_ref`) | Delivered | `000051`; `internal/identity/record.go` |
| `identity_not_duplicates` (signal-scoped) | Delivered | `000051`; `internal/identity/notduplicate.go` |
| Executor link/reject (atomic + decision) | Delivered | `internal/identity/execute.go` |
| Conflict detection (`entity_id_a < b`, shared authority+uri) | Delivered | `internal/identity/service.go`, `queries/identity.sql` |
| REST read + `POST link/reject` | Delivered | `internal/api/handlers/identity.go`, `internal/api/router.go` |
| CLI `identity check\|conflicts\|link\|reject\|tidy` | Delivered | `cmd/server/cmd/identity.go` |
| Transactional merge internals (**own tx, unexported tx-scoped path; UUID args**) | Delivered | `internal/storage/postgres/events_repository.go` `MergePlaces`/`MergeOrganizations` (`:1754`, `:1907`), `internal/domain/tombstones/` |
| Review/agent pattern to mirror | Live | `server review` CLI (`docs/integration/tg-review.md`), `skills/togather-review/SKILL.md` |

### What This Phase Delivers

1. Migration `000052`: `identity_review_queue`; widen `identity_decisions.action` to
   `link|reject|merge|undo|escalate`; add `identity_review_queue.decision_id`.
2. **Exported tx-scoped merge surface** so a merge can compose the Phase 1 internals into the
   caller's transaction and return a **reversal pre-image** (`MergeEntitiesTx`).
3. **Queue generation** from conflict candidates (idempotent; periodic River worker + refresh).
4. **Policy validator** (code-not-prompt) + `GET /api/v1/admin/identity/policy`.
5. **Merge + Undo + PreviewMerge** (dry-run) executor actions using the reversal pre-image.
6. REST actions + OpenAPI (`queue`, `decision undo`, `policy`).
7. CLI `identity queue|merge|undo` + agent skill doc/playbook + scripted end-to-end test.

### Non-Goals (deferred)

- **No LLM/prompts in the server**; the agent is external; the server only validates + executes.
- **No auto-merge** — every merge is agent-proposed, review-gated, reversible (D3 unchanged).
- **Batch actions deferred to Phase 3** (bulk destructive ops need more design).
- No external KG candidate generation or `sameAs` emission → **Phase 3**.
- No events/persons identity → **Phase 4**; no vector/semantic matching.
- No changes to Phase 1 read/link/reject contracts.

### Design Constraint Reminders

SEL ULID canonical; one primary per `(entity, authority)`; no cross-authority transitivity;
**all destructive actions reversible + tombstoned**; evidence ≠ action; policy validation is
**code, not prompt**; keep-all identifiers; config in `internal/config/config.go`; RFC 7807;
SQLc; migrations via `make`; no vector; MCP unchanged; canonical rank defined once in `plan.md`.

---

## User Scenarios & Testing

### User Story 1 — Fetch the identity review queue (Priority: P0)

**Independent Test**: Seed conflicts, run queue generation, and assert `pending` items exist with
evidence + server-computed score and are idempotent on re-run.

**Acceptance Scenarios**
1. Given two places sharing an `artsdata` URI, when queue generation runs, then one `pending`
   item exists for the pair with `evidence` and a server-computed `score`.
2. Given the same conflicts, when generation runs again, then no duplicate `pending` item is
   created (partial-unique `ON CONFLICT ... DO NOTHING`).
3. Given a pair already suppressed by `identity_not_duplicates` (matching fingerprint), when
   generation runs, then no item is created.

### User Story 2 — Policy validation refuses unsafe proposals (Priority: P0)

**Independent Test**: Submit proposals that violate policy and assert escalation with no
destructive change.

**Acceptance Scenarios**
1. Given a `merge` proposal with **no citations** and `IDENTITY_REQUIRE_CITATIONS=true`, when
   validated, then the outcome is `escalate` (reason `missing_citations`); no merge occurs and
   an `escalate` decision is appended.
2. Given a `merge` where either entity has more live events than `IDENTITY_MERGE_MAX_FAN_OUT`,
   when validated, then the outcome is `escalate` (reason `fan_out_exceeded`).
3. Given a request whose action string is not one of `link|reject|merge|undo`, when handled,
   then it is a **structural** `400` (unknown verb), not an escalation.
4. Given `GET /api/v1/admin/identity/policy`, then the returned limits equal the enforced config.

### User Story 3 — Merge a duplicate (review-gated, reversible) (Priority: P0)

**Independent Test**: Propose a valid merge; assert the survivor keeps the union, the duplicate
is tombstoned, and the decision records a complete reversal pre-image.

**Acceptance Scenarios**
1. Given a queued pair and a valid proposal (`primary_id` required), when merge executes, then
   the duplicate's identifiers/events move to the primary, the duplicate is soft-deleted +
   tombstoned, exactly one primary remains, and a `merge` decision is appended (`reversible=true`)
   carrying `ReversalMaterial`.
2. Given the merge, when the queue item is read, then `status='merged'` and `decision_id` points
   at the merge decision.
3. Given `merge --dry-run` (or `PreviewMerge`), then no rows change and the planned reverse steps
   are returned.

### User Story 4 — Undo a merge (Priority: P0)

**Independent Test**: Undo a merge decision; assert the recorded pre-image is restored.

**Acceptance Scenarios**
1. Given a merge decision `D`, when `undo D` executes, then a new decision (`action=undo`,
   `undo_ref=D`) is appended, the duplicate row is restored (merged_into cleared, deleted_at/
   deletion_reason reverted), moved identifiers/events are returned, created tombstones are
   deleted, and the pre-merge primary flags are restored.
2. Given the undo, when the queue item is read, then `status='pending'` (re-opened) and its
   `decision_id` is cleared.
3. Given `undo` of a non-reversible decision or an already-undone decision, then it is a
   structural refusal and **no** undo decision is written.

### User Story 5 — Agent CLI parity (Priority: P1)

**Independent Test**: `server identity queue` and the REST queue return identical items;
`server identity merge --dry-run` previews without mutating.

**Acceptance Scenarios**
1. Given pending items, when fetched via API and CLI, then the fields match.
2. Given `merge --dry-run`, then no rows change and the preview matches `PreviewMerge`.

---

## Technical Design

### Package Layout

```
internal/identity/reversal/          # LEAF package (stdlib only) — avoids postgres import cycle
  reversal.go       # Material, IdentifierPreImage, Apply
internal/identity/
  evidence.go       # Evidence struct (shared)
  queue.go          # ReviewItem, ReviewQueueFilter, QueueStore, generation from conflicts
  policy.go         # Policy, ActionValidator (allowed set, citations, red-lines, fan-out)
  execute.go        # extend Executor: PreviewMerge, Merge, Undo
internal/storage/postgres/entity_merge.go   # NEW exported tx-scoped MergeEntitiesTx + ULID->UUID
internal/jobs/identity_queue.go             # River periodic worker: refresh queue
internal/api/handlers/identity.go           # add queue view + merge/undo/policy handlers
cmd/server/cmd/identity.go                  # add queue|merge|undo
internal/storage/postgres/queries/identity.sql
internal/storage/postgres/migrations/000052_identity_review_queue.{up,down}.sql
docs/integration/tg-identity.md ; skills/togather-identity/SKILL.md
docs/api/openapi.yaml
```

### Data Structures

```go
// internal/identity/evidence.go  (NEW this phase)
type Evidence struct {
    Kind       string  `json:"kind"`       // "identifier" | "name_similarity" | "url"
    Authority  string  `json:"authority"`  // when Kind=="identifier"
    URI        string  `json:"uri"`        // when Kind=="identifier"
    Confidence float64 `json:"confidence"` // 0..1, server-computed
    Source     string  `json:"source"`     // reconciliation|enrichment_sameas|manual|agent
}

// internal/identity/queue.go
type ReviewStatus string // pending|merged|escalated  (rejected/dismissed deferred to Phase 3)

// Status transitions written in Phase 2:
//   pending -> merged      (merge)
//   pending -> escalated   (policy violation)
//   merged  -> pending     (undo)
type ReviewItem struct {
    ID              string       `json:"id"`           // "idr-{ulid}"
    EntityType      EntityType   `json:"entity_type"`
    SubjectID       string       `json:"subject_id"`
    CounterpartType EntityType   `json:"counterpart_type"`
    CounterpartID   string       `json:"counterpart_id"`
    Evidence        []Evidence   `json:"evidence"`
    Score           float64      `json:"score"`        // server-computed (max evidence confidence)
    Status          ReviewStatus `json:"status"`
    CreatedAt       time.Time    `json:"created_at"`
    DecidedBy       *string      `json:"decided_by,omitempty"`
    DecidedAt       *time.Time   `json:"decided_at,omitempty"`
    Rationale       string       `json:"rationale"`
    Citations       []string     `json:"citations"`    // refs: decision id ("idn-*") or rule id
    DecisionID      *string      `json:"decision_id,omitempty"` // resolving decision
}

type ReviewQueueFilter struct {
    Status   []ReviewStatus
    Type     EntityType // "" = both
    Limit    int
    Cursor   string
}

// internal/identity/policy.go
type Policy struct {
    AllowedActions     []string `json:"allowed_actions"`      // link|reject|merge|undo
    RequireCitations   bool     `json:"require_citations"`
    MergeMinScore      float64  `json:"merge_min_score"`      // compared to ReviewItem.Score
    MergeMaxFanOut     int      `json:"merge_max_fan_out"`
    RedLines           []string `json:"red_lines"`            // reason codes (below)
}

// Red-line reason codes (fixed vocabulary):
//   missing_citations | fan_out_exceeded | score_below_min | unknown_authority | self_pair
type ValidationOutcome struct {
    Allowed  bool   `json:"allowed"`
    Action   string `json:"action"`
    Escalate string `json:"escalate_reason,omitempty"`
}

// internal/identity/reversal/reversal.go  (leaf; full pre-image so undo restores state)
type IdentifierPreImage struct {
    ID int32 `json:"id"`; EntityType string `json:"entity_type"`; EntityID string `json:"entity_id"`
    Authority string `json:"authority"`; URI string `json:"uri"`; Confidence float64 `json:"confidence"`
    Method string `json:"method"`; IsPrimary bool `json:"is_primary"`; Source string `json:"source"`
    SupersededByID *int32 `json:"superseded_by_id,omitempty"`; ObservedAt time.Time `json:"observed_at"`
    Metadata []byte `json:"metadata,omitempty"`
}
type ReversalMaterial struct {
    EntityType        EntityType          `json:"entity_type"`
    DuplicateULID     string              `json:"duplicate_ulid"`
    PrimaryULID       string              `json:"primary_ulid"`
    IdentifiersBefore []IdentifierPreImage `json:"identifiers_before"` // all rows of BOTH entities pre-merge
    MovedEventULIDs   []string            `json:"moved_event_ulids"`
    MovedOccurrenceIDs []string           `json:"moved_occurrence_ids"`
    TombstoneIDs      []string            `json:"tombstone_ids"`       // created by merge
    DuplicatePrior    struct {
        MergedIntoID   *string `json:"merged_into_id,omitempty"`
        DeletedAt      *time.Time `json:"deleted_at,omitempty"`
        DeletionReason *string `json:"deletion_reason,omitempty"`
    } `json:"duplicate_prior"`
    PrimaryFieldsBefore map[string]any    `json:"primary_fields_before"` // gap-filled survivor columns incl. updated_at
}
```

### Interfaces

```go
// internal/storage/postgres/entity_merge.go  (NEW: composes Phase 1 internals into caller tx)
// The caller supplies a pgx.Tx; MergeEntitiesTx builds &EventRepository{tx: tx} internally
// (the existing mergePlacesTx/mergeOrganizationsTx run through r.queryer() -> pgx.Tx) and
// resolves ULID -> internal UUID before merging, returning the reversal pre-image.
// internal/identity/reversal is a LEAF package, so postgres -> reversal is cycle-free
// (internal/identity already imports internal/storage/postgres).
func (r *EventRepository) MergeEntitiesTx(ctx context.Context, tx pgx.Tx,
    entityType, duplicateULID, primaryULID string) (reversal.Material, error)

// The executor drives merge + decision + queue-mark in ONE tx:
type EntityMerger interface {
    WithMergeTx(ctx context.Context, fn func(q *postgres.Queries, tx pgx.Tx) error) error
    MergeEntitiesTx(ctx context.Context, tx pgx.Tx, entityType, dupULID, priULID string) (reversal.Material, error)
}

// internal/identity/queue.go
type QueueStore interface {
    Enqueue(ctx context.Context, item ReviewItem) (ReviewItem, error) // idempotent
    Get(ctx context.Context, id string) (ReviewItem, error)
    List(ctx context.Context, f ReviewQueueFilter) ([]ReviewItem, error)
    Mark(ctx context.Context, id string, status ReviewStatus, decidedBy, rationale string, citations []string, decisionID *string) error
    Reopen(ctx context.Context, id string) error // merged -> pending (undo)
}

// internal/identity/policy.go
type ActionValidator interface {
    Policy() Policy
    // Structural errors (unknown action verb, malformed) are returned as errors;
    // policy violations set Escalate (never an error).
    Validate(ctx context.Context, item ReviewItem, rec DecisionRecord) (ValidationOutcome, error)
}

// internal/identity/execute.go (extended)
func (e *Executor) PreviewMerge(ctx context.Context, item ReviewItem, primary IdentityRef) (ReversalMaterial, error)
func (e *Executor) Merge(ctx context.Context, item ReviewItem, primary IdentityRef, actor, rationale string, citations []string) (DecisionRecord, error)
func (e *Executor) Undo(ctx context.Context, mergeDecisionID, actor string) (DecisionRecord, error)

type Writer interface { // extended (handlers depend on this)
    LinkIdentifier(ctx context.Context, ref IdentityRef, obs IdentifierObservation, actor string) (DecisionRecord, error)
    Reject(ctx context.Context, a, b IdentityRef, actor, reason string) (DecisionRecord, error)
    PreviewMerge(ctx context.Context, item ReviewItem, primary IdentityRef) (ReversalMaterial, error)
    Merge(ctx context.Context, item ReviewItem, primary IdentityRef, actor, rationale string, citations []string) (DecisionRecord, error)
    Undo(ctx context.Context, mergeDecisionID, actor string) (DecisionRecord, error)
}
```

`Executor` gains dependencies: `queue QueueStore`, `validator ActionValidator`,
`merges EntityMerger` (wraps `MergeEntitiesTx`), `decisions DecisionStore`, `audit`. `Merge`
runs one transaction: `validator.Validate` → (if escalate) append `escalate` decision + mark
queue `escalated`; else `merges.MergeEntitiesTx` → persist `ReversalMaterial` into the decision
`metadata` → append `merge` decision → mark queue `merged` with `decision_id`. `Undo` reads the
target decision's `metadata` reversal material and applies `reversal.Apply` in one tx, then
appends an `undo` decision (`undo_ref=mergeDecisionID`) and reopens the queue item.

### Migration `000052` (up + down)

```sql
-- UP
CREATE TABLE identity_review_queue (
  id TEXT PRIMARY KEY,
  entity_type TEXT NOT NULL CHECK (entity_type IN ('place','organization')),
  subject_id TEXT NOT NULL,
  counterpart_type TEXT NOT NULL CHECK (counterpart_type IN ('place','organization')),
  counterpart_id TEXT NOT NULL,
  evidence JSONB NOT NULL DEFAULT '[]'::jsonb,
  score NUMERIC(5,4) NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending','merged','escalated')),
  decided_by TEXT, decided_at TIMESTAMPTZ,
  rationale TEXT NOT NULL DEFAULT '', citations JSONB NOT NULL DEFAULT '[]'::jsonb,
  decision_id TEXT REFERENCES identity_decisions(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- one live item per UNORDERED pair: generation canonicalizes subject_id < counterpart_id
-- (by ULID) before insert, so the ordered unique key below enforces it.
CREATE UNIQUE INDEX idx_identity_review_queue_pending
  ON identity_review_queue(entity_type, subject_id, counterpart_type, counterpart_id)
  WHERE status='pending';
CREATE INDEX idx_identity_review_queue_subject ON identity_review_queue(entity_type, subject_id);

ALTER TABLE identity_decisions DROP CONSTRAINT identity_decisions_action_check;
ALTER TABLE identity_decisions ADD CONSTRAINT identity_decisions_action_check
  CHECK (action IN ('link','reject','merge','undo','escalate'));
-- 000051 already has identity_decisions.undo_ref (currently always empty); Phase 2 uses it as
-- "the decision this row undoes" (decision -> undone-decision). No new column.

-- DOWN: delete rows that would violate the narrower set, then restore it, then drop the queue.
DELETE FROM identity_review_queue;
DROP INDEX IF EXISTS idx_identity_review_queue_subject;
DROP INDEX IF EXISTS idx_identity_review_queue_pending;
DROP TABLE IF EXISTS identity_review_queue;
DELETE FROM identity_decisions WHERE action IN ('merge','undo','escalate');
ALTER TABLE identity_decisions DROP CONSTRAINT identity_decisions_action_check;
ALTER TABLE identity_decisions ADD CONSTRAINT identity_decisions_action_check
  CHECK (action IN ('link','reject'));
```
(Constraint name `identity_decisions_action_check` verified present in `000051`.)

### REST / CLI Schemas

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/admin/identity/queue?status=pending&type=&limit=&cursor=` | List items |
| GET | `/api/v1/admin/identity/queue/{id}` | Item + evidence + prior decisions |
| POST | `/api/v1/admin/identity/queue/{id}/merge` | Body below; `?dry_run=true` for preview |
| POST | `/api/v1/admin/identity/decisions/{id}/undo` | Undo a merge decision |
| GET | `/api/v1/admin/identity/policy` | Read-only enforced limits |

**GET `/queue`** → `200`:
```json
{ "items": [ { "id":"idr-...", "entity_type":"place", "subject_id":"01...", "counterpart_type":"place",
  "counterpart_id":"01...", "evidence":[{"kind":"identifier","authority":"artsdata",
  "uri":"https://kg.artsdata.ca/resource/K11-24","confidence":0.99,"source":"reconciliation"}],
  "score":0.99, "status":"pending", "created_at":"...", "rationale":"", "citations":[] } ],
  "next_cursor": null }
```
**POST `/queue/{id}/merge`** body:
```json
{ "primary_id": "01...", "rationale": "same venue, same address", "citations": ["idn-..."] }
```
→ `201` the appended `DecisionRecord` (merge). On policy violation → `200` with
`{ "escalated": true, "reason": "missing_citations", "decision": { ...action:"escalate" } }`.
**POST `/decisions/{id}/undo`** → `201` the appended `undo` `DecisionRecord`.
**GET `/policy`** → `200` `Policy`.

`actor` is always the JWT subject. `primary_id` is **required** (no default survivor); the merge
decision's `entity_id` is the primary and `counterpart_id` is the duplicate.

CLI (mirrors `server review`):
```
server identity queue [--status pending] [--type place|organization] [--json]
server identity merge <type> <subject-id> <counterpart-id> --primary <id> --rationale <t> --citation <ref> [--dry-run] [--json]
server identity undo  <merge-decision-id> [--json]
```

`merge` resolves the `pending` queue item for the canonical pair (`subject_id < counterpart_id`)
and passes its id to the REST merge endpoint (404 if no pending item), so CLI and REST share one
contract.

### Error Handling

- **Structural** (unknown action verb, missing `primary_id`, bad ULID/type, malformed body) →
  `400` RFC 7807.
- **Not found** (queue item / entity / decision absent) → `404`.
- **Semantic/policy** (missing citations, fan-out red-line, score below min, non-reversible or
  already-undone target) → **not a 5xx**: append an `escalate` decision (or refuse the undo
  structurally with a clear reason) and leave the destructive action undone. Merge policy
  violations return `200` with `escalated: true`.
- **Concurrency**: merge/undo run in one tx with the existing per-group advisory lock +
  `SKIP LOCKED`; a concurrent decision on the same pair yields a classed `409`.

### Security Model

- **Authorization**: all endpoints/verbs admin-role (JWT); `actor` from JWT subject.
- **Trust boundary**: proposals (action, `primary_id`, citations, rationale) are **untrusted**;
  the server validates in code and uses its own `ReviewItem.Score` — it never reads an
  agent-supplied confidence.
- **Injection/delimiting**: `evidence` and untrusted agent text (`rationale`) returned to the
  agent are boundary-delimited (`internal/llmsafe`); the policy validator is the backstop so a
  compromised/injected agent cannot run an out-of-set action or an invariant-violating merge.
- **Reversibility**: merge records a full pre-image; undo is a new decision; history immutable;
  no hard deletes.
- **Data lifecycle**: `identity_review_queue` durable Postgres, deploy-snapshot covered; status
  history retained (no-prune default, consistent with decisions).

---

## Implementation Tasks

### Task 1: Migration `000052` (review queue, action widening incl. escalate, undo_ref semantics)
**What**: Create `identity_review_queue` (partial-unique on `pending`, `decision_id` FK); widen
`identity_decisions.action` to `link|reject|merge|undo|escalate`; document `undo_ref` as the
decision→undone-decision link; working down (delete merge/undo/escalate rows then narrow).
Regenerate SQLc.
**Test**: up/down/up; second `pending` insert for the pair is a no-op under the partial unique;
`action='escalate'` insert succeeds; `make sqlc` no diff.
**Acceptance**: round-trips cleanly; constraints/enums match; generated code in sync.

### Task 2: Exported tx-scoped merge surface + reversal pre-image
**What**: `internal/storage/postgres/entity_merge.go` — exported `MergeEntitiesTx(ctx, q, type,
duplicateULID, primaryULID) (ReversalMaterial, error)` that resolves ULID→UUID and runs the
existing internal merge on the caller's transaction, returning the full `IdentifiersBefore` +
moved-event/occurrence ids + created tombstone ids + duplicate prior state + survivor gap-filled
columns. Refactor `mergePlacesTx`/`mergeOrganizationsTx` to hand back that pre-image.
**Test**: testcontainer test merges two seeded entities inside a caller tx and asserts the
returned pre-image matches the DB before/after; rollback on caller error.
**Acceptance**: pre-image is complete enough for `reversal.Apply` to restore state; no behavior
change to the existing merge tests.

### Task 3: Queue generation
**What**: `internal/identity/queue.go` generation from conflict candidates (skip suppressed
fingerprints), idempotent `Enqueue` (`ON CONFLICT ... WHERE status='pending' DO NOTHING`);
`internal/jobs/identity_queue.go` River periodic worker + `QueueStore` refresh; config interval.
**Test**: seed conflicts → items created with evidence+score; re-run no dupes; suppressed skipped;
testcontainer job test.
**Acceptance**: `pending` items match conflicts; idempotent; no item for suppressed pairs.

### Task 4: Policy validator + `/policy`
**What**: `internal/identity/policy.go` enforcing the allowed set, citation requirement,
`ReviewItem.Score >= MergeMinScore`, fan-out red-line (live-event counts), and the fixed
red-line codes; `GET /api/v1/admin/identity/policy`; add config fields; update the OpenAPI lint
manifest and `plan.md`.
**Test**: unit tests per rule (allow + escalate cases); endpoint equals config; unknown action
verb is a structural error.
**Acceptance**: every violation yields `escalate` (no destructive change); policy endpoint == config.

### Task 5: Merge + Undo + PreviewMerge
**What**: `Executor.PreviewMerge` (no writes; returns the planned `ReversalMaterial`),
`Executor.Merge` (validate → `MergeEntitiesTx` in one tx → persist pre-image in the decision
`metadata` → append `merge` decision → mark queue `merged`/`decision_id`; policy violation →
append `escalate` + mark `escalated`), `Executor.Undo` (read pre-image → `reversal.Apply` in one
tx → append `undo` decision with `undo_ref` → reopen queue item). Extend `Executor` deps +
`Writer`; update construction in `router.go`.
**Test**: merge round-trip (identifiers/events/tombstone/decision); policy escalation path;
preview mutates nothing; merge→undo restores the recorded pre-image; double/non-reversible undo
refused; concurrency 409.
**Acceptance**: merge is atomic, review-gated, reversible to the recorded pre-image; append-only
history preserved.

### Task 6: REST actions + OpenAPI
**What**: queue list/get + merge (`?dry_run`) + decision undo + policy handlers; wire `jwtAuth` +
`AdminRequestSize`; OpenAPI for all endpoints incl. escalation/404/409 bodies; lint manifest for
env-tunable limits.
**Test**: handler tests (200/201/400/404/409 + escalation), keyset cursors, dry-run,
`make lint-openapi`.
**Acceptance**: shapes match the schemas; unknown action → 400; policy violation → `escalated`
not 5xx; auth enforced.

### Task 7: CLI + agent skill doc + end-to-end verification
**What**: `cmd/server/cmd/identity.go` `queue|merge|undo` (`--dry-run`, `--json`, STS auth);
`docs/integration/tg-identity.md` + `skills/togather-identity/SKILL.md`; a scripted end-to-end
test (seed conflict → fetch queue → merge → undo → assert state + decisions).
**Test**: CLI↔REST parity; `merge --dry-run` matches `PreviewMerge`; the scripted loop passes.
**Acceptance**: an external agent can follow the doc; the scripted loop proves fetch→act→undo.

---

## Configuration

| Env var | Default | Purpose |
|---|---|---|
| `IDENTITY_MERGE_MIN_SCORE` | `0.90` | Min server-computed `ReviewItem.Score` to merge |
| `IDENTITY_REQUIRE_CITATIONS` | `true` | Require ≥1 citation for non-escalate actions |
| `IDENTITY_MERGE_MAX_FAN_OUT` | `25` | Live events on either entity above which merge escalates |
| `IDENTITY_QUEUE_REFRESH_MINUTES` | `60` | Periodic queue-generation interval |

Add these to `plan.md`'s config table; keep `IDENTITY_MERGE_MAX_FAN_OUT`. Env-controlled
endpoint behaviour must name the env var + default in the OpenAPI `description`
(`internal/config/openapi_lint_test.go`).

## Success Criteria

- Scripted agent: fetch queue → merge → `undo` restores the recorded pre-image; 0 unvalidated
  destructive actions; 100% of merge actions reversible.
- Policy violations convert to escalation (no 5xx, no destructive change); unknown action verb
  is a 400.
- `GET .../identity/policy` equals the enforced config.
- `make ci-fast` green; `make lint-openapi` green; new unit + integration tests pass.
- No regression in Phase 1 behaviour (invariant holds; `reconcile stats` unchanged).

## Open Questions

- PQ2.1 — Queue cadence: periodic River worker only, or also an on-demand refresh endpoint/flag?
  (Proposed: periodic worker + `server identity queue --refresh`.)
- PQ2.2 — `undo` authorization window: unlimited (proposed; pre-image is durable) or time-boxed?
- PQ2.3 — Batch actions (deferred to Phase 3): confirm deferral is acceptable given bulk-merge
  risk.
