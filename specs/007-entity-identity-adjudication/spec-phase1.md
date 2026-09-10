# Phase 1 Specification: Identity Primitives, Repair & Read Loop

**Spec**: 007-entity-identity-adjudication / Phase 1 | **Date**: 2026-09-10 | **Status**: Draft
**Parent**: `specs/007-entity-identity-adjudication/plan.md`
**Goal**: The node stores at most one **primary** external identifier per
`(entity, authority)`, enforced by a DB index; every place/organization merge preserves
identity (no orphaned `entity_identifiers`); and an external agent can **read** identity
state and **record link/reject decisions** for a place/org via the admin REST API and
`server identity` CLI. Target: **0 rows** violating the primary invariant and **0 orphaned
identifiers** for places/orgs after a merge, verified on staging.

> Phase 1 is deterministic. It introduces the model, repairs existing drift, and exposes a
> minimal but complete agent consumer (read + link/reject via CLI, read via HTTP). The
> queue-driven action loop (candidate queue, merge over HTTP, policy validator, undo,
> batch) is Phase 2. The canonical primary-election order is defined once in `plan.md`
> (§ Component Design) and referenced here.

---

## Context

### What Exists Today

| Component | Status | Code reference |
|---|---|---|
| `entity_identifiers` (sameAs edges) | Write-only; `is_canonical` per-row predicate | `internal/kg/reconciliation.go:330,332`, `internal/jobs/workers.go:422` |
| Writers | reconciliation + enrichment | `internal/kg/reconciliation.go:239-275`, `internal/jobs/workers.go:417-441` |
| Readers | generated SQLc methods exist but **uncalled**; only CLI raw SQL | `cmd/server/cmd/reconcile.go:307,443,571` |
| Primary/unique constraint | `UNIQUE(entity_type, entity_id, authority_code, identifier_uri)` (permits multi-canonical) | `migrations/000030_knowledge_graph_tables.up.sql:36` |
| Authority URI patterns | seeded; **artsdata pattern is `http://`-only** while code emits `https://` | `000030:63`; `internal/kg/reconciliation.go:22,105` |
| `knowledge_graph_authorities` (trust/priority) | Seeded, unused by logic | `000030:62-67` |
| Place merge | reassigns events/occurrences; sets `deletion_reason='merged'`; **does not touch identifiers**; no tombstone; **not transactional** | `internal/storage/postgres/events_repository.go:1717`, called via `internal/domain/events/admin_service.go:2589` |
| Org merge | reassigns events; sets `deletion_reason='merged'`; **does not touch identifiers**; no tombstone; **not transactional** | `events_repository.go:1839`, `admin_service.go:2595` |
| Event merge | omits `deletion_reason` | `queries/events.sql:78-85` |
| Tombstone storage | `place_tombstones`/`organization_tombstones` exist; **merge paths don't write them**; existing writers take a URI+payload | `migrations/000008`/`000009`; `places_repository.go:506-532`; handler payload `admin.go` `buildPlaceTombstonePayload` |
| ULID validation helper | `ValidateAndExtractULID` always returns **400** | `internal/api/handlers/validators.go:25` |
| Admin auth | JWT via `POST /api/v1/auth/token` | `internal/api/router.go`, `docs/deploy/api-keys.md` |
| Review CLI pattern | Live agent contract to mirror | `docs/integration/tg-review.md` |

**Observed drift (staging, 2026-09-10 — runtime observation, not repo-verifiable):**
172 `is_canonical=true` artsdata rows across only 144 places (~28 places with >1 canonical).

### What This Phase Delivers

1. Migration `000051`: primary-slot columns + partial unique index + backfill; broaden
   authority URI patterns scheme-insensitively; append-only `identity_decisions`;
   signal-scoped `identity_not_duplicates`; missing merge indexes; **down migration**.
2. Deterministic **primary election**, integrated into reconciliation/enrichment writes
   atomically (one SQL statement), refreshing `observed_at`; stop writing `is_canonical`.
3. **Merge integrity**: `MergePlaces`/`MergeOrganizations` made **transactional**, reassign +
   dedupe identifiers (ULID-resolved), write tombstones, and re-elect a primary.
