# Artsdata Interop Guide

## 0) Target outcome

**Note**: This guide focuses on Artsdata integration specifically. SEL also integrates with other knowledge graphs (Wikidata, MusicBrainz, ISNI, OpenStreetMap). For the complete multi-graph integration strategy, see [Knowledge Graph Integration Strategy](./KNOWLEDGE_GRAPHS.md).

- **Read** Artsdata knowledge graph data (events/places/people/orgs/concepts) reliably.
- **Reconcile** local entities → **Artsdata IDs** + external IDs (Wikidata/ISNI/VIAF/etc.).
- **Mint** Artsdata IDs only when needed (and only after reconciliation).
- **Publish/ingest** interoperable RDF/JSON-LD datasets (via Databus) when acting as a contributor.
- Remain compatible with the broader open-data ecosystem (Schema.org / Wikidata / W3C standards).

**When to use Artsdata**:
- Primary reconciliation for **arts, culture, and music events** (especially Canadian content)
- Event organizers and venues in the arts sector
- When enriching events with curated arts-specific metadata

**When to use other graphs**:
- **Wikidata**: Universal fallback for all domains, especially non-arts events (sports, community, education)
- **MusicBrainz**: Music-specific events, artists, recordings
- **OpenStreetMap**: Venue locations and geographic data
- **ISNI**: Authoritative person/organization identifiers


## 1) Canonical URLs

### Human UI / exploration

- KG UI home: https://kg.artsdata.ca/
- “All Data Feeds” page (graphs you can use as sources): https://kg.artsdata.ca/query/show?sparql=feeds_all&title=Data+Feeds

### Public APIs (read / interop)

- **Reconciliation endpoint (W3C Reconciliation spec)**: `https://api.artsdata.ca/recon`

Docs: https://docs.artsdata.ca/architecture/reconciliation.html

- **SPARQL (core graph only)**: `https://query.artsdata.ca/sparql`
- **SPARQL (all feeds + core graphs)**: `https://db.artsdata.ca/repositories/artsdata`

Docs: https://docs.artsdata.ca/architecture/sparql.html

- **Query API (Event Search API beta + future search)**:

Docs: https://docs.artsdata.ca/architecture/query-api.html

Example (from docs): `http://api.artsdata.ca/events?format=json&frame=event_bn&source=http://kg.artsdata.ca/culture-creates/footlight/placedesarts-com`

### Contributor APIs (write / ingestion)

- **Databus / Graph-store gateway** (requires credentials/team):

Docs: https://docs.artsdata.ca/architecture/graph-store-api.html

- **Minting API** (alpha; mint Artsdata IDs):

Docs: https://docs.artsdata.ca/architecture/minting.html

### Data model / identifiers / shaping

- Data model & platform docs index: https://docs.artsdata.ca/
- Data flow architecture overview: https://docs.artsdata.ca/architecture/overview.html
- Persistent identifier recommendations: https://docs.artsdata.ca/identifier-recommendations.html
- `@id` (local URIs) guidelines: https://docs.artsdata.ca/id.html
- `sameAs` guidelines: https://docs.artsdata.ca/sameas.html
- Retrieve URIs guide: https://docs.artsdata.ca/retrieve-uri.html
- Naming conventions for URIs: https://docs.artsdata.ca/naming_conventions.html
- SHACL reports & validation notes: https://docs.artsdata.ca/shacl_reports.html
- JSON-LD structured data templates (events): https://docs.artsdata.ca/gabarits-jsonld/README.html


## 2) Core data contracts

### 2.1 Vocabulary alignment

- Artsdata is **Schema.org-first** (subset + mappings) and imports multiple ontologies.
- Artsdata’s ontology/contracts are defined as **SHACL shapes**; incoming data must validate or it won’t load.
- Treat SHACL as the “source of truth” for required properties per class (Event/Place/Organization/Person…).
- 
See “Ontologies & Inferencing” section in docs index: https://docs.artsdata.ca/

