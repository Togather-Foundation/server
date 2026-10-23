package postgres

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"
)

// insertLifecycleEvent inserts a minimally valid event in the given lifecycle
// state and returns its ID and database-created created_at. Inserting through
// the real table (not SQLc) exercises trg_events_first_published_at.
func insertLifecycleEvent(t *testing.T, ctx context.Context, pool *pgxpool.Pool, state string) (string, time.Time) {
	t.Helper()

	ulidValue := ulid.Make().String()
	var (
		id        string
		createdAt time.Time
	)
	err := pool.QueryRow(ctx, `
		INSERT INTO events (ulid, name, lifecycle_state, virtual_url)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`,
		ulidValue,
		"First Published Test "+ulidValue,
		state,
		"https://example.test/events/"+ulidValue,
	).Scan(&id, &createdAt)
	require.NoError(t, err)

	return id, createdAt
}

// insertLifecycleEventExplicit inserts an event with a caller-supplied
// first_published_at, exercising the trigger's "explicit value wins" path
// (imports/backfills).
func insertLifecycleEventExplicit(t *testing.T, ctx context.Context, pool *pgxpool.Pool, state string, firstPublished time.Time) (string, time.Time) {
	t.Helper()

	ulidValue := ulid.Make().String()
	var (
		id        string
		createdAt time.Time
	)
	err := pool.QueryRow(ctx, `
		INSERT INTO events (ulid, name, lifecycle_state, virtual_url, first_published_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at`,
		ulidValue,
		"First Published Explicit "+ulidValue,
		state,
		"https://example.test/events/"+ulidValue,
		firstPublished,
	).Scan(&id, &createdAt)
	require.NoError(t, err)

	return id, createdAt
}

// eventFirstPublishedAt reads first_published_at as a nullable time.
func eventFirstPublishedAt(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id string) *time.Time {
	t.Helper()

	var value pgtype.Timestamptz
	err := pool.QueryRow(ctx, `SELECT first_published_at FROM events WHERE id = $1`, id).Scan(&value)
	require.NoError(t, err)
	if !value.Valid {
		return nil
	}
	return &value.Time
}

func setEventLifecycle(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id, state string) {
	t.Helper()

	_, err := pool.Exec(ctx, `UPDATE events SET lifecycle_state = $2 WHERE id = $1`, id, state)
	require.NoError(t, err)
}

