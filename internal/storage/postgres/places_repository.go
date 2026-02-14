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
	ID                      string
	ULID                    string
	Name                    string
	Description             *string
	StreetAddress           *string
	City                    *string
	Region                  *string
	PostalCode              *string
	Country                 *string
	Latitude                pgtype.Numeric
	Longitude               pgtype.Numeric
	Telephone               *string
	Email                   *string
	URL                     *string
	MaximumAttendeeCapacity pgtype.Int4
	VenueType               *string
	FederationURI           *string
	DeletedAt               pgtype.Timestamptz
	Reason                  pgtype.Text
	DistanceKm              *float64
	CreatedAt               pgtype.Timestamptz
	UpdatedAt               pgtype.Timestamptz
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

	// Build query based on whether proximity search is active
	useProximity := filters.Latitude != nil && filters.Longitude != nil
	var query string
	var args []interface{}

	if useProximity {
		// Proximity search query with distance calculation
		radiusKm := 10.0 // default 10km radius
		if filters.RadiusKm != nil {
			radiusKm = *filters.RadiusKm
		}
		radiusMeters := radiusKm * 1000.0

		// Build WHERE conditions dynamically based on filters
		var whereClauses []string
		var queryArgs []interface{}

		// First 3 args are for proximity (lon, lat, radius)
		whereClauses = append(whereClauses, "p.geo_point IS NOT NULL")
		whereClauses = append(whereClauses, "ST_DWithin(p.geo_point::geography, ST_MakePoint($1, $2)::geography, $3)")
		queryArgs = append(queryArgs, *filters.Longitude, *filters.Latitude, radiusMeters)
		argPos := 4

		// Add city filter if provided
		if filters.City != "" {
			whereClauses = append(whereClauses, fmt.Sprintf("p.address_locality ILIKE $%d", argPos))
			queryArgs = append(queryArgs, "%"+filters.City+"%")
			argPos++
		}

		// Add query filter if provided
		if filters.Query != "" {
			whereClauses = append(whereClauses, fmt.Sprintf("(p.name ILIKE $%d OR COALESCE(p.description, '') ILIKE $%d)", argPos, argPos+1))
			queryArgs = append(queryArgs, "%"+filters.Query+"%", "%"+filters.Query+"%")
			argPos += 2
		}

		// Add cursor condition if provided
		// For proximity search sorted by (distance_km, ulid), we use ulid-only cursor.
		// This works because ULIDs are unique and maintain consistent pagination
		// even across items with the same distance.
		if cursorULID != nil {
			whereClauses = append(whereClauses, fmt.Sprintf("p.ulid > $%d", argPos))
			queryArgs = append(queryArgs, *cursorULID)
			argPos++
		}

		// Add LIMIT
		queryArgs = append(queryArgs, limitPlusOne)

		whereClause := strings.Join(whereClauses, " AND ")

		query = fmt.Sprintf(`
SELECT p.id, p.ulid, p.name, p.description,
       p.street_address, p.address_locality, p.address_region, p.postal_code, p.address_country,
       p.latitude, p.longitude,
       p.telephone, p.email, p.url, p.maximum_attendee_capacity, p.venue_type,
       p.federation_uri,
       p.deleted_at, p.deletion_reason,
       ST_Distance(p.geo_point::geography, ST_MakePoint($1, $2)::geography) / 1000.0 AS distance_km,
       p.created_at, p.updated_at
  FROM places p
 WHERE %s
 ORDER BY distance_km ASC, p.ulid ASC
 LIMIT $%d
`, whereClause, argPos)

		args = queryArgs
	} else {
		// Standard query without proximity
		query = `
SELECT p.id, p.ulid, p.name, p.description,
       p.street_address, p.address_locality, p.address_region, p.postal_code, p.address_country,
       p.latitude, p.longitude,
       p.telephone, p.email, p.url, p.maximum_attendee_capacity, p.venue_type,
       p.federation_uri,
       p.deleted_at, p.deletion_reason,
       NULL::numeric AS distance_km,
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
`
		args = []interface{}{
			filters.City,
			filters.Query,
			cursorTimestamp,
			cursorULID,
			limitPlusOne,
		}
	}

	rows, err := queryer.Query(ctx, query, args...)
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
			&row.StreetAddress,
			&row.City,
			&row.Region,
			&row.PostalCode,
			&row.Country,
			&row.Latitude,
			&row.Longitude,
			&row.Telephone,
			&row.Email,
			&row.URL,
			&row.MaximumAttendeeCapacity,
			&row.VenueType,
			&row.FederationURI,
			&row.DeletedAt,
			&row.Reason,
			&row.DistanceKm,
			&row.CreatedAt,
			&row.UpdatedAt,
		); err != nil {
			return places.ListResult{}, fmt.Errorf("scan places: %w", err)
		}
		items = append(items, placeRowToDomain(&row))
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
SELECT p.id, p.ulid, p.name, p.description,
       p.street_address, p.address_locality, p.address_region, p.postal_code, p.address_country,
       p.latitude, p.longitude,
       p.telephone, p.email, p.url, p.maximum_attendee_capacity, p.venue_type,
       p.federation_uri,
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
		&data.StreetAddress,
		&data.City,
		&data.Region,
		&data.PostalCode,
		&data.Country,
		&data.Latitude,
		&data.Longitude,
		&data.Telephone,
		&data.Email,
		&data.URL,
		&data.MaximumAttendeeCapacity,
		&data.VenueType,
		&data.FederationURI,
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

	place := placeRowToDomain(&data)
	if data.DeletedAt.Valid {
		place.Lifecycle = "deleted"
	}
	return &place, nil
}

// placeRowToDomain converts a placeRow (DB scan target) to a places.Place domain struct.
func placeRowToDomain(row *placeRow) places.Place {
	place := places.Place{
		ID:                      row.ID,
		ULID:                    row.ULID,
		Name:                    row.Name,
		Description:             derefString(row.Description),
		StreetAddress:           derefString(row.StreetAddress),
		City:                    derefString(row.City),
		Region:                  derefString(row.Region),
		PostalCode:              derefString(row.PostalCode),
		Country:                 derefString(row.Country),
		Latitude:                numericToFloat64Ptr(row.Latitude),
		Longitude:               numericToFloat64Ptr(row.Longitude),
		Telephone:               derefString(row.Telephone),
		Email:                   derefString(row.Email),
		URL:                     derefString(row.URL),
		MaximumAttendeeCapacity: int4Ptr(row.MaximumAttendeeCapacity),
		VenueType:               derefString(row.VenueType),
		FederationURI:           derefString(row.FederationURI),
		DistanceKm:              row.DistanceKm,
	}
	if row.CreatedAt.Valid {
		place.CreatedAt = row.CreatedAt.Time
	}
	if row.UpdatedAt.Valid {
		place.UpdatedAt = row.UpdatedAt.Time
	}
	return place
}

// numericToFloat64Ptr converts a pgtype.Numeric to *float64.
// Returns nil if the Numeric is not valid (SQL NULL).
func numericToFloat64Ptr(n pgtype.Numeric) *float64 {
	if !n.Valid {
		return nil
	}
	f, _ := n.Float64Value()
	if !f.Valid {
		return nil
	}
	return &f.Float64
}

// int4Ptr converts a pgtype.Int4 to *int. Returns nil if not valid (SQL NULL).
func int4Ptr(n pgtype.Int4) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int32)
	return &v
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
