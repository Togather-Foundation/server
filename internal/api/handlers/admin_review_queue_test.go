package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/api/middleware"
	"github.com/Togather-Foundation/server/internal/audit"
	"github.com/Togather-Foundation/server/internal/auth"
	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const testUserKey contextKey = "user"

// MockRepository is a mock implementation of events.Repository
type MockRepository struct {
	mock.Mock

	// Per-test function overrides for occurrence methods (nil = use hardcoded stub behaviour).
	InsertOccurrenceFn           func(ctx context.Context, params events.OccurrenceCreateParams) (*events.Occurrence, error)
	GetOccurrenceByIDFn          func(ctx context.Context, eventID string, occurrenceID string) (*events.Occurrence, error)
	UpdateOccurrenceFn           func(ctx context.Context, eventID string, occurrenceID string, params events.OccurrenceUpdateParams) (*events.Occurrence, error)
	DeleteOccurrenceByIDFn       func(ctx context.Context, eventID string, occurrenceID string) error
	CountOccurrencesFn           func(ctx context.Context, eventID string) (int64, error)
	CheckOccurrenceOverlapExclFn func(ctx context.Context, eventID string, startTime time.Time, endTime *time.Time, excludeID string) (bool, error)
}

func (m *MockRepository) List(ctx context.Context, filters events.Filters, pagination events.Pagination) (events.ListResult, error) {
	args := m.Called(ctx, filters, pagination)
	return args.Get(0).(events.ListResult), args.Error(1)
}

func (m *MockRepository) GetByULID(ctx context.Context, ulid string) (*events.Event, error) {
	args := m.Called(ctx, ulid)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*events.Event), args.Error(1)
}

func (m *MockRepository) Create(ctx context.Context, params events.EventCreateParams) (*events.Event, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*events.Event), args.Error(1)
}

func (m *MockRepository) CreateOccurrence(ctx context.Context, params events.OccurrenceCreateParams) error {
	args := m.Called(ctx, params)
	return args.Error(0)
}

func (m *MockRepository) CreateSource(ctx context.Context, params events.EventSourceCreateParams) error {
	args := m.Called(ctx, params)
	return args.Error(0)
}

func (m *MockRepository) FindBySourceExternalID(ctx context.Context, sourceID string, sourceEventID string) (*events.Event, error) {
	args := m.Called(ctx, sourceID, sourceEventID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*events.Event), args.Error(1)
}

func (m *MockRepository) FindByDedupHash(ctx context.Context, dedupHash string) (*events.Event, error) {
	args := m.Called(ctx, dedupHash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*events.Event), args.Error(1)
}

func (m *MockRepository) GetOrCreateSource(ctx context.Context, params events.SourceLookupParams) (string, error) {
	args := m.Called(ctx, params)
	return args.String(0), args.Error(1)
}

func (m *MockRepository) GetIdempotencyKey(ctx context.Context, key string) (*events.IdempotencyKey, error) {
	args := m.Called(ctx, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*events.IdempotencyKey), args.Error(1)
}

func (m *MockRepository) InsertIdempotencyKey(ctx context.Context, params events.IdempotencyKeyCreateParams) (*events.IdempotencyKey, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*events.IdempotencyKey), args.Error(1)
}

func (m *MockRepository) UpdateIdempotencyKeyEvent(ctx context.Context, key string, eventID string, eventULID string) error {
	args := m.Called(ctx, key, eventID, eventULID)
	return args.Error(0)
}

func (m *MockRepository) UpsertPlace(ctx context.Context, params events.PlaceCreateParams) (*events.PlaceRecord, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*events.PlaceRecord), args.Error(1)
}

func (m *MockRepository) GetPlaceByULID(ctx context.Context, ulid string) (*events.PlaceRecord, error) {
	args := m.Called(ctx, ulid)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*events.PlaceRecord), args.Error(1)
}

func (m *MockRepository) UpsertOrganization(ctx context.Context, params events.OrganizationCreateParams) (*events.OrganizationRecord, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*events.OrganizationRecord), args.Error(1)
}

func (m *MockRepository) UpdateEvent(ctx context.Context, ulid string, params events.UpdateEventParams) (*events.Event, error) {
	args := m.Called(ctx, ulid, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*events.Event), args.Error(1)
}

