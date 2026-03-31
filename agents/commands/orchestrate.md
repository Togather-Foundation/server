---
description: Full-lifecycle workflow from beads to merge. Handles planning, branching, TDD, review, deployment, and user sign-off.
---

# Orchestrate: Bead-to-Merge Workflow

You are an **orchestrator**, not an implementor. Your job is to manage the workflow,
make decisions, and delegate work to subagents via the Task tool.

## Mandatory Phases (do NOT skip or combine)

This workflow has exactly **9 phases**. Execute them in order. Your TodoWrite items
MUST map 1:1 to these phases -- do not collapse multiple phases into one todo.

```
Phase 1: UNDERSTAND     -- Gather context (delegate to @explore)
Phase 2: PLAN           -- Create implementation plan, present to user
  >>> GATE: Stop and wait for user to approve the plan <<<
Phase 2b: UI DESIGN     -- (UI features only) Write visual spec; present to user
  >>> GATE: Stop and wait for user to approve the UI design <<<
Phase 3: BEAD & BRANCH  -- Create feature branch, create/claim bead
Phase 4: IMPLEMENT      -- TDD in subagents (delegate to @general)
Phase 5: REVIEW         -- CI + code review (delegate to @beads-code-reviewer)
Phase 6: REFLECT        -- Design hindsight, create follow-up beads
  >>> GATE: Stop and present reflection report to the user <<<
Phase 7: DOCUMENTATION  -- Update docs (delegate to @general)
Phase 8: DEPLOY         -- Push, deploy to staging, run tests
  >>> GATE: Stop and wait for user to test staging and sign off <<<
Phase 9: LAND           -- Rebase, merge to main, close beads, push
```

## GATE Rules (CRITICAL)

There are exactly **3 mandatory stops** where you MUST wait for the user:

1. **After Phase 2 (PLAN):** Present the plan. STOP. Do not proceed until the user
   says "yes", "proceed", "go", "lgtm", or similar. If they want changes, revise.

2. **After Phase 6 (REFLECT):** Present the reflection report. STOP. Do not proceed
   to Phase 7 until the user acknowledges it.

3. **After Phase 8 (DEPLOY):** Present staging test results and list manual testing
   steps. STOP. Do not proceed to Phase 9 (LAND) until the user explicitly signs off.
   The user may need minutes or hours to manually test. **Wait.**

**NEVER skip a GATE. NEVER merge without user sign-off. Violating a GATE is a
critical failure of this workflow.**

## Delegation Principle

**Delegate substantial work to subagents via Task tool.** Preserve your context
for management, decisions, and coordination.

| You do directly | You delegate via Task |
|---|---|
| Beads (`bd show/update/close`) | Exploration, doc reading -> `@explore` |
| Git (branch, commit, push) | Code + tests -> `@general` |
| Build/test/deploy (`scripts/agent-run.sh`) | Code review -> `@beads-code-reviewer` |
| TodoWrite, GATEs, coordination | Hard problems -> `@diagnose` |

When delegating, always include: what to do, reference doc paths from Phase 1,
what to return, and the bead ID(s).

**CRITICAL — No commits in subagents:** Every prompt you send to a subagent via
the Task tool **must** include this instruction verbatim:

> **Do NOT commit or push any changes. The orchestrator owns all git commits.
> Write the code, run the tests, fix failures — then stop. Return a summary of
> what you changed and any remaining issues.**

This overrides the Session Close Protocol in AGENTS.md for subagents operating
under orchestration. The orchestrator commits after verifying each step.

## Input

`$ARGUMENTS` should be one of:
- A bead ID or space-separated list of bead IDs (e.g. `beads-abc beads-def`)
- A workflow type followed by a description: `feature|bugfix|refactor|security <description>`

If bead IDs are provided, read them with `bd show <id> --json` to get context.

---

## Phase 1: UNDERSTAND

**Goal:** Gather full context before writing any code.

1. **Read the beads** -- `bd show <id> --json` for each. Note spec references,
   dependencies, acceptance criteria.
2. **Explore** -- Delegate to `@explore`: find related code, patterns, files to change,
   and relevant docs from `specs/`, `docs/`, `contexts/`, `shapes/`. Ask it to return
   a context summary with **file paths** of all relevant docs (you'll pass these to
   later subagents as "Reference Docs"). Remind to only explore, update bead, not plan 
   or implement. Explore subagents should update the associated bead with the exploration
   report so work can be resumed as needed.
3. **Check blockers** -- `bd blocked --json`. If blocked, report to user and STOP.

**Output:** Concise context summary for the user with a "Reference Docs" file list.

---

## Phase 2: PLAN

**Goal:** Create an actionable plan from Phase 1 context.

1. **Restate requirements** from beads, spec references, and docs.
2. **Implementation steps** -- ordered by dependency. Each: file path, changes, why.
3. **Migration plan** -- if schema changes needed, list migrations and ordering.
4. **Testing strategy** -- unit, integration, E2E; in what order.
5. **Risks** -- what could go wrong, mitigations.

Use TodoWrite to create trackable items for the plan.

### >>> GATE: Plan Review <<<

