# Entity Identity & Adjudication — Discovery Findings & Design Context

**Status**: Draft (Design context — not yet normative)
**Date**: 2026-09-10
**Owner**: design spike on kanban `t_f5284b8f`
**Normative spec**: `specs/007-entity-identity-adjudication/` (to be written after review)
**Related**: `docs/design/duplicate-detection.md`, `docs/design/duplicate-review-scenarios.md`,
`docs/architecture/event-review-workflow.md`, `docs/interop/knowledge-graphs.md`,
`docs/integration/tg-review.md`, `specs/004-agentic-maintainer/`.

> This document is **context/understanding**: what exists today, what is broken,
> the design tensions, and the decisions that must be made. It is not a build spec.
> The normative "how it must be made" lives in `specs/007-*`.

---

## 1. Purpose

Unify entity identity across the SEL node so that duplicate real-world entities
(places, organizations, eventually events) are detected, scored, and resolved with
**deterministic rules where truth is knowable** and **LLM adjudication where judgment
is needed** — with every destructive action reversible and fully provenance-tracked.

Target operating model (operator decision, 2026-09-10):
- Pre-alpha, no backward-compatibility constraints — architecture quality is the priority.
- Nodes must be runnable **cheaply and with low maintenance**, including by other cities.
- The **event review flow is already almost entirely LLM-run**; that is the intended
  pattern and identity adjudication must follow it.

---

## 2. Current State — two disconnected identity systems

The node today has **two identity systems that never meet**:

| System | What it is | Where | Wired? |
|---|---|---|---|
| **A. Internal dedup/merge** | ULID identity + `dedup_hash` + pg_trgm similarity + `merged_into_id` + `event_not_duplicates`; review queue | `internal/domain/events/`, `create_event_core.go`, `events_repository.go` | **Wired, active** |
| **B. External knowledge-graph identifiers** | `entity_identifiers` (sameAs edges) + `knowledge_graph_authorities` + `is_canonical` | `internal/kg/`, `migrations/000030` | **Write-only; never consulted** |

Grounded facts (from code inventory, 2026-09-10):

- `entity_identifiers` is written by `internal/kg/reconciliation.go:332` (`storeIdentifier`,
  from `ReconcileEntity`) and `internal/jobs/workers.go:422` (enrichment sameAs). It has
  **zero Go readers**; the only reads are raw SQL joins for CLI stats
  (`cmd/server/cmd/reconcile.go:307,443,571`).
- `knowledge_graph_authorities` is seeded (`000030:62-67`) but its queries
  (`GetActiveAuthorities`, `GetAuthoritiesForDomain`, `GetAuthorityByCode`) have **no Go
  callers**. `trust_level`/`priority_order` are never read by logic; reconciliation
  hardcodes `authority_code = "artsdata"` (`internal/kg/reconciliation.go:292`).
- `is_canonical` is set as a per-row predicate `method == "auto_high"`
  (`internal/kg/reconciliation.go:330`) — no primary-slot semantics, no demotion.
- The unique key `UNIQUE(entity_type, entity_id, authority_code, identifier_uri)`
  (`000030:36`) permits **multiple canonical rows per authority** — observed on staging:
  172 `is_canonical=true` artsdata rows across only 144 places (~28 places with >1).
- Event dedup identity is name+venue+startDate (`internal/domain/events/dedup.go`);
  external IDs are **not** part of the hash. `source_event_id` is per-source and does
  not federate.

### Confirmed gaps (inventory)

1. **Identifiers are never reassigned on merge.** `MergePlaces`
   (`events_repository.go:1717`), `MergeOrganizations` (`:1839`), `MergeEvents` (`:2125`),
   `Consolidate` (`admin_service.go:1562`) leave `entity_identifiers.entity_id` pointing at
   soft-deleted ULIDs → orphaned sameAs edges.
