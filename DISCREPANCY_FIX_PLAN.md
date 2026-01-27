# Documentation-Code Discrepancy Fix Plan

**Date**: 2026-01-26  
**Status**: In Progress  
**Beads**: 23 open issues (9 P1, 10 P2, 4 P3)

---

## Executive Summary

This plan addresses all discrepancies between documentation (Interoperability Profile, Federation Architecture) and implementation discovered during comprehensive code review. The approach prioritizes federation-critical issues (P1), followed by API consistency (P2), with optional enhancements deferred (P3).

**Key Principle**: Fix code where docs represent the intended design (authoritative spec), update docs where code is correct but undocumented.

---

## Priority Classification

| Priority | Count | Description | Impact |
|----------|-------|-------------|--------|
| P1 | 9 | Federation compliance, interoperability breaking | Critical |
| P2 | 10 | API consistency, documentation alignment | Medium |
| P3 | 4 | Security enhancements, operational improvements | Low |

---

## Phase 1: Change Feed & Federation Core (P1)

### Group A: Change Feed Response Structure
**Beads**: `server-ix2`, `server-5t7`  
**Status**: Critical - Breaking change for federation partners

**Problem**: 
- Docs specify `{cursor, changes, next_cursor}` (Interop Profile §4.3, Federation Architecture §4.3)
- Code returns `{items, next_cursor}` (handlers/feeds.go:87-92)

**Solution**: Update code to match Interop Profile

**Files to Modify**:
1. `internal/domain/federation/changefeed.go`
   - Rename `Items` → `Changes` in `ChangeFeedResult` struct (line 97)
   - Add `Cursor string` field to struct (current position, not just next)
   - Update service to populate all three fields

2. `internal/api/handlers/feeds.go`
   - Update response mapping (lines 87-92):
   ```go
   response := map[string]any{
       "cursor":      result.Cursor,      // ADD: current cursor
       "changes":     result.Changes,     // RENAME: was "items"
       "next_cursor": result.NextCursor,
   }
   ```

3. **Integration Tests** (all `tests/integration/feeds_*.go`)
   - Update assertions to expect `changes` instead of `items`
   - Verify `cursor` field is present and correct

**Test Coverage**:
- Verify empty feed returns cursor="seq_0"
- Verify pagination preserves cursor sequence
- Verify change feed contract matches Interop Profile §4.3

---

### Group B: Canonical URI Fixes
**Beads**: `server-lae`, `server-8fb`  
**Status**: Critical - Semantic web compliance

**Problem 1** (`server-lae`):
- Change feed snapshots use API path: `{baseURL}/api/v1/events/{ulid}`
- Docs require canonical URI: `https://{domain}/events/{ulid}` (Interop Profile §1.1)

**Solution**: Use `ids.BuildCanonicalURI()` in snapshot transformation

**File**: `internal/domain/federation/changefeed.go:297`
```go
// Current (WRONG):
jsonLD["@id"] = fmt.Sprintf("%s/api/v1/events/%s", baseURL, ulid)

// Fixed:
if uri, err := ids.BuildCanonicalURI(baseURL, "events", ulid); err == nil {
    jsonLD["@id"] = uri
} else {
    // Fallback to API path if canonical URI fails
    jsonLD["@id"] = fmt.Sprintf("%s/api/v1/events/%s", baseURL, ulid)
}
```

**Problem 2** (`server-8fb`):
- Docs claim context URL: `https://schema.togather.foundation/context/v1.jsonld`
- Code uses: `https://togather.foundation/contexts/sel/v0.1.jsonld`
- Local file: `contexts/sel/v0.1.jsonld`

**Solution Options**:
1. **Option A** (Recommended): Update docs to match actual URL
2. **Option B**: Add HTTP redirect from schema.togather.foundation → togather.foundation
3. **Option C**: Serve context at both URLs

**Decision**: Option A - Update documentation

**File**: `docs/togather_SEL_Interoperability_Profile_v0.1.md:224-226`
- Change documented URL to actual: `https://togather.foundation/contexts/sel/v0.1.jsonld`
- Update examples throughout document

---

### Group C: Query Parameters Alignment
**Beads**: `server-6ww`, `server-307`  
**Status**: Critical - API contract consistency

**Problem**:
- Docs use `since` for cursor parameter (Interop Profile §4.3, Federation Architecture §4.3)
- Code expects `after` for cursor, `since` for timestamp
- Confusion between cursor-based and timestamp-based pagination

**Solution**: Accept both `since` and `after` for cursor, maintain backward compatibility

