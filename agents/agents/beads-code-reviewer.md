---
description: Deprecated — use beads-go-reviewer (global) instead. It auto-discovers the SEL project extension at .opencode/agents/beads-go-reviewer.project.md and applies web-backend + SEL checks automatically. This agent is kept as a redirect to avoid breaking existing invocations.
mode: subagent
temperature: 0.1
permission:
  write: deny
  edit: deny
---

# Deprecated — use `beads-go-reviewer`

This agent has been replaced. The global `beads-go-reviewer` (installed at
`~/.config/opencode/agent/beads-go-reviewer.md`) handles all the generic Go
review, plus auto-loads the web-backend domain profile AND this project's
SEL extension (`.opencode/agents/beads-go-reviewer.project.md`).

**What to do:** invoke `beads-go-reviewer` instead of `beads-code-reviewer`.
The review quality is identical — the generic agent discovers the project's
extension file and applies all the same SEL/togather-specific checks, plus
generic Go checks that were previously duplicated.

**Why this file still exists:** to avoid breaking existing invocations that
reference `beads-code-reviewer`. If you're reading this, switch to
`beads-go-reviewer`. Once all call sites are updated, delete this file and the
symlink in `.opencode/agents/beads-code-reviewer.md`.

For now, re-invoke with `beads-go-reviewer` to get the full review.