2. **`field_provenance` is never written by application code.** Schema + supersession
   columns exist (`migrations/000002:67-111`), SQLc queries exist, readers exist
   (`provenance/service.go`), but `InsertFieldProvenance`/`SupersedeFieldProvenance` have
   no callers. The JSON-LD provenance surface reads an empty table. (Aspirational.)
3. **Place/org merges create no tombstone**, despite `place_tombstones`/
   `organization_tombstones` existing → no 410 for merged entities.
4. **Event merge omits `deletion_reason`** (`queries/events.sql:78-85`) unlike
   `SoftDeleteEvent` (`:69-76`).
5. **Event merge/consolidate do not reassign `event_sources` or `field_provenance`**
   off the retired event → provenance loss.
6. **`merged_into_id` and `duplicate_of_event_id` are unindexed** (`000026:13-17`).
7. **`event_review_queue.status` has no CHECK**; `superseded` is never written by Go.
8. **`ids` roles `foreign`/`alias` are unused**; only `RoleCanonical` is enforced
   (`validation.go:604`).
9. **No place/org review surface.** Place/org duplicates only appear as
   `place_possible_duplicate`/`org_possible_duplicate` warnings on the *event* queue;
   there is no admin "not a duplicate" path for place/org pairs (only `event_not_duplicates`
   exists, `migrations/000027`).
10. **No LLM adjudication exists.** The only MCP duplicate surface is a static prompt
    (`internal/mcp/prompts/templates.go:91-108`) disconnected from warnings/data.
    `internal/llmsafe` is used only by scraper inspection (`scraper/inspect.go:124,201`).

---

## 3. The existing LLM-adjudication pattern (must design *with* it)

**Operational (today):** `server review` CLI (`docs/integration/tg-review.md`) +
`event_review_queue` (`migrations/000025`) + admin API. An LLM agent triages by
source/name in batches; approve/reject/fix/merge/consolidate; decisions record
`reviewed_by` + notes. Skills: `skills/togather-review/SKILL.md`,
`skills/togather-server-ops/SKILL.md`.

**Deployment reality (corrected 2026-09-10).** Spec 004 `agentic-maintainer` is **not
implemented in any form**. The live pattern is an **external agent** (a Hermes agent,
cron-scheduled) that drives the admin API through the `skills/` playbooks. This trial is
working well, and the operator considers the **API-driven external assistant the viable
near-term shape**; the built-in maintainer remains a long-term possibility.

**Design implication — the interface is the seam.** Because the LLM lives *outside* the
server, the design must expose a stable **agent-facing contract** (admin API + `server
review` CLI + MCP tools, plus rich context payloads and an action set) and keep judgment
out of the server:

- **Server owns**: detection, candidate generation, scoring, invariants, execution,
  reversibility, audit, and *policy validation of the agent's proposed action*.
- **Agent owns**: reasoning and the choice among allowed actions, using the context the
  server provides.
- **No prompts or LLM calls in the server.** The server never embeds a model. The same
  contract must serve both drivers (external API agent now; in-process maintainer later),
  so neither is baked into the data model.

**Guardrail principles to adopt server-side from spec 004** (design principles, not an
existing implementation to reuse):

- **Constrained action set**: `approve | reject | fix | merge | add-occurrence | escalate`.
- **Policy validation is code, not prompt**: validate the agent's action against the
  allowed set, required fields, confidence threshold, and red-lines *before* executing;
  violation ⇒ convert to `escalate`/reject. Never trust the model's self-report.
- **Red-line rules** always escalate (e.g. `low_confidence` + unknown source, ambiguous
  near-duplicate target).
- **Memory-citation requirement**: require a decision to cite ≥1 prior decision/rule;
  empty refs ⇒ automatic escalation.
- **Decision journal / record**: append-only, durable, auditable. Learning flywheel
  (precedent → confirmed precedent → rule) is a later phase.

