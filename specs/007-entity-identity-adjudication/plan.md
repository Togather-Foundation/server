# Plan: Entity Identity & Adjudication

**Spec**: 007-entity-identity-adjudication | **Date**: 2026-09-10 | **Status**: Approved (2026-09-10)
**Goal**: A SEL node exposes entity-identity candidates to an external agent via API; the
agent reasons and acts via API; the server deterministically validates and executes
reversible, provenance-tracked identity actions (link / reject / merge), so that places
and organizations converge on a single identity with **at most one primary external
identifier per authority** and zero silent data loss.

> Context/background: `docs/design/entity-identity-adjudication.md` (discovery findings).
> This plan is the architectural blueprint; the normative build detail is `spec-phaseN.md`.

---

## Vision

Togather nodes are meant to be run cheaply and with low maintenance by volunteers in
many cities. The event review queue already works this way: the server deterministically
detects data-quality issues, parks them where an **external agent can fetch them by API,
reason on them, and act by API** (`skills/` + `server review` CLI + admin REST API), and
records the outcome. This plan extends that proven loop to **entity identity** — the
problem of deciding when two SEL entities are the same real-world thing — and connects it
to the external knowledge-graph identifiers the node already stores but never uses.

Success looks like: duplicate places/orgs surface as identity review items; a cron-driven
agent adjudicates them with good context; the server executes only validated, reversible
actions; and every decision is auditable. No part of this requires an LLM inside the Go
server — the interface is the seam.

---

## Current State

| Capability | Status | Code / location |
|---|---|---|
| Event near-dup + place/org fuzzy dedup | **Wired** | `internal/domain/events/create_event_core.go:432-533` (place), `:592-660` (org), `internal/domain/events/dedup.go`, `internal/config/config.go:213-238` |
| Event/place/org merge | **Wired, incomplete** | `MergePlaces` `internal/storage/postgres/events_repository.go:1717`, `MergeOrganizations` `:1839`, `MergeEvents` `:2125`, `Consolidate` `internal/domain/events/admin_service.go:1562` |
| Review queue (events) | **Wired** | `migrations/000025_create_event_review_queue.up.sql`, `internal/api/handlers/admin_review_queue.go` |
| Not-duplicate suppression (events) | **Wired** | `migrations/000027_event_not_duplicates.up.sql`, writer `admin_review_queue.go:1029,1045`, reader `create_event_core.go:906` |
| External identifiers (`entity_identifiers`) | **Write-only** | writers `internal/kg/reconciliation.go:332`, `internal/jobs/workers.go:422`; no live/domain reader (generated SQLc methods exist but are uncalled); only CLI raw SQL reads, `cmd/server/cmd/reconcile.go:307,443,571` |
| `knowledge_graph_authorities` (trust/priority) | **Unused** | seeded `migrations/000030_knowledge_graph_tables.up.sql:62-67`; queries `GetActiveAuthorities`/`GetAuthoritiesForDomain` have no callers |
| Field-level provenance (`field_provenance`) | **Schema-only** | `migrations/000002_provenance.up.sql:67-111`; `InsertFieldProvenance`/`SupersedeFieldProvenance` have no callers; readers exist (`internal/domain/provenance/service.go`) |
| Source trust (`sources.trust_level`) | **Wired (field merge)** | `create_event_core.go:165-173,220-231`; `events_repository.go:1405,1426` |
| LLM injection boundary (`internal/llmsafe`) | **Wired (scraper only)** | `internal/llmsafe/boundary.go`; `internal/scraper/inspect.go:124,201` |
| MCP review/admin surface | **Absent (by design)** | MCP is public-only; no admin/identity tools |
| External agent review loop | **Live** | `server review` CLI (`docs/integration/tg-review.md`), `skills/togather-review/SKILL.md`, `skills/togather-server-ops/SKILL.md` |

### Gaps this plan closes

