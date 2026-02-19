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

type organizationRow struct {
	ID               string
	ULID             string
	Name             string
	LegalName        *string
	Description      *string
	Email            *string
	Telephone        *string
	URL              *string
	AddressLocality  *string
	AddressRegion    *string
	AddressCountry   *string
	StreetAddress    *string
	PostalCode       *string
	OrganizationType *string
	FederationURI    *string
	AlternateName    *string
	DeletedAt        pgtype.Timestamptz
	Reason           pgtype.Text
	CreatedAt        pgtype.Timestamptz
	UpdatedAt        pgtype.Timestamptz
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
				items = append(items, sqlcOrganizationRowToDomain(&row))
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
				items = append(items, sqlcOrganizationRowToDomain(&row))
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
				items = append(items, sqlcOrganizationRowToDomain(&row))
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
				items = append(items, sqlcOrganizationRowToDomain(&row))
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
	queryer := r.queryer()

	row := queryer.QueryRow(ctx, `
SELECT o.id, o.ulid, o.name, o.legal_name, o.description, o.email, o.telephone, o.url,
       o.address_locality, o.address_region, o.address_country, o.street_address, o.postal_code,
       o.organization_type, o.federation_uri, o.alternate_name,
       o.deleted_at, o.deletion_reason, o.created_at, o.updated_at
  FROM organizations o
 WHERE o.ulid = $1
`, ulid)

	var data organizationRow
	if err := row.Scan(
		&data.ID,
		&data.ULID,
		&data.Name,
		&data.LegalName,
		&data.Description,
		&data.Email,
		&data.Telephone,
		&data.URL,
		&data.AddressLocality,
		&data.AddressRegion,
		&data.AddressCountry,
		&data.StreetAddress,
		&data.PostalCode,
		&data.OrganizationType,
		&data.FederationURI,
		&data.AlternateName,
		&data.DeletedAt,
		&data.Reason,
		&data.CreatedAt,
		&data.UpdatedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, organizations.ErrNotFound
		}
		return nil, fmt.Errorf("get organization: %w", err)
	}

	org := organizationRowToDomain(data)
	if data.DeletedAt.Valid {
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

	var orgID string
	_ = row.ID.Scan(&orgID)

	org := &organizations.Organization{
		ID:               orgID,
		ULID:             row.Ulid,
		Name:             row.Name,
		LegalName:        row.LegalName.String,
		Description:      row.Description.String,
		URL:              row.Url.String,
		Email:            row.Email.String,
		Telephone:        row.Telephone.String,
		AddressLocality:  row.AddressLocality.String,
		AddressRegion:    row.AddressRegion.String,
		AddressCountry:   row.AddressCountry.String,
		StreetAddress:    row.StreetAddress.String,
		PostalCode:       row.PostalCode.String,
		OrganizationType: row.OrganizationType.String,
		FederationURI:    row.FederationUri.String,
		AlternateName:    row.AlternateName.String,
		CreatedAt:        row.CreatedAt.Time,
		UpdatedAt:        row.UpdatedAt.Time,
	}

	return org, nil
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

// sqlcOrganizationRowToDomain converts SQLc-generated row types to organizations.Organization domain struct.
func sqlcOrganizationRowToDomain(row interface{}) organizations.Organization {
	// Extract fields based on concrete type
	var id pgtype.UUID
	var ulid, name string
	var legalName, description, email, telephone, url pgtype.Text
	var addressLocality, addressRegion, addressCountry, streetAddress, postalCode pgtype.Text
	var organizationType, federationURI, alternateName pgtype.Text
	var createdAt, updatedAt pgtype.Timestamptz

	switch r := row.(type) {
	case *ListOrganizationsByCreatedAtRow:
		id = r.ID
		ulid = r.Ulid
		name = r.Name
		legalName = r.LegalName
		description = r.Description
		email = r.Email
		telephone = r.Telephone
		url = r.Url
		addressLocality = r.AddressLocality
		addressRegion = r.AddressRegion
		addressCountry = r.AddressCountry
		streetAddress = r.StreetAddress
		postalCode = r.PostalCode
		organizationType = r.OrganizationType
		federationURI = r.FederationUri
		alternateName = r.AlternateName
		createdAt = r.CreatedAt
		updatedAt = r.UpdatedAt
	case *ListOrganizationsByCreatedAtDescRow:
		id = r.ID
		ulid = r.Ulid
		name = r.Name
		legalName = r.LegalName
		description = r.Description
		email = r.Email
		telephone = r.Telephone
		url = r.Url
		addressLocality = r.AddressLocality
		addressRegion = r.AddressRegion
		addressCountry = r.AddressCountry
		streetAddress = r.StreetAddress
		postalCode = r.PostalCode
		organizationType = r.OrganizationType
		federationURI = r.FederationUri
		alternateName = r.AlternateName
		createdAt = r.CreatedAt
		updatedAt = r.UpdatedAt
	case *ListOrganizationsByNameRow:
		id = r.ID
		ulid = r.Ulid
		name = r.Name
		legalName = r.LegalName
		description = r.Description
		email = r.Email
		telephone = r.Telephone
		url = r.Url
		addressLocality = r.AddressLocality
		addressRegion = r.AddressRegion
		addressCountry = r.AddressCountry
		streetAddress = r.StreetAddress
		postalCode = r.PostalCode
		organizationType = r.OrganizationType
		federationURI = r.FederationUri
		alternateName = r.AlternateName
		createdAt = r.CreatedAt
		updatedAt = r.UpdatedAt
	case *ListOrganizationsByNameDescRow:
		id = r.ID
		ulid = r.Ulid
		name = r.Name
		legalName = r.LegalName
		description = r.Description
		email = r.Email
		telephone = r.Telephone
		url = r.Url
		addressLocality = r.AddressLocality
		addressRegion = r.AddressRegion
		addressCountry = r.AddressCountry
		streetAddress = r.StreetAddress
		postalCode = r.PostalCode
		organizationType = r.OrganizationType
		federationURI = r.FederationUri
		alternateName = r.AlternateName
		createdAt = r.CreatedAt
		updatedAt = r.UpdatedAt
	}

	// Extract UUID string representation
	// Note: Use String() method, not Scan() which reads FROM source not TO destination
	orgID := ""
	if id.Valid {
		orgID = uuid.UUID(id.Bytes).String()
	}

	org := organizations.Organization{
		ID:               orgID,
		ULID:             ulid,
		Name:             name,
		LegalName:        pgtextToString(legalName),
		Description:      pgtextToString(description),
		Email:            pgtextToString(email),
		Telephone:        pgtextToString(telephone),
		URL:              pgtextToString(url),
		AddressLocality:  pgtextToString(addressLocality),
		AddressRegion:    pgtextToString(addressRegion),
		AddressCountry:   pgtextToString(addressCountry),
		StreetAddress:    pgtextToString(streetAddress),
		PostalCode:       pgtextToString(postalCode),
		OrganizationType: pgtextToString(organizationType),
		FederationURI:    pgtextToString(federationURI),
		AlternateName:    pgtextToString(alternateName),
	}
	if createdAt.Valid {
		org.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		org.UpdatedAt = updatedAt.Time
	}
	return org
}

// organizationRowToDomain converts an organizationRow to an organizations.Organization domain struct.
func organizationRowToDomain(row organizationRow) organizations.Organization {
	org := organizations.Organization{
		ID:               row.ID,
		ULID:             row.ULID,
		Name:             row.Name,
		LegalName:        derefString(row.LegalName),
		Description:      derefString(row.Description),
		URL:              derefString(row.URL),
		Email:            derefString(row.Email),
		Telephone:        derefString(row.Telephone),
		AddressLocality:  derefString(row.AddressLocality),
		AddressRegion:    derefString(row.AddressRegion),
		AddressCountry:   derefString(row.AddressCountry),
		StreetAddress:    derefString(row.StreetAddress),
		PostalCode:       derefString(row.PostalCode),
		OrganizationType: derefString(row.OrganizationType),
		FederationURI:    derefString(row.FederationURI),
		AlternateName:    derefString(row.AlternateName),
	}
	if row.CreatedAt.Valid {
		org.CreatedAt = row.CreatedAt.Time
	}
	if row.UpdatedAt.Valid {
		org.UpdatedAt = row.UpdatedAt.Time
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