func countEventChanges(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID string) int {
	t.Helper()

	var count int
	err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM event_changes WHERE event_id = $1`, eventID).Scan(&count)
	require.NoError(t, err)
	return count
}

// TestEventFirstPublishedAtInsert seeds the trigger's INSERT branch: a public
// lifecycle state records created_at, system-only states stay NULL.
func TestEventFirstPublishedAtInsert(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)

	t.Run("public states seed created_at", func(t *testing.T) {
		for _, state := range []string{"published", "postponed", "rescheduled", "sold_out", "cancelled", "completed"} {
			t.Run(state, func(t *testing.T) {
				id, createdAt := insertLifecycleEvent(t, ctx, pool, state)
				got := eventFirstPublishedAt(t, ctx, pool, id)
				require.NotNil(t, got, "%s insert must stamp first_published_at", state)
				require.True(t, createdAt.Equal(*got),
					"%s insert must use created_at (%s), got %s", state, createdAt, got)
			})
		}
	})

	t.Run("system-only states stay NULL", func(t *testing.T) {
		for _, state := range []string{"draft", "pending_review", "deleted"} {
			t.Run(state, func(t *testing.T) {
				id, _ := insertLifecycleEvent(t, ctx, pool, state)
				require.Nil(t, eventFirstPublishedAt(t, ctx, pool, id),
					"%s insert must leave first_published_at NULL", state)
			})
		}
	})
}

// TestEventFirstPublishedAtTransitions covers the UPDATE branch: entering a
// public state stamps now(), leaving it preserves the original timestamp, and a
// value is never cleared.
func TestEventFirstPublishedAtTransitions(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)

	t.Run("draft to published stamps the transition", func(t *testing.T) {
		id, _ := insertLifecycleEvent(t, ctx, pool, "draft")
		require.Nil(t, eventFirstPublishedAt(t, ctx, pool, id))

		before := time.Now().Add(-time.Second)
		setEventLifecycle(t, ctx, pool, id, "published")

		got := eventFirstPublishedAt(t, ctx, pool, id)
		require.NotNil(t, got)
		require.True(t, got.After(before), "transition stamp must be current time, got %s", got)
	})

	t.Run("published to draft preserves the stamp", func(t *testing.T) {
		id, createdAt := insertLifecycleEvent(t, ctx, pool, "published")
		require.NotNil(t, eventFirstPublishedAt(t, ctx, pool, id))

		setEventLifecycle(t, ctx, pool, id, "draft")

		got := eventFirstPublishedAt(t, ctx, pool, id)
		require.NotNil(t, got, "unpublishing must not clear first_published_at")
		require.True(t, createdAt.Equal(*got), "unpublishing must preserve the insert stamp")
	})

	t.Run("draft to published to cancelled preserves the stamp", func(t *testing.T) {
		id, _ := insertLifecycleEvent(t, ctx, pool, "draft")
		setEventLifecycle(t, ctx, pool, id, "published")
		first := eventFirstPublishedAt(t, ctx, pool, id)
		require.NotNil(t, first)

		setEventLifecycle(t, ctx, pool, id, "cancelled")

		got := eventFirstPublishedAt(t, ctx, pool, id)
		require.NotNil(t, got)
		require.True(t, first.Equal(*got), "public-to-public transitions must not move the stamp")
	})

	t.Run("published to deleted preserves the stamp", func(t *testing.T) {
		id, createdAt := insertLifecycleEvent(t, ctx, pool, "published")
		setEventLifecycle(t, ctx, pool, id, "deleted")

		got := eventFirstPublishedAt(t, ctx, pool, id)
		require.NotNil(t, got, "deleting a previously-public event must keep its visibility signal")
		require.True(t, createdAt.Equal(*got))
	})

	t.Run("legacy public row with NULL seeds created_at on delete", func(t *testing.T) {
		id, createdAt := insertLifecycleEvent(t, ctx, pool, "published")

		// Simulate a row migrated from before the column existed: a public
		// lifecycle state with no recorded stamp.
		_, err := pool.Exec(ctx, `UPDATE events SET first_published_at = NULL WHERE id = $1`, id)
		require.NoError(t, err)
		require.Nil(t, eventFirstPublishedAt(t, ctx, pool, id))

		setEventLifecycle(t, ctx, pool, id, "deleted")

		got := eventFirstPublishedAt(t, ctx, pool, id)
		require.NotNil(t, got, "leaving a public state with a NULL stamp must seed it")
		require.True(t, createdAt.Equal(*got), "legacy rows seed from OLD.created_at, not now()")
	})
}

// TestEventFirstPublishedAtExplicitValue proves an explicit non-NULL value
// supplied by an import or backfill is never overwritten by the trigger.
func TestEventFirstPublishedAtExplicitValue(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)

	explicit := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Microsecond)

	t.Run("INSERT in a public state keeps the explicit value", func(t *testing.T) {
		id, _ := insertLifecycleEventExplicit(t, ctx, pool, "published", explicit)
		got := eventFirstPublishedAt(t, ctx, pool, id)
		require.NotNil(t, got)
		require.Equal(t, explicit, got.UTC())
	})

	t.Run("INSERT in a non-public state keeps the explicit value", func(t *testing.T) {
		id, _ := insertLifecycleEventExplicit(t, ctx, pool, "draft", explicit)
		got := eventFirstPublishedAt(t, ctx, pool, id)
		require.NotNil(t, got)
		require.Equal(t, explicit, got.UTC())
	})

	t.Run("UPDATE entering a public state keeps the explicit value", func(t *testing.T) {
		id, _ := insertLifecycleEvent(t, ctx, pool, "draft")
		require.Nil(t, eventFirstPublishedAt(t, ctx, pool, id))

		_, err := pool.Exec(ctx,
			`UPDATE events SET lifecycle_state = 'published', first_published_at = $2 WHERE id = $1`,
			id, explicit)
		require.NoError(t, err)

		got := eventFirstPublishedAt(t, ctx, pool, id)
		require.NotNil(t, got)
		require.Equal(t, explicit, got.UTC(), "an explicit transition value must not be replaced with now()")
	})

	t.Run("UPDATE leaving a public state keeps the explicit value", func(t *testing.T) {
		id, _ := insertLifecycleEvent(t, ctx, pool, "published")

		earlier := explicit.Add(-24 * time.Hour)
		_, err := pool.Exec(ctx,
			`UPDATE events SET lifecycle_state = 'draft', first_published_at = $2 WHERE id = $1`,
			id, earlier)
		require.NoError(t, err)

		got := eventFirstPublishedAt(t, ctx, pool, id)
		require.NotNil(t, got)
		require.Equal(t, earlier, got.UTC(), "an explicit value must win over the OLD.created_at seed")
	})
}

// TestEventFirstPublishedAtChangeLogNoise guards the AFTER UPDATE change-log
// trigger interaction: the BEFORE trigger must not touch NEW.* on a no-op
// update, or every touch would inject a synthetic public feed row.
func TestEventFirstPublishedAtChangeLogNoise(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)

	t.Run("no-op update emits zero change rows", func(t *testing.T) {
		id, _ := insertLifecycleEvent(t, ctx, pool, "published")
		before := countEventChanges(t, ctx, pool, id)

		_, err := pool.Exec(ctx, `UPDATE events SET name = name WHERE id = $1`, id)
		require.NoError(t, err)

		require.Equal(t, before, countEventChanges(t, ctx, pool, id),
			"a no-op update on a public row must not emit a change-feed row")
	})

	t.Run("lifecycle transition emits exactly one change row", func(t *testing.T) {
		id, _ := insertLifecycleEvent(t, ctx, pool, "draft")
		before := countEventChanges(t, ctx, pool, id)

		setEventLifecycle(t, ctx, pool, id, "published")

		require.Equal(t, before+1, countEventChanges(t, ctx, pool, id),
			"a lifecycle transition must emit exactly one change-feed row")
	})
}

// TestEventFirstPublishedAtBackfill exercises the migration's one-time backfill
// from event_changes public snapshots, including its idempotence and its
// trigger-disable guard.
func TestEventFirstPublishedAtBackfill(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)

	insertSnapshotAt := func(eventID, state string, changedAt time.Time) {
		t.Helper()
		snapshot, err := json.Marshal(map[string]string{"lifecycle_state": state})
		require.NoError(t, err)
		_, err = pool.Exec(ctx,
			`INSERT INTO event_changes (event_id, action, snapshot, changed_at) VALUES ($1, 'update', $2, $3)`,
			eventID, snapshot, changedAt)
		require.NoError(t, err)
	}

	// Legacy row with public history. The insert's own create row is removed so
	// the expected MIN is unambiguous.
	publicID, _ := insertLifecycleEvent(t, ctx, pool, "draft")
	_, err := pool.Exec(ctx, `DELETE FROM event_changes WHERE event_id = $1`, publicID)
	require.NoError(t, err)

	base := time.Now().UTC().Add(-96 * time.Hour).Truncate(time.Microsecond)
	firstPublic := base.Add(24 * time.Hour) // -72h: the expected backfill value
	insertSnapshotAt(publicID, "draft", base)
	insertSnapshotAt(publicID, "published", firstPublic)
	insertSnapshotAt(publicID, "draft", base.Add(48*time.Hour))
	insertSnapshotAt(publicID, "published", base.Add(72*time.Hour))

	// Legacy row that only ever had non-public snapshots: must stay NULL and
	// therefore fail closed on public surfaces.
	draftOnlyID, _ := insertLifecycleEvent(t, ctx, pool, "draft")
	_, err = pool.Exec(ctx, `DELETE FROM event_changes WHERE event_id = $1`, draftOnlyID)
	require.NoError(t, err)
	insertSnapshotAt(draftOnlyID, "draft", base)
	insertSnapshotAt(draftOnlyID, "pending_review", base.Add(30*time.Minute))

	changesBefore := countEventChanges(t, ctx, pool, publicID) + countEventChanges(t, ctx, pool, draftOnlyID)

	applyFirstPublishedAtBackfill(t, ctx, pool)

	got := eventFirstPublishedAt(t, ctx, pool, publicID)
	require.NotNil(t, got, "public history must backfill first_published_at")
	require.Equal(t, firstPublic, got.UTC(), "backfill must use MIN(changed_at) over public snapshots")

	require.Nil(t, eventFirstPublishedAt(t, ctx, pool, draftOnlyID),
		"rows with no public snapshot must stay NULL")

	require.Equal(t, changesBefore, countEventChanges(t, ctx, pool, publicID)+countEventChanges(t, ctx, pool, draftOnlyID),
		"the backfill must not inject synthetic change-feed rows")

	// Idempotence: a second run must neither move the stamp nor add rows.
	applyFirstPublishedAtBackfill(t, ctx, pool)

	again := eventFirstPublishedAt(t, ctx, pool, publicID)
	require.NotNil(t, again)
	require.Equal(t, firstPublic, again.UTC(), "running the backfill twice must be a no-op")
	require.Equal(t, changesBefore, countEventChanges(t, ctx, pool, publicID)+countEventChanges(t, ctx, pool, draftOnlyID),
		"a repeated backfill must not inject synthetic change-feed rows")
}

// applyFirstPublishedAtBackfill runs the migration's backfill statement with the
// events AFTER UPDATE change-log trigger disabled, exactly as 000055 does.
func applyFirstPublishedAtBackfill(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	_, err := pool.Exec(ctx, `ALTER TABLE events DISABLE TRIGGER trg_event_changes_update`)
	require.NoError(t, err)
	// Never leave the shared container's trigger disabled after a failure.
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `ALTER TABLE events ENABLE TRIGGER trg_event_changes_update`)
	})

	_, err = pool.Exec(ctx, `
		UPDATE events e
		   SET first_published_at = fp.first_public_at
		  FROM (
		    SELECT ec.event_id, MIN(ec.changed_at) AS first_public_at
		      FROM event_changes ec
		     WHERE event_is_public_state(ec.snapshot->>'lifecycle_state')
		     GROUP BY ec.event_id
		  ) fp
		 WHERE e.id = fp.event_id AND e.first_published_at IS NULL`)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `ALTER TABLE events ENABLE TRIGGER trg_event_changes_update`)
	require.NoError(t, err)
}
