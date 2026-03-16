package events

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/config"
)

// TestMergeEvents_AtomicRollback verifies that MergeEvents rolls back on error
func TestMergeEvents_AtomicRollback(t *testing.T) {
	ctx := context.Background()

	// Mock repository that fails on CreateTombstone
	repo := &mockTransactionalRepo{
		getByULIDFunc: func(ctx context.Context, ulid string) (*Event, error) {
			return &Event{
				ID:   "event-" + ulid,
				ULID: ulid,
				Name: "Test Event " + ulid,
			}, nil
		},
		mergeEventsFunc: func(ctx context.Context, duplicateULID, primaryULID string) error {
			return nil // Merge succeeds
		},
		createTombstoneFunc: func(ctx context.Context, params TombstoneCreateParams) error {
			return errors.New("tombstone creation failed") // This should trigger rollback
		},
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500})

	params := MergeEventsParams{
		PrimaryULID:   "01HZTEST1",
		DuplicateULID: "01HZTEST2",
	}

	err := service.MergeEvents(ctx, params)

	// Should return error
	if err == nil {
		t.Fatal("Expected error when tombstone creation fails, got nil")
	}

	// Verify rollback was called
	if !repo.rollbackCalled {
		t.Error("Rollback was not called after transaction error")
	}

	// Verify commit was not called
	if repo.commitCalled {
		t.Error("Commit should not be called after transaction error")
	}
}

// TestMergeEvents_AtomicCommit verifies that MergeEvents commits on success
func TestMergeEvents_AtomicCommit(t *testing.T) {
	ctx := context.Background()

	// Mock repository that succeeds
	repo := &mockTransactionalRepo{
		getByULIDFunc: func(ctx context.Context, ulid string) (*Event, error) {
			return &Event{
				ID:   "event-" + ulid,
				ULID: ulid,
				Name: "Test Event " + ulid,
			}, nil
		},
		mergeEventsFunc: func(ctx context.Context, duplicateULID, primaryULID string) error {
			return nil
		},
		createTombstoneFunc: func(ctx context.Context, params TombstoneCreateParams) error {
			return nil
		},
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500})

	params := MergeEventsParams{
		PrimaryULID:   "01HZTEST1",
		DuplicateULID: "01HZTEST2",
	}

	err := service.MergeEvents(ctx, params)

	// Should succeed
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify commit was called
	if !repo.commitCalled {
		t.Error("Commit was not called after successful transaction")
	}

	// Note: Rollback IS called via defer (idiomatic Go pattern: defer rollback, no-op after commit).
	// This is correct behavior — the deferred Rollback is a safety net, not an error indicator.
	if !repo.rollbackCalled {
		t.Error("Rollback should be called via defer (idiomatic safety-net pattern)")
	}
}

// TestMergeEvents_RollbackOnMergeError verifies rollback when MergeEvents fails
func TestMergeEvents_RollbackOnMergeError(t *testing.T) {
	ctx := context.Background()

	// Mock repository that fails on MergeEvents
	repo := &mockTransactionalRepo{
		getByULIDFunc: func(ctx context.Context, ulid string) (*Event, error) {
			return &Event{
				ID:   "event-" + ulid,
				ULID: ulid,
				Name: "Test Event " + ulid,
			}, nil
		},
		mergeEventsFunc: func(ctx context.Context, duplicateULID, primaryULID string) error {
			return errors.New("merge failed") // This should trigger rollback
		},
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500})

	params := MergeEventsParams{
		PrimaryULID:   "01HZTEST1",
		DuplicateULID: "01HZTEST2",
	}

	err := service.MergeEvents(ctx, params)

	// Should return error
	if err == nil {
		t.Fatal("Expected error when merge fails, got nil")
	}

	// Verify rollback was called
	if !repo.rollbackCalled {
		t.Error("Rollback was not called after transaction error")
	}

	// Verify commit was not called
	if repo.commitCalled {
		t.Error("Commit should not be called after transaction error")
	}
}

// ---------------------------------------------------------------------------
// Tests for AddOccurrenceFromReview atomicity (srv-izykp)
// ---------------------------------------------------------------------------

// makeOccurrenceRepo builds a mockTransactionalRepo pre-wired for the
// "happy path" of AddOccurrenceFromReview. Individual tests override the
// func fields that correspond to the step they want to fail.
func makeOccurrenceRepo(targetID, targetULID, reviewEventULID string, startTime time.Time) *mockTransactionalRepo {
	mergedStatus := "merged"
	return &mockTransactionalRepo{
		getReviewQueueEntryFunc: func(_ context.Context, id int) (*ReviewQueueEntry, error) {
			return &ReviewQueueEntry{
				ID:             id,
				EventID:        "review-event-id",
				EventULID:      reviewEventULID,
				Status:         "pending",
				EventStartTime: startTime,
			}, nil
		},
		getByULIDFunc: func(_ context.Context, ulid string) (*Event, error) {
			if ulid == targetULID {
				return &Event{ID: targetID, ULID: targetULID, Name: "Series", LifecycleState: "published"}, nil
			}
			return &Event{ID: "review-event-id", ULID: reviewEventULID, Name: "Instance"}, nil
		},
		checkOccurrenceOverlapFunc: func(_ context.Context, _ string, _ time.Time, _ *time.Time) (bool, error) {
			return false, nil
		},
		createOccurrenceFunc: func(_ context.Context, _ OccurrenceCreateParams) error { return nil },
		softDeleteEventFunc:  func(_ context.Context, _, _ string) error { return nil },
		createTombstoneFunc:  func(_ context.Context, _ TombstoneCreateParams) error { return nil },
		mergeReviewFunc: func(_ context.Context, id int, _ string, _ string) (*ReviewQueueEntry, error) {
			return &ReviewQueueEntry{ID: id, Status: mergedStatus}, nil
		},
	}
}

// TestAddOccurrenceFromReview_CommitOnSuccess verifies that a successful
// add-occurrence operation commits the transaction.
func TestAddOccurrenceFromReview_CommitOnSuccess(t *testing.T) {
	ctx := context.Background()
	startTime := time.Now()
	repo := makeOccurrenceRepo("target-uuid", "01HTARGET00000000000000001", "01HREVIEW000000000000000001", startTime)

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500})
	_, err := service.AddOccurrenceFromReview(ctx, 1, "01HTARGET00000000000000001", "admin")

	if err != nil {
		t.Fatalf("Expected success, got: %v", err)
	}
	if !repo.commitCalled {
		t.Error("Commit should be called on success")
	}
}

// TestAddOccurrenceFromReview_RollbackOnOverlap verifies that an overlap
// conflict rolls back the transaction without committing.
func TestAddOccurrenceFromReview_RollbackOnOverlap(t *testing.T) {
	ctx := context.Background()
	startTime := time.Now()
	repo := makeOccurrenceRepo("target-uuid", "01HTARGET00000000000000001", "01HREVIEW000000000000000001", startTime)
	repo.checkOccurrenceOverlapFunc = func(_ context.Context, _ string, _ time.Time, _ *time.Time) (bool, error) {
		return true, nil // overlap detected
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500})
	err := func() error {
		_, e := service.AddOccurrenceFromReview(ctx, 1, "01HTARGET00000000000000001", "admin")
		return e
	}()

	if err == nil {
		t.Fatal("Expected ErrOccurrenceOverlap, got nil")
	}
	if !errors.Is(err, ErrOccurrenceOverlap) {
		t.Errorf("Expected ErrOccurrenceOverlap, got: %v", err)
	}
	if repo.commitCalled {
		t.Error("Commit must not be called when overlap is detected")
	}
	if !repo.rollbackCalled {
		t.Error("Rollback must be called via defer on error")
	}
}

