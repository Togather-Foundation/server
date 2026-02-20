# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

#### Email Service API - Context Parameter Addition (Breaking Change)

**What Changed:**
The `email.Sender.SendInvitation()` function signature has been updated to accept a `context.Context` as the first parameter:

- **Old signature:** `SendInvitation(to, inviteLink, invitedBy string) error`
- **New signature:** `SendInvitation(ctx context.Context, to, inviteLink, invitedBy string) error`

**Why:**
This change was necessary to support the Resend API's context-aware operations, enable proper timeout control, and follow Go best practices for cancelable operations. The context parameter allows callers to:
- Set deadlines and timeouts for email operations
- Cancel email sending operations if needed
- Propagate request-scoped values (e.g., request IDs for tracing)

**Migration Path:**
If you have code that calls `SendInvitation()`, add a `context.Context` parameter as the first argument:

```go
// Before
err := emailService.SendInvitation(email, link, admin)

// After
err := emailService.SendInvitation(ctx, email, link, admin)
```

For background operations without a parent context, use `context.Background()`:
```go
err := emailService.SendInvitation(context.Background(), email, link, admin)
```

**Impact:**
- **Scope:** Internal package only (`internal/email`)
- **External Impact:** Low - this is an internal package, not exposed in public APIs
- **Internal Callers:** All internal callers have been updated:
  - `internal/domain/users/service.go:321` (CreateUserAndInvite)
  - `internal/domain/users/service.go:896` (ResendInvitation)

**Related:**
- See `docs/admin/email-setup.md` for complete email configuration documentation
- See `internal/email/README_TESTS.md` for testing patterns with the updated API

---

## [0.1.0] - 2026-02-20

This is the initial release of the Togather SEL Server — a production-ready
Shared Events Library backend implementing the SEL Interoperability Profile.
957 commits across 743 files, built from scratch.

### Added

#### Core Event Pipeline
- Batch event ingestion endpoint with idempotency protection (SHA-256 payload hash)
- Transactional event creation with atomic occurrence and source record writes
- Four-layer duplicate detection: exact hash match, near-duplicate via `pg_trgm`,
  place/org fuzzy dedup with auto-merge, and admin review merge action
- Event review queue: flagged events held for admin approval before publication,
  with approve/reject/merge/delete actions and rejection reason tracking
- Timezone error detection and auto-correction for reversed start/end dates,
  including a conservative early-morning heuristic
- Robust URL normalization for social media and external data sources
- `VALIDATION_REQUIRE_IMAGE` config flag for optional image enforcement
- HTTP 202 responses for async ingestion; review queue cleanup background worker

#### Knowledge Graph & Reconciliation
- Artsdata W3C Reconciliation API client for entity matching (places, organizations)
- `ReconciliationWorker` and `EnrichmentWorker` River background jobs triggered
  on ingestion; auto-dereferences high-confidence (`auto_high`) entity matches
- Artsdata short ID expansion to full URIs before storage
- Confidence score normalization to 0.0–1.0 range
- `server reconcile` CLI command for bulk reconciliation against Artsdata
- SQLc-typed knowledge graph tables with caching via `ReconciliationCacheStore`

#### Geocoding
- Nominatim geocoding client with configurable timeout and country defaults
- Background `GeocodingWorker` River jobs enqueued on batch ingestion
- Reverse geocoding support for map UI interactions
- Proximity search via `near_place` + `radius` query parameters on the events API
- PostgreSQL geocoding result cache with periodic cleanup job
- PostGIS `geography` columns for spatial indexing

#### Federation & Interoperability
- JSON-LD serialization with full schema.org field mapping for events, places,
  and organizations
- Turtle (RDF) serialization for semantic web consumers
- Content negotiation middleware: `application/ld+json`, `text/turtle`, `text/html`
- `/.well-known/sel-profile` node discovery endpoint
- Federated change feed (`/api/v1/changefeed`) with SHACL validation and
  idempotency protection
- Federation node registry CRUD for managing trusted peers
- `sameAs` and `FederationURI` in JSON-LD responses
- Provenance tracking: `source.url`, `source.eventId`, ingestion timestamps,
  and change feed triggers
- Tombstone responses (HTTP 410 Gone) for deleted events, places, and organizations

#### Developer Portal & API Keys
- Self-service developer registration via email invitation
- GitHub OAuth 2.0 integration for developer sign-up (auto-creates account)
- Separate JWT signing keys for admin vs. developer tokens
- Developer dashboard with API key management, usage sparklines, and daily rollup
- Per-key usage tracking with daily breakdown via River background job
- `server api-key` CLI for admin-managed key creation, listing, and revocation
- bcrypt key hashing; security warning displayed on key creation

