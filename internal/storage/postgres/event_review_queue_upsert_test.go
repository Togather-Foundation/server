package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/oklog/ulid/v2"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

// isContentionError reports whether err is one of the transient outcomes the
// admin review path can surface when it races UpsertReviewRow's lock order.
func isContentionError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, events.ErrRetryable) || errors.Is(err, events.ErrConflict) {
		return true
	}
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "40P01" || pgErr.Code == "55P03" || pgErr.Code == "40001")
}

// upsertTestEvent creates a single-occurrence event in the requested lifecycle
// state. Unlike reviewQueueTestEvent it can create a published event, which is
// the state the review-row promotion paths act on.
func upsertTestEvent(t *testing.T, ctx context.Context, repo *EventRepository, place seededEntity, lifecycle string) *events.Event {
	t.Helper()

	eventULID := ulid.Make().String()
	event, err := repo.Create(ctx, events.EventCreateParams{
		ULID:           eventULID,
		Name:           fmt.Sprintf("Upsert Test Event %s", eventULID[:8]),
		Description:    "Event for atomic review-row tests",
		LifecycleState: lifecycle,
		EventDomain:    "arts",
		PrimaryVenueID: &place.ID,
		LicenseURL:     "https://creativecommons.org/publicdomain/zero/1.0/",
		LicenseStatus:  "cc0",
	})
	require.NoError(t, err)
	require.NotNil(t, event)

	require.NoError(t, repo.CreateOccurrence(ctx, events.OccurrenceCreateParams{
		EventID:   event.ID,
		StartTime: reviewQueueStart(),
		VenueID:   &place.ID,
		Timezone:  "America/Toronto",
	}))
	return event
}

func mustWarningsJSON(t *testing.T, warnings ...map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(warnings)
	require.NoError(t, err)
	return b
}

func warningCodes(t *testing.T, raw []byte) map[string]int {
	t.Helper()
	var warnings []map[string]any
	require.NoError(t, json.Unmarshal(raw, &warnings))
	codes := make(map[string]int, len(warnings))
	for _, w := range warnings {
		code, _ := w["code"].(string)
		codes[code]++
	}
	return codes
}

