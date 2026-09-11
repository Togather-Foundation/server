package identity

import (
	"context"
	"fmt"

	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TxManager runs a function against a single transaction-scoped *postgres.Queries,
// committing on success and rolling back on error.
type TxManager interface {
	WithTx(ctx context.Context, fn func(q *postgres.Queries) error) error
}

// IdentifierStore records identifier observations and elects a primary per group.
type IdentifierStore interface {
	// RecordObservation performs the full election in one transaction (own tx).
	RecordObservation(ctx context.Context, ref IdentityRef, obs IdentifierObservation) (IdentifierObservation, error)
	// RecordObservationTx is the tx-scoped form: the caller supplies the tx so the
	// election can commit atomically with surrounding work (e.g. a decision append).
	RecordObservationTx(ctx context.Context, q *postgres.Queries, ref IdentityRef, obs IdentifierObservation) (IdentifierObservation, error)
	// GetEntityIdentifiers returns all observations for an entity (read path).
	GetEntityIdentifiers(ctx context.Context, ref IdentityRef) ([]IdentifierObservation, error)
}

// Store is the concrete IdentifierStore backed by a pgx pool.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a Store from a pgx connection pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// compile-time assertion: *Store satisfies IdentifierStore and TxManager.
var (
	_ IdentifierStore = (*Store)(nil)
	_ TxManager       = (*Store)(nil)
)

// WithTx executes fn in a new transaction, committing on success and rolling
// back on error.
func (s *Store) WithTx(ctx context.Context, fn func(q *postgres.Queries) error) error {
	if s.pool == nil {
		return fmt.Errorf("identity: database pool not configured")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("identity: begin transaction: %w", err)
	}

	if err := fn(postgres.New(tx)); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("identity: rollback after error %v: %w", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("identity: commit transaction: %w", err)
	}
	return nil
}

// RecordObservation performs the full election in its own transaction.
func (s *Store) RecordObservation(ctx context.Context, ref IdentityRef, obs IdentifierObservation) (IdentifierObservation, error) {
	var winner IdentifierObservation
	err := s.WithTx(ctx, func(q *postgres.Queries) error {
		var err error
		winner, err = s.RecordObservationTx(ctx, q, ref, obs)
		return err
	})
	if err != nil {
		return IdentifierObservation{}, err
	}
	return winner, nil
}

// RecordObservationTx performs the full election inside the caller's transaction:
//
//  1. pg_advisory_xact_lock(hash(entity_type|entity_id|authority))
//  2. UpsertObservation (INSERT sets is_primary=false; ON CONFLICT never touches is_primary)
//  3. ListGroupIdentifiersForUpdate (authority trust/priority, FOR UPDATE OF ei)
//  4. ElectPrimary (Go rank)
//  5. DemotePrimaryAndSupersede(group, winner.ID)
//  6. SetPrimary(winner.ID) — unconditional, so zero-primary is impossible.
//
// Steps 5–6 run even when the winner is unchanged (no-ops then), so re-observing
// the current winner never clears its primary flag.
func (s *Store) RecordObservationTx(ctx context.Context, q *postgres.Queries, ref IdentityRef, obs IdentifierObservation) (IdentifierObservation, error) {
	// 1. Serialize concurrent elections on this (entity, authority) group.
	if err := q.LockIdentityGroup(ctx, identityLockKey(ref, obs.Authority)); err != nil {
		return IdentifierObservation{}, fmt.Errorf("identity: acquire group lock: %w", err)
	}

	// 2. Upsert the observation without disturbing the primary slot.
	confidence, err := numericFromFloat(obs.Confidence)
	if err != nil {
		return IdentifierObservation{}, fmt.Errorf("identity: %w", err)
	}

	_, err = q.UpsertObservation(ctx, postgres.UpsertObservationParams{
		EntityType:           string(ref.Type),
		EntityID:             ref.ULID,
		AuthorityCode:        obs.Authority,
		IdentifierUri:        obs.URI,
		Confidence:           confidence,
		ReconciliationMethod: obs.Method,
		Metadata:             nil,
		Source:               pgtype.Text{String: obs.Source, Valid: obs.Source != ""},
	})
	if err != nil {
		return IdentifierObservation{}, fmt.Errorf("identity: upsert observation: %w", err)
	}

	// 3. Load the group's observations joined with authority trust/priority.
	rows, err := q.ListGroupIdentifiersForUpdate(ctx, postgres.ListGroupIdentifiersForUpdateParams{
		EntityType:    string(ref.Type),
		EntityID:      ref.ULID,
		AuthorityCode: obs.Authority,
	})
	if err != nil {
		return IdentifierObservation{}, fmt.Errorf("identity: load group: %w", err)
	}

	observations := make([]IdentifierObservation, 0, len(rows))
	for _, row := range rows {
		observations = append(observations, IdentifierObservation{
			ID:         row.ID,
			Authority:  row.AuthorityCode,
			URI:        row.IdentifierUri,
			Method:     row.ReconciliationMethod,
			Confidence: numericToFloat64(row.Confidence),
			ObservedAt: row.ObservedAt.Time,
			TrustLevel: row.TrustLevel,
			Priority:   row.PriorityOrder,
			IsPrimary:  row.IsPrimary,
			Source:     row.Source.String,
		})
	}

	// 4. Elect the winner.
	winner, ok := ElectPrimary(observations)
	if !ok {
		return IdentifierObservation{}, fmt.Errorf("identity: empty group after upsert")
	}

	// 5. Demote every primary except the winner, recording the superseding row.
	if err := q.DemotePrimaryAndSupersede(ctx, postgres.DemotePrimaryAndSupersedeParams{
		WinnerID:      pgtype.Int4{Int32: winner.ID, Valid: true},
		EntityType:    string(ref.Type),
		EntityID:      ref.ULID,
		AuthorityCode: obs.Authority,
	}); err != nil {
		return IdentifierObservation{}, fmt.Errorf("identity: demote primary: %w", err)
	}

	// 6. Unconditionally set the winner primary (clears its superseded_by_id).
	if err := q.SetPrimary(ctx, winner.ID); err != nil {
		return IdentifierObservation{}, fmt.Errorf("identity: set primary: %w", err)
	}

	winner.IsPrimary = true
	return winner, nil
}

// GetEntityIdentifiers returns all observations for an entity across authorities.
func (s *Store) GetEntityIdentifiers(ctx context.Context, ref IdentityRef) ([]IdentifierObservation, error) {
	q := postgres.New(s.pool)
	rows, err := q.GetEntityIdentifiers(ctx, postgres.GetEntityIdentifiersParams{
		EntityType: string(ref.Type),
		EntityID:   ref.ULID,
	})
	if err != nil {
		return nil, fmt.Errorf("identity: get entity identifiers: %w", err)
	}

	observations := make([]IdentifierObservation, 0, len(rows))
	for _, row := range rows {
		observations = append(observations, IdentifierObservation{
			ID:         row.ID,
			Authority:  row.AuthorityCode,
			URI:        row.IdentifierUri,
			Method:     row.ReconciliationMethod,
			Confidence: numericToFloat64(row.Confidence),
			ObservedAt: row.ObservedAt.Time,
			TrustLevel: row.TrustLevel,
			Priority:   row.PriorityOrder,
			IsPrimary:  row.IsPrimary,
			Source:     row.Source.String,
		})
	}
	return observations, nil
}

// identityLockKey builds the advisory-lock key for a (entity, authority) group.
func identityLockKey(ref IdentityRef, authority string) string {
	return string(ref.Type) + "|" + ref.ULID + "|" + authority
}

// numericFromFloat converts a float64 confidence to pgtype.Numeric.
func numericFromFloat(f float64) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	if err := n.Scan(fmt.Sprintf("%.6f", f)); err != nil {
		return n, fmt.Errorf("convert confidence: %w", err)
	}
	return n, nil
}

// numericToFloat64 converts a pgtype.Numeric to float64 (0 when invalid/NULL).
func numericToFloat64(n pgtype.Numeric) float64 {
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return 0
	}
	return f.Float64
}