#### Admin UI
- Tabler CSS-based admin interface with dark/light theme toggle
- Events list with filters, start date column, and direct edit links
- Admin event edit page with read-only location section
- Places and organizations list pages with inline edit modals and merge UI
- Side-by-side event comparison in merge modal
- Review queue UI with tabbed approve/reject/pending views, badge counts,
  URL auto-linkification, and quality warning badges
- User management: invite, deactivate, resend invitation, password strength
  indicator, ARIA live regions, keyboard accessibility
- Developer management: invite, list, and deactivate developer accounts
- Grafana dashboard embedding with admin authentication proxy
- Reusable pagination component for all admin list pages
- Cache-busting version strings on CSS/JS assets

#### Security
- CSRF protection for all admin HTML forms
- XSS prevention with input sanitization on all user-supplied fields
- Rate limiting: aggressive limits on admin login; separate federation tier
- CORS with `CORS_ALLOWED_ORIGINS` required in production
- Comprehensive audit logging for all admin operations
- Hashed invitation tokens (bcrypt) stored in database; raw token never persisted
- Request body size limits middleware
- Username enumeration prevention in auth error paths
- `REQUIRE_HTTPS` and related production-hardening config flags

#### Monitoring & Observability
- Prometheus `/metrics` endpoint with HTTP middleware instrumentation
- Grafana dashboard with color-coded blue/green slot panels, River job metrics,
  health check metrics, and active slot indicator
- River background job metrics via `RiverMetricsHook` (with `sync.Mutex` safety)
- OpenTelemetry tracing instrumentation (Phase 2 opt-in)
- Structured logging throughout via `rs/zerolog` with correlation IDs
- `server healthcheck` CLI with blue-green slot awareness, watch mode, and
  configurable output formats
- JSON-LD context validation wired into health checks

#### Deployment & Operations
- Zero-downtime blue-green Docker Compose deployment orchestration
- Caddy reverse proxy with TLS, automatic HTTPS, and traffic switching
- `deploy.sh` with remote deployment, deployment lock management, and
  orphaned container cleanup
- `server snapshot` CLI for pg_dump-based database backups with retention policy
- `server deploy` and `server rollback` CLI commands with deployment history
- `server cleanup` CLI for pruning Docker images, snapshots, and logs
- Per-environment `.deploy.conf` files for SSH host, domain, and city metadata
- One-command server provisioning script for Linode/VPS targets
- `install.sh.template` for reproducible server installation
- robots.txt and sitemap.xml auto-generated and deployed on each release
- Interactive API documentation at `/api/docs` via Scalar UI
- `/api/v1/openapi.yaml` endpoint for machine-readable spec
- `server webfiles` CLI for generating SEO assets

#### CLI (Cobra)
- `server serve` — HTTP server with all background workers
- `server setup` — Interactive first-time configuration wizard
- `server ingest` — Ingest events from a JSON file
- `server events` — Query the local SEL server
- `server generate` — Generate synthetic test events from fixtures
- `server snapshot` — Database backup management
- `server healthcheck` — Health monitoring
- `server deploy` / `server rollback` — Deployment management
- `server cleanup` — Artifact pruning
- `server api-key` — API key management
- `server developer` — Developer account management
- `server reconcile` — Bulk knowledge graph reconciliation
- `server webfiles` — Generate robots.txt and sitemap.xml
- `server version` — Print version and build metadata

#### MCP Server
- Model Context Protocol server for AI assistant integration; exposes events,
  places, and organizations with embedded related entity objects

#### Infrastructure
- Go 1.24+, PostgreSQL 16 with PostGIS, pgvector, and pg_trgm extensions
- SQLc for type-safe query generation; `golang-migrate` for schema migrations
- River transactional job queue with retry policies and failure alerting hooks
- `oklog/ulid/v2` for stable, sortable identifiers; `golang-jwt/jwt/v5` for auth
- Multi-stage Docker build with version metadata injection
- Full CI pipeline: unit, integration, batch integration, contract, E2E (Playwright
  + pytest), federation sync, OpenAPI validation, YAML lint, vulnerability scan,
  race detector, 35% coverage threshold
- `scripts/agent-run.sh` wrapper for capturing verbose build output in agent sessions
- Performance load testing tool (`loadtest`) with configurable ramp-up patterns

### Breaking Changes

- **JWT token invalidation on upgrade:** Admin and developer JWT tokens use
  separate signing keys. Existing tokens are invalidated on first deploy and
  require re-authentication.
- **`CORS_ALLOWED_ORIGINS` required in production:** The server will refuse to
  start in production mode without this variable set.

### Security

- Migrated API key storage from SHA-256 to bcrypt (P1 hardening)
- Fixed P0: persistent admin operations (previously in-memory only)
- Fixed P0: migration lock race condition in deployment scripts
- Hashed invitation tokens before storage (P0)
- Fixed email header injection vulnerability
- Fixed username enumeration via audit log reason codes

[Unreleased]: https://github.com/Togather-Foundation/server/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/Togather-Foundation/server/releases/tag/v0.1.0
