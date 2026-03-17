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
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const testUserKey contextKey = "user"

// MockRepository is a mock implementation of events.Repository
type MockRepository struct {
	mock.Mock
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

func (m *MockRepository) UpdateReviewWarnings(ctx context.Context, id int, warnings []byte) error {
	args := m.Called(ctx, id, warnings)
	return args.Error(0)
}

func (m *MockRepository) DismissCompanionWarningMatch(ctx context.Context, companionULID string, eventULID string) error {
	args := m.Called(ctx, companionULID, eventULID)
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
			entry := testReviewQueueEntry(1, "01HTEST1")
			mockRepo.On("GetReviewQueueEntry", mock.Anything, 1).Return(entry, nil)
			setupTxMock(mockRepo)
			mockRepo.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(
				&events.ReviewQueueEntry{ID: 1, EventULID: "01HTEST1", Status: "approved"},
				tt.serviceErr,
			)

			adminService := events.NewAdminService(mockRepo, true, "America/Toronto", config.ValidationConfig{})
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
			entry := testReviewQueueEntry(1, "01HTEST1")
			mockRepo.On("GetReviewQueueEntry", mock.Anything, 1).Return(entry, nil)
			setupTxMock(mockRepo)
			mockRepo.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(
				&events.ReviewQueueEntry{ID: 1, EventULID: "01HTEST1", Status: "rejected"},
				tt.serviceErr,
			)

			adminService := events.NewAdminService(mockRepo, true, "America/Toronto", config.ValidationConfig{})
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
			entry := testReviewQueueEntry(1, "01HTEST1")
			mockRepo.On("GetReviewQueueEntry", mock.Anything, 1).Return(entry, nil)
			setupTxMock(mockRepo)
			mockRepo.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(
				&events.ReviewQueueEntry{ID: 1, EventULID: "01HTEST1", Status: "approved"},
				tt.serviceErr,
			)

			adminService := events.NewAdminService(mockRepo, true, "America/Toronto", config.ValidationConfig{})
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

// TestMergeReview_SentinelErrors verifies that service-layer sentinel errors
// from MergeEventsWithReview map to the correct HTTP status codes.
func TestMergeReview_SentinelErrors(t *testing.T) {
	primaryULID := "01HPRIMARY0000000000000001"

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
			entry := testReviewQueueEntry(1, "01HTEST1")
			mockRepo.On("GetReviewQueueEntry", mock.Anything, 1).Return(entry, nil)
			setupTxMock(mockRepo)
			mockRepo.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(
				&events.ReviewQueueEntry{ID: 1, EventULID: "01HTEST1", Status: "merged"},
				tt.serviceErr,
			)

			adminService := events.NewAdminService(mockRepo, true, "America/Toronto", config.ValidationConfig{})
			handler := &AdminReviewQueueHandler{
				Repository:   mockRepo,
				AdminService: adminService,
				AuditLogger:  audit.NewLogger(),
				Env:          "test",
			}

			body, _ := json.Marshal(map[string]string{"primary_event_ulid": primaryULID})
			req := httptest.NewRequest(http.MethodPost, "/admin/review-queue/1/merge", bytes.NewReader(body))
			req.SetPathValue("id", "1")
			req = withAdminUser(req, "admin")
			rec := httptest.NewRecorder()

			handler.MergeReview(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code)
		})
	}
}

