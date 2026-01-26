# SEL Documentation Index

**Version:** 0.1.2  
**Last Updated:** 2026-01-25

This directory contains the comprehensive technical documentation for the Shared Events Library (SEL) backend.

---

## Core Architecture Documents

### 📖 [Terminology Glossary](./glossary.md)
**Status:** Living Document (NEW - v0.1.3)  
Canonical definitions for all SEL-specific terminology used across documentation and codebase.

**Key Topics:**
- Core concepts (change feed, lifecycle state, field provenance, federation URI)
- Authentication & authorization (API keys, roles, RBAC)
- Data model (ULID, event occurrences, tombstones)
- Provenance & sources (trust levels, confidence scores)
- Federation (origin nodes, sequence numbers)
- Schema.org alignment (event status, attendance mode)
- API patterns (cursor pagination, idempotency keys)
- Testing & validation (contract tests, SHACL validation)

**Use this glossary** to ensure consistent terminology in code, docs, and discussions.

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
**Status:** Updated 2026-01-25  
Complete system architecture including component design, API endpoints, and implementation guidelines.

**This document serves as the implementation plan** for the SEL backend, providing:
- Technology stack decisions and rationale
- Detailed component architecture and interfaces
- API endpoint specifications with OpenAPI 3.1
- Database schema strategy and migration approach
- Security model and threat mitigation
- Testing strategy and quality gates

**Key Topics:**
- Component overview (Go monolith design)
- RESTful API design with OpenAPI 3.1
- Database strategy (PostgreSQL + pgvector)
- Background jobs (River queue)
- Authentication & RBAC
- **§ 7.1 Security Hardening** (NEW)
- Federation layer
- MCP server integration

### 🔒 [SEL Security Model](./SECURITY.md)
**Status:** Living Document (NEW - v0.1.2)  
Comprehensive security architecture, threat model, implemented protections, and operational security practices.

**Key Topics:**
- Threat model and attack vectors
- SQL injection prevention
- Rate limiting (role-based)
- HTTP server hardening
- JWT secret validation
- API key security (SHA-256 → bcrypt migration)
- Configuration requirements
- Operational security best practices
- Security audit history
- Responsible disclosure policy

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

### 🛠️ [Development Guide](./DEVELOPMENT.md)
**Status:** Living Document  
Guidelines and best practices for developing the SEL backend. Complements the Architecture Design's implementation plan with practical development workflows.

**Key Topics:**
- Logging standards (zerolog, structured logging)
- Request correlation IDs
- PII and sensitive data handling
- **SHACL validation and Turtle serialization** (NEW)
  - Setup and configuration
  - Turtle datatype coercion requirements
  - pyshacl multi-shape file limitation
  - uvx vs direct execution
  - Testing and debugging
- Background job logging (River + slog)

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

**Implementation Plan Reference:**  
The SEL backend implementation follows a specification-driven approach documented in:
1. **Feature Specification**: `specs/001-sel-backend/spec.md` — What to build (user needs)
2. **Implementation Plan**: `specs/001-sel-backend/plan.md` — How to build it (tech stack, structure)
3. **Architecture Design**: [Architecture Design v1](./togather_SEL_server_architecture_design_v1.md) — Detailed implementation guidance
4. **Task Breakdown**: `specs/001-sel-backend/tasks.md` — Step-by-step TDD execution plan

**For Developers:**

1. **Start Here**: [Terminology Glossary](./glossary.md) — learn the vocabulary
2. **Understand Contracts**: [SEL Interoperability Profile](./togather_SEL_Interoperability_Profile_v0.1.md) — API contracts
3. **System Design**: [Architecture Design](./togather_SEL_server_architecture_design_v1.md) — implementation plan
4. **Development Guide**: [Development Guide](./DEVELOPMENT.md) — logging, SHACL validation, best practices
5. **Security**: [Security Model](./SECURITY.md) — threat model, protections, operations
6. **Database Schema**: [Schema Design](./togather_schema_design.md) — implementation details
7. **Knowledge Graphs**: [Multi-Graph Integration Strategy](./knowledge_graph_integration_strategy.md) — reconciliation logic

**For API Consumers:**

1. Read the [Interoperability Profile](./togather_SEL_Interoperability_Profile_v0.1.md) sections:
   - § 2: Canonical JSON-LD Output
   - § 3: Minimal Required Fields
   - § 4: Export Formats
2. Check the OpenAPI spec at `/api/v1/openapi.json` (once deployed)
3. Review [Security Model](./SECURITY.md) for rate limits and authentication requirements

**For Operators:**

