# Togather Server Deployment

Quick start guide for deploying Togather server using the one-command deployment system.

## Prerequisites

- Docker >= 20.10
- Docker Compose >= 2.0
- golang-migrate CLI
- jq, psql (PostgreSQL client tools)
- Git (for version tagging)

## Quick Start

### 1. Configure Environment

```bash
# Copy example environment file
cp deploy/config/environments/.env.production.example deploy/config/environments/.env.production

# Edit with your credentials
nano deploy/config/environments/.env.production
```

**Required Variables:**
- `DATABASE_URL` - PostgreSQL connection string
- `JWT_SECRET` - Secret for JWT token signing
- `PORT` - Server port (default: 8080)

### 2. Deploy

```bash
cd deploy/scripts
./deploy.sh production
```

The script will:
1. Validate configuration and prerequisites
2. Build Docker image with version metadata
3. Create database snapshot (automatic backup)
4. Run database migrations
5. Deploy to blue-green slots
6. Run health checks
7. Switch traffic to new version

### 3. Verify

```bash
# Check health endpoint
curl http://your-server/health

# Expected response:
{
  "status": "healthy",
  "version": "v1.0.0-abc1234",
  "database": "connected",
  "migrations": "up-to-date"
}
```

## Command Reference

### Basic Deployment

```bash
./deploy.sh <environment>
```

**Environments:** `development`, `staging`, `production`

### Validation Only (Dry Run)

```bash
./deploy.sh --dry-run staging
```

Validates configuration, checks prerequisites, and verifies git state without deploying.

### Skip Migrations

```bash
./deploy.sh production --skip-migrations
```

**⚠️ Use with caution!** Skips database snapshot and migration execution. Only use when:
- Deploying a hotfix with no schema changes
- Database migrations already applied manually
- Rollback scenario where schema is already current

### Force Deployment

```bash
./deploy.sh staging --force
```

**⚠️ Dangerous!** Bypasses deployment lock. Only use when:
- Previous deployment crashed and left stale lock
- Emergency hotfix required immediately
- You've verified no other deployment is running

### Show Version

```bash
./deploy.sh --version
```

Displays script version and exits.

## Deployment Process

The deployment follows a zero-downtime blue-green strategy:

### Phase 1: Validation
- Verify required tools installed (docker, docker-compose, migrate, jq, psql)
- Check environment configuration file exists
- Validate git repository is clean and committed
- Ensure deployment.yml configuration is valid

### Phase 2: Lock Acquisition
- Create deployment lock file to prevent concurrent deployments
- Generate unique deployment ID: `YYYYMMDD_HHMMSS_<git-short-commit>`
- Lock file: `/tmp/togather-deploy-<environment>.lock`

### Phase 3: Build
- Build Docker image with multi-stage optimization
- Tag with version metadata: `togather-server:<git-commit>`
- Inject build-time labels (version, commit, timestamp)

### Phase 4: Database Safety
- Create timestamped database snapshot using pg_dump
- Store in `/var/backups/togather/` with format: `togather_<env>_<timestamp>.sql.gz`
- Snapshot includes schema, data, and permissions
- Retention: Keeps last 7 snapshots by default

### Phase 5: Migrations
- Run forward migrations using golang-migrate
- Migrations directory: `internal/storage/postgres/migrations/`
- Transactional execution with automatic rollback on failure
- Version tracking in `schema_migrations` table

### Phase 6: Blue-Green Deploy
- Determine active slot (blue or green)
- Deploy to inactive slot
- Start new containers with updated image
- Wait for services to initialize (10s grace period)

### Phase 7: Health Checks
- **Database connectivity** - PostgreSQL connection and query
- **Migration version** - Schema version matches expected
- **HTTP endpoint** - `/health` returns 200 OK
- **Job queue** - River job system operational

Retries: 30 attempts with 2s intervals (60s total timeout)

### Phase 8: Traffic Switch
- Update Nginx configuration to route to new slot
- Reload Nginx (graceful, zero-downtime)
- Old slot remains running (for rollback if needed)

### Phase 9: State Update
- Write deployment metadata to `deploy/config/deployment-state.json`
- Includes: deployment ID, timestamp, version, environment, active slot
- Release deployment lock

### Phase 10: Cleanup
- Stop containers in old slot after successful deployment
- Log deployment completion with metrics

## Flags

