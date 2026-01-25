package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/Togather-Foundation/server/internal/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type APIKeyRepository struct {
	pool *pgxpool.Pool
	tx   pgx.Tx
}

func (r *APIKeyRepository) LookupByPrefix(ctx context.Context, prefix string) (*auth.APIKey, error) {
	queryer := r.queryer()
	row := queryer.QueryRow(ctx, `
SELECT id, prefix, key_hash, name, source_id, role, rate_limit_tier, is_active, expires_at
  FROM api_keys
 WHERE prefix = $1
 LIMIT 1
`, prefix)

	var data struct {
		ID            string
		Prefix        string
		Hash          string
		Name          string
		SourceID      *string
		Role          string
		RateLimitTier string
		IsActive      bool
		ExpiresAt     *time.Time
	}
	if err := row.Scan(
		&data.ID,
		&data.Prefix,
		&data.Hash,
		&data.Name,
		&data.SourceID,
		&data.Role,
		&data.RateLimitTier,
		&data.IsActive,
		&data.ExpiresAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, auth.ErrInvalidAPIKey
		}
		return nil, fmt.Errorf("lookup api key: %w", err)
	}

	key := &auth.APIKey{
		ID:            data.ID,
		Prefix:        data.Prefix,
		Hash:          data.Hash,
		Name:          data.Name,
		Role:          data.Role,
		RateLimitTier: data.RateLimitTier,
		IsActive:      data.IsActive,
		ExpiresAt:     data.ExpiresAt,
	}
	if data.SourceID != nil {
		key.SourceID = *data.SourceID
	}
	return key, nil
}

func (r *APIKeyRepository) UpdateLastUsed(ctx context.Context, id string) error {
	queryer := r.queryer()
	_, err := queryer.Exec(ctx, `UPDATE api_keys SET last_used_at = now() WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("update api key last used: %w", err)
	}
	return nil
}

type apiKeyQueryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (r *APIKeyRepository) queryer() apiKeyQueryer {
	if r.tx != nil {
		return r.tx
	}
	return r.pool
}