**File**: `internal/api/handlers/feeds.go:36-65`
```go
// Parse query parameters
query := r.URL.Query()

// Support both 'after' and 'since' for cursor (prefer 'since' per docs)
cursor := query.Get("since")
if cursor == "" {
    cursor = query.Get("after") // Fallback for backward compatibility
}

// Build service params
params := federation.ChangeFeedParams{
    After:           cursor,
    Action:          query.Get("action"),
    IncludeSnapshot: query.Get("include_snapshot") == "true",
}

// Parse limit (unchanged)
if limitStr := query.Get("limit"); limitStr != "" {
    limit, err := strconv.Atoi(limitStr)
    if err != nil || limit < 1 || limit > 1000 {
        problem.Write(w, r, http.StatusBadRequest, ...)
        return
    }
    params.Limit = limit
} else {
    params.Limit = federation.DefaultChangeFeedLimit
}

// NOTE: Remove timestamp parsing for 'since' - cursors only
// Timestamp-based queries are out of scope for MVP
```

**Documentation Update**:
**File**: `docs/FEDERATION_ARCHITECTURE.md:217-223`
- Clarify that `since` parameter accepts cursor (not timestamp)
- Document `after` as deprecated alias for `since`
- Remove timestamp-based filtering examples (deferred to post-MVP)

---

### Group D: Missing Required Fields in API Responses
**Beads**: `server-685`, `server-28q`  
**Status**: Critical - Interoperability Profile compliance

**Problem**: 
- Event list returns only `{@context, @type, name}` (handlers/events.go:60-64)
- Docs require minimum: `name, startDate, location` (Interop Profile §3.1)
- Place/Org endpoints similarly minimal

**Solution**: Enrich API responses to include required fields

**File 1**: `internal/api/handlers/events.go:57-65`
```go
// Current (WRONG):
items := make([]map[string]any, 0, len(result.Events))
for _, event := range result.Events {
    items = append(items, map[string]any{
        "@context": contextValue,
        "@type":    "Event",
        "name":     event.Name,
    })
}

// Fixed:
items := make([]map[string]any, 0, len(result.Events))
for _, event := range result.Events {
    item := map[string]any{
        "@context": contextValue,
        "@type":    "Event",
        "@id":      buildEventURI(h.BaseURL, event.ULID),
        "name":     event.Name,
    }
    
    // Add required fields per Interop Profile §3.1
    if !event.StartDate.IsZero() {
        item["startDate"] = event.StartDate.Format(time.RFC3339)
    }
    
    // Add location (required)
    if location := buildLocationPayload(event); location != nil {
        item["location"] = location
    }
    
    items = append(items, item)
}
```

**Helper Function** (add to events.go):
```go
// buildLocationPayload constructs location object for list responses
func buildLocationPayload(event events.Event) map[string]any {
    if event.VenueID != nil {
        // Physical location
        return map[string]any{
            "@type": "Place",
            "@id":   fmt.Sprintf("%s/places/%s", baseURL, *event.VenueID),
            "name":  event.VenueName, // If available in result
        }
    }
    if event.VirtualURL != nil && *event.VirtualURL != "" {
        // Virtual location
        return map[string]any{
            "@type": "VirtualLocation",
            "url":   *event.VirtualURL,
        }
    }
    return nil
}
```

**File 2**: `internal/api/handlers/places.go` (similar enrichment)
```go
// Add required fields: @id, address OR geo
item := map[string]any{
    "@context": contextValue,
    "@type":    "Place",
    "@id":      buildPlaceURI(h.BaseURL, place.ULID),
    "name":     place.Name,
}

// Add address if available
if place.Address != nil {
    item["address"] = buildAddressPayload(place.Address)
}

// Add geo if available
if place.Latitude != nil && place.Longitude != nil {
    item["geo"] = map[string]any{
        "@type":     "GeoCoordinates",
        "latitude":  *place.Latitude,
        "longitude": *place.Longitude,
    }
}
```

**File 3**: `internal/api/handlers/organizations.go` (similar enrichment)
```go
// Add url if available per Interop Profile
item := map[string]any{
    "@context": contextValue,
    "@type":    "Organization",
    "@id":      buildOrgURI(h.BaseURL, org.ULID),
    "name":     org.Name,
}

if org.URL != nil && *org.URL != "" {
    item["url"] = *org.URL
}
```

**Database Query Updates**:
- Ensure `events.List()` query includes `start_date`, `venue_id`, `virtual_url`
- Ensure `places.List()` includes `address`, `latitude`, `longitude`
- Ensure `organizations.List()` includes `url`

---

### Group E: Well-Known Discovery Endpoint
**Beads**: `server-bvc`  
**Status**: Critical - Federation discovery requirement

