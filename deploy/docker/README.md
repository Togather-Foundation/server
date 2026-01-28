# Docker Build Instructions

This directory contains the multi-stage Dockerfile for building and running the Togather SEL Server.

## Quick Start

Build the image:

```bash
docker build \
  --build-arg VERSION=$(git describe --tags --always --dirty) \
  --build-arg GIT_COMMIT=$(git rev-parse --short HEAD) \
  --build-arg BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ") \
  -f deploy/docker/Dockerfile \
  -t togather-server:latest \
  .
```

Run the server:

```bash
docker run -p 8080:8080 togather-server:latest serve
```

Check version:

```bash
docker run --rm togather-server:latest version
```

## Build Arguments

The Dockerfile accepts three build arguments for version metadata:

- **VERSION**: Semantic version or git tag (default: `dev`)
- **GIT_COMMIT**: Git commit hash (default: `unknown`)
- **BUILD_DATE**: ISO 8601 build timestamp (default: `unknown`)

These values are embedded into the binary and accessible via the `version` command.

## Image Details

### Multi-Stage Build

**Stage 1: Builder (golang:1.25-alpine)**
- Downloads Go dependencies (cached layer)
- Builds static binary with CGO_ENABLED=0
- Embeds version metadata via ldflags
- Strips debug symbols for minimal binary size

**Stage 2: Runtime (alpine:latest)**
- Minimal Alpine Linux base (~5MB)
- CA certificates for HTTPS
- Non-root user (`togather` UID/GID 1000)
- Final image size: ~18MB content, ~66MB on disk

### Security Features

- Runs as non-root user `togather:togather`
- Static binary (no dynamic dependencies)
- Minimal attack surface (Alpine base)
- CA certificates included for secure outbound connections
- No unnecessary tools or shells in runtime image

### Exposed Ports

- **8080**: HTTP API server

## Docker Compose

For local development and single-node deployments with PostgreSQL, use the included `docker-compose.yml`.

### Quick Start with Docker Compose

1. **Copy the environment template**:
   ```bash
   cp deploy/docker/.env.example deploy/docker/.env
   ```

2. **Edit `.env` and set secure passwords** (search for `CHANGE_ME`):
   ```bash
   # Edit these critical values:
   POSTGRES_PASSWORD=your_secure_db_password
   JWT_SECRET=your_secure_jwt_secret_min_64_chars
   ADMIN_API_KEY=your_secure_api_key
   ADMIN_PASSWORD=your_secure_admin_password
   ```

3. **Start all services** (app + database):
   ```bash
   cd deploy/docker
   docker-compose up -d
   ```

4. **Check service status**:
   ```bash
   docker-compose ps
   docker-compose logs -f app
   ```

5. **Verify health**:
   ```bash
   curl http://localhost:8080/health
   ```

6. **Stop services**:
   ```bash
   docker-compose down
   ```

### Docker Compose Services

**postgres** (PostgreSQL 16 with PostGIS):
- Container: `togather-db`
- Port: `5432` (exposed on localhost)
- Extensions: PostGIS, pgvector, pg_trgm, pg_stat_statements
- Volume: `togather-db-data` (persistent storage)
- Health check: `pg_isready` every 10s
- Initialization: `init-db.sh` enables required extensions

**app** (Togather Server):
- Container: `togather-server`
- Port: `8080` (exposed on localhost)
- Depends on: `postgres` (waits for healthy status)
- Environment: Configured via `.env` file
- Health check: `/app/server healthcheck` every 30s
- Build context: Repository root (multi-stage Dockerfile)

### Persistent Data

Database data is stored in a named Docker volume:
- **togather-db-data**: PostgreSQL data directory
- **togather-db-snapshots**: Database backups (for migrations)

To backup data:
```bash
docker-compose exec postgres pg_dump -U togather togather > backup.sql
```

To reset data (⚠️ destroys all data):
```bash
docker-compose down -v  # -v flag removes volumes
```

### Environment Variables

All configuration is managed through the `.env` file. Key variables:

