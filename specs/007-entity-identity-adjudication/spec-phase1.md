# Phase 1 Specification: Identity Primitives, Repair & Read/Write Loop

**Spec**: 007-entity-identity-adjudication / Phase 1 | **Date**: 2026-09-10 | **Status**: Draft
**Parent**: `specs/007-entity-identity-adjudication/plan.md`
**Goal**: The node stores at most one **primary** external identifier per
`(entity, authority)`, enforced by a DB index; every place/organization merge preserves
identity (no orphaned `entity_identifiers`); and an external agent can **read** identity
state and **record link/reject decisions** for a place/org via the admin REST API and
`server identity` CLI. Target: **0 rows** violating the primary invariant and **0 orphaned
identifiers** for places/orgs after a merge, verified on staging.

> The canonical primary-election order is defined once in `plan.md` § Component Design.
> Phase 1 is deterministic. Queue-driven merge, policy validation, undo, and batch are
> Phase 2.

---

## Context

### What Exists Today

| Component | Status | Code reference |
|---|---|---|
| `entity_identifiers` (sameAs edges) | Write-only; `is_canonical` per-row predicate | `internal/kg/reconciliation.go:330,332`, `internal/jobs/workers.go:422` |
| Writers | reconciliation + enrichment | `internal/kg/reconciliation.go:239-275`, `internal/jobs/workers.go:417-441` |
| Readers | generated SQLc methods exist but **uncalled**; only CLI raw SQL | `cmd/server/cmd/reconcile.go:307,443,571` |
| Primary/unique constraint | `UNIQUE(entity_type, entity_id, authority_code, identifier_uri)` (permits multi-canonical) | `migrations/000030_knowledge_graph_tables.up.sql:36` |
| Authority URI patterns | seeded; **artsdata/wikidata patterns are `http://`-only** while code emits `https://` | `000030:63-64`; `internal/kg/reconciliation.go:22,105` |
| `knowledge_graph_authorities` (trust/priority) | Seeded, unused by logic | `000030:62-67` |
| Place merge | reassigns events/occurrences; sets `deletion_reason='merged'`; **does not touch identifiers**; no tombstone; **not transactional**; receives UUIDs | `internal/storage/postgres/events_repository.go:1717`, `internal/domain/events/admin_service.go:2583` |
| Org merge | reassigns events; sets `deletion_reason='merged'`; **does not touch identifiers**; no tombstone; **not transactional**; receives UUIDs | `events_repository.go:1839`, `admin_service.go:2595` |
| Event merge | omits `deletion_reason` | `queries/events.sql:78-85` |
| Tombstone builders | unexported handler funcs; take URI+payload | `internal/api/handlers/admin.go:471`, `:560` |
| ULID validation helper | always returns **400** | `internal/api/handlers/validators.go:25` |
| Admin JWT claims | `AdminClaims(r).Subject` available | `internal/api/middleware/auth_cookie.go:86`, `internal/auth/jwt.go:11-14` |
| Review CLI pattern | Live agent contract to mirror | `docs/integration/tg-review.md` |

**Observed drift (staging, 2026-09-10 — runtime observation, not repo-verifiable):**
172 `is_canonical=true` artsdata rows across only 144 places (~28 places with >1 canonical).

### What This Phase Delivers

1. Migration `000051`: primary-slot columns + partial unique index + backfill; broaden
   authority URI patterns; append-only `identity_decisions`; signal-scoped
   `identity_not_duplicates`; missing merge indexes; **down migration**.
2. **Atomic primary election** (transaction + Go rank) integrated into reconciliation/
   enrichment writes; refresh `observed_at`; write `superseded_by_id`; never set
   `is_canonical=true` (write `false` only to satisfy the old-container column contract).
3. **Merge integrity**: `MergePlaces`/`MergeOrganizations` made **transactional**, reassign +
   dedupe identifiers (ULID-resolved), write tombstones, re-elect a primary.
4. `LinkIdentifier` / `Reject` executor + decision store, exposed via **admin REST
   (`POST` link/reject)** and CLI verbs.
5. Admin REST read views (identity view + conflicts + decisions feed); OpenAPI + lint manifest.
6. `server identity check|conflicts|link|reject` CLI.
7. `server identity tidy --dry-run|--apply` repair (folds `t_e522e110`).