4. `LinkIdentifier` / `Reject` executor + decision store, exposed via **admin REST
   (`POST` link/reject)** and CLI verbs, so the slice is end-to-end usable by an external agent.
5. Admin REST: identity view + conflicts + decisions feed (read) and link/reject actions
   (write); OpenAPI + lint manifest.
6. `server identity check|conflicts|link|reject` CLI.
7. `server identity tidy --dry-run|--apply` repair (folds `t_e522e110`).

### Non-Goals (deferred)

- No candidate **queue**, no `merge`/`undo`, no policy validator, no batch operations →
  **Phase 2** (link/reject HTTP actions are in Phase 1).
- No LLM, no prompts, no MCP tools.
- No external KG **candidate generation** or `sameAs` emission → **Phase 3**.
- No events/persons identity → **Phase 4** (events remain on the existing `event_not_duplicates`
  mechanism, which is safe long-term because events age out).
- No vector/semantic matching.

### Design Constraint Reminders

Carried from the plan (not repeated in full): SEL ULID is canonical and external URIs are
aliases; exactly one primary per `(entity, authority)`; no cross-authority transitivity;
destructive actions reversible + tombstoned; evidence ≠ action; keep-all identifiers;
config in `internal/config/config.go`; RFC 7807; SQLc; migrations via `make`; no vector;
MCP unchanged.

---

## User Scenarios & Testing

### User Story 1 — One primary identifier per authority (Priority: P0)

**Independent Test**: Insert two identifier observations for the same place+authority and
assert the DB rejects a second primary and the rank elects the correct winner.

**Acceptance Scenarios**
1. Given a place with existing `artsdata` observations at method `auto_high` confidence 0.99
   and `auto_low` confidence 0.90, when election runs, then exactly one row has
   `is_primary=true` and it is the `auto_high` row.
2. Given a place whose current primary is `artsdata` `K11-24` at method `auto_low`, when
   reconciliation writes `K11-99` at method `auto_high`, then `K11-99` becomes primary and
   `K11-24` is demoted (`is_primary=false`, `superseded_by_id` set) and retained.
   (Methods differ so the rank outcome is determinate.)
3. Given any entity+authority, when a second `is_primary=true` row is inserted directly via
   SQL, then the partial unique index raises a unique violation.

### User Story 2 — Merge preserves identity (Priority: P0)

**Independent Test**: Create two places each with an `artsdata` identifier, merge one into the
other, assert the survivor holds both identifiers, the duplicate is tombstoned, and no
`entity_identifiers` row references a soft-deleted ULID.

**Acceptance Scenarios**
1. Given place A (`K11-24`) and place B (`K11-25`) with no shared identifier, when A is merged
   into B, then B's identifier set contains both `K11-24` and `K11-25`, exactly one is
   primary, and A has `merged_into_id=B`, `deleted_at` set, `deletion_reason='merged'`.
2. Given the same merge, then a `place_tombstones` row exists for A with non-null
   `place_uri` and `payload`, and `SELECT count(*) FROM entity_identifiers WHERE entity_id=A`
   is 0.
3. Given the same merge, when a failure is injected after identifier reassignment but before
   commit, then the transaction rolls back: identifiers still belong to A and A is not
   soft-deleted.
4. Given a merge where both A and B already assert the same authority+URI, then the union is
   deduplicated and one primary remains.

### User Story 3 — Inspect identity via API and CLI (Priority: P0)

**Independent Test**: `GET /api/v1/admin/identity/place/{ulid}` and
`server identity check place {ulid}` return the same identifiers, primary map, and decisions.

**Acceptance Scenarios**
1. Given a reconciled place, when the identity view is fetched, then it lists all identifiers
   with authority/method/confidence/`is_primary`/`observed_at` and a `primary` map keyed by
   authority.
2. Given a place with prior decisions, when fetched, then `decisions` is ordered
   `created_at DESC, id DESC`.
3. Given an unknown ULID, when fetched, then `400` (invalid ULID) or `404` (valid, absent)
   with an RFC 7807 body.

### User Story 4 — Record link / reject decisions (Priority: P1)

**Independent Test**: Run `server identity link` then `server identity reject`, and assert two
`identity_decisions` rows and one `identity_not_duplicates` row exist; for `link`, the
identifier is present and elected primary.

