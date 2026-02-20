# SEL Documentation

**Version:** 0.2.1

Welcome to the Shared Events Library (SEL) documentation. This guide will help you find the right documentation for your needs.

---

## Who are you?

### 🛠️ I'm contributing to the SEL codebase

**You're building features, fixing bugs, or improving the server implementation.**

**Start here:** [contributors/README.md](contributors/README.md)

**Quick links:**
- [Development Guide](contributors/development.md) - Coding standards, logging, validation
- [Architecture Overview](contributors/architecture.md) - System design
- [Database Guide](contributors/database.md) - Schema & migrations
- [Testing Guide](contributors/testing.md) - TDD workflow
- [Security Guide](contributors/security.md) - Security implementation

---

### 🔌 I'm integrating with SEL (scrapers, data submission)

**You're building event scrapers, submitting data from ticketing systems, or consuming the API.**

**Start here:** [integration/README.md](integration/README.md)

**Quick links:**
- [API Guide](integration/api-guide.md) - Practical API reference with examples
- [Authentication](integration/authentication.md) - API keys & rate limits
- [Scraper Best Practices](integration/scrapers.md) - Idempotency, deduplication
- [Code Examples](integration/examples/) - Working code samples

**Minimal example:**
```javascript
fetch('https://sel.togather.events/api/v1/events', {
  method: 'POST',
  headers: {
    'Authorization': `Bearer ${process.env.SEL_API_KEY}`,
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

### 🔧 I'm administering a SEL node

**You're operating a SEL server, managing users, or configuring deployment.**

**Start here:** [admin/admin-ui-overview.md](admin/admin-ui-overview.md)

**Quick links:**
- [Admin UI Components](admin/admin-ui-components.md) - Component library, templates, JavaScript architecture
- [User Management Guide](admin/user-management.md) - Create & manage user accounts
- [Email Setup Guide](admin/email-setup.md) - Configure SMTP for invitations
- [Admin Users API](api/admin-users.md) - API endpoint reference
- [Deployment Guide](deploy/quickstart.md) - Production deployment

---

### 🌐 I'm building a SEL-compatible node

**You're implementing your own SEL server or federating with the network.**

**Start here:** [interop/README.md](interop/README.md)

**Quick links:**
- [Core Profile v0.1](interop/core-profile-v0.1.md) - URI scheme, JSON-LD, validation
- [API Contract v1](interop/api-contract-v1.md) - HTTP API specification
- [Federation Protocol v1](interop/federation-v1.md) - Change feeds, sync protocol
- [Knowledge Graphs](interop/knowledge-graphs.md) - Multi-graph reconciliation
- [Artsdata Integration](interop/artsdata.md) - Artsdata-specific guide
- [SHACL Shapes](interop/schemas/) - Validation schemas

---

## Universal Resources

These resources are useful for everyone:

- **[Feature Reference](features.md)** - Complete list of capabilities organized by area
- **[Glossary](glossary.md)** - Canonical terminology reference
- **[Spec Kit](../specs/001-sel-backend/)** - Feature specifications
- **[OpenAPI Spec](https://sel.togather.events/api/v1/openapi.json)** - Machine-readable API *(when deployed)*

---

## Quick Start by Use Case

### Event Scraper
1. Read [API Guide](integration/api-guide.md)
2. Get API key (see [Authentication](integration/authentication.md))
3. Copy [minimal scraper example](integration/examples/minimal_scraper.js)
4. Submit events with `source.url` for duplicate detection

### SEL Node Implementer
1. Read [Core Profile](interop/core-profile-v0.1.md) for requirements
2. Validate against [SHACL shapes](interop/schemas/)
3. Implement [API Contract](interop/api-contract-v1.md)
4. Enable [Federation](interop/federation-v1.md) for network participation

### SEL Contributor
1. Read [Development Guide](contributors/development.md)
2. Pick a task: `bd ready`
3. Run tests: `make ci`
4. Follow TDD workflow in [Testing Guide](contributors/testing.md)

---

## Document Status

| Category | Document | Status | Last Updated |
|----------|----------|--------|--------------|
| **Universal** | Feature Reference | Living | 2026-02-20 |
| **Universal** | Glossary | Living | 2026-01-26 |
| **Admin** | Admin Directory README | Living | 2026-02-19 |
| **Admin** | Admin UI Overview | Living | 2026-02-19 |
| **Admin** | Admin UI Components | Living | 2026-02-19 |
| **Admin** | User Management Guide | Living | 2026-02-19 |
| **Admin** | Email Setup Guide | Living | 2026-02-05 |
| **Admin** | Admin Users API | Living | 2026-02-05 |
| **Integration** | API Guide | Living | 2026-01-27 |
| **Integration** | Authentication | Living | 2026-01-27 |
| **Integration** | Scrapers Guide | Living | 2026-01-27 |
| **Contributors** | Development | Living | 2026-01-26 |
| **Contributors** | Security | Living | 2026-01-25 |
| **Contributors** | Architecture | Living | 2026-01-27 |
| **Contributors** | Database | Living | 2026-01-27 |
| **Contributors** | Testing | Living | 2026-01-27 |
| **Design** | Design Directory README | Living | 2026-02-19 |
| **Operations** | Operations Directory README | Living | 2026-02-19 |
| **Interop** | Core Profile v0.1 | Draft | 2026-01-27 |
| **Interop** | API Contract v1 | Draft | 2026-01-27 |
| **Interop** | Federation v1 | Implemented | 2026-01-26 |
| **Interop** | Knowledge Graphs | Draft | 2026-01-23 |
| **Interop** | Artsdata | Updated | 2026-01-23 |

---

## Contributing to Documentation

All documentation follows these conventions:
- **Markdown** with GitHub-flavored formatting
- **Code blocks** use triple backticks with language hints
- **Links** use relative paths within docs/
- **Dates** in ISO 8601 format (YYYY-MM-DD)
- **Status tags**: Planned, Draft, Living, Implemented, Deprecated

When updating docs:
1. Update the document's date and version fields
2. Add note in root README changelog
3. Update cross-references if paths changed

---

## Questions or Feedback?

- **GitHub Issues**: [togather/server/issues](https://github.com/Togather-Foundation/server/issues)
- **Discussions**: [togather/discussions](https://github.com/Togather-Foundation/discussions)
- **Email**: [info@togather.foundation](mailto:info@togather.foundation)

---

**SEL Documentation** — Part of the [Togather Foundation](https://togather.foundation)  
*Building shared infrastructure for event discovery as a public good.*

---

**Last Updated:** 2026-01-27
