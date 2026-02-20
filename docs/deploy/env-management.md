# Environment Variable Management

Single source of truth for managing environment variables across all Togather environments.

---

## File Locations

| Use Case | .env File | Template |
|----------|-----------|----------|
| Local dev (non-Docker) | `<repo>/.env` | `<repo>/.env.example` |
| Local dev (Docker) | `<repo>/deploy/docker/.env` | `<repo>/deploy/docker/.env.example` |
| Staging server | `/opt/togather/.env.staging` | `deploy/config/environments/.env.staging.example` |
| Production server | `/opt/togather/.env.production` | `deploy/config/environments/.env.production.example` |

**Rule**: Never create `deploy/config/environments/.env.staging` or `.env.production` locally. Secrets live on the servers where they are used.

---

## Architecture: .deploy.conf vs .env

These are two distinct file types with different purposes:

| File | Purpose | Contains | Secrets? | Gitignored? |
|------|---------|----------|----------|-------------|
| `.deploy.conf.<env>` | Deployment metadata | SSH hosts, domains, city/region | No | Yes |
| `.env.<env>` | Application runtime config | DATABASE_URL, JWT_SECRET, passwords | Yes | Yes |
| `.env.example` / `.deploy.conf.example` | Templates | Placeholder values | No | No (committed) |

`.deploy.conf` files are safe to share across the team. `.env` files must never leave the server they belong to. See [deploy-conf.md](deploy-conf.md) for `.deploy.conf` details.

---

## Required Variables

Deployment fails without these three:

| Variable | Description |
|----------|-------------|
| `DATABASE_URL` | PostgreSQL connection string |
| `JWT_SECRET` | JWT signing key (generate: `openssl rand -base64 32`) |
| `ADMIN_PASSWORD` | Admin account password |

---

## Variable Precedence

From highest to lowest:

1. CLI env vars (e.g., `DATABASE_URL=x ./server serve`)
2. Shell environment (`export DATABASE_URL=x`)
3. `.env` file (loaded by the server at startup)
4. `docker-compose.yml` `environment:` defaults (e.g., `${VAR:-default}`)

---

## How Container Recreation Works

`deploy.sh` uses `--force-recreate` on every deploy. This means:

- Update the `.env` file on the server
- Run `deploy.sh`
- The new values are automatically picked up

You do not need to manually restart containers after editing `.env`. A full deploy handles it.

---

## Symlink Mechanism (Remote Servers)

On remote servers, `deploy.sh` creates symlinks so Docker Compose resolves `env_file: ../../.env` correctly:

```
/opt/togather/src/.env                          -> /opt/togather/.env.staging  (for docker-compose ../../.env)
/opt/togather/src/deploy/config/environments/.env.staging -> /opt/togather/.env.staging  (for deploy.sh discovery)
```

The compose file at `deploy/docker/docker-compose.yml` uses `env_file: ../../.env`. The symlink makes this resolve to the correct environment file without modifying the compose file per environment.

---

## Audit Tool

`deploy/scripts/env-audit.sh` compares a template against the actual `.env` file and reports:

- **Errors** (exit 1): Required vars (`DATABASE_URL`, `JWT_SECRET`, `ADMIN_PASSWORD`) that are missing — deploy will fail
- **Warnings** (exit 2): Optional vars in template that are absent from the env file
- **Info** (exit 0): Vars in the env file not present in the template (may be stale)

```bash
# Audit an environment (auto-resolves file paths)
./deploy/scripts/env-audit.sh development
./deploy/scripts/env-audit.sh docker
./deploy/scripts/env-audit.sh staging
./deploy/scripts/env-audit.sh production

# Strict mode: treat warnings as errors
./deploy/scripts/env-audit.sh staging --strict

# JSON output for scripting
./deploy/scripts/env-audit.sh staging --json

# Override file paths
./deploy/scripts/env-audit.sh staging \
  --template deploy/config/environments/.env.staging.example \
  --env-file /opt/togather/.env.staging

# Run via deploy.sh
./deploy/scripts/deploy.sh staging --env-diff
```

