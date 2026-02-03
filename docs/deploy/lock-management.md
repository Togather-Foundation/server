# Deployment Lock Management

## Overview

The deployment system uses atomic file-based locks to prevent concurrent deployments to the same environment. This document explains how locks work and how to manage them safely.

## Lock Mechanism

- **Lock directory:** `/tmp/togather-deploy-{environment}.lock`
- **State file:** `deploy/config/deployment-state.json`
- **Timeout:** 30 minutes (automatically considered stale)
- **Scope:** Per-environment (production, staging, development)

## Lock States

### Normal Lock (Active Deployment)
```
Lock age: < 30 minutes
Process is running
State file shows locked: true
```

### Stale Lock (Crashed Deployment)
```
Lock age: > 30 minutes
Process not running or deployment crashed
Automatically removed by next deployment attempt
```

### Orphaned Lock (Manual Intervention Required)
```
Lock directory exists but state file says unlocked
OR Process crashed and left lock behind
```

## Checking Lock Status

```bash
# Show current lock status
./deploy/scripts/unlock.sh staging

# Example output:
Deployment Lock Status: staging
============================================
Lock directory exists: /tmp/togather-deploy-staging.lock
drwxrwxr-x 2 deploy deploy 4096 Feb  3 20:25 /tmp/togather-deploy-staging.lock

State file info:
  Locked: true
  Locked by: deploy@localhost
  Locked at: 2026-02-03T20:25:46Z
  Deployment ID: dep_1890d66a41888860_47448a50c1f61cc6be49eaf4
  PID: 12345
  Hostname: localhost
  Lock age: 3m 15s
  ✓ Process 12345 is still running: deploy.sh
```

## Safe Lock Management

### 1. Wait for Deployment to Complete (RECOMMENDED)

```bash
# Wait for lock to be released naturally
./deploy/scripts/unlock.sh staging --wait

# This will:
# - Poll every 5 seconds
# - Timeout after 1 hour
# - Exit when lock is released
```

### 2. Use --force Flag on Deploy (If Stale)

```bash
# Skip lock check and deploy anyway
./deploy/scripts/deploy.sh staging --force

# Use when:
# - Lock is stale (>30 minutes)
# - You've verified no deployment is running
# - Previous deployment crashed
```

### 3. Force Unlock (LAST RESORT)

```bash
# Manually remove lock
./deploy/scripts/unlock.sh staging --force

# This will:
# 1. Ask for confirmation (type 'yes')
# 2. Remove lock directory
# 3. Update state file to unlocked

# ⚠️ ONLY use if:
# - Deployment process crashed
# - Lock is orphaned/stale
# - You've verified no deployment is running (ps aux | grep deploy)
```

## When NOT to Force Unlock

**NEVER force unlock if:**

1. **Active deployment running** - Check `ps aux | grep deploy.sh`
2. **Lock age < 30 minutes** - Wait for timeout or use --wait
3. **Multiple team members** - Coordinate first
4. **Production environment** - Extra caution required

## Common Scenarios

### Scenario 1: Deployment Hung or Crashed

```bash
# 1. Check lock status
./deploy/scripts/unlock.sh staging

# 2. Verify process is not running
ps aux | grep deploy.sh
ssh remote-server "ps aux | grep deploy.sh"

# 3. Force unlock if confirmed dead
./deploy/scripts/unlock.sh staging --force

# 4. Investigate why it crashed
tail -100 ~/.togather/logs/deployments/staging_*.log
```

### Scenario 2: Stale Lock from Yesterday

```bash
# Lock will show:
  Lock age: 1200m 30s  # 20 hours
  ⚠ Lock is stale (>30 minutes)
  ⚠ Process 12345 is not running

# Solution 1: Let deploy.sh handle it
./deploy/scripts/deploy.sh staging  # Auto-removes stale locks

# Solution 2: Manual cleanup
./deploy/scripts/unlock.sh staging --force
```

### Scenario 3: Accidental Lock (You Cancelled with Ctrl+C)

```bash
# The deployment script has traps to clean up on exit
# But if something went wrong:

# 1. Check if process is still running
ps aux | grep deploy.sh

# 2. Kill if still running
kill <PID>

# 3. Clean up lock
./deploy/scripts/unlock.sh staging --force
```

### Scenario 4: Remote Deployment Lock

```bash
# Lock is on remote server, not local

# 1. SSH to remote
ssh deploy@staging.server

# 2. Check lock status there
cd /opt/togather/src
./deploy/scripts/unlock.sh staging

# 3. Force unlock if needed
./deploy/scripts/unlock.sh staging --force
```

## Lock Safety Features

### 1. Atomic Directory Creation
- Uses POSIX `mkdir` which is atomic
- Prevents race conditions between processes

### 2. State File Tracking
- JSON file tracks lock metadata (who, when, pid)
- Enables intelligent stale detection

### 3. Auto-Stale Detection
- Locks >30 minutes are auto-removed
- Next deployment attempt cleans them up

### 4. Process Validation
- Checks if deployment PID is still running
- Warns if process crashed

### 5. Trap Handlers
- Script installs cleanup traps
- Releases lock on exit (normal or error)

## Troubleshooting

### Lock Directory Won't Delete

```bash
# Check if something is holding it open
lsof | grep togather-deploy

# Force remove
sudo rm -rf /tmp/togather-deploy-staging.lock
```

### State File Corrupted

```bash
# Backup first
cp deploy/config/deployment-state.json{,.backup}

# Reset to unlocked
jq '.lock.locked = false' deployment-state.json > temp && mv temp deployment-state.json
```

### Lock on Remote Server

```bash
# SSH and run unlock there
ssh deploy@server "cd /opt/togather/src && ./deploy/scripts/unlock.sh staging --force"

# Or use deploy --force
./deploy/scripts/deploy.sh staging --remote deploy@server --force
```

## Best Practices

1. **Always check status first**: `./deploy/scripts/unlock.sh <env>`
2. **Wait if possible**: Use `--wait` instead of `--force`
3. **Verify process is dead**: `ps aux | grep deploy`
4. **Coordinate with team**: Check if someone else is deploying
5. **Log investigation**: Check deployment logs before force unlock
6. **Use --force flag**: Prefer `deploy.sh --force` over manual unlock

## Related Documentation

- [Deployment Overview](quickstart.md)
- [Remote Deployment](remote-deployment.md)
- [Troubleshooting](troubleshooting.md)
- [Deployment Testing](DEPLOYMENT-TESTING.md)
