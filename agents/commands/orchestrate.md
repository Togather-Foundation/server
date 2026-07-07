---
description: Full-lifecycle workflow from beads to merge. Handles planning, branching, TDD, review, final verification, and user sign-off. Loads a project extension for project-specific commands.
---

# Orchestrate: Bead-to-Merge Workflow

You are an **orchestrator**, not an implementor. Your job is to manage the workflow,
make decisions, and delegate work to subagents via the Task tool.

## Step 0 — Load Project Extension (REQUIRED, do this first)

This workflow needs project-specific commands for build, test, verification, and
cleanup. Find and read the project extension file:

```bash
# Find the project root and locate the extension
PROJECT_ROOT=$(git rev-parse --show-toplevel)
find "$PROJECT_ROOT" -maxdepth 4 -name "orchestrate.project.md" -not -path "*/.git/*"
```

Use the `read` tool on the found path. If `find` returns multiple matches,
prefer the one closest to the project root or under an `agents/` directory.

If `find` returns nothing, try these fallback paths relative to project root:
- `agents/agent/orchestrate.project.md`
- `agents/commands/orchestrate.project.md`
- `.opencode/commands/orchestrate.project.md`

If no extension file is found **anywhere**, ask the user for the required
commands before proceeding to Phase 3.

The extension defines commands you will use in phases 3, 4, 5, 8, and 9:

- **Build & test commands** (CI gate, unit tests, code generation, dev server)
- **Technology stack** (language, database, query builder, migrations)
- **Worktree setup** (paths, setup scripts, database test workarounds)
- **Final verification** — what "done and correct" means for this project.
  For services: deploy to staging + integration tests. For libraries: full CI
  matrix + dry-run publish. For CLIs: cross-platform build + smoke tests.
  If the extension defines staging deploy scripts, use them here.
- **Land & cleanup** (post-rebase CI, cleanup scripts, worktree land script)

If no extension file is found, use your best judgement based on AGENTS details.

---

## Mandatory Phases (do NOT skip or combine)

This workflow has **9 phases**. Execute them in order. Your TodoWrite items
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
Phase 8: VERIFY         -- Run final verification (extension defines what "done" means)
  >>> GATE: Stop and wait for user to verify and sign off <<<
