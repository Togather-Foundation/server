package postgres

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const feedIdentityIndex = "idx_event_sources_feed_identity"

// TestFeedIdentityIndex guards migration 000058. The feed-scoped identity lookup
// (EventRepository.FindByFeedEventID) predicates on source_event_id and
// rtrim(source_url, '/'). Without this index the planner reads every page of
// idx_event_sources_external_id and filters, once per ingested event.
func TestFeedIdentityIndex(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)

	t.Run("exists with the expression the query uses", func(t *testing.T) {
		var def string
		err := pool.QueryRow(ctx, `
SELECT indexdef FROM pg_indexes
 WHERE schemaname = 'public' AND tablename = 'event_sources' AND indexname = $1
`, feedIdentityIndex).Scan(&def)
		require.NoError(t, err, "migration 000058 must create %s", feedIdentityIndex)

		// The expression has to match EventRepository.FindByFeedEventID verbatim;
		// a different spelling would leave the query unable to use the index.
		require.Contains(t, def, "btree")
		require.Contains(t, def, "source_event_id")
		require.Contains(t, def, "rtrim(source_url, '/'::text)")
		require.Less(t, strings.Index(def, "source_event_id"), strings.Index(def, "rtrim"), "source_event_id must lead: it is the selective column")

		var valid bool
		var ready bool
		require.NoError(t, pool.QueryRow(ctx, `
SELECT i.indisvalid, i.indisready
  FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
 WHERE c.relname = $1
`, feedIdentityIndex).Scan(&valid, &ready))
		require.True(t, valid, "index must be valid (a failed CREATE INDEX leaves it invalid)")
		require.True(t, ready)
	})

	t.Run("planner can use it for the identity lookup", func(t *testing.T) {
		// The shared test database is tiny, so the planner would take a seq scan
		// on cost alone, and idx_event_sources_external_id offers a full-index-scan
		// fallback. Drop that competitor inside a rolled-back transaction so the
		// only remaining option proves the new index is actually applicable.
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback(ctx) }()

		_, err = tx.Exec(ctx, `DROP INDEX IF EXISTS idx_event_sources_external_id`)
		require.NoError(t, err)
		_, err = tx.Exec(ctx, `SET LOCAL enable_seqscan = off`)
		require.NoError(t, err)

		rows, err := tx.Query(ctx, `
EXPLAIN SELECT e.id
  FROM events e
  JOIN event_sources es ON es.event_id = e.id
 WHERE rtrim(es.source_url, '/') = rtrim($1, '/') AND es.source_event_id = $2
`, "https://example.org/events/", "evt-1")
		require.NoError(t, err)

		var plan strings.Builder
		for rows.Next() {
			var line string
			require.NoError(t, rows.Scan(&line))
			plan.WriteString(line)
			plan.WriteString("\n")
		}
		require.NoError(t, rows.Err())

		require.Contains(t, plan.String(), feedIdentityIndex,
			"the identity query must be able to use the new index; plan was:\n%s", plan.String())
	})

	t.Run("down migration drops the index", func(t *testing.T) {
		sql, err := os.ReadFile(filepath.Join(projectRoot(), DefaultMigrationsPath, "000058_event_sources_feed_identity_index.down.sql"))
		require.NoError(t, err)

		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback(ctx) }()

		_, err = tx.Exec(ctx, string(sql))
		require.NoError(t, err, "down migration must be executable")

		var exists bool
		require.NoError(t, tx.QueryRow(ctx, `
SELECT EXISTS (
    SELECT 1 FROM pg_indexes
     WHERE schemaname = 'public' AND tablename = 'event_sources' AND indexname = $1
)
`, feedIdentityIndex).Scan(&exists))
		require.False(t, exists, "down migration must drop %s", feedIdentityIndex)
	})
}
