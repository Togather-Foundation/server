# PostgreSQL Storage Layer - Agent Guide

Quick reference for AI agents working with this storage layer. For complete documentation, see [README.md](./README.md).

## Critical Rules

### 1. SQLc for Static Queries, Raw SQL ONLY for Dynamic Structure

**Use SQLc (type-safe) for:**
- Standard CRUD operations
- Queries with optional filters (use `sqlc.narg()`)
- Any query where columns/structure are fixed

**Use raw SQL ONLY when:**
- Column selection changes at runtime (e.g., `ST_Distance()` only when needed)
- ORDER BY column/direction is parameterized
- WHERE clause structure fundamentally changes

**Example:** Places repository uses:
- ✅ SQLc for standard list queries (4 queries for sort/order combos)
- ✅ Raw SQL for proximity search (PostGIS distance calculation)

### 2. Nullable Parameter Bug (CRITICAL)

**WRONG:**
```sql
-- Generates Column1 bool (not nullable), defaults to false!
SELECT * FROM users WHERE ($1::boolean IS NULL OR is_active = $1);
```

**CORRECT:**
```sql
-- Generates IsActive pgtype.Bool (nullable with Valid flag)
SELECT * FROM users 
WHERE (sqlc.narg('is_active')::boolean IS NULL OR is_active = sqlc.narg('is_active'));
```

**Go usage:**
```go
params := ListUsersParams{}
if filter.IsActive != nil {
    params.IsActive = pgtype.Bool{Bool: *filter.IsActive, Valid: true}
}
// If not set, Valid=false → SQL receives NULL
```

**Warning signs you have this bug:**
- Generated param names: `Column1`, `Column2`
- Non-pointer scalars for optional filters: `bool`, `string`, `int`
- Empty results despite data existing

See [README.md lines 18-117](./README.md) for full explanation.

### 3. After Modifying `.sql` Files

**Always run:**
```bash
make sqlc
```

This regenerates `*.sql.go` and `querier.go`. Commit both query definitions and generated code together.

### 4. Dynamic SQL Safety

If you MUST use raw SQL:
- ✅ Use numbered placeholders: `$1`, `$2`, `$3`
- ✅ Build parameter arrays separately
- ❌ NEVER concatenate user input into SQL strings
- ✅ Track `$N` positions carefully when building WHERE clauses

**Example (from places_repository.go:84-115):**
```go
var whereClauses []string
var queryArgs []interface{}
argPos := 4  // First 3 are lon, lat, radius

if filters.City != "" {
    whereClauses append(whereClauses, fmt.Sprintf("p.address_locality ILIKE $%d", argPos))
    queryArgs = append(queryArgs, "%"+filters.City+"%")  // Parameterized!
    argPos++
}
```

### 5. Common Pitfalls

**Pitfall: Forgot to handle nullable fields**
```go
// WRONG: Assumes Valid=true
description := row.Description.String  // Panics if NULL!

// CORRECT: Check Valid flag
description := ""
if row.Description.Valid {
    description = row.Description.String
}
```

**Pitfall: Type switch covers all variants**
When using multiple SQLc queries (e.g., 4 sort/order combos), converter functions need a type switch:
```go
switch r := row.(type) {
case *ListPlacesByCreatedAtRow:
    // extract fields
case *ListPlacesByCreatedAtDescRow:
    // extract fields (same fields, different type)
// ... etc
}
```

See `places_repository.go:396` and `organizations_repository.go:383` for examples.

## Quick Commands

```bash
# Regenerate SQLc code
make sqlc

# Run storage tests
go test ./internal/storage/postgres -v

# Run with race detector
go test ./internal/storage/postgres -race

# Create migration
migrate create -ext sql -dir internal/storage/postgres/migrations -seq migration_name

# Run migrations
make migrate-up
```

## Reference Files

- **[README.md](./README.md)** - Complete SQLc patterns and troubleshooting
- **[queries/*.sql](./queries/)** - SQLc query definitions
- **[migrations/](./migrations/)** - Database schema migrations
- **[*.sql.go](./*.sql.go)** - Generated code (DO NOT EDIT)

## Recent Patterns (2026-02-18)

**Multi-query approach for sorting:** Instead of dynamic ORDER BY, create separate named queries:
- `ListPlacesByCreatedAt` (ASC)
- `ListPlacesByCreatedAtDesc` (DESC)
- `ListPlacesByName` (ASC)
- `ListPlacesByNameDesc` (DESC)

Repository selects appropriate query based on `filters.Sort` and `filters.Order`.

**Benefits:** Type-safe, prepared statements, explicit.  
**Trade-off:** More queries, but clearer than string building.
