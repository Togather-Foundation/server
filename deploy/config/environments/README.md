# Environment Configurations

This directory contains environment-specific configuration files for different deployment targets.

## Structure

```
deploy/config/environments/
├── Caddyfile.staging           # Caddy config for staging
├── Caddyfile.production         # Caddy config for production
├── .env.staging.example         # Runtime env template for staging
├── .env.production.example      # Runtime env template for production
└── .env.development.example     # Runtime env template for local dev
```

## File Types

### Caddyfile.* (Reverse Proxy Configs)
- **Purpose**: Caddy reverse proxy configuration for routing traffic
- **Location on server**: `/etc/caddy/Caddyfile`
- **Used by**: Caddy web server
- **Contains**: Domain names, SSL settings, blue-green routing, headers
- **Secrets**: None (safe to commit)

### .env.*.example (Runtime Environment Templates)
- **Purpose**: Templates for application runtime configuration
- **Location on server**: `/opt/togather/.env` (after copying and filling in)
- **Used by**: `./server` binary (the application)
- **Contains**: DATABASE_URL, JWT_SECRET, port settings, feature flags
- **Secrets**: YES - do not commit actual values

### Testing Configs (in `deploy/testing/environments/`)
- **Purpose**: Configuration for smoke tests and performance tests
- **Files**: `local.test.env`, `staging.test.env`, `production.test.env`
- **Used by**: Test scripts (`make test-staging-smoke`, etc.)
- **Contains**: BASE_URL, timeouts, test permissions
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
- [Caddy Deployment](../../../docs/deploy/CADDY-DEPLOYMENT.md) - Caddy-specific setup
