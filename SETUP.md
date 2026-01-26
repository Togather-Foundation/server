# SETUP

# Ryan Kelln

This setup uses Ubuntu, with Spec kit and beads and the copilot model with opencode and openskills.

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
6. Set up beads for separate sync branch
   1. 'bd config set sync.branch beads-sync'
   2. `git branch -c beads-sync`
   3. `bd hooks install`


Notes for setting up new projects:
1. Run `opencode`
   1. `/init` to create `AGENTS.md`

## SEL Server Configuration

1. Copy the environment template:
   1. `cp .env.example .env`
2. Update `.env` with your local database credentials and secrets.
3. Keep `.env` local only (it is gitignored).

### Security Configuration

The SEL server requires specific security configurations for safe operation:

**Required Environment Variables:**

```bash
# JWT Secret (REQUIRED - minimum 32 characters)
# Generate with: openssl rand -base64 48
JWT_SECRET=<your-cryptographically-random-secret-here>

# Database connection (use sslmode=require for production)
DATABASE_URL=postgres://user:pass@localhost:5432/sel?sslmode=require
```

**Optional Security Configuration:**

```bash
# Rate limiting (requests per minute)
RATE_LIMIT_PUBLIC=60      # Public/unauthenticated tier (default: 60)
RATE_LIMIT_AGENT=300      # Agent/authenticated tier (default: 300)
RATE_LIMIT_ADMIN=0        # Admin tier - 0 means unlimited (default: 0)

# HTTP server timeouts (seconds)
HTTP_READ_TIMEOUT=10      # Protects against slow read attacks (default: 10)
HTTP_WRITE_TIMEOUT=30     # Protects against slow write attacks (default: 30)
HTTP_MAX_HEADER_BYTES=1048576  # 1 MB limit (default: 1048576)
```

**Security Best Practices:**

- **JWT_SECRET**: Must be at least 32 characters. Use `openssl rand -base64 48` to generate a strong secret.
- **DATABASE_URL**: Always use `sslmode=require` in production to encrypt database connections.
- **API Keys**: When issuing API keys to agents, use strong random generation and rotate periodically.
- **HTTPS**: Always run the server behind a TLS-terminating reverse proxy (nginx, Traefik, etc.) in production.

For complete security details, see `docs/togather_SEL_server_architecture_design_v1.md` § 7.1 (Security Hardening) and `CODE_REVIEW.md`.
