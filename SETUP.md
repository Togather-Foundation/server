# Development Setup

# opencode + kanban + spec kit

This setup uses Ubuntu, with Spec kit and the Hermes kanban MCP server with opencode and openskills.

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
5. Configure the Hermes kanban MCP server (task tracking on the shared `togather` board)
   1. Add the `hermes-kanban` MCP server to your OpenCode config (`~/.config/opencode/opencode.json` or project `opencode.json`). See the Hermes docs for the current endpoint and auth setup.
   2. Verify connectivity: list boards and confirm `togather` appears (`hermes-kanban_board_list`).
   3. Full workflow: `kanban_help` — claims are kernel-enforced, completion is review-gated.


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
