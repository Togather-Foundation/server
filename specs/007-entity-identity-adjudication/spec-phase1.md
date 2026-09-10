# Phase 1 Specification: Identity Primitives, Repair & Read Loop

**Spec**: 007-entity-identity-adjudication / Phase 1 | **Date**: 2026-09-10 | **Status**: Draft
**Parent**: `specs/007-entity-identity-adjudication/plan.md`
**Goal**: The node stores at most one **primary** external identifier per
`(entity, authority)`, enforced by a DB index; every place/organization merge preserves
identity (no orphaned `entity_identifiers`); and an external agent can **read** identity
state for a place/org via the admin REST API and `server identity` CLI. Target: **0 rows**
violating the primary invariant and **0 orphaned identifiers** after a merge, verified on
staging.

> Phase 1 is deterministic and read-oriented. It introduces the model, repairs existing
> drift, and exposes read + internal link/reject primitives. The agent **action loop**
> (queue-driven link/reject/merge with policy validation) is Phase 2.

---

## Context

### What Exists Today

| Component | Status | Code reference |
|---|---|---|
| `entity_identifiers` (sameAs edges) | Write-only; `is_canonical` per-row predicate | `internal/kg/reconciliation.go:330,332`, `internal/jobs/workers.go:422` |
| Writers | reconciliation + enrichment | `internal/kg/reconciliation.go:239-275`, `internal/jobs/workers.go:417-441` |
| Readers | none in Go (CLI raw SQL only) | `cmd/server/cmd/reconcile.go:307,443,571` |
| Primary/unique constraint | `UNIQUE(entity_type, entity_id, authority_code, identifier_uri)` (permits multi-canonical) | `migrations/000030_knowledge_graph_tables.up.sql:36` |
| `knowledge_graph_authorities` (trust/priority) | Seeded, unused by logic | `000030:62-67` |
| Place merge | reassigns events/occurrences; **does not touch identifiers**; no tombstone | `internal/storage/postgres/events_repository.go:1717` |
| Org merge | reassigns events; **does not touch identifiers**; no tombstone | `events_repository.go:1839` |
| Event merge | omits `deletion_reason` | `queries/events.sql:78-85` |
| `sources.trust_level` | Wired into field merge | `create_event_core.go:165-173,220-231` |
| Admin auth | JWT via `POST /api/v1/auth/token` | `internal/api/router.go`, `docs/deploy/api-keys.md` |
| Review CLI pattern | Live agent contract to mirror | `docs/integration/tg-review.md` |

**Observed drift (staging, 2026-09-10):** 172 `is_canonical=true` artsdata rows across only
144 places (~28 places with >1 canonical).

### What This Phase Delivers

1. Migration `000051`: primary-slot columns + partial unique index + backfill; an
   append-only `identity_decisions` table; `identity_not_duplicates`; missing merge indexes.
2. Deterministic **primary election** logic, integrated into reconciliation/enrichment writes
   (supersede prior primary; never overwrite silently).
3. **Merge integrity**: `MergePlaces`/`MergeOrganizations` reassign/union identifiers onto the
   survivor, create tombstones, and set `deletion_reason`; `MergeEvents` sets `deletion_reason`.
4. Internal `Link`/`Reject` executor + decision-record store (additive; no HTTP action yet).
5. Read-only admin REST: identity view + conflict list; reflected in `docs/api/openapi.yaml`.
6. `server identity check` and `server identity conflicts` CLI commands.
7. `server identity tidy --dry-run|--apply` repair command (folds `t_e522e110`).

### Non-Goals (deferred)

- No candidate **queue** + action endpoints (link/reject/merge over HTTP) → **Phase 2**.
- No merge over HTTP, no `undo`, no policy validator → **Phase 2**.
- No LLM, no prompts, no MCP tools, no batch operations.
- No external KG **candidate generation** or `sameAs` emission → **Phase 3**.
- No events/persons identity → **Phase 4**.
- No vector/semantic matching.

### Design Constraint Reminders