1. **Identifiers are never reassigned on merge** (`MergePlaces`/`MergeOrganizations`/`MergeEvents`/`Consolidate`) → orphaned sameAs edges.
2. **Multiple canonical identifiers per authority** (`is_canonical` is a per-row predicate, `internal/kg/reconciliation.go:330`; unique key `000030:36` permits it) → observed on staging: 172 canonical rows across 144 places.
3. **No cross-entity review surface** for place/org identity (only event-queue warnings).
4. **No agent-facing identity API/CLI** (no fetch/reason/act loop).
5. **No durable adjudication record** for identity decisions.
6. **Place/org merges create no tombstone**; event merge omits `deletion_reason`; merge/provenance columns unindexed.

---

## Decisions (approved 2026-09-10)

| # | Decision | Proposed default | Rationale |
|---|---|---|---|
| D1 | External KG identity: backbone or evidence? | **Backbone for candidate generation/linkage; evidence only for destructive action.** No auto-merge from external IDs in v1. | Recon false positives + `owl:sameAs` non-transitivity + merge irreversibility |
| D2 | Canonical model | **SEL ULID is canonical; external URIs are aliases. Exactly one *primary* identifier per `(entity, authority)`, elected by a rank; keep all, supersede.** No global external canonical. | Matches `docs/interop/knowledge-graphs.md` Strategy 2 (keep-all); avoids transitivity traps |
| D3 | v1 posture | **Additive: link + annotate + review.** No external-ID auto-merge. | Reversibility; staged rollout |
| D4 | Review surface | **A dedicated, entity-type-agnostic identity review queue** (not bolted onto `event_review_queue`). | External agent contract; cross-type identity candidate |
| D5 | Driver model + memory | **External-agent-first**; server owns detection/invariants/execution/audit + policy validation; **server-side decision records are authoritative**; agent memory non-authoritative. | Live pattern; nodes can swap/drop agents without losing audit/precedent |
| D6 | Candidate generation | **Exact-ID pre-pass + existing pg_trgm.** Defer vector. | Ship value now; 004 permits pgvector only when volume demands |

---

## Architecture

```
                         ┌─────────────────────────────────────────────┐
  ingest / enrichment →  │        Identity Resolution Engine (server)   │
  (events, places, orgs) │                                             │
                         │  candidate generation:                      │
                         │    • exact external-ID join (sameAs)        │
                         │    • pg_trgm similarity (places/orgs)       │
                         │  scoring + routing:                         │
                         │    • safe link  → apply (additive)          │
                         │    • review     → identity_review_queue     │
                         │    • escalate   → identity_review_queue     │
                         └───────────────┬─────────────────────────────┘
                                         │  REST admin API + server CLI
                                         ▼
                         ┌─────────────────────────────────────────────┐
                         │   External agent (Hermes, cron, skills/)    │
                         │   fetch candidates → reason → propose action│
                         └───────────────┬─────────────────────────────┘
                                         │  POST decision (action + rationale refs)
                                         ▼
                         ┌─────────────────────────────────────────────┐
                         │        Policy Validator (code, server)      │
                         │  action ∈ allowed set · fields present ·    │
                         │  confidence ≥ threshold · red-lines absent ·│
                         │  citation required · blast-radius caps      │
                         └───────────────┬─────────────────────────────┘
                                         │ validated
                                         ▼
                         ┌─────────────────────────────────────────────┐
                         │      Deterministic Executor (server)        │
                         │  link / reject / merge — reversible,        │
                         │  invariant-checked, transactional           │
                         └───────────────┬─────────────────────────────┘
                                         ▼
             entity_identifiers (observations + primary slot) ·
             identity_decisions (append-only) · tombstones ·
             identity_not_duplicates (suppression)
```

**Layered responsibilities (non-negotiable):**
- **Server owns** detection, candidates, scoring, invariants, execution, reversibility,
  audit, and policy validation of the agent's proposed action.
- **Agent owns** reasoning + choice among allowed actions from server-provided context.
- **No prompts or LLM calls in the server.** The contract is driver-agnostic.

---

## Design Constraints

1. **SEL ULID is the canonical identity.** External URIs are `sameAs` aliases. Nothing
   outside the node may mint our `@id` (`internal/domain/ids/ids.go`).
2. **One primary identifier per `(entity_type, entity_id, authority_code)`.** Enforced by a
   partial unique index, not by convention. Keep all observations; supersede, never silently
   overwrite.
