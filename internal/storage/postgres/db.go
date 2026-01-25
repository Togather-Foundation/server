package postgres

import (
	"context"
	"fmt"

	"github.com/Togather-Foundation/server/internal/storage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
	tx   pgx.Tx
}

func NewRepository(pool *pgxpool.Pool) (*Repository, error) {
	if pool == nil {
		return nil, fmt.Errorf("postgres repository: pool is nil")
	}
	return &Repository{pool: pool}, nil
}

func (r *Repository) Events() storage.EventRepository {
	return &EventRepository{pool: r.pool, tx: r.tx}
}

func (r *Repository) Places() storage.PlaceRepository {
	return &PlaceRepository{pool: r.pool, tx: r.tx}
}

func (r *Repository) Organizations() storage.OrganizationRepository {
	return &OrganizationRepository{pool: r.pool, tx: r.tx}
}

func (r *Repository) Sources() storage.SourceRepository {
	return &SourceRepository{pool: r.pool, tx: r.tx}
}

func (r *Repository) Provenance() storage.ProvenanceRepository {
	return &ProvenanceRepository{pool: r.pool, tx: r.tx}
}

func (r *Repository) Federation() storage.FederationRepository {
	return &FederationRepository{pool: r.pool, tx: r.tx}
}

func (r *Repository) Auth() storage.AuthRepository {
	return &AuthRepository{pool: r.pool, tx: r.tx}
}

func (r *Repository) WithTx(ctx context.Context, fn func(context.Context, storage.Repository) error) error {
	if r.tx != nil {
		return fn(ctx, r)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	wrapped := &Repository{pool: r.pool, tx: tx}
	if err := fn(ctx, wrapped); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

type EventRepository struct {
	pool *pgxpool.Pool
	tx   pgx.Tx
}

type PlaceRepository struct {
	pool *pgxpool.Pool
	tx   pgx.Tx
}

type OrganizationRepository struct {
	pool *pgxpool.Pool
	tx   pgx.Tx
}

type SourceRepository struct {
	pool *pgxpool.Pool
	tx   pgx.Tx
}

type ProvenanceRepository struct {
	pool *pgxpool.Pool
	tx   pgx.Tx
}

type FederationRepository struct {
	pool *pgxpool.Pool
	tx   pgx.Tx
}

type AuthRepository struct {
	pool *pgxpool.Pool
	tx   pgx.Tx
}