**Acceptance Scenarios**
1. Given an entity and a validated external URI, when `LinkIdentifier` executes, then an
   `entity_identifiers` row is upserted with `source`, `observed_at=now()`, the primary slot
   is (re-)elected, and a decision record is appended with `reversible=true`.
2. Given two entities judged distinct, when `Reject` executes, then an
   `identity_not_duplicates` row is recorded (canonical ordering `id_a < id_b`) with an
   **evidence fingerprint**, and the pair disappears from the conflicts list.
3. Given an executor call with an empty actor, then it returns a structural error and writes
   no decision.
4. Given a `Reject` for a pair, when the candidate generator later sees materially new
   evidence (fingerprint differs), then the pair may resurface with the prior decision
   attached; identical evidence keeps it suppressed indefinitely.

### User Story 5 — Repair existing drift (Priority: P1)

**Independent Test**: Seed the legacy state (schema with the index dropped) with two primary
rows for a group, run `server identity tidy --dry-run` then `--apply`, and assert one primary
remains; also seed a group with observations but no primary and assert tidy fills it.

**Acceptance Scenarios**
1. Given drift, when `tidy --dry-run` runs, then it prints affected entities and mutates
   nothing.
2. Given drift, when `tidy --apply` runs, then the invariant holds and a summary reports
   entities scanned / rows demoted / primaries filled.
3. Given no drift, when `tidy --apply` runs, then it reports 0 changes and exits 0.

---

## Technical Design

### Package Layout

```
internal/identity/
  ref.go            # EntityType, IdentityRef
  rank.go           # IdentifierObservation, ElectPrimary
  execute.go        # Executor: LinkIdentifier, Reject
  record.go         # DecisionRecord, DecisionStore contract
  service.go        # IdentityView / ConflictItem / DecisionFeedItem assembly
internal/storage/postgres/queries/identity.sql        # SQLc: ElectPrimary, views, decisions, not-duplicates
internal/storage/postgres/migrations/000051_entity_identity_primary.{up,down}.sql
internal/api/handlers/identity.go                     # GET view + conflicts + decisions feed
cmd/server/cmd/identity.go                            # check | conflicts | link | reject | tidy
internal/kg/reconciliation.go                         # writers use ElectPrimary
internal/jobs/workers.go                              # enrichment writer uses ElectPrimary
internal/storage/postgres/events_repository.go        # transactional merge + identifier reassignment + tombstone
internal/config/openapi_lint_test.go                  # add IDENTITY_CONFLICT_LIMIT_MAX to manifest
docs/api/openapi.yaml                                 # new read endpoints
```

### Data Structures

```go
// internal/identity/ref.go
type EntityType string // "place" | "organization"

type IdentityRef struct {
    Type EntityType `json:"entity_type"`
    ULID string     `json:"entity_id"`
}

// internal/identity/rank.go
type IdentifierObservation struct {
    ID         int32   // entity_identifiers.id (SERIAL)
    Authority  string
    URI        string
    Method     string  // manual|imported|auto_high|auto_low|enrichment_sameas
    Confidence float64
    ObservedAt time.Time
    TrustLevel int     // knowledge_graph_authorities.trust_level
    Priority   int     // knowledge_graph_authorities.priority_order
    Source     string  // reconciliation|enrichment_sameas|manual|agent
}

// ElectPrimary uses the canonical order defined in plan.md:
// method rank > authority trust_level DESC/priority ASC > confidence DESC > observed_at DESC.
func ElectPrimary(obs []IdentifierObservation) (IdentifierObservation, bool)

// internal/identity/record.go — flat, mirrors identity_decisions columns and JSON.
type DecisionRecord struct {
    ID              string     `json:"id"`           // "idn-{ulid}"
    Timestamp       time.Time  `json:"timestamp"`
    EntityType      EntityType `json:"entity_type"`
    EntityID        string     `json:"entity_id"`
    Action          string     `json:"action"`       // link|reject
    CounterpartType *EntityType `json:"counterpart_type,omitempty"`
    CounterpartID   *string    `json:"counterpart_id,omitempty"`
    Rationale       string     `json:"rationale"`
    Citations       []string   `json:"citations"`
    Confidence      float64    `json:"confidence"`
    Actor           string     `json:"actor"`
    Reversible      bool       `json:"reversible"`
    UndoRef         string     `json:"undo_ref,omitempty"`
}

// internal/identity/service.go — REST response types (full structs per specs/AGENTS.md).
type IdentifierView struct {
    Authority  string    `json:"authority"`
    URI        string    `json:"uri"`
    Method     string    `json:"method"`
    Confidence float64   `json:"confidence"`
    IsPrimary  bool      `json:"is_primary"`
    Source     string    `json:"source"`
    ObservedAt time.Time `json:"observed_at"`
}

type IdentityView struct {
    Ref         IdentityRef       `json:"ref"`
    Identifiers []IdentifierView  `json:"identifiers"`
    Primary     map[string]string `json:"primary"` // authority -> URI
    Decisions   []DecisionRecord  `json:"decisions"`
}

type ConflictItem struct {
    Ref        IdentityRef `json:"ref"`
    Candidate  IdentityRef `json:"candidate"`
    Authority  string      `json:"authority"`
    URI        string      `json:"uri"`
    Score      float64     `json:"score"`
    Suppressed bool        `json:"suppressed"` // true if an identity_not_duplicates row exists
}

type ConflictsResponse struct {
    Items      []ConflictItem `json:"items"`
    NextCursor *string        `json:"next_cursor"` // opaque; encodes (created_at, id of last id_a)
}

type DecisionsResponse struct {
    Items      []DecisionRecord `json:"items"`
    NextCursor *string          `json:"next_cursor"` // opaque; encodes (created_at, id of last row)
}
```