3. **No cross-authority `sameAs` transitivity without corroboration.** Two authorities
   asserting the same URI is evidence, not proof.
4. **All destructive actions are reversible and tombstoned.** Soft-delete + merge mapping +
   decision record; no hard deletes.
5. **Policy validation is code, not prompt.** The server validates every proposed action
   before executing; violations become `escalate`/reject.
6. **Evidence ≠ action.** External-ID match alone never triggers a destructive merge in v1.
7. **Keep-all identifiers** (Strategy 2). Emit each authority's primary as `sameAs`; never
   flatten to a single global external URI.
8. **Config in `internal/config/config.go`** (typed struct + `getEnv*` defaults); RFC 7807
   error envelopes; `fmt.Errorf("...: %w", err)`; SQLc for standard queries; migrations via
   `make`; generated files never hand-edited.
9. **No vector/semantic candidate generation** in v1.
10. **MCP stays public-only** for this effort; the agent contract is REST admin API + CLI.

---

## Component Design

### Package layout

```
internal/identity/                  # generic identity resolution (no SEL-specific codes)
  ref.go            # IdentityRef, entity-type enum
  candidate.go      # Candidate, Evidence, CandidateSource interface
  rank.go           # PrimaryRank — election + comparison of identifier observations
  invariants.go     # one-primary, no-transitivity, fan-out caps
  execute.go        # Executor: Link / RejectMerge actions, transactional + reversible
  record.go         # DecisionRecord type + append-only store contract
internal/storage/postgres/queries/identity.sql   # SQLc queries (primary election, candidates, decisions)
internal/storage/postgres/migrations/0000NN_identity_*.{up,down}.sql
internal/api/handlers/identity_review.go         # admin REST: list/get/act on identity items
internal/jobs/identity_scan.go                   # River worker: enqueue candidates for new/changed entities
cmd/server/cmd/identity.go                       # `server identity queue|check|link|reject|merge`
docs/integration/tg-identity.md                  # agent-facing CLI/API contract (mirrors tg-review.md)
```

Existing packages touched: `internal/kg/` (primary election on reconcile/enrichment write),
`internal/storage/postgres/events_repository.go` (identity-merge fixes: `MergePlaces`/
`MergeOrganizations` live here, reached via `internal/domain/events/admin_service.go`),
`internal/api/router.go`, `internal/config/config.go`.

### Data structures

```go
// internal/identity/ref.go
type EntityType string // "place" | "organization" (events deferred)

type IdentityRef struct {
    Type EntityType `json:"entity_type"`
    ULID string     `json:"entity_id"`
}

// internal/identity/candidate.go
type Candidate struct {
    Ref        IdentityRef       `json:"ref"`
    Candidate  IdentityRef       `json:"candidate"`        // the other entity
    Evidence   []Evidence        `json:"evidence"`
    Score      float64           `json:"score"`            // 0.0–1.0, server-computed
    Blocking   string            `json:"blocking"`         // "identifier" | "trigram"
}

type Evidence struct {
    Kind       string   // "identifier" | "name_similarity" | "address_similarity" | "url"
    Authority  string   // when Kind == "identifier" (e.g. "artsdata")
    URI        string   // when Kind == "identifier"
    Confidence float64  // normalized 0.0–1.0
    Source     string   // "reconciliation" | "enrichment_sameas" | "manual" | "agent"
}

// internal/identity/rank.go
// Primary election order (CANONICAL — defined here once; other docs reference it):
//   1. method rank: manual > imported > auto_high > auto_low > enrichment_sameas
//   2. authority trust_level DESC, then priority_order ASC
//   3. confidence DESC
//   4. observed_at DESC (newest)
//   5. id DESC (terminal deterministic tie-break)
type IdentifierObservation struct {
    ID         int32   // entity_identifiers.id (SERIAL)
    Authority  string
    URI        string
    Method     string  // "manual" | "imported" | "auto_high" | "auto_low" | "enrichment_sameas"
    Confidence float64
    ObservedAt time.Time
    TrustLevel int32   // knowledge_graph_authorities.trust_level (SQLc INTEGER -> int32)
    Priority   int32   // knowledge_graph_authorities.priority_order
    IsPrimary  bool    // current slot state (read path)
    Source     string  // provenance: "reconciliation" | "enrichment_sameas" | "manual" | "agent"
}

// internal/identity/record.go
// Flat shape mirrors the identity_decisions columns and the REST/CLI JSON (one contract).
// CreatedAt maps to the identity_decisions.created_at column.
type DecisionRecord struct {
    ID              string      `json:"id"`               // "idn-{ulid}"
    CreatedAt       time.Time   `json:"created_at"`       // identity_decisions.created_at
    EntityType      EntityType  `json:"entity_type"`
    EntityID        string      `json:"entity_id"`
    Action          string      `json:"action"`           // link|reject (merge|escalate reserved, Phase 2)
    CounterpartType *EntityType `json:"counterpart_type,omitempty"`
    CounterpartID   *string     `json:"counterpart_id,omitempty"`
    Rationale       string      `json:"rationale"`
    Citations       []string    `json:"citations"`        // decision/rule ids (precedent)
    Confidence      float64     `json:"confidence"`
    Actor           string      `json:"actor"`
    Reversible      bool        `json:"reversible"`
    UndoRef         string      `json:"undo_ref,omitempty"`
    Metadata        map[string]any `json:"metadata,omitempty"`
}
```

