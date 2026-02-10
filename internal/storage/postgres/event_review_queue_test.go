package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertJSONEqual compares two JSON byte slices semantically, ignoring formatting differences.
// PostgreSQL JSONB normalization adds spaces after colons, so we need semantic comparison.
func assertJSONEqual(t *testing.T, expected, actual []byte, msgAndArgs ...interface{}) bool {
	t.Helper()
	var expectedData, actualData interface{}
	if err := json.Unmarshal(expected, &expectedData); err != nil {
		t.Errorf("Failed to unmarshal expected JSON: %v", err)
		return false
	}
	if err := json.Unmarshal(actual, &actualData); err != nil {
		t.Errorf("Failed to unmarshal actual JSON: %v", err)
		return false
	}
	return assert.Equal(t, expectedData, actualData, msgAndArgs...)
}

// reviewQueueTestEvent creates a unique event suitable for review queue testing.
// Each call creates a new event with a unique ULID, avoiding duplicate key violations
// on the event_review_queue.event_id unique constraint.
func reviewQueueTestEvent(t *testing.T, ctx context.Context, repo *EventRepository, place seededEntity) (eventID string, startTime time.Time) {
	t.Helper()

	eventULID := ulid.Make().String()
	startTime = time.Date(2025, 3, 31, 23, 0, 0, 0, time.UTC)

	eventParams := events.EventCreateParams{
		ULID:           eventULID,
		Name:           fmt.Sprintf("Test Event %s", eventULID[:8]),
		Description:    "Test event for review queue",
		LifecycleState: "pending_review",
		EventDomain:    "arts",
		PrimaryVenueID: &place.ID,
		LicenseURL:     "https://creativecommons.org/publicdomain/zero/1.0/",
		LicenseStatus:  "cc0",
	}
	event, err := repo.Create(ctx, eventParams)
	require.NoError(t, err)
	require.NotNil(t, event)

	occParams := events.OccurrenceCreateParams{
		EventID:   event.ID,
		StartTime: startTime,
		VenueID:   &place.ID,
		Timezone:  "America/Toronto",
	}
	err = repo.CreateOccurrence(ctx, occParams)
	require.NoError(t, err)

	return event.ID, startTime
}