// TestAddOccurrenceFromReview_RollbackOnCreateOccurrenceFailure verifies
// that a DB error during CreateOccurrence triggers a rollback.
func TestAddOccurrenceFromReview_RollbackOnCreateOccurrenceFailure(t *testing.T) {
	ctx := context.Background()
	startTime := time.Now()
	repo := makeOccurrenceRepo("target-uuid", "01HTARGET00000000000000001", "01HREVIEW000000000000000001", startTime)
	repo.createOccurrenceFunc = func(_ context.Context, _ OccurrenceCreateParams) error {
		return errors.New("db write failed")
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500})
	_, err := service.AddOccurrenceFromReview(ctx, 1, "01HTARGET00000000000000001", "admin")

	if err == nil {
		t.Fatal("Expected error from CreateOccurrence failure, got nil")
	}
	if repo.commitCalled {
		t.Error("Commit must not be called after CreateOccurrence error")
	}
	if !repo.rollbackCalled {
		t.Error("Rollback must be called via defer on error")
	}
}

// TestAddOccurrenceFromReview_RollbackOnSoftDeleteFailure verifies that a
// DB error during SoftDeleteEvent triggers a rollback.
func TestAddOccurrenceFromReview_RollbackOnSoftDeleteFailure(t *testing.T) {
	ctx := context.Background()
	startTime := time.Now()
	repo := makeOccurrenceRepo("target-uuid", "01HTARGET00000000000000001", "01HREVIEW000000000000000001", startTime)
	repo.softDeleteEventFunc = func(_ context.Context, _, _ string) error {
		return errors.New("soft-delete failed")
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500})
	_, err := service.AddOccurrenceFromReview(ctx, 1, "01HTARGET00000000000000001", "admin")

	if err == nil {
		t.Fatal("Expected error from SoftDeleteEvent failure, got nil")
	}
	if repo.commitCalled {
		t.Error("Commit must not be called after SoftDeleteEvent error")
	}
	if !repo.rollbackCalled {
		t.Error("Rollback must be called via defer on error")
	}
}

// mockTransactionalRepo implements Repository with transaction support
type mockTransactionalRepo struct {
	getByULIDFunc                     func(ctx context.Context, ulid string) (*Event, error)
	mergeEventsFunc                   func(ctx context.Context, duplicateULID, primaryULID string) error
	createTombstoneFunc               func(ctx context.Context, params TombstoneCreateParams) error
	getReviewQueueEntryFunc           func(ctx context.Context, id int) (*ReviewQueueEntry, error)
	lockReviewQueueEntryForUpdateFunc func(ctx context.Context, id int) (*ReviewQueueEntry, error)
	checkOccurrenceOverlapFunc        func(ctx context.Context, eventID string, startTime time.Time, endTime *time.Time) (bool, error)
	createOccurrenceFunc              func(ctx context.Context, params OccurrenceCreateParams) error
	softDeleteEventFunc               func(ctx context.Context, ulid, reason string) error
	mergeReviewFunc                   func(ctx context.Context, id int, reviewedBy string, primaryEventULID string) (*ReviewQueueEntry, error)
	approveReviewFunc                 func(ctx context.Context, id int, reviewedBy string, notes *string) (*ReviewQueueEntry, error)
	rejectReviewFunc                  func(ctx context.Context, id int, reviewedBy string, reason string) (*ReviewQueueEntry, error)
	updateEventFunc                   func(ctx context.Context, ulid string, params UpdateEventParams) (*Event, error)
	updateOccurrenceDatesFunc         func(ctx context.Context, eventULID string, startTime time.Time, endTime *time.Time) error
	lockEventForUpdateFunc            func(ctx context.Context, eventID string) error
	getPendingReviewByEventUlidFunc   func(ctx context.Context, eventULID string) (*ReviewQueueEntry, error)
	commitCalled                      bool
	rollbackCalled                    bool
}

func (m *mockTransactionalRepo) BeginTx(ctx context.Context) (Repository, TxCommitter, error) {
	// Return self as transaction-scoped repository
	return m, &mockTxCommitter{repo: m}, nil
}

func (m *mockTransactionalRepo) GetByULID(ctx context.Context, ulid string) (*Event, error) {
	if m.getByULIDFunc != nil {
		return m.getByULIDFunc(ctx, ulid)
	}
	return nil, ErrNotFound
}

func (m *mockTransactionalRepo) MergeEvents(ctx context.Context, duplicateULID, primaryULID string) error {
	if m.mergeEventsFunc != nil {
		return m.mergeEventsFunc(ctx, duplicateULID, primaryULID)
	}
	return nil
}

func (m *mockTransactionalRepo) CreateTombstone(ctx context.Context, params TombstoneCreateParams) error {
	if m.createTombstoneFunc != nil {
		return m.createTombstoneFunc(ctx, params)
	}
	return nil
}

func (m *mockTransactionalRepo) GetTombstoneByEventID(ctx context.Context, eventID string) (*Tombstone, error) {
	return nil, ErrNotFound
}

func (m *mockTransactionalRepo) GetTombstoneByEventULID(ctx context.Context, eventULID string) (*Tombstone, error) {
	return nil, ErrNotFound
}

// Unimplemented Repository methods
func (m *mockTransactionalRepo) List(ctx context.Context, filters Filters, pagination Pagination) (ListResult, error) {
	return ListResult{}, errors.New("not implemented")
}
func (m *mockTransactionalRepo) Create(ctx context.Context, params EventCreateParams) (*Event, error) {
	return nil, errors.New("not implemented")
}
func (m *mockTransactionalRepo) CreateOccurrence(ctx context.Context, params OccurrenceCreateParams) error {
	if m.createOccurrenceFunc != nil {
		return m.createOccurrenceFunc(ctx, params)
	}
	return errors.New("not implemented")
}
func (m *mockTransactionalRepo) CreateSource(ctx context.Context, params EventSourceCreateParams) error {
	return errors.New("not implemented")
}
func (m *mockTransactionalRepo) FindBySourceExternalID(ctx context.Context, sourceID, sourceEventID string) (*Event, error) {
	return nil, errors.New("not implemented")
}
func (m *mockTransactionalRepo) FindByDedupHash(ctx context.Context, dedupHash string) (*Event, error) {
	return nil, errors.New("not implemented")
}
func (m *mockTransactionalRepo) GetOrCreateSource(ctx context.Context, params SourceLookupParams) (string, error) {
	return "", errors.New("not implemented")
}
func (m *mockTransactionalRepo) GetIdempotencyKey(ctx context.Context, key string) (*IdempotencyKey, error) {
	return nil, errors.New("not implemented")
}
func (m *mockTransactionalRepo) InsertIdempotencyKey(ctx context.Context, params IdempotencyKeyCreateParams) (*IdempotencyKey, error) {
	return nil, errors.New("not implemented")
}
func (m *mockTransactionalRepo) UpdateIdempotencyKeyEvent(ctx context.Context, key, eventID, eventULID string) error {
	return errors.New("not implemented")
}
func (m *mockTransactionalRepo) UpsertPlace(ctx context.Context, params PlaceCreateParams) (*PlaceRecord, error) {
	return nil, errors.New("not implemented")
}
func (m *mockTransactionalRepo) UpsertOrganization(ctx context.Context, params OrganizationCreateParams) (*OrganizationRecord, error) {
	return nil, errors.New("not implemented")
}
func (m *mockTransactionalRepo) UpdateEvent(ctx context.Context, ulid string, params UpdateEventParams) (*Event, error) {
	if m.updateEventFunc != nil {
		return m.updateEventFunc(ctx, ulid, params)
	}
	return &Event{ULID: ulid}, nil
}
func (m *mockTransactionalRepo) UpdateOccurrenceDates(ctx context.Context, eventULID string, startTime time.Time, endTime *time.Time) error {
	if m.updateOccurrenceDatesFunc != nil {
		return m.updateOccurrenceDatesFunc(ctx, eventULID, startTime, endTime)
	}
	return nil
}
func (m *mockTransactionalRepo) SoftDeleteEvent(ctx context.Context, ulid, reason string) error {
	if m.softDeleteEventFunc != nil {
		return m.softDeleteEventFunc(ctx, ulid, reason)
	}
	return errors.New("not implemented")
}

