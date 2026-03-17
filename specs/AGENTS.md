# Spec-Driven Development Guide

Agent instructions for creating and managing specifications in this project.
Read this file before writing any plan, spec, or task document.

## Overview

Every feature flows through a pipeline:

```
Discovery → Plan → Spec → Review → Tasks (beads) → Implementation → Close
```

Specs are the **source of intent**. Code implements what the spec says. If the spec
is wrong, fix the spec first, then fix the code. Never let code drift from spec
without updating the spec.

## Directory Convention

```
specs/
  NNN-feature-name/       # NNN = zero-padded sequence number
    plan.md               # Architecture, phases, dependencies, risks
    spec.md               # Or spec-phaseN.md for phased delivery
    tasks.md              # Optional — only if NOT using beads for tracking
    research.md           # Optional — exploratory findings
    data-model.md         # Optional — schema/ERD if complex
    checklists/           # Optional — verification checklists
    contracts/            # Optional — SHACL shapes, test contracts
```

**Numbering**: Check existing directories (`ls specs/`) and use the next available
number. Current: 001, 002, 003, 004.

**Naming**: Use kebab-case for directory names. Keep them short but descriptive.

## Breaking Work into Phases

### When to Phase

Not every feature needs phases. Use this decision tree:

1. **Can it be specified in < 400 lines of spec?** → Single spec, no phases needed.
2. **Does it have independent deliverables?** → Phase per deliverable.
3. **Does it touch > 3 existing packages?** → Phase by integration surface.
4. **Does it require new infrastructure before domain logic?** → Infra phase first.

**Rule of thumb**: If a spec has more than 8 implementation tasks, it's too big.
Split into phases. Each phase should have 4-8 tasks and be independently testable.

### How to Phase

Phases should be **vertically sliced** — each phase delivers working, testable
functionality, not horizontal layers. Bad phasing builds all the infrastructure in
Phase 1 and all the features in Phase 2. Good phasing delivers a thin but complete
slice in each phase.

**Bad phasing** (horizontal):
```
Phase 1: Build all storage layers
Phase 2: Build all domain logic
Phase 3: Build all API handlers
Phase 4: Build all tests
```

**Good phasing** (vertical):
```
Phase 1: Decision journal + one MCP tool (end-to-end: storage → domain → tool → test)
Phase 2: Review automation + CLI (builds on Phase 1 journal)
Phase 3: Metrics monitoring agent (independent vertical slice)
```

Each phase gets its own `spec-phaseN.md`. The plan defines all phases at a high level;
each phase spec is written only when the previous phase is in progress or complete.
This avoids specifying details for Phase 3 when Phase 1 hasn't validated the
architecture yet.

### The Plan → Spec → Beads Pipeline

This is where things go wrong if the documents are too large. The failure mode:

```
Big plan (1400 lines) → Big spec (1200 lines) → 20 review issues → 12 beads
                                                  ↑
                                          Problems compound at scale
```

**Prevent this with size gates:**

| Document | Max lines | If exceeded |
|----------|-----------|-------------|
| Plan | ~800 | Split into plan.md (architecture + phases overview) and separate deep-dive docs (data-model.md, security-model.md) |
| Spec (per phase) | ~600 | Phase is too big — split into sub-phases |
| Task count per spec | 8 | Split the phase |

**The review checkpoint is mandatory.** Never go from spec → beads without review.
The cost of finding a naming inconsistency during implementation (touching 8 files)
is 10x the cost of finding it during review (find-replace in one doc).

### Spec Sizing: The "Two Screens" Test

A single spec section (e.g., one user story, one task definition, one struct block)
should fit on two terminal screens (~100 lines). If a section is longer, it's doing
too much:

- A user story with 8 acceptance scenarios → split into two stories
- A Technical Design section with 15 structs → split by package, one subsection each
- An Implementation Tasks section with 12 tasks → the phase is too big

### When NOT to Write a Spec

Small, well-understood changes don't need specs:

- Bug fixes with a clear reproduction case → bead + fix
- Adding a field to an existing struct → bead + PR
- Config changes → bead + PR
- Scraper source configs → `/configure-source` handles it

**Spec threshold**: If the work requires design decisions that affect more than one
package, or introduces a new package, or changes an API contract — write a spec.