func (m *MockRepository) DeleteOccurrencesByEventULID(ctx context.Context, eventULID string) error {
	args := m.Called(ctx, eventULID)
	return args.Error(0)
}
func (m *MockRepository) UpdateOccurrenceDates(ctx context.Context, eventULID string, startTime time.Time, endTime *time.Time) error {
	args := m.Called(ctx, eventULID, startTime, endTime)
	return args.Error(0)
}

func (m *MockRepository) SoftDeleteEvent(ctx context.Context, ulid string, reason string) error {
	args := m.Called(ctx, ulid, reason)
	return args.Error(0)
}

func (m *MockRepository) MergeEvents(ctx context.Context, duplicateULID string, primaryULID string) error {
	args := m.Called(ctx, duplicateULID, primaryULID)
	return args.Error(0)
}

func (m *MockRepository) CreateTombstone(ctx context.Context, params events.TombstoneCreateParams) error {
	args := m.Called(ctx, params)
	return args.Error(0)
}

func (m *MockRepository) GetTombstoneByEventID(ctx context.Context, eventID string) (*events.Tombstone, error) {
	args := m.Called(ctx, eventID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*events.Tombstone), args.Error(1)
}

func (m *MockRepository) GetTombstoneByEventULID(ctx context.Context, eventULID string) (*events.Tombstone, error) {
	args := m.Called(ctx, eventULID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*events.Tombstone), args.Error(1)
}

func (m *MockRepository) FindReviewByDedup(ctx context.Context, sourceID *string, externalID *string, dedupHash *string) (*events.ReviewQueueEntry, error) {
	args := m.Called(ctx, sourceID, externalID, dedupHash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*events.ReviewQueueEntry), args.Error(1)
}

func (m *MockRepository) CreateReviewQueueEntry(ctx context.Context, params events.ReviewQueueCreateParams) (*events.ReviewQueueEntry, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*events.ReviewQueueEntry), args.Error(1)
}

func (m *MockRepository) UpdateReviewQueueEntry(ctx context.Context, id int, params events.ReviewQueueUpdateParams) (*events.ReviewQueueEntry, error) {
	args := m.Called(ctx, id, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*events.ReviewQueueEntry), args.Error(1)
}

func (m *MockRepository) GetReviewQueueEntry(ctx context.Context, id int) (*events.ReviewQueueEntry, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*events.ReviewQueueEntry), args.Error(1)
}

func (m *MockRepository) LockReviewQueueEntryForUpdate(ctx context.Context, id int) (*events.ReviewQueueEntry, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*events.ReviewQueueEntry), args.Error(1)
}

func (m *MockRepository) ListReviewQueue(ctx context.Context, filters events.ReviewQueueFilters) (*events.ReviewQueueListResult, error) {
	args := m.Called(ctx, filters)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*events.ReviewQueueListResult), args.Error(1)
}

func (m *MockRepository) ApproveReview(ctx context.Context, id int, reviewedBy string, notes *string) (*events.ReviewQueueEntry, error) {
	args := m.Called(ctx, id, reviewedBy, notes)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*events.ReviewQueueEntry), args.Error(1)
}

func (m *MockRepository) RejectReview(ctx context.Context, id int, reviewedBy string, reason string) (*events.ReviewQueueEntry, error) {
	args := m.Called(ctx, id, reviewedBy, reason)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*events.ReviewQueueEntry), args.Error(1)
}
func (m *MockRepository) MergeReview(ctx context.Context, id int, reviewedBy string, primaryEventULID string) (*events.ReviewQueueEntry, error) {
	args := m.Called(ctx, id, reviewedBy, primaryEventULID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*events.ReviewQueueEntry), args.Error(1)
}

func (m *MockRepository) CleanupExpiredReviews(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockRepository) BeginTx(ctx context.Context) (events.Repository, events.TxCommitter, error) {
	args := m.Called(ctx)
	return args.Get(0).(events.Repository), args.Get(1).(events.TxCommitter), args.Error(2)
}

func (m *MockRepository) GetSourceTrustLevel(ctx context.Context, eventID string) (int, error) {
	args := m.Called(ctx, eventID)
	return args.Int(0), args.Error(1)
}

func (m *MockRepository) GetSourceTrustLevelBySourceID(ctx context.Context, sourceID string) (int, error) {
	args := m.Called(ctx, sourceID)
	return args.Int(0), args.Error(1)
}