### Non-Goals (deferred)

- No candidate **queue**, no `merge`/`undo`, no policy validator, no batch → **Phase 2**
  (link/reject actions are in Phase 1).
- No LLM, no prompts, no MCP tools.
- No external KG **candidate generation** or `sameAs` emission → **Phase 3**.
- No events/persons identity → **Phase 4** (events keep `event_not_duplicates`, safe because
  events age out).
- No vector/semantic matching.

### Design Constraint Reminders

Carried from the plan: SEL ULID is canonical, external URIs are aliases; exactly one primary
per `(entity, authority)`; no cross-authority transitivity; destructive actions reversible +
tombstoned; evidence ≠ action; keep-all identifiers; config in `internal/config/config.go`;
RFC 7807; SQLc; migrations via `make`; canonical rank defined once in `plan.md`; no vector.

---

## User Scenarios & Testing

### User Story 1 — One primary identifier per authority (Priority: P0)

**Independent Test**: Insert identifier observations for one place+authority and assert the
DB rejects a second primary and the Go rank elects the correct winner.

**Acceptance Scenarios**
1. Given observations `auto_high` (0.99) and `auto_low` (0.90) for one place+authority, when
   election runs, then exactly one row is `is_primary=true` and it is the `auto_high` row.
2. Given current primary `K11-24` at method `auto_low`, when `K11-99` at method `auto_high` is
   recorded, then `K11-99` becomes primary, `K11-24` is demoted with
   `superseded_by_id = K11-99.id`, and both rows are retained. (Methods differ ⇒ determinate.)
3. Given a group, when a second `is_primary=true` row is inserted directly via SQL, then the
   partial unique index raises a unique violation.

### User Story 2 — Merge preserves identity (Priority: P0)

**Independent Test**: Two places with distinct identifiers; merge A into B; assert B holds
both, A is tombstoned, and no identifier references A.

**Acceptance Scenarios**
1. Given A (`K11-24`) and B (`K11-25`), when A is merged into B, then B's set contains both,
   exactly one primary, and A has `merged_into_id=B`, `deleted_at` set, `deletion_reason='merged'`.
2. Given the same merge, then a `place_tombstones` row exists for A with non-null `place_uri`
   and `payload`, and `SELECT count(*) FROM entity_identifiers WHERE entity_id=A` is 0.
3. Given an injected failure after identifier reassignment but before commit, then the
   transaction rolls back: identifiers still belong to A and A is not soft-deleted.
4. Given a merge where both sides assert the same authority+URI, then the union is deduplicated
   and one primary remains.

### User Story 3 — Inspect identity via API and CLI (Priority: P0)

**Independent Test**: `GET /api/v1/admin/identity/place/{ulid}` and
`server identity check place {ulid}` return identical identifiers, primary map, and decisions.

**Acceptance Scenarios**
1. Given a reconciled place, when fetched, then identifiers list authority/method/confidence/
   `is_primary`/`source`/`observed_at` and a `primary` map keyed by authority.
2. Given prior decisions, when fetched, then `decisions` is ordered `created_at DESC, id DESC`.
3. Given an invalid ULID, then `400`; given a valid but absent ULID, then `404`; both RFC 7807.

### User Story 4 — Record link / reject decisions (Priority: P1)

**Independent Test**: `POST /api/v1/admin/identity/link` then `.../reject`; assert one
`identity_decisions` row per call and one `identity_not_duplicates` row.

**Acceptance Scenarios**
1. Given a validated external URI, when `LinkIdentifier` executes, then an
   `entity_identifiers` row is upserted (`source`, `observed_at=now()`, `is_primary=false`),
   election runs atomically, and a decision record is appended (`reversible=true`).
2. Given two entities judged distinct, when `Reject` executes, then an
   `identity_not_duplicates` row is recorded (canonical ordering `id_a < id_b`) with the
   **evidence fingerprint**, and the pair leaves the default conflicts list.
3. Given an empty JWT subject, then the request is rejected structurally (401/400) and no
   decision is written.
4. Given a `Reject` pair, when the candidate generator later sees materially new evidence
   (fingerprint differs), then the pair may resurface with the prior decision attached;
   identical evidence keeps it suppressed indefinitely.

### User Story 5 — Repair existing drift (Priority: P1)