// Review Queue methods
func (m *mockTransactionalRepo) FindReviewByDedup(ctx context.Context, sourceID *string, externalID *string, dedupHash *string) (*ReviewQueueEntry, error) {
	return nil, errors.New("not implemented")
}
func (m *mockTransactionalRepo) CreateReviewQueueEntry(ctx context.Context, params ReviewQueueCreateParams) (*ReviewQueueEntry, error) {
	return nil, errors.New("not implemented")
}
func (m *mockTransactionalRepo) UpdateReviewQueueEntry(ctx context.Context, id int, params ReviewQueueUpdateParams) (*ReviewQueueEntry, error) {
	return nil, errors.New("not implemented")
}
func (m *mockTransactionalRepo) GetReviewQueueEntry(ctx context.Context, id int) (*ReviewQueueEntry, error) {
	if m.getReviewQueueEntryFunc != nil {
		return m.getReviewQueueEntryFunc(ctx, id)
	}
	return nil, errors.New("not implemented")
}
func (m *mockTransactionalRepo) LockReviewQueueEntryForUpdate(ctx context.Context, id int) (*ReviewQueueEntry, error) {
	if m.lockReviewQueueEntryForUpdateFunc != nil {
		return m.lockReviewQueueEntryForUpdateFunc(ctx, id)
	}
	// Default: delegate to getReviewQueueEntryFunc (simulates lock + re-read).
	if m.getReviewQueueEntryFunc != nil {
		return m.getReviewQueueEntryFunc(ctx, id)
	}
	return nil, errors.New("not implemented")
}
func (m *mockTransactionalRepo) ListReviewQueue(ctx context.Context, filters ReviewQueueFilters) (*ReviewQueueListResult, error) {
	return nil, errors.New("not implemented")
}
func (m *mockTransactionalRepo) ApproveReview(ctx context.Context, id int, reviewedBy string, notes *string) (*ReviewQueueEntry, error) {
	if m.approveReviewFunc != nil {
		return m.approveReviewFunc(ctx, id, reviewedBy, notes)
	}
	return &ReviewQueueEntry{ID: id, Status: "approved"}, nil
}
func (m *mockTransactionalRepo) RejectReview(ctx context.Context, id int, reviewedBy string, reason string) (*ReviewQueueEntry, error) {
	if m.rejectReviewFunc != nil {
		return m.rejectReviewFunc(ctx, id, reviewedBy, reason)
	}
	return &ReviewQueueEntry{ID: id, Status: "rejected"}, nil
}
func (m *mockTransactionalRepo) MergeReview(ctx context.Context, id int, reviewedBy string, primaryEventULID string) (*ReviewQueueEntry, error) {
	if m.mergeReviewFunc != nil {
		return m.mergeReviewFunc(ctx, id, reviewedBy, primaryEventULID)
	}
	return nil, errors.New("not implemented")
}
func (m *mockTransactionalRepo) CleanupExpiredReviews(ctx context.Context) error {
	return errors.New("not implemented")
}
func (m *mockTransactionalRepo) GetSourceTrustLevel(ctx context.Context, eventID string) (int, error) {
	return 5, nil
}
func (m *mockTransactionalRepo) GetSourceTrustLevelBySourceID(ctx context.Context, sourceID string) (int, error) {
	return 5, nil
}
func (m *mockTransactionalRepo) FindNearDuplicates(ctx context.Context, venueID string, startTime time.Time, eventName string, threshold float64) ([]NearDuplicateCandidate, error) {
	return nil, nil
}
func (m *mockTransactionalRepo) FindSimilarPlaces(ctx context.Context, name string, locality string, region string, threshold float64) ([]SimilarPlaceCandidate, error) {
	return nil, nil
}
func (m *mockTransactionalRepo) FindSimilarOrganizations(ctx context.Context, name string, locality string, region string, threshold float64) ([]SimilarOrgCandidate, error) {
	return nil, nil
}
func (m *mockTransactionalRepo) MergePlaces(ctx context.Context, duplicateID string, primaryID string) (*MergeResult, error) {
	return &MergeResult{CanonicalID: primaryID}, nil
}
func (m *mockTransactionalRepo) MergeOrganizations(ctx context.Context, duplicateID string, primaryID string) (*MergeResult, error) {
	return &MergeResult{CanonicalID: primaryID}, nil
}
func (m *mockTransactionalRepo) InsertNotDuplicate(ctx context.Context, eventIDa string, eventIDb string, createdBy string) error {
	return nil
}
func (m *mockTransactionalRepo) IsNotDuplicate(ctx context.Context, eventIDa string, eventIDb string) (bool, error) {
	return false, nil
}
func (m *mockTransactionalRepo) GetPendingReviewByEventUlid(_ context.Context, ulid string) (*ReviewQueueEntry, error) {
	if m.getPendingReviewByEventUlidFunc != nil {
		return m.getPendingReviewByEventUlidFunc(context.Background(), ulid)
	}
	return nil, nil
}
func (m *mockTransactionalRepo) UpdateReviewWarnings(_ context.Context, _ int, _ []byte) error {
	return nil
}
func (m *mockTransactionalRepo) DismissCompanionWarningMatch(_ context.Context, _ string, _ string) error {
	return nil
}
func (m *mockTransactionalRepo) CheckOccurrenceOverlap(ctx context.Context, eventID string, startTime time.Time, endTime *time.Time) (bool, error) {
	if m.checkOccurrenceOverlapFunc != nil {
		return m.checkOccurrenceOverlapFunc(ctx, eventID, startTime, endTime)
	}
	return false, nil
}
func (m *mockTransactionalRepo) LockEventForUpdate(ctx context.Context, eventID string) error {
	if m.lockEventForUpdateFunc != nil {
		return m.lockEventForUpdateFunc(ctx, eventID)
	}
	return nil
}

// mockTxCommitter implements TxCommitter
type mockTxCommitter struct {
	repo *mockTransactionalRepo
}

func (m *mockTxCommitter) Commit(ctx context.Context) error {
	m.repo.commitCalled = true
	return nil
}

func (m *mockTxCommitter) Rollback(ctx context.Context) error {
	m.repo.rollbackCalled = true
	return nil
}

// TestAddOccurrenceFromReview_AllowsNonPublishedTarget verifies that the service
// accepts a target event in any non-deleted lifecycle state (draft, pending_review,
// cancelled, postponed, etc.).  Only deleted targets are rejected.
func TestAddOccurrenceFromReview_AllowsNonPublishedTarget(t *testing.T) {
	ctx := context.Background()
	startTime := time.Now()

	for _, state := range []string{"draft", "pending_review", "cancelled", "postponed", "rescheduled"} {
		state := state
		t.Run(state, func(t *testing.T) {
			repo := makeOccurrenceRepo("target-uuid", "01HTARGET00000000000000001", "01HREVIEW000000000000000001", startTime)
			repo.getByULIDFunc = func(_ context.Context, ulid string) (*Event, error) {
				if ulid == "01HTARGET00000000000000001" {
					return &Event{ID: "target-uuid", ULID: ulid, Name: "Series", LifecycleState: state}, nil
				}
				return &Event{ID: "review-event-id", ULID: ulid, Name: "Instance", LifecycleState: "published"}, nil
			}

			service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500})
			_, err := service.AddOccurrenceFromReview(ctx, 1, "01HTARGET00000000000000001", "admin")

			if err != nil {
				t.Errorf("state=%q: expected success, got: %v", state, err)
			}
			if !repo.commitCalled {
				t.Errorf("state=%q: commit must be called on success", state)
			}
		})
	}
}

