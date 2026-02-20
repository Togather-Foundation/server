# Multi-Graph Knowledge Graph Integration Strategy

**Version:** 0.1.0-DRAFT  
**Date:** 2026-01-23  
**Status:** Living Document  
**Related Documents:**
- [SEL Schema Design](../contributors/database.md)
- [SEL Core Profile](./core-profile-v0.1.md)
- [Artsdata Integration Guide](./artsdata.md)

---

## Executive Summary

This document defines the strategy for integrating SEL with **multiple knowledge graphs** beyond Artsdata, enabling support for **non-arts events** (sports, community, education, etc.) while maintaining high-quality reconciliation for arts and cultural events.

**Key Principles:**
- **Domain-aware routing**: Reconciliation targets appropriate knowledge graphs based on event type
- **Multi-graph reconciliation**: Events can link to multiple authorities (Artsdata + Wikidata + others)
- **Graceful degradation**: Falls back to broader graphs when domain-specific graphs lack coverage
- **Extensibility**: Easy to add new knowledge graph authorities via configuration

---

## Table of Contents

1. [Supported Knowledge Graphs](#1-supported-knowledge-graphs)
2. [Reconciliation Routing Rules](#2-reconciliation-routing-rules)
3. [Multi-Graph Reconciliation Workflow](#3-multi-graph-reconciliation-workflow)
4. [Conflict Resolution](#4-conflict-resolution)
5. [Adding New Knowledge Graphs](#5-adding-new-knowledge-graphs)
6. [Implementation Guidelines](#6-implementation-guidelines)

---

## 1. Supported Knowledge Graphs

### 1.1 Primary Authorities

The SEL integrates with the following knowledge graphs, registered in the `knowledge_graph_authorities` table:

| Authority | Code | Domain Coverage | Priority | Use Cases |
|-----------|------|-----------------|----------|-----------|
| **Artsdata** | `artsdata` | Arts, Culture, Music | High (10) | Arts events, cultural organizations, venues |
| **Wikidata** | `wikidata` | Universal | Medium (20) | All entity types, fallback for non-arts |
| **MusicBrainz** | `musicbrainz` | Music | High (15) | Music events, artists, recordings |
| **ISNI** | `isni` | Persons, Organizations | Medium (30) | Authoritative identifiers for people/orgs |
| **OpenStreetMap** | `osm` | Places, Venues | Medium (40) | Venue locations, geographic data |

### 1.2 Authority Registry Schema

See [Schema Design § 5.1](../contributors/database.md#51-knowledge-graph-authorities-registry) for the complete `knowledge_graph_authorities` table definition.

**Key Fields:**
- `authority_code`: Unique identifier (e.g., `artsdata`, `wikidata`)
- `applicable_domains`: Array of event domains this authority covers
- `priority_order`: Lower numbers = higher priority (within applicable domains)
- `trust_level`: 1-10 scale for confidence weighting
- `reconciliation_endpoint`: W3C Reconciliation API URL (if supported)

---

## 2. Reconciliation Routing Rules

### 2.1 Domain-Based Routing

Events are categorized by `event_domain` field (see [Schema § 1.1](../contributors/database.md#11-events-table)):

| Event Domain | Primary Graphs | Fallback Graphs | Notes |
|--------------|----------------|-----------------|-------|
| **arts** | Artsdata → Wikidata | - | Visual arts, theatre, dance |
| **music** | MusicBrainz → Artsdata → Wikidata | - | Concerts, performances |
| **culture** | Artsdata → Wikidata | - | Festivals, heritage events |
| **sports** | Wikidata | - | Athletic events, tournaments |
| **community** | Wikidata | - | Fairs, gatherings, meetups |
| **education** | Wikidata | - | Workshops, lectures, conferences |
| **general** | Wikidata | - | Uncategorized events |

### 2.2 Entity-Type Specific Routing

Different entity types may use different reconciliation strategies:

#### Events
- Use domain-based routing (see § 2.1)
- Always attempt primary domain-specific graph first
- Fall back to Wikidata if domain graph yields no results

#### Places/Venues
- **All domains**: Try multiple graphs in parallel:
  1. Artsdata (if arts/culture/music event)
  2. OpenStreetMap (for geographic data)
  3. Wikidata (universal fallback)
- Merge results from multiple sources (use highest confidence)

#### Organizations
- Try domain-specific graph first (e.g., Artsdata for arts orgs)
- ISNI for persons and organizations (if available)
- Wikidata as universal fallback

#### Persons
- MusicBrainz (if music-related)
- ISNI (authoritative for people)
- Wikidata (universal fallback)

### 2.3 Routing Decision Tree

```
1. Extract event_domain from event
2. Query knowledge_graph_authorities WHERE applicable_domains @> [event_domain]
3. Order by priority_order ASC
4. For each authority in priority order:
   a. Check cache (entity_type, authority_code, lookup_key)
   b. If cache miss, call reconciliation endpoint
   c. If confidence >= 95%, accept and stop
   d. If confidence 80-94%, add to candidates, continue
   e. If confidence < 80%, skip
5. If no high-confidence match found:
   a. Query universal fallback (Wikidata) if not already tried
6. Return best match or candidates for manual review
```

---

## 3. Multi-Graph Reconciliation Workflow

### 3.1 Sequential vs. Parallel Reconciliation

**Sequential (Default)**:
- Query graphs one at a time in priority order
- Stop on first high-confidence match (≥95%)
- **Use for**: Events, Organizations, Persons

**Parallel (Optional)**:
- Query multiple graphs simultaneously
- Merge results and select best match
- **Use for**: Places (where multiple graphs may provide complementary data)

### 3.2 Reconciliation Pipeline

```go
// Pseudocode
func ReconcileEntity(entityType, entityDomain, name, properties) {
  // 1. Determine applicable authorities
  authorities := GetAuthoritiesForDomain(entityType, entityDomain)
  
  // 2. Check cache for each authority
  for _, auth := range authorities {
    cached := CheckReconciliationCache(entityType, auth.Code, name, properties)
    if cached.Found && cached.Confidence >= 0.95 {
      return cached.SelectedID
    }
  }
  
  // 3. Call reconciliation APIs in priority order
  for _, auth := range authorities {
    if !auth.IsActive || auth.ReconciliationEndpoint == "" {
      continue
    }
    
    result := CallReconciliationAPI(auth, name, properties)
    CacheReconciliationResult(entityType, auth.Code, result)
    
    if result.Confidence >= 0.95 {
      // High confidence - accept immediately
      return result.SelectedID
    } else if result.Confidence >= 0.80 {
      // Medium confidence - add to candidates
      candidates = append(candidates, result)
    }
  }
  
  // 4. Fallback to Wikidata if not already tried
  if !contains(authorities, "wikidata") {
    wikidataResult := ReconcileWithWikidata(name, properties)
    if wikidataResult.Confidence >= 0.80 {
      return wikidataResult.SelectedID
    }
  }
  
  // 5. No confident match - return candidates for manual review
  return candidates
}
```

### 3.3 Result Storage

All reconciliation results are stored in the `entity_identifiers` table:

```sql
-- Example: Event linked to both Artsdata and Wikidata
INSERT INTO entity_identifiers (entity_type, entity_id, authority_code, identifier_uri, confidence, reconciliation_method)
VALUES 
  ('event', 'uuid-123', 'artsdata', 'http://kg.artsdata.ca/resource/K11-456', 0.98, 'auto_high'),
  ('event', 'uuid-123', 'wikidata', 'http://www.wikidata.org/entity/Q12345', 0.92, 'auto_low');
```

This allows events to be linked to **multiple knowledge graphs simultaneously**.

---

## 4. Conflict Resolution

### 4.1 Multiple Identifiers for Same Entity

When multiple authorities provide identifiers for the same entity, use the following rules:

**Strategy 1: Trust-Weighted Selection (Default)**
- Calculate weighted confidence: `confidence * trust_level / 10`
- Select identifier with highest weighted confidence
- Mark as `is_canonical = true` in `entity_identifiers` table

**Strategy 2: Keep All (Recommended)**
- Store identifiers from all authorities
- Use `priority_order` to determine canonical for `sameAs` output
- Allows consumers to choose which authority to trust

### 4.2 Conflicting Entity Data

When different graphs provide conflicting information (e.g., different venue names):

1. Use field-level provenance tracking (see [Schema § 3](../contributors/database.md#3-provenance-and-source-tracking))
2. Apply trust-weighted conflict resolution
3. Store all versions with provenance metadata
4. Expose conflicts via admin UI for manual review

### 4.3 Example Conflict

```json
{
  "venue_name": "Massey Hall",
  "provenance": [
    {
      "value": "Massey Hall",
      "source": "artsdata",
      "trust_level": 9,
      "confidence": 0.98,
      "is_canonical": true
    },
    {
      "value": "Roy Thomson Massey Hall",
      "source": "wikidata",
      "trust_level": 8,
      "confidence": 0.85,
      "is_canonical": false
    }
  ]
}
```

---

## 5. Adding New Knowledge Graphs

### 5.1 Evaluation Criteria

Before adding a new knowledge graph authority, evaluate:

1. **Coverage**: Does it provide unique data not available elsewhere?
2. **Quality**: What is the data quality and maintenance level?
3. **Licensing**: Is the data CC0 or compatible with SEL license?
4. **API Availability**: Does it expose a reconciliation or SPARQL endpoint?
5. **Stability**: Is the service reliable and well-maintained?
6. **Community**: Is there active development and support?

### 5.2 Integration Checklist

- [ ] Register authority in `knowledge_graph_authorities` table
- [ ] Implement URI pattern validation regex
- [ ] Add reconciliation adapter (if custom API format)
- [ ] Configure rate limits and authentication
- [ ] Add to domain routing rules (if domain-specific)
- [ ] Update documentation (this file + Interoperability Profile)
- [ ] Add integration tests
- [ ] Create example queries for validation

### 5.3 Example: Adding EventBrite

```sql
INSERT INTO knowledge_graph_authorities (
  authority_code, authority_name, base_uri_pattern,
  reconciliation_endpoint, applicable_domains,
  trust_level, priority_order, documentation_url
) VALUES (
  'eventbrite',
  'Eventbrite Events',
  '^https://www\\.eventbrite\\.ca/e/[^/]+-\\d+$',
  'https://www.eventbrite.com/platform/api/events', -- Hypothetical
  ARRAY['community', 'education', 'general'],
  6,  -- Medium trust
  50, -- Lower priority
  'https://www.eventbrite.com/platform/api'
);
```

Then implement adapter in Go:

```go
// adapters/eventbrite.go
type EventbriteReconciler struct {
  apiKey string
  client *http.Client
}

func (r *EventbriteReconciler) Reconcile(entityType, name string, props map[string]any) (*ReconciliationResult, error) {
  // Implement Eventbrite-specific reconciliation logic
}
```

---

## 6. Implementation Guidelines

### 6.1 Reconciliation Service Architecture

```
┌─────────────────────────────────────────────────────┐
│         Reconciliation Service (Go)                 │
├─────────────────────────────────────────────────────┤
│                                                     │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────┐   │
│  │   Router     │→ │  Cache       │→ │ Strategy │   │
│  │ (Domain-     │  │  (Redis/     │  │ (Priority│   │
│  │  aware)      │  │   Postgres)  │  │  Order)  │   │
│  └──────────────┘  └──────────────┘  └──────────┘   │
│         ↓                                           │
│  ┌──────────────────────────────────────────────┐   │
│  │         Authority Adapters                   │   │
│  ├──────────────────────────────────────────────┤   │
│  │ Artsdata │ Wikidata │ MusicBrainz │ ISNI     │   │
│  └──────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────┘
```

### 6.2 Configuration Management

Authority configuration should be **database-driven** (not hardcoded):

```go
// Load authorities from database
authorities, err := db.GetActiveAuthorities()
for _, auth := range authorities {
  reconciler.RegisterAuthority(auth.Code, CreateAdapter(auth))
}
```

This allows:
- Adding new graphs without code changes
- A/B testing different routing strategies
- Disabling problematic authorities dynamically
- Adjusting trust levels and priorities based on performance

### 6.3 Monitoring and Metrics

Track reconciliation performance per authority:

```sql
CREATE TABLE reconciliation_metrics (
  authority_code TEXT,
  entity_type TEXT,
  date DATE,
  
  -- Performance
  total_queries INTEGER,
  cache_hits INTEGER,
  api_calls INTEGER,
  avg_response_time_ms NUMERIC,
  
  -- Quality
  matches_found INTEGER,
  high_confidence_matches INTEGER,  -- >= 0.95
  medium_confidence_matches INTEGER, -- 0.80-0.94
  low_confidence_matches INTEGER,   -- < 0.80
  
  -- Errors
  errors INTEGER,
  error_rate NUMERIC,
  
  PRIMARY KEY (authority_code, entity_type, date)
);
```

Use metrics to:
- Identify underperforming authorities
- Optimize cache TTLs
- Adjust trust levels based on accuracy
- Detect API outages

### 6.4 Rate Limiting

Respect each authority's rate limits:

```go
type RateLimiter struct {
  perMinute int
  perDay    int
  tokens    chan struct{}
}

func (r *RateLimiter) Wait(ctx context.Context) error {
  select {
  case <-r.tokens:
    return nil
  case <-ctx.Done():
    return ctx.Err()
  }
}
```

Configure limits in `knowledge_graph_authorities.rate_limit_*` fields.

---

## 7. Testing Strategy

### 7.1 Unit Tests

Test each authority adapter independently:

```go
func TestArtsdataReconciliation(t *testing.T) {
  adapter := NewArtsdataAdapter(testAPIKey)
  
  result, err := adapter.Reconcile("place", "Massey Hall", map[string]any{
    "addressLocality": "Toronto",
    "addressRegion": "ON",
  })
  
  require.NoError(t, err)
  assert.True(t, result.Confidence >= 0.95)
  assert.Contains(t, result.URI, "kg.artsdata.ca")
}
```

### 7.2 Integration Tests

Test domain routing and fallback logic:

```go
func TestMusicEventRoutingWithFallback(t *testing.T) {
  // Create music event
  event := &Event{
    Name: "Jazz Festival",
    Domain: "music",
  }
  
  // Mock MusicBrainz failure
  mockMusicBrainz.SetResponse(nil, errors.New("API down"))
  
  // Should fall back to Artsdata, then Wikidata
  result := reconciler.ReconcileEvent(event)
  
  // Verify fallback chain was executed
  assert.True(t, mockArtsdata.WasCalled())
  assert.NotNil(t, result)
}
```

### 7.3 Golden Dataset Tests

Maintain a curated dataset of known entities with correct identifiers:

```yaml
# test/golden_dataset.yaml
- entity:
    type: event
    name: "Toronto International Film Festival"
    domain: "culture"
  expected_identifiers:
    artsdata: "http://kg.artsdata.ca/resource/K11-123"
    wikidata: "http://www.wikidata.org/entity/Q1416300"
```

Run regression tests to ensure reconciliation accuracy doesn't degrade.

---

## 8. Migration Path

### 8.1 Phase 1: Schema Updates (Current)

- ✅ Add `knowledge_graph_authorities` table
- ✅ Update `entity_identifiers` to reference authorities
- ✅ Add `event_domain` field to events table
- ✅ Update reconciliation cache schema

### 8.2 Phase 2: Adapter Implementation

- ✅ Implement Artsdata reconciliation adapter (`internal/kg/artsdata/client.go`)
- ✅ Create reconciliation service with cache→API→threshold→store pipeline (`internal/kg/reconciliation.go`)
- ✅ Add caching layer (PostgreSQL `reconciliation_cache` table with TTL)
- ✅ Implement rate limiting (token bucket in HTTP client + single-worker River queue)
- [ ] Implement domain routing logic (currently Artsdata-only, routing to multiple graphs pending)
- [ ] Add adapters for Wikidata, MusicBrainz, ISNI, OpenStreetMap

### 8.3 Phase 3: Testing & Validation

- ✅ Unit tests for client (18 cases) and service (13 cases)
- ✅ Integration tests with testcontainers (4 test functions)
- [ ] Run golden dataset tests
- [ ] Validate against SHACL shapes
- [ ] Performance testing with realistic loads
- [ ] A/B test routing strategies

### 8.4 Phase 4: Production Rollout

- [ ] Deploy with conservative routing (Artsdata-first for arts events)
- [ ] Monitor reconciliation metrics
- [ ] Gradually enable multi-graph reconciliation
- [ ] Collect feedback and iterate

---

## 9. Future Enhancements

### 9.1 Machine Learning-Assisted Routing

Train ML model to predict best authority based on event characteristics:
- Event type, description, keywords
- Organizer type, venue type
- Historical reconciliation success rates

### 9.2 Collaborative Entity Resolution

Allow users to vote on correct identifiers:
```sql
CREATE TABLE entity_identifier_votes (
  identifier_id UUID REFERENCES entity_identifiers(id),
  user_id UUID REFERENCES users(id),
  vote INTEGER CHECK (vote IN (-1, 0, 1)),  -- Downvote, neutral, upvote
  comment TEXT,
  created_at TIMESTAMPTZ DEFAULT now()
);
```

### 9.3 Cross-Graph Entity Linking

Automatically discover `owl:sameAs` relationships between authorities:
- Query Wikidata for entities with Artsdata IDs (P7627)
- Use SPARQL federation to find cross-references
- Populate `entity_identifiers` with discovered links

---

## Appendix A: Authority Comparison Matrix

| Feature | Artsdata | Wikidata | MusicBrainz | ISNI | OSM |
|---------|----------|----------|-------------|------|-----|
| **Coverage** | Arts/Culture (CA focus) | Universal | Music | Persons/Orgs | Places |
| **Data Quality** | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ |
| **API Quality** | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐ | ⭐⭐⭐ |
| **Rate Limits** | Generous | Strict | Moderate | Strict | Moderate |
| **Recon API** | ✅ W3C standard | ⚠️ Custom | ❌ No | ❌ No | ❌ No |
| **SPARQL** | ✅ Yes | ✅ Yes | ❌ No | ❌ No | ❌ No |
| **License** | CC0 | CC0 | CC-BY-NC-SA | Varies | ODbL |
| **Best For** | Canadian arts events | General fallback | Music artists/events | Name authority | Venue locations |

---

## Appendix B: Reconciliation API Examples

### B.1 Artsdata (W3C Standard)

```bash
curl -X POST https://api.artsdata.ca/recon \
  -H "Content-Type: application/json" \
  -d '{
    "queries": {
      "q0": {
        "query": "Massey Hall",
        "type": "schema:Place",
        "properties": [
          {"p": "schema:address/schema:addressLocality", "v": "Toronto"}
        ]
      }
    }
  }'
```

### B.2 Wikidata (Custom Format)

```bash
curl "https://www.wikidata.org/w/api.php?action=wbsearchentities&search=Massey+Hall&language=en&format=json&type=item&props=description"
```

---

**Document Status**: Draft for review  
**Next Review**: 2026-02-23  
**Maintainers**: SEL Architecture Working Group