func (m *MockRepository) FindNearDuplicates(ctx context.Context, venueID string, startTime time.Time, eventName string, threshold float64) ([]events.NearDuplicateCandidate, error) {
	args := m.Called(ctx, venueID, startTime, eventName, threshold)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]events.NearDuplicateCandidate), args.Error(1)
}
func (m *MockRepository) FindSimilarPlaces(ctx context.Context, name string, locality string, region string, threshold float64) ([]events.SimilarPlaceCandidate, error) {
	args := m.Called(ctx, name, locality, region, threshold)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]events.SimilarPlaceCandidate), args.Error(1)
}
func (m *MockRepository) FindSimilarOrganizations(ctx context.Context, name string, locality string, region string, threshold float64) ([]events.SimilarOrgCandidate, error) {
	args := m.Called(ctx, name, locality, region, threshold)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]events.SimilarOrgCandidate), args.Error(1)
}
func (m *MockRepository) MergePlaces(ctx context.Context, duplicateID string, primaryID string) (*events.MergeResult, error) {
	args := m.Called(ctx, duplicateID, primaryID)
	return args.Get(0).(*events.MergeResult), args.Error(1)
}
func (m *MockRepository) MergeOrganizations(ctx context.Context, duplicateID string, primaryID string) (*events.MergeResult, error) {
	args := m.Called(ctx, duplicateID, primaryID)
	return args.Get(0).(*events.MergeResult), args.Error(1)
}
func (m *MockRepository) InsertNotDuplicate(ctx context.Context, eventIDa string, eventIDb string, createdBy string) error {
	args := m.Called(ctx, eventIDa, eventIDb, createdBy)
	return args.Error(0)
}
func (m *MockRepository) IsNotDuplicate(ctx context.Context, eventIDa string, eventIDb string) (bool, error) {
	args := m.Called(ctx, eventIDa, eventIDb)
	return args.Bool(0), args.Error(1)
}

func (m *MockRepository) GetPendingReviewByEventUlid(ctx context.Context, eventULID string) (*events.ReviewQueueEntry, error) {
	args := m.Called(ctx, eventULID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*events.ReviewQueueEntry), args.Error(1)
}

func (m *MockRepository) GetPendingReviewByEventUlidAndDuplicateUlid(ctx context.Context, eventULID string, duplicateULID string) (*events.ReviewQueueEntry, error) {
	args := m.Called(ctx, eventULID, duplicateULID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*events.ReviewQueueEntry), args.Error(1)
}

func (m *MockRepository) UpdateReviewWarnings(ctx context.Context, id int, warnings []byte) error {
	args := m.Called(ctx, id, warnings)
	return args.Error(0)
}

func (m *MockRepository) DismissCompanionWarningMatch(ctx context.Context, companionULID string, eventULID string) error {
	args := m.Called(ctx, companionULID, eventULID)
	return args.Error(0)
}

func (m *MockRepository) DismissWarningMatchByReviewID(ctx context.Context, id int, eventULID string) error {
	args := m.Called(ctx, id, eventULID)
	return args.Error(0)
}

func (m *MockRepository) CheckOccurrenceOverlap(ctx context.Context, eventID string, startTime time.Time, endTime *time.Time) (bool, error) {
	args := m.Called(ctx, eventID, startTime, endTime)
	return args.Bool(0), args.Error(1)
}

func (m *MockRepository) LockEventForUpdate(ctx context.Context, eventID string) error {
	args := m.Called(ctx, eventID)
	return args.Error(0)
}

func (m *MockRepository) InsertOccurrence(ctx context.Context, params events.OccurrenceCreateParams) (*events.Occurrence, error) {
	if m.InsertOccurrenceFn != nil {
		return m.InsertOccurrenceFn(ctx, params)
	}
	return nil, errors.New("InsertOccurrence: not implemented")
}

func (m *MockRepository) GetOccurrenceByID(ctx context.Context, eventID string, occurrenceID string) (*events.Occurrence, error) {
	if m.GetOccurrenceByIDFn != nil {
		return m.GetOccurrenceByIDFn(ctx, eventID, occurrenceID)
	}
	return nil, events.ErrNotFound
}

func (m *MockRepository) UpdateOccurrence(ctx context.Context, eventID string, occurrenceID string, params events.OccurrenceUpdateParams) (*events.Occurrence, error) {
	if m.UpdateOccurrenceFn != nil {
		return m.UpdateOccurrenceFn(ctx, eventID, occurrenceID, params)
	}
	return nil, errors.New("UpdateOccurrence: not implemented")
}