- **POSTGRES_PASSWORD**: Database password (required)
- **JWT_SECRET**: JWT signing secret (required, min 64 chars)
- **ADMIN_API_KEY**: Admin API authentication key (required)
- **ADMIN_PASSWORD**: Bootstrap admin user password (required)
- **DATABASE_URL**: Connection string (auto-configured for Docker)
- **LOG_LEVEL**: Logging verbosity (debug/info/warn/error)
- **ENVIRONMENT**: deployment environment (development/staging/production)

See `.env.example` for full list and descriptions.

### Networking

Services communicate via the `togather-network` bridge network:
- App connects to database using hostname `postgres:5432`
- Both services exposed on localhost for development access
- In production, consider removing port exposures or binding to `127.0.0.1` only

### Production Considerations

For production deployments:
1. Use environment-specific configs from `deploy/config/environments/`
2. Remove port exposures or bind to `127.0.0.1` (not `0.0.0.0`)
3. Enable SSL/TLS for database connections (`sslmode=require`)
4. Set `POSTGRES_LOG_STATEMENT=none` to reduce log volume
5. Configure resource limits in docker-compose (CPU/memory)
6. Use strong passwords (min 32 chars, high entropy)
7. Enable monitoring stack (Phase 2: Prometheus + Grafana)
8. For zero-downtime updates, use `docker-compose.blue-green.yml` (see Blue-Green Deployment section)

## Makefile Integration

The project Makefile includes Docker build targets:

```bash
make docker-build   # Build Docker image with version metadata
make docker-run     # Run container locally
```

## Version Metadata

The embedded version information matches the Makefile LDFLAGS pattern:

```go
github.com/Togather-Foundation/server/cmd/server/cmd.Version
github.com/Togather-Foundation/server/cmd/server/cmd.GitCommit
github.com/Togather-Foundation/server/cmd/server/cmd.BuildDate
```

This ensures consistency between local builds (`make build`) and Docker builds.

## Health Checks

The Dockerfile includes a HEALTHCHECK directive that calls:

```bash
/app/server healthcheck
```

**Note**: The `healthcheck` subcommand is not yet implemented. This will be added in a future task.

## Build Context Optimization

The `.dockerignore` file at the repository root excludes unnecessary files from the build context:
- Git history
- Documentation
- Build artifacts
- Test coverage reports
- Development tools

This keeps builds fast and reduces context transfer time.

## Blue-Green Deployment (Zero-Downtime Updates)

The `docker-compose.blue-green.yml` configuration enables zero-downtime deployments using the blue-green deployment strategy.

### Strategy Overview

Blue-green deployment runs **two identical instances** of the application simultaneously:
- **Blue slot**: First application instance
- **Green slot**: Second application instance
- **Nginx proxy**: Routes traffic to the active slot

Only ONE slot receives traffic at a time. During deployment:
1. Deploy new version to the **inactive slot**
2. Run health checks on new version
3. Switch traffic to new version (atomic cutover via nginx)
4. Keep old version running briefly for rollback
5. Stop old version after confirmation

### Quick Start - Blue-Green

1. **Start the blue-green stack**:
   ```bash
   cd deploy/docker
   docker compose -f docker-compose.yml -f docker-compose.blue-green.yml up -d
   ```

2. **Check service status**:
   ```bash
   docker compose -f docker-compose.blue-green.yml ps
   # Should show: postgres, togather-blue, togather-green, nginx
   ```

3. **Access the application** (via nginx proxy):
   ```bash
   curl http://localhost/health
   # Response includes X-Togather-Slot header showing active slot
   ```

4. **Access slots directly** (for testing):
   ```bash
   curl http://localhost:8081/health  # Blue slot direct access
   curl http://localhost:8082/health  # Green slot direct access
   ```

### Service Configuration

**togather-blue**:
- Container: `togather-server-blue`
- Direct port: `8081:8080` (for health checks and testing)
- Environment: `SLOT=blue`
- Inherits all settings from base `app` service

**togather-green**:
- Container: `togather-server-green`
- Direct port: `8082:8080` (for health checks and testing)
- Environment: `SLOT=green`
- Inherits all settings from base `app` service

**nginx** (reverse proxy):
- Container: `togather-proxy`
- Public port: `80:80` (all client traffic)
- Configuration: `nginx.conf` specifies active slot
- Routes traffic to active slot upstream

### Manual Traffic Switching

