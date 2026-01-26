package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/api/pagination"
	"github.com/Togather-Foundation/server/internal/domain/places"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ places.Repository = (*PlaceRepository)(nil)

type PlaceRepository struct {
	pool *pgxpool.Pool
	tx   pgx.Tx
}

type placeRow struct {
	ID          string
	ULID        string
	Name        string
	Description *string
	City        *string
	Region      *string
	Country     *string
	DeletedAt   pgtype.Timestamptz
	Reason      pgtype.Text
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
       p.deleted_at, p.deletion_reason, p.created_at, p.updated_at
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
		&data.DeletedAt,
		&data.Reason,
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
		Lifecycle:   "",
		CreatedAt:   time.Time{},
		UpdatedAt:   time.Time{},
	}
	if data.CreatedAt.Valid {
		place.CreatedAt = data.CreatedAt.Time
	}
	if data.UpdatedAt.Valid {
		place.UpdatedAt = data.UpdatedAt.Time
	}
	if data.DeletedAt.Valid {
		place.Lifecycle = "deleted"
	}
	return place, nil
}

// SoftDelete marks a place as deleted
func (r *PlaceRepository) SoftDelete(ctx context.Context, ulid string, reason string) error {
	queries := Queries{db: r.queryer()}

	err := queries.SoftDeletePlace(ctx, SoftDeletePlaceParams{
		Ulid:           ulid,
		DeletionReason: pgtype.Text{String: reason, Valid: reason != ""},
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return places.ErrNotFound
		}
		return fmt.Errorf("soft delete place: %w", err)
	}

	return nil
}

// CreateTombstone creates a tombstone record for a deleted place
func (r *PlaceRepository) CreateTombstone(ctx context.Context, params places.TombstoneCreateParams) error {
	queries := Queries{db: r.queryer()}

	var placeIDUUID pgtype.UUID
	if err := placeIDUUID.Scan(params.PlaceID); err != nil {
		return fmt.Errorf("invalid place ID: %w", err)
	}

	var supersededBy pgtype.Text
	if params.SupersededBy != nil {
		supersededBy = pgtype.Text{String: *params.SupersededBy, Valid: true}
	}

	err := queries.CreatePlaceTombstone(ctx, CreatePlaceTombstoneParams{
		PlaceID:         placeIDUUID,
		PlaceUri:        params.PlaceURI,
		DeletedAt:       pgtype.Timestamptz{Time: params.DeletedAt, Valid: true},
		DeletionReason:  pgtype.Text{String: params.Reason, Valid: params.Reason != ""},
		SupersededByUri: supersededBy,
		Payload:         params.Payload,
	})
	if err != nil {
		return fmt.Errorf("create tombstone: %w", err)
	}

	return nil
}

// GetTombstoneByULID retrieves the tombstone for a deleted place by ULID
func (r *PlaceRepository) GetTombstoneByULID(ctx context.Context, ulid string) (*places.Tombstone, error) {
	queries := Queries{db: r.queryer()}

	row, err := queries.GetPlaceTombstoneByULID(ctx, ulid)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, places.ErrNotFound
		}
		return nil, fmt.Errorf("get tombstone by ulid: %w", err)
	}

	var supersededBy *string
	if row.SupersededByUri.Valid {
		supersededBy = &row.SupersededByUri.String
	}

	tombstone := &places.Tombstone{
		ID:           row.ID.String(),
		PlaceID:      row.PlaceID.String(),
		PlaceURI:     row.PlaceUri,
		DeletedAt:    row.DeletedAt.Time,
		Reason:       row.DeletionReason.String,
		SupersededBy: supersededBy,
		Payload:      row.Payload,
	}

	return tombstone, nil
}

type placeQueryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (r *PlaceRepository) queryer() placeQueryer {
	if r.tx != nil {
		return r.tx
	}
	return r.pool
}
