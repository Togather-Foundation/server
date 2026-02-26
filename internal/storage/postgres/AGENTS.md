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

### 6. Multi-query sort pattern

When ORDER BY is parameterized, create separate named SQLc queries per sort/order combo (e.g. `ListPlacesByCreatedAt`, `ListPlacesByCreatedAtDesc`). Repository selects via type switch. See `places_repository.go:396`.

## Commands

```bash
make sqlc                                                                    # regenerate
go test ./internal/storage/postgres -v                                       # storage tests (needs live DB)
make migrate-up                                                              # run migrations
migrate create -ext sql -dir internal/storage/postgres/migrations -seq name  # new migration
```
