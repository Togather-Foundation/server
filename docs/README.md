# SEL Documentation

**Version:** 0.2.0  
**Last Updated:** 2026-01-27

Welcome to the Shared Events Library (SEL) documentation. This guide will help you find the right documentation for your needs.

---

## Who are you?

### 🛠️ I'm contributing to the SEL codebase

**You're building features, fixing bugs, or improving the server implementation.**

**Start here:** [contributors/README.md](contributors/README.md)

**Quick links:**
- [Development Guide](contributors/DEVELOPMENT.md) - Coding standards, logging, validation
- [Architecture Overview](contributors/ARCHITECTURE.md) - System design (TODO: not yet created)
- [Database Guide](contributors/DATABASE.md) - Schema & migrations (TODO: not yet created)
- [Testing Guide](contributors/TESTING.md) - TDD workflow (TODO: not yet created)
- [Security Guide](contributors/SECURITY.md) - Security implementation

---

### 🔌 I'm integrating with SEL (scrapers, data submission)

**You're building event scrapers, submitting data from ticketing systems, or consuming the API.**

**Start here:** [integration/README.md](integration/README.md)

**Quick links:**
- [API Guide](integration/API_GUIDE.md) - Practical API reference with examples
- [Authentication](integration/AUTHENTICATION.md) - API keys & rate limits (TODO: not yet created)
- [Scraper Best Practices](integration/SCRAPERS.md) - Idempotency, deduplication (TODO: not yet created)
- [Code Examples](integration/examples/) - Working code samples (TODO: not yet created)

**Minimal example:**
```javascript
fetch('https://sel.togather.events/api/v1/events', {
  method: 'POST',
  headers: {
    'X-API-Key': process.env.SEL_API_KEY,
    'Content-Type': 'application/json'
  },
  body: JSON.stringify({
    name: 'Event Name',
    startDate: '2026-02-15T20:00:00-05:00',
    location: { name: 'Venue Name' },
    source: { url: 'https://source.com/event-123' }
  })
})
```

---

### 🌐 I'm building a SEL-compatible node

**You're implementing your own SEL server or federating with the network.**

**Start here:** [interop/README.md](interop/README.md)

**Quick links:**
- [Core Profile v0.1](interop/CORE_PROFILE_v0.1.md) - URI scheme, JSON-LD, validation
- [API Contract v1](interop/API_CONTRACT_v1.md) - HTTP API specification
- [Federation Protocol v1](interop/FEDERATION_v1.md) - Change feeds, sync protocol
- [Knowledge Graphs](interop/KNOWLEDGE_GRAPHS.md) - Multi-graph reconciliation
- [Artsdata Integration](interop/ARTSDATA.md) - Artsdata-specific guide
- [SHACL Shapes](interop/schemas/) - Validation schemas

---

## Universal Resources

These resources are useful for everyone:

- **[Glossary](glossary.md)** - Canonical terminology reference
- **[Spec Kit](../specs/001-sel-backend/)** - Feature specifications
- **[OpenAPI Spec](https://sel.togather.events/api/v1/openapi.json)** - Machine-readable API *(when deployed)*

---

## Quick Start by Use Case

### Event Scraper
1. Read [API Guide](integration/API_GUIDE.md)
2. Get API key (see [Authentication](integration/AUTHENTICATION.md) - TODO: not yet created)
3. Copy [minimal scraper example](integration/examples/minimal_scraper.js) (TODO: not yet created)
4. Submit events with `source.url` for duplicate detection

### SEL Node Implementer
1. Read [Core Profile](interop/CORE_PROFILE_v0.1.md) for requirements
2. Validate against [SHACL shapes](interop/schemas/)
3. Implement [API Contract](interop/API_CONTRACT_v1.md)
4. Enable [Federation](interop/FEDERATION_v1.md) for network participation

### SEL Contributor
1. Read [Development Guide](contributors/DEVELOPMENT.md)
2. Pick a task: `bd ready`
3. Run tests: `make ci`
4. Follow TDD workflow in [Testing Guide](contributors/TESTING.md) (TODO: not yet created)

---

## Document Status

| Category | Document | Status | Last Updated |
|----------|----------|--------|--------------|
| **Universal** | Glossary | Living | 2026-01-26 |
| **Integration** | API Guide | Living | 2026-01-27 |
| **Integration** | Authentication | Planned | - |
| **Integration** | Scrapers Guide | Planned | - |
| **Contributors** | Development | Living | 2026-01-26 |
| **Contributors** | Security | Living | 2026-01-25 |
| **Contributors** | Architecture | Planned | - |
| **Contributors** | Database | Planned | - |
| **Contributors** | Testing | Planned | - |
| **Interop** | Core Profile v0.1 | Planned | - |
| **Interop** | API Contract v1 | Planned | - |
| **Interop** | Federation v1 | Implemented | 2026-01-26 |
| **Interop** | Knowledge Graphs | Draft | 2026-01-23 |
| **Interop** | Artsdata | Updated | 2026-01-23 |

---

## Key Changes in v0.2.0 (2026-01-27)

### 📁 Documentation Reorganization

**Major restructuring for improved user experience:**

1. **Audience-Based Organization**
   - `contributors/` - Internal developers
   - `integration/` - Event scrapers and API consumers
   - `interop/` - SEL node implementers

2. **Clearer Entry Points**
   - Each audience has a dedicated README with navigation
   - Root README routes users to appropriate section

3. **Planned Improvements**
   - Split large documents (Interoperability Profile → 3 focused contracts)
   - Extract focused guides (Authentication, Scrapers, Testing)
   - Add working code examples

**Why This Matters:**
- Reduces cognitive load - you only read docs relevant to your role
- Clearer contracts for interoperability
- Easier to maintain and version independently

---

## Contributing to Documentation

All documentation follows these conventions:
- **Markdown** with GitHub-flavored formatting
- **Code blocks** use triple backticks with language hints
- **Links** use relative paths within docs/
- **Dates** in ISO 8601 format (YYYY-MM-DD)
- **Status tags**: Planned, Draft, Proposed, Living, Implemented, Deprecated

When updating docs:
1. Update the document's date and version fields
2. Add note in root README changelog
3. Update cross-references if paths changed

---

## Questions or Feedback?

- **GitHub Issues**: [togather/server/issues](https://github.com/Togather-Foundation/server/issues)
- **Discussions**: [togather/discussions](https://github.com/Togather-Foundation/discussions)
- **Email**: [email protected]

---

**SEL Documentation** — Part of the [Togather Foundation](https://togather.foundation)  
*Building shared infrastructure for event discovery as a public good.*
