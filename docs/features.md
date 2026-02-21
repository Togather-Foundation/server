# Feature Reference

A comprehensive overview of what the Togather SEL server does — organized by functional area.
For architectural context see [architecture overview](contributors/architecture.md);
for API details see [openapi.yaml](api/openapi.yaml).

---

## Event Management

The core of the system. Events are the primary data type conforming to the SEL specification.

- **Ingestion** — accept events from multiple source formats: HTML (microdata), JSON-LD, ICS/iCal, RSS, and direct API submission
- **Normalization** — standardize incoming data (dates, times, timezones, images, URLs) into canonical form
- **Validation** — configurable validation rules with warnings and hard failures; enforces SEL schema requirements
- **Deduplication** — detect and merge duplicate events via configurable similarity matching (title, date, venue, URL fingerprints)
- **Duplicate review queue** — flag uncertain duplicates for human review before merging
- **Merge** — combine duplicate records with provenance preservation; tombstone superseded events
- **Listing & filtering** — query events by date range, city, region, venue, organizer, with cursor-based pagination
- **Tombstones** — soft-delete records that retain forwarding metadata for federated nodes
- **JSON-LD export** — serve events as Schema.org-compliant JSON-LD with `application/ld+json` content negotiation

---

## Places

Venues and locations associated with events.

- **CRUD** — create, read, update, and delete place records
- **Geocoding** — automatically resolve addresses to coordinates via Nominatim with a local cache
- **Event geocoding** — geocode event locations when no structured place record exists
- **Knowledge graph reconciliation** — match places to Artsdata and Wikidata identifiers
- **Listing & filtering** — query by name, city, region with cursor pagination

---

## Organizations

Event organizers and producers.

- **CRUD** — create, read, update, and delete organization records
- **Knowledge graph reconciliation** — match organizations to Artsdata and Wikidata identifiers
- **Listing & filtering** — query by name with cursor pagination

---

## Ingestion Pipeline (Background Jobs)

Long-running tasks handled by the River transactional job queue.

| Job | Description |
|---|---|
| Batch ingestion | Process bulk event imports asynchronously |
| Deduplication | Run similarity checks across newly ingested events |
| Reconciliation | Match events/places/orgs against knowledge graphs |
| Enrichment | Augment records with data pulled from external sources |
| Place geocoding | Resolve place addresses to lat/lon coordinates |
| Event geocoding | Geocode event location strings without a place record |
| Idempotency cleanup | Purge expired idempotency keys |
| Batch results cleanup | Remove stale batch job result records |
| Review queue cleanup | Archive resolved duplicate review items |
| Geocoding cache cleanup | Evict stale Nominatim cache entries |

---

## Integrated Event Scraper

A two-tier web scraper for automatically extracting events from arts and culture websites.

- **Tier 0 — JSON-LD extraction** — zero-config per site; fetches page, finds `<script type="application/ld+json">` Event blocks, normalises all schema.org variants (`@graph`, `ItemList`, `EventSeries`, arrays, single objects)
- **Tier 1 — Colly CSS selectors** — for sites without reliable JSON-LD; per-site YAML config specifies CSS selectors; handles pagination
- **Source configs** — community-contributed YAML files in `configs/sources/`; validate with `server scrape list`
- **robots.txt compliance** — Tier 0 checks manually; Tier 1 via Colly native support
- **Transparent User-Agent** — `Togather-SEL-Scraper/0.1 (+https://togather.foundation; events@togather.foundation)`
- **Run tracking** — each scrape recorded in `scraper_runs` table with status, timing, and event counts
- **SEL-native submission** — events submitted via batch ingest API, so dedup/reconciliation/provenance run automatically

See [integration/scraper.md](integration/scraper.md) for usage and configuration details.

---

## Knowledge Graph Integration

- **Artsdata reconciliation** — match and link local records to Artsdata entities via their SPARQL/API
- **Wikidata enrichment** — augment place and organization records with Wikidata properties
- **Bulk reconciliation CLI** — `server reconcile` with `--places`, `--organizations`, or `--all` flags
- **Provenance tracking** — record the source and derivation of every piece of enriched data

---

## Federation (ActivityPub)

Federation is not fully implemented or functional yet.

- **ActivityPub protocol** — publish and receive event activities following the ActivityPub spec
- **Actor endpoints** — expose node actors for federation handshakes
- **Inbox / Outbox** — receive inbound activities; publish outbound activities to followers
- **Tombstone propagation** — broadcast event deletions/merges to federated peers

---

## Authentication & Authorization

