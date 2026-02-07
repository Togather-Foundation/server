package postgres

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	// Create test venue first
	place := insertPlace(t, ctx, pool, "Test Venue", "Toronto", "ON")

	// Create a test event first (needed for foreign key)
	eventParams := events.EventCreateParams{
		ULID:           "01HQRS7T8G0000000000000001",
		Name:           "Test Event",
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
	eventID := event.ID // Use the actual event ID from database

	// Add an occurrence (required by event_location_required constraint)
	startTime := time.Date(2025, 3, 31, 23, 0, 0, 0, time.UTC)
	occParams := events.OccurrenceCreateParams{
		EventID:   eventID,
		StartTime: startTime,
		VenueID:   &place.ID,
		Timezone:  "America/Toronto",
	}
	err = repo.CreateOccurrence(ctx, occParams)
	require.NoError(t, err)

	// Test data
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
		assert.Equal(t, originalPayload, entry.OriginalPayload)
		assert.Equal(t, normalizedPayload, entry.NormalizedPayload)
		assert.Equal(t, warnings, entry.Warnings)
		assert.Equal(t, &sourceID, entry.SourceID)
		assert.Equal(t, &externalID, entry.SourceExternalID)
		assert.Equal(t, &dedupHash, entry.DedupHash)
		assert.Equal(t, "pending", entry.Status)
		assert.True(t, entry.EventStartTime.Equal(startTime))
		assert.True(t, entry.EventEndTime.Equal(endTime))
	})

	t.Run("GetReviewQueueEntry", func(t *testing.T) {
		// Create an entry first
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
		// Create an entry
		params := events.ReviewQueueCreateParams{
			EventID:           eventID,
			OriginalPayload:   originalPayload,
			NormalizedPayload: normalizedPayload,
			Warnings:          warnings,
			SourceID:          &sourceID,
			SourceExternalID:  &externalID,
			EventStartTime:    startTime,
		}

		created, err := repo.CreateReviewQueueEntry(ctx, params)
		require.NoError(t, err)

		// Find by source ID
		entry, err := repo.FindReviewByDedup(ctx, &sourceID, &externalID, nil)
		require.NoError(t, err)
		assert.Equal(t, created.ID, entry.ID)
	})

	t.Run("FindReviewByDedup_ByDedupHash", func(t *testing.T) {
		// Create an entry
		uniqueHash := "unique-hash-456"
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
		// Create an entry
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
		assert.Equal(t, newWarnings, updated.Warnings)
	})

	t.Run("ListReviewQueue", func(t *testing.T) {
		// Create multiple entries
		for i := 0; i < 3; i++ {
			params := events.ReviewQueueCreateParams{
				EventID:           eventID,
				OriginalPayload:   originalPayload,
				NormalizedPayload: normalizedPayload,
				Warnings:          warnings,
				EventStartTime:    startTime,
			}

			_, err := repo.CreateReviewQueueEntry(ctx, params)
			require.NoError(t, err)
		}

		// List all pending reviews
		status := "pending"
		filters := events.ReviewQueueFilters{
			Status: &status,
			Limit:  10,
		}

		result, err := repo.ListReviewQueue(ctx, filters)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(result.Entries), 3)
	})

	t.Run("ApproveReview", func(t *testing.T) {
		// Create an entry
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
		// Create an entry
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

	// Create test venue first
	place := insertPlace(t, ctx, pool, "Test Venue 2", "Toronto", "ON")

	// Create a test event
	eventParams := events.EventCreateParams{
		ULID:           "01HQRS7T8G0000000000000002",
		Name:           "Test Event for Pagination",
		Description:    "Test event",
		LifecycleState: "pending_review",
		EventDomain:    "arts",
		PrimaryVenueID: &place.ID,
		LicenseURL:     "https://creativecommons.org/publicdomain/zero/1.0/",
		LicenseStatus:  "cc0",
	}
	event, err := repo.Create(ctx, eventParams)
	require.NoError(t, err)
	eventID := event.ID // Use the actual event ID from database

	// Add an occurrence (required by event_location_required constraint)
	occStartTime := time.Now()
	occParams := events.OccurrenceCreateParams{
		EventID:   eventID,
		StartTime: occStartTime,
		VenueID:   &place.ID,
		Timezone:  "America/Toronto",
	}
	err = repo.CreateOccurrence(ctx, occParams)
	require.NoError(t, err)

	// Create 5 review entries
	originalPayload, _ := json.Marshal(map[string]interface{}{"test": "data"})
	startTime := time.Now()

	for i := 0; i < 5; i++ {
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