### Interfaces

```go
type CandidateSource interface {
    Candidates(ctx context.Context, ref IdentityRef) ([]Candidate, error)
}

type Ranker interface {
    // ElectPrimary returns the observation that should hold the primary slot.
    ElectPrimary(obs []IdentifierObservation) (IdentifierObservation, bool)
}

type ActionValidator interface {
    // Validate returns a semantic outcome: allowed, or escalate with a reason.
    // It never returns a structural error for a well-formed-but-policy-violating action.
    // (Phase 2.)
    Validate(ctx context.Context, rec DecisionRecord) (ValidationOutcome, error)
}

// Executor is a concrete struct in internal/identity/execute.go (not an interface —
// avoids naming drift; Phase 1 implements LinkIdentifier + Reject; Phase 2 adds Merge/Undo).
type Executor struct { /* ids, store, notDup, decisions, clock */ }

func (e *Executor) LinkIdentifier(ctx context.Context, ref IdentityRef, obs IdentifierObservation, actor string) (DecisionRecord, error)
func (e *Executor) Reject(ctx context.Context, a, b IdentityRef, actor, reason string) (DecisionRecord, error)

type DecisionStore interface {
    Append(ctx context.Context, rec DecisionRecord) (string, error)
    List(ctx context.Context, ref IdentityRef) ([]DecisionRecord, error)
}
```

### Storage changes (migration sketch — exact number verified at implementation)

- `entity_identifiers`: add `observed_at TIMESTAMPTZ`, `is_primary BOOLEAN NOT NULL DEFAULT false`,
  `superseded_by_id INT REFERENCES entity_identifiers(id)`, optional `source TEXT`. Backfill
  one primary per `(entity_type, entity_id, authority_code)` by the D2 rank, then create
  `CREATE UNIQUE INDEX ... ON entity_identifiers(entity_type, entity_id, authority_code)
  WHERE is_primary`.
- New `identity_review_queue` (**Phase 2**; entity-type-agnostic): subject ref, counterpart ref,
  `evidence JSONB`, `score`, `status` (`pending|linked|merged|rejected|dismissed|escalated`),
  `decided_by`, `decided_at`, `rationale`, `citations JSONB`, `reversible`, `undo_ref`.
  Partial index on `status='pending'`; index on `(subject_type, subject_id)`.
- New `identity_decisions` (append-only) backing `DecisionRecord`.
- New `identity_not_duplicates` generalizing `event_not_duplicates`, but **signal-scoped**
  (`evidence_fingerprint`): retained indefinitely and re-opened only when evidence changes.
  (Events use plain indefinite suppression because they age out; places/orgs are persistent.)
- Indexes: `places.merged_into_id`, `organizations.merged_into_id`, `identity_review_queue(status)`.
- **Verify the next migration number** (`ls migrations | tail`) before creating.

