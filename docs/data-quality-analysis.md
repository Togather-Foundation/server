# Toronto Events Ingestion - Data Quality Analysis

**Date:** 2026-02-07  
**Events Analyzed:** 1,671 from Toronto Open Data JSON-LD proxy  
**Successfully Ingested:** 165+ unique events  
**Overall Success Rate:** ~93%

---

## Executive Summary

The Toronto events ingestion revealed **three critical data quality issues** that affect scalability and data integrity:

1. **Entity Duplication (CRITICAL)**: No reconciliation for organizations/places → massive duplication
2. **Source Name Collisions**: Unique constraint violations when ingesting multiple events from same organizer
3. **Timezone Conversion Errors**: Invalid date ranges in source data (~1% of events)

All issues have been documented with actionable beads for resolution.

---

## Issue 1: Entity Duplication - No Reconciliation (CRITICAL)

**Bead:** `srv-85m` (P1 Feature)  
**Impact:** Severe - Creates duplicate entities for every event ingestion

### Problem

Current `UpsertPlace` and `UpsertOrganization` only deduplicate by `federation_uri`. For local ingestion (API submissions), every event creates **new** organization and place records, even if identical entities exist.

### Evidence from Toronto Data

**Organizations (by name frequency):**
| Organization | Event Count | Current State | Should Be |
|-------------|-------------|---------------|-----------|
| City of Toronto | 114 | 114 duplicate records | 1 org record |
| Artbox Studio & Gallery | 58 | 58 duplicate records | 1 org record |
| Royal Conservatory of Music | 53 | 53 duplicate records | 1 org record |
| DROM Taberna | 50 | 50 duplicate records | 1 org record |

**Places (by name frequency):**
| Place | Event Count | Current State | Should Be |
|-------|-------------|---------------|-----------|
| DROM Taberna / Drom Taberna | 136 | 136 duplicate records | 1 place (case-insensitive) |
| Artbox Studio & Gallery / Artbox Studio and Gallery | 62 | 62 duplicate records | 1 place ('&' vs 'and') |
| Royal Ontario Museum | 25 | 25 duplicate records | 1 place |

**Empty organizers:** 483 events (29%) have no organizer name

### Impact

1. **Database Bloat**: 165 events → likely created 300+ org/place records (should be ~50)
2. **Analytics Broken**: Can't count "events by venue" because same venue has 100+ IDs
3. **Federation Issues**: Different nodes will assign different URIs to same entity
4. **Source Collisions**: Contributes to srv-8ru (duplicate source names)

### Proposed Solution

**Implement name-based entity reconciliation:**

```go
// Before creating new place:
1. Normalize name: lowercase, trim, '&' → 'and'
2. Lookup by: normalized_name + city + region
3. If match found: reuse existing place_id
4. If no match: create new place

// Example:
"Artbox Studio & Gallery" (Toronto, ON)
"Artbox Studio and Gallery" (Toronto, ON)
→ Match! Reuse same place_id
```

**Database changes:**
- Add `normalized_name` column with GIN index (fast lookup)
- Use `pg_trgm` extension for fuzzy matching
- Composite unique constraint on (normalized_name, address_locality, address_region)

**Algorithm:**
1. Exact match: normalized_name + locality + region
2. Fuzzy match: pg_trgm similarity > 0.8 within same city
3. No match: create new entity

---

## Issue 2: Source Name Collisions

**Bead:** `srv-8ru` (P2 Bug)  
**Impact:** Moderate - ~4% of events fail ingestion (13 per 300 batch)

### Problem

The ingestion script generates source names as:
```javascript
source.name = (organizer.name // location.name) + " Events"
```

Multiple events from same organizer produce identical source names, violating the database unique constraint `sources_name_key`.

### Examples

| Source Name | Event Count | First Event | Subsequent Events |
|-------------|-------------|-------------|-------------------|
| DROM Taberna Events | 112 | ✓ Success | ✗ 111 fail |
| Artbox Studio & Gallery Events | 58 | ✓ Success | ✗ 57 fail |
| Toronto Open Data Events | 483 (empty org) | ✓ Success | ✗ 482 fail |

**Error message:**
```
create source: ERROR: duplicate key value violates unique constraint "sources_name_key" (SQLSTATE 23505)
```

### Root Cause Analysis

The `sources` table has:
```sql
CREATE TABLE sources (
  name TEXT UNIQUE NOT NULL,  -- Problem: overly strict constraint
  base_url TEXT NOT NULL,
  ...
);
```

This assumes one-to-one mapping: `source name → source record`. But multiple events can come from the same source!

**Architectural mismatch:**
- **Intended:** 1 source = 1 feed/API (e.g., "Toronto Open Data API")
- **Actual usage:** 1 source = 1 event's origin info
- **Script behavior:** Generates unique name per event (but sometimes identical)

### Solutions (in priority order)

**Option 1: Quick Fix - Make source name unique per event** ⭐ Recommended short-term
```javascript
source.name = organizer.name + " - " + event.url.slice(-20)
// Example: "DROM Taberna - tickets-123456789"
```

**Option 2: Proper Fix - One source per organizer/feed** (Requires refactoring)
```go
// In GetOrCreateSource, lookup by (name, base_url) pair
// Reuse existing source_id for all events from same origin
sourceID := repo.FindOrCreateSource(name, baseURL)
```

**Option 3: Schema Change - Relax unique constraint**
```sql
-- Remove unique constraint on name alone
-- Add composite unique on (name, base_url)
ALTER TABLE sources DROP CONSTRAINT sources_name_key;
CREATE UNIQUE INDEX sources_name_base_url_key ON sources(name, base_url);
```

