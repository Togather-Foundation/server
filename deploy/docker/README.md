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
8. For zero-downtime updates, use `docker-compose.blue-green.yml` (T009)

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
