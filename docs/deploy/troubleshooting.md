# Troubleshooting Guide

Concise guide for diagnosing common issues in Togather server deployments.

## Installation Issues

### Automated Installation Failed

If `sudo ./install.sh` fails:

1. **Check the installation log:**
   ```bash
   cat /var/log/togather-install.log
   ```

2. **Follow manual installation steps:**
   See [manual-install.md](./manual-install.md) for step-by-step instructions to identify where the process failed.

3. **Common installation failures:**
   - **Docker permission denied**: User not in docker group
     ```bash
     sudo usermod -aG docker $USER
     newgrp docker
     ```
   - **Port already in use**: Check ports 8080, 5433
     ```bash
     sudo lsof -i :8080
     sudo lsof -i :5433
     ```
   - **Disk full**: Need 2GB+ free space
     ```bash
     df -h /opt
     ```

## Quick Diagnosis

### Health Check Status

```bash
# Check overall health
curl http://localhost:8080/health | jq .

# Expected healthy response:
{
  "status": "healthy",
  "version": "0.1.0",
  "git_commit": "abc123",
  "slot": "blue",
  "checks": {
    "database": {"status": "pass", "message": "PostgreSQL connection successful"},
    "migrations": {"status": "pass", "message": "Migrations applied successfully (version 5)"},
    "http_endpoint": {"status": "pass", "message": "HTTP endpoint responding"},
    "job_queue": {"status": "pass", "message": "River job queue operational"}
  },
  "timestamp": "2026-01-30T18:00:00Z"
}
```

### Common Health Statuses

- **healthy** - All checks passed
- **degraded** - Some checks returned warnings
- **unhealthy** - At least one check failed

### Quick Checks

```bash
# Check deployment state (on server)
cat /opt/togather/src/deploy/config/deployment-state.json | jq .

# Check running containers
docker ps -a | grep togather

# View deployment logs
tail -f ~/.togather/logs/deployments/<env>_<timestamp>.log

# Check application logs
docker logs togather-server-<slot>
```

---

## Deployment Issues

### Issue: Deployment Fails to Start

**Symptom:**
```
[ERROR] Deployment failed to start
[ERROR] Container exited with code 1
```

**Diagnosis:**
```bash
# Check deployment logs
tail -100 ~/.togather/logs/deployments/<env>_*.log

# Check Docker logs
docker logs togather-server-blue

# Check environment config (on server)
cat /opt/togather/.env.production
```

**Common Causes:**

1. **Missing required environment variables**
   ```bash
   # Verify all required vars are set
   grep CHANGE_ME /opt/togather/.env.production
   
   # If any CHANGE_ME placeholders exist, deployment will fail
   # Edit .env.production and set real values
   nano /opt/togather/.env.production
   ```

2. **Database connection failure**
   ```bash
   # Test database connectivity
   psql "$DATABASE_URL" -c "SELECT 1"
   
   # Common fixes:
   # - Verify PostgreSQL is running: docker ps | grep togather-db
   # - Check POSTGRES_HOST/PORT/DB/USER/PASSWORD in .env
   # - For production, ensure POSTGRES_SSLMODE=require or verify-full
   ```

3. **Port already in use**
   ```bash
   # Check what's using port 8080
   sudo lsof -i :8080
   
   # Kill conflicting process or change port in .env
   ```

---

### Issue: Deployment Lock Exists

**Symptom:**
```
[ERROR] Another deployment is in progress
[ERROR] Lock file: /tmp/togather-deploy-production.lock
```

**Diagnosis:**
```bash
# Check lock directory
ls -la /tmp/togather-deploy-production.lock

# Check if deployment process is running
ps aux | grep deploy.sh
```

**Resolution:**

**If deployment is still running:**
```bash
# Wait for deployment to complete
# Monitor progress
tail -f ~/.togather/logs/deployments/<env>_<timestamp>.log
```

**If deployment crashed (stale lock):**
```bash
# Verify no deployment running
ps aux | grep deploy.sh

# Remove stale lock
rm -rf /tmp/togather-deploy-production.lock

# Retry deployment
cd /opt/togather/src
./deploy/scripts/deploy.sh production
```

---

### Issue: Blue-Green Switch Fails