Carried from the plan (not repeated in full): SEL ULID is canonical and external URIs are
aliases; exactly one primary per `(entity, authority)`; no cross-authority transitivity;
all destructive actions reversible + tombstoned; evidence ≠ action; keep-all identifiers;
config in `internal/config/config.go`; RFC 7807; SQLc; migrations via `make`; no vector;
MCP unchanged.

---

## User Scenarios & Testing

### User Story 1 — One primary identifier per authority (Priority: P0)

**Independent Test**: Insert two `auto_high` identifier rows for the same place+authority and
assert the DB rejects a second primary and the rank chooses the correct winner.

**Acceptance Scenarios**
1. Given a place with existing `artsdata` observations of confidence 0.99 (`auto_high`) and
   0.90 (`auto_low`), when election runs, then exactly one row has `is_primary=true` and it
   is the `auto_high` row.
2. Given a place whose current primary is `K11-24`, when reconciliation writes a new
   `auto_high` `K11-99`, then `K11-99` becomes primary and `K11-24` is demoted
   (`is_primary=false`, `superseded_by_id` set), and the old row is retained.
3. Given any entity+authority, when a second `is_primary=true` row is inserted directly via
   SQL, then the partial unique index raises a unique-violation.

### User Story 2 — Merge preserves identity (Priority: P0)

**Independent Test**: Create two places each with an `artsdata` identifier, merge one into the
other, assert the survivor holds both identifiers, the duplicate is tombstoned, and no
`entity_identifiers` row references a soft-deleted ULID.

**Acceptance Scenarios**
1. Given place A (`K11-24`) and place B (`K11-25`) with no shared identifier, when A is merged
   into B, then B's identifier set contains both `K11-24` and `K11-25`, exactly one is
   primary, and A has `merged_into_id=B`, `deleted_at` set, `deletion_reason='merged'`.
2. Given the same merge, then a `place_tombstones` row exists for A and
   `SELECT count(*) FROM entity_identifiers WHERE entity_id=A` is 0.
3. Given a merge where both A and B already assert the same authority+URI, then the union is
   deduplicated (no duplicate rows) and one primary remains.

### User Story 3 — Inspect identity via API and CLI (Priority: P0)

**Independent Test**: `GET /api/v1/admin/identity/place/{ulid}` and
`server identity check place {ulid}` return the same identifiers, primary map, and decisions.

**Acceptance Scenarios**
1. Given a reconciled place, when the identity view is fetched, then it lists all identifiers
   with authority/method/confidence/`is_primary`/`observed_at` and a `primary` map keyed by
   authority.
2. Given a place with prior decisions, when fetched, then `decisions` is ordered newest-first.
3. Given an unknown ULID, when fetched, then `404` with an RFC 7807 body and `type` ending
   `/not-found`.

### User Story 4 — Record link / reject decisions (Priority: P1)

**Independent Test**: Call the internal executor `Link` then `Reject`, and assert two
`identity_decisions` rows and one `identity_not_duplicates` row exist.

**Acceptance Scenarios**
1. Given two entities confirmed to share an identifier, when `Link` executes, then the
   identifier is upserted with `source` and a decision record is appended with `reversible=true`.
2. Given two entities judged distinct, when `Reject` executes, then an
   `identity_not_duplicates` pair is recorded (canonical ordering) and future conflict lists
   omit the pair.
3. Given an executor call with an empty actor, then it returns a structural error and writes
   no decision.

### User Story 5 — Repair existing drift (Priority: P1)

**Independent Test**: Seed a place with two primary rows (pre-migration fixture), run
`server identity tidy --dry-run` then `--apply`, and assert one primary remains.

**Acceptance Scenarios**
1. Given drift, when `tidy --dry-run` runs, then it prints affected entities and mutates
   nothing.
2. Given drift, when `tidy --apply` runs, then the invariant holds and a summary reports
   entities scanned / rows demoted.
3. Given no drift, when `tidy --apply` runs, then it reports 0 changes and exits 0.

---

## Technical Design

### Package Layout

