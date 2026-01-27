# Interoperability Guide

Welcome to the SEL interoperability documentation!

## Who This Is For

You're building a **SEL-compatible node** or system that participates in the federated network:
- Running your own SEL server instance
- Building a compatible implementation
- Federating with other SEL nodes
- Ensuring semantic interoperability

## Quick Start

### New to SEL?

1. **[Core Profile v0.1](CORE_PROFILE_v0.1.md)** - Start here for URI scheme, JSON-LD structure, validation
2. **[API Contract v1](API_CONTRACT_v1.md)** - HTTP endpoints, pagination, error formats
3. **[Glossary](../glossary.md)** - Understand SEL terminology

### Building Federation?

1. **[Federation Protocol v1](FEDERATION_v1.md)** - Change feeds, sync protocol, cursor semantics
2. **[Core Profile v0.1](CORE_PROFILE_v0.1.md)** - URI preservation rules, provenance model

### Integrating Knowledge Graphs?

1. **[Knowledge Graphs Guide](KNOWLEDGE_GRAPHS.md)** - Multi-graph reconciliation strategy
2. **[Artsdata Integration](ARTSDATA.md)** - Artsdata-specific details

## Conformance Levels

### MUST (Required for Interoperability)
- URI scheme follows `https://{node-domain}/{entity-type}/{ulid}`
- JSON-LD output validates against SHACL shapes
- Content negotiation supports `application/ld+json`
- CC0 license for all exported data
- RFC 7807 error responses

### SHOULD (Strongly Recommended)
- Support multiple knowledge graph authorities
- Expose change feed at `/api/v1/feeds/changes`
- Implement federation sync endpoint
- Provide OpenAPI 3.1 documentation

### MAY (Optional Extensions)
- Vector search
- Semantic query API
- Webhook delivery
- Geographic filtering

## Core Specifications

### 1. Core Profile v0.1
**Status:** Proposed for Community Review

Defines the foundational interoperability requirements:
- URI scheme and identifier rules
- JSON-LD structure and context
- SHACL validation shapes
- Provenance model
- License policy

**Read:** [CORE_PROFILE_v0.1.md](CORE_PROFILE_v0.1.md)

### 2. API Contract v1
**Status:** Proposed for Community Review

Defines the HTTP API contract:
- Endpoint specifications
- Request/response formats
- Pagination patterns
- Error handling (RFC 7807)
- Content negotiation

**Read:** [API_CONTRACT_v1.md](API_CONTRACT_v1.md)

### 3. Federation Protocol v1
**Status:** Implemented

Defines the federation sync protocol:
- Change feed semantics
- Cursor-based pagination
- URI preservation rules
- Trust-based conflict resolution
- Authentication

**Read:** [FEDERATION_v1.md](FEDERATION_v1.md)

## SHACL Validation Shapes

SEL provides SHACL shapes for validating exported data:

- **[Event Shape](schemas/event-v0.1.ttl)** - Required fields, constraints
- **[Place Shape](schemas/place-v0.1.ttl)** - Venue validation
- **[Organization Shape](schemas/organization-v0.1.ttl)** - Organizer validation

## Knowledge Graph Integration

SEL supports multiple knowledge graph authorities for entity reconciliation:

| Authority | Domain Coverage | Guide |
|-----------|----------------|-------|
| **Artsdata** | Arts, Culture, Music | [ARTSDATA.md](ARTSDATA.md) |
| **Wikidata** | Universal | [KNOWLEDGE_GRAPHS.md](KNOWLEDGE_GRAPHS.md) |
| **MusicBrainz** | Music | [KNOWLEDGE_GRAPHS.md](KNOWLEDGE_GRAPHS.md) |
| **ISNI** | Persons, Orgs | [KNOWLEDGE_GRAPHS.md](KNOWLEDGE_GRAPHS.md) |
| **OpenStreetMap** | Places | [KNOWLEDGE_GRAPHS.md](KNOWLEDGE_GRAPHS.md) |

## Example: Federated Event

```json
{
  "@context": [
    "https://schema.org",
    "https://togather.foundation/contexts/sel/v0.1.jsonld"
  ],
  "@id": "https://montreal.togather.foundation/events/01HYX3...",
  "@type": "Event",
  "name": "Jazz Night",
  "startDate": "2025-07-15T19:00:00-04:00",
  "location": {
    "@id": "https://montreal.togather.foundation/places/01HYX4...",
    "@type": "Place",
    "name": "Centennial Park"
  },
  "sameAs": [
    "http://kg.artsdata.ca/resource/K11-234"
  ],
  "sel:originNode": "https://montreal.togather.foundation",
  "license": "https://creativecommons.org/publicdomain/zero/1.0/"
}
```

## Testing Your Implementation

### 1. URI Minting
```bash
# Test that URIs use your node domain
curl -H "Accept: application/ld+json" \
  https://your-node.example.org/events/01ABC...

# Verify @id uses your domain
```

### 2. SHACL Validation
```bash
# Validate your JSON-LD against shapes
uvx pyshacl -s schemas/event-v0.1.ttl your-event.ttl
```

### 3. Federation Sync
```bash
# Test change feed
curl https://your-node.example.org/api/v1/feeds/changes

# Test sync endpoint
curl -X POST https://your-node.example.org/api/v1/federation/sync \
  -H "Authorization: Bearer api-key" \
  -H "Content-Type: application/ld+json" \
  -d @federated-event.json
```

## Implementation Checklist

- [ ] URI scheme follows `https://{node}/{type}/{ulid}`
- [ ] JSON-LD validates against SHACL shapes
- [ ] Content negotiation: `text/html`, `application/ld+json`, `text/turtle`
- [ ] RFC 7807 error responses
- [ ] Change feed at `/api/v1/feeds/changes`
- [ ] Federation sync at `/api/v1/federation/sync`
- [ ] CC0 license in all exports
- [ ] URI preservation for federated events
- [ ] OpenAPI 3.1 documentation at `/api/v1/openapi.json`

## Getting Help

- **GitHub Issues**: [togather/server/issues](https://github.com/Togather-Foundation/server/issues)
- **Discussions**: [togather/discussions](https://github.com/Togather-Foundation/discussions)
- **Email**: [email protected]

## Reference Implementation

This SEL server is the reference implementation. See:
- **[Source Code](https://github.com/Togather-Foundation/server)**
- **[Contributors Guide](../contributors/README.md)** - for implementation details

---

**Back to:** [Documentation Index](../README.md)
