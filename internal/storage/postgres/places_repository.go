package postgres

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/api/pagination"
	"github.com/Togather-Foundation/server/internal/domain/ids"
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
	Street      *string
	City        *string
	Region      *string
	Postal      *string
	Country     *string
	Latitude    pgtype.Numeric
	Longitude   pgtype.Numeric
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
SELECT p.id, p.ulid, p.name, p.description, p.street_address, p.address_locality, p.address_region,
       p.postal_code, p.address_country, p.latitude, p.longitude, p.created_at, p.updated_at
  FROM places p
 WHERE ($1 = '' OR p.address_locality ILIKE '%' || $1 || '%')
   AND ($2 = '' OR p.name ILIKE '%' || $2 || '%' OR p.description ILIKE '%' || $2 || '%')
   AND (
     $3::timestamptz IS NULL OR
     p.created_at > $3::timestamptz OR
     (p.created_at = $3::timestamptz AND p.ulid > $4)
   )
   AND (
     $6::boolean IS FALSE OR
     ST_DWithin(
       p.geo_point::geography,
       ST_SetSRID(ST_MakePoint($7, $8), 4326)::geography,
       $9
     )
   )
 ORDER BY p.created_at ASC, p.ulid ASC
 LIMIT $5
`,
		filters.City,
		filters.Query,
		cursorTimestamp,
		cursorULID,
		limitPlusOne,
		filters.NearLat != nil && filters.NearLon != nil && filters.RadiusMeters > 0,
		float64Value(filters.NearLon),
		float64Value(filters.NearLat),
		filters.RadiusMeters,
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
			&row.Street,
			&row.City,
			&row.Region,
			&row.Postal,
			&row.Country,
			&row.Latitude,
			&row.Longitude,
			&row.CreatedAt,
			&row.UpdatedAt,
		); err != nil {
			return places.ListResult{}, fmt.Errorf("scan places: %w", err)
		}
		place := places.Place{
			ID:            row.ID,
			ULID:          row.ULID,
			Name:          row.Name,
			Description:   derefString(row.Description),
			StreetAddress: derefString(row.Street),
			City:          derefString(row.City),
			Region:        derefString(row.Region),
			PostalCode:    derefString(row.Postal),
			Country:       derefString(row.Country),
			Latitude:      numericToFloat64Ptr(row.Latitude),
			Longitude:     numericToFloat64Ptr(row.Longitude),
			CreatedAt:     time.Time{},
			UpdatedAt:     time.Time{},
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
SELECT p.id, p.ulid, p.name, p.description, p.street_address, p.address_locality, p.address_region,
       p.postal_code, p.address_country, p.latitude, p.longitude, p.deleted_at, p.deletion_reason,
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
		&data.Street,
		&data.City,
		&data.Region,
		&data.Postal,
		&data.Country,
		&data.Latitude,
		&data.Longitude,
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
		ID:            data.ID,
		ULID:          data.ULID,
		Name:          data.Name,
		Description:   derefString(data.Description),
		StreetAddress: derefString(data.Street),
		City:          derefString(data.City),
		Region:        derefString(data.Region),
		PostalCode:    derefString(data.Postal),
		Country:       derefString(data.Country),
		Latitude:      numericToFloat64Ptr(data.Latitude),
		Longitude:     numericToFloat64Ptr(data.Longitude),
		Lifecycle:     "",
		CreatedAt:     time.Time{},
		UpdatedAt:     time.Time{},
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

func (r *PlaceRepository) Create(ctx context.Context, params places.CreateParams) (*places.Place, error) {
	queryer := r.queryer()

	ulidValue := strings.TrimSpace(params.ULID)
	if ulidValue == "" {
		value, err := ids.NewULID()
		if err != nil {
			return nil, fmt.Errorf("generate place ulid: %w", err)
		}
		ulidValue = value
	}

	row := queryer.QueryRow(ctx, `
INSERT INTO places (
  ulid,
  name,
  description,
  street_address,
  address_locality,
  address_region,
  postal_code,
  address_country,
  latitude,
  longitude,
  federation_uri
) VALUES (
  $1,
  $2,
  NULLIF($3, ''),
  NULLIF($4, ''),
  NULLIF($5, ''),
  NULLIF($6, ''),
  NULLIF($7, ''),
  NULLIF($8, ''),
  $9,
  $10,
  $11
)
RETURNING id, ulid, name, description, street_address, address_locality, address_region,
          postal_code, address_country, latitude, longitude, created_at, updated_at
`,
		ulidValue,
		strings.TrimSpace(params.Name),
		strings.TrimSpace(params.Description),
		strings.TrimSpace(params.StreetAddress),
		strings.TrimSpace(params.AddressLocality),
		strings.TrimSpace(params.AddressRegion),
		strings.TrimSpace(params.PostalCode),
		strings.TrimSpace(params.AddressCountry),
		numericFromFloat64Ptr(params.Latitude),
		numericFromFloat64Ptr(params.Longitude),
		nullableString(params.FederationURI),
	)

	var data placeRow
	if err := row.Scan(
		&data.ID,
		&data.ULID,
		&data.Name,
		&data.Description,
		&data.Street,
		&data.City,
		&data.Region,
		&data.Postal,
		&data.Country,
		&data.Latitude,
		&data.Longitude,
		&data.CreatedAt,
		&data.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("create place: %w", err)
	}

	place := &places.Place{
		ID:            data.ID,
		ULID:          data.ULID,
		Name:          data.Name,
		Description:   derefString(data.Description),
		StreetAddress: derefString(data.Street),
		City:          derefString(data.City),
		Region:        derefString(data.Region),
		PostalCode:    derefString(data.Postal),
		Country:       derefString(data.Country),
		Latitude:      numericToFloat64Ptr(data.Latitude),
		Longitude:     numericToFloat64Ptr(data.Longitude),
		CreatedAt:     time.Time{},
		UpdatedAt:     time.Time{},
	}
	if data.CreatedAt.Valid {
		place.CreatedAt = data.CreatedAt.Time
	}
	if data.UpdatedAt.Valid {
		place.UpdatedAt = data.UpdatedAt.Time
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

func nullableString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func numericFromFloat64Ptr(value *float64) pgtype.Numeric {
	if value == nil {
		return pgtype.Numeric{}
	}
	return pgtype.Numeric{Int: big.NewInt(int64(math.Round(*value * 1e7))), Exp: -7, Valid: true}
}

func numericToFloat64Ptr(value pgtype.Numeric) *float64 {
	if !value.Valid {
		return nil
	}
	floatValue, err := value.Float64Value()
	if err != nil || !floatValue.Valid {
		return nil
	}
	result := floatValue.Float64
	return &result
}

func float64Value(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}