**Problem**: Interop Profile §1.7 requires `/.well-known/sel-profile` but not implemented

**Solution**: Add well-known handler and route

**New File**: `internal/api/handlers/wellknown.go`
```go
package handlers

import (
    "net/http"
    "time"
)

// WellKnownHandler handles .well-known endpoints for discovery.
type WellKnownHandler struct {
    BaseURL string
    Version string
}

// NewWellKnownHandler creates a new well-known handler.
func NewWellKnownHandler(baseURL, version string) *WellKnownHandler {
    return &WellKnownHandler{
        BaseURL: baseURL,
        Version: version,
    }
}

// SELProfile handles GET /.well-known/sel-profile
// Returns profile information per Interop Profile §1.7
func (h *WellKnownHandler) SELProfile(w http.ResponseWriter, r *http.Request) {
    profile := map[string]string{
        "profile": "https://sel.events/profiles/interop",
        "version": h.Version,
        "node":    h.BaseURL,
        "updated": time.Now().Format("2006-01-02"),
    }
    
    writeJSON(w, http.StatusOK, profile, "application/json")
}
```

**File**: `internal/api/router.go`
```go
// Add to router setup (before /api/v1 routes):
wellKnownHandler := handlers.NewWellKnownHandler(baseURL, "0.1.0")
mux.HandleFunc("GET /.well-known/sel-profile", wellKnownHandler.SELProfile)
```

**New Test File**: `tests/integration/wellknown_test.go`
```go
package integration

import (
    "testing"
    "net/http"
    "encoding/json"
)

func TestWellKnownSELProfile(t *testing.T) {
    // Setup test server
    ts := setupTestServer(t)
    defer ts.Cleanup()
    
    // Request well-known profile
    resp := ts.Get("/.well-known/sel-profile")
    defer resp.Body.Close()
    
    // Assert 200 OK
    if resp.StatusCode != http.StatusOK {
        t.Fatalf("Expected 200 OK, got %d", resp.StatusCode)
    }
    
    // Parse response
    var profile map[string]string
    if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
        t.Fatalf("Failed to decode response: %v", err)
    }
    
    // Assert required fields
    if profile["profile"] != "https://sel.events/profiles/interop" {
        t.Errorf("Expected profile URL, got %q", profile["profile"])
    }
    if profile["version"] == "" {
        t.Error("Expected version field")
    }
    if profile["node"] == "" {
        t.Error("Expected node field")
    }
    if profile["updated"] == "" {
        t.Error("Expected updated field")
    }
}
```

---

### Group F: OpenAPI Path Corrections
**Beads**: `server-r84`, `server-abz`  
**Status**: Critical - Documentation accuracy

**Problem**: OpenAPI schema documents incorrect paths

**Solution**: Update OpenAPI schema (documentation-only fix, no code changes)

**Changes Needed**:
1. Update `/auth/login` → `/api/v1/admin/login` in OpenAPI schema
2. Update `/openapi.json` → `/api/v1/openapi.json` in OpenAPI schema
3. Verify all documented paths match router.go routes

**Files**: OpenAPI schema files (exact location TBD - likely `docs/` or `api/`)

---

## Phase 2: API Consistency & Documentation (P2)

### Group G: OpenAPI Response Schema Alignment
**Beads**: `server-82c`, `server-bys`, `server-eha`

#### server-82c: @context Array vs Object
**Problem**: OpenAPI expects array, code returns object or nil

**Solution**: Update code to return array per JSON-LD best practices

**File**: `internal/api/handlers/events.go` (and similar handlers)
```go
// loadDefaultContext should return array, not object
func loadDefaultContext() []any {
    return []any{
        "https://schema.org",
        "https://togather.foundation/contexts/sel/v0.1.jsonld",
    }
}
```

#### server-bys: Duplicates Endpoints Not Implemented
**Problem**: OpenAPI documents `/admin/duplicates` but not implemented

**Solution**: Remove from OpenAPI, document actual merge endpoint

#### server-eha: Change Feed Limit Max 1000 vs Documented 200
**Problem**: Code allows max 1000, docs say 200

**Solution**: Update code to enforce max 200

**File**: `internal/api/handlers/feeds.go:48`
```go
// Change validation
if err != nil || limit < 1 || limit > 200 { // Was 1000
    problem.Write(w, r, http.StatusBadRequest, ...)
    return
}
```

**File**: `internal/domain/federation/changefeed.go:118`
```go
// Update max limit check
if params.Limit > 200 { // Was 1000
    return nil, ErrInvalidLimit
}
```

---

### Group H: Documentation Enhancements
**Beads**: `server-dag`, `server-5kr`, `server-0h3`, `server-nn5`, `server-qdj`

