# Deployment Rollback Troubleshooting Guide

This guide helps operators troubleshoot common issues when rolling back Togather server deployments.

## Table of Contents

- [Quick Reference](#quick-reference)
- [Common Issues](#common-issues)
- [Rollback Process](#rollback-process)
- [Database Considerations](#database-considerations)
- [Health Check Failures](#health-check-failures)
- [Recovery Procedures](#recovery-procedures)
- [Prevention Best Practices](#prevention-best-practices)

## Quick Reference

### Basic Rollback Command
```bash
server deploy rollback <environment>
```

### Force Rollback (Skip Confirmation)
```bash
server deploy rollback <environment> --force
```

### Check Deployment Status
```bash
server deploy status
server deploy status --format json
```

### Dry Run (Validate Without Executing)
```bash
server deploy rollback <environment> --dry-run
```

### Check Deployment History
```bash
ls -lh /var/lib/togather/deployments/<environment>/
cat /var/lib/togather/deployments/<environment>/current.json
cat /var/lib/togather/deployments/<environment>/previous.json
```

### View Rollback Logs
```bash
ls -lh ~/.togather/logs/rollbacks/
tail -f ~/.togather/logs/rollbacks/rollback_*.log
```

## Common Issues

### Issue: "No previous deployment found"

**Symptoms:**
```
[ERROR] No previous deployment found for environment: production
[ERROR] Previous deployment link does not exist: /var/lib/togather/deployments/production/previous.json
```

**Cause:** This is the first deployment to the environment, or deployment history was cleared.

**Solution:**
1. Check if deployment history directory exists:
   ```bash
   ls -la /var/lib/togather/deployments/production/
   ```

2. List available deployment versions:
   ```bash
   ls -lh /var/lib/togather/deployments/production/*.json
   ```

3. Check deployment status:
   ```bash
   server deploy status
   ```

4. If no history exists, you cannot rollback. Deploy the desired version instead:
   ```bash
   git checkout <target-version>
   cd deploy/scripts
   ./deploy.sh production
   ```

---

### Issue: "Docker image not found locally"

**Symptoms:**
```
[WARN] Docker image not found locally: togather-server:abc123
[INFO] Attempting to rebuild from Git commit: abc123...
```

**Cause:** The Docker image for the target version is not available locally.

**Solution:**
The rollback script automatically rebuilds the image from the Git commit. If rebuild fails:

1. **Ensure Git commit exists:**
   ```bash
   git log --oneline | grep abc123
   ```

2. **Manually rebuild the image:**
   ```bash
   git checkout abc123
   docker build -f deploy/docker/Dockerfile \
     -t togather-server:abc123 \
     --build-arg GIT_COMMIT=abc123... \
     --build-arg GIT_SHORT_COMMIT=abc123 \
     .
   git checkout -  # Return to previous branch
   ```

3. **Pull image from registry (if available):**
   ```bash
   docker pull <registry>/togather-server:abc123
   docker tag <registry>/togather-server:abc123 togather-server:abc123
   ```

---

### Issue: Health checks fail after rollback

**Symptoms:**
```
[ERROR] Health checks failed for blue slot
[ERROR] Rollback may not be successful - manual verification required
```

**Cause:** Previous version is incompatible with current environment state, or infrastructure issues.

**Solutions:**

1. **Check application logs:**
   ```bash
   docker logs togather-development-blue
   docker logs togather-development-green
   ```

2. **Verify database connectivity:**
   ```bash
   # From inside container
   docker exec -it togather-development-blue \
     psql $DATABASE_URL -c "SELECT 1"
   ```

3. **Check container status:**
   ```bash
   docker ps -a | grep togather-development
   ```

4. **Manual health check:**
   ```bash
   server healthcheck --slot blue
   server healthcheck --slot green
   # Or use curl directly:
   curl -f http://localhost:8081/health || echo "Health check failed"
   ```

5. **Review deployment state:**
   ```bash
   cat deploy/config/deployment-state.json | jq .
   ```

---

### Issue: Database schema incompatibility

**Symptoms:**
```
[WARN] Database snapshot available: /var/lib/togather/snapshots/...
[WARN] If database migrations were applied after this deployment, you may need to restore the database
```

**Cause:** Rolled-back application version expects older database schema.

**Solution:**

1. **Check migration status:**
   ```bash
   docker exec -it togather-development-postgres \
     psql -U togather -d togather_dev \
     -c "SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 5"
   ```

2. **Restore database snapshot (DESTRUCTIVE - use with caution):**
   ```bash
   # Stop application containers first
   cd deploy/docker
   docker-compose -f docker-compose.blue-green.yml down blue green
   
    # Restore snapshot (.sql.gz from server snapshot create)
    gunzip -c /var/lib/togather/db-snapshots/<snapshot-file>.sql.gz | psql "$DATABASE_URL"
   
    # Restart application
    server deploy rollback development --force
   ```

3. **Rollback migrations manually (safer but complex):**
   ```bash
   # Run down migrations to target version
   migrate -path internal/storage/postgres/migrations \
           -database $DATABASE_URL \
           down <number-of-migrations>
   ```

4. **Create new database snapshot before attempting rollback:**
   ```bash
   server snapshot create --reason "pre-rollback backup"
   ```

---

### Issue: Traffic not switching to rolled-back slot

**Symptoms:**
```
[WARN] Nginx container not found: togather-production-nginx
[WARN] Skipping traffic switch (direct access only)
```

**Cause:** Nginx load balancer is not running or not configured.

**Solution:**

1. **Check nginx container:**
   ```bash
   docker ps | grep nginx
   ```

2. **If using direct access (no nginx), verify ports:**
   ```bash
   # Blue slot typically on 8081
   curl http://localhost:8081/health
   
   # Green slot typically on 8082
   curl http://localhost:8082/health
   ```

3. **Manually switch traffic (if nginx available):**
   ```bash
   # Update nginx config to point to rolled-back slot
   docker exec togather-production-nginx nginx -s reload
   ```

4. **Access application directly by slot:**
   ```bash
   # Determine active slot
   cat deploy/config/deployment-state.json | jq -r '.current_deployment.active_slot'
   
   # Access via port (blue=8081, green=8082)
   ```

---

### Issue: Rollback completes but application behaves incorrectly

**Symptoms:**
- Application starts but returns errors
- Features missing or broken
- Data inconsistencies

**Cause:** Environment configuration mismatch or state corruption.

**Solution:**

1. **Verify environment configuration:**
   ```bash
   docker exec togather-development-blue env | grep -E "POSTGRES|DATABASE"
   ```

2. **Compare current vs expected environment:**
   ```bash
   cat deploy/config/environments/.env.development
   ```

3. **Check deployment state vs actual running version:**
   ```bash
   # Expected version
   cat deploy/config/deployment-state.json | jq -r '.current_deployment.version'
   
   # Running version
   docker exec togather-development-blue /app/togather-server version
   ```

4. **Redeploy from clean state:**
   ```bash
   # Stop all containers
   cd deploy/docker
   docker-compose -f docker-compose.blue-green.yml down
   
   # Deploy target version
   git checkout <target-version>
   cd deploy/scripts
   ./deploy.sh development
   ```

---

## Rollback Process

### Normal Rollback Flow

1. **Prerequisites validation** - Check docker, docker-compose, jq availability
2. **Deployment history lookup** - Find previous deployment from symlink
3. **User confirmation** - Interactive prompt (unless `--force`)
4. **Docker image verification** - Check if image exists, rebuild if needed
5. **Slot deployment** - Deploy to inactive slot (blue ↔ green)
6. **Health validation** - Run health checks on new slot
7. **Traffic switch** - Switch nginx to rolled-back slot
8. **State update** - Update deployment state file

### Time Expectations

- **Normal rollback:** 1-2 minutes
- **With image rebuild:** 3-5 minutes
- **With database restore:** 5-15 minutes (depends on database size)

### What Gets Rolled Back

✅ **Application code** - Previous Git commit version  
✅ **Docker image** - Tagged with previous commit SHA  
✅ **Active slot** - Traffic switches to rolled-back version  
✅ **Deployment state** - Updated with rollback metadata  

❌ **Database schema** - NOT automatically rolled back (manual restore required)  
❌ **Environment variables** - Uses current .env files  
❌ **External state** - Third-party services, caches, etc.  

---

## Database Considerations

### When to Restore Database Snapshot

**Restore required if:**
- Schema migrations were run after the target deployment
- New tables/columns are referenced by current schema
- Application fails with "column does not exist" errors

**Restore NOT required if:**
- Only application code changed (no schema changes)
- Migrations are backward compatible
- Rolling back within same schema version

### Database Restore Checklist

1. ✅ Stop application containers before restore
2. ✅ Verify snapshot exists and is valid
3. ✅ Create current database backup first
4. ✅ Test restore in development/staging first
5. ✅ Communicate downtime window to users (if production)

### Database Restore Command

```bash
# 1. Stop application
cd deploy/docker
docker-compose -f docker-compose.blue-green.yml down blue green

# 2. Restore snapshot (.sql.gz from server snapshot create)
gunzip -c /var/lib/togather/db-snapshots/<snapshot-file>.sql.gz | psql "$DATABASE_URL"

# 3. Verify restore
psql $DATABASE_URL -c "SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1"

# 4. Restart application
server deploy rollback development --force
```

---

## Health Check Failures

### Debug Health Check Issues

1. **View health check endpoint using CLI:**
   ```bash
   server healthcheck --slot blue --format json
   server healthcheck --slot green --format table
   ```

2. **Or check directly with curl:**
   ```bash
   curl -v http://localhost:8081/health | jq .
   curl http://localhost:8081/health | jq '.checks[] | {name, status, message}'
   ```

3. **Test database connectivity:**
   ```bash
   docker exec togather-development-blue \
     psql $DATABASE_URL -c "SELECT 1"
   ```

4. **Test job queue connectivity:**
   ```bash
   docker exec togather-development-blue \
     psql $DATABASE_URL -c "SELECT count(*) FROM river_job"
   ```

5. **Check application logs for errors:**
   ```bash
   docker logs --tail 100 togather-development-blue
   ```

### Common Health Check Failures

| Check | Failure Cause | Solution |
|-------|--------------|----------|
| `database` | Connection refused | Verify DATABASE_URL, check postgres container |
| `migrations` | Dirty state | Run pending migrations or rollback to clean state |
| `job_queue` | Table not found | Run migrations (river tables missing) |
| Overall timeout | Slow queries | Increase health check timeout, optimize queries |

---

## Recovery Procedures

### Complete Rollback Failure Recovery

If rollback fails completely and application is down:

1. **Check what's running:**
   ```bash
   docker ps -a | grep togather
   ```

2. **Stop all deployment containers:**
   ```bash
   cd deploy/docker
   docker-compose -f docker-compose.blue-green.yml down
   ```

3. **Deploy known-good version:**
   ```bash
   git checkout <known-good-commit>
   cd deploy/scripts
   ./deploy.sh development --force
   ```

4. **If deployment still fails, restore from backup:**
   ```bash
# Restore database from snapshot (.sql.gz)
gunzip -c /var/lib/togather/db-snapshots/<latest-good-snapshot>.sql.gz | psql "$DATABASE_URL"
   
   # Redeploy
   ./deploy.sh development --force
   ```

### Manual Slot Switching

If automatic traffic switching fails:

1. **Determine which slot is running the target version:**
   ```bash
   docker ps | grep togather-development-blue
   docker ps | grep togather-development-green
   ```

2. **Update deployment state manually:**
   ```bash
   cat > deploy/config/deployment-state.json <<EOF
   {
     "current_deployment": {
       "deployment_id": "manual_$(date +%s)",
       "version": "<git-commit>",
       "environment": "development",
       "active_slot": "blue",
       "deployed_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
     }
   }
   EOF
   ```

3. **Reload nginx (if applicable):**
   ```bash
   docker exec togather-development-nginx nginx -s reload
   ```

---

## Prevention Best Practices

### Before Rolling Back

1. ✅ **Verify target version** - Confirm previous version is actually working
2. ✅ **Check deployment history** - Review previous deployment metadata
3. ✅ **Notify team** - Communicate rollback plan (especially for production)
4. ✅ **Test in lower environment** - Rollback staging first if possible
5. ✅ **Backup current state** - Create snapshot before rollback

### During Rollback

1. ✅ **Monitor logs** - Watch rollback logs in real-time
2. ✅ **Check health endpoints** - Verify health checks pass
3. ✅ **Test key features** - Smoke test critical functionality
4. ✅ **Watch metrics** - Monitor error rates, response times

### After Rollback

1. ✅ **Verify application behavior** - Test critical user flows
2. ✅ **Check database state** - Ensure data consistency
3. ✅ **Review logs** - Look for errors or warnings
4. ✅ **Document incident** - Record what went wrong and why rollback was needed
5. ✅ **Plan fix** - Address root cause before attempting deployment again

### Rollback Decision Criteria

**Rollback if:**
- Critical functionality broken
- High error rates (>5%)
- Database corruption or data loss
- Security vulnerability introduced
- Performance degradation >50%

**Fix forward if:**
- Minor UI bugs
- Non-critical feature issues
- Fix is quick (< 5 minutes)
- Rollback would cause data loss

---

## Additional Resources

- **CLI Reference:**
  - `server deploy --help` - Deployment operations
  - `server healthcheck --help` - Health check commands
  - `server snapshot --help` - Database snapshot commands
- **Deployment Guide:** See `deploy/scripts/deploy.sh --help`
- **Migration Guide:** See `migrations.md`

## Getting Help

If you encounter issues not covered in this guide:

1. Check rollback logs: `~/.togather/logs/rollbacks/rollback_*.log`
2. Check deployment logs: `~/.togather/logs/deployments/deploy_*.log`
3. Review deployment history: `/var/lib/togather/deployments/<env>/`
4. Contact the Togather infrastructure team

---

**Last Updated:** 2026-01-28  
**Related Tasks:** US5 - Deployment Rollback (T041-T050)