### Phase Boundaries and Dependencies

Each phase must define:

1. **Entry criteria** — what must be true before this phase starts
   (prior phase delivered, specific infra exists, etc.)
2. **Exit criteria** — measurable "done" conditions (tests pass, CI green,
   quantitative success metric)
3. **Interface contracts** — what this phase exposes that later phases depend on
   (function signatures, file formats, MCP tool schemas)

Later phases SHOULD depend only on the **interface contracts**, not the implementation
details, of earlier phases. This lets you revise Phase 1 internals without
invalidating Phase 2's spec.

### Progressive Spec Detail

Don't over-specify future phases. The appropriate detail level:

| Phase distance | Detail level | Example |
|---------------|--------------|---------|
| Current phase | Full spec: structs, schemas, acceptance tests, task list | spec-phase1.md |
| Next phase | User stories + key interfaces + known constraints | 1-2 paragraphs per story in plan.md |
| Later phases | Goals + deliverables + dependencies | Bullet list in plan.md |

This avoids wasting effort specifying Phase 4 details that will change after
Phase 1-3 learnings.

## Document Lifecycle

### 1. Plan (`plan.md`)

The plan is the architectural blueprint. Write it before any code exists.

**Required sections:**

```markdown
# Plan: <Feature Name>

**Spec**: NNN-feature-name | **Date**: YYYY-MM-DD | **Status**: Planning
**Goal**: One sentence — what does success look like?

## Vision              — Why this feature exists, what it enables
## Current State       — What exists today (table format), what's missing
## Architecture        — Diagrams, component relationships, data flow
## Design Constraints  — Non-negotiable rules agents must follow
## Component Design    — Package layout, struct definitions, interfaces
## Implementation Phases — Ordered delivery plan with clear phase gates
## Risks and Mitigations — What could go wrong, contingency plans
## Security            — Threat model, trust boundaries, defense layers
## Open Questions      — Unresolved decisions (tracked, not ignored)
```

**Critical rules for plans:**

- **Ground every claim in the codebase.** Before writing "the system has X", grep
  for it. Before writing "function Y accepts Z", read the source. Agents hallucinate
  when they assume — verify everything.
- **Include a "Current State" table** mapping capabilities to their actual status and
  code locations. This is the single biggest source of spec drift.
- **List exact file paths and line numbers** for existing code being referenced. These
  go stale but are invaluable during initial writing and review.
- **Architecture diagrams** use ASCII art (renders everywhere, no tooling deps).

### 2. Spec (`spec.md` or `spec-phaseN.md`)

The spec defines what to build with testable acceptance criteria. Use `spec-phaseN.md`
when a plan has multiple implementation phases.

**Required sections:**

```markdown
# [Phase N] Specification: <Feature Name>

**Spec**: NNN-feature-name [/ Phase N] | **Date**: YYYY-MM-DD | **Status**: Draft
**Parent**: specs/NNN-feature-name/plan.md
**Goal**: Measurable success criterion (quantitative where possible)

## Context
  ### What Exists Today    — Table of components, status, code references
  ### What This Phase Delivers — Numbered list of deliverables
  ### Non-Goals            — Explicitly out of scope (with phase deferral notes)
  ### Design Constraint Reminders — Carried forward from plan, NOT repeated in full

## User Scenarios & Testing
  ### User Story N — <Title> (Priority: PN)
    **Independent Test**: One sentence describing the test setup
    **Acceptance Scenarios**: Given/When/Then format, numbered

## Technical Design
  ### Package Layout       — Directory tree with descriptions
  ### Data Structures      — Go struct definitions with field comments
  ### Interfaces           — Method signatures for key boundaries
  ### MCP/CLI/API Schemas  — Tool definitions, command signatures, endpoint contracts
  ### Error Handling       — How errors flow, RFC 7807 mapping if applicable
  ### Security Model       — Trust boundaries, input validation, sanitization

## Implementation Tasks
  ### Task N: <Title>
    **What**: What to build
    **Test**: How to verify it
    **Acceptance**: The gate for "done"

## Configuration         — New config fields, env vars, defaults
## Success Criteria      — Quantitative metrics for the phase
## Open Questions        — Unresolved items for this phase specifically
```

