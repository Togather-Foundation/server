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
					pgx.ErrNoRows,
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
					pgx.ErrNoRows,
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

			adminService := events.NewAdminService(mockRepo, true)
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
					pgx.ErrNoRows,
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

			adminService := events.NewAdminService(mockRepo, true)
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
					pgx.ErrNoRows,
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

			adminService := events.NewAdminService(mockRepo, true)
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
	adminService := events.NewAdminService(mockRepo, true)

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
	adminService := events.NewAdminService(mockRepo, true)

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
	adminService := events.NewAdminService(mockRepo, true)

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