func TestEventRepository_ReviewQueue(t *testing.T) {
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

	repo := &EventRepository{pool: pool}

	// Create test venue (shared across subtests)
	place := insertPlace(t, ctx, pool, "Test Venue", "Toronto", "ON")

	// Shared test data templates
	originalPayload, _ := json.Marshal(map[string]interface{}{
		"name":      "Test Event",
		"startDate": "2025-03-31T23:00:00Z",
		"endDate":   "2025-03-31T02:00:00Z",
	})

	normalizedPayload, _ := json.Marshal(map[string]interface{}{
		"name":      "Test Event",
		"startDate": "2025-03-31T23:00:00Z",
		"endDate":   "2025-04-01T02:00:00Z",
	})

	warnings, _ := json.Marshal([]map[string]interface{}{
		{
			"field":   "endDate",
			"code":    "reversed_dates_timezone_likely",
			"message": "endDate was 21h before startDate",
		},
	})

	sourceID := "test-source"
	externalID := "ext-123"
	dedupHash := "hash-abc-123"
	endTime := time.Date(2025, 4, 1, 2, 0, 0, 0, time.UTC)

	t.Run("CreateReviewQueueEntry", func(t *testing.T) {
		eventID, startTime := reviewQueueTestEvent(t, ctx, repo, place)

		params := events.ReviewQueueCreateParams{
			EventID:           eventID,
			OriginalPayload:   originalPayload,
			NormalizedPayload: normalizedPayload,
			Warnings:          warnings,
			SourceID:          &sourceID,
			SourceExternalID:  &externalID,
			DedupHash:         &dedupHash,
			EventStartTime:    startTime,
			EventEndTime:      &endTime,
		}

		entry, err := repo.CreateReviewQueueEntry(ctx, params)
		require.NoError(t, err)
		require.NotNil(t, entry)

		assert.Greater(t, entry.ID, 0)
		assert.Equal(t, eventID, entry.EventID)
		assertJSONEqual(t, originalPayload, entry.OriginalPayload, "OriginalPayload should match")
		assertJSONEqual(t, normalizedPayload, entry.NormalizedPayload, "NormalizedPayload should match")
		assertJSONEqual(t, warnings, entry.Warnings, "Warnings should match")
		assert.Equal(t, &sourceID, entry.SourceID)
		assert.Equal(t, &externalID, entry.SourceExternalID)
		assert.Equal(t, &dedupHash, entry.DedupHash)
		assert.Equal(t, "pending", entry.Status)
		assert.True(t, entry.EventStartTime.Equal(startTime))
		assert.True(t, entry.EventEndTime.Equal(endTime))
	})

	t.Run("GetReviewQueueEntry", func(t *testing.T) {
		eventID, startTime := reviewQueueTestEvent(t, ctx, repo, place)

		params := events.ReviewQueueCreateParams{
			EventID:           eventID,
			OriginalPayload:   originalPayload,
			NormalizedPayload: normalizedPayload,
			Warnings:          warnings,
			EventStartTime:    startTime,
			EventEndTime:      &endTime,
		}

		created, err := repo.CreateReviewQueueEntry(ctx, params)
		require.NoError(t, err)

		// Get it back
		entry, err := repo.GetReviewQueueEntry(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, created.ID, entry.ID)
		assert.Equal(t, eventID, entry.EventID)
	})

	t.Run("GetReviewQueueEntry_NotFound", func(t *testing.T) {
		entry, err := repo.GetReviewQueueEntry(ctx, 999999)
		assert.ErrorIs(t, err, events.ErrNotFound)
		assert.Nil(t, entry)
	})

	t.Run("FindReviewByDedup_BySourceID", func(t *testing.T) {
		eventID, startTime := reviewQueueTestEvent(t, ctx, repo, place)

		findSourceID := "find-source-" + ulid.Make().String()[:8]
		findExternalID := "find-ext-" + ulid.Make().String()[:8]

		params := events.ReviewQueueCreateParams{
			EventID:           eventID,
			OriginalPayload:   originalPayload,
			NormalizedPayload: normalizedPayload,
			Warnings:          warnings,
			SourceID:          &findSourceID,
			SourceExternalID:  &findExternalID,
			EventStartTime:    startTime,
		}

		created, err := repo.CreateReviewQueueEntry(ctx, params)
		require.NoError(t, err)

		// Find by source ID
		entry, err := repo.FindReviewByDedup(ctx, &findSourceID, &findExternalID, nil)
		require.NoError(t, err)
		assert.Equal(t, created.ID, entry.ID)
	})

	t.Run("FindReviewByDedup_ByDedupHash", func(t *testing.T) {
		eventID, startTime := reviewQueueTestEvent(t, ctx, repo, place)

		uniqueHash := "unique-hash-" + ulid.Make().String()[:8]
		params := events.ReviewQueueCreateParams{
			EventID:           eventID,
			OriginalPayload:   originalPayload,
			NormalizedPayload: normalizedPayload,
			Warnings:          warnings,
			DedupHash:         &uniqueHash,
			EventStartTime:    startTime,
		}

		created, err := repo.CreateReviewQueueEntry(ctx, params)
		require.NoError(t, err)

		// Find by dedup hash
		entry, err := repo.FindReviewByDedup(ctx, nil, nil, &uniqueHash)
		require.NoError(t, err)
		assert.Equal(t, created.ID, entry.ID)
	})

	t.Run("FindReviewByDedup_NotFound", func(t *testing.T) {
		nonexistent := "nonexistent"
		entry, err := repo.FindReviewByDedup(ctx, &nonexistent, &nonexistent, nil)
		assert.ErrorIs(t, err, events.ErrNotFound)
		assert.Nil(t, entry)
	})

	t.Run("UpdateReviewQueueEntry", func(t *testing.T) {
		eventID, startTime := reviewQueueTestEvent(t, ctx, repo, place)

		params := events.ReviewQueueCreateParams{
			EventID:           eventID,
			OriginalPayload:   originalPayload,
			NormalizedPayload: normalizedPayload,
			Warnings:          warnings,
			EventStartTime:    startTime,
		}

		created, err := repo.CreateReviewQueueEntry(ctx, params)
		require.NoError(t, err)

		// Update it
		newWarnings, _ := json.Marshal([]map[string]interface{}{
			{
				"field":   "endDate",
				"code":    "reversed_dates_corrected_needs_review",
				"message": "Updated warning",
			},
		})

		updateParams := events.ReviewQueueUpdateParams{
			Warnings: &newWarnings,
		}

		updated, err := repo.UpdateReviewQueueEntry(ctx, created.ID, updateParams)
		require.NoError(t, err)
		assertJSONEqual(t, newWarnings, updated.Warnings, "Updated warnings should match")
	})

	t.Run("ListReviewQueue", func(t *testing.T) {
		// Create 3 entries, each with a unique event
		for i := 0; i < 3; i++ {
			eid, st := reviewQueueTestEvent(t, ctx, repo, place)
			params := events.ReviewQueueCreateParams{
				EventID:           eid,
				OriginalPayload:   originalPayload,
				NormalizedPayload: normalizedPayload,
				Warnings:          warnings,
				EventStartTime:    st,
			}

			_, err := repo.CreateReviewQueueEntry(ctx, params)
			require.NoError(t, err)
		}

		// List all pending reviews
		status := "pending"
		filters := events.ReviewQueueFilters{
			Status: &status,
			Limit:  100,
		}

		result, err := repo.ListReviewQueue(ctx, filters)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(result.Entries), 3)
	})

	t.Run("ApproveReview", func(t *testing.T) {
		eventID, startTime := reviewQueueTestEvent(t, ctx, repo, place)

		params := events.ReviewQueueCreateParams{
			EventID:           eventID,
			OriginalPayload:   originalPayload,
			NormalizedPayload: normalizedPayload,
			Warnings:          warnings,
			EventStartTime:    startTime,
		}

		created, err := repo.CreateReviewQueueEntry(ctx, params)
		require.NoError(t, err)

		// Approve it
		reviewedBy := "admin@test.com"
		notes := "Looks good"
		approved, err := repo.ApproveReview(ctx, created.ID, reviewedBy, &notes)
		require.NoError(t, err)
		assert.Equal(t, "approved", approved.Status)
		assert.Equal(t, &reviewedBy, approved.ReviewedBy)
		assert.Equal(t, &notes, approved.ReviewNotes)
		assert.NotNil(t, approved.ReviewedAt)
	})

	t.Run("RejectReview", func(t *testing.T) {
		eventID, startTime := reviewQueueTestEvent(t, ctx, repo, place)

		params := events.ReviewQueueCreateParams{
			EventID:           eventID,
			OriginalPayload:   originalPayload,
			NormalizedPayload: normalizedPayload,
			Warnings:          warnings,
			EventStartTime:    startTime,
		}

		created, err := repo.CreateReviewQueueEntry(ctx, params)
		require.NoError(t, err)

		// Reject it
		reviewedBy := "admin@test.com"
		reason := "Cannot determine correct time"
		rejected, err := repo.RejectReview(ctx, created.ID, reviewedBy, reason)
		require.NoError(t, err)
		assert.Equal(t, "rejected", rejected.Status)
		assert.Equal(t, &reviewedBy, rejected.ReviewedBy)
		assert.Equal(t, &reason, rejected.RejectionReason)
		assert.NotNil(t, rejected.ReviewedAt)
	})

	t.Run("CleanupExpiredReviews", func(t *testing.T) {
		eventID, _ := reviewQueueTestEvent(t, ctx, repo, place)

		// Create a rejected review for a past event
		pastStart := time.Now().Add(-10 * 24 * time.Hour)
		pastEnd := time.Now().Add(-9 * 24 * time.Hour)

		params := events.ReviewQueueCreateParams{
			EventID:           eventID,
			OriginalPayload:   originalPayload,
			NormalizedPayload: normalizedPayload,
			Warnings:          warnings,
			EventStartTime:    pastStart,
			EventEndTime:      &pastEnd,
		}

		created, err := repo.CreateReviewQueueEntry(ctx, params)
		require.NoError(t, err)

		// Reject it
		_, err = repo.RejectReview(ctx, created.ID, "admin@test.com", "test")
		require.NoError(t, err)

		// Run cleanup
		err = repo.CleanupExpiredReviews(ctx)
		require.NoError(t, err)

		// Verify it was deleted
		entry, err := repo.GetReviewQueueEntry(ctx, created.ID)
		assert.ErrorIs(t, err, events.ErrNotFound)
		assert.Nil(t, entry)
	})
}