```
internal/identity/
  ref.go            # EntityType, IdentityRef
  rank.go           # IdentifierObservation, ElectPrimary
  execute.go        # Executor: Link, Reject
  record.go         # DecisionRecord, DecisionStore contract
internal/identity/service.go   # read model: IdentityView assembly
internal/storage/postgres/queries/identity.sql        # SQLc: primary election, view, decisions, not-duplicates
internal/storage/postgres/migrations/000051_entity_identity_primary.{up,down}.sql
internal/api/handlers/identity.go                     # GET view + conflicts
cmd/server/cmd/identity.go                            # check | conflicts | tidy
internal/kg/reconciliation.go                         # writers use election
internal/jobs/workers.go                              # enrichment writer uses election
internal/storage/postgres/events_repository.go        # merge reassigns identifiers + tombstones
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
    ID         int
    Authority  string
    URI        string
    Method     string    // manual|imported|auto_high|auto_low|enrichment_sameas
    Confidence float64
    ObservedAt time.Time
    TrustLevel int       // knowledge_graph_authorities.trust_level
    Priority   int       // knowledge_graph_authorities.priority_order
}

func ElectPrimary(obs []IdentifierObservation) (IdentifierObservation, bool)

// internal/identity/record.go
type DecisionRecord struct {
    ID          string       `json:"id"`         // "idn-{ulid}"
    Timestamp   time.Time    `json:"timestamp"`
    Ref         IdentityRef  `json:"ref"`
    Action      string       `json:"action"`     // link|reject (merge|escalate reserved, Phase 2)
    Counterpart *IdentityRef `json:"counterpart,omitempty"`
    Rationale   string       `json:"rationale"`
    Citations   []string     `json:"citations"`
    Confidence  float64      `json:"confidence"`
    Actor       string       `json:"actor"`
    Reversible  bool         `json:"reversible"`
    UndoRef     string       `json:"undo_ref,omitempty"`
}

type IdentifierView struct {
    Authority  string    `json:"authority"`
    URI        string    `json:"uri"`
    Method     string    `json:"method"`
    Confidence float64   `json:"confidence"`
    IsPrimary  bool      `json:"is_primary"`
    ObservedAt time.Time `json:"observed_at"`
}

type IdentityView struct {
    Ref         IdentityRef       `json:"ref"`
    Identifiers []IdentifierView  `json:"identifiers"`
    Primary     map[string]string `json:"primary"` // authority -> URI
    Decisions   []DecisionRecord  `json:"decisions"`
}
```

### Interfaces

```go
// internal/identity/execute.go
type IdentifierUpserter interface {
    UpsertEntityIdentifier(ctx context.Context, arg postgres.UpsertEntityIdentifierParams) (postgres.EntityIdentifier, error)
    GetEntityIdentifiers(ctx context.Context, arg postgres.GetEntityIdentifiersParams) ([]postgres.EntityIdentifier, error)
    SetPrimary(ctx context.Context, id int32) error
    ClearPrimary(ctx context.Context, entityType, entityID, authority string) error
}

type NotDuplicateStore interface {
    InsertNotDuplicate(ctx context.Context, arg postgres.InsertIdentityNotDuplicateParams) error
    IsNotDuplicate(ctx context.Context, arg postgres.IsIdentityNotDuplicateParams) (bool, error)
}

type Executor struct { /* ids, upserter, notDup, decisions, clock */ }
func (e *Executor) Link(ctx context.Context, ref IdentityRef, obs IdentifierObservation, actor string) (DecisionRecord, error)
func (e *Executor) Reject(ctx context.Context, a, b IdentityRef, actor, reason string) (DecisionRecord, error)

// internal/identity/record.go
type DecisionStore interface {
    Append(ctx context.Context, rec DecisionRecord) (string, error)
    List(ctx context.Context, ref IdentityRef) ([]DecisionRecord, error)
}
```

`ElectPrimary` compares observations by: method rank (`manual` > `imported` > `auto_high` >
`auto_low` > `enrichment_sameas`) → `TrustLevel`/`Priority` (authority) → `Confidence` →
`ObservedAt` (newest). Returns `(zero, false)` for an empty set.