| Flag | Description | Risk Level |
|------|-------------|-----------|
| `--dry-run` | Validate without deploying | Safe |
| `--skip-migrations` | Skip DB snapshot and migrations | ⚠️ Medium |
| `--force` | Bypass deployment lock | ⚠️ High |
| `--version` | Show script version | Safe |
| `--help` | Show usage information | Safe |

## Troubleshooting

### Deployment Lock Exists

**Symptom:** `Deployment lock already exists for environment`

**Diagnosis:**
```bash
# Check if deployment process is running
ps aux | grep deploy.sh

# Check lock file
cat /tmp/togather-deploy-production.lock
```

**Resolution:**
1. If another deployment is actually running, wait for it to complete
2. If lock is stale (no process running):
   ```bash
   rm /tmp/togather-deploy-production.lock
   ./deploy.sh production
   ```
3. For emergency override:
   ```bash
   ./deploy.sh production --force
   ```

### Migration Failure

**Symptom:** `Migration failed` or `Migration version mismatch`

**Diagnosis:**
```bash
# Check deployment logs
tail -f /var/log/togather/deployments/<deployment-id>.log

# Check migration version in database
psql $DATABASE_URL -c "SELECT * FROM schema_migrations ORDER BY version DESC LIMIT 5;"

# Check migration files
ls -la internal/storage/postgres/migrations/
```

**Resolution:**

**Option 1: Restore snapshot and retry**
```bash
# Find latest snapshot
ls -lh /var/backups/togather/

# Restore manually using server snapshot CLI
# Stop the application first to prevent conflicts
docker compose down

# Restore from gzipped snapshot
gunzip -c /var/backups/togather/togather_production_20260128_143022.sql.gz | psql "$DATABASE_URL"

# Or extract first, then restore
gunzip /var/backups/togather/togather_production_20260128_143022.sql.gz
psql "$DATABASE_URL" < /var/backups/togather/togather_production_20260128_143022.sql

# Fix migration issue in code
# Redeploy
./deploy.sh production
```

**Option 2: Manual migration rollback**
```bash
# Rollback one migration
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" down 1

# Fix issue and redeploy
./deploy.sh production
```

### Health Check Failure

**Symptom:** `Health check failed after X attempts`

**Diagnosis:**
```bash
# Check application logs
docker-compose -f deploy/docker/docker-compose.yml -f deploy/docker/docker-compose.blue-green.yml logs app-blue

# Check if service is running
docker-compose ps

# Test health endpoint directly
curl -v http://localhost:8080/health

# Check database connectivity
psql $DATABASE_URL -c "SELECT 1;"
```

**Resolution:**

**Common causes:**
1. **Database not accessible** - Check DATABASE_URL, network, credentials
2. **Migration version mismatch** - Ensure migrations ran successfully
3. **Port conflict** - Check if port 8080 already in use
4. **Resource exhaustion** - Check CPU/memory limits
5. **Configuration error** - Validate .env file syntax

**Steps:**
```bash
# View detailed health check response
docker-compose exec app-blue curl -s http://localhost:8080/health | jq

# Check environment variables loaded
docker-compose exec app-blue env | grep -E "(DATABASE|JWT|PORT)"

# Check database from container
docker-compose exec app-blue psql $DATABASE_URL -c "\dt"
```

### Build Failure

**Symptom:** `Docker build failed`

**Diagnosis:**
```bash
# Check Docker daemon
docker info

# Check disk space
df -h

# Try manual build with verbose output
docker build -f deploy/docker/Dockerfile -t togather-server:test .
```

**Resolution:**
- Ensure Docker daemon running
- Check sufficient disk space (need ~2GB free)
- Verify Dockerfile syntax
- Check network connectivity for dependency downloads

### Rollback

**Scenario:** Deployed version has critical bug

**Manual Rollback Process:**

1. **Identify previous version:**
   ```bash
   # Check deployment state history
   cat deploy/config/deployment-state.json
   
   # Find previous Docker image
   docker images | grep togather-server
   ```

2. **Switch traffic back to old slot:**
   ```bash
   # If blue is broken, switch back to green
   # Edit deploy/docker/nginx.conf
   # Change upstream from blue (port 8080) to green (port 8081)
   
   # Reload Nginx
   docker-compose exec nginx nginx -s reload
   ```

3. **Restore database if needed:**
   ```bash
   # List available snapshots
   server snapshot list
   
   # Restore from gzipped snapshot
   gunzip -c /var/backups/togather/togather_production_<timestamp>.sql.gz | psql "$DATABASE_URL"
   ```

