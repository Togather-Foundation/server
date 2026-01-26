package postgres

import (
	"context"
	"fmt"

	"github.com/Togather-Foundation/server/internal/domain/federation"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// SyncRepository implements federation.SyncRepository using SQLc queries.
type SyncRepository struct {
	queries *Queries
}

// NewSyncRepository creates a new sync repository.
func NewSyncRepository(queries *Queries) *SyncRepository {
	return &SyncRepository{
		queries: queries,
	}
}

// GetEventByFederationURI fetches an event by its federation URI.
func (r *SyncRepository) GetEventByFederationURI(ctx context.Context, federationUri string) (federation.Event, error) {
	row, err := r.queries.GetEventByFederationURI(ctx, pgtype.Text{String: federationUri, Valid: true})
	if err != nil {
		if err == pgx.ErrNoRows {
			return federation.Event{}, fmt.Errorf("event not found")
		}
		return federation.Event{}, err
	}

	return federation.Event{
		ID:            row.ID,
		ULID:          row.Ulid,
		Name:          row.Name,
		FederationURI: row.FederationUri,
		OriginNodeID:  row.OriginNodeID,
	}, nil
}

// UpsertFederatedEvent upserts a federated event.
func (r *SyncRepository) UpsertFederatedEvent(ctx context.Context, arg federation.UpsertFederatedEventParams) (federation.Event, error) {
	// Convert federation params to SQLc params
	sqlcParams := UpsertFederatedEventParams{
		Ulid:                  arg.Ulid,
		Name:                  arg.Name,
		Description:           arg.Description,
		LifecycleState:        arg.LifecycleState,
		EventStatus:           arg.EventStatus,
		AttendanceMode:        arg.AttendanceMode,
		OrganizerID:           arg.OrganizerID,
		PrimaryVenueID:        arg.PrimaryVenueID,
		SeriesID:              arg.SeriesID,
		ImageUrl:              arg.ImageUrl,
		PublicUrl:             arg.PublicUrl,
		VirtualUrl:            arg.VirtualUrl,
		Keywords:              arg.Keywords,
		InLanguage:            arg.InLanguage,
		DefaultLanguage:       arg.DefaultLanguage,
		IsAccessibleForFree:   arg.IsAccessibleForFree,
		AccessibilityFeatures: arg.AccessibilityFeatures,
		EventDomain:           arg.EventDomain,
		OriginNodeID:          arg.OriginNodeID,
		FederationUri:         arg.FederationUri,
		LicenseUrl:            arg.LicenseUrl,
		LicenseStatus:         arg.LicenseStatus,
		Confidence:            arg.Confidence,
		QualityScore:          arg.QualityScore,
		Version:               arg.Version,
		CreatedAt:             arg.CreatedAt,
		UpdatedAt:             arg.UpdatedAt,
		PublishedAt:           arg.PublishedAt,
	}

	row, err := r.queries.UpsertFederatedEvent(ctx, sqlcParams)
	if err != nil {
		return federation.Event{}, err
	}

	return federation.Event{
		ID:            row.ID,
		ULID:          row.Ulid,
		Name:          row.Name,
		FederationURI: row.FederationUri,
		OriginNodeID:  row.OriginNodeID,
	}, nil
}

// GetFederationNodeByDomain fetches a federation node by domain.
func (r *SyncRepository) GetFederationNodeByDomain(ctx context.Context, nodeDomain string) (federation.FederationNode, error) {
	row, err := r.queries.GetFederationNodeByDomain(ctx, nodeDomain)
	if err != nil {
		if err == pgx.ErrNoRows {
			return federation.FederationNode{}, fmt.Errorf("federation node not found")
		}
		return federation.FederationNode{}, err
	}

	return federation.FederationNode{
		ID:         row.ID,
		NodeDomain: row.NodeDomain,
	}, nil
}
