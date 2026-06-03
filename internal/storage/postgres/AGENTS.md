# PostgreSQL Storage Layer

Quick reference. For complete docs: [README.md](./README.md).

## Critical Rules

### 1. SQLc vs Raw SQL

Use **SQLc** for all standard queries. Use **raw SQL only** when column selection, ORDER BY, or WHERE structure changes at runtime.

### 2. Nullable Parameters (CRITICAL — common bug)

```sql
-- WRONG: generates Column1 bool, defaults false
SELECT * FROM users WHERE ($1::boolean IS NULL OR is_active = $1);

-- CORRECT: generates pgtype.Bool with Valid flag
SELECT * FROM users
WHERE (sqlc.narg('is_active')::boolean IS NULL OR is_active = sqlc.narg('is_active'));
```

```go
params.IsActive = pgtype.Bool{Bool: *filter.IsActive, Valid: true}
// If not set: Valid=false → SQL receives NULL
```

**Warning signs:** `Column1`/`Column2` param names; non-pointer scalars for optional filters.

### 3. After modifying `.sql` files

```bash
make sqlc   # regenerates *.sql.go and querier.go — commit both
```

### 4. Dynamic SQL (raw SQL safety)

- Use `$1`, `$2`, `$3` placeholders — never concatenate user input
- Track `$N` position when building WHERE clauses

### 5. Nullable field access

```go
// WRONG: panics if NULL
description := row.Description.String

// CORRECT
if row.Description.Valid {
    description = row.Description.String
}
```

### 6. Multi-query sort pattern → use `sqlc.embed()` + `toDomain()`

When multiple queries return the same column set (e.g. sort variants, filter variants), use `sqlc.embed()` to share a single model struct across all query row types, then add a `toDomain()` method on that model:

```sql
-- name: ListPlacesByName :many
SELECT sqlc.embed(places) FROM places WHERE ... ORDER BY name ASC;

-- name: ListPlacesByCreatedAt :many
SELECT sqlc.embed(places) FROM places WHERE ... ORDER BY created_at DESC;
```

```go
// toDomain converts the SQLc table model to the domain type.
func (p *Place) toDomain() places.Place {
    return places.Place{ID: p.Ulid, Name: p.Name, ...}
}
```

All call sites become identical: `row.Place.toDomain()`. One method to maintain per model — never write per-query converters or type-switch functions.

For JOIN queries that return extra columns (e.g. `LEFT JOIN LATERAL`), use `sqlc.embed(alias)` for the shared model and list extra columns separately:
```sql
SELECT sqlc.embed(s), r.started_at, r.status
FROM scraper_sources s LEFT JOIN LATERAL (...) r ON true;
```

### 7. INET columns → `netip.Addr`

SQLc maps PostgreSQL `INET` columns to `netip.Addr` (not `string`). Parse before use:

```go
addr, err := netip.ParseAddr(ipString)
if err != nil {
    return fmt.Errorf("parse IP %q: %w", ipString, err)
}
// Pass addr directly to SQLc params.
// Convert back with: addr.String()
```

### 8. `pgtype.Interval` — hours must use Microseconds

`pgtype.Interval` has `Days` and `Microseconds` but **no `Hours` field**. Express hours
as microseconds:

```go
const microsPerHour = int64(3600 * 1_000_000)
interval := pgtype.Interval{Microseconds: 24 * microsPerHour, Valid: true} // 24 h
interval := pgtype.Interval{Days: 30, Valid: true}                         // 30 days
```

## Commands

```bash
make sqlc                                                                    # regenerate
go test ./internal/storage/postgres -v                                       # storage tests (needs live DB)
make migrate-up                                                              # run migrations
migrate create -ext sql -dir internal/storage/postgres/migrations -seq name  # new migration
```
