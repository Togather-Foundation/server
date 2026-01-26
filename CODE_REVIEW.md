# Code Review: User Stories 1 & 2 Implementation
**Reviewer**: Senior Go Backend Engineer  
**Date**: 2026-01-25  
**Scope**: US1 (Client Discovery), US2 (Agent Event Submission)  
**Status**: Implementation Complete with Tests  

## Executive Summary

The implementation demonstrates solid Go fundamentals and generally follows best practices. Tests pass and coverage is reasonable for MVP. However, there are **critical security vulnerabilities** and several architectural concerns that must be addressed before production deployment.

**Overall Grade**: B- (Good structure, critical security gaps)

---

## 🔴 CRITICAL ISSUES (Must Fix Before Production)

### 1. **SQL Injection Vulnerability in Event List Query** 
**File**: `internal/storage/postgres/events_repository.go:68-108`  
**Severity**: CRITICAL

```go
AND ($3 = '' OR p.address_locality ILIKE '%' || $3 || '%')
AND ($4 = '' OR p.address_region ILIKE '%' || $4 || '%')
AND ($9 = '' OR (e.name ILIKE '%' || $9 || '%' OR e.description ILIKE '%' || $9 || '%'))
```

**Problem**: User input is interpolated into ILIKE patterns without escaping. A malicious query parameter like `%'; DROP TABLE events; --` could execute arbitrary SQL.

**Fix**: Use proper parameterized pattern matching:
```go
AND ($3 = '' OR p.address_locality ILIKE '%' || REPLACE(REPLACE(REPLACE($3, '\\', '\\\\'), '%', '\\%'), '_', '\\_') || '%')
```

Or better yet, use PostgreSQL's `ESCAPE` clause or full-text search instead of ILIKE with user input.

**Impact**: Database compromise, data loss, unauthorized access.

---

### 2. **Missing Rate Limiting on Public Endpoints**
**File**: `internal/api/router.go:53-54`  
**Severity**: CRITICAL

```go
publicEvents := http.HandlerFunc(eventsHandler.List)
mux.Handle("/api/v1/events", methodMux(map[string]http.Handler{
    http.MethodGet:  publicEvents,  // ← NO RATE LIMITING!
```

**Problem**: Public read endpoints have no rate limiting applied. The `RateLimit` middleware exists but is never wired in. This allows unlimited scraping/DoS attacks.

**Fix**: Apply rate limiting to ALL public endpoints:
```go
publicRateLimit := middleware.WithRateLimitTierHandler(middleware.TierPublic)
publicEvents := publicRateLimit(http.HandlerFunc(eventsHandler.List))
```

Then ensure the `RateLimit` middleware is applied globally in router setup.

**Impact**: DoS attacks, resource exhaustion, service unavailability.

---

### 3. **JWT Secret Validation Missing**
**File**: `internal/config/config.go:102-104`  
**Severity**: CRITICAL

```go
if cfg.Auth.JWTSecret == "" {
    return Config{}, fmt.Errorf("JWT_SECRET is required")
}
```

**Problem**: No validation of JWT secret strength. A weak secret like `"secret"` or `"123456"` would pass validation but be trivially brute-forced.

**Fix**: Enforce minimum entropy/length:
```go
if cfg.Auth.JWTSecret == "" {
    return Config{}, fmt.Errorf("JWT_SECRET is required")
}
if len(cfg.Auth.JWTSecret) < 32 {
    return Config{}, fmt.Errorf("JWT_SECRET must be at least 32 characters")
}
```

**Impact**: Authentication bypass, unauthorized admin access.

---

### 4. **Sensitive Data Logged in Production**
**File**: `cmd/server/main.go:86`  
**Severity**: HIGH

```go
logger.Info().Str("email", bootstrap.Email).Msg("bootstrapped admin user")
```

**Problem**: Admin email is logged unconditionally. In production, logs are often shipped to third-party services (Datadog, CloudWatch), exposing PII.

**Fix**: Redact sensitive data in production:
```go
if cfg.Environment == "development" {
    logger.Info().Str("email", bootstrap.Email).Msg("bootstrapped admin user")
} else {
    logger.Info().Msg("bootstrapped admin user")
}
```

---

### 5. **API Keys Stored with SHA-256 (Insufficient for Password-Equivalent Secrets)**
**File**: `internal/auth/apikey.go:90-93`  
**Severity**: HIGH

```go
func HashAPIKey(key string) string {
    sum := sha256.Sum256([]byte(key))
    return hex.EncodeToString(sum[:])
}
```

**Problem**: SHA-256 is too fast for hashing secrets that need password-strength protection. An attacker with database access could brute-force API keys offline at billions of hashes/second.