**Trust boundary / injection.** Untrusted scraped content flows into the agent's context
via API responses. The server should delimit/sanitize untrusted fields in the payloads the
agent consumes (`internal/llmsafe` boundaries are available; a `REVIEW` tag is contemplated
but unused). Server-side encoding cannot fully protect the external agent's prompt, so the
skill/agent must also treat payload text as data — but the server must never let an
injection escalate into an unvalidated destructive action (that is the policy wrapper's job).

**Open tension:** spec 004 rejects knowledge graphs and vector stores
(`specs/004-agentic-maintainer/plan.md:191-193`), while `docs/interop/knowledge-graphs.md`
describes multi-graph reconciliation. The new spec must state explicitly whether external KG
identity is in scope and whether semantic/vector candidate generation is admitted.

**Injection defense:** `internal/llmsafe/boundary.go` provides nonce-bounded untrusted-content
wrapping; identity/adjudication prompts must use it (a `REVIEW` tag is contemplated but
currently unused).

---

## 4. Design tensions & decisions requiring sign-off

These are the pivotal choices. My recommendation is in **bold**.

### D1 — Is external KG identity (System B) the identity backbone, or a parallel evidence source?
- **Recommendation: backbone for *candidate generation and linkage*, evidence only for
  *destructive action*.** External IDs become a high-precision blocking/join key that feeds
  the existing dedup/review pipeline; they do **not** by themselves auto-merge in v1.
- Rationale: recon false positives, `owl:sameAs` non-transitivity, and merge irreversibility.

### D2 — Canonical model: boolean flag vs authority-scoped primary slot
- **Recommendation: SEL ULID is the canonical internal identity; external URIs are aliases.
  Adopt exactly one *primary* identifier per `(entity, authority)`, elected by a rank, with
  `is_canonical` redefined as "this row holds the authority's primary slot."**
- Rank: `manual` override > method rank (`auto_high`/`manual`/`imported` > `auto_low`) >
  confidence > authority `trust_level`/`priority_order` > recency. Keep all identifiers
  (the documented "keep-all" strategy); store observations; supersede rather than silently
  overwrite. Enforce with a partial unique index `WHERE is_primary`.
- No single global external canonical; emit each authority's primary as `sameAs`.

### D3 — v1 posture
- **Recommendation: additive first — link + annotate + review; no auto-merge on external IDs
  in v1.** Stage: annotate → propose (LLM adjudicates, executor applies) → auto-merge only
  when corroborated, low fan-out, and reversible.

### D4 — Where does identity adjudication surface?
- Options: (a) reuse `event_review_queue`, (b) new generic identity review table,
  (c) separate place/org review queue.
- **Recommendation: generalize the review entity model** so events/places/orgs share the
  adjudication + decision-journal rails, rather than bolting place/org review onto the event
  queue (current behavior). Exact shape is a plan-level decision.

### D5 — Driver model and where adjudication memory lives
- **Recommendation: external-agent-first.** Expose a stable agent-facing contract (admin
  API + CLI + MCP + context schema + action set) and keep all judgment outside the server.
  Adopt 004's guardrail *principles* (constrained actions, code-not-prompt policy
  validation, red-lines, memory-citation) server-side, but do **not** assume an in-process
  LLM or reuse an unbuilt framework. The contract must be driver-agnostic so the external
  agent (now) and a future built-in maintainer both use it.
- **Recommendation on memory: server-side durable decision records are authoritative.**
  Store an identity-decision record (action, rationale refs, actor, reversibility trail)
  alongside the review queue; agent-side reasoning/memory is allowed but never the source
  of truth, so a node can swap or drop its agent without losing audit or precedent. The
  server must hand the agent rich context (candidates, trust, provenance, prior decisions).

### D6 — Candidate generation: fuzzy-only, exact-ID pre-pass, and/or vector?
- **Recommendation: exact-ID pre-pass + existing pg_trgm for v1; defer vector.** 004 permits
  pgvector only if brute force becomes too slow. Revisit when volume demands it.

---

## 5. Risk register (initial)

