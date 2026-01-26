# API Key Migration Guide: SHA-256 to Bcrypt

**Date**: 2026-01-25  
**Status**: Implemented (v0.1.2)  
**Priority**: P1 (High)

---

## Overview

This document describes the migration from SHA-256 to bcrypt for API key hashing, addressing a critical security vulnerability where SHA-256's speed makes it vulnerable to offline brute-force attacks if the database is compromised.

**Security Impact:**
- **SHA-256**: ~1 billion hashes/second on modern GPUs → 8-character alphanumeric key cracked in ~30 minutes
- **Bcrypt (cost 12)**: ~300ms per hash → same key would take 190+ years to crack

---

## Architecture

### Dual-Hash Support

The system now supports **both** SHA-256 (legacy) and bcrypt (secure) simultaneously using a `hash_version` column:

| Version | Algorithm | Status | Use Case |
|---------|-----------|--------|----------|
| 1 | SHA-256 | Legacy | Existing keys (backwards compatibility) |
| 2 | bcrypt (cost 12) | Secure | New keys (default) |

### Database Schema

```sql
ALTER TABLE api_keys 
  ADD COLUMN hash_version INTEGER NOT NULL DEFAULT 1
  CHECK (hash_version IN (1, 2));
```

### Code Changes

**Before (SHA-256 only):**
```go
func HashAPIKey(key string) string {
    sum := sha256.Sum256([]byte(key))
    return hex.EncodeToString(sum[:])
}
```

**After (bcrypt with backwards compatibility):**
```go
// New keys use bcrypt
func HashAPIKey(key string) (string, error) {
    hash, err := bcrypt.GenerateFromPassword([]byte(key), BcryptCost)
    if err != nil {
        return "", err
    }
    return string(hash), nil
}

// Legacy SHA-256 support for validation only
func HashAPIKeySHA256(key string) string {
    sum := sha256.Sum256([]byte(key))
    return hex.EncodeToString(sum[:])
}

// Validation supports both versions
func ValidateAPIKey(ctx context.Context, store APIKeyStore, authHeader string) (*APIKey, error) {
    // ... prefix lookup ...
    
    switch stored.HashVersion {
    case HashVersionSHA256:
        providedHash := HashAPIKeySHA256(key)
        valid = subtle.ConstantTimeCompare([]byte(providedHash), []byte(stored.Hash)) == 1
    case HashVersionBcrypt:
        err := bcrypt.CompareHashAndPassword([]byte(stored.Hash), []byte(key))
        valid = (err == nil)
    }
    
    // ...
}
```

---

## Migration Strategies

### Strategy 1: In-Place Upgrade (Requires Downtime)

**Best for**: Small deployments with few API keys

**Process:**
1. Schedule maintenance window
2. Run database migration to add `hash_version` column
3. Re-issue all API keys to agents with bcrypt hashes
4. Update all agent configurations
5. Test authentication with new keys
6. Remove old SHA-256 keys

**Downtime**: 15-30 minutes (depending on number of agents)

### Strategy 2: Gradual Migration (Zero Downtime)

**Best for**: Production deployments with many agents

**Process:**
1. Deploy code with dual-hash support (v0.1.2+)
2. Run database migration (adds `hash_version` column, defaults to 1)
3. **Existing keys continue working** (validated as SHA-256)
4. Re-issue keys to agents gradually:
   - Generate new key with `HashAPIKey()` → bcrypt (version 2)
   - Provide new key to agent via secure channel
   - Agent updates configuration and tests
   - Deactivate old key after confirmation
5. Monitor migration progress: `SELECT hash_version, COUNT(*) FROM api_keys WHERE is_active GROUP BY hash_version;`
6. When all keys migrated (hash_version=2), consider removing SHA-256 support (future version)

**Downtime**: None (agents updated individually)

---

## Migration Commands

### 1. Run Database Migration

```bash
# Apply migration 000006
psql -U sel_user -d sel_db -f internal/storage/postgres/migrations/000006_apikey_bcrypt.up.sql
```

Or let the application auto-migrate on startup if using migration tools.

### 2. Monitor Migration Progress

```sql
-- Check distribution of hash versions
SELECT 
    hash_version,
    CASE hash_version 
        WHEN 1 THEN 'SHA-256 (legacy)'
        WHEN 2 THEN 'bcrypt (secure)'
    END as algorithm,
    COUNT(*) as count,
    ROUND(100.0 * COUNT(*) / SUM(COUNT(*)) OVER (), 2) as percentage
FROM api_keys
WHERE is_active = true
GROUP BY hash_version
ORDER BY hash_version;
```

**Example output:**
```
 hash_version |    algorithm     | count | percentage 
--------------+------------------+-------+------------
            1 | SHA-256 (legacy) |     3 |      60.00
            2 | bcrypt (secure)  |     2 |      40.00
```

### 3. Generate New API Key (Bcrypt)

**Manual generation for testing:**
```go
package main

import (
    "fmt"
    "github.com/Togather-Foundation/server/internal/auth"
    "github.com/oklog/ulid/v2"
)

func main() {
    // Generate key
    key := ulid.Make().String() + "secret" // Example: 32 characters
    prefix := key[:8]
    
    // Hash with bcrypt
    hash, err := auth.HashAPIKey(key)
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Key:        %s\n", key)
    fmt.Printf("Prefix:     %s\n", prefix)
    fmt.Printf("Hash:       %s\n", hash)
    fmt.Printf("Version:    %d (bcrypt)\n", auth.HashVersionBcrypt)
}
```