**Independent Test**: With the partial index temporarily dropped, seed a group with two
primaries and a group with observations but no primary; run `tidy --dry-run` then `--apply`.

**Acceptance Scenarios**
1. Given drift, when `tidy --dry-run` runs, then it prints affected entities and mutates nothing.
2. Given drift, when `tidy --apply` runs, then the invariant holds and a summary reports
   entities scanned / primaries filled / rows demoted.
3. Given no drift, when `tidy --apply` runs, then it reports 0 changes and exits 0.

---

## Technical Design

### Package Layout

```
internal/identity/
  ref.go            # EntityType, IdentityRef
  rank.go           # IdentifierObservation, ElectPrimary (Go rank)
  store.go          # IdentifierStore, RecordObservation (tx + election)
  execute.go        # Executor: LinkIdentifier, Reject
  record.go         # DecisionRecord, DecisionStore
  service.go        # IdentityView / ConflictItem / DecisionFeedItem assembly
  fingerprint.go    # evidence fingerprint for identity_not_duplicates
internal/storage/postgres/queries/identity.sql        # SQLc: observation upsert, group load, elect/demote, views, decisions
internal/storage/postgres/migrations/000051_entity_identity_primary.{up,down}.sql
internal/api/handlers/identity.go                     # REST read + link/reject
cmd/server/cmd/identity.go                            # check | conflicts | link | reject | tidy
internal/domain/tombstones/                           # NEW home for tombstone payload builders (moved from handler)
internal/kg/reconciliation.go                         # writers call RecordObservation
internal/jobs/workers.go                              # enrichment writer calls RecordObservation
internal/storage/postgres/events_repository.go        # transactional merge + identifier reassignment + tombstone
internal/config/openapi_lint_test.go                  # add IDENTITY_CONFLICT_LIMIT_MAX
docs/api/openapi.yaml                                 # new endpoints
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
    TrustLevel int32   // knowledge_graph_authorities.trust_level
    Priority   int32   // knowledge_graph_authorities.priority_order
    IsPrimary  bool
    Source     string  // reconciliation|enrichment_sameas|manual|agent
}

// Canonical order (plan.md): method > authority trust/priority > confidence > observed_at > id.
func ElectPrimary(obs []IdentifierObservation) (IdentifierObservation, bool)

// internal/identity/record.go — flat, mirrors identity_decisions columns and JSON.
type DecisionRecord struct {
    ID              string         `json:"id"`           // "idn-{ulid}"
    CreatedAt       time.Time      `json:"created_at"`   // identity_decisions.created_at
    EntityType      EntityType     `json:"entity_type"`
    EntityID        string         `json:"entity_id"`
    Action          string         `json:"action"`       // link|reject
    CounterpartType *EntityType    `json:"counterpart_type,omitempty"`
    CounterpartID   *string        `json:"counterpart_id,omitempty"`
    Rationale       string         `json:"rationale"`
    Citations       []string       `json:"citations"`
    Confidence      float64        `json:"confidence"`
    Actor           string         `json:"actor"`
    Reversible      bool           `json:"reversible"`
    UndoRef         string         `json:"undo_ref,omitempty"`
    Metadata        map[string]any `json:"metadata,omitempty"`
}

// internal/identity/service.go — REST response types.
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
    Ref           IdentityRef     `json:"ref"`
    Candidate     IdentityRef     `json:"candidate"`
    Authority     string          `json:"authority"`
    URI           string          `json:"uri"`
    Score         float64         `json:"score"`          // max confidence of the two observations
    Suppressed    bool            `json:"suppressed"`
    PriorDecision *DecisionRecord `json:"prior_decision,omitempty"` // set when suppressed
}

type ConflictsResponse struct {
    Items      []ConflictItem `json:"items"`
    NextCursor *string        `json:"next_cursor"` // opaque (authority, uri, entity_id of last item)
}

type DecisionsResponse struct {
    Items      []DecisionRecord `json:"items"`
    NextCursor *string          `json:"next_cursor"` // opaque (created_at, id of last row)
}
```

**Cursor semantics**: opaque base64 keyset cursors. Conflicts order
`(authority_code, identifier_uri, entity_id_a, entity_id_b)` (cursor encodes all four);
decisions order `(created_at DESC, id DESC)` with `since` an inclusive RFC3339 lower bound on
`created_at`. No offset paging.

