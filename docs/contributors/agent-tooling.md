# Agent Tooling

Togather supports both Codex and OpenCode from one repository-owned set of instructions.
Repository policy lives in `AGENTS.md`, reusable workflows live in `skills/`, and custom
role prompts live in `agents/agents/`. Small harness adapters expose those canonical files
to each client.

## Codex setup

1. Open the repository in Codex and mark it as trusted. Codex only loads project-scoped
   `.codex/config.toml`, custom agents, hooks, and rules for trusted projects.
2. Export the Togather MCP credential before starting Codex:

   ```bash
   export TOGATHER_MCP_API_KEY="..."
   ```

   Keep the value in local environment or secret-manager configuration. Never commit it.
3. Start Codex from the repository root and verify the project MCP servers:

   ```bash
   codex mcp list
   ```

   The expected project servers are `context7`, `hermes-kanban`, and `togather`.

`context7` provides current external-library documentation. `hermes-kanban` is the
project task board; every call must use `board: "togather"`. `togather` connects to the
staging SEL MCP endpoint and reads its bearer token from `TOGATHER_MCP_API_KEY`.

## Shared instructions

| Purpose | Canonical source | Codex adapter | OpenCode adapter |
|---|---|---|---|
| Repository policy | `AGENTS.md` and nested `AGENTS.md` files | Native discovery | Native discovery |
| Reusable skills | `skills/<name>/SKILL.md` | `.codex/skills/<name>` symlink | Native skill or command symlink |
| Configure a scraper | `skills/configure-source/SKILL.md` | `$configure-source` | `/configure-source` |
| Cut a release | `skills/release/SKILL.md` | `$release` | `/release` |
| Specialized roles | `agents/agents/*.md` | Thin custom-agent wrappers | Symlinked native agent definitions |

The adapter files contain only client-specific schema. Workflow prose is not copied,
so edits happen in one place. Codex follows symlinked skill directories, while OpenCode
command files point at the same `SKILL.md` files.

## Custom agents

Codex project agents are defined in `.codex/agents/`:

- `explore` performs read-only codebase discovery.
- `diagnose` investigates hard failures without editing files.
- `scraper-worker` owns one live scraper-source inspection and YAML update.

Ask Codex to delegate to one of these agents by name. The wrapper tells the subagent to
load the canonical prompt in `agents/agents/`, avoiding a second copy of the role.

## MCP configuration boundary

MCP transports are declared twice because Codex uses TOML and OpenCode uses JSON, and
neither client consumes the other's schema. Keep the server names and endpoints aligned
between `.codex/config.toml` and `opencode.json`. Credentials remain environment-backed;
do not add static authorization headers to either file.

Codex CLI, the Codex IDE extension, and the ChatGPT desktop app share Codex's configuration
layers. Project configuration overrides user defaults only for this trusted repository.

## Verification

After changing agent configuration:

```bash
# Validate JSON syntax
jq empty opencode.json

# Validate that Codex can parse project configuration and enumerate MCP servers
codex mcp list

# Confirm all adapters and symlinks are present
find .codex -maxdepth 3 -type f -o -type l
```

Run the repository's normal documentation and CI checks when the change also touches
application code or generated artifacts.