// TestMergeReview_EntryNotFound verifies that a missing review ID on the initial fetch
// returns 404 (not 500). The repo returns events.ErrNotFound for missing rows.
func TestMergeReview_EntryNotFound(t *testing.T) {
	primaryULID := "01HPRIMARY0000000000000001"
	mockRepo := new(MockRepository)
	mockRepo.On("GetReviewQueueEntry", mock.Anything, 999).Return(
		(*events.ReviewQueueEntry)(nil),
		events.ErrNotFound,
	)

	adminService := events.NewAdminService(mockRepo, true, "America/Toronto", config.ValidationConfig{})
	handler := &AdminReviewQueueHandler{
		Repository:   mockRepo,
		AdminService: adminService,
		AuditLogger:  audit.NewLogger(),
		Env:          "test",
	}

	body, _ := json.Marshal(map[string]string{"primary_event_ulid": primaryULID})
	req := httptest.NewRequest(http.MethodPost, "/admin/review-queue/999/merge", bytes.NewReader(body))
	req.SetPathValue("id", "999")
	req = withAdminUser(req, "admin")
	rec := httptest.NewRecorder()

	handler.MergeReview(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	mockRepo.AssertExpectations(t)
}

// TestListReviewQueue tests listing review queue entries
func TestListReviewQueue(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		mockSetup      func(*MockRepository)
		expectedStatus int
		expectedItems  int
		expectedCursor string
	}{
		{
			name:        "Success - Default filters (pending status, limit 50)",
			queryParams: "",
			mockSetup: func(m *MockRepository) {
				m.On("ListReviewQueue", mock.Anything, mock.MatchedBy(func(f events.ReviewQueueFilters) bool {
					return f.Status != nil && *f.Status == "pending" && f.Limit == 50
				})).Return(&events.ReviewQueueListResult{
					Entries: []events.ReviewQueueEntry{
						*testReviewQueueEntry(1, "01HTEST1"),
						*testReviewQueueEntry(2, "01HTEST2"),
					},
					NextCursor: nil,
					TotalCount: 2,
				}, nil)
			},
			expectedStatus: http.StatusOK,
			expectedItems:  2,
			expectedCursor: "",
		},
		{
			name:        "Success - Custom status filter",
			queryParams: "?status=approved",
			mockSetup: func(m *MockRepository) {
				m.On("ListReviewQueue", mock.Anything, mock.MatchedBy(func(f events.ReviewQueueFilters) bool {
					return f.Status != nil && *f.Status == "approved"
				})).Return(&events.ReviewQueueListResult{
					Entries:    []events.ReviewQueueEntry{},
					NextCursor: nil,
					TotalCount: 0,
				}, nil)
			},
			expectedStatus: http.StatusOK,
			expectedItems:  0,
			expectedCursor: "",
		},
		{
			name:        "Success - Custom limit",
			queryParams: "?limit=10",
			mockSetup: func(m *MockRepository) {
				m.On("ListReviewQueue", mock.Anything, mock.MatchedBy(func(f events.ReviewQueueFilters) bool {
					return f.Limit == 10
				})).Return(&events.ReviewQueueListResult{
					Entries:    []events.ReviewQueueEntry{},
					NextCursor: nil,
					TotalCount: 0,
				}, nil)
			},
			expectedStatus: http.StatusOK,
			expectedItems:  0,
			expectedCursor: "",
		},
		{
			name:        "Success - Invalid limit defaults to 50",
			queryParams: "?limit=invalid",
			mockSetup: func(m *MockRepository) {
				m.On("ListReviewQueue", mock.Anything, mock.MatchedBy(func(f events.ReviewQueueFilters) bool {
					return f.Limit == 50
				})).Return(&events.ReviewQueueListResult{
					Entries:    []events.ReviewQueueEntry{},
					NextCursor: nil,
					TotalCount: 0,
				}, nil)
			},
			expectedStatus: http.StatusOK,
			expectedItems:  0,
			expectedCursor: "",
		},
		{
			name:        "Success - Limit too high defaults to 50",
			queryParams: "?limit=200",
			mockSetup: func(m *MockRepository) {
				m.On("ListReviewQueue", mock.Anything, mock.MatchedBy(func(f events.ReviewQueueFilters) bool {
					return f.Limit == 50
				})).Return(&events.ReviewQueueListResult{
					Entries:    []events.ReviewQueueEntry{},
					NextCursor: nil,
					TotalCount: 0,
				}, nil)
			},
			expectedStatus: http.StatusOK,
			expectedItems:  0,
			expectedCursor: "",
		},
		{
			name:        "Success - With cursor",
			queryParams: "?cursor=10",
			mockSetup: func(m *MockRepository) {
				m.On("ListReviewQueue", mock.Anything, mock.MatchedBy(func(f events.ReviewQueueFilters) bool {
					return f.NextCursor != nil && *f.NextCursor == 10
				})).Return(&events.ReviewQueueListResult{
					Entries:    []events.ReviewQueueEntry{},
					NextCursor: nil,
					TotalCount: 0,
				}, nil)
			},
			expectedStatus: http.StatusOK,
			expectedItems:  0,
			expectedCursor: "",
		},
		{
			name:        "Success - With next cursor",
			queryParams: "",
			mockSetup: func(m *MockRepository) {
				nextCursor := 3
				m.On("ListReviewQueue", mock.Anything, mock.Anything).Return(&events.ReviewQueueListResult{
					Entries: []events.ReviewQueueEntry{
						*testReviewQueueEntry(1, "01HTEST1"),
					},
					NextCursor: &nextCursor,
					TotalCount: 1,
				}, nil)
			},
			expectedStatus: http.StatusOK,
			expectedItems:  1,
			expectedCursor: "3",
		},
		{
			name:        "Error - Repository failure",
			queryParams: "",
			mockSetup: func(m *MockRepository) {
				m.On("ListReviewQueue", mock.Anything, mock.Anything).Return(
					(*events.ReviewQueueListResult)(nil),
					errors.New("database error"),
				)
			},
			expectedStatus: http.StatusInternalServerError,
			expectedItems:  0,
			expectedCursor: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockRepository)
			tt.mockSetup(mockRepo)

			handler := &AdminReviewQueueHandler{
				Repository:  mockRepo,
				AuditLogger: audit.NewLogger(),
				Env:         "test",
			}

			req := httptest.NewRequest(http.MethodGet, "/admin/review-queue"+tt.queryParams, nil)
			rec := httptest.NewRecorder()

			handler.ListReviewQueue(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code)

			if tt.expectedStatus == http.StatusOK {
				var resp listResponse
				err := json.NewDecoder(rec.Body).Decode(&resp)
				assert.NoError(t, err)
				assert.Len(t, resp.Items, tt.expectedItems)
				assert.Equal(t, tt.expectedCursor, resp.NextCursor)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

// TestGetReviewQueueEntry tests fetching a single review queue entry
func TestGetReviewQueueEntry(t *testing.T) {
	tests := []struct {
		name           string
		reviewID       string
		mockSetup      func(*MockRepository)
		expectedStatus int
	}{
		{
			name:     "Success - Valid ID",
			reviewID: "1",
			mockSetup: func(m *MockRepository) {
				m.On("GetReviewQueueEntry", mock.Anything, 1).Return(
					testReviewQueueEntry(1, "01HTEST1"),
					nil,
				)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Error - Missing ID",
			reviewID:       "",
			mockSetup:      func(m *MockRepository) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Error - Invalid ID (not a number)",
			reviewID:       "invalid",
			mockSetup:      func(m *MockRepository) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Error - Invalid ID (zero)",
			reviewID:       "0",
			mockSetup:      func(m *MockRepository) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Error - Invalid ID (negative)",
			reviewID:       "-5",
			mockSetup:      func(m *MockRepository) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:     "Error - Not found",
			reviewID: "999",
			mockSetup: func(m *MockRepository) {
				m.On("GetReviewQueueEntry", mock.Anything, 999).Return(
					(*events.ReviewQueueEntry)(nil),
					events.ErrNotFound,
				)
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:     "Error - Repository failure",
			reviewID: "1",
			mockSetup: func(m *MockRepository) {
				m.On("GetReviewQueueEntry", mock.Anything, 1).Return(
					(*events.ReviewQueueEntry)(nil),
					errors.New("database error"),
				)
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockRepository)
			tt.mockSetup(mockRepo)

			handler := &AdminReviewQueueHandler{
				Repository:  mockRepo,
				AuditLogger: audit.NewLogger(),
				Env:         "test",
			}

			req := httptest.NewRequest(http.MethodGet, "/admin/review-queue/"+tt.reviewID, nil)
			req.SetPathValue("id", tt.reviewID)
			rec := httptest.NewRecorder()

			handler.GetReviewQueueEntry(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code)

			if tt.expectedStatus == http.StatusOK {
				var resp reviewQueueDetail
				err := json.NewDecoder(rec.Body).Decode(&resp)
				assert.NoError(t, err)
				assert.Equal(t, 1, resp.ID)
				assert.Equal(t, "01HTEST1", resp.EventID)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

// TestApproveReview tests approving a review queue entry
func TestApproveReview(t *testing.T) {
	tests := []struct {
		name           string
		reviewID       string
		requestBody    map[string]string
		mockSetup      func(*MockRepository)
		expectedStatus int
	}{
		{
			name:        "Success - Approve with notes",
			reviewID:    "1",
			requestBody: map[string]string{"notes": "Looks good"},
			mockSetup: func(m *MockRepository) {
				entry := testReviewQueueEntry(1, "01HTEST1")
				m.On("GetReviewQueueEntry", mock.Anything, 1).Return(entry, nil)
				setupTxMock(m)
				m.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(entry, nil)
				m.On("GetByULID", mock.Anything, "01HTEST1").Return(&events.Event{LifecycleState: "draft"}, nil)
				m.On("UpdateEvent", mock.Anything, "01HTEST1", mock.Anything).Return(&events.Event{LifecycleState: "published"}, nil)
				notes := "Looks good"
				m.On("ApproveReview", mock.Anything, 1, "admin", &notes).Return(entry, nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:        "Success - Approve without notes",
			reviewID:    "1",
			requestBody: map[string]string{},
			mockSetup: func(m *MockRepository) {
				entry := testReviewQueueEntry(1, "01HTEST1")
				m.On("GetReviewQueueEntry", mock.Anything, 1).Return(entry, nil)
				setupTxMock(m)
				m.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(entry, nil)
				m.On("GetByULID", mock.Anything, "01HTEST1").Return(&events.Event{LifecycleState: "draft"}, nil)
				m.On("UpdateEvent", mock.Anything, "01HTEST1", mock.Anything).Return(&events.Event{LifecycleState: "published"}, nil)
				m.On("ApproveReview", mock.Anything, 1, "admin", (*string)(nil)).Return(entry, nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Error - Missing ID",
			reviewID:       "",
			requestBody:    map[string]string{},
			mockSetup:      func(m *MockRepository) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Error - Invalid ID",
			reviewID:       "invalid",
			requestBody:    map[string]string{},
			mockSetup:      func(m *MockRepository) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:        "Error - Review entry not found on fetch",
			reviewID:    "999",
			requestBody: map[string]string{},
			mockSetup: func(m *MockRepository) {
				m.On("GetReviewQueueEntry", mock.Anything, 999).Return(
					(*events.ReviewQueueEntry)(nil),
					events.ErrNotFound,
				)
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:        "Error - Publish event fails (event not found)",
			reviewID:    "1",
			requestBody: map[string]string{},
			mockSetup: func(m *MockRepository) {
				entry := testReviewQueueEntry(1, "01HTEST1")
				m.On("GetReviewQueueEntry", mock.Anything, 1).Return(entry, nil)
				setupTxMock(m)
				m.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(entry, nil)
				m.On("GetByULID", mock.Anything, "01HTEST1").Return(
					(*events.Event)(nil),
					events.ErrNotFound,
				)
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:        "Error - Publish event fails (service error)",
			reviewID:    "1",
			requestBody: map[string]string{},
			mockSetup: func(m *MockRepository) {
				entry := testReviewQueueEntry(1, "01HTEST1")
				m.On("GetReviewQueueEntry", mock.Anything, 1).Return(entry, nil)
				setupTxMock(m)
				m.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(entry, nil)
				m.On("GetByULID", mock.Anything, "01HTEST1").Return(
					(*events.Event)(nil),
					errors.New("service error"),
				)
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:        "Error - Approve review fails (not found)",
			reviewID:    "1",
			requestBody: map[string]string{},
			mockSetup: func(m *MockRepository) {
				entry := testReviewQueueEntry(1, "01HTEST1")
				m.On("GetReviewQueueEntry", mock.Anything, 1).Return(entry, nil)
				setupTxMock(m)
				m.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(entry, nil)
				m.On("GetByULID", mock.Anything, "01HTEST1").Return(&events.Event{LifecycleState: "draft"}, nil)
				m.On("UpdateEvent", mock.Anything, "01HTEST1", mock.Anything).Return(&events.Event{LifecycleState: "published"}, nil)
				m.On("ApproveReview", mock.Anything, 1, "admin", (*string)(nil)).Return(
					(*events.ReviewQueueEntry)(nil),
					pgx.ErrNoRows,
				)
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:        "Error - Approve review fails (repository error)",
			reviewID:    "1",
			requestBody: map[string]string{},
			mockSetup: func(m *MockRepository) {
				entry := testReviewQueueEntry(1, "01HTEST1")
				m.On("GetReviewQueueEntry", mock.Anything, 1).Return(entry, nil)
				setupTxMock(m)
				m.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(entry, nil)
				m.On("GetByULID", mock.Anything, "01HTEST1").Return(&events.Event{LifecycleState: "draft"}, nil)
				m.On("UpdateEvent", mock.Anything, "01HTEST1", mock.Anything).Return(&events.Event{LifecycleState: "published"}, nil)
				m.On("ApproveReview", mock.Anything, 1, "admin", (*string)(nil)).Return(
					(*events.ReviewQueueEntry)(nil),
					errors.New("database error"),
				)
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockRepository)
			tt.mockSetup(mockRepo)

			adminService := events.NewAdminService(mockRepo, true, "America/Toronto", config.ValidationConfig{})
			handler := &AdminReviewQueueHandler{
				Repository:   mockRepo,
				AdminService: adminService,
				AuditLogger:  audit.NewLogger(),
				Env:          "test",
			}

			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/admin/review-queue/"+tt.reviewID+"/approve", bytes.NewReader(body))
			req.SetPathValue("id", tt.reviewID)
			req = withAdminUser(req, "admin")
			rec := httptest.NewRecorder()

			handler.ApproveReview(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code)

			mockRepo.AssertExpectations(t)
		})
	}
}

// TestRejectReview tests rejecting a review queue entry
func TestRejectReview(t *testing.T) {
	tests := []struct {
		name           string
		reviewID       string
		requestBody    map[string]string
		mockSetup      func(*MockRepository)
		expectedStatus int
	}{
		{
			name:        "Success - Reject with reason",
			reviewID:    "1",
			requestBody: map[string]string{"reason": "Invalid data"},
			mockSetup: func(m *MockRepository) {
				entry := testReviewQueueEntry(1, "01HTEST1")
				m.On("GetReviewQueueEntry", mock.Anything, 1).Return(entry, nil)
				setupTxMock(m)
				m.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(entry, nil)
				m.On("GetByULID", mock.Anything, "01HTEST1").Return(&events.Event{ULID: "01HTEST1", ID: "test-id", Name: "Test Event"}, nil)
				m.On("SoftDeleteEvent", mock.Anything, "01HTEST1", "Invalid data").Return(nil)
				m.On("CreateTombstone", mock.Anything, mock.Anything).Return(nil)
				m.On("RejectReview", mock.Anything, 1, "admin", "Invalid data").Return(entry, nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Error - Missing ID",
			reviewID:       "",
			requestBody:    map[string]string{"reason": "Invalid"},
			mockSetup:      func(m *MockRepository) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Error - Invalid ID",
			reviewID:       "invalid",
			requestBody:    map[string]string{"reason": "Invalid"},
			mockSetup:      func(m *MockRepository) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Error - Missing reason",
			reviewID:       "1",
			requestBody:    map[string]string{},
			mockSetup:      func(m *MockRepository) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Error - Empty reason",
			reviewID:       "1",
			requestBody:    map[string]string{"reason": ""},
			mockSetup:      func(m *MockRepository) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:        "Error - Review entry not found on fetch",
			reviewID:    "999",
			requestBody: map[string]string{"reason": "Invalid"},
			mockSetup: func(m *MockRepository) {
				m.On("GetReviewQueueEntry", mock.Anything, 999).Return(
					(*events.ReviewQueueEntry)(nil),
					events.ErrNotFound,
				)
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:        "Error - Delete event fails (event not found)",
			reviewID:    "1",
			requestBody: map[string]string{"reason": "Invalid"},
			mockSetup: func(m *MockRepository) {
				entry := testReviewQueueEntry(1, "01HTEST1")
				m.On("GetReviewQueueEntry", mock.Anything, 1).Return(entry, nil)
				setupTxMock(m)
				m.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(entry, nil)
				m.On("GetByULID", mock.Anything, "01HTEST1").Return(
					(*events.Event)(nil),
					events.ErrNotFound,
				)
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:        "Error - Delete event fails (service error)",
			reviewID:    "1",
			requestBody: map[string]string{"reason": "Invalid"},
			mockSetup: func(m *MockRepository) {
				entry := testReviewQueueEntry(1, "01HTEST1")
				m.On("GetReviewQueueEntry", mock.Anything, 1).Return(entry, nil)
				setupTxMock(m)
				m.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(entry, nil)
				m.On("GetByULID", mock.Anything, "01HTEST1").Return(
					(*events.Event)(nil),
					errors.New("service error"),
				)
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:        "Error - Reject review fails (not found)",
			reviewID:    "1",
			requestBody: map[string]string{"reason": "Invalid"},
			mockSetup: func(m *MockRepository) {
				entry := testReviewQueueEntry(1, "01HTEST1")
				m.On("GetReviewQueueEntry", mock.Anything, 1).Return(entry, nil)
				setupTxMock(m)
				m.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(entry, nil)
				m.On("GetByULID", mock.Anything, "01HTEST1").Return(&events.Event{ULID: "01HTEST1", ID: "test-id", Name: "Test Event"}, nil)
				m.On("SoftDeleteEvent", mock.Anything, "01HTEST1", "Invalid").Return(nil)
				m.On("CreateTombstone", mock.Anything, mock.Anything).Return(nil)
				m.On("RejectReview", mock.Anything, 1, "admin", "Invalid").Return(
					(*events.ReviewQueueEntry)(nil),
					pgx.ErrNoRows,
				)
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:        "Error - Reject review fails (repository error)",
			reviewID:    "1",
			requestBody: map[string]string{"reason": "Invalid"},
			mockSetup: func(m *MockRepository) {
				entry := testReviewQueueEntry(1, "01HTEST1")
				m.On("GetReviewQueueEntry", mock.Anything, 1).Return(entry, nil)
				setupTxMock(m)
				m.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(entry, nil)
				m.On("GetByULID", mock.Anything, "01HTEST1").Return(&events.Event{ULID: "01HTEST1", ID: "test-id", Name: "Test Event"}, nil)
				m.On("SoftDeleteEvent", mock.Anything, "01HTEST1", "Invalid").Return(nil)
				m.On("CreateTombstone", mock.Anything, mock.Anything).Return(nil)
				m.On("RejectReview", mock.Anything, 1, "admin", "Invalid").Return(
					(*events.ReviewQueueEntry)(nil),
					errors.New("database error"),
				)
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockRepository)
			tt.mockSetup(mockRepo)

			adminService := events.NewAdminService(mockRepo, true, "America/Toronto", config.ValidationConfig{})
			handler := &AdminReviewQueueHandler{
				Repository:   mockRepo,
				AdminService: adminService,
				AuditLogger:  audit.NewLogger(),
				Env:          "test",
			}

			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/admin/review-queue/"+tt.reviewID+"/reject", bytes.NewReader(body))
			req.SetPathValue("id", tt.reviewID)
			req = withAdminUser(req, "admin")
			rec := httptest.NewRecorder()

			handler.RejectReview(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code)

			mockRepo.AssertExpectations(t)
		})
	}
}

// TestFixReview tests the fix review handler
func TestFixReview(t *testing.T) {
	// eventWithOccurrence returns a test event with a single occurrence for FixEventOccurrenceDates
	eventWithOccurrence := func(state string) *events.Event {
		return &events.Event{
			ULID:           "01HTEST1",
			LifecycleState: state,
			Occurrences: []events.Occurrence{
				{
					ID:        "occ-1",
					StartTime: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
					EndTime:   func() *time.Time { t := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC); return &t }(),
				},
			},
		}
	}

	tests := []struct {
		name           string
		reviewID       string
		requestBody    map[string]any
		mockSetup      func(*MockRepository)
		expectedStatus int
	}{
		{
			name:     "Success - Fix with startDate correction",
			reviewID: "1",
			requestBody: map[string]any{
				"corrections": map[string]string{
					"startDate": "2024-01-01T10:00:00Z",
				},
				"notes": "Fixed start date",
			},
			mockSetup: func(m *MockRepository) {
				entry := testReviewQueueEntry(1, "01HTEST1")
				m.On("GetReviewQueueEntry", mock.Anything, 1).Return(entry, nil)
				setupTxMock(m)
				m.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(entry, nil)
				m.On("GetByULID", mock.Anything, "01HTEST1").Return(eventWithOccurrence("draft"), nil)
				m.On("UpdateOccurrenceDates", mock.Anything, "01HTEST1", mock.AnythingOfType("time.Time"), mock.Anything).Return(nil)
				m.On("UpdateEvent", mock.Anything, "01HTEST1", mock.Anything).Return(&events.Event{LifecycleState: "published"}, nil)
				m.On("ApproveReview", mock.Anything, 1, "admin", mock.AnythingOfType("*string")).Return(entry, nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:     "Success - Fix with endDate correction",
			reviewID: "1",
			requestBody: map[string]any{
				"corrections": map[string]string{
					"endDate": "2024-01-02T10:00:00Z",
				},
				"notes": "Fixed end date",
			},
			mockSetup: func(m *MockRepository) {
				entry := testReviewQueueEntry(1, "01HTEST1")
				m.On("GetReviewQueueEntry", mock.Anything, 1).Return(entry, nil)
				setupTxMock(m)
				m.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(entry, nil)
				m.On("GetByULID", mock.Anything, "01HTEST1").Return(eventWithOccurrence("draft"), nil)
				m.On("UpdateOccurrenceDates", mock.Anything, "01HTEST1", mock.AnythingOfType("time.Time"), mock.Anything).Return(nil)
				m.On("UpdateEvent", mock.Anything, "01HTEST1", mock.Anything).Return(&events.Event{LifecycleState: "published"}, nil)
				m.On("ApproveReview", mock.Anything, 1, "admin", mock.AnythingOfType("*string")).Return(entry, nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:     "Success - Fix with both dates and notes",
			reviewID: "1",
			requestBody: map[string]any{
				"corrections": map[string]string{
					"startDate": "2024-01-01T10:00:00Z",
					"endDate":   "2024-01-02T10:00:00Z",
				},
				"notes": "Fixed both dates",
			},
			mockSetup: func(m *MockRepository) {
				entry := testReviewQueueEntry(1, "01HTEST1")
				m.On("GetReviewQueueEntry", mock.Anything, 1).Return(entry, nil)
				setupTxMock(m)
				m.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(entry, nil)
				m.On("GetByULID", mock.Anything, "01HTEST1").Return(eventWithOccurrence("draft"), nil)
				m.On("UpdateOccurrenceDates", mock.Anything, "01HTEST1", mock.AnythingOfType("time.Time"), mock.Anything).Return(nil)
				m.On("UpdateEvent", mock.Anything, "01HTEST1", mock.Anything).Return(&events.Event{LifecycleState: "published"}, nil)
				m.On("ApproveReview", mock.Anything, 1, "admin", mock.AnythingOfType("*string")).Return(entry, nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Error - Missing ID",
			reviewID:       "",
			requestBody:    map[string]any{"corrections": map[string]string{"startDate": "2024-01-01T10:00:00Z"}},
			mockSetup:      func(m *MockRepository) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Error - Invalid ID",
			reviewID:       "invalid",
			requestBody:    map[string]any{"corrections": map[string]string{"startDate": "2024-01-01T10:00:00Z"}},
			mockSetup:      func(m *MockRepository) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Error - No corrections provided",
			reviewID:       "1",
			requestBody:    map[string]any{"corrections": map[string]string{}},
			mockSetup:      func(m *MockRepository) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:        "Error - Review entry not found",
			reviewID:    "999",
			requestBody: map[string]any{"corrections": map[string]string{"startDate": "2024-01-01T10:00:00Z"}},
			mockSetup: func(m *MockRepository) {
				m.On("GetReviewQueueEntry", mock.Anything, 999).Return(
					(*events.ReviewQueueEntry)(nil),
					events.ErrNotFound,
				)
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:        "Error - Fix occurrence dates fails (event not found)",
			reviewID:    "1",
			requestBody: map[string]any{"corrections": map[string]string{"startDate": "2024-01-01T10:00:00Z"}},
			mockSetup: func(m *MockRepository) {
				entry := testReviewQueueEntry(1, "01HTEST1")
				m.On("GetReviewQueueEntry", mock.Anything, 1).Return(entry, nil)
				setupTxMock(m)
				m.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(entry, nil)
				m.On("GetByULID", mock.Anything, "01HTEST1").Return(
					(*events.Event)(nil),
					events.ErrNotFound,
				)
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:        "Error - Publish event fails",
			reviewID:    "1",
			requestBody: map[string]any{"corrections": map[string]string{"startDate": "2024-01-01T10:00:00Z"}},
			mockSetup: func(m *MockRepository) {
				entry := testReviewQueueEntry(1, "01HTEST1")
				m.On("GetReviewQueueEntry", mock.Anything, 1).Return(entry, nil)
				setupTxMock(m)
				m.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(entry, nil)
				// GetByULID succeeds, UpdateOccurrenceDates succeeds, but UpdateEvent fails
				m.On("GetByULID", mock.Anything, "01HTEST1").Return(eventWithOccurrence("draft"), nil)
				m.On("UpdateOccurrenceDates", mock.Anything, "01HTEST1", mock.AnythingOfType("time.Time"), mock.Anything).Return(nil)
				m.On("UpdateEvent", mock.Anything, "01HTEST1", mock.Anything).Return(
					(*events.Event)(nil),
					errors.New("publish error"),
				)
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:        "Error - Approve review fails",
			reviewID:    "1",
			requestBody: map[string]any{"corrections": map[string]string{"startDate": "2024-01-01T10:00:00Z"}},
			mockSetup: func(m *MockRepository) {
				entry := testReviewQueueEntry(1, "01HTEST1")
				m.On("GetReviewQueueEntry", mock.Anything, 1).Return(entry, nil)
				setupTxMock(m)
				m.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(entry, nil)
				m.On("GetByULID", mock.Anything, "01HTEST1").Return(eventWithOccurrence("draft"), nil)
				m.On("UpdateOccurrenceDates", mock.Anything, "01HTEST1", mock.AnythingOfType("time.Time"), mock.Anything).Return(nil)
				m.On("UpdateEvent", mock.Anything, "01HTEST1", mock.Anything).Return(&events.Event{LifecycleState: "published"}, nil)
				m.On("ApproveReview", mock.Anything, 1, "admin", mock.AnythingOfType("*string")).Return(
					(*events.ReviewQueueEntry)(nil),
					errors.New("approve error"),
				)
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockRepository)
			tt.mockSetup(mockRepo)

			adminService := events.NewAdminService(mockRepo, true, "America/Toronto", config.ValidationConfig{})
			handler := &AdminReviewQueueHandler{
				Repository:   mockRepo,
				AdminService: adminService,
				AuditLogger:  audit.NewLogger(),
				Env:          "test",
			}

			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/admin/review-queue/"+tt.reviewID+"/fix", bytes.NewReader(body))
			req.SetPathValue("id", tt.reviewID)
			req = withAdminUser(req, "admin")
			rec := httptest.NewRecorder()

			handler.FixReview(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code)

			mockRepo.AssertExpectations(t)
		})
	}
}

// TestApproveReview_InvalidJSON tests handling of malformed JSON
func TestApproveReview_InvalidJSON(t *testing.T) {
	mockRepo := new(MockRepository)
	adminService := events.NewAdminService(mockRepo, true, "America/Toronto", config.ValidationConfig{})

	handler := &AdminReviewQueueHandler{
		Repository:   mockRepo,
		AdminService: adminService,
		AuditLogger:  audit.NewLogger(),
		Env:          "test",
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/review-queue/1/approve", bytes.NewReader([]byte("{invalid json")))
	req.SetPathValue("id", "1")
	req = withAdminUser(req, "admin")
	rec := httptest.NewRecorder()

	handler.ApproveReview(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestRejectReview_InvalidJSON tests handling of malformed JSON
func TestRejectReview_InvalidJSON(t *testing.T) {
	mockRepo := new(MockRepository)
	adminService := events.NewAdminService(mockRepo, true, "America/Toronto", config.ValidationConfig{})

	handler := &AdminReviewQueueHandler{
		Repository:   mockRepo,
		AdminService: adminService,
		AuditLogger:  audit.NewLogger(),
		Env:          "test",
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/review-queue/1/reject", bytes.NewReader([]byte("{invalid json")))
	req.SetPathValue("id", "1")
	req = withAdminUser(req, "admin")
	rec := httptest.NewRecorder()

	handler.RejectReview(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestFixReview_InvalidJSON tests handling of malformed JSON
func TestFixReview_InvalidJSON(t *testing.T) {
	mockRepo := new(MockRepository)
	adminService := events.NewAdminService(mockRepo, true, "America/Toronto", config.ValidationConfig{})

	handler := &AdminReviewQueueHandler{
		Repository:   mockRepo,
		AdminService: adminService,
		AuditLogger:  audit.NewLogger(),
		Env:          "test",
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/review-queue/1/fix", bytes.NewReader([]byte("{invalid json")))
	req.SetPathValue("id", "1")
	req = withAdminUser(req, "admin")
	rec := httptest.NewRecorder()

	handler.FixReview(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// ---------------------------------------------------------------------------
// Tests for dismissCompanionDuplicateWarning (srv-ihnz7)
// ---------------------------------------------------------------------------

// TestDismissCompanionDuplicateWarning_MatchFound verifies that the handler
// calls DismissCompanionWarningMatch with the correct ULIDs.
func TestDismissCompanionDuplicateWarning_MatchFound(t *testing.T) {
	mockRepo := new(MockRepository)
	adminService := events.NewAdminService(mockRepo, true, "America/Toronto", config.ValidationConfig{})
	handler := &AdminReviewQueueHandler{
		Repository:   mockRepo,
		AdminService: adminService,
		AuditLogger:  audit.NewLogger(),
		Env:          "test",
	}

	companionULID := "01COMPANION000000000000001"
	eventULID := "01EVENTTARGET000000000001"

	mockRepo.On("DismissCompanionWarningMatch", mock.Anything, companionULID, eventULID).Return(nil)

	handler.dismissCompanionDuplicateWarning(context.Background(), companionULID, eventULID)

	mockRepo.AssertExpectations(t)
}

// TestDismissCompanionDuplicateWarning_MultiMatch verifies that DismissCompanionWarningMatch
// is called correctly when there are multiple matches (the atomic SQL handles the filtering).
func TestDismissCompanionDuplicateWarning_MultiMatch(t *testing.T) {
	mockRepo := new(MockRepository)
	adminService := events.NewAdminService(mockRepo, true, "America/Toronto", config.ValidationConfig{})
	handler := &AdminReviewQueueHandler{
		Repository:   mockRepo,
		AdminService: adminService,
		AuditLogger:  audit.NewLogger(),
		Env:          "test",
	}

	companionULID := "01COMPANION000000000000002"
	eventULID := "01EVENTTARGET000000000002"

	mockRepo.On("DismissCompanionWarningMatch", mock.Anything, companionULID, eventULID).Return(nil)

	handler.dismissCompanionDuplicateWarning(context.Background(), companionULID, eventULID)

	mockRepo.AssertExpectations(t)
}

// TestDismissCompanionDuplicateWarning_NotFound verifies that even when the companion
// has no pending review, DismissCompanionWarningMatch is still called (the SQL is a no-op).
func TestDismissCompanionDuplicateWarning_NotFound(t *testing.T) {
	mockRepo := new(MockRepository)
	adminService := events.NewAdminService(mockRepo, true, "America/Toronto", config.ValidationConfig{})
	handler := &AdminReviewQueueHandler{
		Repository:   mockRepo,
		AdminService: adminService,
		AuditLogger:  audit.NewLogger(),
		Env:          "test",
	}

	companionULID := "01COMPANION000000000000003"
	eventULID := "01EVENTTARGET000000000003"

	mockRepo.On("DismissCompanionWarningMatch", mock.Anything, companionULID, eventULID).Return(nil)

	handler.dismissCompanionDuplicateWarning(context.Background(), companionULID, eventULID)

	mockRepo.AssertExpectations(t)
}

// TestDismissCompanionDuplicateWarning_FetchError verifies that a DB error
// is logged but does not panic.
func TestDismissCompanionDuplicateWarning_FetchError(t *testing.T) {
	mockRepo := new(MockRepository)
	adminService := events.NewAdminService(mockRepo, true, "America/Toronto", config.ValidationConfig{})
	handler := &AdminReviewQueueHandler{
		Repository:   mockRepo,
		AdminService: adminService,
		AuditLogger:  audit.NewLogger(),
		Env:          "test",
	}

	companionULID := "01COMPANION000000000000004"
	eventULID := "01EVENTTARGET000000000004"

	mockRepo.On("DismissCompanionWarningMatch", mock.Anything, companionULID, eventULID).Return(errors.New("db error"))

	// Should not panic
	handler.dismissCompanionDuplicateWarning(context.Background(), companionULID, eventULID)

	mockRepo.AssertExpectations(t)
}

// ---------------------------------------------------------------------------
// Tests for AddOccurrenceReview (srv-izykp)
// ---------------------------------------------------------------------------

func TestAddOccurrenceReview(t *testing.T) {
	targetEventULID := "01HTARGET00000000000000001"
	targetEventID := "target-event-uuid"
	now := time.Now()

	// testTargetEvent returns a published target event suitable for add-occurrence tests.
	testTargetEvent := func() *events.Event {
		return &events.Event{
			ID:             targetEventID,
			ULID:           targetEventULID,
			Name:           "Recurring Series",
			LifecycleState: "published",
		}
	}

	// testMergedReview returns a review entry in "merged" state (post-operation result).
	testMergedReview := func(id int, eventULID string) *events.ReviewQueueEntry {
		mergedStatus := "merged"
		_ = mergedStatus
		return &events.ReviewQueueEntry{
			ID:                   id,
			EventID:              "review-event-id",
			EventULID:            eventULID,
			OriginalPayload:      []byte(`{"name":"Series Event"}`),
			NormalizedPayload:    []byte(`{"name":"Series Event"}`),
			Warnings:             []byte(`[]`),
			Status:               "merged",
			DuplicateOfEventULID: &targetEventULID,
			EventStartTime:       now,
			CreatedAt:            now,
			UpdatedAt:            now,
		}
	}

	tests := []struct {
		name           string
		reviewID       string
		requestBody    any
		mockSetup      func(*MockRepository)
		expectedStatus int
	}{
		{
			name:        "Success - occurrence added",
			reviewID:    "1",
			requestBody: map[string]string{"target_event_ulid": targetEventULID},
			mockSetup: func(m *MockRepository) {
				entry := testReviewQueueEntry(1, "01HREVIEW000000000000000001")
				entry.EventStartTime = now
				m.On("GetReviewQueueEntry", mock.Anything, 1).Return(entry, nil)
				setupTxMock(m)
				m.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(entry, nil)
				m.On("GetByULID", mock.Anything, targetEventULID).Return(testTargetEvent(), nil)
				m.On("LockEventForUpdate", mock.Anything, targetEventID).Return(nil)
				m.On("GetByULID", mock.Anything, "01HREVIEW000000000000000001").Return(&events.Event{ID: "review-event-id", ULID: "01HREVIEW000000000000000001", Name: "Series Event",
					Occurrences: []events.Occurrence{{StartTime: now}}}, nil)
				m.On("LockEventForUpdate", mock.Anything, "review-event-id").Return(nil)
				m.On("CheckOccurrenceOverlap", mock.Anything, targetEventID, mock.AnythingOfType("time.Time"), mock.Anything).Return(false, nil)
				m.On("CreateOccurrence", mock.Anything, mock.Anything).Return(nil)
				m.On("SoftDeleteEvent", mock.Anything, "01HREVIEW000000000000000001", "absorbed_as_occurrence").Return(nil)
				m.On("CreateTombstone", mock.Anything, mock.Anything).Return(nil)
				m.On("MergeReview", mock.Anything, 1, "admin", targetEventULID).Return(testMergedReview(1, "01HREVIEW000000000000000001"), nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Error - missing review ID",
			reviewID:       "",
			requestBody:    map[string]string{"target_event_ulid": targetEventULID},
			mockSetup:      func(m *MockRepository) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Error - invalid review ID",
			reviewID:       "abc",
			requestBody:    map[string]string{"target_event_ulid": targetEventULID},
			mockSetup:      func(m *MockRepository) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:        "Error - missing target_event_ulid",
			reviewID:    "1",
			requestBody: map[string]string{},
			mockSetup: func(m *MockRepository) {
				// GetReviewQueueEntry is called to determine dispatch path.
				// No near-dup warning → forward path → target_event_ulid is required.
				entry := testReviewQueueEntry(1, "01HREVIEW000000000000000001")
				m.On("GetReviewQueueEntry", mock.Anything, 1).Return(entry, nil)
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:        "Error - malformed target_event_ulid",
			reviewID:    "1",
			requestBody: map[string]string{"target_event_ulid": "not-a-ulid"},
			mockSetup: func(m *MockRepository) {
				entry := testReviewQueueEntry(1, "01HREVIEW000000000000000001")
				m.On("GetReviewQueueEntry", mock.Anything, 1).Return(entry, nil)
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:        "Error - review entry not found",
			reviewID:    "999",
			requestBody: map[string]string{"target_event_ulid": targetEventULID},
			mockSetup: func(m *MockRepository) {
				// Peek returns not-found before the transaction even starts.
				m.On("GetReviewQueueEntry", mock.Anything, 999).Return(
					(*events.ReviewQueueEntry)(nil),
					events.ErrNotFound,
				)
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:        "Error - occurrence overlaps existing",
			reviewID:    "1",
			requestBody: map[string]string{"target_event_ulid": targetEventULID},
			mockSetup: func(m *MockRepository) {
				entry := testReviewQueueEntry(1, "01HREVIEW000000000000000001")
				entry.EventStartTime = now
				m.On("GetReviewQueueEntry", mock.Anything, 1).Return(entry, nil)
				setupTxMock(m)
				m.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(entry, nil)
				m.On("GetByULID", mock.Anything, targetEventULID).Return(testTargetEvent(), nil)
				m.On("LockEventForUpdate", mock.Anything, targetEventID).Return(nil)
				// Forward path: review event is fetched before the overlap check so that
				// locked occurrence timestamps (not the review-row snapshot) are used.
				m.On("GetByULID", mock.Anything, "01HREVIEW000000000000000001").Return(&events.Event{
					ID: "review-event-id", ULID: "01HREVIEW000000000000000001", Name: "Series Event",
					Occurrences: []events.Occurrence{{StartTime: now}},
				}, nil)
				m.On("LockEventForUpdate", mock.Anything, "review-event-id").Return(nil)
				m.On("CheckOccurrenceOverlap", mock.Anything, targetEventID, mock.AnythingOfType("time.Time"), mock.Anything).Return(true, nil)
			},
			expectedStatus: http.StatusConflict,
		},
		{
			name:        "Error - target event deleted",
			reviewID:    "1",
			requestBody: map[string]string{"target_event_ulid": targetEventULID},
			mockSetup: func(m *MockRepository) {
				entry := testReviewQueueEntry(1, "01HREVIEW000000000000000001")
				m.On("GetReviewQueueEntry", mock.Anything, 1).Return(entry, nil)
				setupTxMock(m)
				m.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(entry, nil)
				deleted := testTargetEvent()
				deleted.LifecycleState = "deleted"
				m.On("GetByULID", mock.Anything, targetEventULID).Return(deleted, nil)
				m.On("LockEventForUpdate", mock.Anything, targetEventID).Return(nil)
			},
			expectedStatus: http.StatusGone,
		},
		{
			name:        "Error - review already processed",
			reviewID:    "1",
			requestBody: map[string]string{"target_event_ulid": targetEventULID},
			mockSetup: func(m *MockRepository) {
				entry := testReviewQueueEntry(1, "01HREVIEW000000000000000001")
				m.On("GetReviewQueueEntry", mock.Anything, 1).Return(entry, nil)
				entry.Status = "approved"
				setupTxMock(m)
				m.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(entry, nil)
			},
			expectedStatus: http.StatusConflict,
		},
		{
			name:        "Success - target event in draft state (non-published target allowed)",
			reviewID:    "1",
			requestBody: map[string]string{"target_event_ulid": targetEventULID},
			mockSetup: func(m *MockRepository) {
				entry := testReviewQueueEntry(1, "01HREVIEW000000000000000001")
				entry.EventStartTime = now
				m.On("GetReviewQueueEntry", mock.Anything, 1).Return(entry, nil)
				setupTxMock(m)
				m.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(entry, nil)
				draftTarget := testTargetEvent()
				draftTarget.LifecycleState = "draft"
				m.On("GetByULID", mock.Anything, targetEventULID).Return(draftTarget, nil)
				m.On("LockEventForUpdate", mock.Anything, targetEventID).Return(nil)
				m.On("GetByULID", mock.Anything, "01HREVIEW000000000000000001").Return(&events.Event{ID: "review-event-id", ULID: "01HREVIEW000000000000000001", Name: "Series Event",
					Occurrences: []events.Occurrence{{StartTime: now}}}, nil)
				m.On("LockEventForUpdate", mock.Anything, "review-event-id").Return(nil)
				m.On("CheckOccurrenceOverlap", mock.Anything, targetEventID, mock.AnythingOfType("time.Time"), mock.Anything).Return(false, nil)
				m.On("CreateOccurrence", mock.Anything, mock.Anything).Return(nil)
				m.On("SoftDeleteEvent", mock.Anything, "01HREVIEW000000000000000001", "absorbed_as_occurrence").Return(nil)
				m.On("CreateTombstone", mock.Anything, mock.Anything).Return(nil)
				m.On("MergeReview", mock.Anything, 1, "admin", targetEventULID).Return(testMergedReview(1, "01HREVIEW000000000000000001"), nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:        "Error - review event same as target (400)",
			reviewID:    "1",
			requestBody: map[string]string{"target_event_ulid": targetEventULID},
			mockSetup: func(m *MockRepository) {
				// The review entry's EventULID matches targetEventULID → ErrCannotMergeSameEvent
				entry := testReviewQueueEntry(1, targetEventULID)
				entry.EventStartTime = now
				m.On("GetReviewQueueEntry", mock.Anything, 1).Return(entry, nil)
				setupTxMock(m)
				m.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(entry, nil)
				m.On("GetByULID", mock.Anything, targetEventULID).Return(testTargetEvent(), nil)
				m.On("LockEventForUpdate", mock.Anything, targetEventID).Return(nil)
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockRepository)
			tt.mockSetup(mockRepo)

			adminService := events.NewAdminService(mockRepo, true, "America/Toronto", config.ValidationConfig{})
			handler := &AdminReviewQueueHandler{
				Repository:   mockRepo,
				AdminService: adminService,
				AuditLogger:  audit.NewLogger(),
				Env:          "test",
			}

			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/admin/review-queue/"+tt.reviewID+"/add-occurrence", bytes.NewReader(body))
			if tt.reviewID != "" {
				req.SetPathValue("id", tt.reviewID)
			}
			req = withAdminUser(req, "admin")
			rec := httptest.NewRecorder()

			handler.AddOccurrenceReview(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code)
			mockRepo.AssertExpectations(t)
		})
	}
}

// TestAddOccurrenceReviewNearDupPath tests the near_duplicate_of_new_event dispatch
// path and verifies that targetEventUlid is returned in the response.
func TestAddOccurrenceReviewNearDupPath(t *testing.T) {
	targetEventULID := "01HTARGET00000000000000001"
	targetEventID := "target-event-uuid"
	sourceEventULID := "01HSOURCE00000000000000001"
	now := time.Now()

	// nearDupEntry returns a review entry with a near_duplicate_of_new_event warning
	// and DuplicateOfEventULID pointing to the source (newly ingested) event.
	nearDupEntry := func() *events.ReviewQueueEntry {
		warningJSON, _ := json.Marshal([]events.ValidationWarning{
			{Code: "near_duplicate_of_new_event"},
		})
		return &events.ReviewQueueEntry{
			ID:                   1,
			EventID:              "target-event-id",
			EventULID:            targetEventULID,
			DuplicateOfEventULID: &sourceEventULID,
			OriginalPayload:      []byte(`{"name":"Series Event"}`),
			NormalizedPayload:    []byte(`{"name":"Series Event"}`),
			Warnings:             warningJSON,
			Status:               "pending",
			EventStartTime:       now,
			CreatedAt:            now,
			UpdatedAt:            now,
		}
	}

	mergedEntry := func() *events.ReviewQueueEntry {
		e := nearDupEntry()
		e.Status = "merged"
		e.DuplicateOfEventULID = &targetEventULID
		return e
	}

	tests := []struct {
		name           string
		requestBody    any
		mockSetup      func(*MockRepository)
		expectedStatus int
		checkResponse  func(t *testing.T, body []byte)
	}{
		{
			name:        "Near-dup path: success — no target_event_ulid required",
			requestBody: map[string]string{}, // no target_event_ulid
			mockSetup: func(m *MockRepository) {
				// Handler peeks the entry first.
				m.On("GetReviewQueueEntry", mock.Anything, 1).Return(nearDupEntry(), nil)
				// Service transaction.
				setupTxMock(m)
				m.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(nearDupEntry(), nil)
				// GetByULID called twice for target (pre-lock + post-lock) and once for source.
				m.On("GetByULID", mock.Anything, targetEventULID).Return(
					&events.Event{ID: targetEventID, ULID: targetEventULID, Name: "Series", LifecycleState: "published"}, nil)
				m.On("GetByULID", mock.Anything, sourceEventULID).Return(
					&events.Event{ID: "source-id", ULID: sourceEventULID, Name: "New Instance",
						Occurrences: []events.Occurrence{{StartTime: now}}}, nil)
				m.On("LockEventForUpdate", mock.Anything, targetEventID).Return(nil)
				m.On("CheckOccurrenceOverlap", mock.Anything, targetEventID, mock.AnythingOfType("time.Time"), mock.Anything).Return(false, nil)
				m.On("CreateOccurrence", mock.Anything, mock.Anything).Return(nil)
				m.On("SoftDeleteEvent", mock.Anything, sourceEventULID, "absorbed_as_occurrence").Return(nil)
				m.On("CreateTombstone", mock.Anything, mock.Anything).Return(nil)
				m.On("GetPendingReviewByEventUlid", mock.Anything, sourceEventULID).Return((*events.ReviewQueueEntry)(nil), nil)
				m.On("LockEventForUpdate", mock.Anything, "source-id").Return(nil)
				m.On("MergeReview", mock.Anything, 1, "admin", targetEventULID).Return(mergedEntry(), nil)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body []byte) {
				var resp addOccurrenceResponse
				assert.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, targetEventULID, resp.TargetEventULID,
					"targetEventUlid should be the existing series ULID")
			},
		},
		{
			name:        "Near-dup path: overlap returns 409",
			requestBody: map[string]string{},
			mockSetup: func(m *MockRepository) {
				m.On("GetReviewQueueEntry", mock.Anything, 1).Return(nearDupEntry(), nil)
				setupTxMock(m)
				m.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(nearDupEntry(), nil)
				m.On("GetPendingReviewByEventUlid", mock.Anything, sourceEventULID).Return((*events.ReviewQueueEntry)(nil), nil)
				m.On("GetByULID", mock.Anything, targetEventULID).Return(
					&events.Event{ID: targetEventID, ULID: targetEventULID, Name: "Series", LifecycleState: "published"}, nil)
				m.On("GetByULID", mock.Anything, sourceEventULID).Return(
					&events.Event{ID: "source-id", ULID: sourceEventULID, Name: "New Instance",
						Occurrences: []events.Occurrence{{StartTime: now}}}, nil)
				m.On("LockEventForUpdate", mock.Anything, targetEventID).Return(nil)
				m.On("LockEventForUpdate", mock.Anything, "source-id").Return(nil)
				m.On("CheckOccurrenceOverlap", mock.Anything, targetEventID, mock.AnythingOfType("time.Time"), mock.Anything).Return(true, nil)
			},
			expectedStatus: http.StatusConflict,
		},
		{
			name:        "Near-dup path: multi-occurrence source returns 422",
			requestBody: map[string]string{},
			mockSetup: func(m *MockRepository) {
				m.On("GetReviewQueueEntry", mock.Anything, 1).Return(nearDupEntry(), nil)
				setupTxMock(m)
				m.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(nearDupEntry(), nil)
				m.On("GetPendingReviewByEventUlid", mock.Anything, sourceEventULID).Return((*events.ReviewQueueEntry)(nil), nil)
				m.On("GetByULID", mock.Anything, targetEventULID).Return(
					&events.Event{ID: targetEventID, ULID: targetEventULID, Name: "Series", LifecycleState: "published"}, nil)
				m.On("LockEventForUpdate", mock.Anything, targetEventID).Return(nil)
				// Source event has two occurrences → ambiguous
				m.On("GetByULID", mock.Anything, sourceEventULID).Return(
					&events.Event{ID: "source-id", ULID: sourceEventULID, Name: "Multi-occ Source",
						Occurrences: []events.Occurrence{
							{StartTime: now},
							{StartTime: now.Add(24 * time.Hour)},
						}}, nil)
				m.On("LockEventForUpdate", mock.Anything, "source-id").Return(nil)
			},
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:        "Forward path still works with target_event_ulid",
			requestBody: map[string]string{"target_event_ulid": targetEventULID},
			mockSetup: func(m *MockRepository) {
				// No near_duplicate_of_new_event warning → forward path.
				fwdEntry := testReviewQueueEntry(1, "01HREVIEW000000000000000001")
				m.On("GetReviewQueueEntry", mock.Anything, 1).Return(fwdEntry, nil)
				setupTxMock(m)
				m.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(fwdEntry, nil)
				m.On("GetByULID", mock.Anything, targetEventULID).Return(
					&events.Event{ID: targetEventID, ULID: targetEventULID, Name: "Series", LifecycleState: "published"}, nil)
				m.On("LockEventForUpdate", mock.Anything, targetEventID).Return(nil)
				m.On("GetByULID", mock.Anything, "01HREVIEW000000000000000001").Return(
					&events.Event{ID: "review-event-id", ULID: "01HREVIEW000000000000000001", Name: "Instance",
						Occurrences: []events.Occurrence{{StartTime: now}}}, nil)
				m.On("LockEventForUpdate", mock.Anything, "review-event-id").Return(nil)
				m.On("CheckOccurrenceOverlap", mock.Anything, targetEventID, mock.AnythingOfType("time.Time"), mock.Anything).Return(false, nil)
				m.On("CreateOccurrence", mock.Anything, mock.Anything).Return(nil)
				m.On("SoftDeleteEvent", mock.Anything, "01HREVIEW000000000000000001", "absorbed_as_occurrence").Return(nil)
				m.On("CreateTombstone", mock.Anything, mock.Anything).Return(nil)
				m.On("MergeReview", mock.Anything, 1, "admin", targetEventULID).Return(
					&events.ReviewQueueEntry{ID: 1, EventID: "review-event-id", EventULID: "01HREVIEW000000000000000001",
						OriginalPayload: []byte(`{"name":"Series Event"}`), NormalizedPayload: []byte(`{"name":"Series Event"}`),
						Warnings: []byte(`[]`), Status: "merged", DuplicateOfEventULID: &targetEventULID,
						EventStartTime: now, CreatedAt: now, UpdatedAt: now}, nil)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body []byte) {
				var resp addOccurrenceResponse
				assert.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, targetEventULID, resp.TargetEventULID,
					"targetEventUlid should equal the supplied target ULID")
			},
		},
		{
			name:        "Near-dup path: missing DuplicateOfEventULID returns 400",
			requestBody: map[string]string{},
			mockSetup: func(m *MockRepository) {
				entry := nearDupEntry()
				entry.DuplicateOfEventULID = nil // broken entry
				m.On("GetReviewQueueEntry", mock.Anything, 1).Return(entry, nil)
				setupTxMock(m)
				m.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(entry, nil)
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockRepository)
			tt.mockSetup(mockRepo)

			adminService := events.NewAdminService(mockRepo, true, "America/Toronto", config.ValidationConfig{})
			handler := &AdminReviewQueueHandler{
				Repository:   mockRepo,
				AdminService: adminService,
				AuditLogger:  audit.NewLogger(),
				Env:          "test",
			}

			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/admin/review-queue/1/add-occurrence", bytes.NewReader(body))
			req.SetPathValue("id", "1")
			req = withAdminUser(req, "admin")
			rec := httptest.NewRecorder()

			handler.AddOccurrenceReview(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code)
			if tt.checkResponse != nil && rec.Code == http.StatusOK {
				tt.checkResponse(t, rec.Body.Bytes())
			}
			mockRepo.AssertExpectations(t)
		})
	}
}

// TestAddOccurrenceReview_BothWarningsRejected verifies that when a review entry
// carries BOTH potential_duplicate and near_duplicate_of_new_event warnings the
// handler returns 422 without touching the AdminService — the dispatch path is
// ambiguous and requires manual resolution.
func TestAddOccurrenceReview_BothWarningsRejected(t *testing.T) {
	targetEventULID := "01HTARGET00000000000000001"
	sourceEventULID := "01HSOURCE00000000000000001"
	now := time.Now()

	// Build a review entry that carries both warning types.
	bothWarnings, _ := json.Marshal([]events.ValidationWarning{
		{Code: "potential_duplicate"},
		{Code: "near_duplicate_of_new_event"},
	})
	ambiguousEntry := &events.ReviewQueueEntry{
		ID:                   1,
		EventID:              "target-event-id",
		EventULID:            targetEventULID,
		DuplicateOfEventULID: &sourceEventULID,
		OriginalPayload:      []byte(`{"name":"Series Event"}`),
		NormalizedPayload:    []byte(`{"name":"Series Event"}`),
		Warnings:             bothWarnings,
		Status:               "pending",
		EventStartTime:       now,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	mockRepo := new(MockRepository)
	// GetReviewQueueEntry is called by the handler to peek the warnings; no TX needed.
	mockRepo.On("GetReviewQueueEntry", mock.Anything, 1).Return(ambiguousEntry, nil)

	adminService := events.NewAdminService(mockRepo, true, "America/Toronto", config.ValidationConfig{})
	handler := &AdminReviewQueueHandler{
		Repository:   mockRepo,
		AdminService: adminService,
		AuditLogger:  audit.NewLogger(),
		Env:          "test",
	}

	body, _ := json.Marshal(map[string]string{"target_event_ulid": targetEventULID})
	req := httptest.NewRequest(http.MethodPost, "/admin/review-queue/1/add-occurrence", bytes.NewReader(body))
	req.SetPathValue("id", "1")
	req = withAdminUser(req, "admin")
	rec := httptest.NewRecorder()

	handler.AddOccurrenceReview(rec, req)

	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	mockRepo.AssertExpectations(t)
}

// TestAddOccurrenceReview_ZeroOccurrenceSourceForwardPath verifies that when the
// review (source) event has no occurrences the forward-path handler returns 422.
func TestAddOccurrenceReview_ZeroOccurrenceSourceForwardPath(t *testing.T) {
	targetEventULID := "01HTARGET00000000000000001"
	targetEventID := "target-event-uuid"
	now := time.Now()

	fwdEntry := testReviewQueueEntry(1, "01HREVIEW000000000000000001")
	fwdEntry.EventStartTime = now

	mockRepo := new(MockRepository)
	mockRepo.On("GetReviewQueueEntry", mock.Anything, 1).Return(fwdEntry, nil)
	setupTxMock(mockRepo)
	mockRepo.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(fwdEntry, nil)
	mockRepo.On("GetByULID", mock.Anything, targetEventULID).Return(
		&events.Event{ID: targetEventID, ULID: targetEventULID, Name: "Series", LifecycleState: "published"}, nil)
	mockRepo.On("LockEventForUpdate", mock.Anything, targetEventID).Return(nil)
	// Source event fetched BEFORE the overlap check (so locked occurrence timestamps are
	// used, not the review-row snapshot).  Zero occurrences → ErrZeroOccurrenceSource.
	// CheckOccurrenceOverlap is NOT called because the method returns early.
	mockRepo.On("GetByULID", mock.Anything, "01HREVIEW000000000000000001").Return(
		&events.Event{ID: "review-event-id", ULID: "01HREVIEW000000000000000001", Name: "Instance",
			Occurrences: []events.Occurrence{}}, nil)
	mockRepo.On("LockEventForUpdate", mock.Anything, "review-event-id").Return(nil)

	adminService := events.NewAdminService(mockRepo, true, "America/Toronto", config.ValidationConfig{})
	handler := &AdminReviewQueueHandler{
		Repository:   mockRepo,
		AdminService: adminService,
		AuditLogger:  audit.NewLogger(),
		Env:          "test",
	}

	body, _ := json.Marshal(map[string]string{"target_event_ulid": targetEventULID})
	req := httptest.NewRequest(http.MethodPost, "/admin/review-queue/1/add-occurrence", bytes.NewReader(body))
	req.SetPathValue("id", "1")
	req = withAdminUser(req, "admin")
	rec := httptest.NewRecorder()

	handler.AddOccurrenceReview(rec, req)

	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	mockRepo.AssertExpectations(t)
}

// TestAddOccurrenceReview_ZeroOccurrenceSourceNearDupPath verifies that when the
// source (newly-ingested) event has no occurrences the near-dup-path handler
// returns 422 rather than absorbing the target's own timestamps.
func TestAddOccurrenceReview_ZeroOccurrenceSourceNearDupPath(t *testing.T) {
	targetEventULID := "01HTARGET00000000000000001"
	targetEventID := "target-event-uuid"
	sourceEventULID := "01HSOURCE00000000000000001"
	now := time.Now()

	warningJSON, _ := json.Marshal([]events.ValidationWarning{
		{Code: "near_duplicate_of_new_event"},
	})
	nearDupEntry := &events.ReviewQueueEntry{
		ID:                   1,
		EventID:              "target-event-id",
		EventULID:            targetEventULID,
		DuplicateOfEventULID: &sourceEventULID,
		OriginalPayload:      []byte(`{"name":"Series Event"}`),
		NormalizedPayload:    []byte(`{"name":"Series Event"}`),
		Warnings:             warningJSON,
		Status:               "pending",
		EventStartTime:       now,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	mockRepo := new(MockRepository)
	mockRepo.On("GetReviewQueueEntry", mock.Anything, 1).Return(nearDupEntry, nil)
	setupTxMock(mockRepo)
	mockRepo.On("LockReviewQueueEntryForUpdate", mock.Anything, 1).Return(nearDupEntry, nil)
	mockRepo.On("GetPendingReviewByEventUlid", mock.Anything, sourceEventULID).Return((*events.ReviewQueueEntry)(nil), nil)
	mockRepo.On("GetByULID", mock.Anything, targetEventULID).Return(
		&events.Event{ID: targetEventID, ULID: targetEventULID, Name: "Series", LifecycleState: "published"}, nil)
	mockRepo.On("LockEventForUpdate", mock.Anything, targetEventID).Return(nil)
	// Source event has zero occurrences — should be rejected.
	mockRepo.On("GetByULID", mock.Anything, sourceEventULID).Return(
		&events.Event{ID: "source-id", ULID: sourceEventULID, Name: "New Instance",
			Occurrences: []events.Occurrence{}}, nil)
	mockRepo.On("LockEventForUpdate", mock.Anything, "source-id").Return(nil)

	adminService := events.NewAdminService(mockRepo, true, "America/Toronto", config.ValidationConfig{})
	handler := &AdminReviewQueueHandler{
		Repository:   mockRepo,
		AdminService: adminService,
		AuditLogger:  audit.NewLogger(),
		Env:          "test",
	}

	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest(http.MethodPost, "/admin/review-queue/1/add-occurrence", bytes.NewReader(body))
	req.SetPathValue("id", "1")
	req = withAdminUser(req, "admin")
	rec := httptest.NewRecorder()

	handler.AddOccurrenceReview(rec, req)

	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	mockRepo.AssertExpectations(t)
}