func TestUpsertReviewRow_ConcurrentWritersCreateOneRowAndMergeWarnings(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	repo := &EventRepository{pool: pool, logger: zerolog.Nop()}
	place := insertPlace(t, ctx, pool, "Upsert Venue", "Toronto", "ON")
	event := upsertTestEvent(t, ctx, repo, place, "published")

	type outcome struct {
		result *events.ReviewQueueUpsertResult
		err    error
	}
	start := make(chan struct{})
	outcomes := make(chan outcome, 2)

	var wg sync.WaitGroup
	for _, warnings := range [][]byte{
		mustWarningsJSON(t,
			map[string]any{"field": "name", "code": "near_duplicate_of_new_event", "message": "first writer"},
		),
		mustWarningsJSON(t,
			map[string]any{"field": "name", "code": "cross_week_series_companion", "message": "second writer"},
		),
	} {
		warnings := warnings
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			txRepo, tx, err := repo.BeginTx(ctx)
			if err != nil {
				outcomes <- outcome{err: err}
				return
			}
			result, err := txRepo.UpsertReviewRow(ctx, events.ReviewQueueUpsertParams{
				EventULID:         event.ULID,
				OriginalPayload:   []byte(`{"name":"Upsert Test Event"}`),
				NormalizedPayload: []byte(`{"name":"Upsert Test Event"}`),
				EventStartTime:    reviewQueueStart(),
				EventAllDay:       false,
				Warnings:          warnings,
				WarningMode:       events.ReviewWarningMerge,
				Promotion:         events.ReviewPromotionInserted,
			})
			if err != nil {
				_ = tx.Rollback(ctx)
				outcomes <- outcome{err: err}
				return
			}
			if err := tx.Commit(ctx); err != nil {
				outcomes <- outcome{err: err}
				return
			}
			outcomes <- outcome{result: result}
		}()
	}
	close(start)
	wg.Wait()
	close(outcomes)

	var inserted, promoted int
	for o := range outcomes {
		require.NoError(t, o.err)
		require.NotNil(t, o.result)
		if o.result.Inserted {
			inserted++
		}
		if o.result.Promoted {
			promoted++
		}
	}
	require.Equal(t, 1, inserted, "exactly one writer must insert")
	require.Equal(t, 1, promoted, "exactly one writer must flip the lifecycle")

	var rowCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM event_review_queue WHERE event_id = $1`, event.ID).Scan(&rowCount))
	require.Equal(t, 1, rowCount, "event_review_queue.event_id is UNIQUE")

	entry, err := repo.GetReviewByEventUlid(ctx, event.ULID)
	require.NoError(t, err)
	require.NotNil(t, entry)
	codes := warningCodes(t, entry.Warnings)
	require.Equal(t, 1, codes["near_duplicate_of_new_event"], "first writer's warning must survive")
	require.Equal(t, 1, codes["cross_week_series_companion"], "second writer's warning must survive")

	var lifecycle string
	require.NoError(t, pool.QueryRow(ctx, `SELECT lifecycle_state FROM events WHERE id = $1`, event.ID).Scan(&lifecycle))
	require.Equal(t, "pending_review", lifecycle)
}

func TestUpsertReviewRow_DecidedRowKeepsStatusAndLifecycle(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	repo := &EventRepository{pool: pool, logger: zerolog.Nop()}
	place := insertPlace(t, ctx, pool, "Decided Venue", "Toronto", "ON")
	event := upsertTestEvent(t, ctx, repo, place, "published")

	entry, err := repo.CreateReviewQueueEntry(ctx, events.ReviewQueueCreateParams{
		EventID:           event.ID,
		OriginalPayload:   []byte(`{"name":"original"}`),
		NormalizedPayload: []byte(`{"name":"original"}`),
		Warnings: mustWarningsJSON(t,
			map[string]any{"field": "name", "code": "potential_duplicate", "message": "old"},
		),
		EventStartTime: reviewQueueStart(),
	})
	require.NoError(t, err)
	_, err = repo.ApproveReview(ctx, entry.ID, "admin", nil)
	require.NoError(t, err)

	txRepo, tx, err := repo.BeginTx(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()

	result, err := txRepo.UpsertReviewRow(ctx, events.ReviewQueueUpsertParams{
		EventULID:         event.ULID,
		OriginalPayload:   []byte(`{"name":"refreshed"}`),
		NormalizedPayload: []byte(`{"name":"refreshed"}`),
		EventStartTime:    reviewQueueStart(),
		Warnings: mustWarningsJSON(t,
			map[string]any{"field": "name", "code": "near_duplicate_of_new_event", "message": "new"},
		),
		WarningMode:    events.ReviewWarningMerge,
		RefreshPayload: true,
		Promotion:      events.ReviewPromotionInserted,
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
	require.False(t, result.Inserted)
	require.False(t, result.Promoted)

	updated, err := repo.GetReviewByEventUlid(ctx, event.ULID)
	require.NoError(t, err)
	require.Equal(t, "approved", updated.Status, "decided rows keep their status")
	codes := warningCodes(t, updated.Warnings)
	require.Equal(t, 1, codes["potential_duplicate"])
	require.Equal(t, 1, codes["near_duplicate_of_new_event"])

	var lifecycle string
	require.NoError(t, pool.QueryRow(ctx, `SELECT lifecycle_state FROM events WHERE id = $1`, event.ID).Scan(&lifecycle))
	require.Equal(t, "published", lifecycle, "a decided row must not hide a live event")
}

func TestUpsertReviewRow_ExistingPendingRowWithInsertedPromotionLeavesLifecycle(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	repo := &EventRepository{pool: pool, logger: zerolog.Nop()}
	place := insertPlace(t, ctx, pool, "Pending Row Venue", "Toronto", "ON")
	event := upsertTestEvent(t, ctx, repo, place, "published")

	_, err := repo.CreateReviewQueueEntry(ctx, events.ReviewQueueCreateParams{
		EventID:           event.ID,
		OriginalPayload:   []byte(`{"name":"original"}`),
		NormalizedPayload: []byte(`{"name":"original"}`),
		Warnings:          []byte(`[{"field":"name","code":"cross_week_series_companion","message":"existing"}]`),
		EventStartTime:    reviewQueueStart(),
	})
	require.NoError(t, err)

	txRepo, tx, err := repo.BeginTx(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()

	result, err := txRepo.UpsertReviewRow(ctx, events.ReviewQueueUpsertParams{
		EventULID:         event.ULID,
		OriginalPayload:   []byte(`{"name":"new"}`),
		NormalizedPayload: []byte(`{"name":"new"}`),
		EventStartTime:    reviewQueueStart(),
		Warnings:          []byte(`[{"field":"name","code":"near_duplicate_of_new_event","message":"new"}]`),
		WarningMode:       events.ReviewWarningMerge,
		Promotion:         events.ReviewPromotionInserted,
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
	require.False(t, result.Inserted)
	require.False(t, result.Promoted)

	var lifecycle string
	require.NoError(t, pool.QueryRow(ctx, `SELECT lifecycle_state FROM events WHERE id = $1`, event.ID).Scan(&lifecycle))
	require.Equal(t, "published", lifecycle, "an existing row leaves the lifecycle untouched")
}

func TestUpsertReviewRow_ReplaceCodeReplacesOnlyMatchingCodes(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	repo := &EventRepository{pool: pool, logger: zerolog.Nop()}
	place := insertPlace(t, ctx, pool, "Replace Code Venue", "Toronto", "ON")
	event := upsertTestEvent(t, ctx, repo, place, "pending_review")

	_, err := repo.CreateReviewQueueEntry(ctx, events.ReviewQueueCreateParams{
		EventID:           event.ID,
		OriginalPayload:   []byte(`{"name":"original"}`),
		NormalizedPayload: []byte(`{"name":"original"}`),
		Warnings: mustWarningsJSON(t,
			map[string]any{"field": "name", "code": "cross_week_series_companion", "message": "stale"},
			map[string]any{"field": "name", "code": "potential_duplicate", "message": "keep"},
		),
		EventStartTime: reviewQueueStart(),
	})
	require.NoError(t, err)

	txRepo, tx, err := repo.BeginTx(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = txRepo.UpsertReviewRow(ctx, events.ReviewQueueUpsertParams{
		EventULID:         event.ULID,
		OriginalPayload:   []byte(`{"name":"new"}`),
		NormalizedPayload: []byte(`{"name":"new"}`),
		EventStartTime:    reviewQueueStart(),
		Warnings: mustWarningsJSON(t,
			map[string]any{"field": "name", "code": "cross_week_series_companion", "message": "fresh"},
		),
		WarningMode: events.ReviewWarningReplaceCode,
		Promotion:   events.ReviewPromotionNone,
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))

	updated, err := repo.GetReviewByEventUlid(ctx, event.ULID)
	require.NoError(t, err)
	codes := warningCodes(t, updated.Warnings)
	require.Equal(t, 1, codes["cross_week_series_companion"], "stale cross-week warning must be replaced")
	require.Equal(t, 1, codes["potential_duplicate"], "unrelated warning must be kept")
	var warnings []map[string]any
	require.NoError(t, json.Unmarshal(updated.Warnings, &warnings))
	for _, w := range warnings {
		if w["code"] == "cross_week_series_companion" {
			require.Equal(t, "fresh", w["message"])
		}
	}
}

func TestUpsertReviewRow_LockTimeoutIsRetryableAndWritesNothing(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	repo := &EventRepository{pool: pool, logger: zerolog.Nop()}
	fastRepo := &EventRepository{pool: pool, logger: zerolog.Nop(), reviewRowLockTimeout: "100ms"}
	place := insertPlace(t, ctx, pool, "Timeout Venue", "Toronto", "ON")
	event := upsertTestEvent(t, ctx, repo, place, "published")

	blocker, blockerTx, err := repo.BeginTx(ctx)
	require.NoError(t, err)
	defer func() { _ = blockerTx.Rollback(ctx) }()
	require.NoError(t, blocker.LockEventForUpdate(ctx, event.ID))

	contender, contenderTx, err := fastRepo.BeginTx(ctx)
	require.NoError(t, err)
	defer func() { _ = contenderTx.Rollback(ctx) }()

	_, err = contender.UpsertReviewRow(ctx, events.ReviewQueueUpsertParams{
		EventULID:         event.ULID,
		OriginalPayload:   []byte(`{"name":"blocked"}`),
		NormalizedPayload: []byte(`{"name":"blocked"}`),
		EventStartTime:    reviewQueueStart(),
		Warnings:          []byte(`[{"field":"name","code":"x","message":"y"}]`),
		WarningMode:       events.ReviewWarningMerge,
		Promotion:         events.ReviewPromotionInserted,
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, events.ErrRetryable), "lock timeout must be retryable, got %v", err)

	require.NoError(t, contenderTx.Rollback(ctx))
	require.NoError(t, blockerTx.Rollback(ctx))

	var rowCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM event_review_queue WHERE event_id = $1`, event.ID).Scan(&rowCount))
	require.Zero(t, rowCount, "the timed-out transaction must write nothing")

	var lifecycle string
	require.NoError(t, pool.QueryRow(ctx, `SELECT lifecycle_state FROM events WHERE id = $1`, event.ID).Scan(&lifecycle))
	require.Equal(t, "published", lifecycle)
}