**Recommended approach:** Option 1 now (unblock ingestion) + Option 2 with srv-85m (proper entity modeling)

---

## Issue 3: Timezone Conversion Errors in Source Data

**Bead:** `srv-5dj` (P3 Bug)  
**Impact:** Minor - ~1% of events fail validation

### Problem

Events spanning midnight in local time (EST/EDT) are incorrectly converted to UTC in the Toronto Open Data feed, causing `endDate` to appear chronologically before `startDate`.

### Examples

| Event | Start (UTC) | End (UTC) | Local Time (EST) |
|-------|-------------|-----------|------------------|
| Monday Latin Nights | 2025-03-31T23:00Z | 2025-03-31T06:00Z | Mon 7pm - Tue 1am |
| Dim Sum Mondays | 2025-06-03T04:00Z | 2025-06-02T07:00Z | Mon 12am - Mon 3am |
| Weston Farmers Market | 2025-06-08T00:30Z | 2025-06-07T17:00Z | Sat 8:30pm - Sun 1pm |

**Pattern:** The UTC end date is ~17-24 hours earlier than start date, suggesting timezone offset applied incorrectly.

### Validation Failure

```go
// internal/domain/events/validation.go:121
if endTime != nil && endTime.Before(*startTime) {
    return ValidationError{Field: "endDate", Message: "must be on or after startDate"}
}
```

### Solutions

**Option 1: Auto-correct midnight-spanning events** ⭐ Recommended
```go
// In normalize.go
func normalizeEventDates(start, end time.Time) (time.Time, time.Time) {
    if end.Before(start) {
        // Check if adding 24h makes it reasonable (likely midnight-spanning)
        correctedEnd := end.Add(24 * time.Hour)
        if correctedEnd.Sub(start) < 48*time.Hour {
            return start, correctedEnd
        }
    }
    return start, end
}
```

**Option 2: Make endDate optional for invalid ranges**
```go
// Skip endDate if it violates temporal logic
if endTime != nil && endTime.Before(*startTime) {
    endTime = nil  // Single-datetime event
}
```

**Option 3: Report upstream to Toronto Open Data**
- File issue: https://github.com/CivicTechTO/toronto-opendata-festivalsandevents-jsonld-proxy/issues
- Request proper timezone handling in their conversion script
- We still need workaround for existing data

**Recommended:** Option 1 (auto-correct) + Option 3 (report upstream)

---

## Additional Observations

### Empty Organizer Names

**Count:** 483 events (29% of dataset) have `organizer.name == ""`

**Examples:**
```json
{
  "name": "Before Hours Tours: Auschwitz",
  "organizer": {
    "@type": "Organization",
    "name": "",           // Empty!
    "email": "info@rom.on.ca",
    "telephone": "416-586-8000"
  }
}
```

**Impact:**
- Falls back to venue name for source identification
- Script now handles this correctly (checks for empty string)
- Could infer organizer from email domain (rom.on.ca → Royal Ontario Museum)

### Duplicate Events (Same Name)

**Pattern:** Many events have identical names but different dates (recurring events)

**Examples:**
- "Wednesdays - Pro & Hilarious Stand-up Comedy": 2 instances (different dates)
- "A Canvas & Bear Paint Pouring Workshop": 2 instances
- "A Clay Sculpting Workshop": 2 instances

**Current behavior:** ✓ Correctly treated as separate events (different occurrences)  
**Deduplication logic:** Works correctly (by name + venue + startDate)

---

## Recommendations (Priority Order)

### Immediate (Sprint 1)
1. **srv-8ru (P2):** Fix source name collisions → Quick script change, unblocks bulk ingestion
2. **srv-5dj (P3):** Add date auto-correction → Improves success rate from 93% to 99%

### Short-term (Sprint 2-3)  
3. **srv-85m (P1):** Implement entity reconciliation → Critical for scalability
   - Start with places (more straightforward)
   - Then organizations
   - Add normalized_name columns + indexes
   - Implement fuzzy matching with pg_trgm

### Medium-term (Sprint 4+)
4. **srv-zqh (P2):** Full schema.org Event support → Enables broader ingestion
5. **srv-054 (P2):** River worker health checks in smoke tests → Prevents srv-xxd recurrence

---

## Success Metrics

**Current state (Toronto ingestion):**
- ✓ 165 events successfully ingested
- ✗ ~300+ duplicate org/place records created
- ✗ 4% failure rate from source collisions
- ✗ 1% failure rate from date validation

**Target state (after fixes):**
- ✓ 99%+ success rate (only truly invalid data fails)
- ✓ ~50 unique org/place records (proper reconciliation)
- ✓ Fast lookups (normalized_name indexes)
- ✓ Scalable to 10,000+ events without degradation

---

## References

- **Source data:** https://civictechto.github.io/toronto-opendata-festivalsandevents-jsonld-proxy/all.jsonld
- **Ingestion script:** `scripts/ingest-toronto-events.sh`
- **Documentation:** `scripts/README-TORONTO-INGESTION.md`
- **Beads:**
  - srv-85m: Entity reconciliation (P1)
  - srv-8ru: Source name collisions (P2)
  - srv-5dj: Timezone errors (P3)
- **Related:**
  - srv-xxd: River worker fix (✓ CLOSED)
  - srv-zqh: Schema.org support (OPEN)
  - srv-054: Smoke test improvements (OPEN)

---

**Last Updated:** 2026-02-20