To switch traffic between slots manually:

1. **Edit nginx.conf** and change the upstream server:
   ```nginx
   upstream togather_backend {
       # Change this line:
       server togather-server-blue:8080;   # To route to blue
       # OR
       server togather-server-green:8080;  # To route to green
   }
   ```

2. **Test nginx configuration**:
   ```bash
   docker compose -f docker-compose.blue-green.yml exec nginx nginx -t
   ```

3. **Reload nginx** (zero-downtime reload):
   ```bash
   docker compose -f docker-compose.blue-green.yml exec nginx nginx -s reload
   ```

4. **Verify traffic switch**:
   ```bash
   curl -I http://localhost/health
   # Check X-Togather-Slot header to confirm active slot
   ```

### Deployment Workflow (Manual)

**Initial state**: Blue is active, green is inactive

1. **Deploy new version to green slot**:
   ```bash
   # Pull latest code
   git pull
   
   # Rebuild and restart green slot only
   docker compose -f docker-compose.blue-green.yml up -d --no-deps --build togather-green
   ```

2. **Health check green slot**:
   ```bash
   # Wait for green to be healthy
   docker compose -f docker-compose.blue-green.yml ps togather-green
   
   # Test green directly
   curl http://localhost:8082/health
   curl http://localhost:8082/version
   ```

3. **Switch traffic to green**:
   ```bash
   # Update nginx.conf to route to green
   sed -i 's/server togather-server-blue:8080/server togather-server-green:8080/' nginx.conf
   
   # Reload nginx
   docker compose -f docker-compose.blue-green.yml exec nginx nginx -s reload
   ```

4. **Verify traffic on green**:
   ```bash
   curl -I http://localhost/health
   # X-Togather-Slot should show green
   
   # Monitor green logs
   docker compose -f docker-compose.blue-green.yml logs -f togather-green
   ```

5. **Stop old blue slot** (after confirmation):
   ```bash
   docker compose -f docker-compose.blue-green.yml stop togather-blue
   ```

**Next deployment**: Deploy to blue (now inactive), switch traffic back to blue

### Rollback

If the new version has issues, rollback by switching traffic back:

```bash
# Update nginx.conf to point to old slot
sed -i 's/server togather-server-green:8080/server togather-server-blue:8080/' nginx.conf

# Reload nginx (instant traffic switch)
docker compose -f docker-compose.blue-green.yml exec nginx nginx -s reload

# Verify rollback
curl -I http://localhost/health
```

Both slots remain running during deployment, so rollback is instant.

### Database Considerations

**Important**: Both blue and green slots share the **same database**.

- Database migrations run before deployment (not during traffic switch)
- Migrations must be **forward-compatible** during the cutover window
- Old code (blue) and new code (green) both run against the new schema
- Use additive migrations (add columns as nullable, add indexes concurrently)
- Breaking changes require two-phase deployments

### Automated Deployment Script

The deployment workflow above is automated by the `deploy` script (T021):
- Detects active slot automatically
- Deploys to inactive slot
- Runs health checks with retries
- Switches traffic automatically
- Supports automatic rollback on failure

Usage (once T021 is complete):
```bash
cd deploy/docker
./deploy.sh production
```

### Monitoring Active Slot

To check which slot is currently active:

```bash
# Check nginx configuration
grep "server togather-server-" nginx.conf | grep -v "^#"

# Or check via HTTP header
curl -I http://localhost/health | grep X-Togather-Slot
```

### Logs and Debugging

```bash
# View all logs
docker compose -f docker-compose.blue-green.yml logs -f

# View specific slot
docker compose -f docker-compose.blue-green.yml logs -f togather-blue
docker compose -f docker-compose.blue-green.yml logs -f togather-green

# View nginx access logs (shows which slot handled requests)
docker compose -f docker-compose.blue-green.yml logs -f nginx | grep upstream=
```

### Resource Usage

Running both slots requires:
- **Memory**: ~2x application memory (both containers running)
- **CPU**: Minimal (inactive slot idle)
- **Disk**: Database shared, minimal overhead for second container

For production with resource constraints:
- Stop inactive slot after successful deployment
- Start inactive slot just before next deployment
- This reduces memory usage between deployments
