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
Phase 3: BEAD & BRANCH  -- Create feature branch, create/claim bead
Phase 4: IMPLEMENT      -- TDD in subagents (delegate to @general)
Phase 5: REVIEW         -- CI + code review (delegate to @beads-code-reviewer)
Phase 6: REFLECT        -- Design hindsight, create follow-up beads
Phase 7: DOCUMENTATION  -- Update docs (delegate to @general)
Phase 8: DEPLOY         -- Push, deploy to staging, run tests
  >>> GATE: Stop and wait for user to test staging and sign off <<<
Phase 9: LAND           -- Rebase, merge to main, close beads, push
```

## GATE Rules (CRITICAL)

There are exactly **2 mandatory stops** where you MUST wait for the user:

1. **After Phase 2 (PLAN):** Present the plan. STOP. Do not proceed until the user
   says "yes", "proceed", "go", "lgtm", or similar. If they want changes, revise.

2. **After Phase 8 (DEPLOY):** Present staging test results and list manual testing
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
   later subagents as "Reference Docs"). Remind to only explore, not plan or implement.
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

**Every subagent prompt must include (copy verbatim):**
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

Reflect on what you (and implementation agents) would do differently: 
awkward abstractions, package boundaries, tech debt, performance concerns, 
test coverage gaps, missing docs, confusing or missing instructions, etc, 
and evaluate your workflow for actionable improvements.

- Present a brief summary to the user (not a file).
- Create follow-up beads for actionable items:
  `bd create --title="<improvement>" --description="Discovered during <bead-id>: <context>" --type=task --priority=3`
- Do not block on these, except for critical tests that are missing, which should be done now.

---

## Phase 7: DOCUMENTATION

**Goal:** Keep docs in sync with code changes.

If behavior changed, delegate to `@general`: update `docs/`, subdirectory AGENTS.md,
`contexts/`/`shapes/` if JSON-LD changed, API docs if endpoints changed. Capture
non-obvious learnings (gotchas, patterns) in relevant AGENTS.md files.

Include the no-commit instruction in the delegation prompt (see Delegation Principle).

After: review changes, `make sqlc` if needed, commit.

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