### REST / CLI Schemas

**GET `/api/v1/admin/identity/{type}/{id}`** → `200`:
```json
{
  "entity_type": "place",
  "entity_id": "01M23B21QYBRFFVVDZ0C0PSQDX",
  "primary": { "artsdata": "https://kg.artsdata.ca/resource/K11-24" },
  "identifiers": [
    { "authority": "artsdata", "uri": "https://kg.artsdata.ca/resource/K11-24",
      "method": "auto_high", "confidence": 0.99, "is_primary": true,
      "observed_at": "2026-09-09T15:02:26Z" }
  ],
  "decisions": []
}
```

**GET `/api/v1/admin/identity/conflicts?type=place&limit=50`** → `200`: entities that share an
identifier with a *different* entity (strong duplicate candidate), newest first:
```json
{
  "items": [
    { "entity_type": "place", "entity_id": "01...A", "candidate": "01...B",
      "authority": "artsdata", "uri": "https://kg.artsdata.ca/resource/K11-24" }
  ],
  "next_cursor": null
}
```

**GET `/api/v1/admin/identity/decisions?type=place&since=&limit=100`** → `200`: append-only
decision feed, newest-first, for agent precedent replay:
```json
{ "items": [ { "id": "idn-...", "timestamp": "...", "entity_type": "place",
  "entity_id": "01...", "action": "reject", "counterpart_id": "01...",
  "rationale": "different address, different operator", "citations": ["idn-..."],
  "confidence": 0.9, "actor": "hermes-agent", "reversible": true } ],
  "next_cursor": null }
```

**CLI** (mirrors `server review`):
```
server identity check place 01M23B21QYBRFFVVDZ0C0PSQDX [--json]
server identity conflicts --type place [--limit 50] [--json]
server identity tidy [--dry-run|--apply] [--type place|organization]
```

### Migration `000051` (sketch)

```sql
-- Add primary-slot columns (is_canonical is DEPRECATED, NOT dropped: blue/green safety).
ALTER TABLE entity_identifiers
  ADD COLUMN observed_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  ADD COLUMN is_primary       BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN superseded_by_id INTEGER REFERENCES entity_identifiers(id);

-- Backfill: elect one primary per (entity_type, entity_id, authority_code).
UPDATE entity_identifiers SET is_primary = false;
WITH ranked AS (
  SELECT id,
         ROW_NUMBER() OVER (
           PARTITION BY entity_type, entity_id, authority_code
           ORDER BY CASE reconciliation_method
                      WHEN 'manual' THEN 5 WHEN 'imported' THEN 4
                      WHEN 'auto_high' THEN 3 WHEN 'auto_low' THEN 2
                      WHEN 'enrichment_sameas' THEN 1 ELSE 0 END DESC,
                    confidence DESC, updated_at DESC
         ) AS rn
  FROM entity_identifiers
)
UPDATE entity_identifiers ei SET is_primary = true FROM ranked r
 WHERE ei.id = r.id AND r.rn = 1;

CREATE UNIQUE INDEX idx_entity_identifiers_one_primary
  ON entity_identifiers(entity_type, entity_id, authority_code) WHERE is_primary;

CREATE TABLE identity_decisions (
  id TEXT PRIMARY KEY, created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  entity_type TEXT NOT NULL, entity_id TEXT NOT NULL,
  action TEXT NOT NULL, counterpart_type TEXT, counterpart_id TEXT,
  rationale TEXT NOT NULL DEFAULT '', citations JSONB NOT NULL DEFAULT '[]',
  confidence NUMERIC(5,4) NOT NULL DEFAULT 0, actor TEXT NOT NULL,
  reversible BOOLEAN NOT NULL DEFAULT true, undo_ref TEXT, metadata JSONB
);
CREATE INDEX idx_identity_decisions_entity ON identity_decisions(entity_type, entity_id, created_at DESC);

CREATE TABLE identity_not_duplicates (
  entity_type TEXT NOT NULL, id_a TEXT NOT NULL, id_b TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), created_by TEXT NOT NULL,
  PRIMARY KEY (entity_type, id_a, id_b), CHECK (id_a < id_b)
);

CREATE INDEX idx_places_merged_into ON places(merged_into_id);
CREATE INDEX idx_organizations_merged_into ON organizations(merged_into_id);
```
**Do not drop `is_canonical` in this migration.** Writers switch to `is_primary` in Task 2;
the column is dropped in a later release once no deployed container reads it (documented
blue/green constraint).

