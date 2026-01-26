package postgres

import (
	"context"
	"errors"

	"github.com/Togather-Foundation/server/internal/domain/federation"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ federation.Repository = (*FederationRepository)(nil)

type FederationRepository struct {
	pool    *pgxpool.Pool
	queries *Queries
}

func NewFederationRepository(pool *pgxpool.Pool) *FederationRepository {
	return &FederationRepository{
		pool:    pool,
		queries: New(pool),
	}
}

func (r *FederationRepository) Create(ctx context.Context, params federation.CreateNodeParams) (*federation.Node, error) {
	var geographicScope pgtype.Text
	if params.GeographicScope != nil {
		geographicScope = pgtype.Text{String: *params.GeographicScope, Valid: true}
	}

	var contactEmail, contactName, notes pgtype.Text
	if params.ContactEmail != nil {
		contactEmail = pgtype.Text{String: *params.ContactEmail, Valid: true}
	}
	if params.ContactName != nil {
		contactName = pgtype.Text{String: *params.ContactName, Valid: true}
	}
	if params.Notes != nil {
		notes = pgtype.Text{String: *params.Notes, Valid: true}
	}

	syncEnabled := pgtype.Bool{Bool: params.SyncEnabled, Valid: true}
	syncDirection := pgtype.Text{String: params.SyncDirection, Valid: true}

	row, err := r.queries.CreateFederationNode(ctx, CreateFederationNodeParams{
		NodeDomain:       params.NodeDomain,
		NodeName:         params.NodeName,
		BaseUrl:          params.BaseURL,
		ApiVersion:       params.APIVersion,
		GeographicScope:  geographicScope,
		TrustLevel:       int32(params.TrustLevel),
		FederationStatus: params.FederationStatus,
		SyncEnabled:      syncEnabled,
		SyncDirection:    syncDirection,
		ContactEmail:     contactEmail,
		ContactName:      contactName,
		Notes:            notes,
	})
	if err != nil {
		return nil, err
	}

	return mapFederationNodeRow(row), nil
}

func (r *FederationRepository) GetByID(ctx context.Context, id uuid.UUID) (*federation.Node, error) {
	var pgUUID pgtype.UUID
	if err := pgUUID.Scan(id); err != nil {
		return nil, err
	}

	row, err := r.queries.GetFederationNodeByID(ctx, pgUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, federation.ErrNodeNotFound
		}
		return nil, err
	}

	return mapFederationNodeRow(row), nil
}

func (r *FederationRepository) GetByDomain(ctx context.Context, domain string) (*federation.Node, error) {
	row, err := r.queries.GetFederationNodeByDomain(ctx, domain)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, federation.ErrNodeNotFound
		}
		return nil, err
	}

	return mapFederationNodeRow(row), nil
}

func (r *FederationRepository) List(ctx context.Context, filters federation.ListNodesFilters) ([]*federation.Node, error) {
	var syncEnabled pgtype.Bool
	if filters.SyncEnabled != nil {
		syncEnabled = pgtype.Bool{Bool: *filters.SyncEnabled, Valid: true}
	}

	var isOnline pgtype.Bool
	if filters.IsOnline != nil {
		isOnline = pgtype.Bool{Bool: *filters.IsOnline, Valid: true}
	}

	rows, err := r.queries.ListFederationNodes(ctx, ListFederationNodesParams{
		FederationStatus: filters.FederationStatus,
		SyncEnabled:      syncEnabled,
		IsOnline:         isOnline,
		Limit:            int32(filters.Limit),
	})
	if err != nil {
		return nil, err
	}

	nodes := make([]*federation.Node, 0, len(rows))
	for _, row := range rows {
		nodes = append(nodes, mapFederationNodeRow(row))
	}

	return nodes, nil
}

