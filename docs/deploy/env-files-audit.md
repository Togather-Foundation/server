# Environment Files Audit

**Last Updated:** 2026-02-03  
**Status:** Current and documented

## Overview

This document audits all .env files in the Togather server repository and documents their purpose, location, and usage.

## File Categories

### 1. Runtime Configuration (Server Secrets & Settings)

These files contain **secrets** and runtime configuration for the server binary.

#### `.env` (gitignored)
- **Location:** Project root
- **Purpose:** Local development runtime configuration
- **Contains:** DATABASE_URL, JWT_SECRET, CORS settings, server config
- **Used by:** `docker-compose.yml`, local server instances
- **Template:** `.env.example`
- **Deployment:** Symlinked from `/opt/togather/.env` on servers

#### `.env.example` (committed)
- **Location:** Project root
- **Purpose:** Template for creating `.env`
- **Contains:** All required environment variables with example values
- **Used by:** Developers setting up local environment
- **Documentation:** Comments explain each variable

#### `deploy/config/environments/.env.{environment}.example` (committed)
- **Location:** `deploy/config/environments/`
- **Files:** 
  - `.env.development.example`
  - `.env.staging.example`
  - `.env.production.example`
- **Purpose:** Per-environment runtime config templates
- **Contains:** Environment-specific DATABASE_URL, secrets, settings
- **Used by:** Servers via symlink from `/opt/togather/.env.{environment}`
- **Deployment:** Copied to `/opt/togather/` on server, never committed with real values

### 2. Deployment Metadata (Non-Secret)

These files contain **non-secret** deployment orchestration metadata.

#### `.deploy.conf.{environment}` (gitignored)
- **Location:** Project root
- **Files:** `.deploy.conf.staging`, `.deploy.conf.production`, etc.
- **Purpose:** Per-environment deployment metadata
- **Contains:** NODE_DOMAIN, SSH_HOST, SSH_USER, CITY, REGION
- **Used by:** 
  - `deploy/scripts/deploy.sh` (auto-loads on deployment)
  - `deploy/testing/config.sh` (auto-loads for tests)
- **Template:** `.deploy.conf.example`
- **Documentation:** `docs/deploy/deploy-conf.md`
- **Note:** Introduced 2026-02-03 to prevent agents from guessing domains/SSH

### 3. Docker Build Configuration

#### `deploy/docker/.env` (gitignored)
- **Location:** `deploy/docker/`
- **Purpose:** Docker Compose variables for build process
- **Contains:** Build args, image tags, registry settings
- **Used by:** `deploy/docker/docker-compose.yml`
- **Template:** `deploy/docker/.env.example`

#### `deploy/docker/.env.example` (committed)
- **Location:** `deploy/docker/`
- **Purpose:** Template for Docker build variables
- **Contains:** Example build configuration
- **Used by:** Developers and CI/CD

### 4. Testing Configuration

#### `deploy/testing/environments/{environment}.test.env` (committed)
- **Location:** `deploy/testing/environments/`
- **Files:**
  - `local.test.env`
  - `staging.test.env`
  - `production.test.env`
- **Purpose:** Test suite configuration per environment
- **Contains:** BASE_URL, test timeouts, permissions, flags
- **Used by:** `deploy/testing/config.sh` and smoke tests
- **Note:** Can contain API keys but should use environment variables for secrets

## File Relationships

```
Runtime Secrets (.env files)
├── .env (local development) ← .env.example
├── /opt/togather/.env.staging (on server) ← .env.staging.example
└── /opt/togather/.env.production (on server) ← .env.production.example

Deployment Metadata (.deploy.conf files)
├── .deploy.conf.staging ← .deploy.conf.example
└── .deploy.conf.production ← .deploy.conf.example

Testing Config
├── deploy/testing/environments/local.test.env
├── deploy/testing/environments/staging.test.env
└── deploy/testing/environments/production.test.env

Docker Build
└── deploy/docker/.env ← deploy/docker/.env.example
```

## Configuration Separation

The configuration is now cleanly separated:

| Type | Files | Secrets? | Gitignored? | Purpose |
|------|-------|----------|-------------|---------|
| Runtime | `.env*` | ✓ Yes | ✓ Yes | Server configuration, database URLs, JWT secrets |
| Deployment | `.deploy.conf.*` | ✗ No | ✓ Yes | SSH hosts, domains, city names |
| Testing | `*.test.env` | ⚠️ Some | ✗ No | Test configuration, can use env vars for secrets |
| Docker Build | `deploy/docker/.env` | ✗ No | ✓ Yes | Build configuration, image tags |

## Best Practices

### Adding New Environments

When adding a new environment (e.g., `production-montreal`):

1. **Create runtime config template:**
   ```bash
   cp deploy/config/environments/.env.production.example \
      deploy/config/environments/.env.production-montreal.example
   ```

2. **Create deployment config:**
   ```bash
   cp .deploy.conf.example .deploy.conf.production-montreal
   # Edit with NODE_DOMAIN=montreal.togather.foundation, SSH_HOST, etc.
   ```

3. **Create test config:**
   ```bash
   cp deploy/testing/environments/production.test.env \
      deploy/testing/environments/production-montreal.test.env
   ```

4. **Deploy to server:**
   ```bash
   # Create /opt/togather/.env.production-montreal on server with real secrets
   ./deploy/scripts/deploy.sh production-montreal
   ```

### Security Guidelines

**DO put in .env files (secrets):**
- Database connection strings with credentials
- JWT secrets
- API keys and tokens
- Private keys
- Passwords

**DO put in .deploy.conf files (metadata):**
- Domain names (NODE_DOMAIN)
- SSH hostnames or aliases
- SSH usernames
- City/region names
- Deployment settings (timeouts, flags)

**DO NOT put secrets in .deploy.conf files!**

## Related Documentation

- **Deployment Config:** `docs/deploy/deploy-conf.md`
- **Deployment Testing:** `docs/deploy/DEPLOYMENT-TESTING.md`
- **Quick Start:** `docs/deploy/quickstart.md`
- **Agent Instructions:** `AGENTS.md`

## Maintenance

This audit should be updated when:
- New environment files are added
- File purposes change
- New configuration patterns are introduced
- Environment variable requirements change
