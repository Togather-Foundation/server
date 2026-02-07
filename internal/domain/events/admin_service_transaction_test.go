package events

import (
	"context"
	"errors"
	"testing"
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

	service := NewAdminService(repo, false)

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

	service := NewAdminService(repo, false)

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

	// Verify rollback was not called
	if repo.rollbackCalled {
		t.Error("Rollback should not be called after successful transaction")
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

	service := NewAdminService(repo, false)

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

// mockTransactionalRepo implements Repository with transaction support
type mockTransactionalRepo struct {
	getByULIDFunc       func(ctx context.Context, ulid string) (*Event, error)
	mergeEventsFunc     func(ctx context.Context, duplicateULID, primaryULID string) error
	createTombstoneFunc func(ctx context.Context, params TombstoneCreateParams) error
	commitCalled        bool
	rollbackCalled      bool
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
	return nil, errors.New("not implemented")
}
func (m *mockTransactionalRepo) SoftDeleteEvent(ctx context.Context, ulid, reason string) error {
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
	return nil, errors.New("not implemented")
}
func (m *mockTransactionalRepo) ListReviewQueue(ctx context.Context, filters ReviewQueueFilters) (*ReviewQueueListResult, error) {
	return nil, errors.New("not implemented")
}
func (m *mockTransactionalRepo) ApproveReview(ctx context.Context, id int, reviewedBy string, notes *string) (*ReviewQueueEntry, error) {
	return nil, errors.New("not implemented")
}
func (m *mockTransactionalRepo) RejectReview(ctx context.Context, id int, reviewedBy string, reason string) (*ReviewQueueEntry, error) {
	return nil, errors.New("not implemented")
}
func (m *mockTransactionalRepo) CleanupExpiredReviews(ctx context.Context) error {
	return errors.New("not implemented")
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