**Fix**: Use bcrypt or argon2 (same as user passwords):
```go
func HashAPIKey(key string) (string, error) {
    hash, err := bcrypt.GenerateFromPassword([]byte(key), bcrypt.DefaultCost)
    return string(hash), err
}
```

**Impact**: API key compromise if database is breached.

---

## 🟡 HIGH PRIORITY ISSUES

### 6. **Missing Request Timeout/Size Limits**
**File**: `cmd/server/main.go:37-41`  

```go
server := &http.Server{
    Addr:              fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
    Handler:           api.NewRouter(cfg, logger),
    ReadHeaderTimeout: 5 * time.Second,
}
```

**Missing**:
- `ReadTimeout`: No limit on total request read time (DoS via slowloris)
- `WriteTimeout`: No limit on response write time
- `MaxHeaderBytes`: No limit on header size (memory exhaustion)

**Fix**:
```go
server := &http.Server{
    Addr:              fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
    Handler:           api.NewRouter(cfg, logger),
    ReadTimeout:       10 * time.Second,
    WriteTimeout:      30 * time.Second,
    ReadHeaderTimeout: 5 * time.Second,
    MaxHeaderBytes:    1 << 20, // 1 MB
}
```

---

### 7. **Database Connection Pool Not Closed on Error**
**File**: `internal/api/router.go:24-28`  

```go
pool, err := pgxpool.New(ctx, cfg.Database.URL)
if err != nil {
    logger.Error().Err(err).Msg("database connection failed")
    return http.NewServeMux()  // ← pool never closed
}
```

**Problem**: Connection pool leak on error. The `defer pool.Close()` should be unconditional.

**Fix**:
```go
pool, err := pgxpool.New(ctx, cfg.Database.URL)
if err != nil {
    logger.Error().Err(err).Msg("database connection failed")
    return http.NewServeMux()
}
defer pool.Close() // Always close, even on later errors
```

---

### 8. **Idempotency Keys Lack Expiration**
**File**: Database schema `000001_core.up.sql` (idempotency_keys table not shown, assumed from code)

**Problem**: Idempotency keys stored forever. From `internal/domain/events/ingest.go:42-59`, keys are checked but never purged. This causes unbounded table growth.

**Fix**: Add `expires_at TIMESTAMPTZ` column and periodic cleanup job:
```sql
ALTER TABLE idempotency_keys ADD COLUMN expires_at TIMESTAMPTZ DEFAULT now() + INTERVAL '24 hours';
CREATE INDEX idx_idempotency_expiry ON idempotency_keys(expires_at) WHERE expires_at IS NOT NULL;
```

Clean up expired keys in a background job (River worker).

---

### 9. **Missing Index on Event Occurrences for Date Range Queries**
**File**: `internal/storage/postgres/migrations/000001_core.up.sql:283-292`  

**Problem**: The query in `events_repository.go:74-75` joins on `event_occurrences` filtering by `start_time`, but the only relevant index is:
```sql
CREATE INDEX idx_occurrences_time_range ON event_occurrences (start_time, end_time);
```

This is good, but queries also filter by `lifecycle_state` on events table. Missing composite index causes full table scans.

**Fix**: Add composite index for common query pattern:
```sql
CREATE INDEX idx_occurrences_event_time ON event_occurrences (event_id, start_time) 
    WHERE start_time >= now() - INTERVAL '1 year';
```

---

### 10. **Concurrent Map Access in Rate Limiter**
**File**: `internal/api/middleware/ratelimit.go:99-110`  

```go
func (s *limiterStore) limiter(tier RateLimitTier, key string) *rate.Limiter {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    if limiter, ok := s.limiters[lookup]; ok {
        return limiter  // ← returned limiter used outside lock
    }
    // ...
}
```

**Problem**: The `rate.Limiter` is accessed outside the mutex after being retrieved. While `rate.Limiter.Allow()` is internally synchronized, the pattern is fragile and could lead to race conditions if extended.

**Impact**: Low (rate.Limiter is safe), but violates Go concurrency best practices.

**Fix**: This is actually safe as-is, but for clarity, document why it's safe or use `sync.Map`.

---

## 🟢 MEDIUM PRIORITY ISSUES

### 11. **Environment Variable Defaults Are Too Permissive**
**File**: `internal/config/config.go:64-80`  

```go
Server: ServerConfig{
    Host:    getEnv("SERVER_HOST", "0.0.0.0"),  // Binds to all interfaces by default
    BaseURL: getEnv("SERVER_BASE_URL", "http://localhost:8080"),  // HTTP not HTTPS
},
```

**Problem**: Defaults suggest insecure deployment patterns.