**Evidence fingerprint** (`internal/identity/fingerprint.go`) is **signal-scoped**: hex
SHA-256 of the sorted, newline-joined `authority|uri` values asserted for *both* entities
(the shared identifier signals) at decision time, keyed by the canonical pair. Identifiers
held by only one entity do not affect it, so an unrelated new identifier does not re-open the
pair. The same function computes the *current* fingerprint on read.

### Interfaces

```go
// internal/identity/store.go
// RecordObservation performs the whole election in ONE transaction (no partial writes):
//   1. pg_advisory_xact_lock(hash(entity_type|entity_id|authority))   -- serialize per group
//   2. UpsertObservation(..., observed_at=now(), source=obs.Source)
//      -- INSERT sets is_primary=false; ON CONFLICT DO UPDATE does NOT touch is_primary
//      and preserves metadata via COALESCE(EXCLUDED.metadata, existing.metadata)
//   3. ListGroupIdentifiersForUpdate(...)  -- group load joining knowledge_graph_authorities
//      for trust/priority, with FOR UPDATE OF ei (READ-COMMITTED reread after the lock)
//   4. winner, ok := ElectPrimary(obs)          -- Go rank
//   5. DemotePrimaryAndSupersede(group, winner.ID)  -- demotes every primary except winner,
//      setting superseded_by_id=winner.ID (immediate successor, not necessarily the
//      eventual primary)
//   6. SetPrimary(winner.ID)                    -- UNCONDITIONAL; guarantees exactly one primary
// Steps 5-6 run even when the winner is unchanged (no-ops then), so a re-observed current
// winner cannot be left non-primary.
// The transaction must be READ COMMITTED (Postgres default): the group SELECT issued
// after the advisory lock must observe rows committed by earlier serialized writers.
type TxManager interface {
    WithTx(ctx context.Context, fn func(q *postgres.Queries) error) error
}

type IdentifierStore interface {
    // Own-transaction convenience for the reconciliation/enrichment writers.
    RecordObservation(ctx context.Context, ref IdentityRef, obs IdentifierObservation) (IdentifierObservation, error)
    // Tx-scoped form: caller supplies the tx so election + decision append commit atomically.
    RecordObservationTx(ctx context.Context, q *postgres.Queries, ref IdentityRef, obs IdentifierObservation) (IdentifierObservation, error)
    GetEntityIdentifiers(ctx context.Context, ref IdentityRef) ([]IdentifierObservation, error)
}

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
type Executor struct { /* ids, store, notDup, decisions, fp, clock */ }
func (e *Executor) LinkIdentifier(ctx context.Context, ref IdentityRef, obs IdentifierObservation, actor string) (DecisionRecord, error)
func (e *Executor) Reject(ctx context.Context, a, b IdentityRef, actor, reason string) (DecisionRecord, error)
```

`LinkIdentifier` semantics (disambiguated): record/confirm an **external identifier
observation** for one SEL entity, then elect the primary. It is *not* entity-to-entity.
Entity-to-entity sameness is expressed by `Reject` (distinct) or, in Phase 2, by merge.

**SQLc queries to add** (`internal/storage/postgres/queries/identity.sql`):
- `UpsertObservation :one` — added in Task 1; **replaces** the deleted `UpsertEntityIdentifier`
  query. INSERT sets `is_primary=false`; `ON CONFLICT DO UPDATE` refreshes `confidence`,
  `reconciliation_method`, `observed_at=now()`, `source`, `updated_at` and does **not** touch
  `is_primary`; `metadata` is preserved non-destructively via
  `COALESCE(EXCLUDED.metadata, entity_identifiers.metadata)` (a NULL incoming metadata keeps the
  existing provenance JSONB); `is_canonical` written `false`. Does **not** elect.
