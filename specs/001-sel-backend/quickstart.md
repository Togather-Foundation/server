# Quickstart: SEL Backend Server

**Date**: 2026-01-23  
**Branch**: `001-sel-backend`

## Overview

This guide helps you get the SEL backend server running locally for development.

## Prerequisites

- **Go 1.22+**: [Download](https://go.dev/dl/)
- **Docker & Docker Compose**: For PostgreSQL with extensions
- **Make**: For running build commands
- **golangci-lint** (optional): `make install-tools`

## Quick Start

### 1. Clone and Setup

```bash
git clone https://github.com/togather/server.git
cd server
git checkout 001-sel-backend
```

### 2. Start Database

```bash
# Start PostgreSQL with PostGIS, pgvector, pg_trgm
docker-compose up -d postgres
```

Alternatively, use the provided Docker Compose file:

```yaml
# docker-compose.yml
version: '3.8'
services:
  postgres:
    image: pgvector/pgvector:pg16
    environment:
      POSTGRES_USER: sel
      POSTGRES_PASSWORD: sel_dev
      POSTGRES_DB: sel
    ports:
      - "5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data
    command: >
      postgres
        -c shared_preload_libraries='pg_stat_statements'

volumes:
  pgdata:
```

### 3. Configure Environment

```bash
cp .env.example .env
```

Edit `.env`:
```bash
# Database
DATABASE_URL=postgres://sel:sel_dev@localhost:5432/sel?sslmode=disable

# Server
PORT=8080
NODE_DOMAIN=localhost:8080

# Auth (generate with: openssl rand -base64 32)
JWT_SECRET=your-secret-here-at-least-32-bytes

# Development
LOG_LEVEL=debug
```

### 4. Run Migrations

```bash
make migrate-up
```

### 5. Start the Server

```bash
# Development mode (with live reload if air is available)
make dev

# Or standard build and run
make run
```

Server starts at `http://localhost:8080`

### 6. Verify

```bash
# Health check
curl http://localhost:8080/healthz

# OpenAPI spec
curl http://localhost:8080/api/v1/openapi.json
```

---

## Development Workflow

### Running Tests

```bash
# All tests
make test

# With race detector
make test-race

# Verbose output
make test-v

# Coverage report
make coverage
```

### Code Quality

```bash
# Format code
make fmt

# Run linter
make lint
```

### Database Management

```bash
# Create new migration
make migrate-create name=add_events_table

# Run migrations
make migrate-up

# Rollback last migration
make migrate-down

# Regenerate SQLc code
make sqlc-generate
```

---

## API Examples

### Create an Event (requires API key)

```bash
curl -X POST http://localhost:8080/api/v1/events \
  -H "Content-Type: application/ld+json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{
    "name": "Jazz in the Park",
    "startDate": "2026-07-15T19:00:00-04:00",
    "location": {
      "name": "Centennial Park",
      "addressLocality": "Toronto",
      "addressRegion": "ON"
    }
  }'
```

### Query Events (public)

```bash
# All events
curl http://localhost:8080/api/v1/events

# With filters
curl "http://localhost:8080/api/v1/events?city=Toronto&startDate=2026-07-01"

# Request JSON-LD
curl -H "Accept: application/ld+json" http://localhost:8080/api/v1/events

# Pagination
curl "http://localhost:8080/api/v1/events?limit=10&after=seq_1000"
```

### Get Single Event

```bash
# JSON-LD
curl -H "Accept: application/ld+json" \
  http://localhost:8080/api/v1/events/01HYX3KQW7ERTV9XNBM2P8QJZF

# HTML
curl -H "Accept: text/html" \
  http://localhost:8080/api/v1/events/01HYX3KQW7ERTV9XNBM2P8QJZF
```

### Admin Login

```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email": "admin@example.com", "password": "admin123"}'
```

### Change Feed (for federation)

```bash
curl "http://localhost:8080/api/v1/feeds/changes?since=seq_0&limit=50"
```

---

## Project Structure

```
server/
├── cmd/server/main.go       # Entry point
├── internal/
│   ├── api/                 # HTTP handlers and routing
│   ├── domain/              # Business logic by feature
│   │   ├── events/
│   │   ├── places/
│   │   └── organizations/
│   ├── jsonld/              # JSON-LD serialization
│   ├── storage/postgres/    # SQLc queries and migrations
│   ├── auth/                # JWT and API key handling
│   └── config/              # Configuration
├── web/admin/               # Embedded admin UI templates
├── contexts/sel/            # JSON-LD context files
├── shapes/                  # SHACL validation shapes
├── specs/001-sel-backend/   # This feature's specs
└── docs/                    # Architecture documentation
```

---

## Common Issues

### Database connection failed

```
Error: dial tcp: connect: connection refused
```

**Solution**: Ensure PostgreSQL is running:
```bash
docker-compose ps
docker-compose up -d postgres
```

### Missing extensions

```
Error: extension "pgvector" is not available
```

**Solution**: Use the pgvector Docker image (included in docker-compose.yml) or install extensions manually:
```sql
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";
CREATE EXTENSION IF NOT EXISTS "vector";
CREATE EXTENSION IF NOT EXISTS "postgis";
```

### JWT secret too short

```
Error: JWT secret must be at least 32 bytes
```

**Solution**: Generate a proper secret:
```bash
openssl rand -base64 32
```

### Rate limit exceeded

```
HTTP 429: Too Many Requests
```

**Solution**: For development, increase limits in config or use authenticated requests (agent: 300/min, admin: unlimited).

---

## Environment Variables Reference

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | — | PostgreSQL connection string |
| `PORT` | No | 8080 | Server port |
| `HOST` | No | 0.0.0.0 | Server bind address |
| `NODE_DOMAIN` | Yes | — | FQDN for URI generation |
| `JWT_SECRET` | Yes | — | JWT signing secret (32+ bytes) |
| `JWT_EXPIRY` | No | 15m | JWT token expiration |
| `LOG_LEVEL` | No | info | Logging level (debug/info/warn/error) |
| `RATE_LIMIT_PUBLIC` | No | 60 | Public rate limit (req/min) |
| `RATE_LIMIT_AGENT` | No | 300 | Agent rate limit (req/min) |

---

## Next Steps

1. **Create an admin user**: Use the seed script or direct database insert
2. **Generate an API key**: Via admin UI or API
3. **Submit test events**: Use the API examples above
4. **Explore the admin UI**: http://localhost:8080/admin
5. **Read the API spec**: http://localhost:8080/api/v1/openapi.json

For detailed architecture information, see:
- [Architecture Design](../../docs/togather_SEL_server_architecture_design_v1.md)
- [Schema Design](../../docs/togather_schema_design.md)
- [Interoperability Profile](../../docs/togather_SEL_Interoperability_Profile_v0.1.md)