**Cursor semantics**: opaque base64 of `(created_at, id)` of the last returned row; ordering is
`created_at DESC, id DESC`; `since` is an RFC3339 timestamp (inclusive lower bound on
`created_at`). No offset paging.

### Interfaces

```go
// internal/identity/execute.go
// Election is performed by a SINGLE SQL statement (ElectPrimary) so the demote-old +
// set-new pair is atomic without an explicit tx handle in the writer.
type IdentifierStore interface {
    ElectPrimary(ctx context.Context, arg postgres.ElectPrimaryParams) (postgres.EntityIdentifier, error)
    GetEntityIdentifiers(ctx context.Context, arg postgres.GetEntityIdentifiersParams) ([]postgres.EntityIdentifier, error)
}

// SQLc query signatures to add in internal/storage/postgres/queries/identity.sql:
//   -- name: ElectPrimary :one
//   --   WITH demote AS (
//   --     UPDATE entity_identifiers SET is_primary = false
//   --      WHERE entity_type=$1 AND entity_id=$2 AND authority_code=$3 AND is_primary
//   --      RETURNING id)
//   --   INSERT INTO entity_identifiers
//   --     (entity_type, entity_id, authority_code, identifier_uri, confidence,
//   --      reconciliation_method, is_canonical, is_primary, observed_at, source, metadata)
//   --   VALUES ($1,$2,$3,$4,$5,$6,false,true,now(),$7,$8)
//   --   ON CONFLICT (entity_type, entity_id, authority_code, identifier_uri)
//   --   DO UPDATE SET confidence=EXCLUDED.confidence,
//   --                 reconciliation_method=EXCLUDED.reconciliation_method,
//   --                 is_primary=true, observed_at=now(), source=EXCLUDED.source,
//   --                 metadata=EXCLUDED.metadata, updated_at=now()
//   --   RETURNING *;
// (is_canonical is written false and kept only for the old container's column contract.)

type NotDuplicateStore interface {
    InsertNotDuplicate(ctx context.Context, arg postgres.InsertIdentityNotDuplicateParams) error
    IsNotDuplicate(ctx context.Context, arg postgres.IsIdentityNotDuplicateParams) (bool, error)
}

type DecisionStore interface {
    Append(ctx context.Context, rec DecisionRecord) (string, error)
    List(ctx context.Context, ref IdentityRef) ([]DecisionRecord, error)
    ListFeed(ctx context.Context, arg postgres.ListIdentityDecisionsParams) ([]DecisionRecord, error)
}

// Executor is a concrete struct.
type Executor struct { /* ids, store, notDup, decisions, clock */ }
func (e *Executor) LinkIdentifier(ctx context.Context, ref IdentityRef, obs IdentifierObservation, actor string) (DecisionRecord, error)
func (e *Executor) Reject(ctx context.Context, a, b IdentityRef, actor, reason string, fingerprint string) (DecisionRecord, error)
```