func (m *MockRepository) DeleteOccurrenceByID(ctx context.Context, eventID string, occurrenceID string) error {
	if m.DeleteOccurrenceByIDFn != nil {
		return m.DeleteOccurrenceByIDFn(ctx, eventID, occurrenceID)
	}
	return errors.New("DeleteOccurrenceByID: not implemented")
}

func (m *MockRepository) CountOccurrences(ctx context.Context, eventID string) (int64, error) {
	if m.CountOccurrencesFn != nil {
		return m.CountOccurrencesFn(ctx, eventID)
	}
	return 0, errors.New("CountOccurrences: not implemented")
}

func (m *MockRepository) CheckOccurrenceOverlapExcluding(ctx context.Context, eventID string, startTime time.Time, endTime *time.Time, excludeOccurrenceID string) (bool, error) {
	if m.CheckOccurrenceOverlapExclFn != nil {
		return m.CheckOccurrenceOverlapExclFn(ctx, eventID, startTime, endTime, excludeOccurrenceID)
	}
	return false, errors.New("CheckOccurrenceOverlapExcluding: not implemented")
}

func (m *MockRepository) DismissPendingReviewsByEventULIDs(ctx context.Context, eventULIDs []string, reviewedBy string) ([]int, error) {
	args := m.Called(ctx, eventULIDs, reviewedBy)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]int), args.Error(1)
}

func (m *MockRepository) FindSeriesCompanion(ctx context.Context, params events.SeriesCompanionQuery) (*events.CrossWeekCompanion, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*events.CrossWeekCompanion), args.Error(1)
}

func (m *MockRepository) Rollback(ctx context.Context) error {
	return nil
}

// Helper to add admin user to request context
func withAdminUser(r *http.Request, userEmail string) *http.Request {
	claims := &auth.Claims{
		Role: "admin",
	}
	claims.Subject = userEmail
	ctx := middleware.ContextWithAdminClaims(r.Context(), claims)
	ctx = context.WithValue(ctx, testUserKey, userEmail)
	return r.WithContext(ctx)
}

// MockTxCommitter implements events.TxCommitter for handler tests
type MockTxCommitter struct {
	mock.Mock
}

func (m *MockTxCommitter) Commit(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockTxCommitter) Rollback(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// setupTxMock configures BeginTx to return the mock repo itself as the txRepo
// and a mock TxCommitter that expects Commit + deferred Rollback (both succeed).
func setupTxMock(m *MockRepository) {
	txCommitter := new(MockTxCommitter)
	txCommitter.On("Commit", mock.Anything).Return(nil)
	txCommitter.On("Rollback", mock.Anything).Return(nil)
	m.On("BeginTx", mock.Anything).Return(m, txCommitter, nil)
}

// Helper to create a test review queue entry
func testReviewQueueEntry(id int, eventULID string) *events.ReviewQueueEntry {
	now := time.Now()
	originalPayload := []byte(`{"name":"Test Event","startDate":"2024-01-01T10:00:00Z"}`)
	normalizedPayload := []byte(`{"name":"Test Event","startDate":"2024-01-01T10:00:00Z"}`)
	warnings := []byte(`[]`)

	return &events.ReviewQueueEntry{
		ID:                id,
		EventID:           "test-event-id",
		EventULID:         eventULID,
		OriginalPayload:   originalPayload,
		NormalizedPayload: normalizedPayload,
		Warnings:          warnings,
		Status:            "pending",
		EventStartTime:    now,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
}

// ---------------------------------------------------------------------------
// Sentinel error mapping: ApproveReview, RejectReview, FixReview, MergeReview
// ---------------------------------------------------------------------------

// TestApproveReview_SentinelErrors verifies that service-layer sentinel errors
// from ApproveEventWithReview map to the correct HTTP status codes.
func TestApproveReview_SentinelErrors(t *testing.T) {
	tests := []struct {
		name           string
		serviceErr     error
		expectedStatus int
	}{
		{
			name:           "ErrConflict → 409",
			serviceErr:     events.ErrConflict,
			expectedStatus: http.StatusConflict,
		},
		{
			name:           "ErrEventDeleted → 410",
			serviceErr:     events.ErrEventDeleted,
			expectedStatus: http.StatusGone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockRepository)
			entry := testReviewQueueEntry(1, "01HTEST0000000000000000001")
			mockRepo.On("GetReviewQueueEntry", mock.Anything, 1).Return(entry, nil)
			setupTxMock(mockRepo)
			mockRepo.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(
				&events.ReviewQueueEntry{ID: 1, EventULID: "01HTEST0000000000000000001", Status: "approved"},
				tt.serviceErr,
			)

			adminService := events.NewAdminService(mockRepo, true, "America/Toronto", config.ValidationConfig{}, "https://toronto.togather.foundation")
			handler := &AdminReviewQueueHandler{
				Repository:   mockRepo,
				AdminService: adminService,
				AuditLogger:  audit.NewLogger(),
				Env:          "test",
			}

			body, _ := json.Marshal(map[string]string{})
			req := httptest.NewRequest(http.MethodPost, "/admin/review-queue/1/approve", bytes.NewReader(body))
			req.SetPathValue("id", "1")
			req = withAdminUser(req, "admin")
			rec := httptest.NewRecorder()

			handler.ApproveReview(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code)
		})
	}
}