**Critical rules for specs:**

- **Every user story needs acceptance scenarios in Given/When/Then format.**
  These become test cases. Vague stories produce vague implementations.
- **Every task needs a "Test" and "Acceptance" field.** If you can't define how to
  verify a task, the task isn't well enough understood to implement.
- **Data structures must include Go type definitions** — not prose descriptions of
  what fields exist. Show the actual struct.
- **MCP tool schemas must include the full JSON input/output** — not just a
  description. Show the exact tool definition and example call/response.
- **Non-goals are as important as goals.** Explicitly state what is deferred and to
  which phase. This prevents scope creep during implementation.

### 3. Tasks → Beads

**Do NOT maintain tasks.md for new features.** Use beads (`bd`) for all task tracking.
The older specs (001, 003) have `tasks.md` files from before beads was adopted — these
are historical artifacts.

**The spec's Implementation Tasks section is the bridge.** Every task in the spec
becomes exactly one bead. The spec task gives the bead its title, description, test
criteria, and acceptance gate. If a task in the spec is too vague to create a bead
from, the spec isn't ready — go back and add detail.

**Verification step: count check.** After creating all beads, verify:
```bash
# Count tasks in spec
grep -c '^### Task' specs/NNN-feature-name/spec-phaseN.md

# Count beads created (check the epic's dependency count)
bd show <epic-id>
```
These numbers must match. If they don't, a task was missed.

**Workflow for converting spec tasks to beads:**

```bash
# 1. Create an epic for the feature/phase
bd create --title="Phase N: <Feature> — <Summary>" \
  --description="Epic for spec-phaseN.md. <What it delivers>. Spec: specs/NNN-feature-name/spec-phaseN.md" \
  --type=feature --priority=1

# 2. Create a bead for each task
bd create --title="<Task title from spec>" \
  --description="<What + Test + Acceptance from spec>" \
  --type=task --priority=1

# 3. Set up dependency graph
bd dep add <downstream-bead> <upstream-bead>

# 4. Make epic depend on all tasks
bd dep add <epic-bead> <task-bead>   # repeat for each task
```

**Tips:**
- Use a subagent (`subagent_type: "basic"`) to create many beads in batch
- Use a second subagent to wire up dependencies after all beads exist
- Map the dependency graph from the spec's task ordering before creating beads
- Include the spec task number in the bead title or description for traceability

**When implementation discovers new tasks:**

During implementation, you will discover tasks that the spec didn't anticipate.
This is normal and expected. Handle them as follows:

1. **Small (< 1 hour)**: Fold into the current bead's scope. Note it in the bead.
2. **Medium (1-4 hours)**: Create a new bead, add it as a dependency of the epic,
   and add a brief note to the spec's Implementation Tasks section.
3. **Large (> 4 hours)**: This is a sign the phase was under-specified. Create the
   bead, add it to the epic, AND update the spec with a new Task section. Consider
   whether the phase needs to be split.

**Never let discovered work go untracked.** If you notice something that needs doing
but you're in the middle of another task, create the bead immediately with a
description of what you noticed and why it matters. You can flesh it out later.

## Review Process

### Why Review Matters

Spec 004 required two full review passes (20 issues total) before implementation.
Common problems that reviews catch:

| Category | Example from 004 | How to prevent |
|----------|-------------------|----------------|
| **Naming drift** | `source_name` vs `source_id` inconsistent across doc | Use grep to verify every identifier appears the same way everywhere |
| **Codebase divergence** | Spec said "5 warning codes" but codebase has 12 | Verify counts/enums by reading actual source, not memory |
| **Missing security model** | No threat analysis for memory system in first draft | Always include a Security section — never skip it |
| **Ambiguous error semantics** | "Returns error" vs "records escalation" unclear | Distinguish structural errors (malformed input → error) from semantic failures (policy violation → alternative behavior) |
| **Schema mismatches** | `[]int` for IDs when they're actually strings | Verify every type against the actual codebase type |
| **Inconsistent enum values** | `outcome = 'success'` in one place, `'confirmed'` in another | Define enums once, reference by name everywhere |
| **Missing data lifecycle** | No mention of where `data/` lives or how it's backed up | Every new storage location needs: where, persistence model, backup strategy, gitignored or committed |
| **Vague validation rules** | "Input is validated" without specifying what happens on failure | Spell out both the happy path AND the failure path for every validation |

