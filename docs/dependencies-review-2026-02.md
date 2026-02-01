# Legacy Dependencies Review - February 2026

**Issue:** server-zny9  
**Date:** 2026-02-01  
**Reviewed by:** OpenCode AI

## Executive Summary

Reviewed the project for legacy libraries (go-autorest, gogo/protobuf, gorilla/mux/handlers, 2015-2017 era libs). Found several old transitive dependencies but **none pose security or maintenance risks**. All are indirect dependencies from well-maintained libraries we actively use.

## Findings

### ✅ Target Libraries - Not Found (Good!)
- ❌ `github.com/Azure/go-autorest` - **FOUND** but safe (see below)
- ✅ `github.com/gogo/protobuf` - Not present
- ✅ `gorilla/mux` - Not present
- ✅ `gorilla/handlers` - Not present

### 🟡 Legacy Transitive Dependencies (2015-2017 era)

These old libraries exist as **transitive dependencies** but don't get compiled into our binary since we don't use their code paths:

#### 1. Azure go-autorest (v14.2.0+incompatible, 2020)
- **Source:** `github.com/golang-migrate/migrate/v4@v4.19.1` → Azure SQL driver support
- **Status:** SAFE - We only use the postgres driver, not Azure SQL
- **Action:** None required (migrate needs this for its Azure support, we're on latest migrate)

#### 2. Cloudflare golz4 (2015-02-17)
- **Source:** `github.com/golang-migrate/migrate/v4@v4.19.1` → Cassandra driver support
- **Status:** SAFE - We only use the postgres driver, not Cassandra
- **Action:** None required

#### 3. hailocab/go-hostpool (2016-01-25)
- **Source:** `github.com/golang-migrate/migrate/v4@v4.19.1` → Cassandra driver support
- **Status:** SAFE - We only use the postgres driver, not Cassandra
- **Action:** None required

#### 4. pborman/getopt (2017-01-12)
- **Source:** `github.com/oklog/ulid/v2@v2.1.1` → CLI tools (we don't use)
- **Status:** SAFE - We only import ulid.New() and core functions, not CLI tools
- **Action:** None required (oklog/ulid v2.1.1 is latest version)

### ✅ Gorilla Packages - Actively Maintained

We use `gorilla/csrf` v1.7.3 (last updated Jan 2025) which is **actively maintained** and current:
- `github.com/gorilla/csrf v1.7.3` - Direct dependency for CSRF protection
- `github.com/gorilla/securecookie v1.1.2` - Indirect via csrf
- `github.com/gorilla/css v1.0.1` - Indirect (not needed by our code)

**Used in:** `internal/api/middleware/csrf.go` for admin UI CSRF protection  
**Status:** Current and well-maintained  
**Action:** None required

## Why These Are Safe

1. **Not Compiled In:** Go's linker only includes code that's actually called. Since we:
   - Only use `postgres` driver in migrate (not Azure/Cassandra)
   - Only use ULID generation functions (not CLI tools)
   
   These legacy libs never make it into our binary.

2. **Latest Versions:** We're using the latest versions of our direct dependencies:
   - `golang-migrate/migrate/v4` v4.19.1 (latest, released Nov 2025)
   - `oklog/ulid/v2` v2.1.1 (latest stable)
   - `gorilla/csrf` v1.7.3 (actively maintained)

3. **No Security Issues:** `govulncheck` found 0 vulnerabilities affecting our code.

## Verification

Confirmed we only use safe drivers:
```go
// internal/storage/postgres/migrate.go
import (
    _ "github.com/golang-migrate/migrate/v4/database/postgres"  // Only postgres
    _ "github.com/golang-migrate/migrate/v4/source/file"
)
```

Build and tests pass successfully with current dependencies.

## Recommendations

### Short Term (Completed)
- ✅ Document findings
- ✅ Verify no security vulnerabilities
- ✅ Confirm build passes

### Long Term (Optional)
1. **Monitor migrate alternatives** - If `golang-migrate/migrate` becomes unmaintained, consider alternatives like:
   - `goose` (lighter, fewer transitive deps)
   - `atlas` (modern schema management)
   - Native SQL migration runner

2. **Monitor gorilla/csrf** - If gorilla toolkit becomes unmaintained, consider:
   - Building minimal CSRF middleware using `gorilla/securecookie` directly
   - Using framework-integrated CSRF (if we adopt a framework)

3. **Periodic Reviews** - Re-run this review annually or when upgrading major dependencies

## Conclusion

**No action required.** All legacy dependencies are transitive, unused in our binary, and come from actively maintained libraries we trust. The project is in good health regarding dependency management.