`LinkIdentifier` semantics (disambiguated): record/confirm an **external identifier
observation** for one SEL entity, then elect the primary. It is *not* an entity-to-entity
operation. Entity-to-entity sameness is expressed by `Reject` (distinct) or, in Phase 2,
by merge.

### REST / CLI Schemas

**GET `/api/v1/admin/identity/{type}/{id}`** → `200`: `IdentityView` (struct above).

**GET `/api/v1/admin/identity/conflicts?type=place&limit=50&cursor=`** → `200`:
`ConflictsResponse`:
```json
{
  "items": [
    { "ref": { "entity_type": "place", "entity_id": "01...A" },
      "candidate": { "entity_type": "place", "entity_id": "01...B" },
      "authority": "artsdata", "uri": "https://kg.artsdata.ca/resource/K11-24",
      "score": 0.99, "suppressed": false }
  ],
  "next_cursor": null
}
```

**GET `/api/v1/admin/identity/decisions?type=place&since=2026-09-01T00:00:00Z&limit=100&cursor=`**
→ `200`: `DecisionsResponse`, newest-first.

**POST `/api/v1/admin/identity/link`** body → `201` `DecisionRecord`:
```json
{ "entity_type": "place", "entity_id": "01...",
  "authority": "artsdata", "uri": "https://kg.artsdata.ca/resource/K11-24",
  "source": "manual", "rationale": "confirmed via venue website" }
```

**POST `/api/v1/admin/identity/reject`** body → `201` `DecisionRecord`:
```json
{ "entity_type": "place", "entity_id": "01...A", "counterpart_id": "01...B",
  "reason": "different address/operator" }
```

`actor` is **not** client-supplied: the server sets it from the admin JWT subject
(`middleware.AdminClaims(r).Subject`), preventing attribution spoofing.

**CLI** (mirrors `server review`; auth via `--key`/`--token`/`--server`, STS exchange):
```
server identity check   place <ulid> [--json]
server identity conflicts --type place [--limit 50] [--json]
server identity link    place <ulid> --authority artsdata --uri <uri> --source manual [--json]
server identity reject  place <ulid> --other <ulid> --reason <text> [--json]
server identity tidy    [--dry-run|--apply] [--type place|organization]
```

### Migration `000051` (up + down)

