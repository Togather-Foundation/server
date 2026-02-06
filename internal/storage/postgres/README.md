# PostgreSQL Storage Layer

This directory contains the PostgreSQL storage implementation using [SQLc](https://sqlc.dev/) for type-safe SQL queries.

## Directory Structure

```
internal/storage/postgres/
├── queries/          # SQLc query definitions (*.sql)
├── migrations/       # Database migrations (golang-migrate)
├── *.sql.go         # SQLc-generated Go code (DO NOT EDIT)
├── models.go        # SQLc-generated model types
└── README.md        # This file
```

## Writing SQLc Queries

### ⚠️ CRITICAL: Nullable Parameters

**Problem:** SQLc cannot automatically infer nullable parameters from standard `$N` placeholders. Using `$1::type IS NULL` checks with numbered parameters generates non-nullable Go types (e.g., `bool`, `string`), causing runtime bugs when optional filters aren't provided.

**Example Bug:**
```sql
-- WRONG: Generates 'Column1 bool' parameter (not nullable)
SELECT * FROM users
WHERE ($1::boolean IS NULL OR is_active = $1);
```

When the Go code doesn't set `params.Column1`, it defaults to `false` (not `NULL`), breaking the SQL logic.

**Correct Pattern:**
```sql
-- CORRECT: Generates 'IsActive pgtype.Bool' parameter (nullable)
SELECT * FROM users
WHERE (sqlc.narg('is_active')::boolean IS NULL OR is_active = sqlc.narg('is_active'));
```

### SQLc Parameter Functions

- **`sqlc.arg('name')`** - Required parameter (generates non-nullable Go type)
- **`sqlc.narg('name')`** - Nullable parameter (generates `pgtype.Type` with Valid flag)

### Go Code Usage

```go
// Service layer
func (s *Service) ListUsers(filters Filters) ([]User, error) {
    params := postgres.ListUsersParams{}
    
    // Only set nullable parameter when filter is provided
    if filters.IsActive != nil {
        params.IsActive = pgtype.Bool{
            Bool:  *filters.IsActive,
            Valid: true,  // Marks value as non-NULL
        }
    }
    // If not set, Valid=false and SQL receives NULL
    
    return s.queries.ListUsers(ctx, params)
}
```

### Common Nullable Types

| SQL Type | pgtype Type |
|----------|-------------|
| `boolean` | `pgtype.Bool` |
| `text` / `varchar` | `pgtype.Text` |
| `integer` / `bigint` | `pgtype.Int4` / `pgtype.Int8` |
| `timestamp` | `pgtype.Timestamptz` |
| `uuid` | `pgtype.UUID` |

### Warning Signs

If you see these in generated code, you likely have a nullable parameter bug:

- Parameter names like `Column1`, `Column2` (instead of descriptive names)
- Non-pointer scalar types (`bool`, `string`, `int`) for optional filters
- SQL with `IS NULL` checks but Go struct has non-nullable fields

### Real-World Example

**Bug:** Users list page returning empty results despite users existing in database.

**Before (Broken):**
```sql
-- queries/auth.sql
-- name: ListUsersWithFilters :many
SELECT * FROM users
WHERE ($1::boolean IS NULL OR is_active = $1)
  AND ($2::text IS NULL OR role = $2);
```

Generated:
```go
type ListUsersWithFiltersParams struct {
    Column1 bool   // ❌ Not nullable, defaults to false
    Column2 string // ❌ Not nullable, defaults to ""
}
```

**After (Fixed):**
```sql
-- queries/auth.sql
-- name: ListUsersWithFilters :many
SELECT * FROM users
WHERE (sqlc.narg('is_active')::boolean IS NULL OR is_active = sqlc.narg('is_active'))
  AND (sqlc.narg('role')::text IS NULL OR role = sqlc.narg('role'));
```

Generated:
```go
type ListUsersWithFiltersParams struct {
    IsActive pgtype.Bool // ✅ Nullable with Valid flag
    Role     pgtype.Text // ✅ Nullable with Valid flag
}
```

## Regenerating SQLc Code

After modifying `.sql` files in `queries/`, regenerate Go code:

```bash
make sqlc
# or
make generate
```

## Migrations

Database migrations are managed by [golang-migrate](https://github.com/golang-migrate/migrate).

**Create migration:**
```bash
migrate create -ext sql -dir internal/storage/postgres/migrations -seq migration_name
```

**Run migrations:**
```bash
make migrate-up
```

## Testing

- **Unit tests:** Mock the `Queries` interface
- **Integration tests:** Use real PostgreSQL with test database
- **Transaction tests:** Verify rollback behavior

## References

- [SQLc Documentation](https://docs.sqlc.dev/)
- [SQLc Query Annotations](https://docs.sqlc.dev/en/stable/reference/query-annotations.html)
- [pgtype Package](https://pkg.go.dev/github.com/jackc/pgx/v5/pgtype)
- [golang-migrate](https://github.com/golang-migrate/migrate)

## Troubleshooting

### Query returns empty results despite data existing

Check if the query uses nullable parameters correctly. Search for `Column1`, `Column2` in generated `*.sql.go` files - this indicates missing `sqlc.narg()`.

### "cannot use X (type T) as type pgtype.T"

You're likely passing a raw value to a nullable parameter. Wrap it:
```go
params.Field = pgtype.Text{String: value, Valid: true}
```

### SQLc generation fails

1. Check SQL syntax in `.sql` files
2. Verify `sqlc.yaml` configuration
3. Run `sqlc verify` to validate queries