// TestAddOccurrenceFromReview_RejectsDeletedTarget verifies that a deleted target
// returns ErrEventDeleted and no commit occurs.
func TestAddOccurrenceFromReview_RejectsDeletedTarget(t *testing.T) {
	ctx := context.Background()
	startTime := time.Now()

	repo := makeOccurrenceRepo("target-uuid", "01HTARGET00000000000000001", "01HREVIEW000000000000000001", startTime)
	repo.getByULIDFunc = func(_ context.Context, ulid string) (*Event, error) {
		if ulid == "01HTARGET00000000000000001" {
			return &Event{ID: "target-uuid", ULID: ulid, Name: "Series", LifecycleState: "deleted"}, nil
		}
		return &Event{ID: "review-event-id", ULID: ulid, Name: "Instance", LifecycleState: "published"}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500})
	_, err := service.AddOccurrenceFromReview(ctx, 1, "01HTARGET00000000000000001", "admin")

	if err == nil {
		t.Fatal("expected ErrEventDeleted, got nil")
	}
	if !errors.Is(err, ErrEventDeleted) {
		t.Errorf("expected ErrEventDeleted, got: %v", err)
	}
	if repo.commitCalled {
		t.Error("commit must not be called when target is deleted")
	}
}

// TestAddOccurrenceFromReview_ConcurrentReviewLock verifies that when a second
// concurrent request locks the same review row after it has already been processed,
// it receives ErrConflict rather than a confusing downstream error.
func TestAddOccurrenceFromReview_ConcurrentReviewLock(t *testing.T) {
	ctx := context.Background()
	startTime := time.Now()

	repo := makeOccurrenceRepo("target-uuid", "01HTARGET00000000000000001", "01HREVIEW000000000000000001", startTime)
	// Simulate the row already having been processed (status="merged") — as the
	// second goroutine would see after the first committed.
	repo.lockReviewQueueEntryForUpdateFunc = func(_ context.Context, id int) (*ReviewQueueEntry, error) {
		return &ReviewQueueEntry{ID: id, Status: "merged"}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500})
	_, err := service.AddOccurrenceFromReview(ctx, 1, "01HTARGET00000000000000001", "admin")

	if err == nil {
		t.Fatal("expected ErrConflict for already-processed review, got nil")
	}
	if !errors.Is(err, ErrConflict) {
		t.Errorf("expected ErrConflict, got: %v", err)
	}
	if repo.commitCalled {
		t.Error("commit must not be called when review is already processed")
	}
}

// TestAddOccurrenceFromReview_PreservesOccurrenceMetadata verifies that the
// occurrence is created using ALL of the review event's occurrence-level metadata
// (timezone, venue, virtual URL, door time, ticket URL, pricing) rather than the
// series-level defaults.  This is a regression test for the phase-5 review finding
// that only timezone/venue/virtualURL were being forwarded.
func TestAddOccurrenceFromReview_PreservesOccurrenceMetadata(t *testing.T) {
	ctx := context.Background()
	startTime := time.Now()

	reviewVenueID := "review-venue-uuid"
	reviewVirtualURL := "https://example.com/livestream"
	reviewTimezone := "America/Vancouver"
	doorTime := startTime.Add(-30 * time.Minute) // 30 min before start
	reviewTicketURL := "https://example.com/tickets/42"
	reviewPriceMin := 15.0
	reviewPriceMax := 45.0
	reviewPriceCurrency := "CAD"

	var capturedParams OccurrenceCreateParams

	repo := makeOccurrenceRepo("target-uuid", "01HTARGET00000000000000001", "01HREVIEW000000000000000001", startTime)
	repo.getByULIDFunc = func(_ context.Context, ulid string) (*Event, error) {
		if ulid == "01HTARGET00000000000000001" {
			targetVenueID := "target-venue-uuid"
			return &Event{
				ID:             "target-uuid",
				ULID:           ulid,
				Name:           "Series",
				LifecycleState: "published",
				PrimaryVenueID: &targetVenueID,
			}, nil
		}
		// Review event with a fully-populated occurrence
		return &Event{
			ID:   "review-event-id",
			ULID: ulid,
			Name: "Instance",
			Occurrences: []Occurrence{
				{
					StartTime:     startTime,
					Timezone:      reviewTimezone,
					VenueID:       &reviewVenueID,
					VirtualURL:    &reviewVirtualURL,
					DoorTime:      &doorTime,
					TicketURL:     reviewTicketURL,
					PriceMin:      &reviewPriceMin,
					PriceMax:      &reviewPriceMax,
					PriceCurrency: reviewPriceCurrency,
				},
			},
		}, nil
	}
	repo.createOccurrenceFunc = func(_ context.Context, params OccurrenceCreateParams) error {
		capturedParams = params
		return nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500})
	_, err := service.AddOccurrenceFromReview(ctx, 1, "01HTARGET00000000000000001", "admin")

	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if capturedParams.Timezone != reviewTimezone {
		t.Errorf("timezone: got %q, want %q", capturedParams.Timezone, reviewTimezone)
	}
	if capturedParams.VenueID == nil || *capturedParams.VenueID != reviewVenueID {
		t.Errorf("venueID: got %v, want %q", capturedParams.VenueID, reviewVenueID)
	}
	if capturedParams.VirtualURL == nil || *capturedParams.VirtualURL != reviewVirtualURL {
		t.Errorf("virtualURL: got %v, want %q", capturedParams.VirtualURL, reviewVirtualURL)
	}
	// Regression: ensure DoorTime, TicketURL, and pricing are also preserved
	if capturedParams.DoorTime == nil || !capturedParams.DoorTime.Equal(doorTime) {
		t.Errorf("doorTime: got %v, want %v", capturedParams.DoorTime, doorTime)
	}
	if capturedParams.TicketURL == nil || *capturedParams.TicketURL != reviewTicketURL {
		t.Errorf("ticketURL: got %v, want %q", capturedParams.TicketURL, reviewTicketURL)
	}
	if capturedParams.PriceMin == nil || *capturedParams.PriceMin != reviewPriceMin {
		t.Errorf("priceMin: got %v, want %v", capturedParams.PriceMin, reviewPriceMin)
	}
	if capturedParams.PriceMax == nil || *capturedParams.PriceMax != reviewPriceMax {
		t.Errorf("priceMax: got %v, want %v", capturedParams.PriceMax, reviewPriceMax)
	}
	if capturedParams.PriceCurrency != reviewPriceCurrency {
		t.Errorf("priceCurrency: got %q, want %q", capturedParams.PriceCurrency, reviewPriceCurrency)
	}
}