### 2.2 Identifiers: local vs external vs Artsdata

- **Local URI (`@id`)**: your system’s stable identifier within your domain. Use when you control stability.

Rules + patterns: https://docs.artsdata.ca/id.html

- **External URI (`sameAs`)**: a globally recognized identifier (Wikidata/ISNI/VIAF/MusicBrainz/etc.).

Rules + examples: https://docs.artsdata.ca/sameas.html

- **Artsdata ID**: globally-unique Artsdata URI:

    - URI format: `http://kg.artsdata.ca/resource/K{digits}-{digits}`
    - Regex: `^K\d+-\d+$`

Source: https://docs.artsdata.ca/architecture/minting.html and https://docs.artsdata.ca/identifier-recommendations.html

### 2.3 URI naming conventions (when you mint your own)

- Follow Artsdata naming conventions when publishing linked data URIs:

https://docs.artsdata.ca/naming_conventions.html


## 3) Read-paths

### 3.1 Reconciliation (first step for interop)

Use when you have: `name`, maybe `url`, maybe `address`, and need IDs.

- Endpoint: `https://api.artsdata.ca/recon`
- Supported types: Event, Place, Person, Organization, Agent, Concept

Docs + UI test bench: https://docs.artsdata.ca/architecture/reconciliation.html

**Protocol (primary spec)**

- W3C Entity Reconciliation CG: https://reconciliation-api.github.io/specs/latest/

(Artsdata says it follows this spec.)

**Request format (important)**

Per W3C Reconciliation API v0.2 section 4.3, queries MUST be sent as `application/x-www-form-urlencoded` POST with the JSON batch in a `queries` form parameter. Sending a JSON body with `Content-Type: application/json` will fail.

```bash
# Correct: form-encoded
curl -X POST "https://api.artsdata.ca/recon" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  --data-urlencode 'queries={"q0":{"query":"Massey Hall","type":"schema:Place"}}'

# Wrong: JSON body (will fail)
curl -X POST "https://api.artsdata.ca/recon" \
  -H "Content-Type: application/json" \
  -d '{"queries":{"q0":{"query":"Massey Hall","type":"schema:Place"}}}'
```

**Properties NOT supported (as of Feb 2026)**

Despite W3C spec support for property-based disambiguation and Artsdata docs mentioning property paths like `schema:address/schema:postalCode`, the Artsdata `/recon` endpoint returns **HTTP 500** when any `properties` array is included in the query. This affects all property types (`schema:address/schema:addressLocality`, `schema:url`, etc.) and all entity types.

Current workaround: send only `query` + `type` fields. Properties are omitted from queries but still used locally for cache key differentiation.

**Score semantics (important)**

Artsdata scores are **unbounded** — they are NOT on a 0-100 or 0-1 scale:
- **Exact matches**: score ~1000+ with `match: true` (e.g., "Bad Dog Theatre Company" → score 1247.4)
- **Partial matches**: score ~3-12 with `match: false` (e.g., "Royal Theatre" → score 4.0-6.5)

The `match` boolean is the primary signal for exact matches. Our implementation normalizes scores to 0.0-1.0: `match=true` → 0.99, others → `score/15.0` (capped at 0.95).

**Type values**

Use CURIE-prefixed types (e.g., `schema:Place`, `schema:Organization`). Full URIs (e.g., `http://schema.org/Place`) also work.

**Extra disambiguation via properties**

- Artsdata docs mention additional properties (property paths like `schema:address/schema:postalCode`) to filter candidates. However, **this does not work in practice** — see note above.

See docs: https://docs.artsdata.ca/architecture/reconciliation.html

### 3.2 SPARQL (power tool; use with constraints)

Use when you need joins, graph scoping, custom ranking, provenance, controlled vocabs, etc.

- Core graph SPARQL: `https://query.artsdata.ca/sparql`
- All graphs SPARQL: `https://db.artsdata.ca/repositories/artsdata`
- Docs: https://docs.artsdata.ca/architecture/sparql.html