// TestRejectReview_SentinelErrors verifies that service-layer sentinel errors
// from RejectEventWithReview map to the correct HTTP status codes.
func TestRejectReview_SentinelErrors(t *testing.T) {
	tests := []struct {
		name           string
		serviceErr     error
		expectedStatus int
	}{
		{
			name:           "ErrConflict → 409",
			serviceErr:     events.ErrConflict,
			expectedStatus: http.StatusConflict,
		},
		{
			name:           "ErrEventDeleted → 410",
			serviceErr:     events.ErrEventDeleted,
			expectedStatus: http.StatusGone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockRepository)
			entry := testReviewQueueEntry(1, "01HTEST0000000000000000001")
			mockRepo.On("GetReviewQueueEntry", mock.Anything, 1).Return(entry, nil)
			setupTxMock(mockRepo)
			mockRepo.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(
				&events.ReviewQueueEntry{ID: 1, EventULID: "01HTEST0000000000000000001", Status: "rejected"},
				tt.serviceErr,
			)

			adminService := events.NewAdminService(mockRepo, true, "America/Toronto", config.ValidationConfig{}, "https://toronto.togather.foundation")
			handler := &AdminReviewQueueHandler{
				Repository:   mockRepo,
				AdminService: adminService,
				AuditLogger:  audit.NewLogger(),
				Env:          "test",
			}

			body, _ := json.Marshal(map[string]string{"reason": "test reason"})
			req := httptest.NewRequest(http.MethodPost, "/admin/review-queue/1/reject", bytes.NewReader(body))
			req.SetPathValue("id", "1")
			req = withAdminUser(req, "admin")
			rec := httptest.NewRecorder()

			handler.RejectReview(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code)
		})
	}
}

// TestFixReview_SentinelErrors verifies that service-layer sentinel errors
// from FixAndApproveEventWithReview map to the correct HTTP status codes.
func TestFixReview_SentinelErrors(t *testing.T) {
	tests := []struct {
		name           string
		serviceErr     error
		expectedStatus int
	}{
		{
			name:           "ErrConflict → 409",
			serviceErr:     events.ErrConflict,
			expectedStatus: http.StatusConflict,
		},
		{
			name:           "ErrEventDeleted → 410",
			serviceErr:     events.ErrEventDeleted,
			expectedStatus: http.StatusGone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockRepository)
			entry := testReviewQueueEntry(1, "01HTEST0000000000000000001")
			mockRepo.On("GetReviewQueueEntry", mock.Anything, 1).Return(entry, nil)
			setupTxMock(mockRepo)
			mockRepo.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(
				&events.ReviewQueueEntry{ID: 1, EventULID: "01HTEST0000000000000000001", Status: "approved"},
				tt.serviceErr,
			)

			adminService := events.NewAdminService(mockRepo, true, "America/Toronto", config.ValidationConfig{}, "https://toronto.togather.foundation")
			handler := &AdminReviewQueueHandler{
				Repository:   mockRepo,
				AdminService: adminService,
				AuditLogger:  audit.NewLogger(),
				Env:          "test",
			}

			body, _ := json.Marshal(map[string]any{
				"corrections": map[string]string{"startDate": "2024-01-01T10:00:00Z"},
			})
			req := httptest.NewRequest(http.MethodPost, "/admin/review-queue/1/fix", bytes.NewReader(body))
			req.SetPathValue("id", "1")
			req = withAdminUser(req, "admin")
			rec := httptest.NewRecorder()

			handler.FixReview(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code)
		})
	}
}

