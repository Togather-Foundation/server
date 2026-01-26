package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/api/pagination"
	"github.com/Togather-Foundation/server/internal/domain/organizations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ organizations.Repository = (*OrganizationRepository)(nil)

type OrganizationRepository struct {
	pool *pgxpool.Pool
	tx   pgx.Tx
}

type organizationRow struct {
	ID        string
	ULID      string
	Name      string
	LegalName *string
	URL       *string
	CreatedAt pgtype.Timestamptz
	UpdatedAt pgtype.Timestamptz
}

func (r *OrganizationRepository) List(ctx context.Context, filters organizations.Filters, paginationArgs organizations.Pagination) (organizations.ListResult, error) {
	queryer := r.queryer()
	query := strings.TrimSpace(filters.Query)
	limit := paginationArgs.Limit
	if limit <= 0 {
		limit = 50
	}
	limitPlusOne := limit + 1

	var cursorTime *time.Time
	var cursorULID *string
	if strings.TrimSpace(paginationArgs.After) != "" {
		cursor, err := pagination.DecodeEventCursor(paginationArgs.After)
		if err != nil {
			return organizations.ListResult{}, err
		}
		value := cursor.Timestamp.UTC()
		cursorTime = &value
		ulid := strings.ToUpper(strings.TrimSpace(cursor.ULID))
		cursorULID = &ulid
	}

	rows, err := queryer.Query(ctx, `
SELECT o.id, o.ulid, o.name, o.legal_name, o.url, o.created_at, o.updated_at
  FROM organizations o
 WHERE ($1 = '' OR o.name ILIKE '%' || $1 || '%' OR o.legal_name ILIKE '%' || $1 || '%')
   AND (
     $2::timestamptz IS NULL OR
     o.created_at > $2::timestamptz OR
     (o.created_at = $2::timestamptz AND o.ulid > $3)
   )
 ORDER BY o.created_at ASC, o.ulid ASC
 LIMIT $4
`,
		query,
		cursorTime,
		cursorULID,
		limitPlusOne,
	)
	if err != nil {
		return organizations.ListResult{}, fmt.Errorf("list organizations: %w", err)
	}
	defer rows.Close()

	items := make([]organizations.Organization, 0, limitPlusOne)
	for rows.Next() {
		var row organizationRow
		if err := rows.Scan(
			&row.ID,
			&row.ULID,
			&row.Name,
			&row.LegalName,
			&row.URL,
			&row.CreatedAt,
			&row.UpdatedAt,
		); err != nil {
			return organizations.ListResult{}, fmt.Errorf("scan organizations: %w", err)
		}
		org := organizations.Organization{
			ID:        row.ID,
			ULID:      row.ULID,
			Name:      row.Name,
			LegalName: derefString(row.LegalName),
			URL:       derefString(row.URL),
			CreatedAt: time.Time{},
			UpdatedAt: time.Time{},
		}
		if row.CreatedAt.Valid {
			org.CreatedAt = row.CreatedAt.Time
		}
		if row.UpdatedAt.Valid {
			org.UpdatedAt = row.UpdatedAt.Time
		}
		items = append(items, org)
	}
	if err := rows.Err(); err != nil {
		return organizations.ListResult{}, fmt.Errorf("iterate organizations: %w", err)
	}

	result := organizations.ListResult{}
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		result.NextCursor = pagination.EncodeEventCursor(last.CreatedAt, last.ULID)
	}
	result.Organizations = items
	return result, nil
}

func (r *OrganizationRepository) GetByULID(ctx context.Context, ulid string) (*organizations.Organization, error) {
	queryer := r.queryer()

	row := queryer.QueryRow(ctx, `
SELECT o.id, o.ulid, o.name, o.legal_name, o.url, o.created_at, o.updated_at
  FROM organizations o
 WHERE o.ulid = $1
`, ulid)

	var data organizationRow
	if err := row.Scan(
		&data.ID,
		&data.ULID,
		&data.Name,
		&data.LegalName,
		&data.URL,
		&data.CreatedAt,
		&data.UpdatedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, organizations.ErrNotFound
		}
		return nil, fmt.Errorf("get organization: %w", err)
	}

	org := &organizations.Organization{
		ID:        data.ID,
		ULID:      data.ULID,
		Name:      data.Name,
		LegalName: derefString(data.LegalName),
		URL:       derefString(data.URL),
		CreatedAt: time.Time{},
		UpdatedAt: time.Time{},
	}
	if data.CreatedAt.Valid {
		org.CreatedAt = data.CreatedAt.Time
	}
	if data.UpdatedAt.Valid {
		org.UpdatedAt = data.UpdatedAt.Time
	}
	return org, nil
}

type organizationQueryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func (r *OrganizationRepository) queryer() organizationQueryer {
	if r.tx != nil {
		return r.tx
	}
	return r.pool
}