func TestEventRepository_ReviewQueue_Pagination(t *testing.T) {
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

	repo := &EventRepository{pool: pool}

	// Create test venue
	place := insertPlace(t, ctx, pool, "Test Venue 2", "Toronto", "ON")

	// Create 5 review entries, each with a unique event
	originalPayload, _ := json.Marshal(map[string]interface{}{"test": "data"})

	for i := 0; i < 5; i++ {
		eventID, startTime := reviewQueueTestEvent(t, ctx, repo, place)
		params := events.ReviewQueueCreateParams{
			EventID:           eventID,
			OriginalPayload:   originalPayload,
			NormalizedPayload: originalPayload,
			Warnings:          originalPayload,
			EventStartTime:    startTime,
		}

		_, err := repo.CreateReviewQueueEntry(ctx, params)
		require.NoError(t, err)
	}

	t.Run("FirstPage", func(t *testing.T) {
		status := "pending"
		filters := events.ReviewQueueFilters{
			Status: &status,
			Limit:  2,
		}

		result, err := repo.ListReviewQueue(ctx, filters)
		require.NoError(t, err)
		assert.Len(t, result.Entries, 2)
		assert.NotNil(t, result.NextCursor)
	})

	t.Run("SecondPage", func(t *testing.T) {
		// Get first page
		status := "pending"
		filters := events.ReviewQueueFilters{
			Status: &status,
			Limit:  2,
		}

		firstPage, err := repo.ListReviewQueue(ctx, filters)
		require.NoError(t, err)

		// Get second page
		filters.NextCursor = firstPage.NextCursor
		secondPage, err := repo.ListReviewQueue(ctx, filters)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(secondPage.Entries), 1)

		// Ensure no overlap
		assert.NotEqual(t, firstPage.Entries[0].ID, secondPage.Entries[0].ID)
	})
}