| # | Risk | Severity | Mitigation direction |
|---|------|----------|----------------------|
| R1 | Recon false positive → wrong identity asserted as truth | High | Evidence-only in v1; corroboration rule before any destructive action |
| R2 | `owl:sameAs` non-transitivity → wrong transitive merges | High | No cross-authority transitivity without corroboration; authority-scoped primaries |
| R3 | Auto-merge blast radius (place/org shared by many events) | High | No external-ID auto-merge in v1; reversibility + fan-out caps |
| R4 | Enrichment feedback loop → runaway consolidation | High | One-way enrichment; adjudication cannot be triggered by its own sameAs output without corroboration |
| R5 | Orphaned identifiers on merge (existing bug) | Med | Reassign/union identifiers on all merge paths; add regression tests |
| R6 | Irreversibility of merge | High | Provenance + soft-delete + explicit undo/restore; tombstone every merge |
| R7 | Prompt injection via scraped entity names/descriptions reaching the external agent | High | Delimit/sanitize untrusted fields in the API payloads the agent consumes; server-side policy wrapper prevents any injection from becoming an unvalidated destructive action; skill treats payload text as data |
| R8 | LLM confidently wrong / variance | Med | Forced reasoning order, memory citation, policy validation, escalate-on-doubt |
| R9 | Poisoned precedent compounding bad decisions | High | Automated memory review (004), red-line rules, human confirmation for rule graduation |
| R10 | Stale external IDs (upstream split/merge/redirect) | Med | `observed_at` + TTL + re-verification; don't treat cache as ground truth |
| R11 | Concurrency on merge/chain resolution | Med | Reuse `FOR UPDATE SKIP LOCKED` + chain resolution; make re-election idempotent and cycle-free |

---

## 6. Proposed staged rollout (gates)

1. **Annotate** — external IDs used as candidate evidence + review signals only; zero
   destructive actions. Verifiable: no merge path invoked from identity.
2. **Propose** — LLM adjudicator recommends link/merge with rationale + cited memory;
   deterministic executor applies invariant-checked, reversible actions (link, or
   review-gated merge).
3. **Auto** — narrow, corroborated, low-fan-out auto-merge with reversibility and audit;
   red-lines still escalate.

Each gate is a phase boundary with explicit entry/exit criteria (spec rules
`specs/AGENTS.md:122-135`).

---

## 7. Non-goals (for the first phase)

- No auto-merge on external-ID evidence alone.
- No cross-authority sameAs transitivity.
- No vector/semantic candidate generation yet.
- No unified person entity yet (places + orgs first).
- No public `sameAs` contract commitment beyond emitting authority primaries.
- No change to event quality-warning behavior beyond adding identity evidence.
- **No MCP public/admin split in this effort.** MCP is public-only by design (context-bloat
  control); admin/identity actions are delivered via the **REST admin API + `server` CLI**,
  which is the external agent's contract. A public-vs-admin MCP split is a **separate
  problem** (see Q8).

---

## 8. Open questions

- Q1 — Scope of v1 entity types: places + organizations only, or include events?
- Q2 — Does the identity primary-slot work block the existing tidy-up ticket
  (`t_e522e110`), or land independently as the merge-orphaning bug fix does?
- Q3 — Where does the operator confirm/adjust thresholds and red-lines (config vs rules
  index vs journal)?
- Q4 — Review entity model: generalize `event_review_queue` or new `identity_review` table?
- Q5 — How is un-merge/restore exposed (CLI, admin API, MCP tool)?
- Q6 — Entry criteria for admitting vector candidate generation.
- Q7 — Decision-record storage & lifecycle: new table vs reuse a review-adjacent store;
  retention, and whether it is exposed via API/CLI so any agent can replay precedent.
- Q8 — MCP public/admin split (deferred, separate problem): should admin/identity tools be
  exposed via a second admin MCP, or remain CLI/REST-only for context economy?

---

## 9. Provenance of this document

Compiled from a three-way discovery spike (dedup/review docs; spec conventions +
004-agentic-maintainer; identity/provenance/review code inventory), each grounded in
file:line references, 2026-09-10. Line numbers go stale; re-verify before implementation.
