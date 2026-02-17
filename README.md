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

---

## Getting Started: Using the API

The SEL API provides open access to event data and supports event submission from authenticated sources.

### Reading Events (No Authentication Required)

**Query public events:**
```bash
curl https://toronto.togather.foundation/api/v1/events
```

**Filter by date range:**
```bash
curl "https://toronto.togather.foundation/api/v1/events?start_date=2026-03-01&end_date=2026-03-31"
```

**Interactive API explorer:**
- Web UI: [https://toronto.togather.foundation/api/docs](https://toronto.togather.foundation/api/docs)
- OpenAPI specs: [JSON](https://toronto.togather.foundation/api/v1/openapi.json) | [YAML](https://toronto.togather.foundation/api/v1/openapi.yaml)

### Submitting Events (Requires API Key)

To submit or manage events, you need an API key. **Onboarding process:**

1. **Request an invitation**: Email [info@togather.foundation](mailto:info@togather.foundation) with your name, email, and use case
2. **Accept your invitation**: Check your email for the invitation link (valid for 7 days), set your password
3. **Create an API key**: Log in to the developer portal at `/dev/login`, navigate to `/dev/api-keys`, and create a key
4. **Use your key**: Include it in the `Authorization: Bearer <key>` header for write requests

**Alternative:** GitHub OAuth is available for invitation-based onboarding.

**Submit an event:**
```bash
curl -X POST https://toronto.togather.foundation/api/v1/events \
     -H "Authorization: Bearer your_api_key_here" \
     -H "Content-Type: application/ld+json" \
     -d @event.json
```

### Comprehensive Guides

- **[Developer Quick Start](docs/integration/DEVELOPER_QUICKSTART.md)** - 4-step onboarding guide
- **[Authentication Guide](docs/integration/AUTHENTICATION.md)** - API keys, JWT tokens, and security
- **[API Guide](docs/integration/API_GUIDE.md)** - Endpoint reference and examples

---

## Quick Start: Deploying SEL

**New server:** provision and install the base system (one-time).

```bash
./deploy/scripts/provision.sh deploy@server production --with-app
```

**Existing server:** deploy with zero downtime.

```bash
# Uses .deploy.conf.production when present
./deploy/scripts/deploy.sh production
```

**Deploy specific version:** `./deploy/scripts/deploy.sh production --version v1.2.3`

**Docs:** [DEPLOY.md](DEPLOY.md) · [docs/deploy/quickstart.md](docs/deploy/quickstart.md) · [docs/deploy/remote-deployment.md](docs/deploy/remote-deployment.md)

## Local Development

```bash
git clone https://github.com/Togather-Foundation/server.git
cd server
make build
./server setup
```

**Docs:** [docs/contributors/DEVELOPMENT.md](docs/contributors/DEVELOPMENT.md) · [docs/contributors/POSTGRESQL_SETUP.md](docs/contributors/POSTGRESQL_SETUP.md)


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