// TestAddOccurrenceFromReview_PreservesAvailability verifies that the occurrence
// Availability field is forwarded from the review event's matched occurrence to
// OccurrenceCreateParams, and that it falls back to empty string when not set.
// This is a regression test for the phase-5 review finding.
func TestAddOccurrenceFromReview_PreservesAvailability(t *testing.T) {
	ctx := context.Background()
	startTime := time.Now()

	t.Run("forwards Availability when set on matched occurrence", func(t *testing.T) {
		var capturedParams OccurrenceCreateParams
		repo := makeOccurrenceRepo("target-uuid", "01HTARGET00000000000000001", "01HREVIEW000000000000000001", startTime)
		repo.getByULIDFunc = func(_ context.Context, ulid string) (*Event, error) {
			if ulid == "01HTARGET00000000000000001" {
				return &Event{ID: "target-uuid", ULID: ulid, Name: "Series", LifecycleState: "published"}, nil
			}
			return &Event{
				ID:   "review-event-id",
				ULID: ulid,
				Name: "Instance",
				Occurrences: []Occurrence{
					{
						StartTime:    startTime,
						Availability: "members_only",
					},
				},
			}, nil
		}
		repo.createOccurrenceFunc = func(_ context.Context, params OccurrenceCreateParams) error {
			capturedParams = params
			return nil
		}

		service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500})
		_, err := service.AddOccurrenceFromReview(ctx, 1, "01HTARGET00000000000000001", "admin")

		if err != nil {
			t.Fatalf("expected success, got: %v", err)
		}
		if capturedParams.Availability != "members_only" {
			t.Errorf("Availability: got %q, want %q", capturedParams.Availability, "members_only")
		}
	})

	t.Run("Availability is empty string when not set", func(t *testing.T) {
		var capturedParams OccurrenceCreateParams
		repo := makeOccurrenceRepo("target-uuid", "01HTARGET00000000000000001", "01HREVIEW000000000000000001", startTime)
		repo.getByULIDFunc = func(_ context.Context, ulid string) (*Event, error) {
			if ulid == "01HTARGET00000000000000001" {
				return &Event{ID: "target-uuid", ULID: ulid, Name: "Series", LifecycleState: "published"}, nil
			}
			return &Event{
				ID:   "review-event-id",
				ULID: ulid,
				Name: "Instance",
				Occurrences: []Occurrence{
					{StartTime: startTime},
				},
			}, nil
		}
		repo.createOccurrenceFunc = func(_ context.Context, params OccurrenceCreateParams) error {
			capturedParams = params
			return nil
		}

		service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500})
		_, err := service.AddOccurrenceFromReview(ctx, 1, "01HTARGET00000000000000001", "admin")

		if err != nil {
			t.Fatalf("expected success, got: %v", err)
		}
		if capturedParams.Availability != "" {
			t.Errorf("Availability: expected empty string, got %q", capturedParams.Availability)
		}
	})
}

// TestAddOccurrenceFromReview_FallsBackToSeriesDefaultsWhenOccurrenceEmpty verifies
// that when the review event has no occurrence-level metadata overrides, the new
// occurrence inherits series-level defaults (timezone from service config, venue
// from target event).
func TestAddOccurrenceFromReview_FallsBackToSeriesDefaultsWhenOccurrenceEmpty(t *testing.T) {
	ctx := context.Background()
	startTime := time.Now()

	targetVenueID := "target-venue-uuid"
	serviceDefaultTZ := "America/Toronto"

	var capturedParams OccurrenceCreateParams

	repo := makeOccurrenceRepo("target-uuid", "01HTARGET00000000000000001", "01HREVIEW000000000000000001", startTime)
	repo.getByULIDFunc = func(_ context.Context, ulid string) (*Event, error) {
		if ulid == "01HTARGET00000000000000001" {
			return &Event{
				ID:             "target-uuid",
				ULID:           ulid,
				Name:           "Series",
				LifecycleState: "published",
				PrimaryVenueID: &targetVenueID,
			}, nil
		}
		// Review event with no occurrences (no override metadata available)
		return &Event{
			ID:   "review-event-id",
			ULID: ulid,
			Name: "Instance",
		}, nil
	}
	repo.createOccurrenceFunc = func(_ context.Context, params OccurrenceCreateParams) error {
		capturedParams = params
		return nil
	}

	service := NewAdminService(repo, false, serviceDefaultTZ, config.ValidationConfig{MaxEventNameLength: 500})
	_, err := service.AddOccurrenceFromReview(ctx, 1, "01HTARGET00000000000000001", "admin")

	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if capturedParams.Timezone != serviceDefaultTZ {
		t.Errorf("timezone: expected service default %q, got %q", serviceDefaultTZ, capturedParams.Timezone)
	}
	if capturedParams.VenueID == nil || *capturedParams.VenueID != targetVenueID {
		t.Errorf("venueID: expected target venue %q, got %v", targetVenueID, capturedParams.VenueID)
	}
	if capturedParams.DoorTime != nil {
		t.Errorf("doorTime: expected nil (no override), got %v", capturedParams.DoorTime)
	}
	if capturedParams.TicketURL != nil {
		t.Errorf("ticketURL: expected nil (no override), got %v", capturedParams.TicketURL)
	}
	if capturedParams.PriceMin != nil {
		t.Errorf("priceMin: expected nil (no override), got %v", capturedParams.PriceMin)
	}
}

// ---------------------------------------------------------------------------
// Lock-ordering tests: review row locked FIRST in all review-based methods
// ---------------------------------------------------------------------------

// makeReviewLockRepo returns a mockTransactionalRepo pre-wired for review-based
// methods (approve/reject/fix/merge).  The review entry starts as "pending".
// Individual tests override lockReviewQueueEntryForUpdateFunc to simulate an
// already-processed review (concurrent admin action).
func makeReviewLockRepo(eventULID string) *mockTransactionalRepo {
	pendingReview := &ReviewQueueEntry{ID: 1, EventULID: eventULID, Status: "pending"}
	return &mockTransactionalRepo{
		lockReviewQueueEntryForUpdateFunc: func(_ context.Context, id int) (*ReviewQueueEntry, error) {
			return pendingReview, nil
		},
		getByULIDFunc: func(_ context.Context, _ string) (*Event, error) {
			return &Event{ID: "event-uuid", ULID: eventULID, Name: "Test", LifecycleState: "draft",
				Occurrences: []Occurrence{{StartTime: time.Now()}}}, nil
		},
		softDeleteEventFunc: func(_ context.Context, _, _ string) error { return nil },
		mergeEventsFunc:     func(_ context.Context, _, _ string) error { return nil },
		mergeReviewFunc: func(_ context.Context, id int, _ string, _ string) (*ReviewQueueEntry, error) {
			return &ReviewQueueEntry{ID: id, Status: "merged"}, nil
		},
	}
}

// TestApproveEventWithReview_ConflictOnAlreadyProcessed verifies that when the
// review row is already processed, ApproveEventWithReview returns ErrConflict.
func TestApproveEventWithReview_ConflictOnAlreadyProcessed(t *testing.T) {
	ctx := context.Background()
	repo := makeReviewLockRepo("01HEVENT000000000000000001")
	repo.lockReviewQueueEntryForUpdateFunc = func(_ context.Context, id int) (*ReviewQueueEntry, error) {
		return &ReviewQueueEntry{ID: id, Status: "approved"}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500})
	_, err := service.ApproveEventWithReview(ctx, "01HEVENT000000000000000001", 1, "admin", nil)

	if err == nil {
		t.Fatal("expected ErrConflict, got nil")
	}
	if !errors.Is(err, ErrConflict) {
		t.Errorf("expected ErrConflict, got: %v", err)
	}
	if repo.commitCalled {
		t.Error("commit must not be called when review is already processed")
	}
}