**Recommendation**: 
- Default to `127.0.0.1` (localhost only) for safety
- Fail fast if `SERVER_BASE_URL` is HTTP in production:
```go
if cfg.Environment == "production" && strings.HasPrefix(cfg.Server.BaseURL, "http://") {
    return Config{}, fmt.Errorf("SERVER_BASE_URL must use HTTPS in production")
}
```

---

### 12. **Deduplication Hash Uses MD5 (Not Collision-Resistant)**
**File**: `internal/storage/postgres/migrations/000001_core.up.sql:189-195`  

```sql
dedup_hash TEXT GENERATED ALWAYS AS (
    md5(
        lower(trim(name)) ||
        COALESCE(primary_venue_id::text, COALESCE(virtual_url, 'null')) ||
        COALESCE(series_id::text, 'single')
    )
) STORED,
```

**Problem**: MD5 is cryptographically broken. While collision attacks are unlikely for deduplication, better alternatives exist.

**Fix**: Use SHA-256 or PostgreSQL's `digest('sha256')`:
```sql
dedup_hash TEXT GENERATED ALWAYS AS (
    encode(digest(
        lower(trim(name)) || 
        COALESCE(primary_venue_id::text, COALESCE(virtual_url, 'null')) ||
        COALESCE(series_id::text, 'single'),
        'sha256'
    ), 'hex')
) STORED,
```

---

### 13. **Missing Context Cancellation Checks in Long-Running Queries**
**File**: `internal/storage/postgres/events_repository.go:68-108`  

**Problem**: The `List()` query could run for seconds with large result sets, but never checks `ctx.Done()` during iteration.

**Fix**: Add cancellation check in loop:
```go
for rows.Next() {
    select {
    case <-ctx.Done():
        return events.ListResult{}, ctx.Err()
    default:
    }
    // ... scan rows
}
```

---

### 14. **API Error Responses Leak Stack Traces in Non-Dev Environments**
**File**: `internal/api/problem/problem.go` (not shown, but referenced in handlers)

**Problem**: From `internal/api/handlers/events.go:38`, errors are passed directly to `problem.Write()`. If the environment check isn't strict, internal error details could leak.