### How to Review

**Self-review checklist (run before requesting external review):**

1. **Naming consistency**: `grep -r "source_name\|source_id" specs/NNN-*/` — pick one,
   use it everywhere. Check field names, JSON keys, CLI flags, env vars.

2. **Codebase grounding**: For every "the system has X" claim, verify with grep/read.
   For every struct field, verify the type matches the actual Go code.

3. **Count verification**: If you say "12 warning codes", list all 12. If you say
   "10 MCP tools", list all 10. Unverified counts are wrong counts.

4. **Security section exists**: Every spec that touches data storage, user input,
   or agent output MUST have a security section. Threat model format:
   - What are the trust boundaries?
   - What can a malicious input do?
   - What are the defense layers?

5. **Error path completeness**: For every operation, answer: what happens when it
   fails? Is it an error return? A fallback behavior? An escalation? A log entry?
   Ambiguous failure modes cause the worst implementation bugs.

6. **Data lifecycle**: For every new file/directory/table:
   - Is it gitignored or committed?
   - Is it runtime-local or shared?
   - How is it backed up?
   - What happens if it's corrupted or deleted?
   - Who owns cleanup/rotation?

7. **Non-goal drift**: Re-read the Non-Goals section. Has any task description crept
   into delivering a non-goal? This is subtle — watch for phrases like "later phases
   may..." becoming implicit requirements.

8. **Struct/schema completeness**: Every Go struct in the spec should be copy-pasteable
   into source code. Missing fields, wrong types, or unclear JSON tags will slow
   implementation.

9. **Cross-reference integrity**: If the spec references the plan, verify the plan
   still says what you think it says. If the plan was updated after the spec was
   written, the spec may be stale.

10. **Accept header for validation semantics**: Distinguish clearly between:
    - **Structural validation** → malformed input → return error (400/422)
    - **Semantic/policy validation** → valid input, wrong decision → alternative behavior
      (escalation, fallback, degraded mode) — NOT an error return

**External review protocol:**

- Export the spec to a review-capable LLM (GPT-5+, Claude Opus, etc.)
- Ask for a structured review with numbered issues
- Track issues in a scratch file (NOT committed — delete after resolving)
- Resolve all issues in the spec before creating beads
- Do NOT create `review-feedback.md` files in the repo — these are working artifacts

### Review Prompting Template

When requesting an external review, provide this context:

```
Review this specification for a Go backend project.

Project context:
- Go 1.24+, PostgreSQL 16+/PostGIS, SQLc, River job queue
- SEL (Shared Events Library) — linked data, Schema.org, CC0
- Agents operate via MCP tools and OpenCode commands
- File-based storage for operational data (no external vector DB)

Review for:
1. Internal consistency (naming, types, enums used the same way everywhere)
2. Codebase grounding (does the spec match what actually exists?)
3. Security gaps (missing threat model, unvalidated input, trust boundaries)
4. Error path completeness (what happens on every failure mode?)
5. Testability (can every acceptance criterion be verified automatically?)
6. Ambiguity (would two different implementers build the same thing?)

Format: numbered issues, each with severity (Critical/Major/Minor),
location (section + line if possible), and suggested fix.
```

## Writing Quality Standards

### Be Precise, Not Verbose

Bad:
> The system should validate the input and handle errors appropriately.

Good:
> `PolicyValidate` checks `action ∈ AllowedActions` and `confidence ≥ MinConfidence`.
> Structural failures (wrong Go types, missing required fields) return
> `fmt.Errorf("policy: %w", err)`. Semantic failures (action not in allowed set,
> confidence too low) convert the decision to `action: "escalate"` and record an
> escalation entry in the journal. The original review entry is NOT modified.

### Show, Don't Describe

Bad:
> The decision struct has fields for the action, confidence, and reasoning.