Present the plan to the user. **STOP HERE.** See GATE Rules above.
Do not proceed to Phase 3 until the user explicitly approves.

---

## Phase 2b: UI DESIGN (UI features only)

**Trigger:** Any feature or bugfix that adds or modifies user-facing HTML/JS UI.
Skip this phase for pure backend work, API-only changes, or trivial copy edits.

**Goal:** Produce a written visual spec that the user approves *before* any HTML is written.
This prevents the most common class of frontend rework: functional code that looks wrong.

1. **List every UI state** the component can be in (e.g. read / editing / adding / loading / error).
2. **Sketch the field layout** for each state in plain text or ASCII, e.g.:
   ```
   [Start *] ──── [End]
   [Timezone *] ── [Door time]
   [Virtual URL] ── [Venue  ▸ name  ✕]
                    [Save]  [Cancel]
   ```
3. **State transition table** — what triggers each transition and what changes in the DOM.
4. **Data display rules** — how each field value is formatted for read view (dates, IDs, nulls).
5. **Checklist** — confirm the design satisfies every item in the `web/AGENTS.md` UI Quality Checklist
   before presenting it. If any item cannot be satisfied, call it out explicitly.

Present the spec to the user. **STOP HERE.** Do not write HTML or JS until the user approves.
The implementation prompt to `@general` in Phase 4 must include this approved spec verbatim.

### >>> GATE: UI Design Review <<<

Present the spec. **STOP.** See GATE Rules above.

---

## Phase 3: BEAD & BRANCH

If no beads have been created for this work yet, create all needed beads first:
```bash
bd create --title="<summary>" --description="<description>" --type=<type> --priority=<1-4>
```

Then create feature branch and claim the appropriate starting bead:
```bash
git checkout -b <type>/<bead-id>-<short-description>  # feat/, fix/, refactor/
bd update <id> --status in_progress
```

---

## Phase 4: IMPLEMENT (TDD)

**Goal:** Write tests first, then implement. Delegate each step to `@general`.

Tell each subagent: the step to implement, that this is Go TDD (failing tests first,
table-driven, edge cases), the Reference Doc paths from Phase 1, files to modify,
and to use context7 MCP for external library docs. For migrations: `migrate create
-ext sql -dir internal/storage/postgres/migrations -seq <name>`, write up+down,
update SQLc queries as needed.

**Implement subagent prompts must include:**
> **Do NOT commit or push any changes. The orchestrator owns all git commits.
> Write the code, run the tests, fix failures — then stop and reflect.
> 
> (Reflect on what you would have done differently: awkward abstractions, 
> package boundaries,tech debt, performance concerns, test coverage gaps, 
> missing docs, confusing or missing instructions, etc, and evaluate your 
> workflow for actionable improvements.)
> 
> Return a summary of what you changed, your reflections, and any remaining issues.**

After each subagent returns:
1. `scripts/agent-run.sh make test` -- verify tests pass
2. `make sqlc` if SQL queries changed
3. Commit: `<type>(<scope>): <description> [<bead-id>]`
   (types: `feat`, `fix`, `refactor`, `test`, `docs`, `chore`)
4. Mark TodoWrite items complete; update bead notes for significant decisions
5. If stuck, delegate to `@diagnose`

**Local verification:** After all steps are implemented, run the full local test
suite and start the local server to verify the feature works end-to-end:
```bash
scripts/agent-run.sh make ci
make run   # or make dev for live reload
```

---

## Phase 5: REVIEW

**Goal:** Verify everything works locally before staging.

1. **CI gate:** `scripts/agent-run.sh make ci` -- fix failures before continuing.
2. **Code review** -- Delegate to `@beads-code-reviewer`: review `git diff main...HEAD`,
   check quality/idioms/errors/tests/security/performance. Include Reference Doc paths.
   Ask for findings as CRITICAL / WARNING / SUGGESTION with file:line references.
3. **Fix** CRITICAL and WARNING (P0 and relevant P1 and P2) findings (delegate to `@general`).
4. **Re-run CI** after fixes: `scripts/agent-run.sh make ci`
5. **Local user review** -- Present a summary of changes to the user. If the server
   is running locally (`make run`), suggest specific things to test (endpoints, UI,
   behavior changes). Ask the user to confirm it works before proceeding. If they
   find issues, fix them here -- it's much faster than iterating on staging.

---

## Phase 6: REFLECT

**Goal:** Capture design and workflow hindsight while context is fresh.

Reflect on what you (and implementation agents) would do differently next time, evaluating the work process, architectural decisions, and paths not taken.
The goal is meta-process improvement and surfacing hidden debt, not recounting a commit log.

- Present a **human-readable reflection report** to the user.
- Use this structure:
  1. **What was hard/surprising** (Why did certain things take longer than expected?)
  2. **Alternate paths considered** (Is there a better way if things were different?)
  3. **Workflow evaluation** (Did the agent tooling/instructions break down? What prompted manual intervention?)
  4. **Tradeoffs and accepted debt** (What you intentionally did not do or polish)
  5. **Quality gaps** (tests/docs/observability still missing)
  6. **Actionable follow-ups** (what should happen next and why)
