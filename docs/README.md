# SEL Documentation Index

**Version:** 0.1.0-DRAFT  
**Last Updated:** 2026-01-23

This directory contains the comprehensive technical documentation for the Shared Events Library (SEL) backend.

---

## Core Architecture Documents

### 📋 [SEL Interoperability Profile v0.1](./togather_SEL_Interoperability_Profile_v0.1.md)
**Status:** Proposed for Community Review  
The authoritative specification for SEL federation, API contracts, and linked data output. This is the "source of truth" for implementation.

**Key Topics:**
- URI scheme and identifier rules
- Canonical JSON-LD output formats
- Minimal required fields & SHACL validation
- Multi-graph `sameAs` linking
- Federation protocols
- Provenance model

### 🏗️ [SEL Server Architecture Design v1](./togather_SEL_server_architecture_design_v1.md)
**Status:** Updated 2026-01-20  
Complete system architecture including component design, API endpoints, and implementation guidelines.

**Key Topics:**
- Component overview (Go monolith design)
- RESTful API design with OpenAPI 3.1
- Database strategy (PostgreSQL + pgvector)
- Background jobs (River queue)
- Authentication & RBAC
- Federation layer
- MCP server integration

### 🗄️ [SEL Comprehensive Schema Design](./togather_schema_design.md)
**Status:** Living Document  
Detailed database schema with all tables, indexes, triggers, and SQL definitions.

**Key Topics:**
- Three-layer hybrid architecture (Document/Relational/Semantic)
- Core entity tables (events, places, organizations, persons)
- Event occurrences and series patterns
- Temporal and lifecycle management
- **Knowledge graph authorities registry** (NEW)
- Field-level provenance tracking
- Federation infrastructure
- Vector embeddings and search

---

## Integration Guides

### 🌐 [Multi-Graph Knowledge Graph Integration Strategy](./knowledge_graph_integration_strategy.md)
**Status:** Draft 2026-01-23  
**NEW**: Comprehensive guide for integrating with multiple knowledge graphs beyond Artsdata.

**Key Topics:**
- Supported knowledge graphs (Artsdata, Wikidata, MusicBrainz, ISNI, OpenStreetMap)
- Domain-based reconciliation routing (arts vs. sports vs. general events)
- Multi-graph reconciliation workflow
- Conflict resolution strategies
- Adding new knowledge graph authorities
- Implementation guidelines and testing strategy

**Why This Matters:**  
SEL now supports **non-arts events** (sports, community, education) by routing reconciliation to appropriate knowledge graphs. Arts events prioritize Artsdata, while general events use Wikidata directly.

### 🎨 [Artsdata Integration Guide](./artsdata-llm-integration-guide.md)
**Status:** Updated 2026-01-23  
Detailed guide for integrating with the Artsdata knowledge graph specifically.

**Key Topics:**
- Artsdata API endpoints (reconciliation, SPARQL, query API)
- Data model and SHACL validation
- Reconciliation workflow
- Publishing to Artsdata via Databus
- Wikidata cross-graph linking
- MCP server tool examples

**Note:** This guide now clarifies Artsdata's role as **one of several** knowledge graph authorities used by SEL.

---

## Domain Models

### 📊 [Togather Schema Design](./togather_schema_design.md)
See above in Core Architecture section.

---

## Quick Start Guide

**For Developers:**

1. **Start Here**: [SEL Interoperability Profile](./togather_SEL_Interoperability_Profile_v0.1.md) — understand the contracts
2. **System Design**: [Architecture Design](./togather_SEL_server_architecture_design_v1.md) — see how it all fits together
3. **Database Schema**: [Schema Design](./togather_schema_design.md) — implementation details
4. **Knowledge Graphs**: [Multi-Graph Integration Strategy](./knowledge_graph_integration_strategy.md) — reconciliation logic

**For API Consumers:**

1. Read the [Interoperability Profile](./togather_SEL_Interoperability_Profile_v0.1.md) sections:
   - § 2: Canonical JSON-LD Output
   - § 3: Minimal Required Fields
   - § 4: Export Formats
2. Check the OpenAPI spec at `/api/v1/openapi.json` (once deployed)

**For Knowledge Graph Integrators:**

1. [Multi-Graph Integration Strategy](./knowledge_graph_integration_strategy.md) — complete workflow
2. [Artsdata Integration Guide](./artsdata-llm-integration-guide.md) — Artsdata-specific details
3. [Schema Design § 5](./togather_schema_design.md#5-reconciliation-and-external-identifiers) — database tables

---

## Document Status

| Document | Status | Last Updated | Next Review |
|----------|--------|--------------|-------------|
| Interoperability Profile | Proposed for Review | 2026-01-20 | 2026-02-20 |
| Architecture Design | Updated | 2026-01-20 | 2026-02-20 |
| Schema Design | Living Document | 2026-01-21 | Ongoing |
| **Multi-Graph Strategy** | **Draft** | **2026-01-23** | **2026-02-23** |
| Artsdata Integration | Updated | 2026-01-23 | 2026-03-23 |

---

## Key Changes in v0.1.1 (2026-01-23)

### ✨ Multi-Graph Support

**Added comprehensive support for non-arts events and multiple knowledge graphs:**

1. **New Documentation**: [Multi-Graph Knowledge Graph Integration Strategy](./knowledge_graph_integration_strategy.md)
   - Defines domain-based reconciliation routing
   - Documents 5 supported knowledge graphs
   - Provides implementation guidelines

2. **Schema Updates**: [Schema Design § 5.1](./togather_schema_design.md#51-knowledge-graph-authorities-registry)
   - Added `knowledge_graph_authorities` table
   - Added `event_domain` field to events table
   - Updated `entity_identifiers` to reference authorities registry
   - Updated `reconciliation_cache` to track per-authority lookups

3. **Interoperability Profile Updates**: [§ 1.4](./togather_SEL_Interoperability_Profile_v0.1.md#14-sameas-usage)
   - Documented multi-graph linking
   - Added domain coverage matrix
   - Clarified validation rules

4. **Architecture Updates**: [Architecture § Reconciliation](./togather_SEL_server_architecture_design_v1.md#reconciliation-system)
   - Domain-aware routing examples
   - Multi-graph reconciliation pipeline
   - Per-authority caching strategy

**Migration Impact**: Existing implementations will need to:
- Add `knowledge_graph_authorities` table and seed data
- Update `entity_identifiers` foreign key
- Add `event_domain` to events (default 'arts' for backward compatibility)
- Update reconciliation logic to use routing rules

---

## Contributing

All documentation follows these conventions:
- **Markdown** with standard formatting
- **Code blocks** use triple backticks with language hints
- **Links** use relative paths within docs/
- **Dates** in ISO 8601 format (YYYY-MM-DD)
- **Status tags**: Draft, Proposed, Living Document, Deprecated

When updating docs:
1. Update the document's **Date** and **Version** fields
2. Add a note in this index's **Key Changes** section
3. Update **Next Review** dates as appropriate

---

## Questions or Feedback?

- **GitHub Issues**: [togather/server/issues](https://github.com/Togather-Foundation/server/issues)
- **Discussion**: [togather/discussions](https://github.com/Togather-Foundation/discussions)
- **Email**: [email protected]

---

**SEL Documentation** — Part of the [Togather Foundation](https://togather.foundation)  
*Building shared infrastructure for event discovery as a public good.*