// TestUpsertReviewRow_ApproveInterleavingKeepsInvariant exercises the
// consolidation writer against a concurrent admin approval. The admin actions
// use review-row-first locking while UpsertReviewRow locks the event first, so
// PostgreSQL may detect a deadlock; either side may lose, but the loser must
// roll back whole. The invariant under test is the one the ticket calls out:
// an event is never left pending_review without a pending queue row.
func TestUpsertReviewRow_ApproveInterleavingKeepsInvariant(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	repo := &EventRepository{pool: pool, logger: zerolog.Nop()}
	place := insertPlace(t, ctx, pool, "Interleave Venue", "Toronto", "ON")
	admin := events.NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{
		MaxEventNameLength: 500,
	}, "https://toronto.togather.foundation", zerolog.Nop())

	for i := 0; i < 5; i++ {
		event := upsertTestEvent(t, ctx, repo, place, "pending_review")
		entry, err := repo.CreateReviewQueueEntry(ctx, events.ReviewQueueCreateParams{
			EventID:           event.ID,
			OriginalPayload:   []byte(`{"name":"original"}`),
			NormalizedPayload: []byte(`{"name":"original"}`),
			Warnings:          []byte(`[{"field":"name","code":"potential_duplicate","message":"existing"}]`),
			EventStartTime:    reviewQueueStart(),
		})
		require.NoError(t, err)

		start := make(chan struct{})
		var wg sync.WaitGroup
		var approveErr, upsertErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, approveErr = admin.ApproveEventWithReview(ctx, event.ULID, entry.ID, "admin", nil)
		}()
		go func() {
			defer wg.Done()
			<-start
			txRepo, tx, err := repo.BeginTx(ctx)
			if err != nil {
				upsertErr = err
				return
			}
			_, upsertErr = txRepo.UpsertReviewRow(ctx, events.ReviewQueueUpsertParams{
				EventULID:         event.ULID,
				OriginalPayload:   []byte(`{"name":"consolidated"}`),
				NormalizedPayload: []byte(`{"name":"consolidated"}`),
				EventStartTime:    reviewQueueStart(),
				Warnings:          []byte(`[{"field":"name","code":"cross_week_series_companion","message":"fresh"}]`),
				WarningMode:       events.ReviewWarningMerge,
				Promotion:         events.ReviewPromotionPending,
			})
			if upsertErr != nil {
				_ = tx.Rollback(ctx)
				return
			}
			upsertErr = tx.Commit(ctx)
		}()
		close(start)
		wg.Wait()

		if approveErr != nil {
			require.True(t, isContentionError(approveErr), "approve error: %v", approveErr)
		}
		if upsertErr != nil {
			require.True(t, isContentionError(upsertErr), "upsert error: %v", upsertErr)
		}

		var lifecycle string
		require.NoError(t, pool.QueryRow(ctx, `SELECT lifecycle_state FROM events WHERE id = $1`, event.ID).Scan(&lifecycle))
		var pendingRows int
		require.NoError(t, pool.QueryRow(ctx, `
SELECT COUNT(*) FROM event_review_queue
 WHERE event_id = $1 AND status = 'pending'`, event.ID).Scan(&pendingRows))
		if lifecycle == "pending_review" {
			require.Equal(t, 1, pendingRows, "pending_review must always have a pending queue row")
		}
		if lifecycle == "published" {
			require.Zero(t, pendingRows, "a published event must not keep a pending row")
		}
	}
}