- `LockIdentityGroup :exec` — `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
  keyed by `entity_type|entity_id|authority_code`; serializes concurrent elections per group.
- `ListGroupIdentifiersForUpdate :many` — group load (one authority) joining
  `knowledge_graph_authorities` (via `a.authority_code = ei.authority_code`) selecting
  `a.trust_level, a.priority_order`, with `FOR UPDATE OF ei`. The election's locking read.
- `GetEntityIdentifiers :many` — existing query extended to join
  `knowledge_graph_authorities` for `trust_level`/`priority_order`; the **non-locking**
  read path (no `FOR UPDATE`) used by `IdentifierStore.GetEntityIdentifiers`.
- `DemotePrimaryAndSupersede :exec` — `UPDATE entity_identifiers SET is_primary=false,
  superseded_by_id=$4, updated_at=now() WHERE entity_type=$1 AND entity_id=$2 AND
  authority_code=$3 AND is_primary AND id<>$4`. `superseded_by_id` records the immediate
  successor at demotion time, not necessarily the eventual primary.
- `SetPrimary :exec` — `UPDATE entity_identifiers SET is_primary=true, superseded_by_id=NULL,
  updated_at=now() WHERE id=$1`.
- `ListConflicts :many` — `entity_identifiers a JOIN entity_identifiers b ON
  a.entity_type=b.entity_type AND a.authority_code=b.authority_code AND
  a.identifier_uri=b.identifier_uri AND a.entity_id<b.entity_id`, `LEFT JOIN
  identity_not_duplicates nd` (matched on the pair's canonical ordered ids with
  `LEAST`/`GREATEST`), filtered by `a.entity_type=$1`, keyset on `(authority_code,
  identifier_uri, a.entity_id, b.entity_id)`; each unordered pair once, with the stored
  `evidence_fingerprint` (nullable).
- `InsertNotDuplicate :exec` — `INSERT ... ON CONFLICT (entity_type,id_a,id_b) DO UPDATE SET
  evidence_fingerprint=EXCLUDED.evidence_fingerprint, decision_id=EXCLUDED.decision_id,
  created_at=now(), created_by=EXCLUDED.created_by`.
- `ListIdentityDecisions :many` — keyset-paginated feed.

### REST / CLI Schemas

**GET `/api/v1/admin/identity/{type}/{id}`** → `200`: `IdentityView`.

**GET `/api/v1/admin/identity/conflicts?type=place&limit=50&cursor=&include_suppressed=false`**
→ `200`: `ConflictsResponse`. **Phase 1 conflict source** = a self-join of
`entity_identifiers` on equal `(authority_code, identifier_uri)` with `entity_id_a <
entity_id_b`, returning each unordered pair once. `Suppressed` is computed in Go: the service
loads the stored `evidence_fingerprint` for the pair and compares it to the current
fingerprint (same `fingerprint.go` function). Suppressed pairs are **excluded by default**;
`include_suppressed=true` includes them with `prior_decision` (joined via
`identity_not_duplicates.decision_id`).
```json
{ "items": [
    { "ref": {"entity_type":"place","entity_id":"01...A"},
      "candidate": {"entity_type":"place","entity_id":"01...B"},
      "authority":"artsdata","uri":"https://kg.artsdata.ca/resource/K11-24",
      "score":0.99,"suppressed":false } ],
  "next_cursor": null }
```

**GET `/api/v1/admin/identity/decisions?type=place&since=2026-09-01T00:00:00Z&limit=100&cursor=`**
→ `200`: `DecisionsResponse`, newest-first.

**POST `/api/v1/admin/identity/link`** → `201` `DecisionRecord`:
```json
{ "entity_type":"place","entity_id":"01...",
  "authority":"artsdata","uri":"https://kg.artsdata.ca/resource/K11-24",
  "method":"manual","confidence":1.0,"source":"manual",
  "rationale":"confirmed via venue website" }
```
Server defaults omitted `method=manual`, `confidence=1.0`, `source=manual`; validates `uri`
against the authority pattern. Client-supplied `method`/`source` are accepted only from the
Phase 1 allow-list (`manual`) and otherwise normalised to `manual` — an agent cannot assert
`auto_high`/`auto_low`; `confidence` is clamped to `[0,1]` and never affects the method rank.
`actor` is taken from the JWT subject, never the body.

**POST `/api/v1/admin/identity/reject`** → `201` `DecisionRecord`:
```json
{ "entity_type":"place","entity_id":"01...A","counterpart_id":"01...B",
  "reason":"different address/operator" }