**Recommendation**: Review `problem.Write()` to ensure:
- Stack traces ONLY in `development` environment
- Database errors sanitized (don't expose table names, SQL)
- Use error codes instead of error text for production

---

### 15. **Missing Database Migration Rollback Tests**
**File**: `internal/storage/postgres/migrations/00000X_*.down.sql`  

**Problem**: Down migrations exist (good!) but there's no automated test ensuring they work. A broken rollback could brick production during emergency rollback.

**Recommendation**: Add integration test:
```go
func TestMigrationRollback(t *testing.T) {
    // Apply all up migrations
    // Apply all down migrations in reverse
    // Assert schema is back to empty state
}
```

---

## 🔵 LOW PRIORITY / CODE QUALITY ISSUES

### 16. **Inconsistent Error Wrapping**

Some functions use `fmt.Errorf("...: %w", err)` (correct), others use bare returns. Standardize on always wrapping errors with context.

**Example** (good): `internal/domain/events/ingest.go:124`  
**Example** (bad): `internal/api/handlers/events.go:44` (passes error without context)

---

### 17. **Magic Numbers for Retry Policies**

In `internal/jobs/workers.go` and config, retry counts like `5`, `10` lack explanation.

**Fix**: Use named constants with comments:
```go
const (
    RetryDedup         = 1   // No retries; duplicate = conflict
    RetryReconciliation = 5   // Allow transient network failures
    RetryEnrichment    = 10  // External APIs may be slow
)
```

---

### 18. **Test Coverage Gaps**

Current coverage (from `make coverage`):
- `internal/domain/events`: **17.7%** ← VERY LOW for core domain logic
- `internal/api/middleware`: **26.8%** ← Low for security-critical code
- `internal/api/handlers`: **54.1%** ← Below 80% target

**Recommendation**: Prioritize covering:
1. Event validation logic (currently 17.7%)
2. Authentication middleware (26.8%)
3. Edge cases in ingestion (dedup, idempotency)

---

### 19. **Unused Field in EventInput**

`internal/domain/events/validation.go:31-32`:
```go
ID   string `json:"@id,omitempty"`
Type string `json:"@type,omitempty"`
```

These are parsed but never used in `ValidateEventInput()`. Either use them or document why they're ignored.

---

### 20. **Missing Structured Logging in Critical Paths**

`internal/domain/events/ingest.go` has no logging. If ingestion fails, debugging requires code changes.

**Fix**: Add structured logging with event context:
```go
logger.Debug().
    Str("event_name", input.Name).
    Str("source_url", input.Source.URL).
    Msg("starting event ingestion")
```

---

## 📊 Database Schema Review

### Positive Observations:
✅ Proper use of UUIDs and ULIDs  
✅ Check constraints on enums and ranges  
✅ Foreign keys with `ON DELETE CASCADE` where appropriate  
✅ Indexes on common query patterns  
✅ Separate occurrences table (good normalization)  
✅ Generated columns for dedup_hash, full_address, geo_point  

### Concerns:

**1. Missing Indexes for Federation Queries**
```sql
-- Missing: Index for origin_node_id + federation_uri lookups
CREATE INDEX idx_events_federation_node ON events (origin_node_id, federation_uri) 
    WHERE federation_uri IS NOT NULL;
```

**2. Overly Broad NULL Constraints**
`events.organizer_id` and `primary_venue_id` allow NULL, but `event_location_required` check only validates one location exists. Should also validate organizer is set or has rational default.

**3. No Soft Delete Mechanism**
`deleted_at` column exists but no index:
```sql
CREATE INDEX idx_events_deleted ON events (deleted_at) WHERE deleted_at IS NOT NULL;
```

---

## 🧪 Test Quality Assessment

### Integration Tests: ✅ GOOD
- Comprehensive coverage of happy paths
- Good use of test fixtures in `helpers_test.go`
- Database cleanup between tests

### Contract Tests: ✅ GOOD
- RFC 7807 validation
- JSON-LD framing
- URI validation
- Pagination cursor encoding

### Missing Tests:
1. **Concurrency**: No tests for simultaneous ingestion of same event
2. **Error Recovery**: What happens if DB transaction fails mid-ingestion?
3. **Large Payloads**: No test with 10,000 character description or 100 keywords
4. **UTF-8 Boundary Conditions**: Emoji, RTL text, zero-width characters
5. **SQL Injection Attempts**: Should add negative security tests

---

## 🎯 Recommendations by Priority

### Immediate (Before Any Deployment):
1. Fix SQL injection in ILIKE queries (CRITICAL)
2. Add rate limiting to public endpoints (CRITICAL)
3. Validate JWT secret strength (CRITICAL)
4. Switch API key hashing to bcrypt (HIGH)
5. Add request timeouts to HTTP server (HIGH)

### Short Term (This Sprint):
6. Add idempotency key expiration
7. Improve test coverage to 80%+ in domain/events
8. Add security tests (SQL injection attempts, oversized payloads)
9. Review and sanitize error messages for production
10. Add structured logging to ingestion pipeline

### Medium Term (Next Sprint):
11. Implement distributed rate limiting (Redis) for multi-instance deployments
12. Add database query performance tests
13. Implement API versioning strategy
14. Add monitoring/alerting for security events
15. Document threat model and security assumptions

---

## ✅ Positive Highlights

1. **Clean Architecture**: Domain logic separated from HTTP handlers
2. **Repository Pattern**: Well-implemented abstraction over Postgres
3. **Idiomatic Go**: Proper use of context, error wrapping, struct embedding
4. **TDD Approach**: Tests written first, good discipline
5. **Migration Strategy**: Up/down migrations properly structured
6. **JSON-LD Support**: Proper Schema.org context handling
7. **Provenance Design**: Field-level tracking architecture is solid
8. **RFC Compliance**: Error responses follow RFC 7807

---

## 📝 Final Recommendations

### Code Quality: B+
- Well-structured, maintainable codebase
- Good separation of concerns
- Needs more inline documentation for complex logic

### Security: C (Would be F without fixes)
- **MUST fix SQL injection, rate limiting, JWT validation before ANY deployment**
- Good foundation (bcrypt for passwords, HTTPS in prod)
- Needs security audit before production

### Database Design: A-
- Excellent normalization
- Good use of Postgres features (GIN indexes, generated columns, PostGIS)
- Minor index optimizations needed

### Test Coverage: B
- Integration tests are solid
- Need more unit tests for edge cases
- Missing concurrency and performance tests

---

## 🚀 Sign-Off Checklist

Before merging to `main`:

- [ ] SQL injection vulnerabilities fixed
- [ ] Rate limiting enabled on all public endpoints  
- [ ] JWT secret validation added
- [ ] API key hashing switched to bcrypt
- [ ] HTTP server timeouts configured
- [ ] Test coverage for events domain ≥ 70%
- [ ] Security review by second engineer
- [ ] Performance testing under load (100 req/s)
- [ ] Error messages sanitized for production
- [ ] Documentation updated with security considerations

---

**Reviewed By**: Senior Go Backend Engineer  
**Review Date**: 2026-01-25  
**Next Review**: After critical fixes implemented
