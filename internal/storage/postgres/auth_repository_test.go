package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/auth"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyRepositoryLookupByPrefixAndUpdate(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)

	repo := &APIKeyRepository{pool: pool}

	_, err := pool.Exec(ctx, `
INSERT INTO api_keys (prefix, key_hash, hash_version, name, role, rate_limit_tier, is_active, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
`, "tok_123", "hash", 2, "Test Key", "agent", "agent", true, time.Now().Add(24*time.Hour))
	require.NoError(t, err)

	key, err := repo.LookupByPrefix(ctx, "tok_123")
	require.NoError(t, err)
	require.Equal(t, "tok_123", key.Prefix)
	require.Equal(t, "hash", key.Hash)
	require.Equal(t, "agent", key.Role)
	require.Equal(t, "agent", key.RateLimitTier)

	err = repo.UpdateLastUsed(ctx, key.ID)
	require.NoError(t, err)

	var lastUsed time.Time
	err = pool.QueryRow(ctx, `SELECT last_used_at FROM api_keys WHERE id = $1`, key.ID).Scan(&lastUsed)
	require.NoError(t, err)
	require.False(t, lastUsed.IsZero())
}

func TestAPIKeyRepositoryLookupMissing(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)

	repo := &APIKeyRepository{pool: pool}

	key, err := repo.LookupByPrefix(ctx, "missing")
	require.ErrorIs(t, err, auth.ErrInvalidAPIKey)
	require.Nil(t, key)
}
