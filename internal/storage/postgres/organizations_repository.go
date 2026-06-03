package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/api/pagination"
	"github.com/Togather-Foundation/server/internal/domain/organizations"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ organizations.Repository = (*OrganizationRepository)(nil)

type OrganizationRepository struct {
	pool *pgxpool.Pool
	tx   pgx.Tx
}

func (r *OrganizationRepository) List(ctx context.Context, filters organizations.Filters, paginationArgs organizations.Pagination) (organizations.ListResult, error) {
	queryer := r.queryer()
	queries := Queries{db: queryer}

	var cursorTimestamp *time.Time
	var cursorULID *string
	var cursorName *string
	if strings.TrimSpace(paginationArgs.After) != "" {
		cursor, err := pagination.DecodeEventCursor(paginationArgs.After)
		if err != nil {
			return organizations.ListResult{}, err
		}
		value := cursor.Timestamp.UTC()
		cursorTimestamp = &value
		ulid := strings.ToUpper(cursor.ULID)
		cursorULID = &ulid

		// For name-based sorting, look up the cursor name from the ULID
		if filters.Sort == "name" {
			var name string
			err := queryer.QueryRow(ctx, "SELECT name FROM organizations WHERE ulid = $1", ulid).Scan(&name)
			if err == nil {
				cursorName = &name
			}
		}
	}

	limit := paginationArgs.Limit
	if limit <= 0 {
		limit = 50
	}
	limitPlusOne := limit + 1

	// Prepare common parameters
	var cityParam, queryParam, cursorULIDParam, cursorNameParam pgtype.Text
	var cursorTimestampParam pgtype.Timestamptz

	if filters.City != "" {
		cityParam = pgtype.Text{String: filters.City, Valid: true}
	}
	if filters.Query != "" {
		queryParam = pgtype.Text{String: filters.Query, Valid: true}
	}
	if cursorULID != nil {
		cursorULIDParam = pgtype.Text{String: *cursorULID, Valid: true}
	}
	if cursorTimestamp != nil {
		cursorTimestampParam = pgtype.Timestamptz{Time: *cursorTimestamp, Valid: true}
	}
	if cursorName != nil {
		cursorNameParam = pgtype.Text{String: *cursorName, Valid: true}
	}

	// Select the appropriate query based on sort and order, and convert rows to domain objects
	var items []organizations.Organization

	if filters.Sort == "name" {
		if filters.Order == "desc" {
			params := ListOrganizationsByNameDescParams{
				City:       cityParam,
				Query:      queryParam,
				CursorName: cursorNameParam,
				CursorUlid: cursorULIDParam,
				Limit:      int32(limitPlusOne),
			}
			rows, err := queries.ListOrganizationsByNameDesc(ctx, params)
			if err != nil {
				return organizations.ListResult{}, fmt.Errorf("list organizations: %w", err)
			}
			items = make([]organizations.Organization, 0, len(rows))
			for _, row := range rows {
				items = append(items, row.Organization.toDomain())
			}
		} else {
			params := ListOrganizationsByNameParams{
				City:       cityParam,
				Query:      queryParam,
				CursorName: cursorNameParam,
				CursorUlid: cursorULIDParam,
				Limit:      int32(limitPlusOne),
			}
			rows, err := queries.ListOrganizationsByName(ctx, params)
			if err != nil {
				return organizations.ListResult{}, fmt.Errorf("list organizations: %w", err)
			}
			items = make([]organizations.Organization, 0, len(rows))
			for _, row := range rows {
				items = append(items, row.Organization.toDomain())
			}
		}
	} else {
		if filters.Order == "desc" {
			params := ListOrganizationsByCreatedAtDescParams{
				City:            cityParam,
				Query:           queryParam,
				CursorTimestamp: cursorTimestampParam,
				CursorUlid:      cursorULIDParam,
				Limit:           int32(limitPlusOne),
			}
			rows, err := queries.ListOrganizationsByCreatedAtDesc(ctx, params)
			if err != nil {
				return organizations.ListResult{}, fmt.Errorf("list organizations: %w", err)
			}
			items = make([]organizations.Organization, 0, len(rows))
			for _, row := range rows {
				items = append(items, row.Organization.toDomain())
			}
		} else {
			params := ListOrganizationsByCreatedAtParams{
				City:            cityParam,
				Query:           queryParam,
				CursorTimestamp: cursorTimestampParam,
				CursorUlid:      cursorULIDParam,
				Limit:           int32(limitPlusOne),
			}
			rows, err := queries.ListOrganizationsByCreatedAt(ctx, params)
			if err != nil {
				return organizations.ListResult{}, fmt.Errorf("list organizations: %w", err)
			}
			items = make([]organizations.Organization, 0, len(rows))
			for _, row := range rows {
				items = append(items, row.Organization.toDomain())
			}
		}
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
	queries := Queries{db: r.queryer()}

	row, err := queries.GetOrganizationByULID(ctx, ulid)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, organizations.ErrNotFound
		}
		return nil, fmt.Errorf("get organization: %w", err)
	}

	org := row.Organization.toDomain()
	if row.Organization.DeletedAt.Valid {
		org.Lifecycle = "deleted"
	}
	return &org, nil
}

