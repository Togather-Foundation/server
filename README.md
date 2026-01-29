# Shared Events Library (SEL)

**Part of the [Togather Ecosystem](https://togather.foundation)**

The **Shared Events Library (SEL)** is the infrastructure component of [**Togather**](https://togather.foundation)—a collaborative initiative to rebuild event discovery as shared civic infrastructure.

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

Event discovery is broken because data is fragmented across walled platforms (social media, ticket sites) and individual websites. Organizers exhaust themselves cross-posting, while users are trapped in algorithm-driven feeds.

**The real issue:** There is no lack of event data, but open access and expert, personalized curation are missing.

## The Solution

We are creating an events commons using shared standards (Schema.org, ActivityPub) that everyone can build on.

### The Architecture
The ecosystem works in three layers:

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

For operators deploying a Togather node to production:

### Prerequisites
- Linux server with Docker
- PostgreSQL 16+ with PostGIS
- 2GB+ RAM, 20GB+ disk

### Deploy to Production
```bash
git clone https://github.com/Togather-Foundation/server.git
cd server

# Configure environment
cp deploy/config/environments/.env.production.example deploy/config/environments/.env.production
nano deploy/config/environments/.env.production  # Edit with your credentials

# Deploy
./deploy/scripts/deploy.sh production
```

### Rollback if Needed
```bash
./deploy/scripts/rollback.sh production
```

### Deployment Documentation
- [Deployment Guide](deploy/docs/quickstart.md) - Complete setup instructions
- [CI/CD Integration](deploy/docs/ci-cd.md) - GitHub Actions, GitLab CI, Jenkins
- [Rollback Guide](deploy/docs/rollback.md) - Troubleshooting and recovery
- [Migration Guide](deploy/docs/migrations.md) - Database schema management

**Deployment Features:**
- ✅ Zero-downtime blue-green deployment
- ✅ Automatic database migrations with snapshots
- ✅ One-command rollback with health validation
- ✅ Multi-environment support (dev/staging/prod)
- ✅ Comprehensive testing and validation

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
- ✅ Generate secure secrets
- ✅ Configure database connection
- ✅ Create .env file
- ✅ Set up database and run migrations
- ✅ Create your first API key

After setup completes, your SEL server will be ready at `http://localhost:8080`!

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
make run           # Or: make dev (with hot reload)
```

### Common Commands
```bash
make docker-down      # Stop Docker containers
make docker-logs      # View logs
make test             # Run tests
make lint             # Run linter
make ci               # Full CI pipeline locally
```

### Development Documentation
- [Development Guide](docs/contributors/DEVELOPMENT.md) - Full development workflow
- [Architecture Guide](docs/contributors/ARCHITECTURE.md) - System design
- [Testing Guide](docs/contributors/TESTING.md) - TDD workflow

## Quick API Test

Once your local environment is running, verify event ingestion works:

### Using the CLI (Recommended)

The SEL server includes built-in CLI commands for easy testing:

```bash
# 1. Create an API key
server api-key create my-test-key

# 2. Save the key
export API_KEY="<key-from-output>"

# 3. Ingest a test event
server ingest test-event.json

# 4. Watch processing in real-time
server ingest test-event.json --watch

# 5. List your API keys
server api-key list

# 6. Revoke a key
server api-key revoke <id>
```

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

If you prefer direct HTTP API testing:

```bash
# 1. Create an API key
go run scripts/create-api-key.go my-test-key

# 2. Export the key
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
