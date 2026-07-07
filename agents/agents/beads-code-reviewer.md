---
description: Deprecated — use the global beads-code-reviewer instead. It auto-discovers language, loads the matching language module and domain profiles, and applies project-specific checks from this project's extension. This agent is kept as a redirect to avoid breaking existing invocations.
mode: subagent
temperature: 0.1
permission:
  write: deny
  edit: deny
---

# Deprecated — use global `beads-code-reviewer`

This agent has been replaced. The global `beads-code-reviewer` (installed at
`~/.config/opencode/agent/beads-code-reviewer.md`) is now multi-language. It
detects Go from `go.mod`, loads the Go language module, auto-loads the
`web-backend` domain profile, and reads this project's review extension
(`agents/agent/beads-go-reviewer.project.md`).

**What to do:** invoke `beads-code-reviewer` instead. The review quality is
identical — the global agent discovers the project's extension file and
applies all the same SEL/togather-specific checks, plus language-agnostic
baseline checks that were previously absent.

**Why this file still exists:** to avoid breaking existing invocations that
reference the project-local `beads-code-reviewer`. If you're reading this,
the global agent is the canonical reviewer. Once all call sites are updated,
delete this file.
