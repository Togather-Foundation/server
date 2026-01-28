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

For local development with PostgreSQL, see `docker-compose.yml` in this directory.

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
