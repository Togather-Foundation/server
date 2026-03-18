package events

import (
	"context"
	"encoding/json"
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

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")

	params := MergeEventsParams{
		PrimaryULID:   "01HZTEST000000000000000001",
		DuplicateULID: "01HZTEST000000000000000002",
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

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")

	params := MergeEventsParams{
		PrimaryULID:   "01HZTEST000000000000000001",
		DuplicateULID: "01HZTEST000000000000000002",
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

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")

	params := MergeEventsParams{
		PrimaryULID:   "01HZTEST000000000000000001",
		DuplicateULID: "01HZTEST000000000000000002",
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
				// Forward path requires potential_duplicate warning.
				Warnings: makeWarningsJSON("potential_duplicate"),
			}, nil
		},
		getByULIDFunc: func(_ context.Context, ulid string) (*Event, error) {
			if ulid == targetULID {
				return &Event{ID: targetID, ULID: targetULID, Name: "Series", LifecycleState: "published"}, nil
			}
			return &Event{ID: "review-event-id", ULID: reviewEventULID, Name: "Instance",
				Occurrences: []Occurrence{{StartTime: startTime}}}, nil
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
	repo := makeOccurrenceRepo("target-uuid", "01HTARGET00000000000000001", "01HREV00000000000000000001", startTime)

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
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
	repo := makeOccurrenceRepo("target-uuid", "01HTARGET00000000000000001", "01HREV00000000000000000001", startTime)
	repo.checkOccurrenceOverlapFunc = func(_ context.Context, _ string, _ time.Time, _ *time.Time) (bool, error) {
		return true, nil // overlap detected
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
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
	repo := makeOccurrenceRepo("target-uuid", "01HTARGET00000000000000001", "01HREV00000000000000000001", startTime)
	repo.createOccurrenceFunc = func(_ context.Context, _ OccurrenceCreateParams) error {
		return errors.New("db write failed")
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
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
	repo := makeOccurrenceRepo("target-uuid", "01HTARGET00000000000000001", "01HREV00000000000000000001", startTime)
	repo.softDeleteEventFunc = func(_ context.Context, _, _ string) error {
		return errors.New("soft-delete failed")
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
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
	getByULIDFunc                                   func(ctx context.Context, ulid string) (*Event, error)
	mergeEventsFunc                                 func(ctx context.Context, duplicateULID, primaryULID string) error
	createTombstoneFunc                             func(ctx context.Context, params TombstoneCreateParams) error
	getReviewQueueEntryFunc                         func(ctx context.Context, id int) (*ReviewQueueEntry, error)
	lockReviewQueueEntryForUpdateFunc               func(ctx context.Context, id int) (*ReviewQueueEntry, error)
	checkOccurrenceOverlapFunc                      func(ctx context.Context, eventID string, startTime time.Time, endTime *time.Time) (bool, error)
	createOccurrenceFunc                            func(ctx context.Context, params OccurrenceCreateParams) error
	softDeleteEventFunc                             func(ctx context.Context, ulid, reason string) error
	mergeReviewFunc                                 func(ctx context.Context, id int, reviewedBy string, primaryEventULID string) (*ReviewQueueEntry, error)
	approveReviewFunc                               func(ctx context.Context, id int, reviewedBy string, notes *string) (*ReviewQueueEntry, error)
	rejectReviewFunc                                func(ctx context.Context, id int, reviewedBy string, reason string) (*ReviewQueueEntry, error)
	updateEventFunc                                 func(ctx context.Context, ulid string, params UpdateEventParams) (*Event, error)
	updateOccurrenceDatesFunc                       func(ctx context.Context, eventULID string, startTime time.Time, endTime *time.Time) error
	lockEventForUpdateFunc                          func(ctx context.Context, eventID string) error
	getPendingReviewByEventUlidFunc                 func(ctx context.Context, eventULID string) (*ReviewQueueEntry, error)
	getPendingReviewByEventUlidAndDuplicateUlidFunc func(ctx context.Context, eventULID string, duplicateULID string) (*ReviewQueueEntry, error)
	deleteOccurrencesByEventULIDFunc                func(ctx context.Context, eventULID string) error
	dismissPendingReviewsByEventULIDsFunc           func(ctx context.Context, eventULIDs []string, reviewedBy string) ([]int, error)
	createReviewQueueEntryFunc                      func(ctx context.Context, params ReviewQueueCreateParams) (*ReviewQueueEntry, error)
	findByDedupHashFunc                             func(ctx context.Context, dedupHash string) (*Event, error)
	findNearDuplicatesFunc                          func(ctx context.Context, venueID string, startTime time.Time, eventName string, threshold float64) ([]NearDuplicateCandidate, error)
	commitCalled                                    bool
	rollbackCalled                                  bool
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
	if m.findByDedupHashFunc != nil {
		return m.findByDedupHashFunc(ctx, dedupHash)
	}
	return nil, ErrNotFound
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
func (m *mockTransactionalRepo) GetPlaceByULID(ctx context.Context, ulid string) (*PlaceRecord, error) {
	return nil, ErrNotFound
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
func (m *mockTransactionalRepo) DeleteOccurrencesByEventULID(ctx context.Context, eventULID string) error {
	if m.deleteOccurrencesByEventULIDFunc != nil {
		return m.deleteOccurrencesByEventULIDFunc(ctx, eventULID)
	}
	return nil
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
	if m.createReviewQueueEntryFunc != nil {
		return m.createReviewQueueEntryFunc(ctx, params)
	}
	return &ReviewQueueEntry{}, nil
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
	if m.findNearDuplicatesFunc != nil {
		return m.findNearDuplicatesFunc(ctx, venueID, startTime, eventName, threshold)
	}
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
func (m *mockTransactionalRepo) GetPendingReviewByEventUlidAndDuplicateUlid(_ context.Context, eventULID string, duplicateULID string) (*ReviewQueueEntry, error) {
	if m.getPendingReviewByEventUlidAndDuplicateUlidFunc != nil {
		return m.getPendingReviewByEventUlidAndDuplicateUlidFunc(context.Background(), eventULID, duplicateULID)
	}
	return nil, nil
}
func (m *mockTransactionalRepo) UpdateReviewWarnings(_ context.Context, _ int, _ []byte) error {
	return nil
}
func (m *mockTransactionalRepo) DismissCompanionWarningMatch(_ context.Context, _ string, _ string) error {
	return nil
}
func (m *mockTransactionalRepo) DismissWarningMatchByReviewID(_ context.Context, _ int, _ string) error {
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

func (m *mockTransactionalRepo) InsertOccurrence(ctx context.Context, params OccurrenceCreateParams) (*Occurrence, error) {
	return &Occurrence{ID: "test-occ-id", StartTime: params.StartTime}, nil
}

func (m *mockTransactionalRepo) GetOccurrenceByID(ctx context.Context, eventID string, occurrenceID string) (*Occurrence, error) {
	return nil, ErrNotFound
}

func (m *mockTransactionalRepo) UpdateOccurrence(ctx context.Context, eventID string, occurrenceID string, params OccurrenceUpdateParams) (*Occurrence, error) {
	return &Occurrence{ID: occurrenceID}, nil
}

func (m *mockTransactionalRepo) DeleteOccurrenceByID(ctx context.Context, eventID string, occurrenceID string) error {
	return nil
}

func (m *mockTransactionalRepo) CountOccurrences(ctx context.Context, eventID string) (int64, error) {
	return 2, nil
}

func (m *mockTransactionalRepo) CheckOccurrenceOverlapExcluding(ctx context.Context, eventID string, startTime time.Time, endTime *time.Time, excludeOccurrenceID string) (bool, error) {
	return false, nil
}

func (m *mockTransactionalRepo) DismissPendingReviewsByEventULIDs(ctx context.Context, eventULIDs []string, reviewedBy string) ([]int, error) {
	if m.dismissPendingReviewsByEventULIDsFunc != nil {
		return m.dismissPendingReviewsByEventULIDsFunc(ctx, eventULIDs, reviewedBy)
	}
	return nil, nil
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
			repo := makeOccurrenceRepo("target-uuid", "01HTARGET00000000000000001", "01HREV00000000000000000001", startTime)
			repo.getByULIDFunc = func(_ context.Context, ulid string) (*Event, error) {
				if ulid == "01HTARGET00000000000000001" {
					return &Event{ID: "target-uuid", ULID: ulid, Name: "Series", LifecycleState: state}, nil
				}
				return &Event{ID: "review-event-id", ULID: ulid, Name: "Instance", LifecycleState: "published",
					Occurrences: []Occurrence{{StartTime: startTime}}}, nil
			}

			service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
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

	repo := makeOccurrenceRepo("target-uuid", "01HTARGET00000000000000001", "01HREV00000000000000000001", startTime)
	repo.getByULIDFunc = func(_ context.Context, ulid string) (*Event, error) {
		if ulid == "01HTARGET00000000000000001" {
			return &Event{ID: "target-uuid", ULID: ulid, Name: "Series", LifecycleState: "deleted"}, nil
		}
		return &Event{ID: "review-event-id", ULID: ulid, Name: "Instance", LifecycleState: "published",
			Occurrences: []Occurrence{{StartTime: startTime}}}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
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

// TestAddOccurrenceFromReview_RejectsDeletedSource verifies that when the source
// event (the review event being absorbed) has been soft-deleted by the time the
// post-lock re-read is performed, AddOccurrenceFromReview returns ErrEventDeleted
// and does not commit.  This guards against a TOCTOU race where a concurrent
// admin action deletes or absorbs the source event between the review lock and
// the occurrence creation.
func TestAddOccurrenceFromReview_RejectsDeletedSource(t *testing.T) {
	ctx := context.Background()
	startTime := time.Now()
	targetULID := "01HTARGET00000000000000001"
	reviewULID := "01HREV00000000000000000001"

	repo := makeOccurrenceRepo("target-uuid", targetULID, reviewULID, startTime)
	repo.getByULIDFunc = func(_ context.Context, ulid string) (*Event, error) {
		if ulid == targetULID {
			return &Event{ID: "target-uuid", ULID: targetULID, Name: "Series", LifecycleState: "published"}, nil
		}
		// Source event is soft-deleted — simulates concurrent deletion.
		return &Event{ID: "review-event-id", ULID: ulid, Name: "Instance",
			LifecycleState: "deleted",
			Occurrences:    []Occurrence{{StartTime: startTime}}}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
	_, err := service.AddOccurrenceFromReview(ctx, 1, targetULID, "admin")

	if err == nil {
		t.Fatal("expected ErrEventDeleted, got nil")
	}
	if !errors.Is(err, ErrEventDeleted) {
		t.Errorf("expected ErrEventDeleted, got: %v", err)
	}
	if repo.commitCalled {
		t.Error("commit must not be called when source is deleted")
	}
}

// TestAddOccurrenceFromReview_ConcurrentReviewLock verifies that when a second
// concurrent request locks the same review row after it has already been processed,
// it receives ErrConflict rather than a confusing downstream error.
func TestAddOccurrenceFromReview_ConcurrentReviewLock(t *testing.T) {
	ctx := context.Background()
	startTime := time.Now()

	repo := makeOccurrenceRepo("target-uuid", "01HTARGET00000000000000001", "01HREV00000000000000000001", startTime)
	// Simulate the row already having been processed (status="merged") — as the
	// second goroutine would see after the first committed.
	repo.lockReviewQueueEntryForUpdateFunc = func(_ context.Context, id int) (*ReviewQueueEntry, error) {
		return &ReviewQueueEntry{ID: id, Status: "merged"}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
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

	repo := makeOccurrenceRepo("target-uuid", "01HTARGET00000000000000001", "01HREV00000000000000000001", startTime)
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

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
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
		repo := makeOccurrenceRepo("target-uuid", "01HTARGET00000000000000001", "01HREV00000000000000000001", startTime)
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

		service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
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
		repo := makeOccurrenceRepo("target-uuid", "01HTARGET00000000000000001", "01HREV00000000000000000001", startTime)
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

		service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
		_, err := service.AddOccurrenceFromReview(ctx, 1, "01HTARGET00000000000000001", "admin")

		if err != nil {
			t.Fatalf("expected success, got: %v", err)
		}
		if capturedParams.Availability != "" {
			t.Errorf("Availability: expected empty string, got %q", capturedParams.Availability)
		}
	})
}

// TestAddOccurrenceFromReview_ZeroOccurrenceSourceRejected verifies that when the
// review event (source) has no occurrences the method returns ErrZeroOccurrenceSource
// without committing the transaction.  Prior to this fix the method would fall back to
// series-level defaults and proceed — silently absorbing nothing while still
// soft-deleting the source event.
func TestAddOccurrenceFromReview_ZeroOccurrenceSourceRejected(t *testing.T) {
	ctx := context.Background()
	startTime := time.Now()

	repo := makeOccurrenceRepo("target-uuid", "01HTARGET00000000000000001", "01HREV00000000000000000001", startTime)
	repo.getByULIDFunc = func(_ context.Context, ulid string) (*Event, error) {
		if ulid == "01HTARGET00000000000000001" {
			return &Event{
				ID:             "target-uuid",
				ULID:           ulid,
				Name:           "Series",
				LifecycleState: "published",
			}, nil
		}
		// Review event with no occurrences — zero-occurrence source
		return &Event{
			ID:   "review-event-id",
			ULID: ulid,
			Name: "Instance",
		}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
	_, err := service.AddOccurrenceFromReview(ctx, 1, "01HTARGET00000000000000001", "admin")

	if err == nil {
		t.Fatal("expected ErrZeroOccurrenceSource, got nil")
	}
	if !errors.Is(err, ErrZeroOccurrenceSource) {
		t.Errorf("expected ErrZeroOccurrenceSource, got: %v", err)
	}
	if repo.commitCalled {
		t.Error("commit must not be called when source has no occurrences")
	}
}

// TestAddOccurrenceFromReview_UsesLockedOccurrenceTimestamps verifies that the
// occurrence added to the target series uses the source event's actual (locked)
// occurrence timestamps rather than the review-row snapshot values.
//
// If the source event was edited after ingest its occurrence start/end times may
// diverge from what the review row recorded.  Using stale snapshot values would
// absorb the wrong time slot and then soft-delete the source event — data loss.
func TestAddOccurrenceFromReview_UsesLockedOccurrenceTimestamps(t *testing.T) {
	ctx := context.Background()
	// Deliberately different timestamps: snapshot (review row) vs locked (source event).
	snapshotStart := time.Date(2024, 9, 10, 18, 0, 0, 0, time.UTC)
	snapshotEnd := time.Date(2024, 9, 10, 20, 0, 0, 0, time.UTC)
	lockedStart := time.Date(2024, 9, 10, 19, 0, 0, 0, time.UTC) // different hour
	lockedEnd := time.Date(2024, 9, 10, 21, 0, 0, 0, time.UTC)   // different hour

	var capturedParams OccurrenceCreateParams

	repo := makeOccurrenceRepo("target-uuid", "01HTARGET00000000000000001", "01HREV00000000000000000001", snapshotStart)
	// The review queue mock returns the snapshot start (snapshotStart) — already wired.
	// Override the source-event fetch to return a *different* occurrence time.
	repo.getByULIDFunc = func(_ context.Context, ulid string) (*Event, error) {
		if ulid == "01HTARGET00000000000000001" {
			return &Event{ID: "target-uuid", ULID: ulid, Name: "Series", LifecycleState: "published"}, nil
		}
		return &Event{
			ID:   "review-event-id",
			ULID: ulid,
			Name: "Instance",
			Occurrences: []Occurrence{
				{StartTime: lockedStart, EndTime: &lockedEnd},
			},
		}, nil
	}
	repo.createOccurrenceFunc = func(_ context.Context, params OccurrenceCreateParams) error {
		capturedParams = params
		return nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
	_, err := service.AddOccurrenceFromReview(ctx, 1, "01HTARGET00000000000000001", "admin")
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}

	// Timestamps must come from the locked occurrence, not the snapshot.
	if !capturedParams.StartTime.Equal(lockedStart) {
		t.Errorf("StartTime: got %v, want locked %v (must not use snapshot %v)",
			capturedParams.StartTime, lockedStart, snapshotStart)
	}
	if capturedParams.EndTime == nil || !capturedParams.EndTime.Equal(lockedEnd) {
		t.Errorf("EndTime: got %v, want locked %v (must not use snapshot %v)",
			capturedParams.EndTime, lockedEnd, snapshotEnd)
	}
}

// ---------------------------------------------------------------------------
// Lock-ordering tests: review row locked FIRST in all review-based methods
// ---------------------------------------------------------------------------

// TestAddOccurrenceFromReview_MultiOccurrenceSourceRejected verifies that when the
// source (review) event has more than one occurrence the method returns
// ErrAmbiguousOccurrenceSource without committing the transaction.  Absorbing only
// one occurrence while soft-deleting the entire source event would silently lose
// the remaining occurrences.
func TestAddOccurrenceFromReview_MultiOccurrenceSourceRejected(t *testing.T) {
	ctx := context.Background()
	startTime := time.Now()

	repo := makeOccurrenceRepo("target-uuid", "01HTARGET00000000000000001", "01HREV00000000000000000001", startTime)
	repo.getByULIDFunc = func(_ context.Context, ulid string) (*Event, error) {
		if ulid == "01HTARGET00000000000000001" {
			return &Event{
				ID:             "target-uuid",
				ULID:           ulid,
				Name:           "Series",
				LifecycleState: "published",
			}, nil
		}
		// Review event has two occurrences — ambiguous: which one to absorb?
		return &Event{
			ID:   "review-event-id",
			ULID: ulid,
			Name: "Multi-Occurrence Instance",
			Occurrences: []Occurrence{
				{StartTime: startTime},
				{StartTime: startTime.Add(24 * time.Hour)},
			},
		}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
	_, err := service.AddOccurrenceFromReview(ctx, 1, "01HTARGET00000000000000001", "admin")

	if err == nil {
		t.Fatal("expected ErrAmbiguousOccurrenceSource, got nil")
	}
	if !errors.Is(err, ErrAmbiguousOccurrenceSource) {
		t.Errorf("expected ErrAmbiguousOccurrenceSource, got: %v", err)
	}
	if repo.commitCalled {
		t.Error("commit must not be called when source has multiple occurrences")
	}
}

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

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
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

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
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

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
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

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
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

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
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

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
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
				Warnings:             []byte(`[{"code":"near_duplicate_of_new_event","message":"near_duplicate_of_new_event"}]`),
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
	repo := makeNearDupOccurrenceRepo("target-id", "01HTARGET00000000000000001", "01HSRC00000000000000000001", time.Now())

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, "https://toronto.togather.foundation")
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
	repo := makeNearDupOccurrenceRepo("target-id", "01HTARGET00000000000000001", "01HSRC00000000000000000001", time.Now())
	repo.lockReviewQueueEntryForUpdateFunc = func(_ context.Context, id int) (*ReviewQueueEntry, error) {
		return &ReviewQueueEntry{
			ID:        id,
			EventULID: "01HTARGET00000000000000001",
			Status:    "pending",
			Warnings:  []byte(`[{"code":"near_duplicate_of_new_event","message":"near_duplicate_of_new_event"}]`),
		}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, "https://toronto.togather.foundation")
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
	repo := makeNearDupOccurrenceRepo("target-id", "01HTARGET00000000000000001", "01HSRC00000000000000000001", time.Now())
	repo.lockReviewQueueEntryForUpdateFunc = func(_ context.Context, id int) (*ReviewQueueEntry, error) {
		dup := "01HSRC00000000000000000001"
		return &ReviewQueueEntry{ID: id, EventULID: "01HTARGET00000000000000001", DuplicateOfEventULID: &dup, Status: "merged"}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, "https://toronto.togather.foundation")
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
	sourceULID := "01HSRC00000000000000000001"
	repo := makeNearDupOccurrenceRepo("target-id", targetULID, sourceULID, time.Now())
	repo.getByULIDFunc = func(_ context.Context, ulid string) (*Event, error) {
		if ulid == targetULID {
			return &Event{ID: "target-id", ULID: targetULID, LifecycleState: "deleted"}, nil
		}
		return &Event{ID: "source-id", ULID: sourceULID, Name: "New Instance"}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, "https://toronto.togather.foundation")
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

// TestAddOccurrenceFromReviewNearDup_RejectsDeletedSource verifies that when the
// source event (the newly-ingested event, DuplicateOfEventULID) has been
// soft-deleted by the time the post-lock re-read is performed,
// AddOccurrenceFromReviewNearDup returns ErrEventDeleted without committing.
// This is the near-dup sibling of TestAddOccurrenceFromReview_RejectsDeletedSource
// and guards against the same TOCTOU race pattern on the near-dup path.
func TestAddOccurrenceFromReviewNearDup_RejectsDeletedSource(t *testing.T) {
	ctx := context.Background()
	targetULID := "01HTARGET00000000000000001"
	sourceULID := "01HSRC00000000000000000001"
	repo := makeNearDupOccurrenceRepo("target-id", targetULID, sourceULID, time.Now())
	repo.getByULIDFunc = func(_ context.Context, ulid string) (*Event, error) {
		if ulid == targetULID {
			return &Event{ID: "target-id", ULID: targetULID, LifecycleState: "published",
				Name: "Series"}, nil
		}
		// Source event is soft-deleted — simulates concurrent deletion.
		return &Event{ID: "source-id", ULID: sourceULID, Name: "New Instance",
			LifecycleState: "deleted",
			Occurrences:    []Occurrence{{StartTime: time.Now()}}}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, "https://toronto.togather.foundation")
	_, _, err := service.AddOccurrenceFromReviewNearDup(ctx, 1, "admin")

	if err == nil {
		t.Fatal("expected ErrEventDeleted, got nil")
	}
	if !errors.Is(err, ErrEventDeleted) {
		t.Errorf("expected ErrEventDeleted, got: %v", err)
	}
	if repo.commitCalled {
		t.Error("commit must not be called when source is deleted")
	}
}

// TestAddOccurrenceFromReviewNearDup_Overlap verifies that an overlap returns
// ErrOccurrenceOverlap and rolls back.
func TestAddOccurrenceFromReviewNearDup_Overlap(t *testing.T) {
	ctx := context.Background()
	repo := makeNearDupOccurrenceRepo("target-id", "01HTARGET00000000000000001", "01HSRC00000000000000000001", time.Now())
	repo.checkOccurrenceOverlapFunc = func(_ context.Context, _ string, _ time.Time, _ *time.Time) (bool, error) {
		return true, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, "https://toronto.togather.foundation")
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
	sourceULID := "01HSRC00000000000000000001"
	repo := makeNearDupOccurrenceRepo("target-id", "01HTARGET00000000000000001", sourceULID, time.Now())
	companionID := 42
	repo.getPendingReviewByEventUlidAndDuplicateUlidFunc = func(_ context.Context, ulid string, _ string) (*ReviewQueueEntry, error) {
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

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, "https://toronto.togather.foundation")
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
	sourceULID := "01HSRC00000000000000000001"
	companionID := 55
	repo := makeNearDupOccurrenceRepo("target-id", "01HTARGET00000000000000001", sourceULID, time.Now())
	repo.getPendingReviewByEventUlidAndDuplicateUlidFunc = func(_ context.Context, ulid string, _ string) (*ReviewQueueEntry, error) {
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
			Warnings:             []byte(`[{"code":"near_duplicate_of_new_event","message":"near_duplicate_of_new_event"}]`),
		}, nil
	}
	mergeCallIDs := []int{}
	repo.mergeReviewFunc = func(_ context.Context, id int, _ string, _ string) (*ReviewQueueEntry, error) {
		mergeCallIDs = append(mergeCallIDs, id)
		return &ReviewQueueEntry{ID: id, Status: "merged"}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, "https://toronto.togather.foundation")
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
	sourceULID := "01HSRC00000000000000000001"
	companionID := 66
	repo := makeNearDupOccurrenceRepo("target-id", "01HTARGET00000000000000001", sourceULID, time.Now())
	repo.getPendingReviewByEventUlidAndDuplicateUlidFunc = func(_ context.Context, ulid string, _ string) (*ReviewQueueEntry, error) {
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
			Warnings:             []byte(`[{"code":"near_duplicate_of_new_event","message":"near_duplicate_of_new_event"}]`),
		}, nil
	}
	mergeCallIDs := []int{}
	repo.mergeReviewFunc = func(_ context.Context, id int, _ string, _ string) (*ReviewQueueEntry, error) {
		mergeCallIDs = append(mergeCallIDs, id)
		return &ReviewQueueEntry{ID: id, Status: "merged"}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, "https://toronto.togather.foundation")
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
// unexpected DB error from GetPendingReviewByEventUlidAndDuplicateUlid (anything other than ErrNotFound)
// surfaces and prevents the transaction from committing.
func TestAddOccurrenceFromReviewNearDup_CompanionLookupUnexpectedError(t *testing.T) {
	ctx := context.Background()
	sourceULID := "01HSRC00000000000000000001"
	repo := makeNearDupOccurrenceRepo("target-id", "01HTARGET00000000000000001", sourceULID, time.Now())
	dbErr := errors.New("connection reset by peer")
	repo.getPendingReviewByEventUlidAndDuplicateUlidFunc = func(_ context.Context, ulid string, _ string) (*ReviewQueueEntry, error) {
		if ulid == sourceULID {
			return nil, dbErr
		}
		return nil, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, "https://toronto.togather.foundation")
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
	sourceULID := "01HSRC00000000000000000001"

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

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, "https://toronto.togather.foundation")
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

// TestAddOccurrenceFromReviewNearDup_MultiOccurrenceSourceRejected verifies that
// when the source event has more than one occurrence the method returns
// ErrAmbiguousOccurrenceSource without committing the transaction.
func TestAddOccurrenceFromReviewNearDup_MultiOccurrenceSourceRejected(t *testing.T) {
	ctx := context.Background()
	targetULID := "01HTARGET00000000000000001"
	sourceULID := "01HSRC00000000000000000001"
	now := time.Now()

	repo := makeNearDupOccurrenceRepo("target-id", targetULID, sourceULID, now)
	// Override the source event to have two occurrences.
	repo.getByULIDFunc = func(_ context.Context, ulid string) (*Event, error) {
		if ulid == targetULID {
			return &Event{ID: "target-id", ULID: targetULID, LifecycleState: "published"}, nil
		}
		// source event: two occurrences — ambiguous
		return &Event{
			ID:   "source-event-id",
			ULID: sourceULID,
			Name: "Multi-occurrence source",
			Occurrences: []Occurrence{
				{StartTime: now},
				{StartTime: now.Add(24 * time.Hour)},
			},
		}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, "https://toronto.togather.foundation")
	_, _, err := service.AddOccurrenceFromReviewNearDup(ctx, 1, "admin")

	if err == nil {
		t.Fatal("expected ErrAmbiguousOccurrenceSource, got nil")
	}
	if !errors.Is(err, ErrAmbiguousOccurrenceSource) {
		t.Errorf("expected ErrAmbiguousOccurrenceSource, got: %v", err)
	}
	if repo.commitCalled {
		t.Error("commit must not be called when source has multiple occurrences")
	}
}

// TestAddOccurrenceFromReviewNearDup_CompanionLockedBeforeTargetEvent verifies the
// review-first lock ordering: the companion review lock must be acquired BEFORE the
// target event lock to prevent lock-order inversion deadlocks.
func TestAddOccurrenceFromReviewNearDup_CompanionLockedBeforeTargetEvent(t *testing.T) {
	ctx := context.Background()
	sourceULID := "01HSRC00000000000000000001"
	targetULID := "01HTARGET00000000000000001"
	companionID := 99

	// Track call order via a slice.
	var callOrder []string

	repo := makeNearDupOccurrenceRepo("target-id", targetULID, sourceULID, time.Now())
	repo.getPendingReviewByEventUlidAndDuplicateUlidFunc = func(_ context.Context, _ string, _ string) (*ReviewQueueEntry, error) {
		return &ReviewQueueEntry{ID: companionID, Status: "pending"}, nil
	}
	repo.lockReviewQueueEntryForUpdateFunc = func(_ context.Context, id int) (*ReviewQueueEntry, error) {
		if id == companionID {
			callOrder = append(callOrder, "lock-companion-review")
			return &ReviewQueueEntry{ID: id, Status: "pending"}, nil
		}
		// primary review
		callOrder = append(callOrder, "lock-primary-review")
		return &ReviewQueueEntry{
			ID:                   id,
			Status:               "pending",
			EventULID:            targetULID,
			DuplicateOfEventULID: &sourceULID,
			Warnings:             []byte(`[{"code":"near_duplicate_of_new_event","message":"near_duplicate_of_new_event"}]`),
		}, nil
	}
	repo.lockEventForUpdateFunc = func(_ context.Context, _ string) error {
		callOrder = append(callOrder, "lock-target-event")
		return nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, "https://toronto.togather.foundation")
	_, _, err := service.AddOccurrenceFromReviewNearDup(ctx, 1, "admin")

	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}

	// Verify ordering: primary review → companion review → target event
	want := []string{"lock-primary-review", "lock-companion-review", "lock-target-event"}
	if len(callOrder) < 3 {
		t.Fatalf("expected at least 3 lock calls, got %d: %v", len(callOrder), callOrder)
	}
	for i, w := range want {
		if callOrder[i] != w {
			t.Errorf("lock ordering violation: position %d want %q got %q (full order: %v)",
				i, w, callOrder[i], callOrder)
		}
	}
}

// TestAddOccurrenceFromReviewNearDup_ZeroOccurrenceSourceRejected verifies that
// when the source (newly-ingested) event has zero occurrences the method returns
// ErrZeroOccurrenceSource without committing.  Prior to this fix the method fell
// back to review.EventStartTime which belongs to the *target* (existing series),
// not the source — using it would absorb the wrong date into the series.
func TestAddOccurrenceFromReviewNearDup_ZeroOccurrenceSourceRejected(t *testing.T) {
	ctx := context.Background()
	targetULID := "01HTARGET00000000000000001"
	sourceULID := "01HSRC00000000000000000001"

	repo := makeNearDupOccurrenceRepo("target-id", targetULID, sourceULID, time.Now())
	// Override source to return zero occurrences.
	repo.getByULIDFunc = func(_ context.Context, ulid string) (*Event, error) {
		if ulid == targetULID {
			return &Event{ID: "target-id", ULID: targetULID, LifecycleState: "published"}, nil
		}
		return &Event{
			ID:          "source-event-id",
			ULID:        sourceULID,
			Name:        "Zero-Occurrence Source",
			Occurrences: []Occurrence{}, // no occurrences
		}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, "https://toronto.togather.foundation")
	_, _, err := service.AddOccurrenceFromReviewNearDup(ctx, 1, "admin")

	if err == nil {
		t.Fatal("expected ErrZeroOccurrenceSource, got nil")
	}
	if !errors.Is(err, ErrZeroOccurrenceSource) {
		t.Errorf("expected ErrZeroOccurrenceSource, got: %v", err)
	}
	if repo.commitCalled {
		t.Error("commit must not be called when source has no occurrences")
	}
}

// makeWarningsJSON builds a minimal warnings JSON blob for the given warning codes.
func makeWarningsJSON(codes ...string) []byte {
	type w struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	warnings := make([]w, 0, len(codes))
	for _, c := range codes {
		warnings = append(warnings, w{Code: c, Message: c})
	}
	b, _ := json.Marshal(warnings)
	return b
}

// ── Fix 1: source event locking ──────────────────────────────────────────────

// TestAddOccurrenceFromReview_SourceEventLockedBeforeOccurrenceRead verifies
// that the source event is locked (via LockEventForUpdate) before the
// create-occurrence call, preventing a TOCTOU race on occurrence timestamps.
func TestAddOccurrenceFromReview_SourceEventLockedBeforeOccurrenceRead(t *testing.T) {
	ctx := context.Background()
	startTime := time.Now()
	targetULID := "01HTARGET00000000000000001"
	reviewEventULID := "01HREV00000000000000000001"

	var callOrder []string
	repo := makeOccurrenceRepo("target-uuid", targetULID, reviewEventULID, startTime)

	repo.lockEventForUpdateFunc = func(_ context.Context, _ string) error {
		callOrder = append(callOrder, "lock")
		return nil
	}
	repo.createOccurrenceFunc = func(_ context.Context, _ OccurrenceCreateParams) error {
		callOrder = append(callOrder, "create-occurrence")
		return nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
	_, err := service.AddOccurrenceFromReview(ctx, 1, targetULID, "admin")

	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	// Lock must be called before create-occurrence.
	lockIdx, createIdx := -1, -1
	for i, s := range callOrder {
		if s == "lock" && lockIdx == -1 {
			lockIdx = i
		}
		if s == "create-occurrence" && createIdx == -1 {
			createIdx = i
		}
	}
	if lockIdx == -1 {
		t.Fatal("LockEventForUpdate was never called")
	}
	if createIdx == -1 {
		t.Fatal("createOccurrence was never called")
	}
	if lockIdx >= createIdx {
		t.Errorf("lock must be called before create-occurrence; order: %v", callOrder)
	}
}

// TestAddOccurrenceFromReview_UsesPostLockSourceTimestamps verifies that when
// getByULID is called a second time (post-lock re-read) and returns different
// occurrence data, the post-lock data wins.
func TestAddOccurrenceFromReview_UsesPostLockSourceTimestamps(t *testing.T) {
	ctx := context.Background()
	targetULID := "01HTARGET00000000000000001"
	reviewEventULID := "01HREV00000000000000000001"

	preLockTime := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	postLockTime := time.Date(2025, 3, 15, 18, 0, 0, 0, time.UTC)

	var capturedStart time.Time
	getCallCount := 0

	repo := makeOccurrenceRepo("target-uuid", targetULID, reviewEventULID, preLockTime)
	repo.getByULIDFunc = func(_ context.Context, ulid string) (*Event, error) {
		if ulid == targetULID {
			return &Event{ID: "target-uuid", ULID: targetULID, Name: "Series", LifecycleState: "published"}, nil
		}
		// Source event: return different timestamps on second read (post-lock).
		getCallCount++
		st := preLockTime
		if getCallCount > 1 {
			st = postLockTime
		}
		return &Event{
			ID:          "review-event-id",
			ULID:        reviewEventULID,
			Name:        "Instance",
			Occurrences: []Occurrence{{StartTime: st}},
		}, nil
	}
	repo.createOccurrenceFunc = func(_ context.Context, p OccurrenceCreateParams) error {
		capturedStart = p.StartTime
		return nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
	_, err := service.AddOccurrenceFromReview(ctx, 1, targetULID, "admin")

	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if !capturedStart.Equal(postLockTime) {
		t.Errorf("expected post-lock timestamp %v, got %v", postLockTime, capturedStart)
	}
}

// TestAddOccurrenceFromReviewNearDup_SourceEventLockedBeforeOccurrenceRead
// verifies that the source event is locked before the create-occurrence call on
// the near-dup path.
func TestAddOccurrenceFromReviewNearDup_SourceEventLockedBeforeOccurrenceRead(t *testing.T) {
	ctx := context.Background()
	targetULID := "01HTARGET00000000000000001"
	sourceULID := "01HSRC00000000000000000001"

	var callOrder []string
	repo := makeNearDupOccurrenceRepo("target-id", targetULID, sourceULID, time.Now())

	repo.lockEventForUpdateFunc = func(_ context.Context, _ string) error {
		callOrder = append(callOrder, "lock")
		return nil
	}
	repo.createOccurrenceFunc = func(_ context.Context, _ OccurrenceCreateParams) error {
		callOrder = append(callOrder, "create-occurrence")
		return nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, "https://toronto.togather.foundation")
	_, _, err := service.AddOccurrenceFromReviewNearDup(ctx, 1, "admin")

	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	// Find the LAST lock call (source lock) and ensure it precedes create-occurrence.
	lastLockIdx, createIdx := -1, -1
	for i, s := range callOrder {
		if s == "lock" {
			lastLockIdx = i
		}
		if s == "create-occurrence" && createIdx == -1 {
			createIdx = i
		}
	}
	if lastLockIdx == -1 {
		t.Fatal("LockEventForUpdate was never called")
	}
	if createIdx == -1 {
		t.Fatal("createOccurrence was never called")
	}
	if lastLockIdx >= createIdx {
		t.Errorf("source lock must precede create-occurrence; order: %v", callOrder)
	}
}

// TestAddOccurrenceFromReviewNearDup_UsesPostLockSourceTimestamps verifies that
// the post-lock re-read of the source event is used for occurrence data on the
// near-dup path.
func TestAddOccurrenceFromReviewNearDup_UsesPostLockSourceTimestamps(t *testing.T) {
	ctx := context.Background()
	targetULID := "01HTARGET00000000000000001"
	sourceULID := "01HSRC00000000000000000001"

	preLockTime := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	postLockTime := time.Date(2025, 3, 15, 18, 0, 0, 0, time.UTC)

	var capturedStart time.Time
	getCallCount := 0

	repo := makeNearDupOccurrenceRepo("target-id", targetULID, sourceULID, preLockTime)
	repo.getByULIDFunc = func(_ context.Context, ulid string) (*Event, error) {
		if ulid == targetULID {
			return &Event{ID: "target-id", ULID: targetULID, Name: "Series", LifecycleState: "published"}, nil
		}
		// Source event: return different timestamps on second read (post-lock).
		getCallCount++
		st := preLockTime
		if getCallCount > 1 {
			st = postLockTime
		}
		return &Event{
			ID:          "source-event-id",
			ULID:        sourceULID,
			Name:        "New Instance",
			Occurrences: []Occurrence{{StartTime: st}},
		}, nil
	}
	repo.createOccurrenceFunc = func(_ context.Context, p OccurrenceCreateParams) error {
		capturedStart = p.StartTime
		return nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, "https://toronto.togather.foundation")
	_, _, err := service.AddOccurrenceFromReviewNearDup(ctx, 1, "admin")

	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if !capturedStart.Equal(postLockTime) {
		t.Errorf("expected post-lock timestamp %v, got %v", postLockTime, capturedStart)
	}
}

// ── Fix 2: stale warning-based path selection ─────────────────────────────────

// TestAddOccurrenceFromReview_WrongPathWhenLockedWarningsIndicateNearDup
// verifies that if the locked review entry has near_duplicate_of_new_event
// warnings, AddOccurrenceFromReview returns ErrWrongOccurrencePath.
func TestAddOccurrenceFromReview_WrongPathWhenLockedWarningsIndicateNearDup(t *testing.T) {
	ctx := context.Background()
	startTime := time.Now()
	repo := makeOccurrenceRepo("target-uuid", "01HTARGET00000000000000001", "01HREV00000000000000000001", startTime)
	repo.lockReviewQueueEntryForUpdateFunc = func(_ context.Context, id int) (*ReviewQueueEntry, error) {
		return &ReviewQueueEntry{
			ID:       id,
			Status:   "pending",
			Warnings: makeWarningsJSON("near_duplicate_of_new_event"),
		}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
	_, err := service.AddOccurrenceFromReview(ctx, 1, "01HTARGET00000000000000001", "admin")

	if err == nil {
		t.Fatal("expected ErrWrongOccurrencePath, got nil")
	}
	if !errors.Is(err, ErrWrongOccurrencePath) {
		t.Errorf("expected ErrWrongOccurrencePath, got: %v", err)
	}
	if repo.commitCalled {
		t.Error("commit must not be called on wrong-path error")
	}
}

// TestAddOccurrenceFromReview_AmbiguousDispatchFromLockedWarnings verifies that
// if the locked review entry has both potential_duplicate and
// near_duplicate_of_new_event warnings, AddOccurrenceFromReview returns
// ErrAmbiguousOccurrenceDispatch.
func TestAddOccurrenceFromReview_AmbiguousDispatchFromLockedWarnings(t *testing.T) {
	ctx := context.Background()
	startTime := time.Now()
	repo := makeOccurrenceRepo("target-uuid", "01HTARGET00000000000000001", "01HREV00000000000000000001", startTime)
	repo.lockReviewQueueEntryForUpdateFunc = func(_ context.Context, id int) (*ReviewQueueEntry, error) {
		return &ReviewQueueEntry{
			ID:       id,
			Status:   "pending",
			Warnings: makeWarningsJSON("potential_duplicate", "near_duplicate_of_new_event"),
		}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
	_, err := service.AddOccurrenceFromReview(ctx, 1, "01HTARGET00000000000000001", "admin")

	if err == nil {
		t.Fatal("expected ErrAmbiguousOccurrenceDispatch, got nil")
	}
	if !errors.Is(err, ErrAmbiguousOccurrenceDispatch) {
		t.Errorf("expected ErrAmbiguousOccurrenceDispatch, got: %v", err)
	}
	if repo.commitCalled {
		t.Error("commit must not be called on ambiguous dispatch error")
	}
}

// TestAddOccurrenceFromReview_ForwardPathRejectsNoWarnings verifies that a
// review entry with nil/empty warnings is rejected by AddOccurrenceFromReview
// with ErrUnsupportedReviewForOccurrence.  The forward path requires a
// potential_duplicate warning; reviews carrying only quality/completeness warnings
// (or none at all) must not be silently routed through the forward path.
func TestAddOccurrenceFromReview_ForwardPathRejectsNoWarnings(t *testing.T) {
	ctx := context.Background()
	startTime := time.Now()
	repo := makeOccurrenceRepo("target-uuid", "01HTARGET00000000000000001", "01HREV00000000000000000001", startTime)
	// Override the lock to return a review with nil Warnings (no duplicate warning).
	repo.lockReviewQueueEntryForUpdateFunc = func(_ context.Context, id int) (*ReviewQueueEntry, error) {
		return &ReviewQueueEntry{
			ID:             id,
			EventULID:      "01HREV00000000000000000001",
			Status:         "pending",
			EventStartTime: startTime,
			Warnings:       nil,
		}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
	_, err := service.AddOccurrenceFromReview(ctx, 1, "01HTARGET00000000000000001", "admin")

	if err == nil {
		t.Fatal("expected ErrUnsupportedReviewForOccurrence, got nil")
	}
	if !errors.Is(err, ErrUnsupportedReviewForOccurrence) {
		t.Errorf("expected ErrUnsupportedReviewForOccurrence, got: %v", err)
	}
	if repo.commitCalled {
		t.Error("commit must not be called when occurrence path is unsupported")
	}
}

// TestAddOccurrenceFromReview_ForwardPathAcceptsPotentialDuplicateWarning
// verifies that a potential_duplicate warning (without near_duplicate_of_new_event)
// is accepted by AddOccurrenceFromReview as a valid forward-path review.
func TestAddOccurrenceFromReview_ForwardPathAcceptsPotentialDuplicateWarning(t *testing.T) {
	ctx := context.Background()
	startTime := time.Now()
	repo := makeOccurrenceRepo("target-uuid", "01HTARGET00000000000000001", "01HREV00000000000000000001", startTime)
	repo.lockReviewQueueEntryForUpdateFunc = func(_ context.Context, id int) (*ReviewQueueEntry, error) {
		return &ReviewQueueEntry{
			ID:       id,
			Status:   "pending",
			Warnings: makeWarningsJSON("potential_duplicate"),
		}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
	_, err := service.AddOccurrenceFromReview(ctx, 1, "01HTARGET00000000000000001", "admin")

	if err != nil {
		t.Fatalf("expected success with potential_duplicate warning, got: %v", err)
	}
}

// TestAddOccurrenceFromReviewNearDup_WrongPathWhenLockedWarningsMissing verifies
// that if the locked review entry has no near_duplicate_of_new_event warning,
// AddOccurrenceFromReviewNearDup returns ErrUnsupportedReviewForOccurrence (when
// the warning is completely absent) rather than silently proceeding.
func TestAddOccurrenceFromReviewNearDup_WrongPathWhenLockedWarningsMissing(t *testing.T) {
	ctx := context.Background()
	repo := makeNearDupOccurrenceRepo("target-id", "01HTARGET00000000000000001", "01HSRC00000000000000000001", time.Now())
	repo.lockReviewQueueEntryForUpdateFunc = func(_ context.Context, id int) (*ReviewQueueEntry, error) {
		dup := "01HSRC00000000000000000001"
		return &ReviewQueueEntry{
			ID:                   id,
			Status:               "pending",
			EventULID:            "01HTARGET00000000000000001",
			DuplicateOfEventULID: &dup,
			Warnings:             nil, // no near_duplicate_of_new_event warning
		}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, "https://toronto.togather.foundation")
	_, _, err := service.AddOccurrenceFromReviewNearDup(ctx, 1, "admin")

	if err == nil {
		t.Fatal("expected error for missing near_duplicate_of_new_event warning, got nil")
	}
	if !errors.Is(err, ErrUnsupportedReviewForOccurrence) {
		t.Errorf("expected ErrUnsupportedReviewForOccurrence, got: %v", err)
	}
	if repo.commitCalled {
		t.Error("commit must not be called when near-dup path is unsupported")
	}
}

// TestAddOccurrenceFromReviewNearDup_WrongPathWhenLockedWarningsForwardOnly verifies
// that if the locked review entry has only a potential_duplicate warning (forward-path),
// AddOccurrenceFromReviewNearDup returns ErrWrongOccurrencePath.
func TestAddOccurrenceFromReviewNearDup_WrongPathWhenLockedWarningsForwardOnly(t *testing.T) {
	ctx := context.Background()
	repo := makeNearDupOccurrenceRepo("target-id", "01HTARGET00000000000000001", "01HSRC00000000000000000001", time.Now())
	repo.lockReviewQueueEntryForUpdateFunc = func(_ context.Context, id int) (*ReviewQueueEntry, error) {
		dup := "01HSRC00000000000000000001"
		return &ReviewQueueEntry{
			ID:                   id,
			Status:               "pending",
			EventULID:            "01HTARGET00000000000000001",
			DuplicateOfEventULID: &dup,
			Warnings:             makeWarningsJSON("potential_duplicate"),
		}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, "https://toronto.togather.foundation")
	_, _, err := service.AddOccurrenceFromReviewNearDup(ctx, 1, "admin")

	if err == nil {
		t.Fatal("expected ErrWrongOccurrencePath, got nil")
	}
	if !errors.Is(err, ErrWrongOccurrencePath) {
		t.Errorf("expected ErrWrongOccurrencePath, got: %v", err)
	}
	if repo.commitCalled {
		t.Error("commit must not be called on wrong-path error")
	}
}

// TestAddOccurrenceFromReviewNearDup_AmbiguousDispatchFromLockedWarnings
// verifies that if the locked review entry has both potential_duplicate and
// near_duplicate_of_new_event warnings, AddOccurrenceFromReviewNearDup returns
// ErrAmbiguousOccurrenceDispatch.
func TestAddOccurrenceFromReviewNearDup_AmbiguousDispatchFromLockedWarnings(t *testing.T) {
	ctx := context.Background()
	repo := makeNearDupOccurrenceRepo("target-id", "01HTARGET00000000000000001", "01HSRC00000000000000000001", time.Now())
	repo.lockReviewQueueEntryForUpdateFunc = func(_ context.Context, id int) (*ReviewQueueEntry, error) {
		dup := "01HSRC00000000000000000001"
		return &ReviewQueueEntry{
			ID:                   id,
			Status:               "pending",
			EventULID:            "01HTARGET00000000000000001",
			DuplicateOfEventULID: &dup,
			Warnings:             makeWarningsJSON("potential_duplicate", "near_duplicate_of_new_event"),
		}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, "https://toronto.togather.foundation")
	_, _, err := service.AddOccurrenceFromReviewNearDup(ctx, 1, "admin")

	if err == nil {
		t.Fatal("expected ErrAmbiguousOccurrenceDispatch, got nil")
	}
	if !errors.Is(err, ErrAmbiguousOccurrenceDispatch) {
		t.Errorf("expected ErrAmbiguousOccurrenceDispatch, got: %v", err)
	}
	if repo.commitCalled {
		t.Error("commit must not be called on ambiguous dispatch error")
	}
}

// TestMergeEventsWithReview_ConflictOnAlreadyProcessed verifies that when the
// review row is already processed, MergeEventsWithReview returns ErrConflict.
func TestMergeEventsWithReview_ConflictOnAlreadyProcessed(t *testing.T) {
	ctx := context.Background()
	repo := makeReviewLockRepo("01HDPCT0000000000000000001")
	repo.lockReviewQueueEntryForUpdateFunc = func(_ context.Context, id int) (*ReviewQueueEntry, error) {
		return &ReviewQueueEntry{ID: id, Status: "merged"}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
	_, err := service.MergeEventsWithReview(ctx, MergeEventsParams{
		PrimaryULID:   "01HPRMARY00000000000000001",
		DuplicateULID: "01HDPCT0000000000000000001",
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
	repo := makeReviewLockRepo("01HDPCT0000000000000000001")
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

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
	_, err := service.MergeEventsWithReview(ctx, MergeEventsParams{
		PrimaryULID:   "01HPRMARY00000000000000001",
		DuplicateULID: "01HDPCT0000000000000000001",
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

// ---------------------------------------------------------------------------
// Regression tests: canonical EventURI / SupersededBy in add-occurrence paths
// (srv-izykp) — guards against eventURI() silently falling back to a bare ULID.
// ---------------------------------------------------------------------------

// TestAddOccurrenceFromReview_TombstoneUsesCanonicalURIs verifies that
// CreateTombstone receives full canonical https:// URIs (not bare ULIDs) for
// both EventURI and SupersededBy in the forward add-occurrence path.
func TestAddOccurrenceFromReview_TombstoneUsesCanonicalURIs(t *testing.T) {
	ctx := context.Background()
	startTime := time.Now()

	const baseURL = "https://toronto.togather.foundation"
	// Use valid 26-char Crockford Base32 ULIDs (no I/L/O/U).
	const reviewEventULID = "01HREV00000000000000000001"
	const targetEventULID = "01HTARGET00000000000000001"

	wantEventURI := "https://toronto.togather.foundation/events/" + reviewEventULID
	wantSupersedesURI := "https://toronto.togather.foundation/events/" + targetEventULID

	var capturedParams TombstoneCreateParams
	repo := makeOccurrenceRepo("target-uuid", targetEventULID, reviewEventULID, startTime)
	repo.createTombstoneFunc = func(_ context.Context, p TombstoneCreateParams) error {
		capturedParams = p
		return nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, baseURL)
	_, err := service.AddOccurrenceFromReview(ctx, 1, targetEventULID, "admin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedParams.EventURI != wantEventURI {
		t.Errorf("EventURI: got %q, want %q", capturedParams.EventURI, wantEventURI)
	}
	if capturedParams.SupersededBy == nil {
		t.Fatal("SupersededBy must not be nil for absorbed_as_occurrence tombstone")
	}
	if *capturedParams.SupersededBy != wantSupersedesURI {
		t.Errorf("SupersededBy: got %q, want %q", *capturedParams.SupersededBy, wantSupersedesURI)
	}
}

// TestAddOccurrenceFromReviewNearDup_TombstoneUsesCanonicalURIs verifies that
// CreateTombstone receives full canonical https:// URIs (not bare ULIDs) for
// both EventURI and SupersededBy in the near-dup add-occurrence path.
func TestAddOccurrenceFromReviewNearDup_TombstoneUsesCanonicalURIs(t *testing.T) {
	ctx := context.Background()
	startTime := time.Now()

	const baseURL = "https://toronto.togather.foundation"
	// Use valid 26-char Crockford Base32 ULIDs (no I/L/O/U).
	const targetULID = "01HTARGET00000000000000001"
	const sourceULID = "01HSRC00000000000000000001"

	wantEventURI := "https://toronto.togather.foundation/events/" + sourceULID
	wantSupersedesURI := "https://toronto.togather.foundation/events/" + targetULID

	var capturedParams TombstoneCreateParams
	repo := makeNearDupOccurrenceRepo("target-id", targetULID, sourceULID, startTime)
	repo.createTombstoneFunc = func(_ context.Context, p TombstoneCreateParams) error {
		capturedParams = p
		return nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, baseURL)
	_, _, err := service.AddOccurrenceFromReviewNearDup(ctx, 1, "admin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedParams.EventURI != wantEventURI {
		t.Errorf("EventURI: got %q, want %q", capturedParams.EventURI, wantEventURI)
	}
	if capturedParams.SupersededBy == nil {
		t.Fatal("SupersededBy must not be nil for absorbed_as_occurrence tombstone")
	}
	if *capturedParams.SupersededBy != wantSupersedesURI {
		t.Errorf("SupersededBy: got %q, want %q", *capturedParams.SupersededBy, wantSupersedesURI)
	}
}

// TestNewAdminService_PanicsOnEmptyBaseURL verifies fail-fast behavior: an
// empty baseURL is a misconfiguration that must be caught at startup rather
// than silently producing invalid SEL tombstone URIs at runtime.
func TestNewAdminService_PanicsOnEmptyBaseURL(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for empty baseURL, but no panic occurred")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("expected panic value to be a string, got %T: %v", r, r)
		}
		if msg == "" {
			t.Fatal("panic message must not be empty")
		}
	}()
	NewAdminService(&mockTransactionalRepo{}, false, "America/Toronto", config.ValidationConfig{}, "")
}

// ── Malformed warnings JSON → ErrMalformedWarnings (data-integrity fault) ─────

// TestAddOccurrenceFromReview_MalformedWarningsJSON verifies that when the locked
// review entry's Warnings column contains invalid JSON, AddOccurrenceFromReview
// returns ErrMalformedWarnings rather than silently routing to the "unsupported"
// path (which would produce a misleading ErrUnsupportedReviewForOccurrence 422).
func TestAddOccurrenceFromReview_MalformedWarningsJSON(t *testing.T) {
	ctx := context.Background()
	startTime := time.Now()
	repo := makeOccurrenceRepo("target-uuid", "01HTARGET00000000000000001", "01HREV00000000000000000001", startTime)
	repo.lockReviewQueueEntryForUpdateFunc = func(_ context.Context, id int) (*ReviewQueueEntry, error) {
		return &ReviewQueueEntry{
			ID:       id,
			Status:   "pending",
			Warnings: []byte(`not-valid-json`),
		}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
	_, err := service.AddOccurrenceFromReview(ctx, 1, "01HTARGET00000000000000001", "admin")

	if err == nil {
		t.Fatal("expected ErrMalformedWarnings, got nil")
	}
	if !errors.Is(err, ErrMalformedWarnings) {
		t.Errorf("expected ErrMalformedWarnings, got: %v", err)
	}
	if repo.commitCalled {
		t.Error("commit must not be called when warnings JSON is malformed")
	}
}

// TestAddOccurrenceFromReviewNearDup_MalformedWarningsJSON verifies that when the
// locked review entry's Warnings column contains invalid JSON,
// AddOccurrenceFromReviewNearDup returns ErrMalformedWarnings rather than silently
// routing to the "unsupported" path.
func TestAddOccurrenceFromReviewNearDup_MalformedWarningsJSON(t *testing.T) {
	ctx := context.Background()
	repo := makeNearDupOccurrenceRepo(
		"target-uuid", "01HTARGET00000000000000001",
		"01HSRC00000000000000000001",
		time.Now(),
	)
	repo.lockReviewQueueEntryForUpdateFunc = func(_ context.Context, id int) (*ReviewQueueEntry, error) {
		return &ReviewQueueEntry{
			ID:       id,
			Status:   "pending",
			Warnings: []byte(`not-valid-json`),
		}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
	_, _, err := service.AddOccurrenceFromReviewNearDup(ctx, 1, "admin")

	if err == nil {
		t.Fatal("expected ErrMalformedWarnings, got nil")
	}
	if !errors.Is(err, ErrMalformedWarnings) {
		t.Errorf("expected ErrMalformedWarnings, got: %v", err)
	}
	if repo.commitCalled {
		t.Error("commit must not be called when warnings JSON is malformed")
	}
}

// ── Fix: lifecycle restoration after add-occurrence ──────────────────────────

// TestAddOccurrenceFromReview_RestoresTargetLifecycleFromPendingReview verifies
// that the forward path restores the target event's lifecycle_state from
// pending_review to published after a successful add-occurrence operation.
// Bug: target events demoted to pending_review during near-dup ingest were
// never restored — they remained stuck and invisible to the public API.
func TestAddOccurrenceFromReview_RestoresTargetLifecycleFromPendingReview(t *testing.T) {
	ctx := context.Background()
	startTime := time.Now()

	var lifecycleUpdated bool
	var capturedLifecycle string

	repo := makeOccurrenceRepo("target-uuid", "01HTARGET00000000000000001", "01HREV00000000000000000001", startTime)
	repo.getByULIDFunc = func(_ context.Context, ulid string) (*Event, error) {
		if ulid == "01HTARGET00000000000000001" {
			return &Event{ID: "target-uuid", ULID: ulid, Name: "Series", LifecycleState: "pending_review"}, nil
		}
		return &Event{ID: "review-event-id", ULID: ulid, Name: "Instance",
			Occurrences: []Occurrence{{StartTime: startTime}}}, nil
	}
	repo.updateEventFunc = func(_ context.Context, ulid string, params UpdateEventParams) (*Event, error) {
		if params.LifecycleState != nil {
			lifecycleUpdated = true
			capturedLifecycle = *params.LifecycleState
		}
		return &Event{ULID: ulid, LifecycleState: "published"}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
	_, err := service.AddOccurrenceFromReview(ctx, 1, "01HTARGET00000000000000001", "admin")

	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if !lifecycleUpdated {
		t.Error("UpdateEvent must be called to restore target lifecycle from pending_review")
	}
	if capturedLifecycle != "published" {
		t.Errorf("lifecycle should be restored to 'published', got %q", capturedLifecycle)
	}
	if !repo.commitCalled {
		t.Error("commit must be called on success")
	}
}

// TestAddOccurrenceFromReview_SkipsLifecycleRestorationWhenNotPendingReview
// verifies that lifecycle restoration is skipped when the target is already
// published (no unnecessary UpdateEvent call).
func TestAddOccurrenceFromReview_SkipsLifecycleRestorationWhenNotPendingReview(t *testing.T) {
	ctx := context.Background()
	startTime := time.Now()

	var lifecycleUpdated bool

	repo := makeOccurrenceRepo("target-uuid", "01HTARGET00000000000000001", "01HREV00000000000000000001", startTime)
	// Default makeOccurrenceRepo sets LifecycleState to "published"
	repo.updateEventFunc = func(_ context.Context, _ string, params UpdateEventParams) (*Event, error) {
		if params.LifecycleState != nil {
			lifecycleUpdated = true
		}
		return &Event{ULID: "01HTARGET00000000000000001"}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
	_, err := service.AddOccurrenceFromReview(ctx, 1, "01HTARGET00000000000000001", "admin")

	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if lifecycleUpdated {
		t.Error("UpdateEvent must NOT be called for lifecycle restoration when target is already published")
	}
}

// TestAddOccurrenceFromReviewNearDup_RestoresTargetLifecycleFromPendingReview
// verifies that the near-dup path restores the target event's lifecycle_state
// from pending_review to published after a successful add-occurrence operation.
func TestAddOccurrenceFromReviewNearDup_RestoresTargetLifecycleFromPendingReview(t *testing.T) {
	ctx := context.Background()
	startTime := time.Now()

	var lifecycleUpdated bool
	var capturedLifecycle string

	repo := makeNearDupOccurrenceRepo("target-id", "01HTARGET00000000000000001", "01HSRC00000000000000000001", startTime)
	repo.getByULIDFunc = func(_ context.Context, ulid string) (*Event, error) {
		if ulid == "01HTARGET00000000000000001" {
			return &Event{ID: "target-id", ULID: ulid, Name: "Series", LifecycleState: "pending_review"}, nil
		}
		return &Event{ID: "source-event-id", ULID: ulid, Name: "New Instance",
			Occurrences: []Occurrence{{StartTime: startTime}}}, nil
	}
	repo.updateEventFunc = func(_ context.Context, ulid string, params UpdateEventParams) (*Event, error) {
		if params.LifecycleState != nil {
			lifecycleUpdated = true
			capturedLifecycle = *params.LifecycleState
		}
		return &Event{ULID: ulid, LifecycleState: "published"}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, "https://toronto.togather.foundation")
	_, _, err := service.AddOccurrenceFromReviewNearDup(ctx, 1, "admin")

	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if !lifecycleUpdated {
		t.Error("UpdateEvent must be called to restore target lifecycle from pending_review")
	}
	if capturedLifecycle != "published" {
		t.Errorf("lifecycle should be restored to 'published', got %q", capturedLifecycle)
	}
	if !repo.commitCalled {
		t.Error("commit must be called on success")
	}
}

// TestAddOccurrenceFromReview_KeepsTargetPendingWhenOtherReviewsRemain verifies
// that the forward path does NOT restore the target event's lifecycle to
// "published" when other pending review rows still exist after the companion is
// dismissed.  This prevents a target with unresolved review issues (e.g. a
// second flagged occurrence on the same series) from becoming publicly visible
// before all issues are resolved.
func TestAddOccurrenceFromReview_KeepsTargetPendingWhenOtherReviewsRemain(t *testing.T) {
	ctx := context.Background()
	startTime := time.Now()

	var lifecycleUpdated bool

	repo := makeOccurrenceRepo("target-uuid", "01HTARGET00000000000000001", "01HREV00000000000000000001", startTime)
	// Target is in pending_review state.
	repo.getByULIDFunc = func(_ context.Context, ulid string) (*Event, error) {
		if ulid == "01HTARGET00000000000000001" {
			return &Event{ID: "target-uuid", ULID: ulid, Name: "Series", LifecycleState: "pending_review"}, nil
		}
		return &Event{ID: "review-event-id", ULID: ulid, Name: "Instance",
			Occurrences: []Occurrence{{StartTime: startTime}}}, nil
	}
	// First: companion lookup via new func (targetULID) — returns no companion.
	// Then: lifecycle recheck via old func (targetULID) — returns another pending review.
	otherReviewID := 77
	repo.getPendingReviewByEventUlidAndDuplicateUlidFunc = func(_ context.Context, ulid string, _ string) (*ReviewQueueEntry, error) {
		// No companion review on target.
		return nil, ErrNotFound
	}
	repo.getPendingReviewByEventUlidFunc = func(_ context.Context, ulid string) (*ReviewQueueEntry, error) {
		if ulid != "01HTARGET00000000000000001" {
			return nil, ErrNotFound
		}
		// Post-action recompute: another review still pending.
		return &ReviewQueueEntry{
			ID:        otherReviewID,
			EventULID: "01HTARGET00000000000000001",
			Status:    "pending",
			Warnings:  makeWarningsJSON("potential_duplicate"),
		}, nil
	}
	repo.updateEventFunc = func(_ context.Context, _ string, params UpdateEventParams) (*Event, error) {
		if params.LifecycleState != nil {
			lifecycleUpdated = true
		}
		return &Event{ULID: "01HTARGET00000000000000001"}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
	_, err := service.AddOccurrenceFromReview(ctx, 1, "01HTARGET00000000000000001", "admin")

	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if lifecycleUpdated {
		t.Error("UpdateEvent must NOT be called to restore lifecycle when other pending reviews remain")
	}
	if !repo.commitCalled {
		t.Error("commit must be called on success")
	}
}

// TestAddOccurrenceFromReviewNearDup_KeepsTargetPendingWhenOtherReviewsRemain
// verifies that the near-dup path does NOT restore the target event's lifecycle
// to "published" when other pending review rows still exist after the near-dup
// review is dismissed.
//
// In the near-dup path the companion lookup uses sourceEventULID (not
// targetEventULID), so the post-action recompute call is the FIRST call with
// targetEventULID.  The mock returns a pending review on that first call to
// simulate another unresolved issue on the target.
func TestAddOccurrenceFromReviewNearDup_KeepsTargetPendingWhenOtherReviewsRemain(t *testing.T) {
	ctx := context.Background()
	startTime := time.Now()

	var lifecycleUpdated bool
	otherReviewID := 88

	repo := makeNearDupOccurrenceRepo("target-id", "01HTARGET00000000000000001", "01HSRC00000000000000000001", startTime)
	// Target is in pending_review state.
	repo.getByULIDFunc = func(_ context.Context, ulid string) (*Event, error) {
		if ulid == "01HTARGET00000000000000001" {
			return &Event{ID: "target-id", ULID: ulid, Name: "Series", LifecycleState: "pending_review"}, nil
		}
		return &Event{ID: "source-event-id", ULID: ulid, Name: "New Instance",
			Occurrences: []Occurrence{{StartTime: startTime}}}, nil
	}
	// Near-dup path companion lookup uses new func with sourceEventULID — no companion.
	// Post-action recompute uses old func with targetEventULID — another review still pending.
	repo.getPendingReviewByEventUlidAndDuplicateUlidFunc = func(_ context.Context, _ string, _ string) (*ReviewQueueEntry, error) {
		return nil, ErrNotFound
	}
	repo.getPendingReviewByEventUlidFunc = func(_ context.Context, ulid string) (*ReviewQueueEntry, error) {
		// Post-action recompute on target: another review still pending.
		return &ReviewQueueEntry{
			ID:        otherReviewID,
			EventULID: "01HTARGET00000000000000001",
			Status:    "pending",
			Warnings:  makeWarningsJSON("potential_duplicate"),
		}, nil
	}
	repo.updateEventFunc = func(_ context.Context, _ string, params UpdateEventParams) (*Event, error) {
		if params.LifecycleState != nil {
			lifecycleUpdated = true
		}
		return &Event{ULID: "01HTARGET00000000000000001"}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, "https://toronto.togather.foundation")
	_, _, err := service.AddOccurrenceFromReviewNearDup(ctx, 1, "admin")

	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if lifecycleUpdated {
		t.Error("UpdateEvent must NOT be called to restore lifecycle when other pending reviews remain")
	}
	if !repo.commitCalled {
		t.Error("commit must be called on success")
	}
}

// ── Fix: forward path companion review dismissal ─────────────────────────────

// TestAddOccurrenceFromReview_DismissesCompanionReview verifies that the forward
// path finds and dismisses the companion review entry on the target event (the
// near-dup review created by ingest on the existing event, cross-linked to the
// source event being absorbed).  Without this fix, the companion review remained
// orphaned in the queue pointing at the now-deleted source event.
func TestAddOccurrenceFromReview_DismissesCompanionReview(t *testing.T) {
	ctx := context.Background()
	startTime := time.Now()

	var companionDismissed bool
	companionID := 99

	repo := makeOccurrenceRepo("target-uuid", "01HTARGET00000000000000001", "01HREV00000000000000000001", startTime)
	// Simulate a companion review entry on the target event, cross-linked to the
	// source/review event (01HREV00000000000000000001).
	repo.getPendingReviewByEventUlidAndDuplicateUlidFunc = func(_ context.Context, ulid string, _ string) (*ReviewQueueEntry, error) {
		if ulid == "01HTARGET00000000000000001" {
			return &ReviewQueueEntry{
				ID:        companionID,
				EventULID: "01HTARGET00000000000000001",
				Status:    "pending",
				Warnings:  makeWarningsJSON("near_duplicate_of_new_event"),
			}, nil
		}
		return nil, ErrNotFound
	}
	// The lockReviewQueueEntryForUpdateFunc must handle both the primary review (id=1)
	// and the companion review (id=99).
	repo.lockReviewQueueEntryForUpdateFunc = func(_ context.Context, id int) (*ReviewQueueEntry, error) {
		if id == 1 {
			return &ReviewQueueEntry{
				ID:             1,
				EventID:        "review-event-id",
				EventULID:      "01HREV00000000000000000001",
				Status:         "pending",
				EventStartTime: startTime,
				Warnings:       makeWarningsJSON("potential_duplicate"),
			}, nil
		}
		if id == companionID {
			return &ReviewQueueEntry{
				ID:        companionID,
				EventULID: "01HTARGET00000000000000001",
				Status:    "pending",
				Warnings:  makeWarningsJSON("near_duplicate_of_new_event"),
			}, nil
		}
		return nil, ErrNotFound
	}
	repo.mergeReviewFunc = func(_ context.Context, id int, _ string, _ string) (*ReviewQueueEntry, error) {
		if id == companionID {
			companionDismissed = true
		}
		return &ReviewQueueEntry{ID: id, Status: "merged"}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
	_, err := service.AddOccurrenceFromReview(ctx, 1, "01HTARGET00000000000000001", "admin")

	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if !companionDismissed {
		t.Error("companion review on target event must be dismissed via MergeReview")
	}
	if !repo.commitCalled {
		t.Error("commit must be called on success")
	}
}

// TestAddOccurrenceFromReview_NoCompanionReviewIsOK verifies that the forward
// path succeeds even when no companion review exists on the target event.
func TestAddOccurrenceFromReview_NoCompanionReviewIsOK(t *testing.T) {
	ctx := context.Background()
	startTime := time.Now()

	repo := makeOccurrenceRepo("target-uuid", "01HTARGET00000000000000001", "01HREV00000000000000000001", startTime)
	// getPendingReviewByEventUlidFunc defaults to nil → returns nil, nil (no companion)

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
	_, err := service.AddOccurrenceFromReview(ctx, 1, "01HTARGET00000000000000001", "admin")

	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if !repo.commitCalled {
		t.Error("commit must be called on success")
	}
}

// ── Fix: source event occurrence cleanup after absorption ────────────────────

// TestAddOccurrenceFromReview_DeletesSourceOccurrences verifies that the forward
// path calls DeleteOccurrencesByEventULID after soft-deleting the source event.
// Bug: soft-delete (UPDATE) does not trigger ON DELETE CASCADE, so source
// event occurrences were left orphaned in the database.
func TestAddOccurrenceFromReview_DeletesSourceOccurrences(t *testing.T) {
	ctx := context.Background()
	startTime := time.Now()

	var occurrencesDeleted bool
	var deletedForULID string

	repo := makeOccurrenceRepo("target-uuid", "01HTARGET00000000000000001", "01HREV00000000000000000001", startTime)
	repo.deleteOccurrencesByEventULIDFunc = func(_ context.Context, eventULID string) error {
		occurrencesDeleted = true
		deletedForULID = eventULID
		return nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
	_, err := service.AddOccurrenceFromReview(ctx, 1, "01HTARGET00000000000000001", "admin")

	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if !occurrencesDeleted {
		t.Error("DeleteOccurrencesByEventULID must be called for the source event")
	}
	if deletedForULID != "01HREV00000000000000000001" {
		t.Errorf("DeleteOccurrencesByEventULID called with wrong ULID: got %q, want %q",
			deletedForULID, "01HREV00000000000000000001")
	}
}

// TestAddOccurrenceFromReviewNearDup_DeletesSourceOccurrences verifies that the
// near-dup path calls DeleteOccurrencesByEventULID after soft-deleting the
// source event.
func TestAddOccurrenceFromReviewNearDup_DeletesSourceOccurrences(t *testing.T) {
	ctx := context.Background()
	startTime := time.Now()

	var occurrencesDeleted bool
	var deletedForULID string

	repo := makeNearDupOccurrenceRepo("target-id", "01HTARGET00000000000000001", "01HSRC00000000000000000001", startTime)
	repo.deleteOccurrencesByEventULIDFunc = func(_ context.Context, eventULID string) error {
		occurrencesDeleted = true
		deletedForULID = eventULID
		return nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, "https://toronto.togather.foundation")
	_, _, err := service.AddOccurrenceFromReviewNearDup(ctx, 1, "admin")

	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if !occurrencesDeleted {
		t.Error("DeleteOccurrencesByEventULID must be called for the source event")
	}
	if deletedForULID != "01HSRC00000000000000000001" {
		t.Errorf("DeleteOccurrencesByEventULID called with wrong ULID: got %q, want %q",
			deletedForULID, "01HSRC00000000000000000001")
	}
}

// TestAddOccurrenceFromReview_RollbackOnOccurrenceDeleteError verifies that
// a DeleteOccurrencesByEventULID failure rolls back the transaction.
func TestAddOccurrenceFromReview_RollbackOnOccurrenceDeleteError(t *testing.T) {
	ctx := context.Background()
	startTime := time.Now()

	repo := makeOccurrenceRepo("target-uuid", "01HTARGET00000000000000001", "01HREV00000000000000000001", startTime)
	repo.deleteOccurrencesByEventULIDFunc = func(_ context.Context, _ string) error {
		return errors.New("occurrence cleanup failed")
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
	_, err := service.AddOccurrenceFromReview(ctx, 1, "01HTARGET00000000000000001", "admin")

	if err == nil {
		t.Fatal("expected error from DeleteOccurrencesByEventULID failure, got nil")
	}
	if repo.commitCalled {
		t.Error("commit must not be called after occurrence cleanup failure")
	}
	if !repo.rollbackCalled {
		t.Error("rollback must be called via defer on error")
	}
}

// ── Regression: precise companion selection with multiple pending reviews ─────

// TestAddOccurrenceFromReview_OnlyTrueCompanionDismissed verifies that when the
// target event has multiple pending review rows, only the one that is the true
// companion (cross-linked to the source event via duplicate_of_event_id) is
// dismissed.  The old code used LIMIT 1 with no duplicate filter and could pick
// the wrong review.
func TestAddOccurrenceFromReview_OnlyTrueCompanionDismissed(t *testing.T) {
	ctx := context.Background()
	startTime := time.Now()

	targetULID := "01HTARGET00000000000000001"
	reviewEventULID := "01HREV00000000000000000001"
	unrelatedCompanionID := 200
	trueCompanionID := 201

	repo := makeOccurrenceRepo("target-uuid", targetULID, reviewEventULID, startTime)

	// New func: returns the true companion only when called with the correct
	// (targetULID, reviewEventULID) pair.
	repo.getPendingReviewByEventUlidAndDuplicateUlidFunc = func(_ context.Context, ulid string, dupULID string) (*ReviewQueueEntry, error) {
		if ulid == targetULID && dupULID == reviewEventULID {
			return &ReviewQueueEntry{
				ID:        trueCompanionID,
				EventULID: targetULID,
				Status:    "pending",
				Warnings:  makeWarningsJSON("near_duplicate_of_new_event"),
			}, nil
		}
		return nil, ErrNotFound
	}

	// Lock handles primary (id=1), true companion (trueCompanionID), and
	// the unrelated companion (unrelatedCompanionID).
	repo.lockReviewQueueEntryForUpdateFunc = func(_ context.Context, id int) (*ReviewQueueEntry, error) {
		switch id {
		case 1:
			return &ReviewQueueEntry{
				ID:             1,
				EventID:        "review-event-id",
				EventULID:      reviewEventULID,
				Status:         "pending",
				EventStartTime: startTime,
				Warnings:       makeWarningsJSON("potential_duplicate"),
			}, nil
		case trueCompanionID:
			return &ReviewQueueEntry{
				ID:        trueCompanionID,
				EventULID: targetULID,
				Status:    "pending",
				Warnings:  makeWarningsJSON("near_duplicate_of_new_event"),
			}, nil
		case unrelatedCompanionID:
			return &ReviewQueueEntry{
				ID:        unrelatedCompanionID,
				EventULID: targetULID,
				Status:    "pending",
				Warnings:  makeWarningsJSON("potential_duplicate"),
			}, nil
		}
		return nil, ErrNotFound
	}

	mergedIDs := []int{}
	repo.mergeReviewFunc = func(_ context.Context, id int, _ string, _ string) (*ReviewQueueEntry, error) {
		mergedIDs = append(mergedIDs, id)
		return &ReviewQueueEntry{ID: id, Status: "merged"}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
	_, err := service.AddOccurrenceFromReview(ctx, 1, targetULID, "admin")

	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	// True companion must be dismissed.
	foundTrue := false
	for _, id := range mergedIDs {
		if id == trueCompanionID {
			foundTrue = true
		}
		if id == unrelatedCompanionID {
			t.Errorf("unrelated companion (id=%d) must NOT be dismissed", unrelatedCompanionID)
		}
	}
	if !foundTrue {
		t.Errorf("true companion (id=%d) must be dismissed via MergeReview", trueCompanionID)
	}
	if !repo.commitCalled {
		t.Error("commit must be called on success")
	}
}

// TestAddOccurrenceFromReviewNearDup_OnlyTrueCompanionDismissed verifies that
// when the source event has multiple pending review rows, only the one that is
// the true companion (cross-linked to the target event via duplicate_of_event_id)
// is dismissed on the near-dup path.
func TestAddOccurrenceFromReviewNearDup_OnlyTrueCompanionDismissed(t *testing.T) {
	ctx := context.Background()
	startTime := time.Now()

	targetULID := "01HTARGET00000000000000001"
	sourceULID := "01HSRC00000000000000000001"
	unrelatedCompanionID := 300
	trueCompanionID := 301

	repo := makeNearDupOccurrenceRepo("target-id", targetULID, sourceULID, startTime)

	// New func: returns the true companion only when called with the correct
	// (sourceULID, targetULID) pair.
	repo.getPendingReviewByEventUlidAndDuplicateUlidFunc = func(_ context.Context, ulid string, dupULID string) (*ReviewQueueEntry, error) {
		if ulid == sourceULID && dupULID == targetULID {
			return &ReviewQueueEntry{
				ID:        trueCompanionID,
				EventULID: sourceULID,
				Status:    "pending",
				Warnings:  makeWarningsJSON("near_duplicate_of_new_event"),
			}, nil
		}
		return nil, ErrNotFound
	}

	// Lock handles primary (id=1), true companion (trueCompanionID), and
	// the unrelated companion (unrelatedCompanionID) on the source event.
	repo.lockReviewQueueEntryForUpdateFunc = func(_ context.Context, id int) (*ReviewQueueEntry, error) {
		switch id {
		case 1:
			return &ReviewQueueEntry{
				ID:                   1,
				Status:               "pending",
				EventULID:            targetULID,
				DuplicateOfEventULID: &sourceULID,
				Warnings:             []byte(`[{"code":"near_duplicate_of_new_event","message":"near_duplicate_of_new_event"}]`),
			}, nil
		case trueCompanionID:
			return &ReviewQueueEntry{
				ID:        trueCompanionID,
				EventULID: sourceULID,
				Status:    "pending",
				Warnings:  makeWarningsJSON("near_duplicate_of_new_event"),
			}, nil
		case unrelatedCompanionID:
			return &ReviewQueueEntry{
				ID:        unrelatedCompanionID,
				EventULID: sourceULID,
				Status:    "pending",
				Warnings:  makeWarningsJSON("potential_duplicate"),
			}, nil
		}
		return nil, ErrNotFound
	}

	mergedIDs := []int{}
	repo.mergeReviewFunc = func(_ context.Context, id int, _ string, _ string) (*ReviewQueueEntry, error) {
		mergedIDs = append(mergedIDs, id)
		return &ReviewQueueEntry{ID: id, Status: "merged"}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, "https://toronto.togather.foundation")
	_, _, err := service.AddOccurrenceFromReviewNearDup(ctx, 1, "admin")

	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	// True companion must be dismissed.
	foundTrue := false
	for _, id := range mergedIDs {
		if id == trueCompanionID {
			foundTrue = true
		}
		if id == unrelatedCompanionID {
			t.Errorf("unrelated companion (id=%d) must NOT be dismissed", unrelatedCompanionID)
		}
	}
	if !foundTrue {
		t.Errorf("true companion (id=%d) must be dismissed via MergeReview", trueCompanionID)
	}
	if !repo.commitCalled {
		t.Error("commit must be called on success")
	}
}

// ---------------------------------------------------------------------------
// Regression tests: MergeEventsWithReview companion cleanup uses PrimaryULID
// (srv-izykp) — guards against the bug where DuplicateULID was passed to
// MergeReview for the companion review after executeMerge had already
// soft-deleted the duplicate, causing MergeReview's live-event lookup to fail.
// ---------------------------------------------------------------------------

// makeMergeWithReviewRepo builds a mockTransactionalRepo pre-wired for the
// "happy path" of MergeEventsWithReview with a companion review present.
// Tests override individual func fields to inspect or inject failures.
func makeMergeWithReviewRepo(primaryULID, duplicateULID string, companionID int) *mockTransactionalRepo {
	primaryEvent := &Event{ID: "primary-uuid", ULID: primaryULID, Name: "Primary", LifecycleState: "published"}
	duplicateEvent := &Event{ID: "duplicate-uuid", ULID: duplicateULID, Name: "Duplicate", LifecycleState: "published"}

	return &mockTransactionalRepo{
		// Lock: review row (id=1) is pending; companion (companionID) is also pending.
		lockReviewQueueEntryForUpdateFunc: func(_ context.Context, id int) (*ReviewQueueEntry, error) {
			switch id {
			case 1:
				return &ReviewQueueEntry{ID: 1, EventULID: duplicateULID, Status: "pending",
					Warnings: makeWarningsJSON("potential_duplicate")}, nil
			case companionID:
				return &ReviewQueueEntry{ID: companionID, EventULID: primaryULID, Status: "pending",
					Warnings: makeWarningsJSON("near_duplicate_of_new_event")}, nil
			}
			return nil, ErrNotFound
		},
		// Companion lookup: primary event has a near-dup companion pointing at duplicate.
		getPendingReviewByEventUlidAndDuplicateUlidFunc: func(_ context.Context, eventULID, dupULID string) (*ReviewQueueEntry, error) {
			if eventULID == primaryULID && dupULID == duplicateULID {
				return &ReviewQueueEntry{ID: companionID, EventULID: primaryULID, Status: "pending"}, nil
			}
			return nil, ErrNotFound
		},
		getByULIDFunc: func(_ context.Context, ulid string) (*Event, error) {
			switch ulid {
			case primaryULID:
				return primaryEvent, nil
			case duplicateULID:
				return duplicateEvent, nil
			}
			return nil, ErrNotFound
		},
		mergeEventsFunc:     func(_ context.Context, _, _ string) error { return nil },
		createTombstoneFunc: func(_ context.Context, _ TombstoneCreateParams) error { return nil },
		mergeReviewFunc: func(_ context.Context, id int, _ string, _ string) (*ReviewQueueEntry, error) {
			return &ReviewQueueEntry{ID: id, Status: "merged"}, nil
		},
		// No remaining pending reviews on primary -> lifecycle recomputes to published.
		getPendingReviewByEventUlidFunc: func(_ context.Context, _ string) (*ReviewQueueEntry, error) {
			return nil, nil
		},
		updateEventFunc: func(_ context.Context, _ string, _ UpdateEventParams) (*Event, error) {
			return primaryEvent, nil
		},
	}
}

// TestMergeEventsWithReview_CompanionCleanupUsesPrimaryULID is the critical regression
// test for the bug where the companion review's MergeReview call received DuplicateULID
// instead of PrimaryULID.  After executeMerge the duplicate is soft-deleted, so the
// live-event lookup inside MergeReview (WHERE ulid = $1 AND deleted_at IS NULL) would
// fail, causing the transaction to roll back.
//
// This test verifies that:
//  1. The companion review IS dismissed (merged) as part of MergeEventsWithReview.
//  2. The ULID passed to MergeReview for the companion is params.PrimaryULID — the
//     surviving event — NOT params.DuplicateULID which has been soft-deleted.
func TestMergeEventsWithReview_CompanionCleanupUsesPrimaryULID(t *testing.T) {
	ctx := context.Background()
	primaryULID := "01HPRMARY00000000000000002"
	duplicateULID := "01HDPCT0000000000000000002"
	companionID := 42

	repo := makeMergeWithReviewRepo(primaryULID, duplicateULID, companionID)

	// Capture ULID arguments passed to MergeReview per review-entry id.
	type mergeCall struct {
		id   int
		ulid string
	}
	var mergeCalls []mergeCall
	repo.mergeReviewFunc = func(_ context.Context, id int, _ string, ulid string) (*ReviewQueueEntry, error) {
		mergeCalls = append(mergeCalls, mergeCall{id: id, ulid: ulid})
		return &ReviewQueueEntry{ID: id, Status: "merged"}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
	_, err := service.MergeEventsWithReview(ctx, MergeEventsParams{
		PrimaryULID:   primaryULID,
		DuplicateULID: duplicateULID,
	}, 1, "admin")

	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if !repo.commitCalled {
		t.Error("commit must be called on success")
	}

	// Exactly two MergeReview calls: one for the companion, one for the primary review.
	if len(mergeCalls) != 2 {
		t.Fatalf("expected 2 MergeReview calls (companion + primary review), got %d: %v", len(mergeCalls), mergeCalls)
	}

	// Find the companion call (id=companionID) and verify it was given the primaryULID,
	// NOT the duplicateULID.  Passing duplicateULID after executeMerge would cause the
	// real repository to fail with "primary event not found" because the duplicate is
	// soft-deleted and excluded by the WHERE deleted_at IS NULL condition.
	foundCompanion := false
	for _, call := range mergeCalls {
		if call.id == companionID {
			foundCompanion = true
			if call.ulid != primaryULID {
				t.Errorf("companion MergeReview received ULID %q, want primaryULID %q (must NOT be duplicateULID %q)",
					call.ulid, primaryULID, duplicateULID)
			}
		}
	}
	if !foundCompanion {
		t.Errorf("companion review (id=%d) was never dismissed via MergeReview", companionID)
	}
}

// TestMergeEventsWithReview_NoCompanionSucceeds verifies that MergeEventsWithReview
// succeeds cleanly when there is no companion review entry (e.g. the primary event
// has no pending near-dup review pairing with the duplicate).
func TestMergeEventsWithReview_NoCompanionSucceeds(t *testing.T) {
	ctx := context.Background()
	primaryULID := "01HPRMARY00000000000000003"
	duplicateULID := "01HDPCT0000000000000000003"

	repo := makeMergeWithReviewRepo(primaryULID, duplicateULID, 0 /* unused */)
	// Override companion lookup to return ErrNotFound (no companion).
	repo.getPendingReviewByEventUlidAndDuplicateUlidFunc = func(_ context.Context, _, _ string) (*ReviewQueueEntry, error) {
		return nil, ErrNotFound
	}

	var mergeCallCount int
	repo.mergeReviewFunc = func(_ context.Context, id int, _ string, _ string) (*ReviewQueueEntry, error) {
		mergeCallCount++
		return &ReviewQueueEntry{ID: id, Status: "merged"}, nil
	}

	service := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{MaxEventNameLength: 500}, "https://toronto.togather.foundation")
	_, err := service.MergeEventsWithReview(ctx, MergeEventsParams{
		PrimaryULID:   primaryULID,
		DuplicateULID: duplicateULID,
	}, 1, "admin")

	if err != nil {
		t.Fatalf("expected success without companion, got: %v", err)
	}
	if !repo.commitCalled {
		t.Error("commit must be called on success")
	}
	// Only the primary review row should be merged (no companion).
	if mergeCallCount != 1 {
		t.Errorf("expected exactly 1 MergeReview call (primary review), got %d", mergeCallCount)
	}
}
