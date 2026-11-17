<!-- AGENT:NAV
purpose:~codex;agents;repository;mcp;configuration;skills;configure;files
lines:92
nav[6]{s,n,name,about}:
13,80,#Agent Tooling,~codex;agents;repository;mcp;configuration
20,18,##Codex setup,~codex;togather;hermes;mcp;optional
38,14,##Shared instructions,~skill;skills;agents;configure;files
52,11,##Custom agents,~agents;codex;scraper;ask;avoiding
63,13,##MCP configuration boundary,~codex;configuration;mcp;repository;opencode
76,17,##Verification,~codex;configuration;mcp;pathlib;toml
-->

# Agent Tooling

Togather supports both Codex and OpenCode from one repository-owned set of instructions.
Repository policy lives in `AGENTS.md`, reusable workflows live in `skills/`, and custom
role prompts live in `agents/agents/`. Small harness adapters expose those canonical files
to each client.

## Codex setup

1. Open the repository in Codex and mark it as trusted. Codex only loads project-scoped
   `.codex/config.toml`, custom agents, hooks, and rules for trusted projects.
2. Use the shared instructions, skills and role adapters below. Configure only the
   optional services your workflow needs; contributing does not require Hermes,
   a personal board or access to a hosted Togather instance.
3. If using the repository's optional MCP transports, export the corresponding
   credentials (`KANBAN_MCP_API_KEY` for Hermes, `TOGATHER_MCP_API_KEY` for the
   configured Togather endpoint) before starting the client. Keep them in your
   environment or secret manager, never in committed configuration. Configure your
   own board and review routing outside repository instructions.
4. Inspect available connections with `codex mcp list`. The checked-in configuration
   declares `context7`, `hermes-kanban` and `togather`; their availability depends on
   your setup. `context7` supplies external-library docs; Hermes is an optional
   tracker; the Togather connection targets a configured SEL instance. These are
   client integrations, not backend runtime dependencies.

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

Repository MCP transports are declared in `.codex/config.toml`. Selected kanban tools
have explicit approval policies there. Credentials remain environment-backed; do not
add static authorization headers.

The repository no longer supplies `opencode.json`. OpenCode users configure MCP
transports in their own client configuration; shared instructions, skills, and role
adapters remain available.

Codex CLI, the Codex IDE extension, and the ChatGPT desktop app share Codex's configuration
layers. Project configuration overrides user defaults only for this trusted repository.

## Verification

After changing agent configuration:

```bash
# Validate TOML syntax
python3 -c 'import pathlib, tomllib; tomllib.loads(pathlib.Path(".codex/config.toml").read_text())'

# Validate that Codex can parse project configuration and enumerate MCP servers
codex mcp list

# Confirm all adapters and symlinks are present
find .codex -maxdepth 3 -type f -o -type l
```

Run the repository's normal documentation and CI checks when the change also touches
application code or generated artifacts.
