package identity

import (
	"context"
	"errors"
	"fmt"

	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NotDuplicateStore reads and writes the identity_not_duplicates suppression
// ledger. The write half is tx-scoped so the executor can commit it atomically
// with the decision append; the read half lets the read path (Task 5) compare a
// stored evidence fingerprint to the current Fingerprint.
type NotDuplicateStore interface {
	// GetNotDuplicate returns the stored evidence fingerprint for a pair,
	// canonicalizing the two ULIDs. found is false when the pair is not
	// suppressed.
	GetNotDuplicate(ctx context.Context, entityType EntityType, a, b string) (evidenceFingerprint string, found bool, err error)
	// UpsertNotDuplicateTx records or refreshes a not-duplicate pair inside the
	// caller's transaction, canonicalizing the two ULIDs.
	UpsertNotDuplicateTx(ctx context.Context, q *postgres.Queries, entityType EntityType, a, b, evidenceFingerprint, decisionID, actor string) error
}

// notDuplicateStore is the concrete NotDuplicateStore backed by a pgx pool.
type notDuplicateStore struct {
	pool *pgxpool.Pool
}

// NewNotDuplicateStore creates a NotDuplicateStore from a pgx connection pool.
func NewNotDuplicateStore(pool *pgxpool.Pool) NotDuplicateStore {
	return &notDuplicateStore{pool: pool}
}

var _ NotDuplicateStore = (*notDuplicateStore)(nil)

// GetNotDuplicate returns the stored evidence fingerprint for a canonicalized
// pair, or found=false when the pair has no suppression row.
func (s *notDuplicateStore) GetNotDuplicate(ctx context.Context, entityType EntityType, a, b string) (string, bool, error) {
	idA, idB := canonicalPair(a, b)
	row, err := postgres.New(s.pool).GetIdentityNotDuplicate(ctx, postgres.GetIdentityNotDuplicateParams{
		EntityType: string(entityType),
		IDA:        idA,
		IDB:        idB,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("identity: get not-duplicate: %w", err)
	}
	return row.EvidenceFingerprint, true, nil
}

// UpsertNotDuplicateTx records or refreshes a not-duplicate pair inside the
// caller's transaction. Re-recording after an evidence change overwrites the
// stored fingerprint, re-opening the pair.
func (s *notDuplicateStore) UpsertNotDuplicateTx(ctx context.Context, q *postgres.Queries, entityType EntityType, a, b, evidenceFingerprint, decisionID, actor string) error {
	idA, idB := canonicalPair(a, b)
	if err := q.InsertIdentityNotDuplicate(ctx, postgres.InsertIdentityNotDuplicateParams{
		EntityType:          string(entityType),
		IDA:                 idA,
		IDB:                 idB,
		EvidenceFingerprint: evidenceFingerprint,
		DecisionID:          pgtype.Text{String: decisionID, Valid: decisionID != ""},
		CreatedBy:           actor,
	}); err != nil {
		return fmt.Errorf("identity: record not-duplicate: %w", err)
	}
	return nil
}