func (r *FederationRepository) Update(ctx context.Context, id uuid.UUID, params federation.UpdateNodeParams) (*federation.Node, error) {
	var pgUUID pgtype.UUID
	if err := pgUUID.Scan(id); err != nil {
		return nil, err
	}

	updateParams := UpdateFederationNodeParams{
		ID: pgUUID,
	}

	if params.NodeName != nil {
		updateParams.NodeName = pgtype.Text{String: *params.NodeName, Valid: true}
	}
	if params.BaseURL != nil {
		updateParams.BaseUrl = pgtype.Text{String: *params.BaseURL, Valid: true}
	}
	if params.APIVersion != nil {
		updateParams.ApiVersion = pgtype.Text{String: *params.APIVersion, Valid: true}
	}
	if params.GeographicScope != nil {
		updateParams.GeographicScope = pgtype.Text{String: *params.GeographicScope, Valid: true}
	}
	if params.TrustLevel != nil {
		updateParams.TrustLevel = pgtype.Int4{Int32: int32(*params.TrustLevel), Valid: true}
	}
	if params.FederationStatus != nil {
		updateParams.FederationStatus = pgtype.Text{String: *params.FederationStatus, Valid: true}
	}
	if params.SyncEnabled != nil {
		updateParams.SyncEnabled = pgtype.Bool{Bool: *params.SyncEnabled, Valid: true}
	}
	if params.SyncDirection != nil {
		updateParams.SyncDirection = pgtype.Text{String: *params.SyncDirection, Valid: true}
	}
	if params.ContactEmail != nil {
		updateParams.ContactEmail = pgtype.Text{String: *params.ContactEmail, Valid: true}
	}
	if params.ContactName != nil {
		updateParams.ContactName = pgtype.Text{String: *params.ContactName, Valid: true}
	}
	if params.Notes != nil {
		updateParams.Notes = pgtype.Text{String: *params.Notes, Valid: true}
	}

	row, err := r.queries.UpdateFederationNode(ctx, updateParams)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, federation.ErrNodeNotFound
		}
		return nil, err
	}

	return mapFederationNodeRow(row), nil
}

func (r *FederationRepository) Delete(ctx context.Context, id uuid.UUID) error {
	var pgUUID pgtype.UUID
	if err := pgUUID.Scan(id); err != nil {
		return err
	}

	err := r.queries.DeleteFederationNode(ctx, pgUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return federation.ErrNodeNotFound
		}
		return err
	}
	return nil
}

func mapFederationNodeRow(row FederationNode) *federation.Node {
	var id uuid.UUID
	_ = row.ID.Scan(&id)

	node := &federation.Node{
		ID:               id,
		NodeDomain:       row.NodeDomain,
		NodeName:         row.NodeName,
		BaseURL:          row.BaseUrl,
		APIVersion:       row.ApiVersion,
		TrustLevel:       int(row.TrustLevel),
		FederationStatus: row.FederationStatus,
		SyncEnabled:      row.SyncEnabled.Bool,
		SyncDirection:    row.SyncDirection.String,
		RequiresAuth:     row.RequiresAuthentication.Bool,
		IsOnline:         row.IsOnline.Bool,
		CreatedAt:        row.CreatedAt.Time,
		UpdatedAt:        row.UpdatedAt.Time,
	}

	if row.GeographicScope.Valid {
		node.GeographicScope = &row.GeographicScope.String
	}
	if row.LastSyncAt.Valid {
		node.LastSyncAt = &row.LastSyncAt.Time
	}
	if row.LastSuccessfulSyncAt.Valid {
		node.LastSuccessfulSyncAt = &row.LastSuccessfulSyncAt.Time
	}
	if row.SyncCursor.Valid {
		node.SyncCursor = &row.SyncCursor.String
	}
	if row.ContactEmail.Valid {
		node.ContactEmail = &row.ContactEmail.String
	}
	if row.ContactName.Valid {
		node.ContactName = &row.ContactName.String
	}
	if row.LastHealthCheckAt.Valid {
		node.LastHealthCheckAt = &row.LastHealthCheckAt.Time
	}
	if row.LastErrorAt.Valid {
		node.LastErrorAt = &row.LastErrorAt.Time
	}
	if row.LastErrorMessage.Valid {
		node.LastErrorMessage = &row.LastErrorMessage.String
	}
	if row.Notes.Valid {
		node.Notes = &row.Notes.String
	}

	return node
}
