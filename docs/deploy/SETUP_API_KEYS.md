# Setting Up Staging Test API Keys

Quick guide to set up test API keys on staging for the first time.

## SSH to Staging

```bash
ssh togather
cd /opt/togather
```

## Create Test API Keys

```bash
# Load environment
source .env.staging

# Create agent key for ingestion
./server api-key create "Staging Test Agent" --role agent

# Save output (example):
# Key: 01KGJZ40ZG0WQ5SQKCKA6VEH79wKp8mN3vL9xR2qT4yB6nC5dE8fG
#      ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

# Create admin key for tests
./server api-key create "Staging Test Admin" --role admin

# Save output (example):
# Key: 01KGJZ3V49SKAPRK99NTT5PF5WxYz7aB3cD4eF5gH6iJ7kL8mN9oP
#      ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
```

## Add to Environment Files

### On Remote Server

```bash
# Add to staging env
nano /opt/togather/.env.staging

# Append at the end:
# Test API Keys for Performance Testing
PERF_ADMIN_API_KEY=01KGJZ3V49SKAPRK99NTT5PF5WxYz7aB3cD4eF5gH6iJ7kL8mN9oP
PERF_AGENT_API_KEY=01KGJZ40ZG0WQ5SQKCKA6VEH79wKp8mN3vL9xR2qT4yB6nC5dE8fG

# Save and exit (Ctrl+O, Enter, Ctrl+X)
```

### On Local Machine

```bash
# Update local deploy config
nano .deploy.conf.staging

# Add the same keys:
PERF_ADMIN_API_KEY=01KGJZ3V49SKAPRK99NTT5PF5WxYz7aB3cD4eF5gH6iJ7kL8mN9oP
PERF_AGENT_API_KEY=01KGJZ40ZG0WQ5SQKCKA6VEH79wKp8mN3vL9xR2qT4yB6nC5dE8fG
```

## Test the Keys

```bash
# From local machine
cd /path/to/server

# Test ingestion script
./scripts/ingest-toronto-events.sh staging 50 10

# Expected: ✓ Batch 1 of 1: 10 events submitted successfully
```

## Verify in Database

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

## Security

**IMPORTANT:** 
- These keys grant access to staging data
- Keep `.env.staging` with `chmod 600` permissions
- Never commit these keys to git
- `.deploy.conf.staging` is gitignored
- Rotate keys every 90 days

## Troubleshooting

If ingestion fails with 401 Unauthorized:

```bash
# 1. Verify key exists
ssh togather
cd /opt/togather
source .env.staging
psql "$DATABASE_URL" -c "SELECT prefix, is_active FROM api_keys WHERE role = 'agent';"

# 2. Test key manually
curl -H "Authorization: Bearer $PERF_AGENT_API_KEY" \
     https://staging.toronto.togather.foundation/api/v1/events?limit=1

# 3. If still failing, restart server to reload
docker restart togather-server-green
sleep 5
```

See [API Key Management](api-keys.md) for complete troubleshooting guide.