// ---------------------------------------------------------------------------
// GetReviewQueueEntry (detail endpoint): relatedEvents expansion
// ---------------------------------------------------------------------------

// TestReviewQueueDetail_RelatedEventsExpanded verifies that when a review entry
// has a potential_duplicate warning with a related ULID, the detail endpoint
// fetches the full event and populates all fields (name, url, similarity, occurrences).
func TestReviewQueueDetail_RelatedEventsExpanded(t *testing.T) {
	const primaryULID = "01HTEST0000000000000000001"
	const relatedULID = "01HTEST0000000000000000002"
	sim := 0.92

	warningsJSON := []byte(`[{"code":"potential_duplicate","message":"Potential duplicate","details":{"matches":[{"ulid":"` + relatedULID + `","similarity":0.92}]}}]`)
	originalPayload := []byte(`{"name":"Test Event","startDate":"2024-01-01T10:00:00Z"}`)
	normalizedPayload := []byte(`{"name":"Test Event","startDate":"2024-01-01T10:00:00Z"}`)
	now := time.Now()

	entry := &events.ReviewQueueEntry{
		ID:                1,
		EventID:           "primary-event-id",
		EventULID:         primaryULID,
		OriginalPayload:   originalPayload,
		NormalizedPayload: normalizedPayload,
		Warnings:          warningsJSON,
		Status:            "pending",
		EventStartTime:    now,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	primaryEvent := &events.Event{
		ULID:        primaryULID,
		Name:        "Test Event",
		Description: "Primary event description",
		PublicURL:   "https://example.com/primary",
	}

	venueName := "The Venue"
	venueULID := "01HVENUE000000000000000001"
	relatedEvent := &events.Event{
		ULID:             relatedULID,
		Name:             "Related Event",
		Description:      "Related event description",
		PublicURL:        "https://example.com/related",
		ImageURL:         "https://example.com/img.jpg",
		PrimaryVenueName: &venueName,
		PrimaryVenueULID: &venueULID,
		Occurrences: []events.Occurrence{
			{
				ID:        "occ-1",
				StartTime: now,
				Timezone:  "America/Toronto",
			},
		},
	}

	mockRepo := new(MockRepository)
	mockRepo.On("GetReviewQueueEntry", mock.Anything, 1).Return(entry, nil)
	// Primary event fetch
	mockRepo.On("GetByULID", mock.Anything, primaryULID).Return(primaryEvent, nil)
	// Related event fetch
	mockRepo.On("GetByULID", mock.Anything, relatedULID).Return(relatedEvent, nil)

	adminService := events.NewAdminService(mockRepo, true, "America/Toronto", config.ValidationConfig{}, "https://toronto.togather.foundation")
	handler := &AdminReviewQueueHandler{
		Repository:   mockRepo,
		AdminService: adminService,
		AuditLogger:  audit.NewLogger(),
		Env:          "test",
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/review-queue/1", nil)
	req.SetPathValue("id", "1")
	req = withAdminUser(req, "admin")
	rec := httptest.NewRecorder()

	handler.GetReviewQueueEntry(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	assert.NoError(t, json.NewDecoder(rec.Body).Decode(&body))

	relatedEvents, ok := body["relatedEvents"].([]any)
	assert.True(t, ok, "relatedEvents should be an array")
	assert.Len(t, relatedEvents, 1, "should have one related event")

	re := relatedEvents[0].(map[string]any)
	assert.Equal(t, relatedULID, re["ulid"])
	assert.Equal(t, "Related Event", re["name"])
	assert.Equal(t, "https://example.com/related", re["url"])
	assert.Equal(t, "https://example.com/img.jpg", re["imageUrl"])
	assert.Equal(t, venueName, re["venueName"])
	assert.Equal(t, venueULID, re["venueUlid"])
	assert.InDelta(t, sim, re["similarity"].(float64), 0.001)

	occs, ok := re["occurrences"].([]any)
	assert.True(t, ok, "occurrences should be an array")
	assert.Len(t, occs, 1)

	mockRepo.AssertExpectations(t)
}

// TestReviewQueueDetail_RelatedEventFetchFails verifies that when GetByULID fails
// for a related event, the handler returns a ULID-only fallback entry (graceful degradation).
func TestReviewQueueDetail_RelatedEventFetchFails(t *testing.T) {
	const primaryULID = "01HTEST0000000000000000001"
	const relatedULID = "01HTEST0000000000000000003"

	warningsJSON := []byte(`[{"code":"potential_duplicate","message":"Potential duplicate","details":{"matches":[{"ulid":"` + relatedULID + `","similarity":0.85}]}}]`)
	originalPayload := []byte(`{"name":"Test Event","startDate":"2024-01-01T10:00:00Z"}`)
	normalizedPayload := []byte(`{"name":"Test Event","startDate":"2024-01-01T10:00:00Z"}`)
	now := time.Now()

	entry := &events.ReviewQueueEntry{
		ID:                2,
		EventID:           "primary-event-id",
		EventULID:         primaryULID,
		OriginalPayload:   originalPayload,
		NormalizedPayload: normalizedPayload,
		Warnings:          warningsJSON,
		Status:            "pending",
		EventStartTime:    now,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	primaryEvent := &events.Event{
		ULID: primaryULID,
		Name: "Test Event",
	}

	mockRepo := new(MockRepository)
	mockRepo.On("GetReviewQueueEntry", mock.Anything, 2).Return(entry, nil)
	mockRepo.On("GetByULID", mock.Anything, primaryULID).Return(primaryEvent, nil)
	// Simulate failure fetching the related event
	mockRepo.On("GetByULID", mock.Anything, relatedULID).Return((*events.Event)(nil), errors.New("db timeout"))

	adminService := events.NewAdminService(mockRepo, true, "America/Toronto", config.ValidationConfig{}, "https://toronto.togather.foundation")
	handler := &AdminReviewQueueHandler{
		Repository:   mockRepo,
		AdminService: adminService,
		AuditLogger:  audit.NewLogger(),
		Env:          "test",
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/review-queue/2", nil)
	req.SetPathValue("id", "2")
	req = withAdminUser(req, "admin")
	rec := httptest.NewRecorder()

	handler.GetReviewQueueEntry(rec, req)

	// Handler should still return 200 with a partial entry (ULID + similarity, no name/url)
	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	assert.NoError(t, json.NewDecoder(rec.Body).Decode(&body))

	relatedEvents, ok := body["relatedEvents"].([]any)
	assert.True(t, ok)
	assert.Len(t, relatedEvents, 1)

	re := relatedEvents[0].(map[string]any)
	assert.Equal(t, relatedULID, re["ulid"])
	// Similarity should still be present from warnings parsing
	assert.InDelta(t, 0.85, re["similarity"].(float64), 0.001)
	// Name should be absent (omitempty — key won't exist)
	_, hasName := re["name"]
	assert.False(t, hasName, "name should be absent when fetch failed")

	mockRepo.AssertExpectations(t)
}

// TestReviewQueueDetail_DuplicateOfEventULID verifies that when a review entry
// has a DuplicateOfEventULID set, it is included as a related event (no similarity).
func TestReviewQueueDetail_DuplicateOfEventULID(t *testing.T) {
	const primaryULID = "01HTEST0000000000000000001"
	const companionULID = "01HTEST0000000000000000004"

	dupULID := companionULID
	now := time.Now()
	entry := &events.ReviewQueueEntry{
		ID:                   3,
		EventID:              "primary-event-id",
		EventULID:            primaryULID,
		OriginalPayload:      []byte(`{"name":"Test Event","startDate":"2024-01-01T10:00:00Z"}`),
		NormalizedPayload:    []byte(`{"name":"Test Event","startDate":"2024-01-01T10:00:00Z"}`),
		Warnings:             []byte(`[]`),
		Status:               "pending",
		EventStartTime:       now,
		CreatedAt:            now,
		UpdatedAt:            now,
		DuplicateOfEventULID: &dupULID,
	}

	primaryEvent := &events.Event{ULID: primaryULID, Name: "Test Event"}
	companionEvent := &events.Event{
		ULID:      companionULID,
		Name:      "Companion Event",
		PublicURL: "https://example.com/companion",
	}

	mockRepo := new(MockRepository)
	mockRepo.On("GetReviewQueueEntry", mock.Anything, 3).Return(entry, nil)
	mockRepo.On("GetByULID", mock.Anything, primaryULID).Return(primaryEvent, nil)
	mockRepo.On("GetByULID", mock.Anything, companionULID).Return(companionEvent, nil)

	adminService := events.NewAdminService(mockRepo, true, "America/Toronto", config.ValidationConfig{}, "https://toronto.togather.foundation")
	handler := &AdminReviewQueueHandler{
		Repository:   mockRepo,
		AdminService: adminService,
		AuditLogger:  audit.NewLogger(),
		Env:          "test",
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/review-queue/3", nil)
	req.SetPathValue("id", "3")
	req = withAdminUser(req, "admin")
	rec := httptest.NewRecorder()

	handler.GetReviewQueueEntry(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	assert.NoError(t, json.NewDecoder(rec.Body).Decode(&body))

	relatedEvents, ok := body["relatedEvents"].([]any)
	assert.True(t, ok)
	assert.Len(t, relatedEvents, 1)

	re := relatedEvents[0].(map[string]any)
	assert.Equal(t, companionULID, re["ulid"])
	assert.Equal(t, "Companion Event", re["name"])
	assert.Equal(t, "https://example.com/companion", re["url"])
	// Companion ULIDs have no similarity score
	_, hasSim := re["similarity"]
	assert.False(t, hasSim, "companion ULID should have no similarity")

	mockRepo.AssertExpectations(t)
}

// TestReviewQueueDetail_CrossWeekSeriesCompanionExpanded verifies that
// cross_week_series_companion warnings populate relatedEvents so the UI can render
// the side-by-side consolidation flow for weekly series survivors.
func TestReviewQueueDetail_CrossWeekSeriesCompanionExpanded(t *testing.T) {
	const primaryULID = "01HTEST0000000000000000101"
	const companionULID = "01HTEST0000000000000000102"

	warningsJSON := []byte(`[{"code":"cross_week_series_companion","message":"Weekly companion","details":{"companion_ulid":"` + companionULID + `","companion_name":"RS-11 Pottery Studio","companion_date":"2026-04-07"}}]`)
	now := time.Now()
	entry := &events.ReviewQueueEntry{
		ID:                4,
		EventID:           "primary-event-id",
		EventULID:         primaryULID,
		OriginalPayload:   []byte(`{"name":"RS-11 Pottery Studio"}`),
		NormalizedPayload: []byte(`{"name":"RS-11 Pottery Studio"}`),
		Warnings:          warningsJSON,
		Status:            "pending",
		EventStartTime:    now,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	primaryEvent := &events.Event{ULID: primaryULID, Name: "RS-11 Pottery Studio"}
	companionEvent := &events.Event{
		ULID:        companionULID,
		Name:        "RS-11 Pottery Studio",
		Description: "Week 2 survivor",
		PublicURL:   "https://example.com/week2",
		Occurrences: []events.Occurrence{{ID: "occ-2", StartTime: now.Add(7 * 24 * time.Hour), Timezone: "America/Toronto"}},
	}

	mockRepo := new(MockRepository)
	mockRepo.On("GetReviewQueueEntry", mock.Anything, 4).Return(entry, nil)
	mockRepo.On("GetByULID", mock.Anything, primaryULID).Return(primaryEvent, nil)
	mockRepo.On("GetByULID", mock.Anything, companionULID).Return(companionEvent, nil)

	adminService := events.NewAdminService(mockRepo, true, "America/Toronto", config.ValidationConfig{}, "https://toronto.togather.foundation")
	handler := &AdminReviewQueueHandler{
		Repository:   mockRepo,
		AdminService: adminService,
		AuditLogger:  audit.NewLogger(),
		Env:          "test",
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/review-queue/4", nil)
	req.SetPathValue("id", "4")
	req = withAdminUser(req, "admin")
	rec := httptest.NewRecorder()

	handler.GetReviewQueueEntry(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	assert.NoError(t, json.NewDecoder(rec.Body).Decode(&body))

	relatedEvents, ok := body["relatedEvents"].([]any)
	assert.True(t, ok)
	assert.Len(t, relatedEvents, 1)

	re := relatedEvents[0].(map[string]any)
	assert.Equal(t, companionULID, re["ulid"])
	assert.Equal(t, "RS-11 Pottery Studio", re["name"])
	assert.Equal(t, "https://example.com/week2", re["url"])
	_, hasSim := re["similarity"]
	assert.False(t, hasSim, "cross-week companion should not carry similarity")

	mockRepo.AssertExpectations(t)
}