### 4. Insert New API Key

```sql
INSERT INTO api_keys (
    prefix,
    key_hash,
    hash_version,
    name,
    role,
    rate_limit_tier,
    is_active
) VALUES (
    '01HZK7QW',  -- First 8 chars of key
    '$2a$12$...',  -- Bcrypt hash from HashAPIKey()
    2,  -- bcrypt version
    'Agent XYZ',
    'agent',
    'agent',
    true
);
```

### 5. Deactivate Old Key

```sql
UPDATE api_keys 
SET is_active = false 
WHERE prefix = 'OLD_PREF';
```

---

## Agent Communication Template

**Subject**: REQUIRED: Update SEL API Key (Security Upgrade)

Dear [Agent Name],

We are upgrading our API key security from SHA-256 to bcrypt hashing. This change significantly improves the security of your credentials.

**Action Required:**
1. Replace your current API key with the new key below
2. Update your application configuration
3. Test authentication by making a test API call
4. Reply to confirm successful migration

**New API Key:**
```
[INSERT NEW BCRYPT KEY HERE]
```

**Configuration:**
```bash
# Update your .env or config file
SEL_API_KEY="[NEW KEY]"
```

**Test Command:**
```bash
curl -H "Authorization: Bearer [NEW KEY]" https://sel.example.com/api/v1/events?limit=1
```

**Timeline:**
- **By [DATE]**: Update your configuration
- **[DATE + 7 days]**: Old key will be deactivated

If you experience any issues, please contact [email protected].

Thank you for your cooperation in keeping our infrastructure secure.

---
SEL Operations Team

---

## Testing

### Unit Tests

```bash
# Run API key tests
go test ./internal/auth -v -run="TestValidateAPIKey|TestHash"
```

**Test coverage:**
- ✅ Bcrypt hash generation (non-deterministic, unique salts)
- ✅ Bcrypt validation (correct key passes)
- ✅ Legacy SHA-256 validation (backwards compatibility)
- ✅ Wrong key rejection (both versions)
- ✅ Expired key rejection
- ✅ Inactive key rejection

### Integration Tests

```bash
# Run full integration test suite
make test-integration
```

### Manual Validation

```bash
# 1. Create test key with bcrypt
psql -c "INSERT INTO api_keys (prefix, key_hash, hash_version, name) VALUES ('test1234', '\$2a\$12\$...', 2, 'Test Key');"

# 2. Test authentication
curl -H "Authorization: Bearer test1234secret" http://localhost:8080/api/v1/events

# 3. Verify hash_version in logs
grep "hash_version" /var/log/sel/auth.log
```

---

## Rollback Plan

If issues arise during migration:

### Immediate Rollback (Code)

1. Revert to previous version without bcrypt support
2. All SHA-256 keys continue working (hash_version=1 is default)
3. No data loss

### Database Rollback

```bash
# Rollback migration 000006
psql -U sel_user -d sel_db -f internal/storage/postgres/migrations/000006_apikey_bcrypt.down.sql
```

**Note**: Only rollback if NO bcrypt keys (hash_version=2) have been issued. Check first:

```sql
SELECT COUNT(*) FROM api_keys WHERE hash_version = 2;
-- If result > 0, do NOT rollback (keys would become invalid)
```

---

## Security Considerations

### Why Bcrypt?

| Feature | SHA-256 | Bcrypt (cost 12) |
|---------|---------|------------------|
| Speed | ~1 billion/sec (GPU) | ~3 hashes/sec | 
| Salt | No | Yes (automatic, random) |
| Adaptive | No | Yes (configurable cost) |
| GPU Resistant | No | Yes |
| Brute-force time (8 chars) | 30 minutes | 190+ years |

### Bcrypt Cost Factor

**Current**: `BcryptCost = 12` (~300ms per hash)

**Rationale:**
- Cost 10 (~75ms): Too fast, vulnerable in 5-10 years as hardware improves
- Cost 12 (~300ms): Recommended by OWASP, acceptable auth latency
- Cost 14 (~1.2s): Too slow for API authentication

**Future**: Consider increasing to cost 13 in 2028-2030 as CPUs improve.

### Attack Scenarios Mitigated

1. **Database Breach + Offline Cracking**:
   - Before: Attacker cracks 8-char keys in hours (SHA-256)
   - After: Attacker needs centuries (bcrypt cost 12)

2. **Rainbow Table Attacks**:
   - Before: Pre-computed tables could crack unsalted SHA-256
   - After: Impossible (bcrypt uses random salts per hash)

3. **Timing Attacks**:
   - Before: Constant-time compare mitigated this
   - After: Still mitigated (bcrypt naturally resistant)

---

## Post-Migration Checklist

- [ ] Database migration applied successfully
- [ ] All agents notified with new keys
- [ ] All agents confirmed successful migration
- [ ] Old SHA-256 keys deactivated
- [ ] Monitor errors for authentication failures
- [ ] Update documentation with completion date
- [ ] Update CODE_REVIEW.md to mark `server-jjf` as resolved
- [ ] Close bead `server-jjf` with migration details

---

## References

- **Code Review**: `CODE_REVIEW.md` § Issue #5
- **Bead**: `server-jjf` (SECURITY: Switch API key hashing from SHA-256 to bcrypt)
- **Security Model**: `docs/SECURITY.md` § API Key Security
- **OWASP**: [Password Storage Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html)

---

**SEL API Key Migration Guide** — Part of the [Togather Foundation](https://togather.foundation)  
*Implementing defense-in-depth security for public infrastructure.*