### Error Handling

- **Structural** (malformed type/ULID, unknown type) → `400`/`422` RFC 7807 via
  `ValidateAndExtractULID` pattern (`internal/api/handlers/admin.go:367`).
- **Not found** → `404`, `type` `.../not-found`.
- **Semantic** (e.g. `Link` with a URI that violates the authority regex) → return an error
  to the caller; no decision recorded. There is no escalation concept in Phase 1 (no queue).
- All errors wrapped (`fmt.Errorf("identity: %w", err)`); SQLc violation of the primary index
  is translated to a domain error, not surfaced raw.

### Security Model

- **Authorization**: all identity endpoints are admin-role (JWT), matching `/api/v1/admin/*`.
  No public/MCP exposure.
- **Write surface in Phase 1**: none over HTTP (executor is internal). Read endpoints only.
- **Untrusted data**: identifier URIs are validated against
  `knowledge_graph_authorities.base_uri_pattern` before storage; API responses echo stored
  values only. No scraped free-text is introduced in Phase 1 responses.
- **Data lifecycle**: `identity_decisions` and `identity_not_duplicates` are durable,
  backed-up DB tables (Postgres); `identity_not_duplicates` is the only irreversible record
  and is admin-only. No new file/data-directory storage.

---

## Implementation Tasks

### Task 1: Migration 000051 — primary slot, decision store, not-duplicates, merge indexes

**What**: Add `observed_at`/`is_primary`/`superseded_by_id` to `entity_identifiers`;
backfill election; create the partial unique index; create `identity_decisions` and
`identity_not_duplicates`; add `merged_into_id` indexes. Regenerate SQLc (`make sqlc`).
**Test**: `make migrate-up` then `make migrate-down` on a scratch DB; run the invariant query
below and expect `0 rows`; unit test that a direct second-primary insert fails.
**Acceptance**: migration up/down succeed; `SELECT entity_type, entity_id, authority_code,
count(*) FROM entity_identifiers WHERE is_primary GROUP BY 1,2,3 HAVING count(*) > 1;` returns
0 rows; `make sqlc` produces no diff after commit.

### Task 2: Primary election + writer integration

**What**: Implement `internal/identity/rank.go` (`ElectPrimary`); update
`internal/kg/reconciliation.go` `storeIdentifier` and `internal/jobs/workers.go` sameAs write
to use election (set the new primary, demote the prior, set `superseded_by_id`) inside a
transaction. Stop writing `is_canonical`.
**Test**: unit tests for `ElectPrimary` ordering (method, trust, confidence, recency); fake
store test that a second write demotes the first.
**Acceptance**: after reconciling an entity twice with different top matches, exactly one
`is_primary` row exists and the newer/better one wins; prior row retained and superseded.

### Task 3: Merge integrity — reassign identifiers, tombstones, deletion_reason

**What**: In `MergePlaces` (`events_repository.go:1717`) and `MergeOrganizations` (`:1839`),
reassign `entity_identifiers.entity_id` from duplicate → primary, dedupe identical
authority+URI rows, re-elect one primary, write a `place_tombstones`/`organization_tombstones`
row, and set `deletion_reason='merged'`. Fix `MergeEvents` (`queries/events.sql:78-85`) to set
`deletion_reason`.
**Test**: integration test creating two places/orgs with distinct identifiers, merge, assert
union on survivor, 0 identifiers on duplicate, tombstone exists, `deletion_reason` set.
**Acceptance**: the merge-orphaning regression test passes; no `entity_identifiers` row
references a soft-deleted entity after any merge path.

