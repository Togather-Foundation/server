# Shared Events Library (SEL)

The **Shared Events Library (SEL)** is the events storage and query component of [**Togather**](https://togather.foundation)—a collaborative, community driven initiative to rebuild event discovery as shared civic infrastructure.

## Project Vision

We are building a data commons for open events ecosystems. The goal is to coordinate three groups to adopt shared practices:

1.  **Event Publishers** (venues, organizers)
2.  **Infrastructure Builders** (developers, civic technologists)
3.  **Application Creators** (curators, AI devs)

This repository focuses on the **Infrastructure** layer: building shared, open tools instead of proprietary silos.

## The Mission

We exist to rebuild event discovery as a public good where:
*   **People** discover events that matter to them without surveillance.
*   **Organizers** reach audiences directly without platform lock-in.
*   **Communities** coordinate themselves using shared infrastructure.
*   **Developers** build on open APIs and data standards.

## The Problem

Events are scattered across platforms.
Organizers post everywhere, reach no one.
Communities miss what matters most.

**The real issue:** There is no lack of event data, but open access and expert, personalized curation are missing.

## The Solution

We are creating an events commons using shared standards (Schema.org, ActivityPub) that everyone can build on.

### What the SEL Does

The **Shared Events Library (SEL)** is a backend server that:
- **Ingests** event data from any source using Schema.org Event markup
- **Validates** events against JSON-LD schemas and SHACL shapes
- **Enriches** events with linked open data from knowledge graphs (Artsdata, Wikidata, MusicBrainz)
- **Stores** events in a queryable database with provenance tracking (preserving original source data)
- **Serves** events via REST and GraphQL APIs with content negotiation (JSON-LD, JSON)
- **Federates** using ActivityPub to sync with other SEL nodes and services

Each SEL instance typically serves a specific geographic area (city/region), acting as a public utility for event data in that community.

### The Architecture

The larger ecosystem works in three layers:

1.  **Data Publishing (Structured Metadata):** Events are published on source websites using AI-assisted Schema.org markup.
2.  **Shared Infrastructure (This Project):** A distributed collection system and "Shared Event Library" that aggregates, validates, and serves this data as a public utility. SEL integrates with multiple knowledge graphs (Artsdata, Wikidata, MusicBrainz, etc.) to enrich events with linked open data, supporting both arts and non-arts events.
3.  **Discovery Applications:** Personal AI curators and apps that read from the commons to verify and recommend events to users locally.

## Why This Matters

Events are the heartbeat of local culture. By building this shared infrastructure, we enable:
*   **Cultural Vibrancy:** Small, DIY, and community events become discoverable.
*   **Privacy-First Discovery:** Personal AI agents can find events for you without a central platform tracking your behavior.
*   **Resilience:** Cities can coordinate culture without depending on corporate platforms.

## Who We Are

The [**Togather Foundation**](https://togather.foundation) is a coordination point, not a platform owner. We provide reference implementations (like this library), documentation, and standards guidance. We do not own the data or monetize the community.

## Quick Start: Deploying SEL

Two deployment workflows:

### Development/Staging: Simple Deployment

```bash
make deploy-package
scp dist/togather-server-*.tar.gz deploy@server:~/
ssh deploy@server 'cd ~ && tar -xzf togather-server-*.tar.gz && cd togather-server-* && sudo ./install.sh'
```

**Use when:** dev/staging, low-traffic updates, brief downtime OK.

See [DEPLOY.md](DEPLOY.md) and `docs/deploy/quickstart.md` for full steps.

### Production: Zero-Downtime Deployment

```bash
# Uses .deploy.conf.production when present
./deploy/scripts/deploy.sh production
```

**Features:** blue/green, Caddy traffic switch, health-gated, auto rollback.

**Deploy specific version:** `./deploy/scripts/deploy.sh production --version v1.2.3`

### Deployment Documentation
- [Deployment Guide](DEPLOY.md) - Upgrade and deployment methods
- [Quickstart Guide](docs/deploy/quickstart.md) - Complete setup instructions
- [Remote Deployment](docs/deploy/remote-deployment.md) - Zero-downtime deployment guide
- [Rollback Guide](docs/deploy/rollback.md) - Troubleshooting and recovery
- [Deployment Testing](docs/deploy/deployment-testing.md) - Post-deploy checklist
- [CI/CD Integration](docs/deploy/ci-cd.md) - GitHub Actions, GitLab CI, Jenkins

---

## Quick Start: Local Development

For developers working on the SEL codebase:

### Recommended: Interactive Setup
The fastest way to get started is with the interactive setup command:

```bash
# Clone and build
git clone https://github.com/Togather-Foundation/server.git
cd server
make build

# Interactive guided setup (recommended)
./server setup

# Or non-interactive Docker setup
./server setup --docker --non-interactive
# Or: make setup-docker
```

The setup command will:
- ✅ Detect your environment (Docker or local PostgreSQL)
- ✅ Check prerequisites
- ✅ **Generate secure secrets** (JWT_SECRET, CSRF_KEY, admin password using crypto/rand)
- ✅ Configure database connection
- ✅ **Create .env file** in project root with generated secrets and configuration
- ✅ Set up database and run migrations
- ✅ **Create your first API key** and save it to .env as API_KEY

After setup completes, your SEL server will be ready at `http://localhost:8080`!

**Global CLI flags** (available for all commands):
- `--config <path>` - Custom config file path (optional, defaults to .env in project root)
- `--log-level <level>` - Set log level: debug, info, warn, error (default: info)
- `--log-format <format>` - Set log format: json, console (default: json)

Example: `./server serve --log-level debug --log-format console`

**Note:** For local PostgreSQL, you'll need PostgreSQL 16+ with PostGIS, pgvector, and pg_trgm extensions installed. See [PostgreSQL Setup Guide](docs/contributors/POSTGRESQL_SETUP.md) for installation instructions.

### Option 1: Docker (Manual Setup)
```bash
# Install development tools
make install-tools

# Start everything (database + server on Docker)
make docker-up

# Server: http://localhost:8080
# Database: localhost:5433
# Migrations run automatically ✅
```

### Option 2: Local PostgreSQL (Manual Setup)

**Prerequisites:** PostgreSQL 16+ with PostGIS, pgvector, and pg_trgm extensions. See [PostgreSQL Setup Guide](docs/contributors/POSTGRESQL_SETUP.md) for installation instructions.

```bash
# Install tools
make install-tools

# Set up local database
make db-check      # Verify PostgreSQL has required extensions
make db-setup      # Create database with postgis, pgvector
make db-init       # Generate .env with auto-generated secrets

# Run migrations
make migrate-up
make migrate-river

# Start server
make run           # Build and run server (auto-kills existing processes)
make dev           # Run with live reload (auto-restarts on code changes)
                   # Note: First run 'make install-tools' to get air for live reload

# Manage running server
make stop          # Stop any running server processes
make restart       # Restart the server (stop + run)

# Quick reference for manual control:
pkill togather-server           # Kill server if make stop doesn't work
ps aux | grep togather-server   # Check if server is running
```

### Common Commands
```bash
make docker-down      # Stop Docker containers
make docker-logs      # View logs
make test             # Run tests
make lint             # Run linter
make ci               # Full CI pipeline locally
```

### Build Output

Both `make build` and `go build ./cmd/server` produce the same output:
- **Binary location**: `./server` (in the project root)
- **Version info**: `make build` includes git version metadata, `go build` shows "dev"

```bash
# Either method works:
make build              # Builds to ./server with version info
go build ./cmd/server   # Builds to ./server (dev version)

# Run the binary:
./server --help
./server version
```

### Development Documentation
- [Development Guide](docs/contributors/DEVELOPMENT.md) - Full development workflow
- [Architecture Guide](docs/contributors/ARCHITECTURE.md) - System design
- [Testing Guide](docs/contributors/TESTING.md) - TDD workflow

## Quick API Test

Once your local environment is running, verify event ingestion works:

### Exploring the API

The SEL server provides interactive API documentation at `http://localhost:8080/api/docs` using [Scalar](https://scalar.com).

**Features:**
- 🌐 **Interactive Testing**: Try all API endpoints directly from your browser
- 📖 **Complete Documentation**: Automatically generated from OpenAPI 3.1 spec
- 🔑 **API Key Instructions**: Email info@togather.foundation to request an API key
- 📥 **OpenAPI Spec**: Available at `/api/v1/openapi.json`

**Quick links:**
- API Documentation: http://localhost:8080/api/docs
- Health Check: http://localhost:8080/health (includes `docs_url` field)
- SEL Profile: http://localhost:8080/.well-known/sel-profile (includes `api_documentation` field)

### Using the CLI (Recommended)

The SEL server includes built-in CLI commands for easy testing:

```bash
# 1. Create an API key (adds to .env automatically)
./server api-key create my-test-key

# 2. Ingest a test event (reads API_KEY from .env automatically)
./server ingest test-event.json

# 3. Watch processing in real-time
./server ingest test-event.json --watch

# 4. List your API keys
./server api-key list

# 5. Revoke a key
./server api-key revoke <id>
```

**Note:** After running `make build` or `go build ./cmd/server`, the binary is located at `./server`. 

**Configuration Loading:**
- The CLI automatically loads configuration from `.env` file in the project root (development/test environments)
- All commands (serve, ingest, api-key, etc.) read from `.env` by default
- In staging/production, set `ENV_FILE` environment variable to specify a different config file location
- Priority: Environment variables > .env file > defaults

**Example test-event.json:**
```json
{
  "events": [{
    "@type": "Event",
    "name": "Test Concert",
    "startDate": "2026-02-15T20:00:00Z",
    "location": {
      "@type": "Place",
      "name": "Community Hall",
      "addressLocality": "Toronto",
      "addressRegion": "ON",
      "addressCountry": "CA"
    }
  }]
}
```

### Using curl (Alternative)

If you prefer direct HTTP API testing without using the CLI:

```bash
# 1. Create an API key
go run scripts/create-api-key.go my-test-key
# (Or use: ./server api-key create my-test-key)

# 2. Export the key (if not using CLI which reads from .env)
export API_KEY="<key-from-output>"

# 3. Ingest event
curl -X POST http://localhost:8080/api/v1/events:batch \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $API_KEY" \
  -d @test-event.json

# 4. List events
curl http://localhost:8080/api/v1/events | jq .

# 5. Check health
curl http://localhost:8080/health | jq .
```

## Documentation & Contributor Resources

- [SEL Documentation](docs/README.md) - Landing page for contributors, integrators, and node builders
- [Architecture Guide](docs/contributors/ARCHITECTURE.md) - System design and core patterns
- [Development Guide](docs/contributors/DEVELOPMENT.md) - Standards, tools, and workflows
- [Database Guide](docs/contributors/DATABASE.md) - Schema and migrations
- [Testing Guide](docs/contributors/TESTING.md) - TDD workflow and test commands
- [Security Guide](docs/contributors/SECURITY.md) - Security model and practices
- [Integration Guides](docs/integration/README.md) - API usage and scraper guidance
- [Development Velocity](docs/contributors/meta/agent_workflows.md) - Collaboration highlights and delivery pace

## Thanks

This project is built with help from [OpenCode](https://opencode.ai/), [Spec Kit](https://github.com/github/spec-kit), and [Beads](https://github.com/steveyegge/beads), along with the teams behind:

- ChatGPT 5.2 Codex
- Claude Sonnet 4.5
- Claude Opus 4.5
- Gemini Pro 3

---
*This README provides a high-level overview. [Technical documentation](docs/README.md) can be found in the `docs/` directory.*