4. **Redeploy working version:**
   ```bash
   # Checkout previous commit
   git checkout <previous-working-commit>
   
   # Deploy
   ./deploy.sh production
   ```

## Architecture

### Blue-Green Deployment

Zero-downtime deployment using two identical production environments:

- **Blue slot:** Port 8080, service name `app-blue`
- **Green slot:** Port 8081, service name `app-green`
- **Nginx proxy:** Routes traffic to active slot
- **Atomic switch:** Nginx config reload switches traffic instantly

### Nginx Configuration Management

The nginx reverse proxy is configured in `deploy/docker/nginx.conf` and handles traffic routing, rate limiting, TLS/SSL termination, and security headers.

#### Testing Configuration Changes

Always test configuration syntax before reloading:

```bash
# Test configuration syntax
docker compose -f docker-compose.blue-green.yml exec nginx nginx -t

# Expected output:
# nginx: the configuration file /etc/nginx/nginx.conf syntax is ok
# nginx: configuration file /etc/nginx/nginx.conf test is successful
```

#### Reloading Configuration (Graceful)

After making changes to `nginx.conf`, reload without dropping connections:

```bash
# Graceful reload (zero downtime)
docker compose -f docker-compose.blue-green.yml exec nginx nginx -s reload

# Verify reload succeeded (check logs)
docker compose -f docker-compose.blue-green.yml logs nginx --tail=20
```

**How Graceful Reload Works:**
1. Nginx master process receives SIGHUP signal
2. New worker processes start with updated configuration
3. Old worker processes finish serving existing requests
4. Old workers shut down gracefully after completing connections
5. **No dropped connections or 502 errors**

#### Manual Traffic Switching

To manually switch traffic between blue and green slots:

```bash
# 1. Edit nginx.conf upstream configuration
nano deploy/docker/nginx.conf

# Find the line (around line 77):
#   server togather-server-blue:8080;
# Change to:
#   server togather-server-green:8080;

# 2. Test configuration
docker compose -f docker-compose.blue-green.yml exec nginx nginx -t

# 3. Reload if test passes
docker compose -f docker-compose.blue-green.yml exec nginx nginx -s reload

# 4. Verify active slot
curl -I http://localhost/health | grep X-Togather-Slot
# Should show: X-Togather-Slot: togather-server-green:8080
```

#### Nginx Configuration Features

The nginx configuration includes production-ready features (see `nginx.conf` for details):

**Rate Limiting:**
- General API: 30 requests/minute per IP
- Batch ingestion: 10 requests/minute per IP  
- Auth endpoints: 5 requests/minute per IP
- Health/metrics: No limits (for monitoring)

**TLS/SSL Support:**
- TLS 1.2/1.3 with modern cipher suites
- Let's Encrypt integration ready (commented out, enable for production)
- OCSP stapling and SSL session caching
- HTTP-to-HTTPS redirect (enable in production)

**Security Headers:**
- Content-Security-Policy (CSP)
- X-Frame-Options, X-Content-Type-Options
- Strict-Transport-Security (HSTS when TLS enabled)
- Permissions-Policy

