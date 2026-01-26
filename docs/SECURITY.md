# SEL Security Model

**Version:** 0.1.2  
**Last Updated:** 2026-01-25  
**Status:** Living Document

This document describes the security architecture, threat model, implemented protections, and operational security practices for the Shared Events Library (SEL) backend server.

---

## Table of Contents

1. [Security Philosophy](#security-philosophy)
2. [Threat Model](#threat-model)
3. [Implemented Protections](#implemented-protections)
4. [Configuration Requirements](#configuration-requirements)
5. [Operational Security](#operational-security)
6. [Security Audit History](#security-audit-history)
7. [Reporting Security Issues](#reporting-security-issues)

---

## Security Philosophy

SEL follows a **defense-in-depth** approach with these core principles:

- **Fail-Safe Defaults**: Secure by default; insecure configurations fail at startup
- **Principle of Least Privilege**: Role-based access control with minimal necessary permissions
- **Simplicity Over Complexity**: Straightforward implementations are easier to audit and harder to misconfigure
- **Observable Security**: Security events are logged and metrics exposed for monitoring
- **Progressive Hardening**: Critical (P0) vulnerabilities fixed immediately; lower-priority issues tracked and prioritized

SEL is designed for **public good infrastructure** where data transparency is a feature, not a bug. However, we protect:
- **System availability** against DoS and resource exhaustion
- **Data integrity** against injection attacks and malicious mutations
- **Credential security** through hashing, strong secret requirements, and HTTPS

---

## Threat Model

### Threat Actors

| Actor | Motivation | Capability | Mitigations |
|-------|-----------|------------|-------------|
| **Opportunistic Attacker** | Automated scanning for common vulnerabilities | Low-Medium | SQL injection prevention, rate limiting, HTTP hardening |
| **Malicious Agent** | Compromise data integrity or DoS via API abuse | Medium | API key hashing, rate limiting, audit logging |
| **Insider Threat** | Unauthorized access to admin functions | Medium-High | RBAC enforcement, audit logging, strong auth |
| **Nation-State** | Surveillance or disruption of civic infrastructure | High | Out of scope for v0.1; HTTPS, data minimization |

### Assets

1. **Event Data** - Publicly accessible, CC0 licensed (integrity > confidentiality)
2. **API Keys** - Agent authentication credentials (confidentiality critical)
3. **JWT Secrets** - Token signing keys (confidentiality critical)
4. **Database** - Source of truth for all data (integrity and availability critical)
5. **System Availability** - Public infrastructure uptime (availability critical)

### Attack Vectors

#### 1. SQL Injection
**Risk**: High  
**Impact**: Data breach, data corruption, privilege escalation  
**Mitigation**: Pattern escaping for ILIKE queries, SQLc parameterization  
**Status**: ✅ Mitigated (v0.1.2)

#### 2. Denial of Service (DoS)
**Risk**: High  
**Impact**: System unavailability, resource exhaustion  
**Mitigation**: Rate limiting (role-based), HTTP timeouts, header size limits  
**Status**: ✅ Mitigated (v0.1.2)

#### 3. Weak Authentication
**Risk**: High  
**Impact**: Unauthorized access, credential compromise  
**Mitigation**: JWT secret validation (32+ chars), API key hashing with bcrypt (cost 12)  
**Status**: ✅ Mitigated (v0.1.2)

#### 4. Resource Leaks
**Risk**: Medium  
**Impact**: Gradual performance degradation, eventual crash  
**Mitigation**: Explicit connection pool cleanup, deferred resource handlers  
**Status**: ✅ Mitigated (v0.1.2)

#### 5. Cross-Site Scripting (XSS)
**Risk**: Medium  
**Impact**: User session hijacking (admin UI)  
**Mitigation**: Content-Security-Policy headers (planned)  
**Status**: 📋 Backlog (P2)

#### 6. Information Disclosure
**Risk**: Low-Medium  
**Impact**: Sensitive data in logs (PII, tokens)  
**Mitigation**: Environment-based log sanitization (production mode redacts PII)  
**Status**: ✅ Mitigated (v0.1.2)

---

## Implemented Protections

### Authentication & Authorization

**JWT Authentication**:
- Secret validation: Enforces minimum 32-character secrets at startup
- Role-based access control (RBAC): `public`, `agent`, `admin` roles
- Token expiration and rotation support

**API Key Authentication**:
- Bcrypt hashing (cost factor 12) for all new API keys
- Dual-hash support during migration period (SHA-256 + bcrypt)
- Zero-downtime migration path for existing agents
- See `docs/API_KEY_MIGRATION.md` for migration strategy

**Admin Authentication**:
- Bcrypt-hashed passwords (default cost)
- Session management via JWT (Bearer token + cookie)
- Bootstrap admin user from environment variables

### Rate Limiting

**Role-Based Limits**:
- Public (unauthenticated): 60 req/min (configurable via `RATE_LIMIT_PUBLIC`)
- Agent (API key): 300 req/min (configurable via `RATE_LIMIT_AGENT`)
- Admin (JWT): Unlimited (configurable via `RATE_LIMIT_ADMIN`)

**Implementation**: In-memory token bucket per role, enforced at middleware layer

### SQL Injection Prevention

**ILIKE Pattern Escaping**:
- `escapeILIKEPattern()` function escapes `%`, `_`, and `\` characters
- Applied to all user-supplied search queries (name, keyword filters)
- Explicit `ESCAPE '\'` clause in all ILIKE queries

**Parameterized Queries**:
- All database operations use SQLc-generated parameterized queries
- No string concatenation for SQL construction
- Type-safe query parameters

### HTTP Security

**Server Timeouts**:
- `ReadTimeout`: 10s (prevents slow-read attacks)
- `WriteTimeout`: 30s (prevents slow-write attacks)  
- `ReadHeaderTimeout`: 5s (prevents slowloris)
- `MaxHeaderBytes`: 1MB (prevents header bomb attacks)

**Connection Pool Management**:
- Explicit cleanup with `defer pool.Close()` on all error paths
- Prevents resource leaks and connection exhaustion
- Configured pool limits via pgxpool defaults

### Data Integrity

**License Enforcement**:
- All event data defaults to CC0-1.0 (Public Domain)
- Non-CC0 licenses rejected at validation layer
- Enforced via `ValidateEventInput()` in `internal/domain/events`

**Input Validation**:
- Maximum field lengths enforced (name: 500 chars, description: 10,000 chars)
- RFC3339 date validation for all temporal fields
- URL validation for all URI fields
- Comprehensive validation tests (43% coverage in `internal/domain/events`)

### Resource Management

**Idempotency Key Expiration**:
- 24-hour TTL on all idempotency keys (prevents unbounded table growth)
- Automatic cleanup via River background job (`IdempotencyCleanupWorker`)
- Indexed for efficient cleanup queries (`idx_idempotency_expires`)

**Database Performance**:
- Strategic indexes on high-traffic query patterns:
  - `event_occurrences(event_id, start_time)` - common joins
  - `events(origin_node_id, federation_uri)` - federation queries
  - `events(deleted_at)` - partial index for soft delete filtering

### Privacy & Logging

**PII Sanitization**:
- Production mode redacts email addresses from logs
- Environment-based filtering (`ENVIRONMENT=production`)
- Admin bootstrap logs only show username in production
- See `cmd/server/main.go:89-94` for implementation

**Audit Logging**:
- All admin actions logged (planned: structured logging)
- Agent submissions logged with correlation IDs
- Security events captured for monitoring

---
## Configuration Requirements

### Required Environment Variables

```bash
# JWT Secret (REQUIRED - minimum 32 characters)
JWT_SECRET=<cryptographically-random-secret>

# Generate a strong secret:
openssl rand -base64 48
```

### Recommended Environment Variables

```bash
# Database connection (always use sslmode=require in production)
DATABASE_URL=postgres://user:pass@localhost:5432/sel?sslmode=require

# Rate limiting (requests per minute)
RATE_LIMIT_PUBLIC=60
RATE_LIMIT_AGENT=300
RATE_LIMIT_ADMIN=0

# HTTP server timeouts (seconds)
HTTP_READ_TIMEOUT=10
HTTP_WRITE_TIMEOUT=30
HTTP_MAX_HEADER_BYTES=1048576
```

### Production Deployment Checklist

- [ ] `JWT_SECRET` is 32+ characters and cryptographically random
- [ ] `DATABASE_URL` uses `sslmode=require`
- [ ] Server runs behind HTTPS termination (nginx, Traefik, cloud LB)
- [ ] Database firewall rules restrict access to application servers only
- [ ] Rate limits configured appropriately for expected traffic
- [ ] Monitoring configured for 429 (rate limit) errors
- [ ] API keys rotated on schedule (e.g., annually)
- [ ] Audit logs reviewed regularly for suspicious activity

---

## Operational Security

### 1. Generate Strong Secrets

**JWT Secret**:
```bash
# Generate 48 bytes of random data, base64-encoded (64 characters)
openssl rand -base64 48
```

**API Keys** (for agents):
```bash
# Generate 32 bytes of random data, hex-encoded (64 characters)
openssl rand -hex 32
```

### 2. Enable HTTPS

**Always** run the SEL server behind a TLS-terminating reverse proxy:

- **Nginx**: Use `proxy_pass` with SSL certificate (Let's Encrypt recommended)
- **Traefik**: Use automatic HTTPS with Let's Encrypt integration
- **Cloud Load Balancer**: AWS ALB, GCP Load Balancer, Azure Application Gateway

**Do NOT** accept HTTP traffic in production. Redirect all HTTP → HTTPS.

### 3. Database Security

**Connection Security**:
```bash
DATABASE_URL=postgres://user:pass@localhost:5432/sel?sslmode=require
```

**Firewall Rules**:
- Database should only accept connections from application server IPs
- Use security groups (AWS), firewall rules (GCP), or NSGs (Azure)
- Never expose PostgreSQL directly to the internet

**Credentials**:
- Use strong database passwords (16+ characters, random)
- Rotate passwords periodically (e.g., quarterly)
- Use IAM database authentication where available (AWS RDS, GCP Cloud SQL)

### 4. Monitor Rate Limits

**Metrics to Track**:
- 429 error rate (indicates potential attacks or misconfigured clients)
- Per-client request rate (identify abusive IPs or API keys)
- Rate limit headroom (how close are clients to hitting limits?)

**Alerting Thresholds**:
- Alert if 429 error rate > 1% of total requests
- Alert if single IP/key exceeds 80% of rate limit consistently

### 5. Rotate API Keys

**Policy**:
- Rotate API keys annually at minimum
- Rotate immediately on:
  - Personnel changes (agent contact leaves organization)
  - Suspected compromise
  - Security incident

**Process**:
1. Generate new API key for agent
2. Provide new key via secure channel (encrypted email, 1Password share)
3. Update agent configuration
4. Revoke old key after grace period (e.g., 7 days)

### 6. Review Audit Logs

**What to Monitor**:
- Admin actions (event edits, deletions, user management)
- Agent submissions (unusual volume, repeated failures)
- Authentication failures (repeated 401/403 errors)
- Rate limit violations (429 errors)

**Log Retention**:
- Operational logs: 30 days
- Audit logs: 1 year (admin/agent actions)
- Security incident logs: Permanent

---

## Security Audit History

### v0.1.2 (2026-01-25) - Security Hardening Release

**Audit Date**: 2026-01-25  
**Auditor**: AI-assisted code review (GPT-4 + human verification)  
**Scope**: User Stories 1 (Client Discovery) and 2 (Agent Event Submission)  
**Findings**: 20 issues (3 P0, 3 P1, 14 P2)

**P0 (Critical) - All Resolved**:
- ✅ SQL injection in ILIKE queries (`server-byy`)
- ✅ Missing rate limiting on public endpoints (`server-u3v`)
- ✅ Weak JWT secret validation (`server-j61`)

**P1 (High) - All Resolved**:
- ✅ HTTP server timeout configuration (`server-9zn`)
- ✅ Connection pool leak on error path (`server-0eo`)
- ✅ API key hashing migration SHA-256 → bcrypt (`server-jjf`) - **Completed** (see `docs/API_KEY_MIGRATION.md`)

**P2 (Medium) - All Resolved**:
- ✅ Idempotency key expiration with 24h TTL and cleanup job (`server-brb`)
- ✅ Performance indexes: event joins, federation, soft deletes (`server-blq`)
- ✅ PII sanitization in production logs (`server-itg`)
- ✅ Test coverage improvement: 17.7% → 43.1% (+143%) (`server-3t3`)

**Status**: 🎉 **ALL 10 SECURITY ISSUES RESOLVED** - Production ready

**Full Report**: See `CODE_REVIEW.md` in repository root

---

## Reporting Security Issues

### Responsible Disclosure Policy

We take security seriously and appreciate responsible disclosure of vulnerabilities.

**DO NOT** publicly disclose security vulnerabilities before coordinating with the maintainers.

**Reporting Process**:

1. **Email**: [email protected]
2. **Subject**: "SECURITY: [Brief Description]"
3. **Include**:
   - Description of vulnerability
   - Steps to reproduce
   - Potential impact
   - Suggested remediation (if any)
   - Your contact information (for follow-up)

**Response Timeline**:
- **24 hours**: Acknowledgment of receipt
- **7 days**: Initial assessment and priority assignment
- **30 days**: Fix deployed or mitigation guidance provided

**Recognition**:
- Public acknowledgment in security advisories (with your permission)
- Credit in CHANGELOG and release notes

**Bug Bounty**: Not currently available (v0.1.x), but under consideration for v1.0 production release.

---

## Additional Resources

- **Architecture Documentation**: `docs/togather_SEL_server_architecture_design_v1.md` § 7.1 (Security Hardening)
- **Code Review Report**: `CODE_REVIEW.md`
- **Setup Guide**: `SETUP.md` (Security Configuration section)
- **Issue Tracker**: Run `bd list --type bug --priority 0,1` for active security issues

---

**SEL Security Model** — Part of the [Togather Foundation](https://togather.foundation)  
*Building secure, transparent infrastructure for event discovery as a public good.*