**Primary specs**

- SPARQL 1.1 Query: [https://www.w3.org/TR/sparql11-query/](https://www.w3.org/TR/sparql11-query/)

### 3.3 Query API (simpler event retrieval)

Use when you want “upcoming events list” style output without writing SPARQL.

- Docs: https://docs.artsdata.ca/architecture/query-api.html
- Event Search API beta example (docs):
    - `GET http://api.artsdata.ca/events?format=json&frame=event_bn&source=<graph-or-catalog-uri>`

**Source selection (graph scoping)**

- Use graphs from: https://kg.artsdata.ca/query/show?sparql=feeds_all&title=Data+Feeds

Choose a single feed graph for deterministic results; only broaden scope when necessary.

### 3.4 Dereference/crawl Artsdata URIs (entity payload retrieval)

- Artsdata docs explicitly recommend "crawl persistent identifiers to access associated metadata in JSON-LD".

Start point: https://docs.artsdata.ca/architecture/overview.html

#### 3.4.1 Content negotiation

- `GET http://kg.artsdata.ca/resource/K…` returns **HTML by default**.
- Request RDF via either:
  - `Accept:` header (e.g. `application/ld+json`, `text/turtle`), or
  - explicit format parameters (where supported by the endpoint/UI).
- Use this to fetch entity JSON-LD deterministically from a known Artsdata URI.

### 3.5 iCalendar feeds (consumer convenience)

- Mentioned in architecture overview; entry points are exposed in KG UI navigation.

Start from: https://docs.artsdata.ca/architecture/overview.html and https://kg.artsdata.ca/

### 3.6 Data dumps (bulk/offline)

- Mentioned in architecture overview. Prefer dumps for analytics/offline indexing.

Start from: https://docs.artsdata.ca/architecture/overview.html


## 4) Write-paths (contributing data / ensuring ingestion)

### 4.1 Preferred: publish Schema.org event markup on your site

- Emit JSON-LD (or RDFa/microdata) with:
    - Stable `@id` (local URI) when possible
    - `sameAs` links for any known external IDs
    - Conform to Artsdata templates where applicable

Templates: https://docs.artsdata.ca/gabarits-jsonld/README.html

#### 4.1.1 Artsdata Crawler behavior

- Artsdata runs a crawler that scans pages for Schema.org JSON-LD (search-engine style).
- Ensure `robots.txt` allows Artsdata’s crawler to fetch event pages.
- Some ETL may extract from unstructured text (NLP assist), but **JSON-LD markup remains the reliable contract**.

### 4.2 Databus ingestion (datasets → Artsdata)

Use when you maintain an RDF dataset (JSON-LD, N-Quads, Turtle, etc.) and need it loaded.

- Docs: https://docs.artsdata.ca/architecture/graph-store-api.html
- Key mechanics:
    - Databus stores **metadata about datasets**, not the dataset itself (dataset must remain downloadable).
    - **Only RDF formats** are loaded into Artsdata KG.
    - Import may apply generic transformations; SHACL reports can be generated if SHACL provided.
    - Dataset version replaces prior artifact-version in Artsdata.
    - **Do not use `kg.artsdata.ca` domain as triple subject** when uploading.
    - Ontologies are not allowed to be uploaded (contact stewards instead).

**Auth**

- Databus upload requires credentials (GitHub/WebID/X-API-KEY).

See "Credentials" section: https://docs.artsdata.ca/architecture/graph-store-api.html

### 4.3 Other contribution onramps (non-Databus)

- **Direct feeds/APIs**: if a provider has an open JSON/JSON-LD feed or API, Artsdata can ingest via ETL.
- **Google Sheets → linked data**: a Sheets-based workflow exists for orgs without dev capacity.
- **Footlight (Culture Creates)**: Footlight Console/CMS structures event data and feeds Artsdata.

### 4.4 SHACL validation workflow (before upload)

- SHACL reports overview: https://docs.artsdata.ca/shacl_reports.html
- Playground (external service): [https://shacl-playground.zazuko.com/](https://shacl-playground.zazuko.com/)

**Primary spec**

- SHACL: [https://www.w3.org/TR/shacl/](https://www.w3.org/TR/shacl/)

### 4.5 Minting Artsdata IDs (only after reconciliation)

- Docs: https://docs.artsdata.ca/architecture/minting.html
- Rule: run reconciliation first; mint only if no matching Artsdata URI exists.


## 5) Wikidata + cross-graph interlinking

- Artsdata created **Wikidata property P7627** for linking Wikidata entities to Artsdata.

Mentioned here: https://docs.artsdata.ca/architecture/sparql.html

Wikidata property page: https://www.wikidata.org/wiki/Property:P7627

**Agent rule**

- If your entity has a Wikidata QID:

1. represent it as `http://www.wikidata.org/entity/Q...` in `sameAs` (not the short "Q…" form)

https://docs.artsdata.ca/sameas.html

2. reconcile against Artsdata; if Artsdata ID exists, add it as `sameAs` as well.


## 6) Implementation rules

### 6.1 Deterministic entity resolution

- Always attempt in this order:

1. If you already have an Artsdata URI → use it.
2. Else reconcile (`/recon`) using name + type (property-based disambiguation is not currently supported by Artsdata — see §3.1).
3. Only if reconcile yields no match → mint (if policy allows) or store as unresolved.

### 6.2 Graph scoping and provenance

- Prefer a specific source graph (from Data Feeds list) for reproducible results.

https://kg.artsdata.ca/query/show?sparql=feeds_all&title=Data+Feeds

- When using SPARQL across all graphs, expect duplicates/conflicts; incorporate provenance fields if you need trust decisions.

Provenance overview in docs index: https://docs.artsdata.ca/

**Provenance is structural**

- Artsdata tracks source provenance using **named graphs + load metadata** (who/when/how loaded).
- When you need “trust” decisions, incorporate provenance (graph) into queries and outputs.

**Contributor gates (don’t surprise providers later)**

- Minimum viable Event must include (at least): `name`, `startDate`, `location`.
- Incoming data must pass SHACL validation or it won’t import.
- Contributors typically need an Artsdata account and must accept CC0 release of contributed data.

### 6.3 Data normalization conventions (Artsdata-specific notes)

- Artsdata docs mention transforms around Schema.org IRIs and certain property datatypes; treat Schema.org terms carefully and validate output against Shapes when contributing.

See "Exceptions handling schema.org in Artsdata" in docs index: https://docs.artsdata.ca/

### 6.3.1 Reasoning/inference (query semantics)

- Artsdata applies basic **RDFS/OWL reasoning** (OWL-Horst-like ruleset). Expect some inferred facts/types in SPARQL results.
- If you need strict “asserted only” behavior, scope queries accordingly (graph/provenance patterns) and test.

### 6.4 Licensing / OSS constraints

- Artsdata data is **CC0** (no restrictions).
- Triplestore product referenced: **GraphDB Free (Ontotext)** (free-to-use; **not OSS**).

Architecture overview: https://docs.artsdata.ca/architecture/overview.html


## 7) MCP server + “skills” examples

Create tools that map cleanly to Artsdata capabilities.

### 7.1 Example Tools (names are suggestions; keep them stable)

1. `reconcile_entity`
    - Input: `{ type, name?, url?, properties? }`
    - Calls: `POST https://api.artsdata.ca/recon` (W3C reconciliation)
    - Output: top candidates with `{ id(uri), score, name, match? }`

2. `mint_artsdata_id`
    - Input: `{ type, label, minimal_graph }`
    - Precondition: must have called `reconcile_entity` with no confident match
    - Calls: Minting API (alpha) per docs: https://docs.artsdata.ca/architecture/minting.html

3. `sparql_query`
    - Input: `{ endpoint: core|all, query }`
    - Endpoints: `https://query.artsdata.ca/sparql` or `https://db.artsdata.ca/repositories/artsdata`

4. `search_events`
    - Input: `{ source_graph_uri, frame?, format?, filters... }`
    - Calls: Query API event endpoint per docs: https://docs.artsdata.ca/architecture/query-api.html

5. `list_data_feeds`
    - Output feed graph URIs + metadata
    - Source: https://kg.artsdata.ca/query/show?sparql=feeds_all&title=Data+Feeds

6. `get_controlled_vocab`
    - Vocab entry points are exposed in KG UI navigation; start at `https://kg.artsdata.ca/`

### 7.2 Hard safety rules (non-optional)

- Never mint if reconciliation returns a plausible match.
- Never upload triples with subject URIs under `kg.artsdata.ca` via Databus.
- Never upload ontologies via Databus.
- When generating JSON-LD for contribution, validate against SHACL (when possible).


## 8) SEL Implementation Reference

The Artsdata reconciliation adapter is implemented in the following locations:

### Code

| Component | Location | Description |
|-----------|----------|-------------|
| HTTP Client | `internal/kg/artsdata/client.go` | W3C Reconciliation API client with rate limiting, retries, entity dereference |
| Reconciliation Service | `internal/kg/reconciliation.go` | Cache→API→threshold→store pipeline |
| Workers | `internal/jobs/workers.go` | `ReconciliationWorker` (River job) and `EnrichmentWorker` (placeholder) |
| CLI Command | `cmd/server/cmd/reconcile.go` | `server reconcile places\|organizations\|all` with `--dry-run`, `--limit`, `--force` |
| Configuration | `internal/config/config.go` | `ArtsdataConfig` struct (env vars: `ARTSDATA_ENABLED`, `ARTSDATA_ENDPOINT`, etc.) |
| DB Migration | `internal/storage/postgres/migrations/000030_*` | `knowledge_graph_authorities`, `entity_identifiers`, `reconciliation_cache` tables |
| SQLc Queries | `internal/storage/postgres/queries/knowledge_graph.sql` | 13 queries for CRUD on KG tables |

### Pipeline Flow

1. Event ingested via API → handler enqueues `ReconciliationArgs` job for place and/or organization
2. `ReconciliationWorker` picks up job → queries entity from DB → calls `ReconciliationService.ReconcileEntity()`
3. Service checks `reconciliation_cache` → if miss, builds query (name + type only, no properties) and sends form-encoded POST to Artsdata W3C Reconciliation API
4. Raw Artsdata scores (unbounded, e.g. 1247 for exact match) are normalized to 0.0-1.0 via `normalizeArtsdataScore()` — `match=true` → 0.99, others → `score/15.0` capped at 0.95
5. Results classified by confidence: >=0.95 with `match=true` → `auto_high`, >=0.80 → `auto_low`, <0.80 → rejected
6. Matches stored in `entity_identifiers`, results cached in `reconciliation_cache` (both positive and negative)
7. High-confidence matches enqueue `EnrichmentArgs` job (enrichment is a TODO placeholder)

### Bulk Reconciliation

```bash
server reconcile places --limit 100          # Reconcile up to 100 unreconciled places
server reconcile organizations --dry-run     # Preview what would be reconciled
server reconcile all --force                 # Force re-reconcile everything (bypass cache)
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `ARTSDATA_ENABLED` | `false` | Enable Artsdata reconciliation |
| `ARTSDATA_ENDPOINT` | `https://api.artsdata.ca/recon` | W3C Reconciliation API endpoint |
| `ARTSDATA_TIMEOUT_SECONDS` | `10` | HTTP client timeout (seconds) |
| `ARTSDATA_RATE_LIMIT_PER_SEC` | `1.0` | Max API calls per second |
| `ARTSDATA_CACHE_TTL_DAYS` | `30` | Cache TTL for successful reconciliation results |
| `ARTSDATA_FAILURE_TTL_DAYS` | `7` | Cache TTL for negative/failed reconciliation attempts |

Confidence thresholds are hardcoded: >=0.95 with `match=true` → `auto_high`, >=0.80 → `auto_low`, <0.80 → rejected.


## 9) Minimal test suite

### 9.1 Reconciliation correctness

- Place by name + type (`schema:Place`) should return stable Artsdata URI (properties like `postalCode` cannot be used for disambiguation — see §3.1).
- Organization by name + type (`schema:Organization`) should return stable Artsdata URI.
- The `match` boolean flag is the primary signal for exact matches; raw scores are unbounded and must be normalized before storage.

### 9.2 SPARQL smoke tests

- Core endpoint returns 200 and a small result set for a known-safe query.
- All-graphs endpoint returns superset (or equal) vs core, for same query pattern.

### 9.3 Identifier validation

- Any Artsdata ID accepted must match `^http://kg\.artsdata\.ca/resource/K\d+-\d+$`
- Any Wikidata URI must match `^http://www\.wikidata\.org/entity/Q\d+$`


## 10) Known Limitations (as of Feb 2026)

1. **Properties not supported**: The Artsdata `/recon` endpoint returns HTTP 500 when any `properties` array is included. Disambiguation relies solely on `query` (name) + `type` fields.
2. **Scores are unbounded**: Artsdata scores are not 0-100 or 0-1. Exact matches score ~1000+, partial matches ~3-12. Must normalize before storage.
3. **Form-encoded requests only**: W3C Reconciliation API requires `application/x-www-form-urlencoded` POST with queries in a `queries` form parameter. JSON body requests fail silently or error.
4. **No enrichment yet**: The `EnrichmentWorker` is a placeholder. It logs the job and completes without fetching additional data.
5. **Rate limiting**: Artsdata API has no documented rate limits, but we default to 1 req/sec and use a single-worker queue to be conservative.


## 11) Primary standards

- JSON-LD 1.1: [https://www.w3.org/TR/json-ld11/](https://www.w3.org/TR/json-ld11/)
- RDF 1.1 Concepts: [https://www.w3.org/TR/rdf11-concepts/](https://www.w3.org/TR/rdf11-concepts/)
- SPARQL 1.1 Query: [https://www.w3.org/TR/sparql11-query/](https://www.w3.org/TR/sparql11-query/)
- SHACL: [https://www.w3.org/TR/shacl/](https://www.w3.org/TR/shacl/)
- Schema.org: [https://schema.org/](https://schema.org/)
- W3C Entity Reconciliation CG (spec used by Artsdata): https://reconciliation-api.github.io/specs/latest/

## 12) “If you only remember 7 rules”

1. Prefer `sameAs` with global URIs (Wikidata/ISNI/VIAF/etc.) for people/orgs/places; use Artsdata IDs where available.
2. Use reconciliation (`/recon`) before minting.
3. Mint only when necessary; validate ID format (`K\d+-\d+`).
4. Scope queries to a source graph when you want deterministic lists.
5. Use SPARQL only when Query API can’t express the retrieval.
6. Keep local `@id` stable and never equal to the page `url`.
7. When contributing via Databus: no `kg.artsdata.ca` subjects; no ontologies; validate with SHACL.

### Web-verified against these Artsdata primary docs (for audit)

Artsdata KG quick links + entry points: https://kg.artsdata.ca/

Architecture overview (consumer access methods, CC0, triplestore reference): https://docs.artsdata.ca/architecture/overview.html

Reconciliation endpoint + W3C reconciliation mention: https://docs.artsdata.ca/architecture/reconciliation.html

SPARQL endpoints + Wikidata P7627 mention: https://docs.artsdata.ca/architecture/sparql.html

Minting ID format + regex: https://docs.artsdata.ca/architecture/minting.html

Identifier guidance (`@id` vs `sameAs`) + related pages: https://docs.artsdata.ca/identifier-recommendations.html

Databus constraints + auth modes: https://docs.artsdata.ca/architecture/graph-store-api.html