**To enable TLS/SSL in production:**
1. Obtain certificates (recommended: Let's Encrypt)
2. Uncomment HTTPS server block in `nginx.conf` (lines ~200-350)
3. Update `ssl_certificate` and `ssl_certificate_key` paths
4. Enable HTTP-to-HTTPS redirect
5. Test and reload: `nginx -t && nginx -s reload`

#### Common Nginx Operations

```bash
# View nginx logs
docker compose logs nginx -f

# Check nginx status
docker compose exec nginx nginx -V  # Version and build info

# Check running nginx processes
docker compose exec nginx ps aux | grep nginx

# Test if nginx is responding
curl -I http://localhost/health

# Restart nginx (hard restart, brief downtime)
docker compose restart nginx
```

**When to use reload vs restart:**
- **Reload (`nginx -s reload`)**: Configuration changes only. **Use this!**
- **Restart (`docker compose restart`)**: nginx binary upgrade, debugging crashes. Causes brief downtime.

### Database Snapshots

Automatic backup before every deployment:

- Format: PostgreSQL SQL dump (gzip compressed)
- Location: `/var/backups/togather/`
- Naming: `togather_<env>_<YYYYMMDD_HHMMSS>.sql.gz`
- Retention: Last 7 snapshots kept (configurable)
- Includes: Schema, data, roles, permissions

### Health Checks

Multi-point validation before traffic switch:

1. **Database:** Connection test + query execution
2. **Migrations:** Version check against expected schema
3. **HTTP:** GET `/health` returns 200 with valid JSON
4. **Jobs:** River queue system responding

### State Tracking

Deployment metadata stored in JSON:

```json
{
  "deployment_id": "20260128_143022_abc1234",
  "environment": "production",
  "active_slot": "blue",
  "version": "v1.0.0",
  "git_commit": "abc1234567890abcdef",
  "timestamp": "2026-01-28T14:30:22Z",
  "deployed_by": "deploy-bot",
  "previous_slot": "green"
}
```

Location: `deploy/config/deployment-state.json`

### Logging

Structured JSON logs with automatic secret sanitization:

- Deployment logs: `/var/log/togather/deployments/<deployment-id>.log`
- Application logs: Docker Compose logs (stdout/stderr)
- Log levels: DEBUG, INFO, WARN, ERROR, SUCCESS
- Secrets filtered: DATABASE_URL, JWT_SECRET, API keys

## Files & Directories

```
deploy/
├── README.md                   # This file (quick start guide)
├── ARCHITECTURE.md             # Deployment architecture details
├── config/
│   ├── deployment.yml          # Deployment configuration
│   ├── deployment-state.json   # Current deployment state
│   └── environments/
│       ├── .env.development.example
│       ├── .env.staging.example
│       └── .env.production.example
├── docker/
│   ├── Dockerfile              # Multi-stage app build
│   ├── Dockerfile.postgres     # Custom PostgreSQL with extensions
│   ├── docker-compose.yml      # Base service definitions
│   ├── docker-compose.blue-green.yml  # Blue-green slot configuration
│   ├── nginx.conf              # Traffic routing configuration
│   └── README.md               # Docker-specific documentation
└── scripts/
    ├── deploy.sh               # Main deployment script (853 lines)
    ├── health-check.sh         # Health validation utility
    └── snapshot-db.sh          # Database backup wrapper (deprecated, use: server snapshot)
```

### Key Configuration Files

**deployment.yml** - Deployment behavior:
```yaml
deployment:
  strategy: blue-green
  health_check_retries: 30
  health_check_interval: 2
  grace_period: 10

database:
  snapshot_retention: 7
  backup_location: /var/backups/togather
```

**docker-compose.yml** - Service definitions:
- `app` - Togather server application
- `db` - PostgreSQL 16 with PostGIS, pgvector, pg_trgm
- `nginx` - Reverse proxy for blue-green routing

**docker-compose.blue-green.yml** - Slot configuration:
- `app-blue` - Blue deployment slot (port 8080)
- `app-green` - Green deployment slot (port 8081)

## Security

### Secrets Management

**Never commit secrets to version control:**
```bash
# .env files are git-ignored
cat .gitignore | grep .env
# Output: .env*
#         !.env*.example

# Verify no secrets in git history
git log --all --full-history --source -- "*env*" -- "*secret*"
```

**Protect environment files:**
```bash
# Set restrictive permissions
chmod 600 deploy/config/environments/.env.production

# Verify ownership
ls -la deploy/config/environments/
# Should show: -rw------- (only owner read/write)
```

### Required Secrets

All `.env` files must include:

1. **DATABASE_URL** - PostgreSQL connection string
   - Format: `postgresql://user:password@host:5432/database`
   - Use strong password (16+ chars, mixed case, numbers, symbols)
   - Restrict database user privileges (no SUPERUSER)

2. **JWT_SECRET** - Token signing key
   - Generate: `openssl rand -base64 32`
   - Rotate quarterly
   - Never reuse across environments


### Log Sanitization

All logging automatically sanitizes sensitive patterns:

- `DATABASE_URL=...` → `DATABASE_URL=***REDACTED***`
- `JWT_SECRET=...` → `JWT_SECRET=***REDACTED***`
- `password=...` → `password=***REDACTED***`
- `Authorization: Bearer ...` → `Authorization: Bearer ***REDACTED***`

Implementation: `sanitize_secrets()` function in deploy.sh

### Network Security

**Production checklist:**
- [ ] Use HTTPS (TLS 1.2+) for all external traffic
- [ ] Restrict database access to application subnet only
- [ ] Enable PostgreSQL SSL mode (`sslmode=require`)
- [ ] Configure firewall rules (allow only 80/443 inbound)
- [ ] Use Docker network isolation (no `host` mode)
- [ ] Enable rate limiting in Nginx
- [ ] Set up fail2ban for brute force protection

## Environment-Specific Notes

### Development

```bash
./deploy.sh development
```

- Uses `docker-compose.yml` only (no blue-green)
- Hot-reload enabled (if `air` available)
- Verbose logging (DEBUG level)
- Database: Local PostgreSQL in Docker
- No HTTPS required

### Staging

```bash
./deploy.sh staging
```

- Full blue-green deployment
- Mirrors production configuration
- Uses staging database (separate from production)
- HTTPS recommended
- Integration tests run post-deployment

### Production

```bash
./deploy.sh production
```

- Full blue-green deployment
- Automated database snapshots before migrations
- Health checks with strict timeouts
- HTTPS required
- Deployment locks enforced
- Logging to persistent storage

## Monitoring

### Health Endpoint

```bash
curl http://localhost:8080/health
```

**Response fields:**
- `status` - Overall health: "healthy" | "degraded" | "unhealthy"
- `version` - Deployed version (git tag or commit)
- `database` - Database status: "connected" | "disconnected"
- `migrations` - Migration status: "up-to-date" | "pending" | "unknown"
- `jobs` - Job queue status: "operational" | "degraded"
- `uptime` - Seconds since server start

### Logs

**Deployment logs:**
```bash
tail -f /var/log/togather/deployments/<deployment-id>.log
```

**Application logs:**
```bash
docker-compose logs -f app

# Specific slot
docker-compose logs -f app-blue
docker-compose logs -f app-green

# Filter by level
docker-compose logs app | jq 'select(.level == "ERROR")'
```

**Nginx logs:**
```bash
docker-compose logs -f nginx

# Access log
docker-compose exec nginx tail -f /var/log/nginx/access.log

# Error log
docker-compose exec nginx tail -f /var/log/nginx/error.log
```

### Metrics

Deployment state includes timing metrics:

```bash
cat deploy/config/deployment-state.json | jq
```

**Available metrics:**
- `timestamp` - Deployment start time
- `duration` - Total deployment time (if logged)
- `version` - Deployed version
- `active_slot` - Current active slot (blue/green)

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Deploy to Production

on:
  push:
    tags:
      - 'v*'

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Install dependencies
        run: |
          sudo apt-get update
          sudo apt-get install -y postgresql-client jq
          
      - name: Install golang-migrate
        run: |
          curl -L https://github.com/golang-migrate/migrate/releases/download/v4.17.0/migrate.linux-amd64.tar.gz | tar xvz
          sudo mv migrate /usr/local/bin/
          
      - name: Deploy to production
        env:
          DATABASE_URL: ${{ secrets.DATABASE_URL }}
          JWT_SECRET: ${{ secrets.JWT_SECRET }}
        run: |
          cd deploy/scripts
          ./deploy.sh production
```

### Pre-deployment Checks

Run before deploying:

```bash
# Run full CI pipeline locally
make ci

# Includes:
# - go build (compile check)
# - make test-ci (tests with race detector)
# - make lint-ci (golangci-lint)

# If make ci passes, deploy:
cd deploy/scripts
./deploy.sh production
```

## Further Reading

- **Architecture:** [`deploy/ARCHITECTURE.md`](./ARCHITECTURE.md) - Deployment infrastructure details
- **Specification:** `specs/001-deployment-infrastructure/spec.md`
- **Tasks:** `specs/001-deployment-infrastructure/tasks.md`
- **Docker Details:** `deploy/docker/README.md`
- **SEL Documentation:** `docs/profiles/`
- **Migration Guide:** `internal/storage/postgres/migrations/README.md`

## Support

For issues or questions:

1. Check troubleshooting section above
2. Review deployment logs: `/var/log/togather/deployments/`
3. Check application logs: `docker-compose logs app`
4. Consult architecture guide: `deploy/ARCHITECTURE.md`
5. Review specification: `specs/001-deployment-infrastructure/`

## Version History

- **v1.0.0** (2026-01-28) - Initial deployment system
  - Blue-green zero-downtime deployment
  - Automatic database snapshots
  - Health check validation
  - Deployment locking
  - CLI flags: --dry-run, --skip-migrations, --force, --version