```sql
-- UP
ALTER TABLE entity_identifiers
  ADD COLUMN observed_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  ADD COLUMN is_primary       BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN superseded_by_id INTEGER REFERENCES entity_identifiers(id),
  ADD COLUMN source           TEXT;   -- nullable for legacy rows

-- Broaden authority URI patterns scheme-insensitively (artsdata/wikidata/isni were http-only
-- or mismatched vs emitted https URIs). Records are matched on 000030 regexes today.
UPDATE knowledge_graph_authorities
   SET base_uri_pattern = replace(base_uri_pattern, '^http://', '^https?://')
 WHERE base_uri_pattern LIKE '^http://%';
-- (If a pattern already uses ^https?:// it is unchanged.)

-- Backfill: elect one primary per (entity_type, entity_id, authority_code).
UPDATE entity_identifiers SET is_primary = false;
WITH ranked AS (
  SELECT id, ROW_NUMBER() OVER (
    PARTITION BY entity_type, entity_id, authority_code
    ORDER BY CASE reconciliation_method
               WHEN 'manual' THEN 5 WHEN 'imported' THEN 4
               WHEN 'auto_high' THEN 3 WHEN 'auto_low' THEN 2
               WHEN 'enrichment_sameas' THEN 1 ELSE 0 END DESC,
             confidence DESC, updated_at DESC, id DESC) AS rn
  FROM entity_identifiers)
UPDATE entity_identifiers ei SET is_primary = true FROM ranked r
 WHERE ei.id = r.id AND r.rn = 1;

CREATE UNIQUE INDEX idx_entity_identifiers_one_primary
  ON entity_identifiers(entity_type, entity_id, authority_code) WHERE is_primary;

CREATE TABLE identity_decisions (
  id TEXT PRIMARY KEY, created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  entity_type TEXT NOT NULL CHECK (entity_type IN ('place','organization')),
  entity_id TEXT NOT NULL,
  action TEXT NOT NULL CHECK (action IN ('link','reject')),   -- widen in Phase 2
  counterpart_type TEXT, counterpart_id TEXT,
  rationale TEXT NOT NULL DEFAULT '', citations JSONB NOT NULL DEFAULT '[]'::jsonb,
  confidence NUMERIC(5,4) NOT NULL DEFAULT 0, actor TEXT NOT NULL,
  reversible BOOLEAN NOT NULL DEFAULT true, undo_ref TEXT, metadata JSONB
);
CREATE INDEX idx_identity_decisions_entity ON identity_decisions(entity_type, entity_id, created_at DESC, id DESC);
CREATE INDEX idx_identity_decisions_feed   ON identity_decisions(created_at DESC, id DESC);

-- Signal-scoped suppression: retained indefinitely unless the evidence fingerprint changes.
CREATE TABLE identity_not_duplicates (
  entity_type TEXT NOT NULL CHECK (entity_type IN ('place','organization')),
  id_a TEXT NOT NULL, id_b TEXT NOT NULL,
  evidence_fingerprint TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), created_by TEXT NOT NULL,
  PRIMARY KEY (entity_type, id_a, id_b), CHECK (id_a < id_b)
);
CREATE INDEX idx_identity_not_duplicates_b ON identity_not_duplicates(entity_type, id_b);

CREATE INDEX idx_places_merged_into ON places(merged_into_id);
CREATE INDEX idx_organizations_merged_into ON organizations(merged_into_id);

-- DOWN (reverse; is_canonical is never dropped)
DROP INDEX IF EXISTS idx_organizations_merged_into;
DROP INDEX IF EXISTS idx_places_merged_into;
DROP INDEX IF EXISTS idx_identity_not_duplicates_b;
DROP TABLE IF EXISTS identity_not_duplicates;
DROP INDEX IF EXISTS idx_identity_decisions_feed;
DROP INDEX IF EXISTS idx_identity_decisions_entity;
DROP TABLE IF EXISTS identity_decisions;
DROP INDEX IF EXISTS idx_entity_identifiers_one_primary;
ALTER TABLE entity_identifiers
  DROP COLUMN IF EXISTS source,
  DROP COLUMN IF EXISTS superseded_by_id,
  DROP COLUMN IF EXISTS is_primary,
  DROP COLUMN IF EXISTS observed_at;
-- (Authority pattern broadening is data, not schema; do not revert on down.)
```

**Blue/green note**: `is_canonical` is written `false` and kept (not dropped) because
`deploy/scripts/deploy.sh` runs migrations while the **old** container still serves and still
`INSERT`s that column. It is dropped in a later release once no deployed container references
it. During the overlap the old container writes rows with `is_primary=false`; run
`server identity tidy --apply` after cutover to fill any missing primaries.

**Append-only**: `identity_decisions` is append-only by convention (no UPDATE/DELETE in
application code). This phase does not add a DB trigger to enforce it.

### Error Handling

- **Structural** (bad entity type, invalid ULID, URI failing the authority regex, empty
  actor) → domain error of BadRequest class; surfaced by the API/CLI as `400` RFC 7807
  (`ValidateAndExtractULID`, `internal/api/handlers/validators.go:25`, returns 400).
- **Not found** (valid ULID, absent entity) → `404`, `type` `.../not-found`.
- **Semantic/policy** outcomes (escalation) do not exist in Phase 1; they arrive with the
  Phase 2 validator.
- All errors wrapped (`fmt.Errorf("identity: %w", err)`). A primary-index unique violation
  is translated to a domain `ErrPrimaryConflict`, never surfaced raw.

### Security Model

- **Authorization**: all identity endpoints/CLI verbs are admin-role (JWT), matching
  `/api/v1/admin/*`. No public/MCP exposure.
- **Write surface in Phase 1**: admin-JWT `POST .../identity/link` and `.../identity/reject`
  (plus CLI verbs). No merge/undo writes.
- **Untrusted input**: identifier URIs validated scheme-insensitively against
  `knowledge_graph_authorities.base_uri_pattern` **before** storage; failure is structural and
  the observation is rejected (logged, no row written). API responses echo stored values only.
