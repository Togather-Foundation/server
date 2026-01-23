# SETUP

# Ryan Kelln

This setup uses Ubuntu, with Spec kit and beads and the copilot model with opencode and openskills.

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
4. Install Spec Kit
   1. `uv tool install specify-cli --from git+https://github.com/github/spec-kit.git`
5. Install beads
   1. `curl -fsSL https://raw.githubusercontent.com/steveyegge/beads/main/scripts/install.sh | bash`
   2. `bd init`
   3. `bd doctor --fix`
      1. Install beads skill manually
         1. `npx openskills install steveyegge/beads --universal`
   4. Install opencode-beads
      1. Add to your OpenCode config (`~/.config/opencode/opencode.json`):
         1. ```
            {
               "plugin": ["opencode-beads"]
            }
            ```
   5. `bd quickstart`


Notes for setting up new projects:
1. Run `opencode`
   1. `/init`
   2. 