```
The server computes the **evidence fingerprint** from current observations; it is not
client-supplied.

**CLI** (mirrors `server review`; STS exchange via `--key`/`--server`):
```
server identity check   place <ulid> [--json]
server identity conflicts --type place [--limit 50] [--include-suppressed] [--json]
server identity link    place <ulid> --authority artsdata --uri <uri> [--source manual] [--json]
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

-- Broaden http://-anchored authority patterns to https?:// (artsdata, wikidata);
-- isni/musicbrainz/osm are already https-anchored and unchanged.
UPDATE knowledge_graph_authorities
   SET base_uri_pattern = replace(base_uri_pattern, '^http://', '^https?://')
 WHERE base_uri_pattern LIKE '^http://%';

-- Backfill: elect one primary per group using the canonical order. Pre-`observed_at`,
-- `updated_at` is the recency proxy; `id` is the terminal tie-break.
UPDATE entity_identifiers SET is_primary = false;
WITH ranked AS (
  SELECT ei.id, ROW_NUMBER() OVER (
    PARTITION BY ei.entity_type, ei.entity_id, ei.authority_code
    ORDER BY CASE ei.reconciliation_method
               WHEN 'manual' THEN 5 WHEN 'imported' THEN 4
               WHEN 'auto_high' THEN 3 WHEN 'auto_low' THEN 2
               WHEN 'enrichment_sameas' THEN 1 ELSE 0 END DESC,
             a.trust_level DESC, a.priority_order ASC,
             ei.confidence DESC, ei.updated_at DESC, ei.id DESC) AS rn
  FROM entity_identifiers ei
  JOIN knowledge_graph_authorities a ON a.authority_code = ei.authority_code)
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

CREATE TABLE identity_not_duplicates (
  entity_type TEXT NOT NULL CHECK (entity_type IN ('place','organization')),
  id_a TEXT NOT NULL, id_b TEXT NOT NULL,
  evidence_fingerprint TEXT NOT NULL,
  decision_id TEXT REFERENCES identity_decisions(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), created_by TEXT NOT NULL,
  PRIMARY KEY (entity_type, id_a, id_b), CHECK (id_a < id_b)
);
CREATE INDEX idx_identity_not_duplicates_b ON identity_not_duplicates(entity_type, id_b);

CREATE INDEX idx_places_merged_into ON places(merged_into_id);
CREATE INDEX idx_organizations_merged_into ON organizations(merged_into_id);

