package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/api/pagination"
	"github.com/Togather-Foundation/server/internal/domain/places"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var _ places.Repository = (*PlaceRepository)(nil)

type placeRow struct {
	ID          string
	ULID        string
	Name        string
	Description *string
	City        *string
	Region      *string
	Country     *string
	CreatedAt   pgtype.Timestamptz
	UpdatedAt   pgtype.Timestamptz
}

func (r *PlaceRepository) List(ctx context.Context, filters places.Filters, paginationArgs places.Pagination) (places.ListResult, error) {
	queryer := r.queryer()

	var cursorTimestamp *time.Time
	var cursorULID *string
	if strings.TrimSpace(paginationArgs.After) != "" {
		cursor, err := pagination.DecodeEventCursor(paginationArgs.After)
		if err != nil {
			return places.ListResult{}, err
		}
		value := cursor.Timestamp.UTC()
		cursorTimestamp = &value
		ulid := strings.ToUpper(cursor.ULID)
		cursorULID = &ulid
	}

	limit := paginationArgs.Limit
	if limit <= 0 {
		limit = 50
	}
	limitPlusOne := limit + 1

	rows, err := queryer.Query(ctx, `
SELECT p.id, p.ulid, p.name, p.description, p.address_locality, p.address_region, p.address_country,
       p.created_at, p.updated_at
  FROM places p
 WHERE ($1 = '' OR p.address_locality ILIKE '%' || $1 || '%')
   AND ($2 = '' OR p.name ILIKE '%' || $2 || '%' OR p.description ILIKE '%' || $2 || '%')
   AND (
     $3::timestamptz IS NULL OR
     p.created_at > $3::timestamptz OR
     (p.created_at = $3::timestamptz AND p.ulid > $4)
   )
 ORDER BY p.created_at ASC, p.ulid ASC
 LIMIT $5
`,
		filters.City,
		filters.Query,
		cursorTimestamp,
		cursorULID,
		limitPlusOne,
	)
	if err != nil {
		return places.ListResult{}, fmt.Errorf("list places: %w", err)
	}
	defer rows.Close()

	items := make([]places.Place, 0, limitPlusOne)
	for rows.Next() {
		var row placeRow
		if err := rows.Scan(
			&row.ID,
			&row.ULID,
			&row.Name,
			&row.Description,
			&row.City,
			&row.Region,
			&row.Country,
			&row.CreatedAt,
			&row.UpdatedAt,
		); err != nil {
			return places.ListResult{}, fmt.Errorf("scan places: %w", err)
		}
		place := places.Place{
			ID:          row.ID,
			ULID:        row.ULID,
			Name:        row.Name,
			Description: derefString(row.Description),
			City:        derefString(row.City),
			Region:      derefString(row.Region),
			Country:     derefString(row.Country),
			CreatedAt:   time.Time{},
			UpdatedAt:   time.Time{},
		}
		if row.CreatedAt.Valid {
			place.CreatedAt = row.CreatedAt.Time
		}
		if row.UpdatedAt.Valid {
			place.UpdatedAt = row.UpdatedAt.Time
		}
		items = append(items, place)
	}
	if err := rows.Err(); err != nil {
		return places.ListResult{}, fmt.Errorf("iterate places: %w", err)
	}

	result := places.ListResult{}
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		if !last.CreatedAt.IsZero() {
			result.NextCursor = pagination.EncodeEventCursor(last.CreatedAt, last.ULID)
		}
	}
	result.Places = items
	return result, nil
}

func (r *PlaceRepository) GetByULID(ctx context.Context, ulid string) (*places.Place, error) {
	queryer := r.queryer()

	row := queryer.QueryRow(ctx, `
SELECT p.id, p.ulid, p.name, p.description, p.address_locality, p.address_region, p.address_country,
       p.created_at, p.updated_at
  FROM places p
 WHERE p.ulid = $1
`, ulid)

	var data placeRow
	if err := row.Scan(
		&data.ID,
		&data.ULID,
		&data.Name,
		&data.Description,
		&data.City,
		&data.Region,
		&data.Country,
		&data.CreatedAt,
		&data.UpdatedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, places.ErrNotFound
		}
		return nil, fmt.Errorf("get place: %w", err)
	}

	place := &places.Place{
		ID:          data.ID,
		ULID:        data.ULID,
		Name:        data.Name,
		Description: derefString(data.Description),
		City:        derefString(data.City),
		Region:      derefString(data.Region),
		Country:     derefString(data.Country),
		CreatedAt:   time.Time{},
		UpdatedAt:   time.Time{},
	}
	if data.CreatedAt.Valid {
		place.CreatedAt = data.CreatedAt.Time
	}
	if data.UpdatedAt.Valid {
		place.UpdatedAt = data.UpdatedAt.Time
	}
	return place, nil
}

type placeQueryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func (r *PlaceRepository) queryer() placeQueryer {
	if r.tx != nil {
		return r.tx
	}
	return r.pool
}