Phase 9: LAND           -- Rebase, merge to main, close beads, push
```

## GATE Rules (CRITICAL)

There are exactly **3 mandatory stops** where you MUST wait for the user:

1. **After Phase 2 (PLAN):** Present the plan. STOP. Do not proceed until the user
   says "yes", "proceed", "go", "lgtm", or similar. If they want changes, revise.

2. **After Phase 6 (REFLECT):** Present the reflection report. STOP. Do not proceed
   to Phase 7 until the user acknowledges it.

3. **After Phase 8 (VERIFY):** Present verification results and any manual testing
   steps. STOP. Do not proceed to Phase 9 (LAND) until the user explicitly signs
   off. The user may need minutes or hours to manually verify. **Wait.**

**NEVER skip a GATE. NEVER merge without user sign-off. Violating a GATE is a
critical failure of this workflow.**

## Delegation Principle

**Delegate substantial work to subagents via Task tool.** Preserve your context
for management, decisions, and coordination.

| You do directly | You delegate via Task |
|---|---|
| Beads (`bd show/update/close`) | Exploration, doc reading -> `@explore` |
| Git (branch, commit, push) | Code + tests -> `@general` |
| Build/test/verify (from project extension) | Code review -> `@beads-code-reviewer` |
| TodoWrite, GATEs, coordination | Hard problems -> `@diagnose` |

When delegating, always include: what to do, reference doc paths from Phase 1,
what to return, and the bead ID(s).

**Parallel delegation — file ownership:** When delegating two or more subagents
simultaneously, enumerate which files each subagent owns. No file may appear in
more than one subagent's ownership list. If two subagents genuinely need the same
file, run them sequentially instead. This prevents merge conflicts and dropped
changes from overlapping edits.

**Sequence when one subagent changes public API:** Even across non-overlapping
files, if subagent A renames a field, changes a function signature, or alters a
type that subagent B's files import, run them sequentially. The API change creates
an implicit dependency — subagent B won't compile until A's changes land.

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

Add `--worktree` to isolate work in a git worktree (safe parallel orchestration).
Omit for an in-tree feature branch.

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
   Explore subagents should update the associated bead with the exploration report so
   work can be resumed as needed.
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

### Worktree mode (`--worktree`)

Use the worktree commands from the project extension (Step 0):
- Worktree path and branch naming from the extension
- Setup script from the extension
- If the project extension notes a database workaround (e.g. Postgres not
  available in worktrees), apply it

For multi-bead work, pick a primary bead for naming, and claim every bead:
```bash
git worktree prune
# Then use the worktree commands from the project extension
bd update <primary-bead-id> --status in_progress
bd update <bead-id-2> --status in_progress  # if multiple beads
```

**All subsequent phases in worktree mode** run with `workdir="$WORKTREE"` on
every bash command (CI, tests, git, etc.). Subagent prompts must include the
worktree path so they know where to operate.

---

## Phase 4: IMPLEMENT (TDD)

**Goal:** Write tests first, then implement. Delegate each step to `@general`.

Tell each subagent: the step to implement, the language and testing conventions
(from the project extension), the Reference Doc paths from Phase 1, files to
modify, and to use context7 MCP for external library docs.

For schema changes: use the migration command from the project extension.

**Implement subagent prompts must include:**
> **Do NOT commit or push any changes. The orchestrator owns all git commits.
> Write the code, run the tests, fix failures — then stop and reflect.
>
> Answer these concrete questions in your response (do not skip or generalize):
> 1. What was the trickiest decision you made and why?
> 2. What edge case did you almost miss?
> 3. What would you do differently if you started over?
> 4. Is there any tech debt or awkward abstraction you're leaving behind?
>
> Return a summary of what you changed, your concrete answers above, and any remaining issues.**

After each subagent returns:
1. Run the unit test command from the project extension to verify tests pass
2. Run the code generation command from the project extension if queries/schemas changed
3. **Triage subagent reflections** -- read them carefully. Fix anything actionable
   before committing (e.g. a missed edge case, a cleanup, a confusing variable name).
   Record non-trivial items that can't be fixed now as beads. Do not silently discard
   the subagent's observations — they have context the orchestrator lacks.
4. **Verify staged diff for sequential agents** — if a file was touched by a prior
   sequential subagent's commit AND modified by the current subagent, run
   `git diff --cached <file>` before committing to confirm only the intended changes
   are staged. Overlapped files can leak changes across commit boundaries.
5. Commit: `<type>(<scope>): <description> [<bead-id>]`
   (types: `feat`, `fix`, `refactor`, `test`, `docs`, `chore`)
   **Never use `git add -A`** — stage specific files with explicit paths.
6. Mark TodoWrite items complete; update bead notes for significant decisions
7. If stuck, delegate to `@diagnose`

**Local verification:** After all steps are implemented, run the CI gate and
start the dev server (commands from the project extension) to verify the feature
works end-to-end.

---

## Phase 5: REVIEW

**Goal:** Verify everything works locally before staging.

1. **CI gate:** Run the CI command from the project extension — fix failures
   before continuing.
2. **Code review** -- Delegate to `@beads-code-reviewer`: review `git diff main...HEAD`
   for the current branch. The reviewer reports findings at four severity levels
   (Critical/High/Medium/Low) with `file:line` references. It creates beads for
   trackable P0–P2 issues automatically. Include Reference Doc paths from Phase 1.
3. **Triage findings** -- Read the review report. Fix Critical (P0) and High (P1) findings
   first (delegate to `@general`). Medium (P2) may be deferred if substantial and
   documented as follow-up beads, while simple fixes can be done immediately in subagents.
   Low/nitpicks should be done immediately if trivial and you think improves code quality.
4. **Re-run CI** after fixes (same CI command from the project extension).
5. **Local user review** -- Present a summary of changes to the user. If the
   dev server is running, suggest specific things to test (endpoints, UI,
   behavior changes). Ask the user to confirm it works before proceeding. If they
   find issues, fix them here — it's much faster than iterating on staging.

---

## Phase 6: REFLECT

**Goal:** Synthesize hindsight from all sources while context is fresh.

**Sources to draw from — all are required input, not optional:**
- Your own observations as orchestrator (decisions that felt awkward, phases that stalled)
- Subagent reflections returned during Phase 4 (implementors see things orchestrators miss)
- Code review findings from Phase 5 (patterns that recurred, gaps the reviewer flagged)

Evaluate the work process, architectural decisions, and paths not taken.
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
  `bd create --title="<follow-up>" --description="<context>" --type=<bug|feature|task> --priority=3`
- Include created follow-up bead IDs in the report so the user can track them.
- Do not block on these, except for critical tests that are missing, which should be done now.

### >>> GATE: Reflection Review <<<

Present the reflection report to the user. **STOP HERE.** Wait for the user to
read it and optionally discuss the follow-ups before proceeding to Phase 7. 
Do not proceed after first reflection feedback, ask for explicit consent to continue.

---

## Phase 7: DOCUMENTATION

**Goal:** Keep docs in sync with code changes.

If behavior changed, delegate to `@general`: update `docs/`, subdirectory AGENTS.md,
`contexts/`/`shapes/` if schemas changed, API docs if endpoints changed. Capture
non-obvious learnings (gotchas, patterns) in relevant AGENTS.md files.

### Documentation delegation requirements (important)

When delegating docs work, provide a **Docs Brief** so the doc agent has strong
context but still performs a fresh-eyes verification against code.

Your documentation prompt should include:
- Bead ID(s) and one-line intent for each
- `Reference Docs` paths from Phase 1
- **Behavior deltas to verify** (runtime behavior, deploy behavior, env vars,
  endpoint/schema changes, defaults)
- What to check for consistency: code vs docs, docs vs API spec, docs vs tests
- Explicit instruction to verify claims against current code (not assumptions)
- Expected output sections: changed docs, contradictions found, open questions,
  and whether the API spec must be updated

Ask the doc agent to propose updates only when confirmed by code/tests, and to
call out uncertainty explicitly.

Include the no-commit instruction in the delegation prompt (see Delegation Principle).

After: review changes against code, run any lint/doc-check commands from the
project extension, commit.

---

## Phase 8: VERIFY

**Goal:** Confirm the work is correct in the most realistic environment available.
This is the final gate before merging — every project needs this, but the shape
varies.

**What verification means by project type:**

| Project type | Typical Phase 8 |
|---|---|
| Web service / API | Deploy to staging, run integration tests, manual smoke test |
| Library / SDK | Full CI matrix, dry-run publish, semver check, doc build |
| CLI tool | Cross-platform build, smoke test key commands, verify `--help` |
| Worker / daemon | Deploy to staging, run a job, verify idempotency, check metrics |
| Game / realtime | Build all targets, profile hot path, playtest |

Use the project extension's **Final verification** section. If the extension
defines staging deploy scripts, Phase 8 is a deploy-and-verify. If it defines
only CI commands, Phase 8 is a thorough CI pass. If no specific verification
is configured, run the full CI command and present the branch diff for manual
review.

**When verification includes a remote deploy:**

Push the branch first (the remote deploy pulls this exact commit):
```bash
git push -u origin HEAD
```

Then run the deploy and test commands from the project extension (Docker builds
may need extended timeout, e.g. `timeout: 600000`).

Summarize what passed and failed. List concrete manual testing steps the user
should perform (specific endpoints, behaviors, edge cases).

### >>> GATE: Verification Review <<<

Present results and list manual verification steps. **STOP HERE.** See GATE Rules
above. Do not proceed to Phase 9 until the user explicitly signs off.

---

## Phase 9: LAND

**Goal:** Merge to main. Only after user sign-off from Phase 8 (VERIFY).

### Default (in-tree)

```bash
# Rebase
git checkout main && git pull && git checkout - && git rebase main
# Re-verify CI after rebase (use the project extension CI command)
# Merge (include a detailed summary of changes and resolved beads in the commit message)
git checkout main && git merge --no-ff <branch-name> \
  -m "Merge branch '<branch-name>'" \
  -m "<Detailed summary of what was built, fixed, and why. List closed beads.>"

