<!-- AGENT:NAV
purpose:~install;opencode;agent;env;https;kit;local;openskills
lines:53
-->

# Development Setup

# OpenCode + Spec Kit

This example setup uses Ubuntu, Spec Kit, OpenCode and openskills. Task tracking
and review use your configured tools; Hermes is optional, and no shared board is
required. See [agent tooling](docs/contributors/agent-tooling.md) for other clients.

Note: Using '.agent/' directory with a symlink to '.claude/' for compatibility.

1. Install `uv`
   1. `curl -LsSf https://astral.sh/uv/install.sh | sh`
2. Install opencode
   1. `curl -fsSL https://opencode.ai/install | bash`
3. Install openskills
   1. Install node 
      1. `sudo apt install nodejs npm -y` or
      2. See https://nodejs.org/en/download
   2. `npx openskills install anthropics/skills --universal`
   3. `npx openskills sync`
   4. Make symlink for compatibility: `ln -s .agent .claude`
4. Install Spec Kit
   1. `uv tool install specify-cli --from git+https://github.com/github/spec-kit.git`
5. Optionally configure your preferred task and review tools.
   1. If using Hermes, add your own endpoint and authentication to your local client configuration, using the Hermes docs.
   2. Select your own board and review routing. Read live `kanban_help` for its lifecycle; Togather does not prescribe a board or reviewer profile.


Notes for setting up new projects:
1. Run `opencode`
   1. `/init` to create `AGENTS.md`

## SEL Server Configuration

1. Copy the environment template:
   1. `cp .env.example .env`
2. Update `.env` with your local database credentials and secrets.
3. Keep `.env` local only (it is gitignored).

## Local Symlinks (not tracked by git)

Some files in `.opencode/` (gitignored) are symlinks to tracked files. Recreate
them after a fresh clone:

```bash
# /release command — points to the tracked agents/release.md
ln -s ../../agents/release.md .opencode/command/release.md
```