1. [Security Model](./SECURITY.md) — configuration requirements and operational best practices
2. [Architecture Design § 7.1](./togather_SEL_server_architecture_design_v1.md#71-security-hardening) — security implementation details
3. [Schema Design](./togather_schema_design.md) — database setup and migrations

**For Knowledge Graph Integrators:**

1. [Multi-Graph Integration Strategy](./knowledge_graph_integration_strategy.md) — complete workflow
2. [Artsdata Integration Guide](./artsdata-llm-integration-guide.md) — Artsdata-specific details
3. [Schema Design § 5](./togather_schema_design.md#5-reconciliation-and-external-identifiers) — database tables

---

## Document Status

| Document | Status | Last Updated | Next Review |
|----------|--------|--------------|-------------|
| **Glossary** | **Living Document** | **2026-01-26** | **Ongoing** |
| Interoperability Profile | Proposed for Review | 2026-01-20 | 2026-02-20 |
| Architecture Design | Updated | 2026-01-25 | 2026-02-25 |
| **Security Model** | **Living Document** | **2026-01-25** | **Ongoing** |
| **Development Guide** | **Living Document** | **2026-01-26** | **Ongoing** |
| Schema Design | Living Document | 2026-01-21 | Ongoing |
| **Multi-Graph Strategy** | **Draft** | **2026-01-23** | **2026-02-23** |
| Artsdata Integration | Updated | 2026-01-23 | 2026-03-23 |

---

## Key Changes in v0.1.3 (2026-01-26)

### 📖 New Terminology Glossary

**Created comprehensive canonical terminology reference:**

1. **New Document**: [Terminology Glossary](./glossary.md)
   - 25+ defined terms across 8 categories
   - Core concepts: change feed, lifecycle state, field provenance, federation URI
   - Authentication: API keys, roles, RBAC
   - Data model: ULID, event occurrences, tombstones, sequence numbers
   - Federation: origin nodes, federation sync patterns
   - Schema.org alignment: event status, attendance mode
   - API patterns: cursor pagination, idempotency keys
   - Testing: contract tests, SHACL validation

2. **Documentation Updates**: [README](./README.md)
   - Added glossary to core architecture documents
   - Clarified implementation plan structure (spec → plan → architecture → tasks)
   - Updated developer quick start to reference glossary first
   - Added glossary to document status tracking

**Why This Matters:**
- Ensures consistent terminology across codebase and documentation
- Reduces onboarding time for new developers
- Provides canonical definitions for code reviews and discussions
- Cross-references related concepts for deeper understanding

### 🔧 SHACL Validation Improvements

**Fixed SHACL validation to work correctly with uvx and proper RDF serialization:**

1. **Development Guide Updates**: [Development Guide § SHACL](./DEVELOPMENT.md#shacl-validation-and-turtle-serialization)
   - Comprehensive SHACL validation documentation
   - Turtle serialization requirements and datatype coercion
   - pyshacl multi-shape file limitation workaround
   - uvx vs direct pyshacl execution
   - Testing and debugging guide
   - Performance considerations

2. **Technical Fixes** (commit `bf95fb1`):
   - **Fixed pyshacl multi-shape handling**: Merge all shape files into single temp file (pyshacl doesn't handle multiple `-s` flags correctly)
   - **Fixed Turtle datetime serialization**: Add `^^xsd:dateTime` and `^^xsd:date` type coercion for Schema.org date properties
   - **Added uvx support**: Automatically detect and use `uvx` (modern Python package runner) before falling back to direct `pyshacl`

3. **Test Coverage**:
   - ✅ All 31 jsonld tests pass (was 27/31)
   - ✅ 4/4 validator tests pass (was 1/4 - tests were inverted)
   - ✅ Coverage: 79.4% (increased from 70.6%)
   - ✅ SHACL validation correctly detects missing required fields

**Why This Matters:**
- SHACL validation ensures SEL nodes produce conformant JSON-LD output
- Proper RDF datatypes are critical for semantic interoperability
- uvx support enables validation in modern Python environments (PEP 668)

---

## Key Changes in v0.1.2 (2026-01-25)

### 🔒 Security Hardening

**Major security improvements based on comprehensive code review:**

1. **New Documentation**: [Security Model](./SECURITY.md)
   - Comprehensive threat model and attack vector analysis
   - Detailed documentation of implemented protections
   - Operational security best practices
   - Security audit history
   - Responsible disclosure policy

2. **Architecture Updates**: [Architecture § 7.1](./togather_SEL_server_architecture_design_v1.md#71-security-hardening)
   - SQL injection prevention (ILIKE escaping)
   - Rate limiting (role-based: 60/300/0 req/min)
   - HTTP server hardening (timeouts, header limits)
   - JWT secret validation (32+ character minimum)
   - API key security (bcrypt migration completed with zero-downtime support)
   - Connection pool leak fixes
   - Idempotency key expiration (24h TTL with automatic cleanup)
   - Performance database indexes (event joins, federation queries)
   - PII sanitization in production logs

3. **Code Review**: `CODE_REVIEW.md` (repository root)
   - 20 issues identified and categorized (P0-P2)
   - ✅ All 3 critical (P0) vulnerabilities resolved
   - ✅ All 3 high-priority (P1) issues resolved
   - ✅ All 4 tracked medium-priority (P2) improvements completed

4. **Configuration Requirements**: `SETUP.md` updated
   - Security-focused environment variable documentation
   - Strong secret generation guidance
   - Production deployment checklist

**Security Fixes (v0.1.2)**:

**P0 (Critical) - All Resolved:**
- ✅ SQL injection in ILIKE queries (`server-byy`)
- ✅ Missing rate limiting on public endpoints (`server-u3v`)
- ✅ Weak JWT secret validation (`server-j61`)

**P1 (High) - All Resolved:**
- ✅ HTTP server timeouts (`server-9zn`)
- ✅ Connection pool leak (`server-0eo`)
- ✅ API key hashing migration SHA-256 → bcrypt (`server-jjf`) - see [API_KEY_MIGRATION.md](./API_KEY_MIGRATION.md)

**P2 (Medium) - All Resolved:**
- ✅ Idempotency key expiration with 24h TTL and cleanup job (`server-brb`)
- ✅ Performance indexes: event joins, federation, soft deletes (`server-blq`)
- ✅ PII sanitization in production logs (`server-itg`)
- ✅ Test coverage improvement: 17.7% → 43.1% (+143%) (`server-3t3`)

**Result**: 🎉 **ALL 10 SECURITY ISSUES RESOLVED** - Production-ready security posture achieved.

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