// Update updates an organization's fields. Nil pointer fields in params are not changed (COALESCE pattern).
func (r *OrganizationRepository) Update(ctx context.Context, ulid string, params organizations.UpdateOrganizationParams) (*organizations.Organization, error) {
	queries := Queries{db: r.queryer()}

	updateParams := UpdateOrganizationParams{
		Ulid: ulid,
	}

	if params.Name != nil {
		updateParams.Name = pgtype.Text{String: *params.Name, Valid: true}
	}
	if params.Description != nil {
		updateParams.Description = pgtype.Text{String: *params.Description, Valid: true}
	}
	if params.StreetAddress != nil {
		updateParams.StreetAddress = pgtype.Text{String: *params.StreetAddress, Valid: true}
	}
	if params.AddressLocality != nil {
		updateParams.AddressLocality = pgtype.Text{String: *params.AddressLocality, Valid: true}
	}
	if params.AddressRegion != nil {
		updateParams.AddressRegion = pgtype.Text{String: *params.AddressRegion, Valid: true}
	}
	if params.PostalCode != nil {
		updateParams.PostalCode = pgtype.Text{String: *params.PostalCode, Valid: true}
	}
	if params.AddressCountry != nil {
		updateParams.AddressCountry = pgtype.Text{String: *params.AddressCountry, Valid: true}
	}
	if params.Telephone != nil {
		updateParams.Telephone = pgtype.Text{String: *params.Telephone, Valid: true}
	}
	if params.Email != nil {
		updateParams.Email = pgtype.Text{String: *params.Email, Valid: true}
	}
	if params.URL != nil {
		updateParams.Url = pgtype.Text{String: *params.URL, Valid: true}
	}

	row, err := queries.UpdateOrganization(ctx, updateParams)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, organizations.ErrNotFound
		}
		return nil, fmt.Errorf("update organization: %w", err)
	}

	org := row.Organization.toDomain()
	return &org, nil
}

// SoftDelete marks an organization as deleted
func (r *OrganizationRepository) SoftDelete(ctx context.Context, ulid string, reason string) error {
	queries := Queries{db: r.queryer()}

	err := queries.SoftDeleteOrganization(ctx, SoftDeleteOrganizationParams{
		Ulid:           ulid,
		DeletionReason: pgtype.Text{String: reason, Valid: reason != ""},
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return organizations.ErrNotFound
		}
		return fmt.Errorf("soft delete organization: %w", err)
	}

	return nil
}

// CreateTombstone creates a tombstone record for a deleted organization
func (r *OrganizationRepository) CreateTombstone(ctx context.Context, params organizations.TombstoneCreateParams) error {
	queries := Queries{db: r.queryer()}

	var orgIDUUID pgtype.UUID
	if err := orgIDUUID.Scan(params.OrgID); err != nil {
		return fmt.Errorf("invalid organization ID: %w", err)
	}

	var supersededBy pgtype.Text
	if params.SupersededBy != nil {
		supersededBy = pgtype.Text{String: *params.SupersededBy, Valid: true}
	}

	err := queries.CreateOrganizationTombstone(ctx, CreateOrganizationTombstoneParams{
		OrganizationID:  orgIDUUID,
		OrganizationUri: params.OrgURI,
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

// GetTombstoneByULID retrieves the tombstone for a deleted organization by ULID
func (r *OrganizationRepository) GetTombstoneByULID(ctx context.Context, ulid string) (*organizations.Tombstone, error) {
	queries := Queries{db: r.queryer()}

	row, err := queries.GetOrganizationTombstoneByULID(ctx, ulid)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, organizations.ErrNotFound
		}
		return nil, fmt.Errorf("get tombstone by ulid: %w", err)
	}

	var supersededBy *string
	if row.SupersededByUri.Valid {
		supersededBy = &row.SupersededByUri.String
	}

	tombstone := &organizations.Tombstone{
		ID:           row.ID.String(),
		OrgID:        row.OrganizationID.String(),
		OrgURI:       row.OrganizationUri,
		DeletedAt:    row.DeletedAt.Time,
		Reason:       row.DeletionReason.String,
		SupersededBy: supersededBy,
		Payload:      row.Payload,
	}

	return tombstone, nil
}

// toDomain converts the SQLc Organization table model to the domain Organization type.
func (o *Organization) toDomain() organizations.Organization {
	org := organizations.Organization{
		Name:             o.Name,
		ULID:             o.Ulid,
		LegalName:        pgtextToString(o.LegalName),
		Description:      pgtextToString(o.Description),
		Email:            pgtextToString(o.Email),
		Telephone:        pgtextToString(o.Telephone),
		URL:              pgtextToString(o.Url),
		AddressLocality:  pgtextToString(o.AddressLocality),
		AddressRegion:    pgtextToString(o.AddressRegion),
		AddressCountry:   pgtextToString(o.AddressCountry),
		StreetAddress:    pgtextToString(o.StreetAddress),
		PostalCode:       pgtextToString(o.PostalCode),
		OrganizationType: pgtextToString(o.OrganizationType),
		FederationURI:    pgtextToString(o.FederationUri),
		AlternateName:    pgtextToString(o.AlternateName),
	}
	if o.ID.Valid {
		org.ID = uuid.UUID(o.ID.Bytes).String()
	}
	if o.CreatedAt.Valid {
		org.CreatedAt = o.CreatedAt.Time
	}
	if o.UpdatedAt.Valid {
		org.UpdatedAt = o.UpdatedAt.Time
	}
	return org
}

type organizationQueryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (r *OrganizationRepository) queryer() organizationQueryer {
	if r.tx != nil {
		return r.tx
	}
	return r.pool
}