// TestEventRepository_ListReviewQueue_PaginationEdgeCases tests the LIMIT+1 pagination pattern edge cases
// for ListReviewQueue (internal/storage/postgres/events_repository.go:1330-1384)
func TestEventRepository_ListReviewQueue_PaginationEdgeCases(t *testing.T) {
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

	repo := &EventRepository{pool: pool}

	t.Run("SinglePageResult_NoNextCursor", func(t *testing.T) {
		// Create test venue
		place := insertPlace(t, ctx, pool, "Single Page Venue", "Toronto", "ON")

		// Create exactly 2 entries (limit will be 5, so no next page)
		originalPayload, _ := json.Marshal(map[string]interface{}{"test": "data"})

		for i := 0; i < 2; i++ {
			eventID, startTime := reviewQueueTestEvent(t, ctx, repo, place)
			params := events.ReviewQueueCreateParams{
				EventID:           eventID,
				OriginalPayload:   originalPayload,
				NormalizedPayload: originalPayload,
				Warnings:          originalPayload,
				EventStartTime:    startTime,
			}

			_, err := repo.CreateReviewQueueEntry(ctx, params)
			require.NoError(t, err)
		}

		// Query with limit > number of entries
		status := "pending"
		filters := events.ReviewQueueFilters{
			Status: &status,
			Limit:  5,
		}

		result, err := repo.ListReviewQueue(ctx, filters)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(result.Entries), 2, "Should have at least 2 entries")
		assert.Nil(t, result.NextCursor, "NextCursor should be nil when count <= limit")
	})

	t.Run("ExactlyLimitPlusOne_BoundaryCondition", func(t *testing.T) {
		// Create test venue
		place := insertPlace(t, ctx, pool, "Boundary Venue", "Toronto", "ON")

		// Create exactly limit+1 entries (3 entries with limit=2)
		originalPayload, _ := json.Marshal(map[string]interface{}{"test": "data"})

		for i := 0; i < 3; i++ {
			eventID, startTime := reviewQueueTestEvent(t, ctx, repo, place)
			params := events.ReviewQueueCreateParams{
				EventID:           eventID,
				OriginalPayload:   originalPayload,
				NormalizedPayload: originalPayload,
				Warnings:          originalPayload,
				EventStartTime:    startTime,
			}

			_, err := repo.CreateReviewQueueEntry(ctx, params)
			require.NoError(t, err)
		}

		// Query with limit that will hit exactly limit+1 boundary
		status := "pending"
		filters := events.ReviewQueueFilters{
			Status: &status,
			Limit:  2,
		}

		result, err := repo.ListReviewQueue(ctx, filters)
		require.NoError(t, err)

		// Should return exactly 'limit' entries (2), not limit+1
		assert.Len(t, result.Entries, 2, "Should return exactly limit entries")

		// NextCursor should be set because there are more entries
		assert.NotNil(t, result.NextCursor, "NextCursor should be set when hasMore is true")

		// NextCursor should be the ID of the last returned entry
		assert.Equal(t, result.Entries[1].ID, *result.NextCursor, "NextCursor should be ID of last entry")
	})

	t.Run("EmptyResultSet_NoEntries", func(t *testing.T) {
		// Query with a status that has no entries
		status := "nonexistent_status"
		filters := events.ReviewQueueFilters{
			Status: &status,
			Limit:  10,
		}

		result, err := repo.ListReviewQueue(ctx, filters)
		require.NoError(t, err)
		assert.Empty(t, result.Entries, "Should return empty entries for nonexistent status")
		assert.Nil(t, result.NextCursor, "NextCursor should be nil for empty result")
		assert.Equal(t, int64(0), result.TotalCount, "TotalCount should be 0 for empty result")
	})

	t.Run("InvalidCursor_HandlesGracefully", func(t *testing.T) {
		// Create test venue and one entry
		place := insertPlace(t, ctx, pool, "Invalid Cursor Venue", "Toronto", "ON")
		originalPayload, _ := json.Marshal(map[string]interface{}{"test": "data"})

		eventID, startTime := reviewQueueTestEvent(t, ctx, repo, place)
		params := events.ReviewQueueCreateParams{
			EventID:           eventID,
			OriginalPayload:   originalPayload,
			NormalizedPayload: originalPayload,
			Warnings:          originalPayload,
			EventStartTime:    startTime,
		}

		_, err := repo.CreateReviewQueueEntry(ctx, params)
		require.NoError(t, err)

		// Query with an invalid (very high) cursor ID
		status := "pending"
		invalidCursor := 999999
		filters := events.ReviewQueueFilters{
			Status:     &status,
			Limit:      10,
			NextCursor: &invalidCursor,
		}

		result, err := repo.ListReviewQueue(ctx, filters)
		require.NoError(t, err, "Should handle invalid cursor gracefully")
		assert.Empty(t, result.Entries, "Should return empty entries for cursor beyond all entries")
		assert.Nil(t, result.NextCursor, "NextCursor should be nil when no more entries")
	})

	t.Run("ExactlyLimit_NoNextPage", func(t *testing.T) {
		// Create test venue
		place := insertPlace(t, ctx, pool, "Exact Limit Venue", "Toronto", "ON")

		// Create exactly 3 entries
		originalPayload, _ := json.Marshal(map[string]interface{}{"test": "data"})
		var createdIDs []int

		for i := 0; i < 3; i++ {
			eventID, startTime := reviewQueueTestEvent(t, ctx, repo, place)
			params := events.ReviewQueueCreateParams{
				EventID:           eventID,
				OriginalPayload:   originalPayload,
				NormalizedPayload: originalPayload,
				Warnings:          originalPayload,
				EventStartTime:    startTime,
			}

			entry, err := repo.CreateReviewQueueEntry(ctx, params)
			require.NoError(t, err)
			createdIDs = append(createdIDs, entry.ID)
		}

		// Query from a cursor position where exactly 'limit' entries remain
		// Use the first created ID as the cursor, so only 2 entries remain (< limit of 3)
		status := "pending"
		filters := events.ReviewQueueFilters{
			Status:     &status,
			Limit:      3,
			NextCursor: &createdIDs[0], // Start after first entry
		}

		result, err := repo.ListReviewQueue(ctx, filters)
		require.NoError(t, err)

		// Should return the 2 remaining entries (less than limit)
		assert.GreaterOrEqual(t, len(result.Entries), 2, "Should return at least 2 entries")

		// NextCursor should be nil because we got fewer than limit entries
		// This tests the case where hasMore=false (len(rows) <= limit)
		assert.Nil(t, result.NextCursor, "NextCursor should be nil when fewer than limit entries returned")
	})
}
