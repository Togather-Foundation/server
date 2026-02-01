package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/federation"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestChangeFeedRepositoryListEventChanges(t *testing.T) {
	ctx := context.Background()
	container, dbURL := setupPostgres(t, ctx)
	defer func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}()

	pool, err := pgxpool.New(ctx, dbURL)
	require.NoError(t, err)
	defer pool.Close()

	queries := New(pool)
	repo := NewChangeFeedRepository(queries)

	org := insertOrganization(t, ctx, pool, "Toronto Arts Org")
	place := insertPlace(t, ctx, pool, "Centennial Park", "Toronto", "ON")
	start := time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC)
	ulidValue := insertEvent(t, ctx, pool, "Jazz in the Park", "Live jazz", org, place, "music", "published", []string{"jazz"}, start)

	var eventID string
	err = pool.QueryRow(ctx, `SELECT id FROM events WHERE ulid = $1`, ulidValue).Scan(&eventID)
	require.NoError(t, err)

	rows, err := repo.ListEventChanges(ctx, federation.ListEventChangesParams{
		AfterSequence: 0,
		Limit:         10,
	})
	require.NoError(t, err)
	require.NotEmpty(t, rows)
	require.Equal(t, ulidValue, rows[0].EventUlid)
}
