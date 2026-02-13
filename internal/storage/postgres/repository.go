package postgres

import (
	"context"
	"fmt"

	"github.com/Togather-Foundation/server/internal/storage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository implements storage.Repository interface with PostgreSQL backend
type Repository struct {
	pool *pgxpool.Pool
	tx   pgx.Tx

	events        *EventRepository
	places        *PlaceRepository
	organizations *OrganizationRepository
	provenance    *ProvenanceRepository
	federation    *FederationRepository
	auth          *AuthRepository
	developers    *DeveloperRepositoryAdapter
}

// NewRepository creates a new PostgreSQL-backed repository
func NewRepository(pool *pgxpool.Pool) (*Repository, error) {
	if pool == nil {
		return nil, fmt.Errorf("pool cannot be nil")
	}

	return &Repository{
		pool:          pool,
		events:        &EventRepository{pool: pool},
		places:        &PlaceRepository{pool: pool},
		organizations: &OrganizationRepository{pool: pool},
		provenance:    &ProvenanceRepository{pool: pool},
		federation:    NewFederationRepository(pool),
		auth:          &AuthRepository{pool: pool},
		developers:    NewDeveloperRepositoryAdapter(pool),
	}, nil
}

// Events returns the events repository
func (r *Repository) Events() storage.EventRepository {
	if r.tx != nil {
		return &EventRepository{pool: r.pool, tx: r.tx}
	}
	return r.events
}

// Places returns the places repository
func (r *Repository) Places() storage.PlaceRepository {
	if r.tx != nil {
		return &PlaceRepository{pool: r.pool, tx: r.tx}
	}
	return r.places
}

// Organizations returns the organizations repository
func (r *Repository) Organizations() storage.OrganizationRepository {
	if r.tx != nil {
		return &OrganizationRepository{pool: r.pool, tx: r.tx}
	}
	return r.organizations
}

// Sources returns the sources repository (placeholder)
func (r *Repository) Sources() storage.SourceRepository {
	return nil // TODO: implement
}

// Provenance returns the provenance repository
func (r *Repository) Provenance() storage.ProvenanceRepository {
	if r.tx != nil {
		return &ProvenanceRepository{pool: r.pool, tx: r.tx}
	}
	return r.provenance
}

// Federation returns the federation repository
func (r *Repository) Federation() storage.FederationRepository {
	if r.tx != nil {
		return NewFederationRepository(r.pool)
	}
	return r.federation
}

// Auth returns the auth repository
func (r *Repository) Auth() storage.AuthRepository {
	if r.tx != nil {
		return &AuthRepository{pool: r.pool, tx: r.tx}
	}
	return r.auth
}

// Developers returns the developers repository
func (r *Repository) Developers() storage.DeveloperRepository {
	// Note: Developer repository handles its own transactions via BeginTx
	// so we don't need to support WithTx pattern here
	return r.developers
}

// WithTx executes a function within a database transaction
func (r *Repository) WithTx(ctx context.Context, fn func(context.Context, storage.Repository) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	txRepo := &Repository{
		pool:          r.pool,
		tx:            tx,
		events:        &EventRepository{pool: r.pool, tx: tx},
		places:        &PlaceRepository{pool: r.pool, tx: tx},
		organizations: &OrganizationRepository{pool: r.pool, tx: tx},
		provenance:    &ProvenanceRepository{pool: r.pool, tx: tx},
		federation:    NewFederationRepository(r.pool),
		auth:          &AuthRepository{pool: r.pool, tx: tx},
		developers:    r.developers, // Developer repo handles its own transactions
	}

	if err := fn(ctx, txRepo); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("rollback after error %v: %w", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// AuthRepository wraps API key operations
type AuthRepository struct {
	pool *pgxpool.Pool
	tx   pgx.Tx
}

// APIKeys returns the API key repository
func (r *AuthRepository) APIKeys() storage.APIKeyRepository {
	return &APIKeyRepository{pool: r.pool, tx: r.tx}
}
