# Environment Configurations

This directory contains environment-specific configuration files for different deployment targets.

## Quick Reference: Environment File Locations

**The golden rule: Environment configs live where they're used.**

| Use Case | File Location | Template | Commit? |
|----------|---------------|----------|---------|
| **Local dev (non-Docker)** | `<repo>/.env` | `<repo>/.env.example` | ❌ No |
| **Local dev (Docker)** | `<repo>/deploy/docker/.env` | `<repo>/deploy/docker/.env.example` | ❌ No |
| **Staging server** | `/opt/togather/.env.staging` | `deploy/config/environments/.env.staging.example` | ❌ No |
| **Production server** | `/opt/togather/.env.production` | `deploy/config/environments/.env.production.example` | ❌ No |

### IMPORTANT: What NOT to Do

- ❌ **DO NOT** create `deploy/config/environments/.env.staging` locally
- ❌ **DO NOT** create `deploy/config/environments/.env.production` locally
- ❌ **DO NOT** commit any `.env` files (only `.env.example` templates)
- ❌ **DO NOT** copy production secrets to your local machine
- ✅ **DO** keep secrets on servers where they belong
- ✅ **DO** use templates (`.env.example`) as documentation

## Structure

```
deploy/config/environments/
├── Caddyfile.staging           # Caddy config for staging
├── Caddyfile.production         # Caddy config for production
├── .env.staging.example         # Runtime env template for staging (SERVER-SIDE)
├── .env.production.example      # Runtime env template for production (SERVER-SIDE)
└── .env.development.example     # Runtime env template for local Docker dev
```

## Environment File Setup

### Local Development (Non-Docker)

**Use case**: Running `go run cmd/server/main.go` or `./server` directly

1. Copy template to repo root:
   ```bash
   cp .env.example .env
   ```

2. Edit configuration:
   ```bash
   nano .env
   # Update DATABASE_URL to point to local PostgreSQL
   # Set JWT_SECRET to a dev value
   ```

3. Run server:
   ```bash
   go run cmd/server/main.go
   # or
   make run
   ```

### Local Development (Docker)

**Use case**: Running via `docker-compose up` for testing blue-green deployment locally

1. Copy template:
   ```bash
   cp deploy/docker/.env.example deploy/docker/.env
   ```

2. Edit configuration:
   ```bash
   nano deploy/docker/.env
   # DATABASE_URL should use 'togather-db' as hostname (Docker service name)
   # Update passwords and secrets
   ```

3. Start Docker stack:
   ```bash
   docker-compose -f deploy/docker/docker-compose.blue-green.yml up -d
   ```

**Note**: Root `.env` and `deploy/docker/.env` have different purposes:
- Root `.env`: For local non-Docker Go development
- `deploy/docker/.env`: For local Docker Compose deployment

Keep them in sync for DATABASE_URL (with appropriate hostname differences: `localhost` vs `togather-db`).

### Remote Staging Server

**Use case**: Deploying to staging.toronto.togather.foundation

1. SSH to staging server:
   ```bash
   ssh deploy@staging.toronto.togather.foundation
   ```

2. Copy template from repo (pulled during provision):
   ```bash
   cp /opt/togather/src/deploy/config/environments/.env.staging.example /opt/togather/.env.staging
   ```

3. Edit configuration:
   ```bash
   nano /opt/togather/.env.staging
   # Update DATABASE_URL with staging database credentials
   # Generate new JWT_SECRET: openssl rand -base64 32
   # Set ENVIRONMENT=staging
   # Set LOG_LEVEL=info
   ```

4. Secure permissions:
   ```bash
   chmod 600 /opt/togather/.env.staging
   ```

5. Deploy from local machine:
   ```bash
   # No local .env.staging needed!
   ./deploy/scripts/deploy.sh staging --remote deploy@staging.toronto.togather.foundation
   ```

The deploy script will:
- SSH to server
- Find `/opt/togather/.env.staging` on the server
- Symlink it to the repo location
- Use it for deployment

### Remote Production Server