### Task 4: Link / Reject executor + decision store

**What**: Implement `internal/identity/execute.go` (`Link`, `Reject`) and
`internal/identity/record.go` (DecisionStore over `identity_decisions`), plus
`identity_not_duplicates` read/write. Validate URIs against the authority regex. Enforce
non-empty actor.
**Test**: unit + integration tests for Link (upsert + primary election + decision record),
Reject (not-duplicate pair + decision record), empty-actor structural error.
**Acceptance**: `Link`/`Reject` each append exactly one decision record; `Reject` makes the
pair disappear from the conflicts query.

### Task 5: Read-only admin REST + OpenAPI

**What**: Add `GET /api/v1/admin/identity/{type}/{id}`,
`GET /api/v1/admin/identity/conflicts`, and `GET /api/v1/admin/identity/decisions` (filterable
decision feed for precedent replay) via a new `internal/api/handlers/identity.go`, wired
through `internal/api/router.go` with `jwtAuth` + `AdminRequestSize`. Update
`docs/api/openapi.yaml` (OAS 3.1; `type: [string, 'null']` where needed).
**Test**: handler tests with a stub repo; `make lint-openapi`; contract test that an unknown
ULID returns 404 with RFC 7807; feed pagination/cursor test.
**Acceptance**: endpoints return the shapes above; the decisions feed is newest-first and
supports `type`/`since`/`limit`; openapi lint passes; auth required (401 without JWT).

### Task 6: `server identity` CLI

**What**: Add `cmd/server/cmd/identity.go` with `check`, `conflicts` (`tidy` is Task 7).
Auth mirrors `server review` (`--key`/`--token`/`--server`; STS exchange).
**Test**: CLI unit tests against an httptest server; golden output snapshots for table + JSON.
**Acceptance**: `server identity check place <ulid>` and the REST endpoint return the same
data; `--json` matches the schema.

### Task 7: `server identity tidy` repair (folds `t_e522e110`)

**What**: Add `tidy [--dry-run|--apply] [--type ...]` that finds `(entity, authority)` groups
with >1 primary (pre-index / legacy) or with no primary where observations exist, elects a
primary, demotes others, and reports counts. `--dry-run` mutates nothing.
**Test**: seed multi-primary/no-primary fixtures; dry-run purity; apply idempotence (second
run reports 0 changes).
**Acceptance**: after `--apply`, the invariant query returns 0 rows; dry-run leaves DB
unchanged; closing `t_e522e110` is justified when this ships.

---

## Configuration

New typed fields in `internal/config/config.go` with `getEnv*` defaults (per DRY rule):

| Env var | Default | Purpose |
|---|---|---|
| `IDENTITY_OBSERVATION_TTL_DAYS` | `180` | Re-verification staleness for identifiers (used Phase 3) |
| `IDENTITY_CONFLICT_LIMIT_MAX` | `200` | Max `limit` for the conflicts endpoint |

(Threshold/fan-out config is introduced in Phase 2 when actions land.) If any read endpoint
behaviour is env-controlled, the OpenAPI `description` must name the env var and default
(enforced by `internal/config/openapi_lint_test.go`).

## Success Criteria

- **0 rows** for the primary-slot invariant query on staging after deploy + `tidy --apply`.
- **0 orphaned** `entity_identifiers` rows (referencing soft-deleted entities) after running
  the merge integration tests and after any production merge path.
- Identity view API + `server identity check` return identical data for the same ULID.
- `make ci-fast` green; new unit + integration tests pass; `make lint-openapi` passes.
- No regression in `server reconcile stats` matched/enriched counts on staging.

## Open Questions

- ~~PQ1~~ **Resolved (2026-09-10): `Link` reversal is a superseding observation, not row
  deletion.**
- PQ2 — `identity_not_duplicates` retention: indefinite (proposed) or time-boxed? (operator
  requested more information.)
- ~~PQ3~~ **Resolved (2026-09-10): add a read-only filterable decisions feed** for agent
  precedent replay (delivered in Task 5).
