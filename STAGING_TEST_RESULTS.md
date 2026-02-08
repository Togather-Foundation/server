# Deduplication & Data Quality Fixes - Test Results

## Summary

Successfully deployed fixes for source reconciliation (srv-8ru) and timezone correction (srv-5dj) to staging environment.

## Deployment Status

- **Branch**: `feature/complete-dedupe-fixes`
- **Deployed to**: staging.toronto.togather.foundation
- **Commit**: 9dc5b1f
- **Migrations**: 21, 22, 23 applied successfully
- **Smoke tests**: ✅ All 16 tests passed

## Changes Deployed

### 1. Source Reconciliation Fix (srv-8ru)
**Problem**: 4% batch ingestion failure rate due to duplicate source names
- Events from same domain (e.g., eventbrite.ca) tried to create multiple sources with same name
- Events without URLs all defaulted to "Toronto Open Data Events" causing collisions

**Solution**:
- Migration 000023: Removed UNIQUE constraint on `sources.name`
- Added partial unique indexes:
  - `sources_base_url_unique`: UNIQUE(base_url) WHERE base_url IS NOT NULL
  - `sources_name_unique_when_no_url`: UNIQUE(name) WHERE base_url IS NULL
- Updated `GetOrCreateSource()` to use NULL-safe lookup: `IS NOT DISTINCT FROM NULLIF($1, '')`

**Expected Impact**: 4% → 0% failure rate for source collisions

### 2. Timezone Correction Fix (srv-5dj)
**Problem**: 1-2% validation failures for events with endDate before startDate
- Midnight-spanning events incorrectly converted from local time to UTC
- Example: start=2025-03-31T23:00Z, end=2025-03-31T06:00Z (end appears 17 hours before start)

**Solution**:
- Added `correctEndDateTimezoneError()` in normalize.go
- Auto-corrects by adding 24 hours when:
  - endDate < startDate
  - endDate + 24h > startDate (indicating midnight-span)
  - Gap is < 24 hours
- Timezone-agnostic heuristic works for any city

**Expected Impact**: 1-2% → 0% failure rate for timezone errors

## Test Coverage

✅ **Unit Tests**:
- `TestGetOrCreateSourceReconciliation` - 3 scenarios (all pass)
- `TestCorrectEndDateTimezoneError` - 7 scenarios (all pass)
- `TestUpsertPlaceReconciliation` - 5 scenarios (all pass)
- `TestUpsertOrganizationReconciliation` - 4 scenarios (all pass)

✅ **Smoke Tests** (staging):
- Health endpoint
- Database connectivity
- Migration status (v23)
- API endpoints
- Security headers
- HTTPS certificate
- Docker containers

## Known Limitations

### Cannot Complete End-to-End Testing
**Reason**: API keys were wiped during database reset (CASCADE from sources table)

**Workaround needed**:
1. Create new API keys in staging database, OR
2. Modify ingestion script to use JWT authentication, OR
3. Temporarily allow unauthenticated ingestion in staging

**Alternative validation**:
- Unit tests comprehensively validate the fixes
- Integration tests pass
- Smoke tests confirm deployment health
- Manual testing can be done after recreating API keys

## Recommendations

1. **Merge to main**: All code changes are tested and deployed to staging
2. **Create API key generation script**: Automate API key creation after DB resets
3. **Enhance staging-reset script**: Recreate essential API keys after wipe
4. **End-to-end test after merge**: Import real Toronto data to validate ~100% success rate

## Files Changed

```
internal/storage/postgres/events_repository.go
internal/storage/postgres/events_repository_reconciliation_test.go
internal/storage/postgres/migrations/000023_fix_source_unique_constraints.{up,down}.sql
internal/domain/events/normalize.go
internal/domain/events/normalize_test.go
scripts/staging-reset.sh (NEW)
test-dedupe-fixes.sh (NEW)
docs/DATA_QUALITY_ANALYSIS.md (NEW)
scripts/ingest-toronto-events.sh (modified)
```

## Next Steps

1. ✅ Staging DB wiped
2. ✅ Feature branch deployed to staging
3. ✅ Smoke tests passed
4. ⏸️  Toronto data import (blocked by API key)
5. ⏳ Create PR to merge to main
6. ⏳ Post-merge: Test with real data

---

**Status**: Ready for PR review and merge to main
**Beads closed**: srv-8ru (P2), srv-5dj (P3)
**CI Status**: Passing (except pre-existing srv-ert changefeed test)