- Create follow-up beads for actionable items:
  `bd create --title="<improvement>" --description="Discovered during <bead-id>: <context>" --type=task --priority=3`
- Include created follow-up bead IDs in the report so the user can track them.
- Do not block on these, except for critical tests that are missing, which should be done now.

### >>> GATE: Reflection Review <<<

Present the reflection report to the user. **STOP HERE.** Wait for the user to read it and optionally discuss the follow-ups before proceeding to Phase 7.

---

## Phase 7: DOCUMENTATION

**Goal:** Keep docs in sync with code changes.

If behavior changed, delegate to `@general`: update `docs/`, subdirectory AGENTS.md,
`contexts/`/`shapes/` if JSON-LD changed, API docs if endpoints changed. Capture
non-obvious learnings (gotchas, patterns) in relevant AGENTS.md files.

### Documentation delegation requirements (important)

When delegating docs work, provide a **Docs Brief** so the doc agent has strong
context but still performs a fresh-eyes verification against code.

Your documentation prompt should include:
- Bead ID(s) and one-line intent for each
- `Reference Docs` paths from Phase 1
- **Behavior deltas to verify** (runtime behavior, deploy behavior, env vars,
  endpoint/schema changes, defaults)
- What to check for consistency: code vs docs, docs vs openapi, docs vs tests
- Explicit instruction to verify claims against current code (not assumptions)
- Expected output sections: changed docs, contradictions found, open questions,
  and whether `docs/api/openapi.yaml` must be updated

Ask the doc agent to propose updates only when confirmed by code/tests, and to
call out uncertainty explicitly.

Include the no-commit instruction in the delegation prompt (see Delegation Principle).

After: review changes against code, run relevant docs/openapi lint checks, `make sqlc`
if needed, commit.

---

## Phase 8: DEPLOY TO STAGING

**Goal:** Confirm in a real environment what already works locally.

Staging deploys are slow -- all testing and user review should happen locally first
(Phase 4-5). Staging is for final confirmation, not debugging.

**Step 1 — Push the branch first (REQUIRED). The remote deploy pulls this exact commit.
If the branch is not pushed, the deploy will fail with "reference is not a tree".**
```bash
git push -u origin HEAD
```

**Step 2 — Deploy and test:**
> **WARNING:** Deployment scripts (specifically Docker builds) often take longer than the default agent `bash` tool timeout (120s). You MUST increase the tool's timeout limit (e.g., `timeout: 600000` / 10 minutes) when running these deploy commands to prevent premature termination.

```bash
source .deploy.conf.staging 2>/dev/null
scripts/agent-run.sh ./deploy/scripts/deploy.sh staging --version HEAD
scripts/agent-run.sh ./deploy/scripts/test-remote.sh staging all
```

**If staging data needs resetting** (schema changes, clean slate needed):
```bash
scripts/staging-reset.sh --yes          # wipe events, keep users/keys/sources
scripts/staging-reset.sh --wipe-all     # full wipe (keeps only users)
scripts/ingest-toronto-events.sh staging 50 300   # re-populate with test data
```

Summarize what passed and failed.

### >>> GATE: Staging Review <<<

Present results and list manual testing steps. **STOP HERE.** See GATE Rules above.
Do not proceed to Phase 9 until user explicitly signs off.

---

## Phase 9: LAND

**Goal:** Merge to main. Only after user sign-off from Phase 8.

```bash
# Rebase
git checkout main && git pull && git checkout - && git rebase main
scripts/agent-run.sh make ci    # re-verify after rebase

# Merge
git checkout main && git merge --no-ff <branch-name>

# Close beads
bd close <id> --reason "<summary of what changed and why>"

# Add any final follow-ups discovered during staging
bd create --title="<follow-up>" --description="<context>" --type=task --priority=3

# Push and clean up
bd sync && git push
git status    # Must show "up to date with origin"
git branch -d <branch-name>
scripts/agent-cleanup.sh
```

---

## Workflow Variants

The phases above are the full `feature` workflow. For other types, skip or
reorder as follows:

### bugfix
Same as feature but Phase 4 starts with a **reproducing test** before any fix.

### refactor
Skip Phase 8 (staging deploy) unless the refactor changes observable behavior.
Phase 5 (review) is critical -- verify no behavior changes.

### security
Phase 5 adds a security-focused review. Use `@diagnose` with a security lens.
Phase 8 (staging) is mandatory. Phase 9 requires explicit security sign-off.

## Rules and Error Recovery

- **GATE rules** are in the "GATE Rules (CRITICAL)" section above -- never skip them.
- **Never deploy to production** -- staging only.
- **Always use `scripts/agent-run.sh`** for build/test/deploy commands.
- **Always update bead status** as you progress.
- **Commit early, push before stopping** -- work is not safe until pushed.
- **Tests/CI fail:** Fix before proceeding. Use `@diagnose` if stuck.
- **Deploy fails:** Check agent-run output. Use `@diagnose` if stuck.
- **User says "stop":** Commit, update bead notes, push branch. Safe to resume later.