Same process as staging, but use `.env.production.example` template and create `/opt/togather/.env.production` on the production server.

## File Types

### Caddyfile.* (Reverse Proxy Configs)
- **Purpose**: Caddy reverse proxy configuration for routing traffic
- **Location on server**: `/etc/caddy/Caddyfile`
- **Used by**: Caddy web server
- **Contains**: Domain names, SSL settings, blue-green routing, headers
- **Secrets**: None (safe to commit)

### .env.*.example (Runtime Environment Templates)
- **Purpose**: Templates for application runtime configuration
- **Location on server**: `/opt/togather/.env.{environment}` (after copying and filling in)
- **Used by**: `./server` binary (the application)
- **Contains**: DATABASE_URL, JWT_SECRET, port settings, feature flags
- **Secrets**: YES - do not commit actual values

### Testing Configs (in `deploy/testing/environments/`)
- **Purpose**: Configuration for smoke tests and performance tests
- **Files**: `local.test.env`, `staging.test.env`, `production.test.env`
- **Used by**: Test scripts (`./deploy/scripts/test-remote.sh`, etc.)
- **Contains**: BASE_URL, SSH_SERVER, timeouts, test permissions
- **Secrets**: API keys (set via environment variables, not committed)

## Deployment Workflow

### Staging Deployment

1. **Deploy Caddyfile**:
   ```bash
   scp deploy/config/environments/Caddyfile.staging SERVER:/tmp/
   ssh SERVER 'sudo cp /tmp/Caddyfile.staging /etc/caddy/Caddyfile'
   ssh SERVER 'sudo systemctl reload caddy'
   ```

2. **Deploy application**:
   ```bash
   ./deploy/scripts/deploy.sh staging
   ```

3. **Run smoke tests**:
   ```bash
   make test-staging-smoke
   ```

### Production Deployment

1. **Deploy Caddyfile** (if changed):
   ```bash
   scp deploy/config/environments/Caddyfile.production SERVER:/tmp/
   ssh SERVER 'sudo cp /tmp/Caddyfile.production /etc/caddy/Caddyfile'
   ssh SERVER 'sudo caddy validate --config /etc/caddy/Caddyfile'
   ssh SERVER 'sudo systemctl reload caddy'
   ```

2. **Deploy application**:
   ```bash
   ./deploy/scripts/deploy.sh production
   ```

3. **Run smoke tests**:
   ```bash
   make test-production-smoke
   ```

## Blue-Green Traffic Switching

### Via Caddyfile (Recommended)

Edit the Caddyfile to change the upstream:

**Staging** (`Caddyfile.staging`):
```caddyfile
# Line 29: Change from blue to green
reverse_proxy localhost:8082 {  # Changed from 8081

# Line 41: Update slot header
header_down X-Togather-Slot "green"  # Changed from "blue"
```

Then reload:
```bash
sudo systemctl reload caddy
```

### Verification

```bash
# Check which slot is active
curl -I https://staging.toronto.togather.foundation/health | grep X-Togather-Slot

# Run smoke tests
make test-staging-smoke
```

## Configuration Separation (Why?)

We separate configs by **purpose** and **consumer**:

| Config Type | Purpose | Consumer | Location | Secrets? |
|------------|---------|----------|----------|----------|
| `Caddyfile.*` | Reverse proxy routing | Caddy | `/etc/caddy/` | No |
| `.env.*` | Application runtime | Server binary | `/opt/togather/` | YES |
| `*.test.env` | Testing parameters | Test scripts | `deploy/testing/` | API keys only |

This separation allows:
- ✅ Independent updates (change proxy without touching app)
- ✅ Clear responsibility (one config = one tool)
- ✅ Security (secrets only where needed)
- ✅ Version control (safe to commit proxy configs)

## Related Documentation

- [Testing Infrastructure](../../testing/README.md) - Smoke tests and performance tests
- [Deployment Guide](../../../docs/deploy/quickstart.md) - Full deployment instructions
- [Caddy Guide](../../../docs/deploy/caddy.md) - Caddy-specific setup