// TestApproveEventWithReview_SuccessLockFirst verifies the happy path commits and
// that the review lock is acquired before event work.
func TestApproveEventWithReview_SuccessLockFirst(t *testing.T) {
	ctx := context.Background()
	var lockCalledBefore bool
	var lockCallCount int
	repo := makeReviewLockRepo("01HEVENT000000000000000001")
	repo.lockReviewQueueEntryForUpdateFunc = func(_ context.Context, id int) (*ReviewQueueEntry, error) {
		lockCallCount++
		lockCalledBefore = true
		return &ReviewQueueEntry{ID: id, Status: "pending"}, nil
	}
	repo.getByULIDFunc = func(_ context.Context, _ string) (*Event, error) {
		// Must be called after lock
		if !lockCalledBefore {
			return nil, errors.New("getByULID called before lock")
		}
		return &Event{ID: "event-uuid", ULID: "01HEVENT000000000000000001", LifecycleState: "draft"}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500})
	_, err := service.ApproveEventWithReview(ctx, "01HEVENT000000000000000001", 1, "admin", nil)

	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if lockCallCount != 1 {
		t.Errorf("expected lock called once, got %d", lockCallCount)
	}
	if !repo.commitCalled {
		t.Error("commit should be called on success")
	}
}

// TestRejectEventWithReview_ConflictOnAlreadyProcessed verifies that when the
// review row is already processed, RejectEventWithReview returns ErrConflict.
func TestRejectEventWithReview_ConflictOnAlreadyProcessed(t *testing.T) {
	ctx := context.Background()
	repo := makeReviewLockRepo("01HEVENT000000000000000001")
	repo.lockReviewQueueEntryForUpdateFunc = func(_ context.Context, id int) (*ReviewQueueEntry, error) {
		return &ReviewQueueEntry{ID: id, Status: "rejected"}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500})
	_, err := service.RejectEventWithReview(ctx, "01HEVENT000000000000000001", 1, "admin", "spam")

	if err == nil {
		t.Fatal("expected ErrConflict, got nil")
	}
	if !errors.Is(err, ErrConflict) {
		t.Errorf("expected ErrConflict, got: %v", err)
	}
	if repo.commitCalled {
		t.Error("commit must not be called when review is already processed")
	}
}

// TestRejectEventWithReview_SuccessLockFirst verifies the happy path commits.
func TestRejectEventWithReview_SuccessLockFirst(t *testing.T) {
	ctx := context.Background()
	repo := makeReviewLockRepo("01HEVENT000000000000000001")

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500})
	_, err := service.RejectEventWithReview(ctx, "01HEVENT000000000000000001", 1, "admin", "spam")

	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if !repo.commitCalled {
		t.Error("commit should be called on success")
	}
}

// TestFixAndApproveEventWithReview_ConflictOnAlreadyProcessed verifies that when
// the review row is already processed, FixAndApproveEventWithReview returns ErrConflict.
func TestFixAndApproveEventWithReview_ConflictOnAlreadyProcessed(t *testing.T) {
	ctx := context.Background()
	endTime := time.Now().Add(2 * time.Hour)
	repo := makeReviewLockRepo("01HEVENT000000000000000001")
	repo.lockReviewQueueEntryForUpdateFunc = func(_ context.Context, id int) (*ReviewQueueEntry, error) {
		return &ReviewQueueEntry{ID: id, Status: "approved"}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500})
	_, err := service.FixAndApproveEventWithReview(ctx, "01HEVENT000000000000000001", 1, "admin", nil, nil, &endTime)

	if err == nil {
		t.Fatal("expected ErrConflict, got nil")
	}
	if !errors.Is(err, ErrConflict) {
		t.Errorf("expected ErrConflict, got: %v", err)
	}
	if repo.commitCalled {
		t.Error("commit must not be called when review is already processed")
	}
}

// TestFixAndApproveEventWithReview_SuccessLockFirst verifies the happy path commits.
func TestFixAndApproveEventWithReview_SuccessLockFirst(t *testing.T) {
	ctx := context.Background()
	endTime := time.Now().Add(2 * time.Hour)
	repo := makeReviewLockRepo("01HEVENT000000000000000001")

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500})
	_, err := service.FixAndApproveEventWithReview(ctx, "01HEVENT000000000000000001", 1, "admin", nil, nil, &endTime)

	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if !repo.commitCalled {
		t.Error("commit should be called on success")
	}
}

// ---------------------------------------------------------------------------
// Tests for AddOccurrenceFromReviewNearDup atomicity (srv-izykp)
// ---------------------------------------------------------------------------

// makeNearDupOccurrenceRepo builds a mockTransactionalRepo pre-wired for the
// "happy path" of AddOccurrenceFromReviewNearDup. Individual tests override
// the func fields that correspond to the step they want to fail or inspect.
func makeNearDupOccurrenceRepo(targetID, targetULID, sourceULID string, startTime time.Time) *mockTransactionalRepo {
	sourceDup := sourceULID
	return &mockTransactionalRepo{
		lockReviewQueueEntryForUpdateFunc: func(_ context.Context, id int) (*ReviewQueueEntry, error) {
			return &ReviewQueueEntry{
				ID:                   id,
				EventID:              "target-event-id",
				EventULID:            targetULID,
				DuplicateOfEventULID: &sourceDup,
				Status:               "pending",
				EventStartTime:       startTime,
			}, nil
		},
		getByULIDFunc: func(_ context.Context, ulid string) (*Event, error) {
			if ulid == targetULID {
				return &Event{
					ID:             targetID,
					ULID:           targetULID,
					Name:           "Series",
					LifecycleState: "published",
				}, nil
			}
			// source event
			return &Event{
				ID:   "source-event-id",
				ULID: sourceULID,
				Name: "New Instance",
				Occurrences: []Occurrence{
					{StartTime: startTime},
				},
			}, nil
		},
		lockEventForUpdateFunc: func(_ context.Context, _ string) error { return nil },
		checkOccurrenceOverlapFunc: func(_ context.Context, _ string, _ time.Time, _ *time.Time) (bool, error) {
			return false, nil
		},
		createOccurrenceFunc: func(_ context.Context, _ OccurrenceCreateParams) error { return nil },
		softDeleteEventFunc:  func(_ context.Context, _, _ string) error { return nil },
		createTombstoneFunc:  func(_ context.Context, _ TombstoneCreateParams) error { return nil },
		mergeReviewFunc: func(_ context.Context, id int, _ string, _ string) (*ReviewQueueEntry, error) {
			return &ReviewQueueEntry{ID: id, Status: "merged"}, nil
		},
		// getPendingReviewByEventUlidFunc defaults to nil → returns nil, nil (no companion)
	}
}

// TestAddOccurrenceFromReviewNearDup_CommitOnSuccess verifies that a successful
// near-dup add-occurrence commits the transaction.
func TestAddOccurrenceFromReviewNearDup_CommitOnSuccess(t *testing.T) {
	ctx := context.Background()
	repo := makeNearDupOccurrenceRepo("target-id", "01HTARGET00000000000000001", "01HSOURCE00000000000000001", time.Now())

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{})
	entry, targetULID, err := service.AddOccurrenceFromReviewNearDup(ctx, 1, "admin")

	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if entry == nil {
		t.Fatal("expected non-nil review entry")
	}
	if targetULID == nil || *targetULID != "01HTARGET00000000000000001" {
		t.Errorf("expected targetULID=%q, got %v", "01HTARGET00000000000000001", targetULID)
	}
	if !repo.commitCalled {
		t.Error("commit should be called on success")
	}
}

// TestAddOccurrenceFromReviewNearDup_MissingDuplicateULID verifies that when
// the review entry has no DuplicateOfEventULID, the method returns ErrInvalidUpdateParams.
func TestAddOccurrenceFromReviewNearDup_MissingDuplicateULID(t *testing.T) {
	ctx := context.Background()
	repo := makeNearDupOccurrenceRepo("target-id", "01HTARGET00000000000000001", "01HSOURCE00000000000000001", time.Now())
	repo.lockReviewQueueEntryForUpdateFunc = func(_ context.Context, id int) (*ReviewQueueEntry, error) {
		return &ReviewQueueEntry{ID: id, EventULID: "01HTARGET00000000000000001", Status: "pending"}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{})
	_, _, err := service.AddOccurrenceFromReviewNearDup(ctx, 1, "admin")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrInvalidUpdateParams) {
		t.Errorf("expected ErrInvalidUpdateParams, got: %v", err)
	}
	if repo.commitCalled {
		t.Error("commit must not be called on validation error")
	}
}