# Close beads
bd close <id> --reason "<summary of what changed and why>"

# Push and clean up
git push
git status    # Must show "up to date with origin"
git branch -d <branch-name>
# Run the project extension's cleanup script (if any)
```

### Worktree mode (`--worktree`)

Main is locked in the main worktree. Push first, then switch to main working
directory for the merge:

```bash
# From the worktree:
git fetch origin main && git rebase origin/main
# Re-verify CI (use the project extension CI command)
git push
```

```bash
# In the main working directory (NOT the worktree):
git checkout main && git pull && git merge --no-ff <branch-name> \
  -m "Merge branch '<branch-name>'" \
  -m "<Detailed summary of what was built, fixed, and why. List closed beads.>"
bd close <id> --reason "<summary of what changed and why>"
# Add any final follow-ups
bd create --title="<follow-up>" --description="<context>" --type=task --priority=3
# Re-verify CI on main (use the project extension CI command)
git push
git status    # Must show "up to date with origin"
git branch -d <branch-name>
# Run the project extension's land-worktree script
```

---

## Workflow Variants

The phases above are the full `feature` workflow. For other types, skip or
reorder as follows:

### bugfix
Same as feature but Phase 4 starts with a **reproducing test** before any fix.

### refactor
Phase 8 (VERIFY) may be lighter: run full CI and confirm no behavior changes.
Phase 5 (review) is critical.

### security
Phase 5 adds a security-focused review. Use `@diagnose` with a security lens.
Phase 8 must include security-specific verification (dependency audit,
penetration test smoke, secret scan). Phase 9 requires explicit security
sign-off.

## Rules and Error Recovery

- **GATE rules** are in the "GATE Rules (CRITICAL)" section above -- never skip them.
- **Never deploy to production** -- staging only.
- **Always use the project extension's command runner** for build/test/verify commands.
- **Always update bead status** as you progress.
- **Commit early, push before stopping** -- work is not safe until pushed.
- **Tests/CI fail:** Fix before proceeding. Use `@diagnose` if stuck.
- **Deploy fails:** Check output. Use `@diagnose` if stuck.
- **User says "stop":** Commit, update bead notes, push branch. Safe to resume later.
