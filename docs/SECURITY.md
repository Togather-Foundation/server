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
**Mitigation**: JWT secret validation (32+ chars), API key hashing (SHA-256 → bcrypt)  
**Status**: 🔄 Partially mitigated (bcrypt migration P1)

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
**Impact**: Sensitive data in logs (tokens, credentials)  
**Mitigation**: Log sanitization (planned)  
**Status**: 📋 Backlog (P2)

---

## Implemented Protections

### 1. SQL Injection Prevention

**Protection**: Custom escaping function for ILIKE pattern queries

```go
// escapeILIKEPattern escapes special ILIKE pattern characters
func escapeILIKEPattern(input string) string {
    input = strings.ReplaceAll(input, "\\", "\\\\")  // Escape backslash first
    input = strings.ReplaceAll(input, "%", "\\%")     // Escape percent
    input = strings.ReplaceAll(input, "_", "\\_")     // Escape underscore
    return input
}
```

**Implementation Details**:
- All user-supplied text used in `ILIKE` queries is escaped via `escapeILIKEPattern()`
- All `ILIKE` queries include explicit `ESCAPE '\'` clause
- Protected fields: city, region, free-text search parameters
- SQLc provides additional compile-time safety via parameterized queries

**Test Coverage**: `internal/storage/postgres/events_repository_escape_test.go` (8 test cases including SQL injection attempts)

### 2. Rate Limiting

**Protection**: Role-based request throttling

| Role | Default Limit | Configurable Via |
|------|---------------|------------------|
| Public (unauthenticated) | 60 req/min | `RATE_LIMIT_PUBLIC` |
| Agent (API key auth) | 300 req/min | `RATE_LIMIT_AGENT` |
| Admin (JWT auth) | Unlimited (0) | `RATE_LIMIT_ADMIN` |

**Implementation Details**:
- Middleware applied to ALL public-facing endpoints (`/api/v1/events`, `/api/v1/places`, `/api/v1/organizations`)
- Per-IP tracking for public tier
- Per-API-key tracking for agent tier
- HTTP 429 (Too Many Requests) response on limit exceeded
- Exponential backoff recommended for clients

**Configuration**:
```bash
RATE_LIMIT_PUBLIC=60   # Requests per minute
RATE_LIMIT_AGENT=300
RATE_LIMIT_ADMIN=0     # 0 = unlimited
```

### 3. HTTP Server Hardening

**Protection**: Timeout and resource limit configuration

```go
server := &http.Server{
    Addr:           ":8080",
    ReadTimeout:    10 * time.Second,  // Prevent slow read attacks
    WriteTimeout:   30 * time.Second,  // Prevent slow write attacks
    MaxHeaderBytes: 1 << 20,           // 1 MB header limit
}
```

**Rationale**:
- **ReadTimeout (10s)**: Balances mobile/slow networks with protection against slowloris attacks
- **WriteTimeout (30s)**: Allows time for large responses (bulk exports) while preventing indefinite connections
- **MaxHeaderBytes (1 MB)**: Prevents memory exhaustion from oversized header attacks

### 4. JWT Secret Validation

**Protection**: Enforced minimum secret strength at startup

```go
func validateJWTSecret(secret string) error {
    const minLength = 32
    if len(secret) < minLength {
        return fmt.Errorf("JWT_SECRET must be at least %d characters (current: %d)", 
            minLength, len(secret))
    }
    return nil
}
```

**Implementation Details**:
- Application fails fast on startup if `JWT_SECRET` < 32 characters
- Prevents weak secrets like "secret", "123456", "test", "password"
- Recommended: Use `openssl rand -base64 48` for cryptographically random secrets
- Secrets should be rotated periodically (e.g., annually or on personnel changes)

### 5. API Key Hashing

**Current Implementation**: SHA-256 hashing (fast but vulnerable to offline brute-force)

**Planned Improvement (P1)**: Migration to bcrypt

```go
// Current (SHA-256)
hash := sha256.Sum256([]byte(apiKey))

// Planned (bcrypt with cost factor 12)
hash, err := bcrypt.GenerateFromPassword([]byte(apiKey), 12)
```

**Rationale for Migration**:
- SHA-256 is too fast (~billions of hashes/sec on modern GPUs)
- bcrypt's adaptive work factor (cost 12) makes brute-force attacks infeasible
- Cost factor 12 = ~300ms per attempt (acceptable for auth, prohibitive for cracking)
- Requires re-issuing API keys to agents (cannot reverse-hash SHA-256)

**Migration Plan**: Tracked in bead `server-jjf` (Priority 1)

### 6. Connection Pool Management

**Protection**: Explicit resource cleanup on error paths

```go
// Ensure pool is closed on initialization failure
pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
if err != nil {
    return nil, fmt.Errorf("failed to create connection pool: %w", err)
}

repo, err := postgres.NewEventsRepository(pool)
if err != nil {
    pool.Close()  // ← Explicit cleanup on error path
    return nil, fmt.Errorf("failed to initialize repository: %w", err)
}
```

**Implementation Details**:
- Connection pools are explicitly closed on initialization failures
- Deferred cleanup ensures resources are released even on panics
- Pool metrics should be exposed for monitoring (future work)

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

**P1 (High) - Partially Resolved**:
- ✅ HTTP server timeout configuration (`server-9zn`)
- ✅ Connection pool leak on error path (`server-0eo`)
- 🔄 API key hashing migration SHA-256 → bcrypt (`server-jjf`) - **In Progress**

**P2 (Medium) - Tracked**:
- 📋 Idempotency key expiration (`server-brb`)
- 📋 Missing database indexes (`server-blq`)
- 📋 Log sanitization (`server-itg`)
- 📋 Test coverage improvements (`server-3t3`)

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
