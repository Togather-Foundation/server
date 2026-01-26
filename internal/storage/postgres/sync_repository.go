package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/federation"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SyncRepository implements federation.SyncRepository using SQLc queries.
type SyncRepository struct {
	pool    *pgxpool.Pool
	queries *Queries
	tx      pgx.Tx
}

// NewSyncRepository creates a new sync repository.
func NewSyncRepository(pool *pgxpool.Pool, queries *Queries) *SyncRepository {
	return &SyncRepository{
		pool:    pool,
		queries: queries,
	}
}

// queryer returns the appropriate database interface (transaction or pool).
func (r *SyncRepository) queryer() DBTX {
	if r.tx != nil {
		return r.tx
	}
	return r.pool
}

// WithTransaction executes the given function within a database transaction.
// If fn returns an error, the transaction is rolled back. Otherwise it's committed.
func (r *SyncRepository) WithTransaction(ctx context.Context, fn func(txRepo federation.SyncRepository) error) error {
	if r.tx != nil {
		return fmt.Errorf("already in transaction")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	// Create a new repository instance with the transaction
	txRepo := &SyncRepository{
		pool:    r.pool,
		queries: r.queries.WithTx(tx),
		tx:      tx,
	}

	// Execute the function
	if err := fn(txRepo); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("transaction error: %w, rollback error: %v", err, rbErr)
		}
		return err
	}

	// Commit the transaction
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
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

// CreateOccurrence creates an event occurrence for a federated event.
func (r *SyncRepository) CreateOccurrence(ctx context.Context, params federation.OccurrenceCreateParams) error {
	err := r.queries.CreateFederatedEventOccurrence(ctx, CreateFederatedEventOccurrenceParams{
		EventID:    params.EventID,
		StartTime:  pgtype.Timestamptz{Time: params.StartTime, Valid: true},
		EndTime:    timestamptzFromTimePtr(params.EndTime),
		Timezone:   params.Timezone,
		VirtualUrl: textFromStringPtr(params.VirtualURL),
	})
	if err != nil {
		return fmt.Errorf("create occurrence: %w", err)
	}
	return nil
}

// Helper functions to convert between domain and pgtype
func timestamptzFromTimePtr(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{Valid: false}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func textFromStringPtr(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: *s, Valid: true}
}

// GetIdempotencyKey retrieves an idempotency key entry.
func (r *SyncRepository) GetIdempotencyKey(ctx context.Context, key string) (*federation.IdempotencyKey, error) {
	const query = `
		SELECT key, request_hash, event_ulid, created_at
		FROM idempotency_keys
		WHERE key = $1
	`

	var entry federation.IdempotencyKey
	var eventULID pgtype.Text

	err := r.queryer().QueryRow(ctx, query, key).Scan(
		&entry.Key,
		&entry.RequestHash,
		&eventULID,
		&entry.CreatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // Not found
		}
		return nil, fmt.Errorf("get idempotency key: %w", err)
	}

	if eventULID.Valid {
		entry.EventULID = &eventULID.String
	}

	return &entry, nil
}

// InsertIdempotencyKey inserts a new idempotency key entry.
func (r *SyncRepository) InsertIdempotencyKey(ctx context.Context, params federation.IdempotencyKeyParams) error {
	const query = `
		INSERT INTO idempotency_keys (key, request_hash, event_ulid)
		VALUES ($1, $2, $3)
		ON CONFLICT (key) DO NOTHING
	`

	_, err := r.queryer().Exec(ctx, query, params.Key, params.RequestHash, params.EventULID)
	if err != nil {
		return fmt.Errorf("insert idempotency key: %w", err)
	}

	return nil
}
