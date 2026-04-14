# Phase 5 Specification: Toronto ICS Inventory Rollout

**Spec**: 005-ics-integration / Phase 5 | **Date**: 2026-04-13 | **Status**: Draft — PROVISIONAL
**Parent**: `specs/005-ics-integration/plan.md`

> **PROVISIONAL**: This spec captures current thinking but depends on learnings from
> Phases 1-4. Cohort strategies and success thresholds will be revised based on actual
> ingest/export behavior observed in earlier phases. The value of specifying Phase 5
> now is that the Toronto inventory research (89 ICS feeds, platform fingerprints,
> high-value source list) directly informs Phase 1 parser requirements (which feed
> shapes must work), Phase 2 pagination sizing (feeds with 2,900-6,558 events), and
> Phase 4 interop fixture selection (which platforms to prioritize).
**Goal**: Turn the Toronto ICS source inventory into a staged rollout program with measurable onboarding outcomes.

## Context

The plan includes a detailed Toronto inventory (overlap, SEL-only, net-new, non-starters), but inventory data alone is not operational execution. Phase 5 converts that research into owned rollout work and measurable outcomes.

### What Exists Today

| Component | Status | Reference |
|---|---|---|
| Toronto ICS Source Inventory analysis in plan | Present | `specs/005-ics-integration/plan.md:457-554` |
| ICS ingest/export/recurrence capabilities | Delivered by prior phases | `spec-phase1.md` to `spec-phase4.md` |
| Interop fixtures and runbook baseline | Delivered by Phase 4 | `spec-phase4.md` |
| Rollout manifest + cohort tracking artifacts | Missing (Phase 5 deliverable) | — |

### What This Phase Delivers

1. Inventory manifest in machine-readable form.
2. Cohort strategy for staged onboarding.
3. Bead-backed onboarding work graph.
4. Outcome taxonomy and reporting template.
5. First staged cohort execution + metrics.

### Non-Goals

- Rewriting phase 1-4 technical architecture.
- Bulk onboarding all 90+ net-new sources in one phase.
- Building a new dashboard application.

### Design Constraint Reminders

- Prefer small staged cohorts over big-bang rollout.
- Keep status terms standardized (no ad-hoc labels).
- Every onboarding decision should be traceable to source evidence.

### Out-of-Phase Guardrails

- Do not redesign ingest/export/recurrence architecture in Phase 5; create follow-up
  beads against Phases 1-3 if rollout uncovers core behavior gaps.
- Do not expand Phase 5 into UI/dashboard feature work.
- Keep Phase 5 focused on inventory operationalization, cohort execution, and evidence capture.

---

## User Scenarios & Testing

### User Story 1 - Inventory to Manifest (Priority: P1)

As a maintainer, I can query the inventory in a structured artifact.

**Independent Test**: Validate manifest schema and counts against inventory totals.

**Acceptance Scenarios**:

1. **Given** inventory categories, **When** manifest is generated, **Then** each source has a unique key and category label.
2. **Given** existing SEL source overlap entries, **When** manifest is reviewed, **Then** overlap/non-overlap flags are explicit.

### User Story 2 - Cohort Rollout (Priority: P1)

As an operator, I can execute onboarding in priority cohorts.

**Independent Test**: Run first cohort through staging workflow and capture outcomes.

**Acceptance Scenarios**:

1. **Given** high-value net-new cohort, **When** staging onboarding is attempted, **Then** outcomes are recorded using the standard taxonomy.
2. **Given** blocked sources, **When** cohort closes, **Then** block reasons and next actions are documented.

### User Story 3 - Repeatable Reporting (Priority: P2)

As a project lead, I can evaluate rollout progress objectively.

**Independent Test**: Generate one rollout report from cohort execution data.

**Acceptance Scenarios**:

1. **Given** cohort results, **When** report is generated, **Then** it includes attempted/onboarded/blocked/deferred counts.
2. **Given** onboarded sources, **When** report is generated, **Then** it includes ingestion quality notes and maintenance risk markers.

---

## Technical Design

### Package/Layout

```text
specs/005-ics-integration/
  toronto-ics-manifest.json            # NEW - structured source inventory (kept in specs/
                                       # because tightly coupled to spec; not a runtime artifact)
  toronto-rollout-cohorts.md           # NEW - cohort plan and rationale
  toronto-rollout-report-template.md   # NEW - standard reporting template
  spec-phase5.md                       # THIS FILE
```

### Data Structures

```json
{
  "source_key": "string",
  "display_name": "string",
  "category": "overlap|sel_only|net_new|non_starter",
  "feed_type": "string",
  "priority": "high|medium|low",
  "rollout_status": "planned|onboarded|deferred|blocked|non_starter",
  "notes": "string"
}
```

**Priority mapping to beads**: `high` -> P1, `medium` -> P2, `low` -> P3. Task 3
uses this mapping when creating onboarding beads from manifest rows.

### Error Handling

- Manifest schema violations fail validation checks.
- Unknown rollout statuses are rejected.

### Security Model

- No secrets in manifests/reports.
- External URLs are metadata only; no automatic live fetch in manifest generation step.

---

## Implementation Tasks

### Task 1: Create Toronto manifest artifact

**What**: Convert inventory table data into `toronto-ics-manifest.json`.

**Test**: schema and count validation.

**Acceptance**: manifest rows map cleanly to all known inventory categories.

### Task 2: Define rollout cohorts and priorities

**What**: Create `toronto-rollout-cohorts.md` with explicit cohort boundaries and rationale.

**Test**: manual review against runbook constraints.

**Acceptance**: each cohort has entry/exit criteria and expected throughput.

### Task 3: Create onboarding beads from manifest

**What**: Generate bead tasks using manifest rows and cohort ordering.

**Test**: bead count parity with selected cohort manifest rows.

**Acceptance**: every in-scope source has a tracked onboarding bead.

### Task 4: Add standardized rollout report template

**What**: Create `toronto-rollout-report-template.md` with metric and decision sections.

**Test**: fill template using one sample cohort.

**Acceptance**: report format supports objective comparison across cohorts.

### Task 5: Execute first staging cohort + publish report

**What**: Run the first prioritized cohort (5-10 high-value net-new sources from manifest)
in staging and produce a filled rollout report. Success threshold: >= 80% onboarded;
remaining sources documented as blocked/deferred with reasons and next actions.

**Test**: staging execution evidence + report artifact published as
`specs/005-ics-integration/toronto-rollout-report-cohort1.md`.

**Acceptance**: cohort outcomes recorded with standardized taxonomy; report includes
attempted/onboarded/blocked/deferred counts, ingestion quality notes, median setup time,
and maintenance risk markers.

### Task 6: Feed outcomes back into integration docs

**What**: Update platform heuristics and ICS runbook with observed Toronto rollout lessons.

**Test**: docs consistency review.

**Acceptance**: docs include concrete examples and revised guidance from rollout evidence.

---

## Success Criteria

1. Toronto inventory exists as a validated manifest artifact.
2. At least one rollout cohort completes with published metrics.
3. Source onboarding work is tracked consistently via beads.
4. Integration docs improve from real rollout evidence.

## Rollback Notes (Phase 5)

- If a cohort rollout introduces unstable source configs, revert affected sources to
  disabled state and preserve manifest/report evidence for retry planning.
- If manifest schema changes break automation, roll back to previous manifest version
  and regenerate beads from the last validated artifact.
