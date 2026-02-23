# API Key Management

API keys are used to authenticate with the SEL API endpoints. This guide covers creating, managing, and configuring API keys for testing and production use.

## Overview

API keys are **infrastructure configuration**, not deployment artifacts. They should be:
- Created once per environment
- Stored securely in environment configuration files
- Reused across deployments
- Rotated periodically for security

**Key Creation Methods:**

1. **Developer Self-Service** (recommended for application developers): Developers create and manage their own keys through the developer portal. See [Developer Self-Service](../integration/authentication.md#developer-self-service) for details.

2. **CLI/Admin Creation** (for infrastructure and testing): Admins create keys via the `server api-key` command for infrastructure services, testing, and special purposes.

The server uses **bcrypt hashing** (hash_version=2) for secure key storage. Legacy SHA256 (hash_version=1) is supported for backward compatibility but should not be used for new keys.

## Creating API Keys

### Using the Server CLI (Recommended)

The `server api-key` command provides a secure way to create and manage API keys:

```bash
# SSH to the target server
ssh togather  # or your server hostname

# Navigate to the deployment directory
cd /opt/togather

# Load environment variables
source .env.staging  # or .env.production

# Create an agent key (for event ingestion)
./server api-key create "Production Agent" --role agent

# Create an admin key (for administrative operations)
./server api-key create "Production Admin" --role admin

# List all API keys
./server api-key list

# Revoke a key (by ID from list command)
./server api-key revoke <key-id>
```

**Output format:**
```
✓ API key created successfully

Key: 01KGJZ40ZG0WQ5SQKCKA6VEH79wKp8mN3vL9xR2qT4yB6nC5dE8fG
Name: Production Agent
Role: agent
Prefix: 01KGJZ40ZG0WQ5SQKCKA6VEH79

⚠️  Save this key securely - it will not be shown again!
```

**IMPORTANT:** Save the full key immediately - it cannot be retrieved later. Only the prefix is stored for identification.

## Scraper Ingestion Key

The `server scrape all` command submits events to a SEL server using `SEL_API_KEY` (or
`SEL_INGEST_KEY`) and `SEL_SERVER_URL`. A dedicated agent-role key must exist **on the
target server** before running scrapes against it.

### First-time setup for a new environment

```bash
# 1. Create the key on the target server (run once per environment)
ssh togather "docker exec togather-server-green /app/server api-key create scraper --role agent"
# Save the printed key — it is shown only once.

# 2. Add to your local .env for scraping against that environment
echo "SEL_SERVER_URL=https://staging.toronto.togather.foundation" >> .env
echo "SEL_API_KEY=<key-from-step-1>" >> .env
```

### Running a scrape

```bash
source .env
./bin/togather-server scrape all --sources configs/sources
```

Or pass the flags directly without sourcing `.env`:

```bash
./bin/togather-server scrape all \
  --server https://staging.toronto.togather.foundation \
  --key <key> \
  --sources configs/sources
```

### Persistent key on staging

The key created above (`scraper-ingest` / role `agent`) should be treated as
infrastructure — store it in `.env` locally alongside `SEL_SERVER_URL`. It does **not**
need to be in `.env.staging` on the remote server; it only needs to exist in the staging
database (which it does once created via `api-key create`).

## Configuring Test Keys

Test API keys are used for performance testing, smoke tests, and integration tests. They should be configured in environment-specific files.

### For Remote Deployments (Staging/Production)

Add keys to the server's environment file:

```bash
# On the remote server
ssh togather
nano /opt/togather/.env.staging

# Add test keys at the end:
# Test API Keys for Performance Testing
PERF_ADMIN_API_KEY=01KGJZ3V49SKAPRK99NTT5PF5W...  # Full key from api-key create
PERF_AGENT_API_KEY=01KGJZ40ZG0WQ5SQKCKA6VEH79...  # Full key from api-key create
```

### For Local Development

Add keys to your local `.env` file:

```bash
# In project root
nano .env

# Add test keys:
PERF_ADMIN_API_KEY=01KGJZ3V49SKAPRK99NTT5PF5W...
PERF_AGENT_API_KEY=01KGJZ40ZG0WQ5SQKCKA6VEH79...
```

### For Deployment Configuration

Optionally add keys to `.deploy.conf.{env}` for deployment scripts:

```bash
# .deploy.conf.staging
PERF_ADMIN_API_KEY=01KGJZ3V49SKAPRK99NTT5PF5W...
PERF_AGENT_API_KEY=01KGJZ40ZG0WQ5SQKCKA6VEH79...
```

## API Key Roles

### Agent Role
- **Purpose:** Event ingestion, place/organization creation
- **Permissions:** Write access to events, places, organizations
- **Rate Limit:** 20,000 requests/day (configurable)
- **Use Cases:** Batch imports, automated scrapers, partner integrations

### Admin Role
- **Purpose:** Administrative operations, system management
- **Permissions:** Full API access including user management
- **Rate Limit:** Unlimited (configurable)
- **Use Cases:** System administration, testing, debugging

## Security Best Practices

### Key Storage
- ✅ Store keys in `.env` files with `chmod 600` permissions
- ✅ Use environment variables, never commit keys to git
- ✅ Rotate keys periodically (every 90 days recommended)
- ❌ Never log full API keys (only prefixes)
- ❌ Never commit `.env` files (use `.env.example` templates)

### Key Rotation
```bash
# 1. Create new key
./server api-key create "Production Agent v2" --role agent

# 2. Update .env.staging with new key
nano /opt/togather/.env.staging

# 3. Test with new key
curl -H "Authorization: Bearer 01NEW..." https://staging.../api/v1/events

# 4. Revoke old key after confirmation
./server api-key list  # Find old key ID
./server api-key revoke <old-key-id>
```

## Testing API Keys

### Verify Keys in Database

```bash
# On remote server
source .env.staging
psql "$DATABASE_URL" -c "SELECT prefix, role, name FROM api_keys;"

# Expected output:
#            prefix           | role  |         name
# ----------------------------+-------+-----------------------
#  01KGJZ3V49SKAPRK99NTT5PF5W | admin | Staging Test Admin
#  01KGJZ40ZG0WQ5SQKCKA6VEH79 | agent | Staging Test Agent
```

### Manual Test
```bash
# Test agent key
curl -H "Authorization: Bearer ${PERF_AGENT_API_KEY}" \
     https://staging.toronto.togather.foundation/api/v1/events?limit=1

# Expected: 200 OK with events array
```

### Using Ingestion Scripts
```bash
# Toronto events ingestion (uses PERF_AGENT_API_KEY from .deploy.conf.staging)
./scripts/ingest-toronto-events.sh staging 50 300
```

### Using Performance Tests
```bash
# Performance tests (uses both PERF_ADMIN_API_KEY and PERF_AGENT_API_KEY)
./deploy/scripts/performance-test.sh --profile light
```

## Troubleshooting

### "Unauthorized" Errors

**Symptom:** API returns 401 Unauthorized

**Causes:**
1. Key doesn't exist in database
2. Key is inactive or expired
3. Wrong key for endpoint (agent key on admin endpoint)
4. Typo in key value

**Debug:**
```bash
# Check if key exists
source .env.staging
psql "$DATABASE_URL" -c "
  SELECT prefix, role, is_active, expires_at 
  FROM api_keys 
  WHERE prefix = '01KGJZ40ZG0WQ5SQKCKA6VEH79';
"

# Check server logs
ssh togather "docker logs togather-server-green --tail 20 | grep -i 'invalid.*key'"
```

**Solutions:**
```bash
# If key doesn't exist, create it:
./server api-key create "Agent Key" --role agent

# If key is inactive, check database:
psql "$DATABASE_URL" -c "
  UPDATE api_keys 
  SET is_active = true 
  WHERE prefix = '01KGJZ40ZG0WQ5SQKCKA6VEH79';
"

# If wrong role, create new key with correct role
```

### Key Created But Not Working

**Symptom:** Key shows in `api-key list` but still returns 401

**Likely cause:** Server container needs restart to pick up database changes

**Solution:**
```bash
# Restart the active slot
ssh togather "docker restart togather-server-green"

# Wait for health check
sleep 5
curl https://staging.toronto.togather.foundation/health

# Test again
curl -H "Authorization: Bearer ${PERF_AGENT_API_KEY}" \
     https://staging.../api/v1/events?limit=1
```

### Creating Keys During Deployment

**Don't do this!** API keys should be created manually as infrastructure configuration, not auto-generated during deployments.

**Why?**
- Keys are long-lived credentials, not ephemeral
- Deployment scripts should be read-only on production databases
- Manual creation ensures proper security review
- Bcrypt hashing is complex and error-prone in shell scripts

## API Key Lifecycle

```
┌─────────────┐
│   Create    │  server api-key create <name> --role <role>
└──────┬──────┘
       │
       v
┌─────────────┐
│   Active    │  Key is stored in .env, used by services
└──────┬──────┘
       │
       v
┌─────────────┐
│   Rotate    │  Create new key, update .env, revoke old
└──────┬──────┘
       │
       v
┌─────────────┐
│  Revoked    │  server api-key revoke <id>
└─────────────┘
```

## Quick Reference

```bash
# Create agent key
./server api-key create "My Agent" --role agent

# Create admin key
./server api-key create "My Admin" --role admin

# List all keys (shows prefix, role, name)
./server api-key list

# Revoke a key
./server api-key revoke <key-id>

# Test a key
curl -H "Authorization: Bearer <full-key>" \
     https://<domain>/api/v1/events?limit=1

# Check key in database
psql "$DATABASE_URL" -c "SELECT * FROM api_keys WHERE prefix = '<prefix>';"
```

## See Also

- [Developer Self-Service](../integration/authentication.md#developer-self-service) - How developers create and manage their own API keys
- [Authentication Guide](../integration/authentication.md) - Complete authentication documentation
- [Deployment Guide](deployment-testing.md)
- [Performance Testing](performance-testing.md)
- [Security Review](../../.opencode/skill/security-review/SKILL.md)
- [Environment Configuration](deploy-conf.md)

---

**Last Updated:** 2026-02-20