- **Data lifecycle**: `identity_decisions` and `identity_not_duplicates` are standard durable
  Postgres tables covered by the existing deploy DB snapshot/backup path
  (`deploy/scripts/deploy.sh` pre-migration snapshot). `identity_decisions` is never pruned
  (append-only audit); `identity_not_duplicates` is retained indefinitely and re-opened only
  on evidence-fingerprint change (see PQ2). No new file/data-directory storage.

---

## Implementation Tasks

### Task 1: Migration `000051` (primary slot, patterns, decision/suppression stores, indexes)

**What**: Add `observed_at`/`is_primary`/`superseded_by_id`/`source`; broaden authority URI
patterns; backfill one primary per group; create the partial unique index; create
`identity_decisions` and `identity_not_duplicates` (with `evidence_fingerprint`) and indexes;
add merge indexes; include a working **down** migration. Regenerate SQLc (`make sqlc`).
**Test**: `make migrate-up` then `make migrate-down` then `make migrate-up` on a scratch DB;
invariant query returns 0 rows; a direct second-primary insert fails.
**Acceptance**: up/down/up succeed; `SELECT entity_type, entity_id, authority_code, count(*)
FROM entity_identifiers WHERE is_primary GROUP BY 1,2,3 HAVING count(*)>1` returns 0 rows;
`make sqlc` produces no diff after commit.

### Task 2: Atomic primary election + writer integration

**What**: Add `internal/identity/rank.go` (`ElectPrimary`); add the `ElectPrimary` SQLc query
and `IdentifierStore` interface; update `internal/kg/reconciliation.go` `storeIdentifier` and
`internal/jobs/workers.go` sameAs write to call `ElectPrimary` (single statement,
`observed_at=now()`, `source` set, `is_canonical=false`); remove `IsCanonical` from writer
params. Update `UpsertEntityIdentifier` SQL/params and regenerate.
**Test**: unit tests for `ElectPrimary` ordering (method → trust/priority → confidence →
recency, with explicit ties); integration test that a second write with a higher method rank
demotes the first and sets `superseded_by_id`.
**Acceptance**: after two reconciles with different top matches, exactly one `is_primary` row
exists and the higher-ranked one wins; prior row retained and superseded; `observed_at`
refreshed.

### Task 3: Transactional merge integrity (identifiers, tombstones)

**What**: Make `MergePlaces` and `MergeOrganizations` (`events_repository.go:1717`, `:1839`)
run in a single transaction. Resolve the duplicate/primary **ULIDs** (the methods currently
receive internal UUIDs, while `entity_identifiers.entity_id` stores ULIDs) and reassign/
dedupe identifiers onto the survivor, re-elect one primary, write a
`place_tombstones`/`organization_tombstones` row (URI + payload supplied from the domain
service, reusing the existing tombstone-payload construction), and set `deletion_reason`.
Fix `MergeEvents` (`queries/events.sql:78-85`) to set `deletion_reason`. (`deletion_reason`
is already set for place/org merges — do not duplicate.)
**Test**: integration tests for the US2 scenarios, including an injected mid-merge failure
asserting rollback, and a shared-identifier dedup case.
**Acceptance**: 0 `entity_identifiers` rows reference a soft-deleted place/org after any merge
path; tombstone present with non-null URI/payload; failure rolls back cleanly.

### Task 4: LinkIdentifier / Reject executor + decision store

**What**: Implement `internal/identity/execute.go` (`LinkIdentifier`, `Reject`) and
`internal/identity/record.go` (DecisionStore over `identity_decisions`), plus
`identity_not_duplicates` read/write with an **evidence fingerprint** (hash of the
participating observations at decision time). Validate URIs via the authority patterns;
require non-empty actor.
**Test**: unit + integration for LinkIdentifier (upsert + election + decision), Reject
(not-duplicate row with fingerprint + decision), empty-actor structural error, and
fingerprint re-open (different evidence ⇒ `IsNotDuplicate` false).
**Acceptance**: each call appends exactly one decision record; Reject suppresses identical
evidence indefinitely and permits resurface on changed evidence.

### Task 5: Read-only admin REST + OpenAPI + lint manifest