Good:
```go
type Decision struct {
    ID             string         `json:"id"`             // "dec-{nanoID}"
    Timestamp      time.Time      `json:"timestamp"`
    Trigger        Trigger        `json:"trigger"`
    Action         string         `json:"action"`         // approve|reject|fix|merge|add-occurrence|escalate
    Confidence     float64        `json:"confidence"`     // 0.0–1.0
    Reasoning      string         `json:"reasoning"`      // Full chain, not just conclusion
    MemoryRefs     []string       `json:"memory_refs"`    // IDs of precedents/rules consulted
    DecisionSource string         `json:"decision_source"` // rule|precedent|escalation
}
```

### Use Tables for Status/Inventory

Bad:
> The system has several warning codes including reversed dates, potential
> duplicates, missing descriptions, and others.

Good:

| Warning Code | Source | Auto-resolvable? |
|---|---|---|
| `reversed_dates_timezone_likely` | validation | Yes (timezone fix) |
| `reversed_dates_corrected_needs_review` | validation | No (needs human check) |
| `potential_duplicate` | ingest | Conditional (single match = auto) |
| `near_duplicate_of_new_event` | ingest | No (always escalate Phase 1) |
| ... | ... | ... |

### Link to Code, Not to Memory

Bad:
> The review queue has methods for approving and rejecting events.

Good:
> Review queue operations in `internal/domain/events/admin_service.go`:
> - `ApproveReviewEntry(ctx, entryID, adminUser)` — line 156
> - `RejectReviewEntry(ctx, entryID, adminUser, reason)` — line 189
> - `FixReviewEntry(ctx, entryID, adminUser, corrections)` — line 223

Line numbers go stale, but they're still valuable at write time and during review.
Update them if you're editing the spec and notice drift.

## Common Mistakes

### 1. Skipping the security section

Every spec that handles external input, stores data, or runs agent code needs a
security section. "It's internal only" is not an excuse — agents process untrusted
data from scraped websites.

Minimum viable security section:
- Trust boundaries (what's trusted vs untrusted input?)
- Input sanitization (how is untrusted data cleaned?)
- Output constraints (what can agents NOT do?)
- Defense layers (what if one layer fails?)

### 2. Writing prose instead of schemas

If the implementation needs a Go struct, the spec should show the Go struct.
If the implementation needs a JSON schema, the spec should show the JSON schema.
Prose descriptions of data structures are ambiguous and cause type mismatches.

### 3. Inconsistent naming across documents

The plan says `source_name`, the spec says `source_id`, the MCP tool says `source`.
Pick one. Grep for all variants. Fix them all before review.

**Prevention**: After writing a spec, run:
```bash
# Find all identifier-like patterns and check for variants
grep -rn 'source_name\|source_id\|sourceId\|sourceName' specs/NNN-*/
```

### 4. Forgetting the failure path

For every operation, answer:
- What if the input is malformed? (structural error → return error)
- What if the input is valid but the operation can't proceed? (semantic failure → what?)
- What if the underlying storage is unavailable? (infra error → what?)
- What if a concurrent operation conflicts? (race → what?)

### 5. Letting non-goals creep in

The spec says "automated graduation is Phase 5" but Task 4 description mentions
"auto-graduate when threshold is reached". That's scope creep. Non-goals must stay
non-goals until the spec is explicitly revised.

### 6. Not verifying enums against the codebase

The spec says there are 5 review actions but the codebase has added a 6th since the
spec was written. Always `grep` for enum values before listing them.

### 7. Omitting data locality decisions

Every new storage location (`data/`, `configs/`, temp files) needs:
- **Where**: exact path
- **Committed or gitignored?**: runtime data is ALWAYS gitignored
- **Persistence**: survives restart? survives redeploy? survives disk loss?
- **Backup**: how? (script, cron, manual)
- **Corruption recovery**: what happens if files are damaged?

### 8. Review feedback files in the repo

Review feedback is a working artifact. Track issues, resolve them, delete the file.
Do not commit `review-feedback.md` files — they add noise and go stale immediately.

## Updating Specs After Implementation

Specs are living documents. Update them when:

- Implementation deviates from spec (update spec to match reality)
- A phase is completed (update Status field, add completion notes)
- New tasks are discovered during implementation (add to spec, create beads)
- Open questions are resolved (move from Open Questions to the relevant section)

**Status values**: `Planning` → `Draft` → `In Review` → `Approved` → `In Progress`
→ `Delivered`

When updating, also update the plan's phase status table and any cross-references
in other spec documents.