**Symptom:**
```
[ERROR] Failed to switch traffic to green slot
[ERROR] Caddy reload failed
```

**Diagnosis:**
```bash
# Check Caddy configuration
sudo caddy validate --config /etc/caddy/Caddyfile

# Check current traffic routing
curl -I https://<domain>/health | grep -i x-togather-slot

# Check deployment state
jq '.current_deployment.active_slot' /opt/togather/src/deploy/config/deployment-state.json
```

**Resolution:**

1. **Fix Caddy configuration syntax error**
   ```bash
   # Validate configuration
   sudo caddy validate --config /etc/caddy/Caddyfile
   
   # If syntax error, edit Caddyfile
   sudo nano /etc/caddy/Caddyfile
   
   # Reload Caddy
   sudo systemctl reload caddy
   ```

2. **Manual traffic switch**
   ```bash
   # Update Caddyfile to point to the target slot (8081 or 8082)
   sudo nano /etc/caddy/Caddyfile
   sudo systemctl reload caddy
   ```

---

## Health Check Issues

### Issue: Database Check Fails

**Symptom:**
```json
{
  "status": "unhealthy",
  "checks": {
    "database": {
      "status": "fail",
      "message": "Database query failed",
      "details": {"error": "connection refused"}
    }
  }
}
```

**Diagnosis:**
```bash
# Check PostgreSQL is running
docker ps | grep togather-db

# Test database connection
psql "$DATABASE_URL" -c "SELECT 1"

# Check connection pool stats
curl http://localhost:8080/health | jq '.checks.database.details'
```

**Common Causes:**

1. **PostgreSQL not running**
   ```bash
   # Start PostgreSQL
    docker compose -f /opt/togather/src/deploy/docker/docker-compose.yml up -d togather-db
   
   # Check logs
    docker logs togather-db
   ```

2. **Wrong credentials**
   ```bash
   # Verify credentials in .env file
    cat /opt/togather/.env | grep POSTGRES
   
   # Test connection with explicit credentials
    PGPASSWORD=your_password psql -h localhost -U togather -d togather -c "SELECT 1"
   ```

3. **Connection pool exhausted**
   ```bash
   # Check pool stats in health response
   curl http://localhost:8080/health | jq '.checks.database.details'
   
   # If acquired_connections == max_connections, increase pool size
   # Edit .env and set DB_MAX_CONNECTIONS=50 (default: 25)
   ```

---

### Issue: Migration Check Fails (Dirty State)

**Symptom:**
```json
{
  "checks": {
    "migrations": {
      "status": "fail",
      "message": "Database in dirty migration state",
      "details": {
        "version": 5,
        "dirty": true,
        "remediation": "See migrations.md for recovery steps"
      }
    }
  }
}
```

**Diagnosis:**
```bash
# Check migration status
migrate -path /opt/togather/src/internal/storage/postgres/migrations -database "$DATABASE_URL" version

# Query schema_migrations table
psql "$DATABASE_URL" -c "SELECT * FROM schema_migrations;"
```

**Resolution:**