**What**: Add `GET /api/v1/admin/identity/{type}/{id}`,
`.../identity/conflicts`, `.../identity/decisions`, and `POST .../identity/link`,
`.../identity/reject` (the latter two call the executor; `actor` taken from the JWT subject)
via `internal/api/handlers/identity.go`, wired in `internal/api/router.go` (`jwtAuth` +
`AdminRequestSize`). Update `docs/api/openapi.yaml` (OAS 3.1; include env-var name/default for
`IDENTITY_CONFLICT_LIMIT_MAX` in the endpoint `description`). Add `IDENTITY_CONFLICT_LIMIT_MAX`
to the `internal/config/openapi_lint_test.go` manifest.
**Test**: handler tests with a stub repo; cursor round-trip test; `make lint-openapi`;
contract test for 400 (bad ULID) and 404 (absent) RFC 7807.
**Acceptance**: response shapes match the structs above; feed is newest-first and honours
`type`/`since`/`limit`/`cursor`; openapi lint passes with the manifest entry; 401 without JWT.

### Task 6: `server identity` CLI (check | conflicts | link | reject)

**What**: Add `cmd/server/cmd/identity.go`. `check`/`conflicts` call the read endpoints;
`link`/`reject` call the admin write endpoints (Task 5). Auth mirrors `server review`
(`--key`/`--token`/`--server`, STS exchange).
**Test**: CLI unit tests against an httptest server for reads; executor-backed tests for
`link`/`reject` with a temp DB.
**Acceptance**: `check`/`conflicts` match the REST output; `link`/`reject` append decision
records and are reversible-by-superseding-observation; `--json` matches the schemas.

### Task 7: `server identity tidy` repair (folds `t_e522e110`)

**What**: Add `tidy [--dry-run|--apply] [--type ...]` that (a) fills a primary where a group
has observations but none is primary, and (b) demotes extras where a group has >1 primary
(legacy/pre-index state). Reports entities scanned / primaries filled / rows demoted.
`--dry-run` mutates nothing.
**Test**: fixtures — a "no primary" group and a legacy multi-primary group created **after
temporarily dropping the partial index** (this is the only way a post-000051 schema can hold
multi-primary; the test documents this). Dry-run purity; apply idempotence.
**Acceptance**: after `--apply`, the invariant query returns 0 rows and every group with
observations has a primary; dry-run leaves the DB unchanged; a second `--apply` reports 0.

---

## Configuration

New typed fields in `internal/config/config.go` (`getEnv*` defaults):

| Env var | Default | Purpose |
|---|---|---|
| `IDENTITY_CONFLICT_LIMIT_MAX` | `200` | Max `limit` accepted by conflicts/decisions read endpoints |
| `IDENTITY_OBSERVATION_TTL_DAYS` | `180` | Re-verification staleness (used Phase 3) |

Threshold/fan-out config lands in Phase 2 with actions. Any env-controlled endpoint
behaviour must name the env var + default in the OpenAPI `description`
(`internal/config/openapi_lint_test.go` enforces the manifest).

## Success Criteria

- **0 rows** for the primary-slot invariant query on staging after deploy + `identity tidy
  --apply` (tidy also fills rows written during the blue/green overlap window).
- **0 orphaned** `entity_identifiers` rows for **places/orgs** after the merge integration
  tests and after any production place/org merge path. (Events/consolidate remain on the
  existing path — out of scope, see Non-Goals.)
- Identity view API and `server identity check` return identical data for the same ULID.
- `make ci-fast` green; new unit + integration tests pass; `make lint-openapi` passes.
- No regression in `server reconcile stats` matched/enriched counts on staging.

## Open Questions

- ~~PQ1~~ **Resolved (2026-09-10): `Link` reversal is a superseding observation, not row
  deletion.**
- ~~PQ2~~ **Resolved (2026-09-10): signal-scoped suppression.** `identity_not_duplicates` is
  retained indefinitely and re-opened only when the recorded **evidence fingerprint** changes.
  Unlike events (which age out, so time-based safety is implicit), places/orgs are persistent,
  so suppression keys on evidence, not a clock. No cleanup job.
- PQ3 — deferred to Phase 2: whether `identity_decisions` needs additional precedent-projection
  (e.g. embedding-free "similar past decision" lookup) beyond the filterable feed.