- **API keys** — long-lived keys for programmatic access; scoped to agents/integrators
- **JWT (admin)** — short-lived tokens with HKDF-SHA256 derivation for admin sessions
- **JWT (developer)** — separate token family for developer portal sessions
- **GitHub OAuth** — social login flow for admin and developer accounts
- **RBAC** — role-based access control with `admin`, `agent`, and `viewer` roles
- **Audit log** — record authentication events and privileged actions

---

## Developer Portal

Self-service access management for API consumers.

- **Registration** — invite-based developer account creation
- **API key management** — create, list, and revoke personal API keys
- **Usage tracking** — per-key request metrics with quota visibility
- **Developer API** — dedicated set of endpoints under `/api/developer/`

---

## Admin Interface

Operator tools for managing the node.

- **Event review** — inspect, approve, reject, or merge events from the review queue
- **Duplicate detection dashboard** — visualize flagged duplicate pairs and their similarity scores
- **User management** — invite, list, and deactivate admin/developer accounts
- **Developer management** — view developer registrations and revoke access
- **Admin UI** — server-rendered HTML interface (see [admin/README.md](admin/README.md))

---

## API

- **Public Events API** — list and fetch events; supports JSON and JSON-LD
- **Public Places API** — list and fetch places
- **Public Organizations API** — list and fetch organizations
- **Geocoding API** — resolve address strings to coordinates
- **Admin API** — event/user/developer management endpoints (requires admin JWT)
- **Developer API** — key management and usage endpoints (requires developer JWT)
- **Federation API** — ActivityPub actor, inbox, outbox endpoints
- **MCP server** — Model Context Protocol endpoint for LLM tool integrations
- **OpenAPI spec** — machine-readable spec at `openapi.yaml`, human readable generated by Scalar

---

## CLI (`server` binary)

| Command | Description |
|---|---|
| `server serve` | Start the HTTP server (default command) |
| `server setup` | Interactive first-time configuration wizard |
| `server ingest` | Ingest events from a JSON file |
| `server events` | Query events from a running SEL node |
| `server generate` | Generate test events from fixtures |
| `server scrape` | Scrape events from URLs or configured sources (url, list, source, all) |
| `server reconcile` | Bulk-reconcile records against knowledge graphs |
| `server snapshot` | Database backup management (create, list, cleanup) |
| `server healthcheck` | Health monitoring with blue-green slot support and watch mode |
| `server deploy` | Manage deployments and rollbacks |
| `server cleanup` | Remove stale deployment artifacts (images, snapshots, logs) |
| `server api-key` | Create, list, and revoke API keys |
| `server developer` | Invite, list, and deactivate developer accounts |
| `server webfiles` | Generate `robots.txt` and `sitemap.xml` |
| `server version` | Print version, git commit, and build date |

---

## Deployment & Operations

- **Blue-green deployments** — zero-downtime slot-based switching via `server deploy`
- **Rollback** — revert to previous deployment slot with a single command
- **Database snapshots** — `pg_dump` backups to filesystem or S3-compatible storage with retention policies
- **Database migrations** — forward-only versioned migrations via `golang-migrate`
- **Health checks** — `/health` endpoint with slot awareness; watchable via `server healthcheck --watch`
- **Prometheus metrics** — request rates, latencies, job queue depths at `/metrics`
- **Structured logging** — zerolog JSON logs with request IDs and correlation IDs
- **Caddy reverse proxy** — TLS termination, routing, and static file serving (see [deploy/caddy.md](deploy/caddy.md))
- **Docker Compose** — orchestration for local development and production
- **CI/CD pipeline** — GitHub Actions with lint, test, build, and deploy stages

---

## Semantic Web & Interoperability

- **JSON-LD processing** — parse and serialize JSON-LD with Schema.org vocabulary
- **SHACL validation** — validate event/place/org shapes against `shapes/*.ttl` definitions
- **Linked Data contexts** — publish reusable JSON-LD contexts at `contexts/`
- **Content negotiation** — serve `application/ld+json` or `application/json` based on `Accept` header
- **CC0 licensing** — default open-data license applied to all exported records per SEL spec

---

## Observability

- **Request logging** — every HTTP request logged with method, path, status, duration, and request ID
- **Prometheus metrics** — exposed at `/metrics` for scraping by Grafana/Alertmanager
- **Grafana dashboards** — pre-built dashboards for request rates, job queues, and database health
- **Health endpoint** — `/health` returns node status, version, and DB connectivity
- **Audit trail** — privileged actions (key creation, user changes) recorded with actor and timestamp

---

**Last Updated:** 2026-02-20