See [migrations.md](./migrations.md#issue-2-dirty-migration-state) for detailed recovery steps.

See [rollback.md](./rollback.md) for snapshot restore steps.

---

### Issue: Job Queue Check Warns (Table Missing)

**Symptom:**
```json
{
  "status": "degraded",
  "checks": {
    "job_queue": {
      "status": "warn",
      "message": "River job queue table not found (migrations not yet applied)"
    }
  }
}
```

**Diagnosis:**
```bash
# Check if river_job table exists
psql "$DATABASE_URL" -c "\dt river_job"

# Check migration status
migrate -path /opt/togather/src/internal/storage/postgres/migrations -database "$DATABASE_URL" version
```

**Resolution:**

This is expected during early development or initial deployment.

```bash
# Run migrations to create River tables
cd /opt/togather/src
./deploy/scripts/deploy.sh <environment>

# Or run migrations manually
migrate -path /opt/togather/src/internal/storage/postgres/migrations -database "$DATABASE_URL" up

# Verify job queue health
curl http://localhost:8080/health | jq '.checks.job_queue'
```

---

### Issue: Health Check Returns 503 (Shutting Down)

**Symptom:**
```json
{
  "status": "shutting_down"
}
```

**Diagnosis:**
```bash
# Check if graceful shutdown is in progress
   docker logs togather-server-<slot> | grep -i shutdown

# Check deployment state
   jq . /opt/togather/src/deploy/config/deployment-state.json
```

**Resolution:**

This is **expected behavior** during:
- Blue-green deployment switch
- Container restart
- Manual shutdown

**What happens:**
1. Server receives SIGTERM
2. Health endpoint returns 503
3. Load balancer stops sending new traffic
4. Existing requests complete (up to 30s grace period)
5. Container exits cleanly

**If stuck in shutdown:**
```bash
# Check for hung connections
    docker exec togather-server-<slot> netstat -an | grep :8080

# Force restart if necessary (last resort)
   docker restart togather-server-<slot>
```

---

## Migration Issues

For comprehensive migration troubleshooting, see [migrations.md](./migrations.md).

### Quick Reference

| Issue | Quick Fix |
|-------|-----------|
| **Migration lock exists** | `rm /tmp/togather-migration-production.lock` (if stale) |
| **Dirty migration state** | Restore from snapshot, fix migration, redeploy (see rollback.md) |
| **Migration failed** | Restore from snapshot, fix SQL syntax, test locally (see rollback.md) |
| **Version mismatch** | Check git branch, ensure codebase matches database state |
| **Connection timeout** | Increase migration timeout in deploy.sh |
| **Permission denied** | Grant necessary privileges to database user |

---

## Container Issues

### Issue: Container Keeps Restarting

**Symptom:**
```bash
$ docker ps -a | grep togather
togather-server-blue   Restarting (1) 2 seconds ago
```

**Diagnosis:**
```bash
# Check why container is restarting
docker logs togather-server-blue --tail 50

# Check exit code
docker inspect togather-server-blue | jq '.[0].State'

# Common exit codes:
# - 1: General error (check logs for panic/fatal)
# - 137: Killed by OOM (out of memory)
# - 139: Segmentation fault
```

**Resolution:**

1. **Out of Memory (exit 137)**
   ```bash
   # Increase memory limit in docker-compose.yml
   nano /opt/togather/src/deploy/docker/docker-compose.yml
   
   # Update limits:
   # deploy:
   #   resources:
   #     limits:
   #       memory: 1G  # Increase from 512M
   
   # Restart with new limits
    docker compose -f /opt/togather/src/deploy/docker/docker-compose.yml up -d --force-recreate
   ```

2. **Application panic/fatal error**
   ```bash
   # Check logs for panic stack trace
docker logs togather-server-blue 2>&1 | grep -A 20 "panic:"
   
   # Common causes:
   # - Database connection failure
   # - Missing environment variables
   # - Configuration parsing error
   
   # Fix configuration and redeploy
   ```

---

### Issue: Container Cannot Connect to Database

**Symptom:**
```
[ERROR] Failed to connect to database
dial tcp: lookup togather-db: no such host
```

**Diagnosis:**
```bash
# Check if PostgreSQL container is running
  docker ps | grep togather-db

# Check Docker network
docker network inspect togather-net

# Verify POSTGRES_HOST setting
cat /opt/togather/.env | grep POSTGRES_HOST
```

**Resolution:**

1. **Using Docker Compose (containers on same network)**
   ```bash
   # POSTGRES_HOST should be container name, not 'localhost'
   # Edit .env:
  POSTGRES_HOST=togather-db  # NOT localhost
   
   # Restart services
  docker compose -f /opt/togather/src/deploy/docker/docker-compose.yml restart
   ```

2. **Using external PostgreSQL**
   ```bash
   # Use actual hostname/IP
   POSTGRES_HOST=db.example.com
   # OR
   POSTGRES_HOST=10.0.1.100
   
   # Ensure PostgreSQL allows external connections
   # Edit postgresql.conf:
   listen_addresses = '*'
   
   # Edit pg_hba.conf to allow connections from Docker network
   ```

---

## Configuration Issues

### Issue: Environment Variables Not Set

**Symptom:**
```
[FATAL] Required environment variable not set: JWT_SECRET
```

**Diagnosis:**
```bash
# Check which variables are undefined
docker exec togather-server-blue env | sort

# Check .env file for CHANGE_ME placeholders
grep CHANGE_ME /opt/togather/.env.production
```

**Resolution:**

1. **Generate missing secrets**
   ```bash
   # Generate JWT secret
   openssl rand -base64 32
   
   # Generate API key
   openssl rand -hex 32
   
   # Edit .env file
nano /opt/togather/.env.production
   
   # Replace CHANGE_ME with generated values
   JWT_SECRET=<generated-value>
   API_KEY_SECRET=<generated-value>
   ```

2. **Verify all required variables**
   ```bash
   # Required for all environments:
   POSTGRES_HOST
   POSTGRES_PORT
   POSTGRES_DB
   POSTGRES_USER
   POSTGRES_PASSWORD
   JWT_SECRET
   
   # Required for production:
POSTGRES_SSLMODE=require
TLS_ENABLED=true
   ```

---

### Issue: Invalid Configuration Syntax

**Symptom:**
```
[ERROR] Failed to parse configuration
yaml: line 42: mapping values are not allowed in this context
```

**Diagnosis:**
```bash
# Validate YAML syntax
python3 -c "import yaml; yaml.safe_load(open('deploy/config/deployment.yml'))"

# Or use yamllint if installed
yamllint deploy/config/deployment.yml
```

**Resolution:**

Common YAML syntax errors:

```yaml
# ❌ Wrong (inconsistent indentation)
server:
  port: 8080
   host: 0.0.0.0  # Extra space

# ✅ Correct
server:
  port: 8080
  host: 0.0.0.0

# ❌ Wrong (missing quotes for special chars)
password: p@ssw0rd!

# ✅ Correct
password: "p@ssw0rd!"

# ❌ Wrong (tab characters instead of spaces)
database:
	host: localhost  # Tab character

# ✅ Correct (use spaces)
database:
  host: localhost  # Two spaces
```

---

## Network Issues

### Issue: Cannot Access Application

**Symptom:**
```bash
$ curl http://localhost:8080/health
curl: (7) Failed to connect to localhost port 8080: Connection refused
```

**Diagnosis:**
```bash
# Check if port is exposed
docker ps | grep togather

# Check if application is listening
docker exec togather-server-blue netstat -tlnp | grep 8080

# Check firewall rules (if applicable)
sudo ufw status
```

**Resolution:**

1. **Port not mapped**
   ```bash
   # Check docker-compose.yml port mapping
    cat /opt/togather/src/deploy/docker/docker-compose.yml | grep -A 5 "ports:"
   
   # Should have:
   # ports:
   #   - "8080:8080"  # host:container
   
   # Recreate container with port mapping
    docker compose -f /opt/togather/src/deploy/docker/docker-compose.yml up -d --force-recreate
   ```

2. **Application not binding to 0.0.0.0**
   ```bash
   # Check SERVER_HOST in .env
   # Should be:
   SERVER_HOST=0.0.0.0  # NOT 127.0.0.1
   
   # Edit and restart
nano /opt/togather/.env
docker compose -f /opt/togather/src/deploy/docker/docker-compose.yml restart
   ```

3. **Firewall blocking port**
   ```bash
   # Allow port 8080
   sudo ufw allow 8080/tcp
   
   # Or disable firewall temporarily for testing
   sudo ufw disable
   ```

---

### Issue: Caddy Returns 502 Bad Gateway

**Symptom:**
```bash
$ curl http://localhost/health
<html>
<head><title>502 Bad Gateway</title></head>
<body>
<center><h1>502 Bad Gateway</h1></center>
</body>
</html>
```

**Diagnosis:**
```bash
# Check Caddy logs
sudo journalctl -u caddy -n 100

# Check upstream containers
docker ps | grep togather-server

# Test direct connection to upstream
curl http://localhost:8081/health
```

**Common Causes:**

1. **Upstream container not running**
   ```bash
   # Check active slot
    jq '.current_deployment.active_slot' /opt/togather/src/deploy/config/deployment-state.json
   
   # Ensure that slot's container is running
    docker ps | grep togather-server-blue  # if active_slot is "blue"
   ```

2. **Nginx configuration error**
   ```bash
    # Validate Caddy config
    sudo caddy validate --config /etc/caddy/Caddyfile
   ```

3. **Port mismatch**
   ```bash
    # Verify reverse_proxy target port (8081 or 8082)
    grep -n "reverse_proxy" /etc/caddy/Caddyfile
   ```

**Resolution:**

```bash
# Restart Caddy
sudo systemctl restart caddy
```

---

## Performance Issues

### Issue: Slow Response Times

**Symptom:**
Health checks or API requests take >1 second to respond.

**Diagnosis:**
```bash
# Check database latency
curl http://localhost:8080/health | jq '.checks.database.latency_ms'

# Check database connection pool
curl http://localhost:8080/health | jq '.checks.database.details'

# Check system resources
docker stats togather-server-blue
```

**Common Causes:**

1. **Database connection pool exhausted**
   ```bash
   # Check pool utilization
   curl http://localhost:8080/health | jq '.checks.database.details'
   
   # If acquired_connections is near max_connections:
   # Increase pool size in .env
   DB_MAX_CONNECTIONS=50  # Default: 25
   
   # Restart to apply
    docker compose -f /opt/togather/src/deploy/docker/docker-compose.yml restart
   ```

2. **Database query slow**
   ```bash
   # Enable slow query logging in PostgreSQL
    docker exec togather-db psql -U togather -c \
     "ALTER SYSTEM SET log_min_duration_statement = 1000;"  # Log queries >1s
   
   # Reload PostgreSQL
    docker exec togather-db psql -U togather -c "SELECT pg_reload_conf();"
   
   # Check slow query log
    docker logs togather-db | grep "duration:"
   ```

3. **CPU/Memory constrained**
   ```bash
   # Check resource usage
docker stats togather-server-blue
   
   # If CPU near 100% or memory near limit:
   # Increase limits in docker-compose.yml
nano /opt/togather/src/deploy/docker/docker-compose.yml
   
   # Update:
   # deploy:
   #   resources:
   #     limits:
   #       cpus: '2.0'
   #       memory: 1G
   ```



See [rollback.md](./rollback.md) for rollback procedures and database recovery.

## Getting Help

### Collect Diagnostic Information

When reporting issues, include:

```bash
# 1. Health check output
curl http://localhost:8080/health | jq . > health.json

# 2. Deployment state
jq . /opt/togather/src/deploy/config/deployment-state.json > deployment-state.json

# 3. Recent deployment logs
tail -500 ~/.togather/logs/deployments/<env>_*.log > deployment.log

# 4. Application logs
docker logs togather-server-blue --tail 500 > application.log

# 5. Container status
docker ps -a > docker-ps.txt

# 6. Environment info
cat /opt/togather/.env.production | sed 's/=.*$/=REDACTED/' > env-redacted.txt

# 7. Database migration status
migrate -path /opt/togather/src/internal/storage/postgres/migrations -database "$DATABASE_URL" version > migration-status.txt
```

### Log Locations

- **Deployment logs**: `~/.togather/logs/deployments/<env>_<timestamp>.log`
- **Application logs**: `docker logs togather-server-<slot>`
- **Caddy logs**: `sudo journalctl -u caddy -n 100`
- **PostgreSQL logs**: `docker logs togather-db`
- **Snapshots**: `/var/lib/togather/db-snapshots/`

### Common Log Patterns

Search logs for these patterns:

```bash
# Errors
grep -i "error\|fatal\|panic" <log-file>

# Database issues
grep -i "database\|postgres\|migration" <log-file>

# Authentication issues
grep -i "auth\|jwt\|unauthorized" <log-file>

# Deployment progress
grep "\[INFO\]" <log-file>

# Performance issues
grep -i "slow\|timeout\|latency" <log-file>
```

---

## Additional Resources

- **Migration Troubleshooting**: [migrations.md](./migrations.md)
- **Rollback Procedures**: [rollback.md](./rollback.md)
- **CI/CD Setup**: [ci-cd.md](./ci-cd.md)
- **Quick Start Guide**: [quickstart.md](./quickstart.md)
- **Deployment README**: [deploy/README.md](../README.md)

For production incidents, follow the incident response process in your organization's runbook.