### Agent-facing contract (REST + CLI)

Follow `docs/integration/tg-review.md` conventions. Endpoints (must be added to
`docs/api/openapi.yaml`, enforced by `make lint-openapi`):

| Method | Path | Phase | Purpose |
|---|---|---|---|
| GET | `/api/v1/admin/identity/{type}/{id}` | 1 | Identity view: identifiers, primary, decision history |
| GET | `/api/v1/admin/identity/conflicts` | 1 | Entities sharing an external identifier (read) |
| GET | `/api/v1/admin/identity/decisions` | 1 | Filterable decision feed (precedent replay) |
| POST | `/api/v1/admin/identity/link` | 1 | Record an external identifier observation |
| POST | `/api/v1/admin/identity/reject` | 1 | Record not-a-duplicate (requires reason) |
| GET | `/api/v1/admin/identity/queue` | 2 | List pending identity review items |
| GET | `/api/v1/admin/identity/queue/{id}` | 2 | Full review item + evidence + prior decisions |
| POST | `/api/v1/admin/identity/queue/{id}/merge` | 2 | Merge duplicate → primary (review-gated, reversible) |
| POST | `/api/v1/admin/identity/queue/{id}/undo` | 2 | Reverse a prior reversible action |

CLI mirrors `server review`: Phase 1 `server identity check|conflicts|link|reject|tidy`;
Phase 2 adds `queue|merge|undo` and batch; `--dry-run` on `tidy` (and merge in Phase 2).

### Configuration (new fields in `internal/config/config.go`)

| Env var | Default | Purpose |
|---|---|---|
| `IDENTITY_TRIGRAM_REVIEW_THRESHOLD` | `0.6` | Flag identity candidates for review |
| `IDENTITY_TRIGRAM_LINK_THRESHOLD` | `0.95` | Auto-link (additive) threshold |
| `IDENTITY_MERGE_MAX_FAN_OUT` | `25` | Max events on either entity before merge always escalates |
| `IDENTITY_CANDIDATE_MAX` | `5` | Max candidates surfaced per subject (Phase 2/3) |
| `IDENTITY_CONFLICT_LIMIT_MAX` | `200` | Max `limit` accepted by the conflicts/decisions read endpoints (Phase 1) |
| `IDENTITY_OBSERVATION_TTL_DAYS` | `180` | Re-verify staleness for external identifiers |

---

## Implementation Phases

Each phase is a vertical slice with entry/exit criteria and interface contracts.
Phase detail is progressive: Phase 1 is specified next (`spec-phase1.md`); later phases are
outlined only.

### Phase 1 — Identity primitives + repair (deterministic; no LLM)

**Delivers:** the data model (primary slot, decision record, not-duplicates, indexes) and
the deterministic executor for *link* and *reject*; fixes identifier orphaning on merge;
exposes the identity API + CLI (read views and link/reject actions). No merge/undo or
policy validator yet.

**Entry:** plan approved.
**Exit:** one primary per authority enforced by index; merge paths reassign identifiers;
`server identity check <type> <id>` returns identifiers + primary + decision history; CI green.
**Interface contracts:** `IdentityRef`, `Candidate`, `Evidence`, `DecisionRecord`,
`ElectPrimary(obs)`, `Executor.{LinkIdentifier,Reject}`, `DecisionStore`; REST read endpoints +
`POST link/reject`; CLI `identity check|conflicts|link|reject|tidy`.

### Phase 2 — Agent adjudication loop

**Delivers:** `identity_review_queue` + REST/CLI actions `merge`/`undo` (link/reject land in
Phase 1); server-side policy validator (allowed set, citation requirement, red-lines, fan-out
caps); agent skill doc (`docs/integration/tg-identity.md` + a `skills/togather-identity` playbook).

**Entry:** Phase 1 delivered.
**Exit:** an external agent (scripted in test) can fetch queue → act → observe decision
record; an invalid/unvalidated action is refused and converted to escalation; merge is
reversible via `undo_ref`.
**Interface contracts:** action endpoints, `ActionValidator.Validate`, `Executor.Merge/Undo`.