// TestAddOccurrenceFromReviewNearDup_AlreadyProcessed verifies that a non-pending
// review entry returns ErrConflict without committing.
func TestAddOccurrenceFromReviewNearDup_AlreadyProcessed(t *testing.T) {
	ctx := context.Background()
	repo := makeNearDupOccurrenceRepo("target-id", "01HTARGET00000000000000001", "01HSOURCE00000000000000001", time.Now())
	repo.lockReviewQueueEntryForUpdateFunc = func(_ context.Context, id int) (*ReviewQueueEntry, error) {
		dup := "01HSOURCE00000000000000001"
		return &ReviewQueueEntry{ID: id, EventULID: "01HTARGET00000000000000001", DuplicateOfEventULID: &dup, Status: "merged"}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{})
	_, _, err := service.AddOccurrenceFromReviewNearDup(ctx, 1, "admin")

	if err == nil {
		t.Fatal("expected ErrConflict, got nil")
	}
	if !errors.Is(err, ErrConflict) {
		t.Errorf("expected ErrConflict, got: %v", err)
	}
	if repo.commitCalled {
		t.Error("commit must not be called when review is already processed")
	}
}

// TestAddOccurrenceFromReviewNearDup_DeletedTarget verifies that a deleted target
// event returns ErrEventDeleted and rolls back.
func TestAddOccurrenceFromReviewNearDup_DeletedTarget(t *testing.T) {
	ctx := context.Background()
	targetULID := "01HTARGET00000000000000001"
	sourceULID := "01HSOURCE00000000000000001"
	repo := makeNearDupOccurrenceRepo("target-id", targetULID, sourceULID, time.Now())
	repo.getByULIDFunc = func(_ context.Context, ulid string) (*Event, error) {
		if ulid == targetULID {
			return &Event{ID: "target-id", ULID: targetULID, LifecycleState: "deleted"}, nil
		}
		return &Event{ID: "source-id", ULID: sourceULID, Name: "New Instance"}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{})
	_, _, err := service.AddOccurrenceFromReviewNearDup(ctx, 1, "admin")

	if err == nil {
		t.Fatal("expected ErrEventDeleted, got nil")
	}
	if !errors.Is(err, ErrEventDeleted) {
		t.Errorf("expected ErrEventDeleted, got: %v", err)
	}
	if repo.commitCalled {
		t.Error("commit must not be called when target is deleted")
	}
}

// TestAddOccurrenceFromReviewNearDup_Overlap verifies that an overlap returns
// ErrOccurrenceOverlap and rolls back.
func TestAddOccurrenceFromReviewNearDup_Overlap(t *testing.T) {
	ctx := context.Background()
	repo := makeNearDupOccurrenceRepo("target-id", "01HTARGET00000000000000001", "01HSOURCE00000000000000001", time.Now())
	repo.checkOccurrenceOverlapFunc = func(_ context.Context, _ string, _ time.Time, _ *time.Time) (bool, error) {
		return true, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{})
	_, _, err := service.AddOccurrenceFromReviewNearDup(ctx, 1, "admin")

	if err == nil {
		t.Fatal("expected ErrOccurrenceOverlap, got nil")
	}
	if !errors.Is(err, ErrOccurrenceOverlap) {
		t.Errorf("expected ErrOccurrenceOverlap, got: %v", err)
	}
	if repo.commitCalled {
		t.Error("commit must not be called on overlap")
	}
}

// TestAddOccurrenceFromReviewNearDup_CompanionDismissalNonFatal verifies that a
// failure to dismiss the companion review entry is non-fatal: the transaction
// still commits and the near-dup review entry is resolved.
func TestAddOccurrenceFromReviewNearDup_CompanionDismissalNonFatal(t *testing.T) {
	ctx := context.Background()
	sourceULID := "01HSOURCE00000000000000001"
	repo := makeNearDupOccurrenceRepo("target-id", "01HTARGET00000000000000001", sourceULID, time.Now())
	companionID := 42
	repo.getPendingReviewByEventUlidFunc = func(_ context.Context, ulid string) (*ReviewQueueEntry, error) {
		if ulid == sourceULID {
			return &ReviewQueueEntry{ID: companionID, Status: "pending"}, nil
		}
		return nil, nil
	}
	mergeCallCount := 0
	repo.mergeReviewFunc = func(_ context.Context, id int, _ string, _ string) (*ReviewQueueEntry, error) {
		mergeCallCount++
		if id == companionID {
			return nil, ErrConflict // race: companion already dismissed — non-fatal
		}
		return &ReviewQueueEntry{ID: id, Status: "merged"}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{})
	entry, _, err := service.AddOccurrenceFromReviewNearDup(ctx, 1, "admin")

	if err != nil {
		t.Fatalf("expected success (companion failure is non-fatal), got: %v", err)
	}
	if entry == nil {
		t.Fatal("expected non-nil review entry")
	}
	if !repo.commitCalled {
		t.Error("commit should be called despite companion merge failure")
	}
	if mergeCallCount != 2 {
		t.Errorf("expected 2 MergeReview calls (companion + near-dup), got %d", mergeCallCount)
	}
}

// TestAddOccurrenceFromReviewNearDup_CompanionLockSkipsAlreadyProcessed verifies that
// when the companion review lock is acquired but the re-read status is no longer
// "pending" (concurrent admin action already resolved it), MergeReview is NOT called
// for the companion and the transaction still commits successfully.
func TestAddOccurrenceFromReviewNearDup_CompanionLockSkipsAlreadyProcessed(t *testing.T) {
	ctx := context.Background()
	sourceULID := "01HSOURCE00000000000000001"
	companionID := 55
	repo := makeNearDupOccurrenceRepo("target-id", "01HTARGET00000000000000001", sourceULID, time.Now())
	repo.getPendingReviewByEventUlidFunc = func(_ context.Context, ulid string) (*ReviewQueueEntry, error) {
		if ulid == sourceULID {
			return &ReviewQueueEntry{ID: companionID, Status: "pending"}, nil
		}
		return nil, nil
	}
	// Lock returns a non-pending entry — simulates a concurrent admin action that
	// already resolved the companion between GetPending and LockForUpdate.
	// The primary review lock (id=1) must still return pending.
	repo.lockReviewQueueEntryForUpdateFunc = func(_ context.Context, id int) (*ReviewQueueEntry, error) {
		if id == companionID {
			return &ReviewQueueEntry{ID: id, Status: "merged"}, nil
		}
		// primary review
		return &ReviewQueueEntry{
			ID:                   id,
			Status:               "pending",
			EventULID:            "01HTARGET00000000000000001",
			DuplicateOfEventULID: &sourceULID,
		}, nil
	}
	mergeCallIDs := []int{}
	repo.mergeReviewFunc = func(_ context.Context, id int, _ string, _ string) (*ReviewQueueEntry, error) {
		mergeCallIDs = append(mergeCallIDs, id)
		return &ReviewQueueEntry{ID: id, Status: "merged"}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{})
	entry, _, err := service.AddOccurrenceFromReviewNearDup(ctx, 1, "admin")

	if err != nil {
		t.Fatalf("expected success when companion already processed, got: %v", err)
	}
	if entry == nil {
		t.Fatal("expected non-nil review entry")
	}
	if !repo.commitCalled {
		t.Error("commit should be called")
	}
	// Companion merge must NOT have been called — only the primary review merge.
	for _, id := range mergeCallIDs {
		if id == companionID {
			t.Errorf("MergeReview must not be called for companion (id=%d) when it is already processed", companionID)
		}
	}
}

