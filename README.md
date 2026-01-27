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

## Security

This server implements defense-in-depth security measures including:

- **SQL Injection Protection**: Pattern escaping for all user inputs in database queries
- **Rate Limiting**: Role-based request throttling (60/min public, 300/min agent)
- **HTTP Hardening**: Timeout configuration to prevent DoS attacks
- **JWT Secret Validation**: Enforced minimum 32-character secrets at startup
- **API Key Hashing**: SHA-256 hashing (bcrypt migration planned)

**Required Configuration:**
- `JWT_SECRET` must be at least 32 characters (generate with `openssl rand -base64 48`)
- Database connections should use `sslmode=require` in production
- Server must run behind HTTPS termination (nginx, Traefik, cloud LB)

For complete security documentation, see:
- `CODE_REVIEW.md` - Security audit and findings
- `docs/togather_SEL_server_architecture_design_v1.md` § 7.1 - Security hardening details
- `SETUP.md` - Security configuration guide

## Development Velocity 🚀

**Recent Accomplishments (January 2026):**

In just **2 days** of collaborative development between human oversight and AI agents, we achieved:

- ✅ **135 commits** implementing core SEL functionality
- ✅ **35,415 total lines** of production Go code, **18,824 lines of tests** added
- ✅ **92 integration tests** passing at 100% (federation, change feeds, provenance, tombstones)
- ✅ **14 database migrations** for PostgreSQL + PostGIS schema
- $28.36 in metered Github Copilot charges
- ChatGPT 5.2 Codex initially and when large context needed and then mostly Claude Sonnet 4.5

**Key Features Implemented:**
- Federation sync with URI preservation and nested entity extraction
- Change feed system with JSON-LD transformation and cursor pagination
- Event tombstone tracking for deletions
- Provenance tracking with field-level attribution
- Security hardening (CORS, rate limiting, request size limits)
- Full CRUD operations for events, places, and organizations

**Tech Stack:**
- Go 1.23+ with Huma (HTTP/OpenAPI 3.1)
- PostgreSQL 16+ with PostGIS, pgvector, full-text search
- SQLc for type-safe SQL queries
- River for transactional job queues
- JSON-LD for semantic web interoperability

This rapid development demonstrates the power of AI-assisted development combined with specification-driven design, comprehensive testing, and issue tracking (using our custom `bd` beads system).

For a concrete example of how human direction and coding agents collaborated in this repo, see `docs/agent_workflow_summary.md`.

---
*This README provides a high-level overview. Technical documentation can be found in the `docs/` directory.*