**All documentation updates** (no code changes):

1. **server-dag**: Clarify content negotiation scope
   - Update docs: content negotiation only on dereferenceable URIs (`/events/{id}`)
   - Not on API endpoints (`/api/v1/events/{id}`)

2. **server-5kr**: Add OpenStreetMap to authority list
   - Update Interop Profile §1.4 to include OSM examples

3. **server-0h3**: Document cursor encoding
   - Add note: cursors are base64url encoded, treat as opaque

4. **server-nn5**: Update federation error types
   - Clarify that `validation-error` covers JSON-LD parsing failures

5. **server-qdj**: Document extra change feed fields
   - Add to Federation Architecture: license_url, timestamps, federation_uri fields

---

## Phase 3: Optional Enhancements (P3)

### Group I: Security & Operations
**Beads**: `server-o86`, `server-3m6`, `server-1od`

**Defer to separate work sessions**:
1. RFC7807 for CSRF errors - nice-to-have consistency
2. Timing attack protection - security review post-MVP
3. Monitoring for cleanup job - operational concern

---

## Testing Strategy

For each change:
1. **Unit tests**: Update existing or add new for modified functions
2. **Integration tests**: Verify end-to-end behavior matches docs
3. **Contract tests**: Validate JSON-LD output against SHACL shapes
4. **Regression tests**: Ensure existing functionality unaffected

### Test Execution
```bash
# Run all tests
make test

# Run integration tests only
go test ./tests/integration/...

# Run with race detector
make test-race

# Check coverage
make coverage
```

---

## Success Criteria

- [ ] All P1 beads closed with passing tests
- [ ] All P2 beads closed with passing tests
- [ ] Documentation updated and consistent with code
- [ ] `make test` passes (all tests green)
- [ ] `make lint` passes (no linter errors)
- [ ] OpenAPI schema validates
- [ ] Change feed contract matches Interop Profile §4.3
- [ ] Federation sync tested end-to-end
- [ ] Git commits pushed to remote
- [ ] Beads synced with git

---

## Estimated Timeline

| Phase | Scope | Estimate |
|-------|-------|----------|
| Phase 1 (P1) | 9 critical issues | 4-6 hours |
| Phase 2 (P2) | 10 consistency issues | 2-3 hours |
| Phase 3 (P3) | 4 optional enhancements | Deferred |

**Total for P1+P2**: 6-9 hours

---

## Rollout Plan

### Step 1: Code Changes
1. Implement all P1 fixes
2. Run tests, fix failures
3. Commit with descriptive messages

### Step 2: Documentation Updates
1. Update all affected docs
2. Verify consistency across all files
3. Commit separately from code

### Step 3: Testing & Validation
1. Run full test suite
2. Manual smoke test of API endpoints
3. Validate JSON-LD against SHACL shapes

### Step 4: Cleanup & Close
1. Close all related beads
2. Sync beads to git
3. Push all changes to remote
4. Verify CI passes (if applicable)

---

## Files Affected (Summary)

### Code Files
- `internal/domain/federation/changefeed.go`
- `internal/api/handlers/feeds.go`
- `internal/api/handlers/events.go`
- `internal/api/handlers/places.go`
- `internal/api/handlers/organizations.go`
- `internal/api/handlers/wellknown.go` (NEW)
- `internal/api/router.go`
- `tests/integration/wellknown_test.go` (NEW)
- `tests/integration/feeds_*.go` (multiple)

### Documentation Files
- `docs/togather_SEL_Interoperability_Profile_v0.1.md`
- `docs/FEDERATION_ARCHITECTURE.md`
- OpenAPI schema files (location TBD)

### Test Files
- All integration tests under `tests/integration/`

---

## Risk Assessment

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Breaking changes for existing consumers | Medium | High | Version API, provide migration guide |
| Test failures reveal deeper issues | Low | Medium | Fix root cause, may require design discussion |
| Performance impact from enriched responses | Low | Low | Monitor query performance, add indexes if needed |
| Documentation drift during implementation | Low | Low | Update docs alongside code in same commit |

---

## Next Steps

1. **Immediate**: Begin Phase 1 (P1 fixes)
2. **Session 1**: Groups A-C (change feed, URIs, parameters)
3. **Session 2**: Groups D-F (required fields, well-known, OpenAPI)
4. **Session 3**: Phase 2 (P2 fixes)
5. **Deferred**: Phase 3 (P3 enhancements)

---

## Notes

- This plan assumes PostgreSQL schema already supports required fields
- If schema changes needed, add migrations before code changes
- All changes maintain backward compatibility where possible
- Breaking changes are documented and justified by spec compliance