### Phase 3 — External KG integration + sameAs emission

**Delivers:** exact-ID candidate generation backed by `entity_identifiers`; primary election
wired into reconcile/enrichment writes; `sameAs` emission from authority primaries;
TTL re-verification; enrichment→identity feedback with runaway guardrails.
**Entry:** Phase 2 delivered; KG reconciliation stable on staging.
**Exit:** staging shows no multi-primary rows; sameAs emitted deterministically; enrichment
cannot trigger an unreviewed merge.

### Phase 4 — Events + learning (later)

**Delivers:** extend identity to events; decision journal → precedent → rule graduation
(reuse the phase-1 decision records); metrics.
**Entry:** Phases 1–3 delivered and stable.
**Exit:** defined per this plan when Phase 3 completes.

---

## Risks and Mitigations

| # | Risk | Severity | Mitigation |
|---|---|---|---|
| R1 | Recon false positive asserted as truth | High | Evidence-only for destructive actions in v1; corroboration required |
| R2 | `owl:sameAs` non-transitivity | High | Authority-scoped primaries; no cross-authority transitivity without corroboration |
| R3 | Merge blast radius (shared place/org) | High | `IDENTITY_MERGE_MAX_FAN_OUT`; merge always review-gated in v1 |
| R4 | Enrichment feedback loop | High | One-way feedback; enrichment cannot self-trigger merges |
| R5 | Identifier orphaning on merge (existing) | Medium | Phase 1 fixes all merge paths; regression tests |
| R6 | Irreversibility | High | Soft-delete + tombstone + `undo_ref`; no hard deletes |
| R7 | Prompt injection reaching the agent | High | Delimit untrusted fields in API payloads; server policy validator blocks unvalidated destructive actions; skill treats payload as data |
| R8 | LLM confidently wrong / variance | Medium | Server-side validation, citation requirement, escalation on doubt |
| R9 | Poisoned precedent compounding | High | Decision records append-only; red-lines; rule graduation human-gated (Phase 4) |
| R10 | Stale external IDs (upstream split/merge) | Medium | `observed_at` + `IDENTITY_OBSERVATION_TTL_DAYS` re-verification |
| R11 | Concurrent merge / chain races | Medium | Reuse `FOR UPDATE SKIP LOCKED` + chain resolution; idempotent, cycle-free primary election |

---

## Security

**Trust boundaries.**
- Untrusted: scraped/gathered payloads (names, descriptions, addresses), external KG
  responses, agent-proposed actions.
- Trusted: server-computed scores, DB invariants, configured thresholds.

**Threats and defenses.**
- *Prompt injection via entity content* → server delimits/sanitizes untrusted fields in
  API payloads (`internal/llmsafe` available); the **policy validator is the backstop** —
  even a fully compromised agent cannot execute an action outside the allowed set or an
  invariant-violating merge.
- *Malicious agent action* → admin JWT authz; allowed-action set; citation requirement;
  fan-out caps; reviewer/actor recorded on every decision.
- *Destructive data loss* → no hard deletes; merge is review-gated, tombstoned, and
  reversible via `undo_ref`; decision record is append-only.
- *Poisoned memory* → agent memory is non-authoritative; server-side decision records only
  influence future actions through server code (Phase 4 rule graduation is human-gated).

**Authorization:** all identity endpoints are admin-role (JWT), matching existing admin
routes. No identity action is exposed on the public/MCP surface.

---

## Open Questions

- ~~Q1~~ **Resolved (2026-09-10): v1 entity types = places + organizations only.**
- ~~Q2~~ **Resolved (2026-09-10): the merge-orphaning fix is part of Phase 1.**
- Q3 — Threshold/red-line governance: config only, or also agent-visible rules?
- Q4 — Review surface: confirm dedicated `identity_review_queue` over generalizing
  `event_review_queue`.
- Q5 — Undo exposure: CLI, admin API, or both (and retention of undo material).
- Q6 — Entry criteria for admitting vector candidate generation.
- Q7 — Decision-record storage/lifecycle/retention and replay exposure.
- Q8 — MCP public/admin split (deferred, separate problem).