// TestAddOccurrenceFromReviewNearDup_CompanionLockConcurrentDelete verifies that
// when LockReviewQueueEntryForUpdate returns ErrNotFound for the companion (the row
// was deleted by a concurrent request), the error is treated as non-fatal and the
// transaction still commits.
func TestAddOccurrenceFromReviewNearDup_CompanionLockConcurrentDelete(t *testing.T) {
	ctx := context.Background()
	sourceULID := "01HSOURCE00000000000000001"
	companionID := 66
	repo := makeNearDupOccurrenceRepo("target-id", "01HTARGET00000000000000001", sourceULID, time.Now())
	repo.getPendingReviewByEventUlidFunc = func(_ context.Context, ulid string) (*ReviewQueueEntry, error) {
		if ulid == sourceULID {
			return &ReviewQueueEntry{ID: companionID, Status: "pending"}, nil
		}
		return nil, nil
	}
	repo.lockReviewQueueEntryForUpdateFunc = func(_ context.Context, id int) (*ReviewQueueEntry, error) {
		if id == companionID {
			return nil, ErrNotFound // companion row deleted by concurrent request
		}
		// primary review
		return &ReviewQueueEntry{
			ID:                   id,
			Status:               "pending",
			EventULID:            "01HTARGET00000000000000001",
			DuplicateOfEventULID: &sourceULID,
		}, nil
	}
	mergeCallIDs := []int{}
	repo.mergeReviewFunc = func(_ context.Context, id int, _ string, _ string) (*ReviewQueueEntry, error) {
		mergeCallIDs = append(mergeCallIDs, id)
		return &ReviewQueueEntry{ID: id, Status: "merged"}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{})
	entry, _, err := service.AddOccurrenceFromReviewNearDup(ctx, 1, "admin")

	if err != nil {
		t.Fatalf("expected success when companion lock returns ErrNotFound, got: %v", err)
	}
	if entry == nil {
		t.Fatal("expected non-nil review entry")
	}
	if !repo.commitCalled {
		t.Error("commit should be called")
	}
	// Only the primary review merge should have been called.
	for _, id := range mergeCallIDs {
		if id == companionID {
			t.Errorf("MergeReview must not be called for companion (id=%d) when lock returns ErrNotFound", companionID)
		}
	}
}

// TestAddOccurrenceFromReviewNearDup_CompanionLookupUnexpectedError verifies that an
// unexpected DB error from GetPendingReviewByEventUlid (anything other than ErrNotFound)
// surfaces and prevents the transaction from committing.
func TestAddOccurrenceFromReviewNearDup_CompanionLookupUnexpectedError(t *testing.T) {
	ctx := context.Background()
	sourceULID := "01HSOURCE00000000000000001"
	repo := makeNearDupOccurrenceRepo("target-id", "01HTARGET00000000000000001", sourceULID, time.Now())
	dbErr := errors.New("connection reset by peer")
	repo.getPendingReviewByEventUlidFunc = func(_ context.Context, ulid string) (*ReviewQueueEntry, error) {
		if ulid == sourceULID {
			return nil, dbErr
		}
		return nil, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{})
	_, _, err := service.AddOccurrenceFromReviewNearDup(ctx, 1, "admin")

	if err == nil {
		t.Fatal("expected error from unexpected DB failure, got nil")
	}
	if !errors.Is(err, dbErr) {
		t.Errorf("expected wrapped dbErr, got: %v", err)
	}
	if repo.commitCalled {
		t.Error("commit must not be called when companion lookup returns unexpected error")
	}
}

// TestAddOccurrenceFromReviewNearDup_TargetIsKeepNotAbsorbed verifies that the
// semantic direction is correct: the review's EventULID (existing series) is the
// target that is kept, and DuplicateOfEventULID (newly ingested event) is the
// source that is soft-deleted.
func TestAddOccurrenceFromReviewNearDup_TargetIsKeepNotAbsorbed(t *testing.T) {
	ctx := context.Background()
	targetULID := "01HTARGET00000000000000001"
	sourceULID := "01HSOURCE00000000000000001"

	var softDeletedULID string
	var mergedWithTargetULID string

	repo := makeNearDupOccurrenceRepo("target-id", targetULID, sourceULID, time.Now())
	repo.softDeleteEventFunc = func(_ context.Context, ulid, _ string) error {
		softDeletedULID = ulid
		return nil
	}
	repo.mergeReviewFunc = func(_ context.Context, id int, _ string, primaryULID string) (*ReviewQueueEntry, error) {
		mergedWithTargetULID = primaryULID
		return &ReviewQueueEntry{ID: id, Status: "merged"}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{})
	_, _, err := service.AddOccurrenceFromReviewNearDup(ctx, 1, "admin")

	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if softDeletedULID != sourceULID {
		t.Errorf("wrong event soft-deleted: want %s (source/new), got %s", sourceULID, softDeletedULID)
	}
	if mergedWithTargetULID != targetULID {
		t.Errorf("review merged with wrong target: want %s (existing series), got %s", targetULID, mergedWithTargetULID)
	}
}

// TestMergeEventsWithReview_ConflictOnAlreadyProcessed verifies that when the
// review row is already processed, MergeEventsWithReview returns ErrConflict.
func TestMergeEventsWithReview_ConflictOnAlreadyProcessed(t *testing.T) {
	ctx := context.Background()
	repo := makeReviewLockRepo("01HDUPLICATE000000000000001")
	repo.lockReviewQueueEntryForUpdateFunc = func(_ context.Context, id int) (*ReviewQueueEntry, error) {
		return &ReviewQueueEntry{ID: id, Status: "merged"}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500})
	_, err := service.MergeEventsWithReview(ctx, MergeEventsParams{
		PrimaryULID:   "01HPRIMARY000000000000001",
		DuplicateULID: "01HDUPLICATE000000000000001",
	}, 1, "admin")

	if err == nil {
		t.Fatal("expected ErrConflict, got nil")
	}
	if !errors.Is(err, ErrConflict) {
		t.Errorf("expected ErrConflict, got: %v", err)
	}
	if repo.commitCalled {
		t.Error("commit must not be called when review is already processed")
	}
}

// TestMergeEventsWithReview_SuccessLockFirst verifies the happy path commits and
// that review lock is acquired before any event work.
func TestMergeEventsWithReview_SuccessLockFirst(t *testing.T) {
	ctx := context.Background()
	var lockCalledBefore bool
	var lockCallCount int
	repo := makeReviewLockRepo("01HDUPLICATE000000000000001")
	repo.lockReviewQueueEntryForUpdateFunc = func(_ context.Context, id int) (*ReviewQueueEntry, error) {
		lockCallCount++
		lockCalledBefore = true
		return &ReviewQueueEntry{ID: id, Status: "pending"}, nil
	}
	repo.getByULIDFunc = func(_ context.Context, ulid string) (*Event, error) {
		// Must be called after lock
		if !lockCalledBefore {
			return nil, errors.New("getByULID called before lock")
		}
		return &Event{ID: "event-" + ulid, ULID: ulid, Name: "Event " + ulid, LifecycleState: "published"}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500})
	_, err := service.MergeEventsWithReview(ctx, MergeEventsParams{
		PrimaryULID:   "01HPRIMARY000000000000001",
		DuplicateULID: "01HDUPLICATE000000000000001",
	}, 1, "admin")

	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if lockCallCount != 1 {
		t.Errorf("expected lock called once, got %d", lockCallCount)
	}
	if !repo.commitCalled {
		t.Error("commit should be called on success")
	}
}