File resolution for `env-audit.sh`:

| Environment | Template | Env file |
|-------------|----------|----------|
| `development` | `.env.example` | `.env` |
| `docker` | `deploy/docker/.env.example` | `deploy/docker/.env` |
| `staging` | `deploy/config/environments/.env.staging.example` | `/opt/togather/.env.staging` (server) or local fallback |
| `production` | `deploy/config/environments/.env.production.example` | `/opt/togather/.env.production` (server) or local fallback |

---

## Workflow: Adding a New Variable

When adding a new env var to the codebase:

1. **Add to all templates** (run in parallel or sequence):
   - `<repo>/.env.example`
   - `<repo>/deploy/docker/.env.example`
   - `deploy/config/environments/.env.staging.example`
   - `deploy/config/environments/.env.production.example`

2. **Add to `docker-compose.yml`** under the relevant service's `environment:` block:
   ```yaml
   # If a safe default exists:
   MY_VAR: ${MY_VAR:-default_value}

   # If no safe default (deploy will fail without it):
   MY_VAR: ${MY_VAR:?MY_VAR must be set in .env file}
   ```

3. **Audit each environment** to confirm nothing is missing:
   ```bash
   ./deploy/scripts/env-audit.sh development
   ./deploy/scripts/env-audit.sh docker
   ./deploy/scripts/env-audit.sh staging   # run on server, or SSH first
   ```

4. **On staging/production**: SSH to the server, add the var to the `.env` file, then deploy:
   ```bash
   ssh deploy@server
   echo "MY_VAR=value" >> /opt/togather/.env.staging
   chmod 600 /opt/togather/.env.staging
   exit

   ./deploy/scripts/deploy.sh staging --version HEAD
   ```

---

## Workflow: Debugging Env Issues

**Step 1**: Check what the running container actually sees:

```bash
docker exec togather-server-blue printenv | sort
docker exec togather-server-blue printenv | grep MY_VAR
```

**Step 2**: Check what the `.env` file contains on the server:

```bash
ssh deploy@server "grep MY_VAR /opt/togather/.env.staging"
```

**Step 3**: Run the audit:

```bash
./deploy/scripts/env-audit.sh staging
```

**Step 4**: Resolve based on findings:

| Finding | Cause | Fix |
|---------|-------|-----|
| Var in `.env` but not in container | Container not recreated since last `.env` edit | Run `deploy.sh` (it force-recreates) |
| Var missing from `.env` | Not added to server-side file | SSH to server, add var, deploy |
| Var missing from template | Template out of date | Add to `.env.example` and commit |
| Var in `docker-compose.yml` `environment:` with default | Container has it even if not in `.env` | This is expected behavior |

---

## Setting Up a New Server

1. SSH to the server:
   ```bash
   ssh deploy@server
   ```

2. Copy the template:
   ```bash
   cp /opt/togather/src/deploy/config/environments/.env.staging.example /opt/togather/.env.staging
   ```

3. Fill in required values:
   ```bash
   nano /opt/togather/.env.staging
   # Set DATABASE_URL, JWT_SECRET, ADMIN_PASSWORD at minimum
   ```

4. Secure the file:
   ```bash
   chmod 600 /opt/togather/.env.staging
   ```

5. Deploy from your local machine:
   ```bash
   ./deploy/scripts/deploy.sh staging --version HEAD
   ```

---

## Generating Secrets

```bash
# JWT secret
openssl rand -base64 32

# Admin password (readable characters)
openssl rand -base64 24

# PostgreSQL password
openssl rand -hex 32
```

---

## Related Documentation

- [deploy/config/environments/README.md](../../deploy/config/environments/README.md) — Environment file locations and setup detail
- [docs/deploy/deploy-conf.md](deploy-conf.md) — `.deploy.conf.*` deployment metadata files
- [docs/deploy/quickstart.md](quickstart.md) — Deployment quick start
- [docs/deploy/troubleshooting.md](troubleshooting.md) — Common deployment failures