-- DOWN
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
-- Authority pattern broadening is data; not reverted on down.
```

**Blue/green note**: `is_canonical` is written `false` and kept (not dropped) because
`deploy/scripts/deploy.sh` runs migrations while the **old** container still serves and still
`INSERT`s that column. It is dropped in a later release once no deployed container references
it. During overlap the old container writes rows with `is_primary=false`; run
`server identity tidy --apply` after cutover to fill missing primaries.

**Append-only**: `identity_decisions` is append-only by convention (no UPDATE/DELETE in
application code); no DB trigger in this phase.

### Error Handling

- **Structural** (bad entity type, invalid ULID, URI failing the authority pattern, empty
  actor) → domain error of BadRequest class; API/CLI returns `400` RFC 7807
  (`internal/api/handlers/validators.go:25`).
- **Not found** (valid ULID, absent entity) → `404`, `type` `.../not-found`.
- **Semantic/policy** (escalation) does not exist in Phase 1; it arrives with the Phase 2
  validator.
- All errors wrapped (`fmt.Errorf("identity: %w", err)`). A primary-index unique violation is
  translated to domain `ErrPrimaryConflict`, never surfaced raw.

### Security Model

- **Authorization**: all identity endpoints/CLI verbs are admin-role (JWT). No public/MCP
  exposure.
- **Write surface in Phase 1**: admin-JWT `POST .../identity/link` and `.../reject` (+ CLI).
  No merge/undo writes.
- **Untrusted input**: identifier URIs validated against `base_uri_pattern` before storage;
  failure is structural and the observation is rejected (logged, no row written). API
  responses echo stored values only.
- **Attribution**: `actor` is derived from `middleware.AdminClaims(r).Subject`; never
  client-supplied.
- **Data lifecycle**: `identity_decisions` and `identity_not_duplicates` are durable Postgres
  tables covered by the existing deploy pre-migration snapshot/backup
  (`deploy/scripts/deploy.sh`). `identity_decisions` is never pruned (audit); suppression is
  retained indefinitely unless the evidence fingerprint changes (PQ2). No new file storage.

---

## Implementation Tasks

### Task 1: Migration `000051` (primary slot, patterns, decision/suppression stores, indexes)

**What**: Add `observed_at`/`is_primary`/`superseded_by_id`/`source`; broaden authority
patterns; backfill one primary per group (canonical order, `updated_at` recency proxy); create
the partial unique index; create `identity_decisions` and `identity_not_duplicates` with
`evidence_fingerprint`; add merge indexes; include a working **down**. Regenerate SQLc.
**Test**: `make migrate-up`, `make migrate-down`, `make migrate-up` on a scratch DB; invariant
query returns 0 rows; a direct second-primary insert fails.
**Acceptance**: up/down/up succeed; `SELECT entity_type, entity_id, authority_code, count(*)
FROM entity_identifiers WHERE is_primary GROUP BY 1,2,3 HAVING count(*)>1` returns 0 rows;
`make sqlc` produces no diff after commit.

### Task 2: Atomic primary election + writer integration

**What**: Implement `internal/identity/rank.go` (`ElectPrimary`) and
`internal/identity/store.go` (`RecordObservation`: advisory lock → `UpsertObservation` →
group load with authority join → Go `ElectPrimary` → demote+supersede →
unconditional set primary, in one transaction). **Delete** the existing
`UpsertEntityIdentifier` query (do **not** rename it — `UpsertObservation` was already
added in Task 1 and now replaces it); its `ON CONFLICT DO UPDATE` must not touch
`is_primary`. Add the SQLc queries. Update `internal/kg/reconciliation.go` and
`internal/jobs/workers.go` to call `RecordObservation`; remove `IsCanonical` from writer
params; regenerate SQLc.
**Test**: unit tests for `ElectPrimary` ordering with explicit ties (each tier, incl. `id`
tie-break); integration test that a higher-ranked second write demotes the prior primary, sets
`superseded_by_id`, and retains both rows; **idempotency test**: re-observing the current
winner leaves it primary; concurrency test (two goroutines) yields exactly one primary.
**Acceptance**: after two reconciles with different top matches, exactly one `is_primary` row
exists and the higher-ranked wins; `superseded_by_id` points at the new primary; `observed_at`
refreshed; re-observing the current winner never clears its primary flag.

### Task 3: Transactional merge integrity (identifiers, tombstones)

**What**: Make `MergePlaces`/`MergeOrganizations` (`events_repository.go:1717`, `:1839`) run
in one transaction; resolve the duplicate/primary **ULIDs** (methods receive UUIDs, while
`entity_identifiers.entity_id` stores ULIDs); reassign/dedupe identifiers onto the survivor;
re-elect one primary; write a `place_tombstones`/`organization_tombstones` row (URI+payload
from the domain service). Move the tombstone-payload builders from
`internal/api/handlers/admin.go:471,560` to a new exported `internal/domain/tombstones`
package (moving `buildPlaceURI`/`buildOrganizationURI` with them) and call them from the
domain service; the domain service resolves UUIDs to `(ULID, name)` via the existing place/org
getters and receives `BaseURL` via constructor injection. Fix `MergeEvents`
(`queries/events.sql:78-85`) to set `deletion_reason`. (`deletion_reason` is already set for
place/org merges.)
**Test**: integration tests for US2 scenarios incl. injected-failure rollback and shared-
identifier dedup; tombstone builder unit tests.
**Acceptance**: 0 `entity_identifiers` rows reference a soft-deleted place/org after any merge
path; tombstone present with non-null URI/payload; failure rolls back cleanly.

### Task 4: LinkIdentifier / Reject executor + decision store

**What**: Implement `internal/identity/execute.go` (`LinkIdentifier`, `Reject`),
`internal/identity/record.go` (DecisionStore), `internal/identity/fingerprint.go` (evidence
fingerprint), and `identity_not_duplicates` read/write. Validate URIs via authority patterns;
require non-empty actor. `LinkIdentifier` calls `RecordObservationTx` inside a transaction
that also appends the decision (atomic).
**Test**: unit + integration for LinkIdentifier (upsert + election + decision), Reject
(not-duplicate row with fingerprint + decision), empty-actor structural error, fingerprint
re-open (different evidence ⇒ `IsNotDuplicate` false), idempotent re-link, and an
atomic-rollback test (a decision-append failure rolls back the election).
**Acceptance**: each call appends exactly one decision record; Reject suppresses identical
evidence indefinitely and permits resurface on changed evidence.

### Task 5: Admin REST (read + link/reject) + OpenAPI + lint manifest

**What**: Add `GET .../identity/{type}/{id}`, `.../conflicts`, `.../decisions`, and
`POST .../link`, `.../reject` via `internal/api/handlers/identity.go`, wired in
`internal/api/router.go` (`jwtAuth` + `AdminRequestSize`); `actor` from JWT subject. Update
`docs/api/openapi.yaml` (OAS 3.1; include env-var name/default for `IDENTITY_CONFLICT_LIMIT_MAX`
in the `description`). Add `IDENTITY_CONFLICT_LIMIT_MAX` to the
`internal/config/openapi_lint_test.go` manifest.
**Test**: handler tests with a stub repo; keyset cursor round-trip tests (conflicts +
decisions); `make lint-openapi`; contract tests for 400 (bad ULID), 404 (absent), 401 (no JWT).
**Acceptance**: response shapes match the structs; conflicts exclude suppressed by default and
include `prior_decision` when `include_suppressed=true`; openapi lint passes with the manifest
entry.

### Task 6: `server identity` CLI (check | conflicts | link | reject)

**What**: Add `cmd/server/cmd/identity.go`. `check`/`conflicts` call the read endpoints;
`link`/`reject` call the admin write endpoints. Auth mirrors `server review`.
**Test**: CLI tests against an httptest server (all four verbs); golden table/JSON snapshots.
**Acceptance**: output matches REST; `link`/`reject` produce decision records; `--json`
matches the schemas.

### Task 7: `server identity tidy` repair (folds `t_e522e110`)

**What**: Add `tidy [--dry-run|--apply] [--type ...]` that (a) fills a primary where a group
has observations but none primary, and (b) demotes extras where a group has >1 primary
(legacy/pre-index). Reports entities scanned / primaries filled / rows demoted.
**Test**: fixtures — a "no primary" group and a legacy multi-primary group created **after
temporarily dropping the partial index** (the only way a post-000051 schema can hold
multi-primary; documented in the test). Dry-run purity; apply idempotence.
**Acceptance**: after `--apply`, the invariant query returns 0 rows and every group with
observations has a primary; dry-run leaves DB unchanged; a second `--apply` reports 0.

---

## Configuration

New typed fields in `internal/config/config.go` (`getEnv*` defaults):

| Env var | Default | Purpose |
|---|---|---|
| `IDENTITY_CONFLICT_LIMIT_MAX` | `200` | Max `limit` accepted by conflicts/decisions read endpoints |
| `IDENTITY_OBSERVATION_TTL_DAYS` | `180` | Re-verification staleness (used Phase 3) |

Threshold/fan-out config lands in Phase 2 with actions. Env-controlled endpoint behaviour must
name the env var + default in the OpenAPI `description`
(`internal/config/openapi_lint_test.go` enforces the manifest).

## Success Criteria

- **0 rows** for the primary-slot invariant query on staging after deploy + `identity tidy
  --apply` (tidy also fills rows written during the blue/green overlap).
- **0 orphaned** `entity_identifiers` rows for **places/orgs** after the merge integration
  tests and after any production place/org merge path. (Events/consolidate out of scope.)
- Identity view API and `server identity check` return identical data for the same ULID.
- `make ci-fast` green; new unit + integration tests pass; `make lint-openapi` passes.
- No regression in `server reconcile stats` matched/enriched counts on staging.

## Open Questions

- ~~PQ1~~ **Resolved (2026-09-10): `Link` reversal is a superseding observation, not row
  deletion.**
- ~~PQ2~~ **Resolved (2026-09-10): signal-scoped suppression.** `identity_not_duplicates` is
  retained indefinitely and re-opened only when the recorded **evidence fingerprint** changes.
  Places/orgs are persistent (unlike events, which age out), so suppression keys on evidence,
  not a clock. No cleanup job.
- PQ3 — **deferred to Phase 2**: whether `identity_decisions` needs additional precedent
  projection beyond the filterable feed.
