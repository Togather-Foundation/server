# Deployment Best Practices

Best practices and operational guidelines for deploying and maintaining Togather server in production.

## Table of Contents

1. [Pre-Deployment Checklist](#pre-deployment-checklist)
2. [Deployment Timing](#deployment-timing)
3. [Monitoring and Observability](#monitoring-and-observability)
4. [Rollback Procedures](#rollback-procedures)
5. [Security Best Practices](#security-best-practices)
6. [Database Management](#database-management)
7. [Configuration Management](#configuration-management)
8. [Incident Response](#incident-response)
9. [Performance Optimization](#performance-optimization)
10. [Maintenance Windows](#maintenance-windows)

---

## Pre-Deployment Checklist

### Before Every Deployment

Run through this checklist before deploying to production:

```bash
# 1. Code Quality
[ ] All tests pass locally: make test
[ ] Linter passes: make lint
[ ] Integration tests pass: make test-ci
[ ] Code reviewed and approved

# 2. Configuration
[ ] .env.production has no CHANGE_ME placeholders
[ ] Database connection string verified
[ ] SSL/TLS enabled (POSTGRES_SSLMODE=require or verify-full)
[ ] Secrets rotated if needed (JWT_SECRET, API_KEY_SECRET)
[ ] Resource limits appropriate for load

# 3. Infrastructure
[ ] PostgreSQL backup recent (<24h)
[ ] Disk space sufficient (>20% free)
[ ] Database connection pool sized appropriately
[ ] Health check endpoints responding

# 4. Migration Safety
[ ] Migration tested on staging database
[ ] Migration can be rolled back (has .down.sql)
[ ] No destructive changes (DROP TABLE, DROP COLUMN)
[ ] Snapshot retention policy allows rollback window

# 5. Monitoring
[ ] Alert channels configured (email/Slack/PagerDuty)
[ ] Dashboard shows current baseline metrics
[ ] Log aggregation working (if used)
[ ] On-call rotation up to date

# 6. Communication
[ ] Stakeholders notified of deployment window
[ ] Rollback owner identified
[ ] Incident response plan reviewed
```

### Automated Pre-Flight Checks

Before deployment, the system runs these checks automatically:

```bash
# Run pre-flight checks manually
cd deploy/scripts
./deploy.sh production --dry-run

# What gets checked:
# ✓ Configuration files exist and are valid
# ✓ No CHANGE_ME placeholders in production config
# ✓ Database accessible and migrations up to date
# ✓ Docker containers can be built
# ✓ Sufficient disk space for snapshots
# ✓ No existing deployment locks
```

---

## Deployment Timing

### Recommended Deployment Windows

**Production:**
- **Best**: Tuesday-Thursday, 10am-2pm local time
- **Avoid**: Friday afternoons, weekends, holidays
- **Never**: During peak traffic hours or critical events

**Why Tuesday-Thursday?**
- Monday: Let weekend issues surface first
- Friday: Limited time for incident response
- Tuesday-Thursday: Full team available, adequate recovery time

### Traffic Considerations

Check traffic patterns before deploying:

```bash
# Example: Check current load
docker stats togather-production-blue

# If CPU >70% or Memory >80%, consider waiting
# Blue-green deployments cause temporary 2x resource usage
```

**Deployment Impact:**
- During switch: ~5 seconds of dual resource usage
- New container warm-up: ~30 seconds
- Database connection pool initialization: ~10 seconds

**Safe to deploy when:**
- CPU usage <70%
- Memory usage <80%
- Response times normal (<500ms p95)
- Error rate <1%

---

## Monitoring and Observability

### Essential Metrics to Monitor

**Application Health:**
```bash
# Health check status (poll every 30s)
curl http://localhost:8080/health | jq '.status'

# Key metrics to track:
# - Overall status (healthy/degraded/unhealthy)
# - Database check latency
# - Database connection pool utilization
# - Migration version
# - Active job queue depth
```

**System Resources:**
```bash
# Container stats
docker stats togather-production-blue --no-stream

# Monitor:
# - CPU %
# - Memory usage / limit
# - Network I/O
# - Block I/O
```

**Database:**
```bash
# Connection count
psql "$DATABASE_URL" -c "SELECT count(*) FROM pg_stat_activity;"

# Long-running queries
psql "$DATABASE_URL" -c "
  SELECT pid, now() - query_start AS duration, query 
  FROM pg_stat_activity 
  WHERE state = 'active' AND now() - query_start > interval '5 seconds';
"

# Database size
psql "$DATABASE_URL" -c "
  SELECT pg_size_pretty(pg_database_size('togather'));
"
```

### Setting Up Alerts

**Critical Alerts (Page on-call):**
- Health check returns unhealthy for >2 minutes
- Error rate >5% for >2 minutes
- Database connection failures
- Disk space <10%
- Container restarts >3 times in 10 minutes

**Warning Alerts (Notify team channel):**
- Health check degraded for >5 minutes
- Response time p95 >1s for >5 minutes
- Database connection pool >80% utilized
- Disk space <20%
- Memory usage >85%

**Example: Health Check Alert (Prometheus)**
```yaml
groups:
  - name: togather_alerts
    rules:
      - alert: TogatherUnhealthy
        expr: togather_health_status{status="unhealthy"} == 1
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "Togather server is unhealthy"
          description: "Health check has been failing for 2+ minutes"
      
      - alert: TogatherDegraded
        expr: togather_health_status{status="degraded"} == 1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Togather server is degraded"
          description: "Some health checks are warning for 5+ minutes"
```

### Post-Deployment Monitoring

**First 5 minutes after deployment:**
```bash
# Watch health checks
watch -n 5 'curl -s http://localhost:8080/health | jq ".status"'

# Monitor logs for errors
docker logs -f togather-production-green | grep -i "error\|fatal\|panic"

# Check database connection pool
watch -n 10 'curl -s http://localhost:8080/health | jq ".checks.database.details"'
```

**What to look for:**
- Health status stays "healthy"
- No error spikes in logs
- Response times similar to pre-deployment
- Database pool not exhausted
- No container restarts

**First 30 minutes:**
- Error rate <1%
- p95 latency within 20% of baseline
- No unusual log patterns
- Database query performance stable

**First 24 hours:**
- Monitor for slow memory leaks
- Watch for batch job failures
- Check database growth rate
- Review error logs for new patterns

---

## Rollback Procedures

### When to Rollback

**Rollback immediately if:**
- Health checks fail for >5 minutes
- Error rate >5%
- Critical functionality broken (events/places/organizations CRUD)
- Database queries timing out
- Memory leak detected (unbounded growth)
- Security vulnerability discovered

**Rollback during business hours if:**
- Non-critical feature broken
- Performance degraded >30%
- Minor errors affecting <10% of requests

### Rollback Decision Tree

```
Is production impacted?
├─ Yes → Is it critical? (data loss, security, total outage)
│  ├─ Yes → ROLLBACK IMMEDIATELY
│  └─ No → Can it wait until business hours?
│     ├─ Yes → Schedule rollback, notify stakeholders
│     └─ No → ROLLBACK NOW
└─ No → Monitor closely, prepare rollback plan
```

### Rollback Execution

**Automatic Rollback (Recommended):**
```bash
cd deploy/scripts
server deploy rollback production

# What it does:
# 1. Confirms rollback intent
# 2. Switches traffic to previous slot
# 3. Stops failed deployment
# 4. Updates deployment state
# 5. Logs rollback reason
```

**Force Rollback (No Confirmation):**
```bash
# Use when every second counts
server deploy rollback production --force
```

**Database Rollback (If Migrations Applied):**
```bash
# Only if migrations were run during failed deployment

# 1. List snapshots
server snapshot list

# 2. Restore from pre-deployment snapshot
gunzip -c /var/lib/togather/db-snapshots/togather_production_<timestamp>.sql.gz | psql "$DATABASE_URL"

# 3. Verify database state
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" version
```

**Post-Rollback:**
1. Verify health checks pass
2. Confirm critical functionality works
3. Notify stakeholders of rollback
4. Create incident report
5. Plan fix and re-deployment

See [rollback.md](./rollback.md) for detailed procedures.

---

## Security Best Practices

### Environment Configuration

**Production .env Requirements:**

```bash
# ✅ Required for Production
POSTGRES_SSLMODE=require                    # Or verify-full
TLS_ENABLED=true                            # Always enable TLS
JWT_SECRET=<64-char-random-hex>            # Strong secret
API_KEY_SECRET=<64-char-random-hex>        # Strong secret

# ❌ Never in Production
DEBUG=true                                  # Disable debug mode
POSTGRES_SSLMODE=disable                    # Require SSL
ALLOW_INSECURE_COOKIES=true                # Use secure cookies
CORS_ALLOW_ALL=true                        # Restrict CORS
```

**Generating Strong Secrets:**
```bash
# JWT secret (256-bit)
openssl rand -hex 32

# API key secret (256-bit)
openssl rand -hex 32

# Database password (128-bit, base64)
openssl rand -base64 24
```

### Secret Management

**Best Practices:**
1. **Never commit secrets to git**
   - Use `.gitignore` for `.env*` files
   - Rotate secrets if accidentally committed

2. **Rotate secrets regularly**
   - JWT secrets: Every 90 days
   - Database passwords: Every 180 days
   - API keys: On suspected compromise

3. **Use secret management tools** (recommended for production)
   - HashiCorp Vault
   - AWS Secrets Manager
   - Azure Key Vault
   - Google Secret Manager

4. **Limit secret access**
   - Different secrets per environment
   - Principle of least privilege
   - Audit secret access

### TLS/SSL Configuration

**PostgreSQL SSL (Production):**
```bash
# Minimum (require encrypted connection)
POSTGRES_SSLMODE=require

# Recommended (verify certificate)
POSTGRES_SSLMODE=verify-full
POSTGRES_SSLROOTCERT=/path/to/ca-certificate.crt
```

**Application TLS:**
```bash
# Enable HTTPS in production
TLS_ENABLED=true
TLS_CERT_FILE=/path/to/certificate.crt
TLS_KEY_FILE=/path/to/private.key

# Let's Encrypt recommended for public deployments
```

**Caddy TLS:**

TLS is handled by Caddy automatically when it terminates HTTPS. See `docs/deploy/caddy-deployment.md` for the default config and operational guidance.

### Rate Limiting

Rate limiting is implemented in the Togather application layer. If you need reverse proxy rate limits, use a Caddy rate limit plugin and monitor 429s in application logs.

### Security Headers

Prefer app-level security headers. If you need proxy-level headers, add them to your Caddyfile.

---

## Database Management

### Backup Strategy

**Automatic Snapshots:**
- Created before every deployment (via `server snapshot`)
- Retained for 7 days by default
- Stored in `/var/lib/togather/db-snapshots/` or S3

**Manual Backups:**
```bash
# Create snapshot
server snapshot create --reason "before_risky_change"

# List snapshots
server snapshot list

# Restore snapshot
gunzip -c /var/lib/togather/db-snapshots/<snapshot>.sql.gz | psql "$DATABASE_URL"
```

**Backup Best Practices:**
1. Test restores monthly
2. Store backups off-site (S3/cloud storage)
3. Encrypt backups at rest
4. Monitor backup success/failure
5. Document restore procedures

**Backup Retention:**
- Development: 1 day
- Staging: 7 days
- Production: 30 days (increase via `RETENTION_DAYS` in `.env`)

### Migration Best Practices

**Writing Safe Migrations:**

```sql
-- ✅ Good: Idempotent, can retry safely
CREATE TABLE IF NOT EXISTS users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL
);

-- ❌ Bad: Fails if table exists
CREATE TABLE users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL
);

-- ✅ Good: Check before adding column
DO $$ 
BEGIN
  IF NOT EXISTS (
    SELECT FROM information_schema.columns 
    WHERE table_name='users' AND column_name='email'
  ) THEN
    ALTER TABLE users ADD COLUMN email TEXT;
  END IF;
END $$;

-- ❌ Bad: Fails if column exists
ALTER TABLE users ADD COLUMN email TEXT;

-- ✅ Good: Add column with default, backfill separately
ALTER TABLE users ADD COLUMN status TEXT DEFAULT 'active';

-- ❌ Bad: Locks table during backfill
ALTER TABLE users ADD COLUMN status TEXT;
UPDATE users SET status = 'active'; -- Locks entire table
```

**Migration Testing Checklist:**
```bash
# 1. Test on dev database
DATABASE_URL="postgresql://localhost/togather_dev" \
  migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" up

# 2. Verify schema changes
psql "$DATABASE_URL" -c "\d+ tablename"

# 3. Test rollback (.down.sql)
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" down 1

# 4. Verify rollback
psql "$DATABASE_URL" -c "\d+ tablename"

# 5. Re-apply migration
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" up

# 6. Test on staging with production-size data
# 7. Monitor migration duration
```

**Migration Performance:**
- Migrations should complete in <5 minutes
- For long-running migrations (>5min), consider:
  - Adding index CONCURRENTLY (doesn't lock table)
  - Breaking into smaller migrations
  - Running during maintenance window
  - Using background job for data backfills

### Database Connection Pool

**Sizing Guidelines:**

```bash
# Development (low traffic)
DB_MAX_CONNECTIONS=10

# Staging (moderate traffic)
DB_MAX_CONNECTIONS=25

# Production (high traffic)
DB_MAX_CONNECTIONS=50  # Or more based on load

# Formula: (max_concurrent_requests / avg_query_duration_seconds) * safety_factor
# Example: (100 req/s / 0.1s) * 2 = 20 connections minimum
```

**Monitor Pool Health:**
```bash
# Check pool utilization
curl http://localhost:8080/health | jq '.checks.database.details'

# If acquired_connections approaches max_connections:
# 1. Increase max_connections
# 2. Optimize slow queries
# 3. Add read replicas
# 4. Enable connection pooling (PgBouncer)
```

**PostgreSQL Max Connections:**
```bash
# Check current PostgreSQL max connections
psql "$DATABASE_URL" -c "SHOW max_connections;"

# Increase if needed (requires restart)
# Edit postgresql.conf:
max_connections = 200

# Rule: PostgreSQL max_connections > (sum of all app pools + admin connections)
```

---

## Configuration Management

### Environment Separation

**Never share configurations between environments:**

| Setting | Development | Staging | Production |
|---------|------------|---------|------------|
| `POSTGRES_SSLMODE` | `disable` | `require` | `verify-full` |
| `DEBUG` | `true` | `false` | `false` |
| `LOG_LEVEL` | `debug` | `info` | `warn` |
| `RATE_LIMIT` | `1000/min` | `100/min` | `30/min` |
| `CORS_ORIGINS` | `*` | `staging.example.com` | `example.com` |
| `TLS_ENABLED` | `false` | `true` | `true` |

### Configuration Validation

**Before deployment, validate configuration:**

```bash
# Check for CHANGE_ME placeholders
grep -r "CHANGE_ME" /opt/togather/.env.production

# Validate required variables are set
required_vars=(
  POSTGRES_HOST
  POSTGRES_PORT
  POSTGRES_DB
  POSTGRES_USER
  POSTGRES_PASSWORD
  JWT_SECRET
  API_KEY_SECRET
)

for var in "${required_vars[@]}"; do
  if ! grep -q "^${var}=" /opt/togather/.env.production; then
    echo "ERROR: Missing required variable: $var"
    exit 1
  fi
done

# Test database connection
psql "$(grep DATABASE_URL /opt/togather/.env.production | cut -d'=' -f2-)" -c "SELECT 1"
```

### Configuration Change Process

1. **Make changes in version control**
   ```bash
   # Edit configuration file
   nano /opt/togather/.env.staging
   
   # Commit change
   # Do not commit .env files
   git commit -m "Update staging database pool size"
   ```

2. **Test on non-production environment**
   ```bash
   # Deploy to staging
   ./deploy.sh staging
   
   # Verify change took effect
   curl http://staging.example.com/health | jq .
   ```

3. **Deploy to production during maintenance window**
   ```bash
   # Deploy to production
   ./deploy.sh production
   
   # Monitor for issues
   watch -n 5 'curl -s http://localhost:8080/health | jq ".status"'
   ```

4. **Document change**
   - Update runbook if operational procedures changed
   - Notify team of new configuration
   - Update alerts if thresholds changed

---

## Incident Response

### Incident Severity Levels

**SEV1 (Critical):** Complete outage, data loss, security breach
- **Response time:** Immediate
- **Escalation:** Page on-call immediately
- **Communication:** Notify stakeholders every 30 minutes

**SEV2 (Major):** Partial outage, degraded performance, critical feature broken
- **Response time:** Within 15 minutes
- **Escalation:** Notify on-call, engage if not resolved in 30min
- **Communication:** Notify stakeholders every hour

**SEV3 (Minor):** Non-critical feature broken, performance degraded <30%
- **Response time:** Within 2 hours
- **Escalation:** Notify team channel
- **Communication:** Include in next status update

### Incident Response Process

**1. Detect**
```bash
# Monitoring alert fires
# Or manual detection via health check
curl http://localhost:8080/health | jq .
```

**2. Triage**
```bash
# Assess severity
# - Is production impacted?
# - How many users affected?
# - Is data at risk?

# Determine if rollback is needed
# (See "When to Rollback" section above)
```

**3. Mitigate**
```bash
# If rollback needed:
cd deploy/scripts
server deploy rollback production --force

# If not rolling back:
# - Scale resources if performance issue
# - Restart container if crashed
# - Fix configuration if misconfigured
```

**4. Communicate**
```bash
# Notify stakeholders
# - What's impacted
# - What you're doing about it
# - When you'll provide next update

# Example:
# "We're experiencing elevated error rates on the events API.
#  The team is investigating and will provide an update in 30 minutes."
```

**5. Investigate**
```bash
# Collect diagnostic information (see troubleshooting.md)
curl http://localhost:8080/health | jq . > health.json
docker logs togather-production-blue --tail 500 > app.log
cat deploy/config/deployment-state.json > state.json
```

**6. Resolve**
```bash
# Apply fix
# - Deploy hotfix if needed
# - Roll back if appropriate
# - Adjust configuration
# - Scale resources

# Verify resolution
curl http://localhost:8080/health | jq '.status'  # Should be "healthy"
```

**7. Document**
- Create incident report (what/when/why/how)
- Identify root cause
- List action items to prevent recurrence
- Update runbooks if needed

### Incident Communication Template

```
**Incident Update**

**Status:** [Investigating/Identified/Monitoring/Resolved]
**Severity:** [SEV1/SEV2/SEV3]
**Impact:** [What's affected, % of users impacted]

**What happened:**
[Brief description of the issue]

**What we're doing:**
[Actions being taken]

**Next update:**
[Time of next update]

**Timeline:**
- [HH:MM] Issue detected
- [HH:MM] On-call paged
- [HH:MM] Root cause identified
- [HH:MM] Fix applied
- [HH:MM] Service restored
```

---

## Performance Optimization

### Application Performance

**Monitoring Performance:**
```bash
# Check response times
curl -w "@curl-format.txt" -o /dev/null -s http://localhost:8080/api/v1/events

# curl-format.txt:
time_namelookup:  %{time_namelookup}s\n
time_connect:     %{time_connect}s\n
time_appconnect:  %{time_appconnect}s\n
time_pretransfer: %{time_pretransfer}s\n
time_starttransfer: %{time_starttransfer}s\n
time_total:       %{time_total}s\n
```

**Performance Targets:**
- **p50 latency:** <100ms
- **p95 latency:** <500ms
- **p99 latency:** <1000ms
- **Error rate:** <0.1%
- **Availability:** >99.9% (uptime)

**Common Optimizations:**

1. **Database Query Optimization**
   ```bash
   # Enable slow query logging
   docker exec togather-postgres psql -U togather -c \
     "ALTER SYSTEM SET log_min_duration_statement = 1000;"  # Log queries >1s
   
   # Reload config
   docker exec togather-postgres psql -U togather -c "SELECT pg_reload_conf();"
   
   # Review slow queries
   docker logs togather-postgres | grep "duration:"
   
   # Add indexes for common queries
   psql "$DATABASE_URL" -c "CREATE INDEX CONCURRENTLY idx_events_start_time ON events(start_time);"
   ```

2. **Connection Pool Tuning**
   ```bash
   # Monitor pool utilization
   curl http://localhost:8080/health | jq '.checks.database.details'
   
   # If acquired_connections near max:
   DB_MAX_CONNECTIONS=50  # Increase pool size
   
   # If many idle connections:
   DB_IDLE_CONNECTION_TIMEOUT=30s  # Reduce idle timeout
   ```

3. **Resource Limits**
   ```bash
   # Increase if CPU/memory constrained
   # Edit deploy/docker/docker-compose.yml:
   deploy:
     resources:
       limits:
         cpus: '2.0'
         memory: 1G
       reservations:
         cpus: '1.0'
         memory: 512M
   ```

### Database Performance

**Regular Maintenance:**
```bash
# Analyze tables (updates query planner statistics)
psql "$DATABASE_URL" -c "ANALYZE;"

# Vacuum (reclaims space, prevents transaction ID wraparound)
psql "$DATABASE_URL" -c "VACUUM;"

# Vacuum full (more aggressive, requires downtime)
# Only during maintenance windows
psql "$DATABASE_URL" -c "VACUUM FULL;"

# Reindex (rebuilds indexes)
psql "$DATABASE_URL" -c "REINDEX DATABASE togather;"
```

**Automated Maintenance (recommended):**
```bash
# Enable autovacuum (should already be on)
psql "$DATABASE_URL" -c "SHOW autovacuum;"  # Should be 'on'

# Tune autovacuum for your workload
psql "$DATABASE_URL" -c "
  ALTER SYSTEM SET autovacuum_naptime = '1min';  # Check more frequently
  ALTER SYSTEM SET autovacuum_vacuum_scale_factor = 0.1;  # Vacuum at 10% dead tuples
"

# Reload config
psql "$DATABASE_URL" -c "SELECT pg_reload_conf();"
```

**Index Optimization:**
```bash
# Find missing indexes (queries without index usage)
psql "$DATABASE_URL" -c "
  SELECT schemaname, tablename, attname, n_distinct, correlation
  FROM pg_stats
  WHERE schemaname = 'public'
  ORDER BY n_distinct DESC;
"

# Find unused indexes (candidates for removal)
psql "$DATABASE_URL" -c "
  SELECT schemaname, tablename, indexname, idx_scan
  FROM pg_stat_user_indexes
  WHERE idx_scan = 0 AND indexname NOT LIKE '%_pkey';
"

# Remove unused indexes
psql "$DATABASE_URL" -c "DROP INDEX CONCURRENTLY idx_unused;"
```

---

## Maintenance Windows

### Planning Maintenance Windows

**When to schedule:**
- **Frequency:** Monthly for routine maintenance, ad-hoc for urgent updates
- **Duration:** 1-4 hours depending on work scope
- **Timing:** Outside business hours, low-traffic periods

**Communication Timeline:**
- **7 days before:** Announce maintenance window
- **3 days before:** Reminder email
- **1 day before:** Final reminder
- **Start of window:** Status page update "Maintenance in progress"
- **End of window:** Status page update "Maintenance complete"

### Maintenance Window Checklist

**Pre-Maintenance (1 week before):**
```bash
# 1. Create full database backup
server snapshot create --reason "pre-maintenance"

# 2. Test all changes on staging
./deploy.sh staging

# 3. Document rollback plan

# 4. Notify stakeholders

# 5. Prepare incident response team
```

**During Maintenance:**
```bash
# 1. Put site in maintenance mode (optional)
# Configure Caddy to return 503 with custom page

# 2. Create pre-maintenance snapshot
server snapshot create --reason "pre-maintenance"

# 3. Execute maintenance tasks
# - Deploy new version
# - Run database maintenance
# - Update configuration
# - Scale resources

# 4. Run smoke tests
curl http://localhost:8080/health
curl http://localhost:8080/api/v1/events

# 5. Monitor for issues
watch -n 5 'curl -s http://localhost:8080/health | jq ".status"'
```

**Post-Maintenance:**
```bash
# 1. Remove maintenance mode

# 2. Verify all systems operational

# 3. Monitor for 30 minutes

# 4. Update status page "All systems operational"

# 5. Send completion email
```

### Zero-Downtime Deployments

Most deployments should be zero-downtime using blue-green strategy:

```bash
# Blue-green deployment (no downtime)
cd deploy/scripts
./deploy.sh production

# What happens:
# 1. New version deployed to inactive slot (green)
# 2. Health checks verify green is healthy
# 3. Traffic switched from blue to green (~5s)
# 4. Old version (blue) remains running for rollback
```

**Maintenance windows are ONLY needed for:**
- Breaking schema changes (column removal, table drops)
- Database major version upgrades
- Infrastructure changes (network reconfiguration)
- Security patches requiring restart of all services

---

## Additional Resources

- **Troubleshooting Guide:** [troubleshooting.md](./troubleshooting.md)
- **Rollback Procedures:** [rollback.md](./rollback.md)
- **Migration Guide:** [migrations.md](./migrations.md)
- **CI/CD Integration:** [ci-cd.md](./ci-cd.md)
- **Quick Start:** [quickstart.md](./quickstart.md)
- **Deployment README:** [../README.md](../README.md)

For questions or clarification, consult the team runbook or contact the platform team.
