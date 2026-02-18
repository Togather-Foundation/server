package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/api/pagination"
	"github.com/Togather-Foundation/server/internal/domain/organizations"
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
	query := strings.TrimSpace(filters.Query)
	city := strings.TrimSpace(filters.City)
	limit := paginationArgs.Limit
	if limit <= 0 {
		limit = 50
	}
	limitPlusOne := limit + 1

	var cursorTime *time.Time
	var cursorULID *string
	var cursorName *string
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

	// Determine sort column and order (whitelist to prevent SQL injection)
	sortCol := "o.created_at"
	if filters.Sort == "name" {
		sortCol = "o.name"
	}
	orderDir := "ASC"
	if filters.Order == "desc" {
		orderDir = "DESC"
	}

	// Build cursor comparison operator based on sort direction
	cursorOp := ">"
	if orderDir == "DESC" {
		cursorOp = "<"
	}

	// For name-based sorting, we need to look up the cursor name from the ULID
	if filters.Sort == "name" && cursorULID != nil {
		var name string
		err := queryer.QueryRow(ctx, "SELECT name FROM organizations WHERE ulid = $1", *cursorULID).Scan(&name)
		if err == nil {
			cursorName = &name
		}
		// If lookup fails, skip cursor (start from beginning)
	}

	// Build query dynamically based on sort column
	var cursorCondition string
	var args []interface{}
	argPos := 1

	// Start building WHERE clauses
	whereClauses := []string{"o.deleted_at IS NULL"}

	// Query filter
	if query != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("(o.name ILIKE '%%' || $%d || '%%' OR o.legal_name ILIKE '%%' || $%d || '%%')", argPos, argPos))
		args = append(args, query)
		argPos++
	}

	// City filter
	if city != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("o.address_locality ILIKE '%%' || $%d || '%%'", argPos))
		args = append(args, city)
		argPos++
	}

	// Cursor condition
	if filters.Sort == "name" && cursorName != nil && cursorULID != nil {
		cursorCondition = fmt.Sprintf("(o.name %s $%d OR (o.name = $%d AND o.ulid > $%d))", cursorOp, argPos, argPos, argPos+1)
		args = append(args, *cursorName, *cursorULID)
		argPos += 2
	} else if cursorTime != nil && cursorULID != nil {
		cursorCondition = fmt.Sprintf("(o.created_at %s $%d::timestamptz OR (o.created_at = $%d::timestamptz AND o.ulid > $%d))", cursorOp, argPos, argPos, argPos+1)
		args = append(args, *cursorTime, *cursorULID)
		argPos += 2
	}

	if cursorCondition != "" {
		whereClauses = append(whereClauses, cursorCondition)
	}

	// Build final query
	whereClause := strings.Join(whereClauses, " AND ")
	args = append(args, limitPlusOne)

	sqlQuery := fmt.Sprintf(`
SELECT o.id, o.ulid, o.name, o.legal_name, o.description, o.email, o.telephone, o.url,
       o.address_locality, o.address_region, o.address_country, o.street_address, o.postal_code,
       o.organization_type, o.federation_uri, o.alternate_name, o.created_at, o.updated_at
  FROM organizations o
 WHERE %s
 ORDER BY %s %s, o.ulid ASC
 LIMIT $%d
`, whereClause, sortCol, orderDir, argPos)

	rows, err := queryer.Query(ctx, sqlQuery, args...)
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
			&row.Description,
			&row.Email,
			&row.Telephone,
			&row.URL,
			&row.AddressLocality,
			&row.AddressRegion,
			&row.AddressCountry,
			&row.StreetAddress,
			&row.PostalCode,
			&row.OrganizationType,
			&row.FederationURI,
			&row.AlternateName,
			&row.CreatedAt,
			&row.UpdatedAt,
		); err != nil {
			return organizations.ListResult{}, fmt.Errorf("scan organizations: %w", err)
		}
		org := organizationRowToDomain(row)
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
