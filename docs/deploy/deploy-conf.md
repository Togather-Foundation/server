# Deployment Configuration Files

## Overview

`.deploy.conf.*` files contain per-environment deployment metadata that is **not secret** but should not be committed to git (e.g., SSH hostnames, domain names, city-specific settings).

## Purpose

These files solve several problems:

1. **Prevent hallucination**: Agents and scripts have a single source of truth for environment-specific values
2. **Separate concerns**: Deployment metadata (domains, SSH hosts) separate from runtime secrets (.env files)
3. **Per-city configuration**: Each city/node can have its own domain, SSH connection, etc.
4. **Git safety**: No need to commit environment-specific values that differ per deployment

## File Structure

### Location
- Root directory: `.deploy.conf.{environment}`
- Example: `.deploy.conf.staging`, `.deploy.conf.production`
- Template: `.deploy.conf.example` (committed to git)

### Format
Standard shell variable format (KEY=value):

```bash
# Node domain (used for URI generation and webfiles)
NODE_DOMAIN=staging.toronto.togather.foundation

# SSH connection details
SSH_HOST=togather
SSH_USER=deploy

# City/region identifier
CITY=toronto
REGION=ontario

# Environment identifier
ENVIRONMENT=staging

# Deployment settings
BLUE_GREEN_ENABLED=true
HEALTH_CHECK_TIMEOUT=30
```

## Usage

### For Deployment Scripts

```bash
# Load deployment configuration
if [ -f ".deploy.conf.${ENVIRONMENT}" ]; then
    source ".deploy.conf.${ENVIRONMENT}"
else
    echo "Error: .deploy.conf.${ENVIRONMENT} not found"
    exit 1
fi

# Use variables
echo "Deploying to ${NODE_DOMAIN} via ${SSH_USER}@${SSH_HOST}"
```

### For Agents

When asked to deploy to an environment, agents should:

1. Check if `.deploy.conf.{environment}` exists
2. Read the file to get NODE_DOMAIN, SSH_HOST, etc.
3. Use these values instead of guessing or hallucinating

Example agent prompt:
```
Deploy to staging environment:
1. Read .deploy.conf.staging for deployment metadata
2. Run: ./deploy/scripts/deploy.sh staging --remote ${SSH_USER}@${SSH_HOST}
3. Test deployment at https://${NODE_DOMAIN}/health
```

## Configuration Keys

### Required Keys

| Key | Description | Example |
|-----|-------------|---------|
| `NODE_DOMAIN` | Fully qualified domain name for this node | `staging.toronto.togather.foundation` |
| `SSH_HOST` | SSH hostname or IP for deployment | `togather` or `deploy@192.168.1.100` |
| `SSH_USER` | SSH username for deployment | `deploy` |
| `ENVIRONMENT` | Environment identifier | `staging`, `production` |

### Optional Keys

| Key | Description | Default |
|-----|-------------|---------|
| `CITY` | City name for this node | `toronto` |
| `REGION` | Region/province/state | `ontario` |
| `BLUE_GREEN_ENABLED` | Enable blue-green deployment | `true` |
| `HEALTH_CHECK_TIMEOUT` | Health check timeout in seconds | `30` |
| `RATE_LIMIT_PUBLIC` | Public read requests per minute | `60` |
| `RATE_LIMIT_AGENT` | Authenticated write requests per minute | `300` |
| `RATE_LIMIT_ADMIN` | Admin requests per minute | `0` (unlimited) |
| `RATE_LIMIT_LOGIN` | Login attempts per 15 minutes | `5` |
| `RATE_LIMIT_FEDERATION` | Federation sync requests per minute | `500` |
| `PERF_ADMIN_API_KEY` | Perf API key with admin role (sensitive) | (unset) |
| `PERF_AGENT_API_KEY` | Perf API key with agent role (sensitive) | (unset) |

## Setup

### Creating Your Configuration

1. Copy the example file:
   ```bash
   cp .deploy.conf.example .deploy.conf.staging
   ```

2. Edit with your environment details:
   ```bash
   vim .deploy.conf.staging
   ```

3. Verify it's gitignored:
   ```bash
   git status  # Should not show .deploy.conf.staging
   ```

### For New Environments

When setting up a new city/node:

1. Create `.deploy.conf.{city}` with the node's domain and SSH details
2. Ensure the node domain follows the pattern: `{environment}.{city}.togather.foundation`
3. Update deployment scripts to support the new environment name

## Security

### What Goes Here (Safe)
- Domain names
- SSH hostnames (public IPs or hostnames)
- SSH usernames
- City/region names
- Non-sensitive deployment settings

### What Stays in .env (Secret)
- Database URLs with credentials
- JWT secrets
- API keys
- Passwords
- Private keys

Note: Perf API keys are sensitive. If you store them in `.deploy.conf.*`, protect the file and avoid sharing it.

## Integration

### Deployment Scripts

The `deploy.sh` script should be updated to:

1. Auto-detect environment from argument
2. Source `.deploy.conf.{environment}`
3. Use variables from the config file
4. Fall back to command-line arguments if config not found

### Test Scripts

Test scripts like `test-deployment.sh` should:

1. Accept environment name
2. Read `.deploy.conf.{environment}` for NODE_DOMAIN
3. Run tests against that domain

## Related Files

- `.env` files: Runtime secrets (gitignored)
- `.env.example`: Runtime config template (committed)
- `.deploy.conf.example`: Deployment config template (committed)
- `deploy/scripts/deploy.sh`: Main deployment script
- `docs/deploy/quickstart.md`: Deployment documentation

## Migration Guide

### From Manual Configuration

If you currently pass values via command line:

**Before:**
```bash
./deploy/scripts/deploy.sh staging --remote deploy@togather --domain staging.toronto.togather.foundation
```

**After:**
```bash
# Create .deploy.conf.staging with NODE_DOMAIN and SSH_HOST
./deploy/scripts/deploy.sh staging
```

### From Environment Variables

If you use environment variables:

```bash
# Old way
export NODE_DOMAIN=staging.toronto.togather.foundation
./deploy/scripts/deploy.sh staging

# New way (more discoverable, gitignored)
# Put NODE_DOMAIN in .deploy.conf.staging
./deploy/scripts/deploy.sh staging
```

## Troubleshooting

### File Not Found

```
Error: .deploy.conf.staging not found
```

**Solution**: Copy `.deploy.conf.example` to `.deploy.conf.staging` and customize.

### Wrong Domain

If deployment uses wrong domain, check:

1. `.deploy.conf.{environment}` has correct NODE_DOMAIN
2. File is being sourced correctly by deployment script
3. No conflicting environment variables override it

### SSH Connection Fails

Check:

1. SSH_HOST is correct (hostname, IP, or SSH config alias)
2. SSH_USER has permission to deploy
3. SSH keys are set up correctly

## Future Enhancements

- Auto-generate `.deploy.conf` from interactive setup script
- Validate config file format on deployment
- Support multiple cities per environment (production-toronto, production-montreal)
- Integration with secrets management (AWS Parameter Store, Vault)

---

**Last Updated:** 2026-02-20
